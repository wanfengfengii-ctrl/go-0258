package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_BlindSplitDuplicateBatchConflictRollsBack(t *testing.T) {
	ctx := context.Background()
	seedID := inspection.TaskID("task-seed-blind-conflict")
	splitOp := inspection.OperationID("op-candidate-blind-split")

	cases := []struct {
		name            string
		taskID          inspection.TaskID
		tankBatch       inspection.TankBatch
		compartments    []catalog.CompartmentCode
		seals           []catalog.SealCode
		codes           []blindcode.BlindCode
		wantFault       string
		wantStatus      inspection.Status
		wantBlinds      int
		wantIdempotency bool
		wantBlindAudit  bool
	}{
		{
			name:         "duplicate batch compartment rolls back prior inserts",
			taskID:       "task-duplicate-blind-conflict",
			tankBatch:    "BATCH-BLIND-LOCKED",
			compartments: []catalog.CompartmentCode{"B", "A"},
			seals:        []catalog.SealCode{"seal-0002", "seal-0001"},
			codes:        []blindcode.BlindCode{"NEW-BLIND-B", "NEW-BLIND-A"},
			wantFault:    CodeDuplicateBlind,
			wantStatus:   inspection.StatusBlindSplitting,
		},
		{
			name:         "reused blind code rolls back prior inserts",
			taskID:       "task-reused-blind-code",
			tankBatch:    "BATCH-BLIND-REUSED-CODE",
			compartments: []catalog.CompartmentCode{"B", "A"},
			seals:        []catalog.SealCode{"seal-0002", "seal-0001"},
			codes:        []blindcode.BlindCode{"NEW-BLIND-FOR-ROLLBACK", "LOCKED-BLIND-A"},
			wantFault:    CodeBlindReuse,
			wantStatus:   inspection.StatusBlindSplitting,
		},
		{
			name:            "fresh batch compartment split still succeeds",
			taskID:          "task-fresh-blind-split",
			tankBatch:       "BATCH-BLIND-FRESH",
			compartments:    []catalog.CompartmentCode{"B"},
			seals:           []catalog.SealCode{"seal-0002"},
			codes:           []blindcode.BlindCode{"FRESH-BLIND-B"},
			wantStatus:      inspection.StatusPlateOccupied,
			wantBlinds:      1,
			wantIdempotency: true,
			wantBlindAudit:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			defer st.Close()
			svc := NewService(st, NewManualClock(1000))

			createAndSample := func(id inspection.TaskID, batch inspection.TankBatch, compartments []catalog.CompartmentCode, seals []catalog.SealCode) {
				t.Helper()
				if _, fault := svc.CreateTask(ctx, CreateTaskRequest{
					TaskID:        id,
					FarmID:        catalog.FixedFarmID,
					TankBatch:     batch,
					Compartments:  compartments,
					Seals:         seals,
					RecorderModel: "recorder-x1",
					RuleVersion:   catalog.FixedRuleVersion,
					Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
				}); fault != nil {
					t.Fatalf("create task %s: %v", id, fault)
				}
				for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
					if _, fault := svc.SamplingConfirm(ctx, id, SamplingConfirmationRequest{
						OperationID:  inspection.OperationID(string(id) + "-sample-" + []string{"a", "b"}[i]),
						Person:       sampler,
						FarmID:       catalog.FixedFarmID,
						TankBatch:    batch,
						Compartments: compartments,
						Seals:        seals,
						Generation:   1,
					}); fault != nil {
						t.Fatalf("sample task %s by %s: %v", id, sampler, fault)
					}
				}
			}

			createAndSample(seedID, "BATCH-BLIND-LOCKED", []catalog.CompartmentCode{"A"}, []catalog.SealCode{"seal-0001"})
			if _, fault := svc.BlindSplit(ctx, seedID, BlindSplitRequest{
				OperationID: "op-seed-blind-split",
				Generation:  1,
				Codes:       []blindcode.BlindCode{"LOCKED-BLIND-A"},
			}); fault != nil {
				t.Fatalf("seed blind split: %v", fault)
			}

			createAndSample(tc.taskID, tc.tankBatch, tc.compartments, tc.seals)
			result, fault := svc.BlindSplit(ctx, tc.taskID, BlindSplitRequest{
				OperationID: splitOp,
				Generation:  1,
				Codes:       tc.codes,
			})
			if tc.wantFault != "" {
				if fault == nil || fault.Code != tc.wantFault {
					t.Fatalf("fault = %v, want %s", fault, tc.wantFault)
				}
				if result != nil {
					t.Fatalf("result = %+v, want nil on failed split", result)
				}
			} else if fault != nil {
				t.Fatalf("blind split: %v", fault)
			}

			snap, snapFault := svc.GetSnapshot(ctx, tc.taskID)
			if snapFault != nil {
				t.Fatalf("snapshot: %v", snapFault)
			}
			if snap.Task.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s", snap.Task.Status, tc.wantStatus)
			}
			if len(snap.BlindSamples) != tc.wantBlinds {
				t.Fatalf("blind samples = %d, want %d", len(snap.BlindSamples), tc.wantBlinds)
			}

			gotBlindAudit := false
			for _, ev := range snap.Audit {
				if ev.EventType == inspection.EventBlindSplit {
					gotBlindAudit = true
				}
			}
			if gotBlindAudit != tc.wantBlindAudit {
				t.Fatalf("blind audit event = %t, want %t", gotBlindAudit, tc.wantBlindAudit)
			}

			var gotIdempotency bool
			if err := st.WithTx(ctx, func(tx store.Tx) error {
				_, exists, err := tx.GetIdempotency(ctx, tc.taskID, splitOp)
				gotIdempotency = exists
				return err
			}); err != nil {
				t.Fatalf("read idempotency: %v", err)
			}
			if gotIdempotency != tc.wantIdempotency {
				t.Fatalf("idempotency record = %t, want %t", gotIdempotency, tc.wantIdempotency)
			}
		})
	}
}
