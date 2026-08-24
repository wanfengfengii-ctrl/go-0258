package service_test

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_SamplingConfirmRetryAfterBlindSplittingUsesRecordedIdempotency(t *testing.T) {
	ctx := context.Background()
	const taskID inspection.TaskID = "task-sampling-idempotent-retry"

	samplingRequest := func(op inspection.OperationID, person catalog.PersonID) service.SamplingConfirmationRequest {
		return service.SamplingConfirmationRequest{
			OperationID:  op,
			Person:       person,
			FarmID:       catalog.FixedFarmID,
			TankBatch:    "BATCH-2026-001",
			Compartments: []catalog.CompartmentCode{"A", "B"},
			Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
			Generation:   1,
		}
	}

	cases := []struct {
		name         string
		retryRequest func(service.SamplingConfirmationRequest) service.SamplingConfirmationRequest
		wantFault    string
		wantStatus   inspection.Status
		wantComplete bool
	}{
		{
			name: "identical recorded retry returns stable blind splitting result",
			retryRequest: func(first service.SamplingConfirmationRequest) service.SamplingConfirmationRequest {
				return first
			},
			wantStatus:   inspection.StatusBlindSplitting,
			wantComplete: true,
		},
		{
			name: "same recorded operation with changed content remains content conflict",
			retryRequest: func(first service.SamplingConfirmationRequest) service.SamplingConfirmationRequest {
				first.TankBatch = "BATCH-2026-999"
				return first
			},
			wantFault: service.CodeContentConflict,
		},
		{
			name: "unrecorded sampling operation remains gated by current status",
			retryRequest: func(service.SamplingConfirmationRequest) service.SamplingConfirmationRequest {
				return samplingRequest("op-sample-new-after-blind", catalog.FixedSamplerA)
			},
			wantFault: service.CodeIllegalTransition,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			defer st.Close()
			svc := service.NewService(st, service.NewManualClock(1000))

			_, fault := svc.CreateTask(ctx, service.CreateTaskRequest{
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

			firstReq := samplingRequest("op-sample-a", catalog.FixedSamplerA)
			if _, fault := svc.SamplingConfirm(ctx, taskID, firstReq); fault != nil {
				t.Fatalf("first sampling confirm: %v", fault)
			}
			if _, fault := svc.SamplingConfirm(ctx, taskID, samplingRequest("op-sample-b", catalog.FixedSamplerB)); fault != nil {
				t.Fatalf("second sampling confirm: %v", fault)
			}

			snapshot, fault := svc.GetSnapshot(ctx, taskID)
			if fault != nil {
				t.Fatalf("snapshot after dual confirm: %v", fault)
			}
			if snapshot.Task.Status != inspection.StatusBlindSplitting {
				t.Fatalf("status after dual confirm = %s, want %s", snapshot.Task.Status, inspection.StatusBlindSplitting)
			}

			result, fault := svc.SamplingConfirm(ctx, taskID, tc.retryRequest(firstReq))
			if tc.wantFault != "" {
				if fault == nil || fault.Code != tc.wantFault {
					t.Fatalf("fault = %v, want %s", fault, tc.wantFault)
				}
				return
			}
			if fault != nil {
				t.Fatalf("retry fault = %v, want success", fault)
			}
			if result.Status != tc.wantStatus || result.Complete != tc.wantComplete || len(result.Confirmed) != 2 {
				t.Fatalf("retry result = %+v, want status %s complete %t with two confirmations", result, tc.wantStatus, tc.wantComplete)
			}
		})
	}
}
