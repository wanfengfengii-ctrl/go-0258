package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_FinalizeManualOverridesFromPendingReview(t *testing.T) {
	ctx := context.Background()

	assertUnfinalizedPendingReview := func(t *testing.T, svc *Service, id inspection.TaskID) {
		t.Helper()
		snap, fault := svc.GetSnapshot(ctx, id)
		if fault != nil {
			t.Fatalf("snapshot: %v", fault)
		}
		if snap.Task.Status != inspection.StatusPendingReview {
			t.Fatalf("status = %s, want %s", snap.Task.Status, inspection.StatusPendingReview)
		}
		if snap.Task.FinalType != "" {
			t.Fatalf("final type = %s, want empty", snap.Task.FinalType)
		}
		if snap.FinalDecision != nil {
			t.Fatalf("final decision = %+v, want nil", snap.FinalDecision)
		}
	}

	assertFinalizedAudit := func(t *testing.T, audit []inspection.AuditEvent, outcome inspection.FinalType) {
		t.Helper()
		count := 0
		for _, ev := range audit {
			if ev.EventType == inspection.EventFinalized {
				count++
				if ev.Detail != string(outcome) {
					t.Fatalf("finalized audit detail = %q, want %q", ev.Detail, outcome)
				}
			}
		}
		if count != 1 {
			t.Fatalf("finalized audit events = %d, want 1", count)
		}
	}

	assertFinalizeIdempotency := func(t *testing.T, st *store.MemoryStore, id inspection.TaskID, req FinalizeRequest) {
		t.Helper()
		var (
			rec inspection.IdempotencyRecord
			ok  bool
		)
		if err := st.WithTx(ctx, func(tx store.Tx) error {
			var err error
			rec, ok, err = tx.GetIdempotency(ctx, id, req.OperationID)
			return err
		}); err != nil {
			t.Fatalf("get idempotency: %v", err)
		}
		if !ok {
			t.Fatal("finalize idempotency record missing")
		}
		if rec.OperationType != inspection.OpFinalize {
			t.Fatalf("operation type = %s, want %s", rec.OperationType, inspection.OpFinalize)
		}
		if rec.RequestDigest != inspection.DigestOf(req) {
			t.Fatalf("request digest = %s, want %s", rec.RequestDigest, inspection.DigestOf(req))
		}
	}

	cases := []struct {
		name          string
		outcome       inspection.FinalType
		wantStatus    inspection.Status
		addReviews    bool
		wantFaultCode string
	}{
		{
			name:       "cancelled bypasses missing reviews",
			outcome:    inspection.FinalCancelled,
			wantStatus: inspection.StatusCancelled,
		},
		{
			name:       "quarantined bypasses admissible arbiter conclusion",
			outcome:    inspection.FinalQuarantined,
			wantStatus: inspection.StatusQuarantined,
			addReviews: true,
		},
		{
			name:          "admissible still requires review thresholds",
			outcome:       inspection.FinalAdmissible,
			wantFaultCode: CodeFinalizeConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()
			id := createFixtureTask(t, svc)
			advanceToReview(t, svc, id)
			if tc.addReviews {
				passReviews(t, svc, id)
			}

			req := FinalizeRequest{
				OperationID: inspection.OperationID("op-final-" + string(tc.outcome)),
				Generation:  1,
				Outcome:     tc.outcome,
			}
			result, fault := svc.Finalize(ctx, id, req)
			if tc.wantFaultCode != "" {
				if fault == nil || fault.Code != tc.wantFaultCode {
					t.Fatalf("fault = %v, want %s", fault, tc.wantFaultCode)
				}
				assertUnfinalizedPendingReview(t, svc, id)
				return
			}
			if fault != nil {
				t.Fatalf("finalize: %v", fault)
			}
			if result.FinalType != tc.outcome || result.Credential == "" {
				t.Fatalf("result = %+v, want %s with credential", result, tc.outcome)
			}

			snap, fault := svc.GetSnapshot(ctx, id)
			if fault != nil {
				t.Fatalf("snapshot: %v", fault)
			}
			if snap.Task.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s", snap.Task.Status, tc.wantStatus)
			}
			if snap.Task.FinalType != tc.outcome {
				t.Fatalf("task final type = %s, want %s", snap.Task.FinalType, tc.outcome)
			}
			if snap.FinalDecision == nil {
				t.Fatal("final decision missing")
			}
			if snap.FinalDecision.FinalType != tc.outcome || snap.FinalDecision.Credential != result.Credential {
				t.Fatalf("final decision = %+v, want %s with credential %q", snap.FinalDecision, tc.outcome, result.Credential)
			}
			assertFinalizedAudit(t, snap.Audit, tc.outcome)
			assertFinalizeIdempotency(t, st, id, req)

			_, fault = svc.Review(ctx, id, ReviewRequest{
				OperationID: inspection.OperationID("op-late-review-" + string(tc.outcome)),
				Generation:  1,
				Reviewer:    "person-reviewer-a",
				Conclusion:  "pass",
			})
			if fault == nil || fault.Code != CodeTerminalState {
				t.Fatalf("late review fault = %v, want %s", fault, CodeTerminalState)
			}
		})
	}
}
