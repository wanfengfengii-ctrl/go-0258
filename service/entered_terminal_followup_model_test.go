package service

import (
	"context"
	"sync"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func TestModel_EnteredTerminalFollowup(t *testing.T) {
	ctx := context.Background()

	newAdmissibleTask := func(t *testing.T) (*Service, inspection.TaskID, func()) {
		t.Helper()
		svc, st := newFixtureService(t)
		id := createFixtureTask(t, svc)
		advanceToReview(t, svc, id)
		passReviews(t, svc, id)
		return svc, id, func() { _ = st.Close() }
	}

	finalizeAdmissible := func(t *testing.T, svc *Service, id inspection.TaskID) *FinalizeResult {
		t.Helper()
		result, fault := svc.Finalize(ctx, id, FinalizeRequest{
			OperationID: "op-final-admissible",
			Generation:  1,
			Outcome:     inspection.FinalAdmissible,
		})
		if fault != nil {
			t.Fatalf("finalize admissible: %v", fault)
		}
		if result == nil || result.FinalType != inspection.FinalAdmissible || result.Credential == "" {
			t.Fatalf("admissible result = %+v, want admissible credential", result)
		}
		return result
	}

	finalizeEntered := func(t *testing.T, svc *Service, id inspection.TaskID, op inspection.OperationID) *FinalizeResult {
		t.Helper()
		result, fault := svc.Finalize(ctx, id, FinalizeRequest{
			OperationID: op,
			Generation:  1,
			Outcome:     inspection.FinalEntered,
		})
		if fault != nil {
			t.Fatalf("finalize entered: %v", fault)
		}
		if result == nil || result.FinalType != inspection.FinalEntered || result.Credential == "" {
			t.Fatalf("entered result = %+v, want entered credential", result)
		}
		return result
	}

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "admissible advances to entered and persists auditable terminal state",
			run: func(t *testing.T) {
				svc, id, closeStore := newAdmissibleTask(t)
				defer closeStore()

				finalizeAdmissible(t, svc, id)
				entered := finalizeEntered(t, svc, id, "op-final-entered")

				snap, fault := svc.GetSnapshot(ctx, id)
				if fault != nil {
					t.Fatalf("snapshot: %v", fault)
				}
				if snap.Task.Status != inspection.StatusEntered {
					t.Fatalf("status = %s, want entered", snap.Task.Status)
				}
				if snap.Task.FinalType != inspection.FinalEntered {
					t.Fatalf("task final type = %s, want entered", snap.Task.FinalType)
				}
				if snap.FinalDecision == nil {
					t.Fatal("final decision missing after entered")
				}
				if snap.FinalDecision.FinalType != inspection.FinalEntered {
					t.Fatalf("final decision type = %s, want entered", snap.FinalDecision.FinalType)
				}
				if snap.FinalDecision.Credential != entered.Credential {
					t.Fatalf("persisted credential = %q, want entered result credential %q", snap.FinalDecision.Credential, entered.Credential)
				}

				finalized := map[inspection.FinalType]int{}
				for _, ev := range snap.Audit {
					if ev.EventType == inspection.EventFinalized {
						finalized[inspection.FinalType(ev.Detail)]++
					}
				}
				if finalized[inspection.FinalAdmissible] != 1 || finalized[inspection.FinalEntered] != 1 {
					t.Fatalf("finalized audit counts = %v, want one admissible and one entered", finalized)
				}
			},
		},
		{
			name: "entered is rejected before admissible",
			run: func(t *testing.T) {
				svc, st := newFixtureService(t)
				defer st.Close()
				id := createFixtureTask(t, svc)

				result, fault := svc.Finalize(ctx, id, FinalizeRequest{
					OperationID: "op-final-entered",
					Generation:  1,
					Outcome:     inspection.FinalEntered,
				})
				if result != nil {
					t.Fatalf("result = %+v, want nil", result)
				}
				if fault == nil || fault.Code != CodeFinalizeConflict {
					t.Fatalf("fault = %v, want finalize_conflict", fault)
				}

				snap, snapFault := svc.GetSnapshot(ctx, id)
				if snapFault != nil {
					t.Fatalf("snapshot: %v", snapFault)
				}
				if snap.Task.Status != inspection.StatusPendingSampling {
					t.Fatalf("status = %s, want pending_sampling", snap.Task.Status)
				}
			},
		},
		{
			name: "concurrent entered finalization has one winner",
			run: func(t *testing.T) {
				svc, id, closeStore := newAdmissibleTask(t)
				defer closeStore()
				finalizeAdmissible(t, svc, id)

				const contenders = 8
				start := make(chan struct{})
				var wg sync.WaitGroup
				results := make([]*FinalizeResult, contenders)
				faults := make([]*Fault, contenders)

				for i := 0; i < contenders; i++ {
					i := i
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						results[i], faults[i] = svc.Finalize(ctx, id, FinalizeRequest{
							OperationID: inspection.OperationID("op-final-entered-race-" + strconvItoa(i)),
							Generation:  1,
							Outcome:     inspection.FinalEntered,
						})
					}()
				}
				close(start)
				wg.Wait()

				winners := 0
				conflicts := 0
				for i := range results {
					if faults[i] == nil {
						winners++
						if results[i] == nil || results[i].FinalType != inspection.FinalEntered || results[i].Credential == "" {
							t.Fatalf("result[%d] = %+v, want entered credential", i, results[i])
						}
						continue
					}
					if faults[i].Code != CodeFinalizeConflict {
						t.Fatalf("fault[%d] = %v, want nil or finalize_conflict", i, faults[i])
					}
					conflicts++
				}
				if winners != 1 || conflicts != contenders-1 {
					t.Fatalf("winners=%d conflicts=%d, want 1 winner and %d conflicts", winners, conflicts, contenders-1)
				}

				snap, fault := svc.GetSnapshot(ctx, id)
				if fault != nil {
					t.Fatalf("snapshot: %v", fault)
				}
				if snap.Task.Status != inspection.StatusEntered {
					t.Fatalf("status = %s, want entered", snap.Task.Status)
				}
				if snap.FinalDecision == nil || snap.FinalDecision.FinalType != inspection.FinalEntered {
					t.Fatalf("final decision = %+v, want entered", snap.FinalDecision)
				}
			},
		},
		{
			name: "ordinary command after entered is still terminal state",
			run: func(t *testing.T) {
				svc, id, closeStore := newAdmissibleTask(t)
				defer closeStore()

				finalizeAdmissible(t, svc, id)
				finalizeEntered(t, svc, id, "op-final-entered")

				_, fault := svc.Review(ctx, id, ReviewRequest{
					OperationID: "op-review-after-entered",
					Generation:  1,
					Reviewer:    "person-reviewer-a",
					Conclusion:  "pass",
				})
				if fault == nil || fault.Code != CodeTerminalState {
					t.Fatalf("fault = %v, want terminal_state", fault)
				}

				snap, snapFault := svc.GetSnapshot(ctx, id)
				if snapFault != nil {
					t.Fatalf("snapshot: %v", snapFault)
				}
				if snap.Task.Status != inspection.StatusEntered {
					t.Fatalf("status = %s, want entered", snap.Task.Status)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}
