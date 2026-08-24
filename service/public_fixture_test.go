package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// newFixtureService builds a service over an in-memory store with a manual
// clock at a fixed start time.
func newFixtureService(t *testing.T) (*Service, *store.MemoryStore) {
	t.Helper()
	st := store.NewMemoryStore(catalog.NewFixedCatalog())
	svc := NewService(st, NewManualClock(1000))
	return svc, st
}

func createFixtureTask(t *testing.T, svc *Service) inspection.TaskID {
	t.Helper()
	_, fault := svc.CreateTask(context.Background(), CreateTaskRequest{
		TaskID:        "task-fixture",
		FarmID:        catalog.FixedFarmID,
		TankBatch:     "BATCH-2026-001",
		Compartments:  []catalog.CompartmentCode{"A", "B"},
		Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
		RecorderModel: "recorder-x1",
		RuleVersion:   catalog.FixedRuleVersion,
		Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
	})
	if fault != nil {
		t.Fatalf("create task: %v", fault)
	}
	return "task-fixture"
}

func confirmSampling(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
		_, fault := svc.SamplingConfirm(context.Background(), id, SamplingConfirmationRequest{
			OperationID:  inspection.OperationID("op-sample-" + strconvItoa(i)),
			Person:       sampler,
			FarmID:       catalog.FixedFarmID,
			TankBatch:    "BATCH-2026-001",
			Compartments: []catalog.CompartmentCode{"A", "B"},
			Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
			Generation:   1,
		})
		if fault != nil {
			t.Fatalf("sampling confirm %d: %v", i, fault)
		}
	}
}

func splitBlind(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	_, fault := svc.BlindSplit(context.Background(), id, BlindSplitRequest{
		OperationID: "op-split",
		Generation:  1,
		Codes:       []blindcode.BlindCode{"BCODE-A", "BCODE-B"},
	})
	if fault != nil {
		t.Fatalf("blind split: %v", fault)
	}
}

func occupyResources(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	_, fault := svc.AcquireOccupancy(context.Background(), id, OccupancyRequest{
		OperationID: "op-occ",
		Generation:  1,
		Occupancies: []occupancy.Occupancy{
			{TaskID: string(id), ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-1", Well: "A1", StartAt: 0, EndAt: 3600},
			{TaskID: string(id), ResourceType: occupancy.ResourceIncubator, IncubatorID: "inc-1", StartAt: 0, EndAt: 3600},
		},
	})
	if fault != nil {
		t.Fatalf("occupancy: %v", fault)
	}
}

func writeColdChain(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	rules, _ := svc.Catalog().Rules(catalog.FixedRuleVersion)
	var cells []evidence.TemperatureCell
	n := int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds) + 1
	for i := 0; i < n; i++ {
		cells = append(cells, evidence.TemperatureCell{
			AtSeconds: int64(i * 60),
			Celsius:   evidence.FixedPoint{Value: 40, Scale: 1}, // 4.0 C, in range
		})
	}
	_, fault := svc.ColdChainReadings(context.Background(), id, ColdChainReadingsRequest{
		OperationID: "op-cold",
		Generation:  1,
		BaseTime:    0,
		RecorderID:  "recorder-x1",
		Cells:       cells,
	})
	if fault != nil {
		t.Fatalf("cold chain: %v", fault)
	}
}

// TestPublicFixtureAdmissibleEntry runs the full happy path to a plant-entry
// credential, exercising every command and the fixed-point thresholds.
func TestPublicFixtureAdmissibleEntry(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)
	confirmSampling(t, svc, id)
	splitBlind(t, svc, id)
	occupyResources(t, svc, id)
	writeColdChain(t, svc, id)

	// Antibiotic: one negative inhibition reading per blind code.
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		_, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
			OperationID: inspection.OperationID("op-anti-" + strconvItoa(i)),
			Generation:  1, Type: evidence.EvidenceAntibiotic,
			BlindCode: code, Well: "A" + strconvItoa(i+1), Value: "20.0",
		})
		if fault != nil {
			t.Fatalf("antibiotic reading: %v", fault)
		}
	}

	// Microbial: somatic + colony per blind code.
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		_, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
			OperationID: inspection.OperationID("op-som-" + strconvItoa(i)),
			Generation:  1, Type: evidence.EvidenceSomaticCell,
			BlindCode: code, Value: "350", // 350 thousand, under 400
		})
		if fault != nil {
			t.Fatalf("somatic reading: %v", fault)
		}
		_, fault = svc.SubmitReading(context.Background(), id, ReadingRequest{
			OperationID: inspection.OperationID("op-col-" + strconvItoa(i)),
			Generation:  1, Type: evidence.EvidenceColony,
			BlindCode: code, Value: "50000",
		})
		if fault != nil {
			t.Fatalf("colony reading: %v", fault)
		}
	}

	// Physicochemical: freezing point, fat, protein per blind code.
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		_, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
			OperationID: inspection.OperationID("op-fp-" + strconvItoa(i)),
			Generation:  1, Type: evidence.EvidenceFreezingPoint,
			BlindCode: code, Value: "-53.0",
		})
		if fault != nil {
			t.Fatalf("freezing point reading: %v", fault)
		}
		_, fault = svc.SubmitReading(context.Background(), id, ReadingRequest{
			OperationID: inspection.OperationID("op-fat-" + strconvItoa(i)),
			Generation:  1, Type: evidence.EvidenceFat,
			BlindCode: code, Value: "3.5",
		})
		if fault != nil {
			t.Fatalf("fat reading: %v", fault)
		}
		_, fault = svc.SubmitReading(context.Background(), id, ReadingRequest{
			OperationID: inspection.OperationID("op-prot-" + strconvItoa(i)),
			Generation:  1, Type: evidence.EvidenceProtein,
			BlindCode: code, Value: "3.1",
		})
		if fault != nil {
			t.Fatalf("protein reading: %v", fault)
		}
	}

	// Two independent reviewers pass.
	for _, reviewer := range []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB} {
		_, fault := svc.Review(context.Background(), id, ReviewRequest{
			OperationID: inspection.OperationID("op-rev-" + string(reviewer)),
			Generation:  1, Reviewer: reviewer, Conclusion: "pass",
		})
		if fault != nil {
			t.Fatalf("review: %v", fault)
		}
	}

	result, fault := svc.Finalize(context.Background(), id, FinalizeRequest{
		OperationID: "op-final", Generation: 1, Outcome: inspection.FinalAdmissible,
	})
	if fault != nil {
		t.Fatalf("finalize: %v", fault)
	}
	if result.FinalType != inspection.FinalAdmissible || result.Credential == "" {
		t.Fatalf("unexpected final result: %+v", result)
	}

	snap, fault := svc.GetSnapshot(context.Background(), id)
	if fault != nil {
		t.Fatalf("snapshot: %v", fault)
	}
	if snap.Task.Status != inspection.StatusAdmissible {
		t.Fatalf("status = %s, want admissible", snap.Task.Status)
	}
	if len(snap.Evidence) != 12 {
		t.Fatalf("evidence count = %d, want 12", len(snap.Evidence))
	}
	if len(snap.Reviews) != 2 {
		t.Fatalf("reviews = %d, want 2", len(snap.Reviews))
	}
}
