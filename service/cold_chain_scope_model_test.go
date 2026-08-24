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
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_ColdChainGridScopedByTask(t *testing.T) {
	ctx := context.Background()
	baseTime := int64(1710000000)
	sharedRecorder := "recorder-shared-window"

	newService := func(t *testing.T) (*Service, *store.MemoryStore) {
		t.Helper()
		st := store.NewMemoryStore(catalog.NewFixedCatalog())
		return NewService(st, NewManualClock(1000)), st
	}

	coldCells := func(t *testing.T, svc *Service, base int64) []evidence.TemperatureCell {
		t.Helper()
		rules, ok := svc.Catalog().Rules(catalog.FixedRuleVersion)
		if !ok {
			t.Fatal("fixed rules missing")
		}
		window := rules.Temperature
		count := int(window.WindowSeconds/window.SampleEverySeconds) + 1
		cells := make([]evidence.TemperatureCell, count)
		for i := range cells {
			cells[i] = evidence.TemperatureCell{
				AtSeconds: base + int64(i)*window.SampleEverySeconds,
				Celsius:   evidence.FixedPoint{Value: 40, Scale: window.Scale},
			}
		}
		return cells
	}

	reachColdChain := func(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch, plateID, incubatorID string, codes []blindcode.BlindCode) {
		t.Helper()
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
			t.Fatalf("create %s: %v", id, fault)
		}

		for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
			_, fault = svc.SamplingConfirm(ctx, id, SamplingConfirmationRequest{
				OperationID:  inspection.OperationID("op-sample-" + string(id) + "-" + strconv.Itoa(i)),
				Person:       sampler,
				FarmID:       catalog.FixedFarmID,
				TankBatch:    batch,
				Compartments: []catalog.CompartmentCode{"A", "B"},
				Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
				Generation:   1,
			})
			if fault != nil {
				t.Fatalf("sample %s/%d: %v", id, i, fault)
			}
		}

		_, fault = svc.BlindSplit(ctx, id, BlindSplitRequest{
			OperationID: inspection.OperationID("op-split-" + string(id)),
			Generation:  1,
			Codes:       codes,
		})
		if fault != nil {
			t.Fatalf("split %s: %v", id, fault)
		}

		_, fault = svc.AcquireOccupancy(ctx, id, OccupancyRequest{
			OperationID: inspection.OperationID("op-occupy-" + string(id)),
			Generation:  1,
			Occupancies: []occupancy.Occupancy{
				{ResourceType: occupancy.ResourcePlateWell, PlateID: plateID, Well: "A1", StartAt: 0, EndAt: 3600},
				{ResourceType: occupancy.ResourceIncubator, IncubatorID: incubatorID, StartAt: 0, EndAt: 3600},
			},
		})
		if fault != nil {
			t.Fatalf("occupy %s: %v", id, fault)
		}
	}

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "complete cold chain advances separate tasks sharing recorder and window",
			run: func(t *testing.T) {
				svc, st := newService(t)
				defer st.Close()

				firstID := inspection.TaskID("task-cold-shared-a")
				secondID := inspection.TaskID("task-cold-shared-b")
				reachColdChain(t, svc, firstID, "BATCH-COLD-A", "plate-cold-a", "inc-cold-a", []blindcode.BlindCode{"COLD-A1", "COLD-A2"})
				reachColdChain(t, svc, secondID, "BATCH-COLD-B", "plate-cold-b", "inc-cold-b", []blindcode.BlindCode{"COLD-B1", "COLD-B2"})

				first, fault := svc.ColdChainReadings(ctx, firstID, ColdChainReadingsRequest{
					OperationID: "op-cold-shared-a",
					Generation:  1,
					BaseTime:    baseTime,
					RecorderID:  sharedRecorder,
					Cells:       coldCells(t, svc, baseTime),
				})
				if fault != nil {
					t.Fatalf("first cold chain: %v", fault)
				}
				if !first.Complete || first.Status != inspection.StatusAntibioticReading {
					t.Fatalf("first result = %+v, want complete antibiotic_reading", first)
				}

				second, fault := svc.ColdChainReadings(ctx, secondID, ColdChainReadingsRequest{
					OperationID: "op-cold-shared-b",
					Generation:  1,
					BaseTime:    baseTime,
					RecorderID:  sharedRecorder,
					Cells:       coldCells(t, svc, baseTime),
				})
				if fault != nil {
					t.Fatalf("second cold chain with shared recorder/window: %v", fault)
				}
				if !second.Complete || second.Status != inspection.StatusAntibioticReading {
					t.Fatalf("second result = %+v, want complete antibiotic_reading", second)
				}
			},
		},
		{
			name: "same task duplicate time point is rejected",
			run: func(t *testing.T) {
				svc, st := newService(t)
				defer st.Close()

				id := inspection.TaskID("task-cold-duplicate")
				reachColdChain(t, svc, id, "BATCH-COLD-DUP", "plate-cold-dup", "inc-cold-dup", []blindcode.BlindCode{"COLD-D1", "COLD-D2"})
				cells := coldCells(t, svc, baseTime)
				cells[1].AtSeconds = cells[0].AtSeconds

				_, fault := svc.ColdChainReadings(ctx, id, ColdChainReadingsRequest{
					OperationID: "op-cold-duplicate",
					Generation:  1,
					BaseTime:    baseTime,
					RecorderID:  sharedRecorder,
					Cells:       cells,
				})
				if fault == nil || fault.Code != CodeTemperatureDuplicate {
					t.Fatalf("fault = %v, want %s", fault, CodeTemperatureDuplicate)
				}
			},
		},
		{
			name: "missing cold chain sample is rejected",
			run: func(t *testing.T) {
				svc, st := newService(t)
				defer st.Close()

				id := inspection.TaskID("task-cold-missing")
				reachColdChain(t, svc, id, "BATCH-COLD-MISS", "plate-cold-miss", "inc-cold-miss", []blindcode.BlindCode{"COLD-M1", "COLD-M2"})
				cells := coldCells(t, svc, baseTime)
				cells = cells[:len(cells)-1]

				_, fault := svc.ColdChainReadings(ctx, id, ColdChainReadingsRequest{
					OperationID: "op-cold-missing",
					Generation:  1,
					BaseTime:    baseTime,
					RecorderID:  sharedRecorder,
					Cells:       cells,
				})
				if fault == nil || fault.Code != CodeTemperatureMissing {
					t.Fatalf("fault = %v, want %s", fault, CodeTemperatureMissing)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}
