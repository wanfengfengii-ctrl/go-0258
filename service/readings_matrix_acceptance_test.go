package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_ReadingsRequireCurrentGenerationBlindCodeAndFullStageCoverage(t *testing.T) {
	ctx := context.Background()
	const codeBlindUnknown = "blind_unknown"
	cases := []struct {
		name string
		run  func(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID)
	}{
		{
			name: "unknown blind code is rejected and cannot seed evidence",
			run: func(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
				_, fault := svc.SubmitReading(ctx, id, ReadingRequest{
					OperationID: "model-unknown-blind",
					Generation:  1,
					Type:        evidence.EvidenceAntibiotic,
					BlindCode:   "BCODE-NOT-MAPPED",
					Value:       "20.0",
				})
				if fault == nil || fault.Code != codeBlindUnknown {
					t.Fatalf("fault = %v, want %s", fault, codeBlindUnknown)
				}
				modelRequireStatus(t, ctx, svc, id, inspection.StatusAntibioticReading)
				modelRequireEvidenceCount(t, ctx, svc, id, 0)
			},
		},
		{
			name: "somatic-only microbial readings cannot replace colony coverage",
			run: func(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
				modelCompleteAntibiotic(t, ctx, svc, id)

				for i, code := range []string{"BCODE-A", "BCODE-B"} {
					modelSubmitReading(t, ctx, svc, id, "model-somatic-"+strconvItoa(i), evidence.EvidenceSomaticCell, code, "350")
				}

				modelRequireStatus(t, ctx, svc, id, inspection.StatusMicrobialCulturing)
				modelRequireEvidenceCount(t, ctx, svc, id, 4)
				modelRequireReviewRejected(t, ctx, svc, id)
				modelRequireAdmissibleRejected(t, ctx, svc, id)
			},
		},
		{
			name: "fat duplicates cannot replace freezing-point and protein coverage",
			run: func(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
				modelCompleteAntibiotic(t, ctx, svc, id)
				modelCompleteMicrobial(t, ctx, svc, id)

				for i, code := range []string{"BCODE-A", "BCODE-B", "BCODE-A"} {
					modelSubmitReading(t, ctx, svc, id, "model-fat-"+strconvItoa(i), evidence.EvidenceFat, code, "3.5")
				}

				modelRequireStatus(t, ctx, svc, id, inspection.StatusPhysicochemical)
				modelRequireEvidenceCount(t, ctx, svc, id, 9)
				modelRequireReviewRejected(t, ctx, svc, id)
				modelRequireAdmissibleRejected(t, ctx, svc, id)
			},
		},
		{
			name: "complete required reading matrix remains admissible",
			run: func(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
				modelCompleteAllReadings(t, ctx, svc, id)
				modelRequireStatus(t, ctx, svc, id, inspection.StatusPendingReview)
				modelPassReviews(t, ctx, svc, id)

				result, fault := svc.Finalize(ctx, id, FinalizeRequest{
					OperationID: "model-final-admissible",
					Generation:  1,
					Outcome:     inspection.FinalAdmissible,
				})
				if fault != nil {
					t.Fatalf("finalize admissible: %v", fault)
				}
				if result.FinalType != inspection.FinalAdmissible || result.Credential == "" {
					t.Fatalf("final result = %+v, want admissible with credential", result)
				}
				modelRequireStatus(t, ctx, svc, id, inspection.StatusAdmissible)
				modelRequireEvidenceCount(t, ctx, svc, id, 12)
			},
		},
		{
			name: "instrument failure remains a retry without advancing phase",
			run: func(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
				result, fault := svc.SubmitReading(ctx, id, ReadingRequest{
					OperationID:    "model-instrument-failure",
					Generation:     1,
					Type:           evidence.EvidenceAntibiotic,
					BlindCode:      "BCODE-A",
					Well:           "A1",
					InstrumentType: "plate-reader",
					ScriptResult:   "timeout",
					ErrorClass:     ErrClassTimeout,
				})
				if fault != nil {
					t.Fatalf("instrument failure reading: %v", fault)
				}
				if result.Instrument == nil || result.Instrument.ErrorClass != ErrClassTimeout {
					t.Fatalf("instrument result = %+v, want timeout retry call", result.Instrument)
				}
				snap := modelSnapshot(t, ctx, svc, id)
				if len(snap.InstrumentCalls) != 1 {
					t.Fatalf("instrument calls = %d, want 1", len(snap.InstrumentCalls))
				}
				modelRequireStatus(t, ctx, svc, id, inspection.StatusAntibioticReading)
				modelRequireEvidenceCount(t, ctx, svc, id, 0)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st, id := modelReadingServiceAtAntibiotic(t, ctx)
			defer st.Close()
			tc.run(t, ctx, svc, id)
		})
	}
}

func modelReadingServiceAtAntibiotic(t *testing.T, ctx context.Context) (*Service, *store.MemoryStore, inspection.TaskID) {
	t.Helper()
	st := store.NewMemoryStore(catalog.NewFixedCatalog())
	svc := NewService(st, NewManualClock(1000))
	id := inspection.TaskID("task-model-readings")

	_, fault := svc.CreateTask(ctx, CreateTaskRequest{
		TaskID:        id,
		FarmID:        catalog.FixedFarmID,
		TankBatch:     "BATCH-MODEL-READINGS",
		Compartments:  []catalog.CompartmentCode{"A", "B"},
		Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
		RecorderModel: "recorder-x1",
		RuleVersion:   catalog.FixedRuleVersion,
		Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
	})
	if fault != nil {
		t.Fatalf("create task: %v", fault)
	}

	for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
		_, fault := svc.SamplingConfirm(ctx, id, SamplingConfirmationRequest{
			OperationID:  inspection.OperationID("model-sampling-" + strconvItoa(i)),
			Generation:   1,
			Person:       sampler,
			FarmID:       catalog.FixedFarmID,
			TankBatch:    "BATCH-MODEL-READINGS",
			Compartments: []catalog.CompartmentCode{"A", "B"},
			Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
		})
		if fault != nil {
			t.Fatalf("sampling confirm %d: %v", i, fault)
		}
	}

	_, fault = svc.BlindSplit(ctx, id, BlindSplitRequest{
		OperationID: "model-blind-split",
		Generation:  1,
		Codes:       []blindcode.BlindCode{"BCODE-A", "BCODE-B"},
	})
	if fault != nil {
		t.Fatalf("blind split: %v", fault)
	}

	_, fault = svc.AcquireOccupancy(ctx, id, OccupancyRequest{
		OperationID: "model-occupancy",
		Generation:  1,
		Occupancies: []occupancy.Occupancy{
			{TaskID: string(id), ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-model", Well: "A1", StartAt: 0, EndAt: 3600},
			{TaskID: string(id), ResourceType: occupancy.ResourceIncubator, IncubatorID: "inc-model", StartAt: 0, EndAt: 3600},
		},
	})
	if fault != nil {
		t.Fatalf("occupancy: %v", fault)
	}

	rules, ok := svc.Catalog().Rules(catalog.FixedRuleVersion)
	if !ok {
		t.Fatalf("missing fixed rule version %s", catalog.FixedRuleVersion)
	}
	cells := make([]evidence.TemperatureCell, 0, int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds)+1)
	for i := 0; i <= int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds); i++ {
		cells = append(cells, evidence.TemperatureCell{
			AtSeconds: int64(i * int(rules.Temperature.SampleEverySeconds)),
			Celsius:   evidence.FixedPoint{Value: 40, Scale: 1},
		})
	}
	_, fault = svc.ColdChainReadings(ctx, id, ColdChainReadingsRequest{
		OperationID: "model-cold-chain",
		Generation:  1,
		BaseTime:    0,
		RecorderID:  "recorder-x1",
		Cells:       cells,
	})
	if fault != nil {
		t.Fatalf("cold chain readings: %v", fault)
	}

	modelRequireStatus(t, ctx, svc, id, inspection.StatusAntibioticReading)
	return svc, st, id
}

func modelCompleteAllReadings(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
	t.Helper()
	modelCompleteAntibiotic(t, ctx, svc, id)
	modelCompleteMicrobial(t, ctx, svc, id)
	modelCompletePhysicochemical(t, ctx, svc, id)
}

func modelCompleteAntibiotic(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
	t.Helper()
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		modelSubmitReading(t, ctx, svc, id, "model-antibiotic-"+strconvItoa(i), evidence.EvidenceAntibiotic, code, "20.0")
	}
	modelRequireStatus(t, ctx, svc, id, inspection.StatusMicrobialCulturing)
}

func modelCompleteMicrobial(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
	t.Helper()
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		modelSubmitReading(t, ctx, svc, id, "model-somatic-complete-"+strconvItoa(i), evidence.EvidenceSomaticCell, code, "350")
		modelSubmitReading(t, ctx, svc, id, "model-colony-"+strconvItoa(i), evidence.EvidenceColony, code, "50000")
	}
	modelRequireStatus(t, ctx, svc, id, inspection.StatusPhysicochemical)
}

func modelCompletePhysicochemical(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
	t.Helper()
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		modelSubmitReading(t, ctx, svc, id, "model-freezing-"+strconvItoa(i), evidence.EvidenceFreezingPoint, code, "-53.0")
		modelSubmitReading(t, ctx, svc, id, "model-fat-complete-"+strconvItoa(i), evidence.EvidenceFat, code, "3.5")
		modelSubmitReading(t, ctx, svc, id, "model-protein-"+strconvItoa(i), evidence.EvidenceProtein, code, "3.1")
	}
	modelRequireStatus(t, ctx, svc, id, inspection.StatusPendingReview)
}

func modelSubmitReading(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID, op string, typ evidence.EvidenceType, code string, value string) *ReadingResult {
	t.Helper()
	result, fault := svc.SubmitReading(ctx, id, ReadingRequest{
		OperationID: inspection.OperationID(op),
		Generation:  1,
		Type:        typ,
		BlindCode:   code,
		Value:       value,
	})
	if fault != nil {
		t.Fatalf("submit %s for %s: %v", typ, code, fault)
	}
	return result
}

func modelPassReviews(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
	t.Helper()
	for _, reviewer := range []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB} {
		_, fault := svc.Review(ctx, id, ReviewRequest{
			OperationID: inspection.OperationID("model-review-" + string(reviewer)),
			Generation:  1,
			Reviewer:    reviewer,
			Conclusion:  arbiter.ReviewPass,
		})
		if fault != nil {
			t.Fatalf("review by %s: %v", reviewer, fault)
		}
	}
}

func modelRequireReviewRejected(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
	t.Helper()
	_, fault := svc.Review(ctx, id, ReviewRequest{
		OperationID: "model-review-before-matrix-complete",
		Generation:  1,
		Reviewer:    catalog.FixedReviewerA,
		Conclusion:  arbiter.ReviewPass,
	})
	if fault == nil {
		t.Fatalf("review succeeded before reading matrix was complete")
	}
}

func modelRequireAdmissibleRejected(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) {
	t.Helper()
	_, fault := svc.Finalize(ctx, id, FinalizeRequest{
		OperationID: "model-final-before-matrix-complete",
		Generation:  1,
		Outcome:     inspection.FinalAdmissible,
	})
	if fault == nil {
		t.Fatalf("admissible finalization succeeded before reading matrix was complete")
	}
}

func modelRequireStatus(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID, want inspection.Status) {
	t.Helper()
	snap := modelSnapshot(t, ctx, svc, id)
	if snap.Task.Status != want {
		t.Fatalf("status = %s, want %s", snap.Task.Status, want)
	}
}

func modelRequireEvidenceCount(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID, want int) {
	t.Helper()
	snap := modelSnapshot(t, ctx, svc, id)
	if len(snap.Evidence) != want {
		t.Fatalf("evidence count = %d, want %d", len(snap.Evidence), want)
	}
}

func modelSnapshot(t *testing.T, ctx context.Context, svc *Service, id inspection.TaskID) *store.Snapshot {
	t.Helper()
	snap, fault := svc.GetSnapshot(ctx, id)
	if fault != nil {
		t.Fatalf("snapshot: %v", fault)
	}
	return snap
}
