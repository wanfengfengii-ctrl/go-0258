package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_InstrumentFailureDoesNotAdvanceAntibioticReading(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name         string
		scriptResult string
		errorClass   string
		wantClass    string
	}{
		{name: "timeout", scriptResult: "timeout", wantClass: "timeout"},
		{name: "rejected", scriptResult: "rejected", wantClass: "rejected"},
		{name: "explicit error class", scriptResult: "bad-payload", errorClass: "malformed", wantClass: "malformed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			defer st.Close()
			svc := NewService(st, NewManualClock(1000))
			taskID := inspection.TaskID("task-instrument-failure")

			_, fault := svc.CreateTask(ctx, CreateTaskRequest{
				TaskID:        taskID,
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

			for _, sampler := range []struct {
				operation inspection.OperationID
				person    catalog.PersonID
			}{
				{operation: "op-sample-a", person: catalog.FixedSamplerA},
				{operation: "op-sample-b", person: catalog.FixedSamplerB},
			} {
				_, fault = svc.SamplingConfirm(ctx, taskID, SamplingConfirmationRequest{
					OperationID:  sampler.operation,
					Person:       sampler.person,
					FarmID:       catalog.FixedFarmID,
					TankBatch:    "BATCH-2026-001",
					Compartments: []catalog.CompartmentCode{"A", "B"},
					Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
					Generation:   1,
				})
				if fault != nil {
					t.Fatalf("sampling confirm %s: %v", sampler.operation, fault)
				}
			}

			_, fault = svc.BlindSplit(ctx, taskID, BlindSplitRequest{
				OperationID: "op-split",
				Generation:  1,
				Codes:       []blindcode.BlindCode{"BCODE-A", "BCODE-B"},
			})
			if fault != nil {
				t.Fatalf("blind split: %v", fault)
			}

			_, fault = svc.AcquireOccupancy(ctx, taskID, OccupancyRequest{
				OperationID: "op-occupancy",
				Generation:  1,
				Occupancies: []occupancy.Occupancy{
					{ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-1", Well: "A1", StartAt: 0, EndAt: 3600},
					{ResourceType: occupancy.ResourceIncubator, IncubatorID: "incubator-1", StartAt: 0, EndAt: 3600},
				},
			})
			if fault != nil {
				t.Fatalf("occupancy: %v", fault)
			}

			rules, ok := svc.Catalog().Rules(catalog.FixedRuleVersion)
			if !ok {
				t.Fatal("fixed rule version not found")
			}
			cellCount := int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds) + 1
			cells := make([]evidence.TemperatureCell, 0, cellCount)
			for i := 0; i < cellCount; i++ {
				cells = append(cells, evidence.TemperatureCell{
					AtSeconds: int64(i) * rules.Temperature.SampleEverySeconds,
					Celsius:   evidence.FixedPoint{Value: 40, Scale: 1},
				})
			}
			_, fault = svc.ColdChainReadings(ctx, taskID, ColdChainReadingsRequest{
				OperationID: "op-cold-chain",
				Generation:  1,
				BaseTime:    0,
				RecorderID:  "recorder-x1",
				Cells:       cells,
			})
			if fault != nil {
				t.Fatalf("cold chain: %v", fault)
			}

			before, fault := svc.GetSnapshot(ctx, taskID)
			if fault != nil {
				t.Fatalf("snapshot before instrument failure: %v", fault)
			}
			if before.Task.Status != inspection.StatusAntibioticReading {
				t.Fatalf("precondition status = %s, want %s", before.Task.Status, inspection.StatusAntibioticReading)
			}
			beforeOccupancies := append([]occupancy.Occupancy(nil), before.Occupancies...)

			result, fault := svc.SubmitReading(ctx, taskID, ReadingRequest{
				OperationID:    "op-instrument-failure",
				Generation:     before.Task.Generation,
				Type:           evidence.EvidenceAntibiotic,
				BlindCode:      "BCODE-A",
				Well:           "A1",
				InstrumentType: "inhibition-reader",
				ScriptResult:   tc.scriptResult,
				ErrorClass:     tc.errorClass,
			})
			if fault != nil {
				t.Fatalf("submit instrument failure: %v", fault)
			}
			if result.Pass {
				t.Fatal("instrument failure returned a passing reading")
			}
			if result.Status != inspection.StatusAntibioticReading {
				t.Fatalf("result status = %s, want %s", result.Status, inspection.StatusAntibioticReading)
			}
			if result.Instrument == nil {
				t.Fatal("result did not include an instrument call")
			}

			after, fault := svc.GetSnapshot(ctx, taskID)
			if fault != nil {
				t.Fatalf("snapshot after instrument failure: %v", fault)
			}
			if after.Task.Status != inspection.StatusAntibioticReading {
				t.Fatalf("status advanced after %s failure: got %s, want %s", tc.name, after.Task.Status, inspection.StatusAntibioticReading)
			}
			if after.Task.Generation != before.Task.Generation {
				t.Fatalf("generation changed after %s failure: got %d, want %d", tc.name, after.Task.Generation, before.Task.Generation)
			}
			if len(after.Evidence) != 0 {
				t.Fatalf("instrument %s failure wrote %d evidence records, want 0", tc.name, len(after.Evidence))
			}
			if !reflect.DeepEqual(after.Occupancies, beforeOccupancies) {
				t.Fatalf("occupancies changed after %s failure: before=%+v after=%+v", tc.name, beforeOccupancies, after.Occupancies)
			}
			if len(after.InstrumentCalls) != 1 {
				t.Fatalf("instrument calls = %d, want 1", len(after.InstrumentCalls))
			}
			call := after.InstrumentCalls[0]
			if call.InstrumentType != "inhibition-reader" || call.Target != "BCODE-A" || call.ScriptResult != tc.scriptResult {
				t.Fatalf("instrument call does not preserve failed invocation: %+v", call)
			}
			if call.ErrorClass != tc.wantClass {
				t.Fatalf("error class = %s, want %s", call.ErrorClass, tc.wantClass)
			}
			if call.RetryCount != 0 || call.NextRetryAt != 1001 {
				t.Fatalf("retry plan = count %d at %d, want count 0 at 1001", call.RetryCount, call.NextRetryAt)
			}
		})
	}
}
