package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_OccupancyAcquireBatchRollback(t *testing.T) {
	ctx := context.Background()
	plateWell := func(plateID, well string) occupancy.Occupancy {
		return occupancy.Occupancy{
			ResourceType: occupancy.ResourcePlateWell,
			PlateID:      plateID,
			Well:         well,
			StartAt:      0,
			EndAt:        3600,
		}
	}
	invalidLease := plateWell("plate-invalid", "C1")
	invalidLease.EndAt = invalidLease.StartAt

	cases := []struct {
		name              string
		key               string
		seedOccupiedWell  bool
		request           []occupancy.Occupancy
		wantFault         string
		retryAfterFailure []occupancy.Occupancy
	}{
		{
			name:             "occupied well aborts whole batch",
			key:              "conflict",
			seedOccupiedWell: true,
			request: []occupancy.Occupancy{
				plateWell("plate-free", "B1"),
				plateWell("plate-shared", "A1"),
			},
			wantFault: CodeOccupancyConflict,
			retryAfterFailure: []occupancy.Occupancy{
				plateWell("plate-free", "B1"),
				plateWell("plate-free", "C1"),
			},
		},
		{
			name: "invalid lease aborts whole batch",
			key:  "invalid",
			request: []occupancy.Occupancy{
				plateWell("plate-free", "B1"),
				invalidLease,
			},
			wantFault: CodeInvalidLease,
			retryAfterFailure: []occupancy.Occupancy{
				plateWell("plate-free", "B1"),
				plateWell("plate-free", "C1"),
			},
		},
		{
			name: "all wells available advances once",
			key:  "available",
			request: []occupancy.Occupancy{
				plateWell("plate-free", "B1"),
				plateWell("plate-free", "C1"),
			},
		},
	}

	advanceToPlateOccupied := func(t *testing.T, svc *Service, taskID inspection.TaskID, suffix string) {
		t.Helper()
		tankBatch := inspection.TankBatch("BATCH-" + suffix)
		_, fault := svc.CreateTask(ctx, CreateTaskRequest{
			TaskID:        taskID,
			FarmID:        catalog.FixedFarmID,
			TankBatch:     tankBatch,
			Compartments:  []catalog.CompartmentCode{"A", "B"},
			Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
			RecorderModel: "recorder-x1",
			RuleVersion:   catalog.FixedRuleVersion,
			Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
		})
		if fault != nil {
			t.Fatalf("create task %s: %v", taskID, fault)
		}
		for _, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
			_, fault := svc.SamplingConfirm(ctx, taskID, SamplingConfirmationRequest{
				OperationID:  inspection.OperationID("op-sample-" + suffix + "-" + string(sampler)),
				Person:       sampler,
				FarmID:       catalog.FixedFarmID,
				TankBatch:    tankBatch,
				Compartments: []catalog.CompartmentCode{"A", "B"},
				Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
				Generation:   1,
			})
			if fault != nil {
				t.Fatalf("sampling %s by %s: %v", taskID, sampler, fault)
			}
		}
		_, fault = svc.BlindSplit(ctx, taskID, BlindSplitRequest{
			OperationID: "op-blind-" + inspection.OperationID(suffix),
			Generation:  1,
			Codes: []blindcode.BlindCode{
				blindcode.BlindCode("BCODE-" + suffix + "-A"),
				blindcode.BlindCode("BCODE-" + suffix + "-B"),
			},
		})
		if fault != nil {
			t.Fatalf("blind split %s: %v", taskID, fault)
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			defer st.Close()
			svc := NewService(st, NewManualClock(1000))
			targetID := inspection.TaskID("target-" + tc.key)

			if tc.seedOccupiedWell {
				holderID := inspection.TaskID("holder-" + tc.key)
				advanceToPlateOccupied(t, svc, holderID, "holder-"+tc.key)
				_, fault := svc.AcquireOccupancy(ctx, holderID, OccupancyRequest{
					OperationID: "op-holder-" + inspection.OperationID(tc.key),
					Generation:  1,
					Occupancies: []occupancy.Occupancy{plateWell("plate-shared", "A1")},
				})
				if fault != nil {
					t.Fatalf("seed occupancy: %v", fault)
				}
			}

			advanceToPlateOccupied(t, svc, targetID, "target-"+tc.key)
			result, fault := svc.AcquireOccupancy(ctx, targetID, OccupancyRequest{
				OperationID: "op-target",
				Generation:  1,
				Occupancies: tc.request,
			})
			if tc.wantFault == "" {
				if fault != nil {
					t.Fatalf("fault = %v, want success", fault)
				}
				if result.Status != inspection.StatusColdChainVerifying || len(result.Acquired) != 2 {
					t.Fatalf("result = %+v, want cold_chain_verifying with two occupancies", result)
				}
				snap, err := st.Snapshot(ctx, targetID)
				if err != nil {
					t.Fatalf("snapshot: %v", err)
				}
				if snap.Task.Status != inspection.StatusColdChainVerifying || len(snap.Occupancies) != 2 {
					t.Fatalf("snapshot status=%s occupancies=%d, want cold_chain_verifying and 2", snap.Task.Status, len(snap.Occupancies))
				}
				return
			}

			if fault == nil || fault.Code != tc.wantFault {
				t.Fatalf("fault = %v result = %+v, want %s", fault, result, tc.wantFault)
			}
			snap, err := st.Snapshot(ctx, targetID)
			if err != nil {
				t.Fatalf("snapshot after rejected batch: %v", err)
			}
			if snap.Task.Status != inspection.StatusPlateOccupied {
				t.Fatalf("status after rejected batch = %s, want %s", snap.Task.Status, inspection.StatusPlateOccupied)
			}
			if len(snap.Occupancies) != 0 {
				t.Fatalf("occupancies after rejected batch = %+v, want none", snap.Occupancies)
			}

			result, fault = svc.AcquireOccupancy(ctx, targetID, OccupancyRequest{
				OperationID: "op-target",
				Generation:  1,
				Occupancies: tc.retryAfterFailure,
			})
			if fault != nil {
				t.Fatalf("retry with same operation id after rejected batch: %v", fault)
			}
			if result.Status != inspection.StatusColdChainVerifying || len(result.Acquired) != 2 {
				t.Fatalf("retry result = %+v, want cold_chain_verifying with two occupancies", result)
			}
		})
	}
}
