package service

import (
	"context"
	"strconv"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
)

func TestModel_FinalDecisionCredentialScopedPerTask(t *testing.T) {
	ctx := context.Background()

	readyTask := func(t *testing.T, svc *Service, suffix string) inspection.TaskID {
		t.Helper()
		taskID := inspection.TaskID("task-final-scope-" + suffix)
		tankBatch := inspection.TankBatch("BATCH-FINAL-SCOPE-" + suffix)
		blindCodes := []blindcode.BlindCode{
			blindcode.BlindCode("BCODE-" + suffix + "-A"),
			blindcode.BlindCode("BCODE-" + suffix + "-B"),
		}

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

		for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
			_, fault = svc.SamplingConfirm(ctx, taskID, SamplingConfirmationRequest{
				OperationID:  inspection.OperationID("op-sample-" + suffix + "-" + strconv.Itoa(i)),
				Person:       sampler,
				FarmID:       catalog.FixedFarmID,
				TankBatch:    tankBatch,
				Compartments: []catalog.CompartmentCode{"A", "B"},
				Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
				Generation:   1,
			})
			if fault != nil {
				t.Fatalf("sampling confirm %s/%d: %v", taskID, i, fault)
			}
		}

		_, fault = svc.BlindSplit(ctx, taskID, BlindSplitRequest{
			OperationID: inspection.OperationID("op-split-" + suffix),
			Generation:  1,
			Codes:       blindCodes,
		})
		if fault != nil {
			t.Fatalf("blind split %s: %v", taskID, fault)
		}

		_, fault = svc.AcquireOccupancy(ctx, taskID, OccupancyRequest{
			OperationID: inspection.OperationID("op-occ-" + suffix),
			Generation:  1,
			Occupancies: []occupancy.Occupancy{
				{ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-" + suffix, Well: "A1", StartAt: 0, EndAt: 3600},
				{ResourceType: occupancy.ResourceIncubator, IncubatorID: "inc-" + suffix, StartAt: 0, EndAt: 3600},
			},
		})
		if fault != nil {
			t.Fatalf("occupancy %s: %v", taskID, fault)
		}

		rules, ok := svc.Catalog().Rules(catalog.FixedRuleVersion)
		if !ok {
			t.Fatal("fixed rules missing")
		}
		var cells []evidence.TemperatureCell
		n := int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds) + 1
		for i := 0; i < n; i++ {
			cells = append(cells, evidence.TemperatureCell{
				AtSeconds: int64(i * 60),
				Celsius:   evidence.FixedPoint{Value: 40, Scale: 1},
			})
		}
		_, fault = svc.ColdChainReadings(ctx, taskID, ColdChainReadingsRequest{
			OperationID: inspection.OperationID("op-cold-" + suffix),
			Generation:  1,
			BaseTime:    0,
			RecorderID:  "recorder-" + suffix,
			Cells:       cells,
		})
		if fault != nil {
			t.Fatalf("cold chain %s: %v", taskID, fault)
		}

		for i, code := range blindCodes {
			_, fault = svc.SubmitReading(ctx, taskID, ReadingRequest{
				OperationID: inspection.OperationID("op-anti-" + suffix + "-" + strconv.Itoa(i)),
				Generation:  1,
				Type:        evidence.EvidenceAntibiotic,
				BlindCode:   string(code),
				Value:       "20.0",
			})
			if fault != nil {
				t.Fatalf("antibiotic reading %s/%d: %v", taskID, i, fault)
			}
		}
		for i, code := range blindCodes {
			_, fault = svc.SubmitReading(ctx, taskID, ReadingRequest{
				OperationID: inspection.OperationID("op-som-" + suffix + "-" + strconv.Itoa(i)),
				Generation:  1,
				Type:        evidence.EvidenceSomaticCell,
				BlindCode:   string(code),
				Value:       "350",
			})
			if fault != nil {
				t.Fatalf("somatic reading %s/%d: %v", taskID, i, fault)
			}
			_, fault = svc.SubmitReading(ctx, taskID, ReadingRequest{
				OperationID: inspection.OperationID("op-col-" + suffix + "-" + strconv.Itoa(i)),
				Generation:  1,
				Type:        evidence.EvidenceColony,
				BlindCode:   string(code),
				Value:       "50000",
			})
			if fault != nil {
				t.Fatalf("colony reading %s/%d: %v", taskID, i, fault)
			}
		}
		for i, code := range blindCodes {
			_, fault = svc.SubmitReading(ctx, taskID, ReadingRequest{
				OperationID: inspection.OperationID("op-fp-" + suffix + "-" + strconv.Itoa(i)),
				Generation:  1,
				Type:        evidence.EvidenceFreezingPoint,
				BlindCode:   string(code),
				Value:       "-53.0",
			})
			if fault != nil {
				t.Fatalf("freezing point reading %s/%d: %v", taskID, i, fault)
			}
			_, fault = svc.SubmitReading(ctx, taskID, ReadingRequest{
				OperationID: inspection.OperationID("op-fat-" + suffix + "-" + strconv.Itoa(i)),
				Generation:  1,
				Type:        evidence.EvidenceFat,
				BlindCode:   string(code),
				Value:       "3.5",
			})
			if fault != nil {
				t.Fatalf("fat reading %s/%d: %v", taskID, i, fault)
			}
			_, fault = svc.SubmitReading(ctx, taskID, ReadingRequest{
				OperationID: inspection.OperationID("op-prot-" + suffix + "-" + strconv.Itoa(i)),
				Generation:  1,
				Type:        evidence.EvidenceProtein,
				BlindCode:   string(code),
				Value:       "3.1",
			})
			if fault != nil {
				t.Fatalf("protein reading %s/%d: %v", taskID, i, fault)
			}
		}

		for _, reviewer := range []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB} {
			_, fault = svc.Review(ctx, taskID, ReviewRequest{
				OperationID: inspection.OperationID("op-review-" + suffix + "-" + string(reviewer)),
				Generation:  1,
				Reviewer:    reviewer,
				Conclusion:  "pass",
			})
			if fault != nil {
				t.Fatalf("review %s/%s: %v", taskID, reviewer, fault)
			}
		}

		return taskID
	}

	tests := []struct {
		name       string
		outcome    inspection.FinalType
		wantStatus inspection.Status
	}{
		{name: "two_admissible_tasks", outcome: inspection.FinalAdmissible, wantStatus: inspection.StatusAdmissible},
		{name: "two_quarantined_tasks", outcome: inspection.FinalQuarantined, wantStatus: inspection.StatusQuarantined},
		{name: "two_cancelled_tasks", outcome: inspection.FinalCancelled, wantStatus: inspection.StatusCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()

			credentials := map[string]inspection.TaskID{}
			for i := 1; i <= 2; i++ {
				suffix := string(tt.outcome) + "-" + strconv.Itoa(i)
				taskID := readyTask(t, svc, suffix)

				result, fault := svc.Finalize(ctx, taskID, FinalizeRequest{
					OperationID: inspection.OperationID("op-final-" + suffix),
					Generation:  1,
					Outcome:     tt.outcome,
				})
				if fault != nil {
					t.Fatalf("finalize %s as %s: %v", taskID, tt.outcome, fault)
				}
				if result.FinalType != tt.outcome {
					t.Fatalf("final type = %s, want %s", result.FinalType, tt.outcome)
				}
				if result.Credential == "" {
					t.Fatalf("empty credential for %s", taskID)
				}
				if previous, exists := credentials[result.Credential]; exists {
					t.Fatalf("credential %s reused for %s and %s", result.Credential, previous, taskID)
				}
				credentials[result.Credential] = taskID

				snap, fault := svc.GetSnapshot(ctx, taskID)
				if fault != nil {
					t.Fatalf("snapshot %s: %v", taskID, fault)
				}
				if snap.Task.Status != tt.wantStatus {
					t.Fatalf("status = %s, want %s", snap.Task.Status, tt.wantStatus)
				}
				if snap.Task.FinalType != tt.outcome {
					t.Fatalf("task final type = %s, want %s", snap.Task.FinalType, tt.outcome)
				}
				if snap.FinalDecision == nil {
					t.Fatalf("missing final decision for %s", taskID)
				}
				if snap.FinalDecision.TaskID != taskID {
					t.Fatalf("final decision task = %s, want %s", snap.FinalDecision.TaskID, taskID)
				}
				if snap.FinalDecision.FinalType != tt.outcome || snap.FinalDecision.Credential != result.Credential {
					t.Fatalf("final decision = %+v, result = %+v", snap.FinalDecision, result)
				}
			}
		})
	}
}
