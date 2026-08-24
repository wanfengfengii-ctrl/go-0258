package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_OverflowReadingRejectedWithoutSideEffects(t *testing.T) {
	parseCases := []struct {
		name      string
		text      string
		scale     int
		wantValue int64
		wantScale int
		wantErr   error
		wantError bool
	}{
		{
			name:    "multiplication overflow at scale",
			text:    "922337203685477581",
			scale:   1,
			wantErr: evidence.ErrOverflow,
		},
		{
			name:      "maximum scaled integer with fraction remains valid",
			text:      "922337203685477580.7",
			scale:     1,
			wantValue: 9223372036854775807,
			wantScale: 1,
		},
		{
			name:      "signed decimal remains valid",
			text:      "-12.3",
			scale:     1,
			wantValue: -123,
			wantScale: 1,
		},
		{
			name:      "positive sign remains valid",
			text:      "+12.3",
			scale:     1,
			wantValue: 123,
			wantScale: 1,
		},
		{
			name:      "fraction beyond scale remains invalid",
			text:      "12.34",
			scale:     1,
			wantError: true,
		},
		{
			name:    "scale limit remains enforced",
			text:    "12",
			scale:   10,
			wantErr: evidence.ErrScale,
		},
	}
	for _, tc := range parseCases {
		t.Run("parse_"+tc.name, func(t *testing.T) {
			got, err := evidence.ParseFixedPoint(tc.text, tc.scale)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ParseFixedPoint(%q, %d) error = %v, want %v", tc.text, tc.scale, err, tc.wantErr)
				}
			case tc.wantError:
				if err == nil {
					t.Fatalf("ParseFixedPoint(%q, %d) succeeded, want error", tc.text, tc.scale)
				}
			default:
				if err != nil {
					t.Fatalf("ParseFixedPoint(%q, %d) error = %v", tc.text, tc.scale, err)
				}
				if got.Value != tc.wantValue || got.Scale != tc.wantScale {
					t.Fatalf("ParseFixedPoint(%q, %d) = %+v, want value %d scale %d", tc.text, tc.scale, got, tc.wantValue, tc.wantScale)
				}
			}
		})
	}

	ctx := context.Background()
	memoryStore := store.NewMemoryStore(catalog.NewFixedCatalog())
	defer memoryStore.Close()
	svc := service.NewService(memoryStore, service.NewManualClock(1000))
	handler := NewHandler(svc)
	taskID := inspection.TaskID("task-overflow-reading")
	operationID := inspection.OperationID("op-overflow-reading")

	mustFaultless := func(step string, fault *service.Fault) {
		t.Helper()
		if fault != nil {
			t.Fatalf("%s: %v", step, fault)
		}
	}
	_, fault := svc.CreateTask(ctx, service.CreateTaskRequest{
		TaskID:        taskID,
		FarmID:        catalog.FixedFarmID,
		TankBatch:     "BATCH-OVERFLOW-001",
		Compartments:  []catalog.CompartmentCode{"A", "B"},
		Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
		RecorderModel: "recorder-x1",
		RuleVersion:   catalog.FixedRuleVersion,
		Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
	})
	mustFaultless("create task", fault)
	for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
		_, fault = svc.SamplingConfirm(ctx, taskID, service.SamplingConfirmationRequest{
			OperationID:  inspection.OperationID("op-overflow-sample-" + string(rune('a'+i))),
			Person:       sampler,
			FarmID:       catalog.FixedFarmID,
			TankBatch:    "BATCH-OVERFLOW-001",
			Compartments: []catalog.CompartmentCode{"A", "B"},
			Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
			Generation:   1,
		})
		mustFaultless("sampling confirmation", fault)
	}
	_, fault = svc.BlindSplit(ctx, taskID, service.BlindSplitRequest{
		OperationID: "op-overflow-split",
		Generation:  1,
		Codes:       []blindcode.BlindCode{"BCODE-A", "BCODE-B"},
	})
	mustFaultless("blind split", fault)
	_, fault = svc.AcquireOccupancy(ctx, taskID, service.OccupancyRequest{
		OperationID: "op-overflow-occupancy",
		Generation:  1,
		Occupancies: []occupancy.Occupancy{
			{ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-overflow", Well: "A1", StartAt: 0, EndAt: 3600},
			{ResourceType: occupancy.ResourceIncubator, IncubatorID: "inc-overflow", StartAt: 0, EndAt: 3600},
		},
	})
	mustFaultless("occupancy", fault)

	rules, ok := svc.Catalog().Rules(catalog.FixedRuleVersion)
	if !ok {
		t.Fatal("fixed rules missing")
	}
	cells := make([]evidence.TemperatureCell, int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds)+1)
	for i := range cells {
		cells[i] = evidence.TemperatureCell{
			AtSeconds: int64(i) * rules.Temperature.SampleEverySeconds,
			Celsius:   evidence.FixedPoint{Value: 40, Scale: rules.Temperature.Scale},
		}
	}
	_, fault = svc.ColdChainReadings(ctx, taskID, service.ColdChainReadingsRequest{
		OperationID: "op-overflow-cold-chain",
		Generation:  1,
		BaseTime:    0,
		RecorderID:  "recorder-x1",
		Cells:       cells,
	})
	mustFaultless("cold chain", fault)

	before, err := memoryStore.Snapshot(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Task.Status != inspection.StatusAntibioticReading {
		t.Fatalf("status before overflow reading = %s, want %s", before.Task.Status, inspection.StatusAntibioticReading)
	}

	apiCases := []struct {
		name       string
		value      string
		wantStatus int
		wantError  string
	}{
		{
			name:       "reading API rejects scale multiplication overflow",
			value:      "922337203685477581",
			wantStatus: http.StatusUnprocessableEntity,
			wantError:  service.CodeArithmeticFailure,
		},
	}
	for _, tc := range apiCases {
		t.Run("api_"+tc.name, func(t *testing.T) {
			body, err := json.Marshal(service.ReadingRequest{
				OperationID: operationID,
				Generation:  1,
				Type:        evidence.EvidenceAntibiotic,
				BlindCode:   "BCODE-A",
				Well:        "A1",
				Value:       tc.value,
			})
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/tasks/"+string(taskID)+"/readings", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var got map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got["error"] != tc.wantError {
				t.Fatalf("error = %v, want %s; body = %s", got["error"], tc.wantError, rec.Body.String())
			}

			after, err := memoryStore.Snapshot(ctx, taskID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Task.Status != before.Task.Status {
				t.Fatalf("status changed to %s, want %s", after.Task.Status, before.Task.Status)
			}
			if len(after.Evidence) != len(before.Evidence) {
				t.Fatalf("evidence count changed from %d to %d", len(before.Evidence), len(after.Evidence))
			}
			if len(after.Audit) != len(before.Audit) {
				t.Fatalf("audit count changed from %d to %d", len(before.Audit), len(after.Audit))
			}
			if len(after.InstrumentCalls) != len(before.InstrumentCalls) {
				t.Fatalf("instrument call count changed from %d to %d", len(before.InstrumentCalls), len(after.InstrumentCalls))
			}

			var exists bool
			if err := memoryStore.WithTx(ctx, func(tx store.Tx) error {
				_, exists, err = tx.GetIdempotency(ctx, taskID, operationID)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if exists {
				t.Fatalf("idempotency record was written for failed reading operation %s", operationID)
			}
		})
	}
}
