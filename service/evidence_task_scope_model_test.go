package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_EvidenceRecoveryIsTaskScoped(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "parallel antibiotic readings do not complete from another task evidence",
			run: func(t *testing.T) {
				svc, st := newModelEvidenceScopeService(t)
				defer st.Close()

				modelReachAntibioticReading(t, svc, "task-scope-donor", "BATCH-SCOPE-DONOR", []blindcode.BlindCode{"DONOR-A", "DONOR-B"})
				modelSubmitReading(t, svc, "task-scope-donor", "donor-anti-a", evidence.EvidenceAntibiotic, "DONOR-A", "20.0")
				modelSubmitReading(t, svc, "task-scope-donor", "donor-anti-b", evidence.EvidenceAntibiotic, "DONOR-B", "20.0")

				modelReachAntibioticReading(t, svc, "task-scope-current", "BATCH-SCOPE-CURRENT", []blindcode.BlindCode{"CURRENT-A", "CURRENT-B"})
				got := modelSubmitReading(t, svc, "task-scope-current", "current-anti-a", evidence.EvidenceAntibiotic, "CURRENT-A", "20.0")
				if got.Status != inspection.StatusAntibioticReading {
					t.Fatalf("current task status after one antibiotic = %s, want %s", got.Status, inspection.StatusAntibioticReading)
				}

				snap, fault := svc.GetSnapshot(context.Background(), "task-scope-current")
				if fault != nil {
					t.Fatalf("snapshot current task: %v", fault)
				}
				if len(snap.Evidence) != 1 {
					t.Fatalf("current task snapshot evidence count = %d, want 1", len(snap.Evidence))
				}
				if snap.Evidence[0].TaskID != "task-scope-current" {
					t.Fatalf("snapshot evidence task = %s, want task-scope-current", snap.Evidence[0].TaskID)
				}

				report, fault := svc.BuildReport(context.Background(), "task-scope-current")
				if fault != nil {
					t.Fatalf("report current task: %v", fault)
				}
				if len(report.Readings) != 1 {
					t.Fatalf("current task report reading count = %d, want 1", len(report.Readings))
				}
			},
		},
		{
			name: "final admissible decision ignores another task failing evidence",
			run: func(t *testing.T) {
				svc, st := newModelEvidenceScopeService(t)
				defer st.Close()

				modelReachAntibioticReading(t, svc, "task-scope-final", "BATCH-SCOPE-FINAL", []blindcode.BlindCode{"FINAL-A", "FINAL-B"})
				modelSubmitAllPassingReadings(t, svc, "task-scope-final", []string{"FINAL-A", "FINAL-B"})
				modelPassReviews(t, svc, "task-scope-final")

				modelReachAntibioticReading(t, svc, "task-scope-foreign-fail", "BATCH-SCOPE-FOREIGN-FAIL", []blindcode.BlindCode{"FOREIGN-FAIL-A", "FOREIGN-FAIL-B"})
				modelSubmitReading(t, svc, "task-scope-foreign-fail", "foreign-anti-fail", evidence.EvidenceAntibiotic, "FOREIGN-FAIL-A", "10.0")

				result, fault := svc.Finalize(context.Background(), "task-scope-final", FinalizeRequest{
					OperationID: "final-admissible",
					Generation:  1,
					Outcome:     inspection.FinalAdmissible,
				})
				if fault != nil {
					t.Fatalf("finalize current task admissible: %v", fault)
				}
				if result.FinalType != inspection.FinalAdmissible {
					t.Fatalf("final type = %s, want %s", result.FinalType, inspection.FinalAdmissible)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}

func newModelEvidenceScopeService(t *testing.T) (*Service, *store.MemoryStore) {
	t.Helper()
	st := store.NewMemoryStore(catalog.NewFixedCatalog())
	return NewService(st, NewManualClock(1000)), st
}

func modelReachAntibioticReading(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch, codes []blindcode.BlindCode) {
	t.Helper()
	modelCreateTask(t, svc, id, batch)
	modelConfirmSampling(t, svc, id, batch)
	modelBlindSplit(t, svc, id, codes)
	modelAcquireOccupancy(t, svc, id)
	modelWriteColdChain(t, svc, id)
}

func modelCreateTask(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch) {
	t.Helper()
	_, fault := svc.CreateTask(context.Background(), CreateTaskRequest{
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
		t.Fatalf("create task %s: %v", id, fault)
	}
}

func modelConfirmSampling(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch) {
	t.Helper()
	for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
		_, fault := svc.SamplingConfirm(context.Background(), id, SamplingConfirmationRequest{
			OperationID:  inspection.OperationID(fmt.Sprintf("%s-sample-%d", id, i)),
			Person:       sampler,
			FarmID:       catalog.FixedFarmID,
			TankBatch:    batch,
			Compartments: []catalog.CompartmentCode{"A", "B"},
			Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
			Generation:   1,
		})
		if fault != nil {
			t.Fatalf("sampling confirm %s/%d: %v", id, i, fault)
		}
	}
}

func modelBlindSplit(t *testing.T, svc *Service, id inspection.TaskID, codes []blindcode.BlindCode) {
	t.Helper()
	_, fault := svc.BlindSplit(context.Background(), id, BlindSplitRequest{
		OperationID: inspection.OperationID(fmt.Sprintf("%s-split", id)),
		Generation:  1,
		Codes:       codes,
	})
	if fault != nil {
		t.Fatalf("blind split %s: %v", id, fault)
	}
}

func modelAcquireOccupancy(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	_, fault := svc.AcquireOccupancy(context.Background(), id, OccupancyRequest{
		OperationID: inspection.OperationID(fmt.Sprintf("%s-occ", id)),
		Generation:  1,
		Occupancies: []occupancy.Occupancy{
			{ResourceType: occupancy.ResourcePlateWell, PlateID: fmt.Sprintf("plate-%s", id), Well: "A1", StartAt: 0, EndAt: 3600},
			{ResourceType: occupancy.ResourceIncubator, IncubatorID: fmt.Sprintf("inc-%s", id), StartAt: 0, EndAt: 3600},
		},
	})
	if fault != nil {
		t.Fatalf("occupancy %s: %v", id, fault)
	}
}

func modelWriteColdChain(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	rules, ok := svc.Catalog().Rules(catalog.FixedRuleVersion)
	if !ok {
		t.Fatal("fixed rules missing")
	}
	cells := make([]evidence.TemperatureCell, 0, int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds)+1)
	for at := int64(0); at <= rules.Temperature.WindowSeconds; at += rules.Temperature.SampleEverySeconds {
		cells = append(cells, evidence.TemperatureCell{
			AtSeconds: at,
			Celsius:   evidence.FixedPoint{Value: 40, Scale: 1},
		})
	}
	_, fault := svc.ColdChainReadings(context.Background(), id, ColdChainReadingsRequest{
		OperationID: inspection.OperationID(fmt.Sprintf("%s-cold", id)),
		Generation:  1,
		BaseTime:    0,
		RecorderID:  fmt.Sprintf("recorder-%s", id),
		Cells:       cells,
	})
	if fault != nil {
		t.Fatalf("cold chain %s: %v", id, fault)
	}
}

func modelSubmitReading(t *testing.T, svc *Service, id inspection.TaskID, op string, typ evidence.EvidenceType, blindCode, value string) *ReadingResult {
	t.Helper()
	result, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
		OperationID: inspection.OperationID(op),
		Generation:  1,
		Type:        typ,
		BlindCode:   blindCode,
		Value:       value,
	})
	if fault != nil {
		t.Fatalf("reading %s/%s: %v", id, op, fault)
	}
	return result
}

func modelSubmitAllPassingReadings(t *testing.T, svc *Service, id inspection.TaskID, blindCodes []string) {
	t.Helper()
	for i, code := range blindCodes {
		modelSubmitReading(t, svc, id, fmt.Sprintf("%s-anti-%d", id, i), evidence.EvidenceAntibiotic, code, "20.0")
	}
	for i, code := range blindCodes {
		modelSubmitReading(t, svc, id, fmt.Sprintf("%s-somatic-%d", id, i), evidence.EvidenceSomaticCell, code, "350")
		modelSubmitReading(t, svc, id, fmt.Sprintf("%s-colony-%d", id, i), evidence.EvidenceColony, code, "50000")
	}
	for i, code := range blindCodes {
		modelSubmitReading(t, svc, id, fmt.Sprintf("%s-freezing-%d", id, i), evidence.EvidenceFreezingPoint, code, "-53.0")
		modelSubmitReading(t, svc, id, fmt.Sprintf("%s-fat-%d", id, i), evidence.EvidenceFat, code, "3.5")
		modelSubmitReading(t, svc, id, fmt.Sprintf("%s-protein-%d", id, i), evidence.EvidenceProtein, code, "3.1")
	}
}

func modelPassReviews(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	for _, reviewer := range []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB} {
		_, fault := svc.Review(context.Background(), id, ReviewRequest{
			OperationID: inspection.OperationID(fmt.Sprintf("%s-review-%s", id, reviewer)),
			Generation:  1,
			Reviewer:    reviewer,
			Conclusion:  "pass",
		})
		if fault != nil {
			t.Fatalf("review %s/%s: %v", id, reviewer, fault)
		}
	}
}
