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

func TestModel_OccupancyUsesResourceKeyBoundaries(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name                  string
		first                 occupancy.Occupancy
		second                []occupancy.Occupancy
		wantFault             string
		wantSecondStatus      inspection.Status
		wantSecondOccupancies int
	}{
		{
			name: "different wells on same plate share a window",
			first: occupancy.Occupancy{
				ResourceType: occupancy.ResourcePlateWell,
				PlateID:      "plate-1",
				Well:         "A1",
				StartAt:      100,
				EndAt:        200,
			},
			second: []occupancy.Occupancy{{
				ResourceType: occupancy.ResourcePlateWell,
				PlateID:      "plate-1",
				Well:         "A2",
				StartAt:      100,
				EndAt:        200,
			}},
			wantSecondStatus:      inspection.StatusColdChainVerifying,
			wantSecondOccupancies: 1,
		},
		{
			name: "same plate well overlap rejects and rolls back request",
			first: occupancy.Occupancy{
				ResourceType: occupancy.ResourcePlateWell,
				PlateID:      "plate-1",
				Well:         "A1",
				StartAt:      100,
				EndAt:        200,
			},
			second: []occupancy.Occupancy{
				{
					ResourceType: occupancy.ResourcePlateWell,
					PlateID:      "plate-1",
					Well:         "B1",
					StartAt:      100,
					EndAt:        200,
				},
				{
					ResourceType: occupancy.ResourcePlateWell,
					PlateID:      "plate-1",
					Well:         "A1",
					StartAt:      150,
					EndAt:        180,
				},
			},
			wantFault:             CodeOccupancyConflict,
			wantSecondStatus:      inspection.StatusPlateOccupied,
			wantSecondOccupancies: 0,
		},
		{
			name: "same incubator overlap rejects and rolls back request",
			first: occupancy.Occupancy{
				ResourceType: occupancy.ResourceIncubator,
				IncubatorID:  "inc-1",
				StartAt:      100,
				EndAt:        200,
			},
			second: []occupancy.Occupancy{
				{
					ResourceType: occupancy.ResourcePlateWell,
					PlateID:      "plate-1",
					Well:         "A2",
					StartAt:      100,
					EndAt:        200,
				},
				{
					ResourceType: occupancy.ResourceIncubator,
					IncubatorID:  "inc-1",
					StartAt:      150,
					EndAt:        180,
				},
			},
			wantFault:             CodeOccupancyConflict,
			wantSecondStatus:      inspection.StatusPlateOccupied,
			wantSecondOccupancies: 0,
		},
	}

	for caseIndex, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()

			readyTask := func(role string) inspection.TaskID {
				id := inspection.TaskID("task-occupancy-" + strconvItoa(caseIndex) + "-" + role)
				batch := inspection.TankBatch("BATCH-2026-KEY-" + strconvItoa(caseIndex) + "-" + role)
				_, fault := svc.CreateTask(ctx, CreateTaskRequest{
					TaskID:        id,
					FarmID:        catalog.FixedFarmID,
					TankBatch:     batch,
					Compartments:  []catalog.CompartmentCode{"A", "B"},
					Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
					RecorderModel: "recorder-x1",
					RuleVersion:   catalog.FixedRuleVersion,
					Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
				})
				if fault != nil {
					t.Fatalf("create %s task: %v", role, fault)
				}
				for samplerIndex, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
					_, fault = svc.SamplingConfirm(ctx, id, SamplingConfirmationRequest{
						OperationID:  inspection.OperationID("op-sample-" + role + "-" + strconvItoa(samplerIndex)),
						Person:       sampler,
						FarmID:       catalog.FixedFarmID,
						TankBatch:    batch,
						Compartments: []catalog.CompartmentCode{"A", "B"},
						Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
						Generation:   1,
					})
					if fault != nil {
						t.Fatalf("sampling %s task: %v", role, fault)
					}
				}
				_, fault = svc.BlindSplit(ctx, id, BlindSplitRequest{
					OperationID: inspection.OperationID("op-split-" + role),
					Generation:  1,
					Codes: []blindcode.BlindCode{
						blindcode.BlindCode("BCODE-" + strconvItoa(caseIndex) + "-" + role + "-A"),
						blindcode.BlindCode("BCODE-" + strconvItoa(caseIndex) + "-" + role + "-B"),
					},
				})
				if fault != nil {
					t.Fatalf("blind split %s task: %v", role, fault)
				}
				return id
			}

			firstID := readyTask("first")
			secondID := readyTask("second")

			_, fault := svc.AcquireOccupancy(ctx, firstID, OccupancyRequest{
				OperationID: "op-occ-first",
				Generation:  1,
				Occupancies: []occupancy.Occupancy{tc.first},
			})
			if fault != nil {
				t.Fatalf("first occupancy: %v", fault)
			}

			secondReq := OccupancyRequest{
				OperationID: "op-occ-second",
				Generation:  1,
				Occupancies: tc.second,
			}
			second, fault := svc.AcquireOccupancy(ctx, secondID, secondReq)
			if tc.wantFault != "" {
				if fault == nil || fault.Code != tc.wantFault {
					t.Fatalf("second fault = %v, want %s", fault, tc.wantFault)
				}
			} else {
				if fault != nil {
					t.Fatalf("second occupancy: %v", fault)
				}
				if second.Status != tc.wantSecondStatus {
					t.Fatalf("second status = %s, want %s", second.Status, tc.wantSecondStatus)
				}
			}

			snap, snapshotFault := svc.GetSnapshot(ctx, secondID)
			if snapshotFault != nil {
				t.Fatalf("second snapshot: %v", snapshotFault)
			}
			if snap.Task.Status != tc.wantSecondStatus {
				t.Fatalf("persisted second status = %s, want %s", snap.Task.Status, tc.wantSecondStatus)
			}
			if len(snap.Occupancies) != tc.wantSecondOccupancies {
				t.Fatalf("persisted second occupancies = %d, want %d: %+v", len(snap.Occupancies), tc.wantSecondOccupancies, snap.Occupancies)
			}
			wantCommitted := tc.wantFault == ""
			occupiedAudit := false
			for _, event := range snap.Audit {
				if event.EventType == inspection.EventOccupied {
					occupiedAudit = true
					break
				}
			}
			if occupiedAudit != wantCommitted {
				t.Fatalf("occupied audit present = %t, want %t", occupiedAudit, wantCommitted)
			}
			var occupancyRecord bool
			if err := svc.Store().WithTx(ctx, func(tx store.Tx) error {
				_, exists, err := tx.GetIdempotency(ctx, secondID, "op-occ-second")
				occupancyRecord = exists
				return err
			}); err != nil {
				t.Fatalf("read occupancy idempotency: %v", err)
			}
			if occupancyRecord != wantCommitted {
				t.Fatalf("occupancy idempotency record present = %t, want %t", occupancyRecord, wantCommitted)
			}
		})
	}
}
