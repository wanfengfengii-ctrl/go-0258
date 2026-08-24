package service_test

import (
	"context"
	"reflect"
	"strconv"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_BlindSplitIdempotentRetryAfterAdvance(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name      string
		retry     func(service.BlindSplitRequest) service.BlindSplitRequest
		wantFault string
	}{
		{
			name: "same_task_operation_and_digest_replays_split_result",
			retry: func(req service.BlindSplitRequest) service.BlindSplitRequest {
				return req
			},
		},
		{
			name: "same_task_operation_with_different_digest_conflicts",
			retry: func(req service.BlindSplitRequest) service.BlindSplitRequest {
				req.Codes = []blindcode.BlindCode{"BCODE-A", "BCODE-C"}
				return req
			},
			wantFault: service.CodeContentConflict,
		},
		{
			name: "new_operation_after_advance_still_uses_state_machine",
			retry: func(req service.BlindSplitRequest) service.BlindSplitRequest {
				req.OperationID = "op-blind-split-new"
				return req
			},
			wantFault: service.CodeIllegalTransition,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			defer st.Close()
			svc := service.NewService(st, service.NewManualClock(1000))
			taskID := inspection.TaskID("task-blind-split-retry")

			if _, fault := svc.CreateTask(ctx, service.CreateTaskRequest{
				TaskID:        taskID,
				FarmID:        catalog.FixedFarmID,
				TankBatch:     "BATCH-2026-001",
				Compartments:  []catalog.CompartmentCode{"A", "B"},
				Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
				RecorderModel: "recorder-x1",
				RuleVersion:   catalog.FixedRuleVersion,
				Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
			}); fault != nil {
				t.Fatalf("create task: %v", fault)
			}

			for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
				if _, fault := svc.SamplingConfirm(ctx, taskID, service.SamplingConfirmationRequest{
					OperationID:  inspection.OperationID("op-sample-" + strconv.Itoa(i)),
					Person:       sampler,
					FarmID:       catalog.FixedFarmID,
					TankBatch:    "BATCH-2026-001",
					Compartments: []catalog.CompartmentCode{"A", "B"},
					Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
					Generation:   1,
				}); fault != nil {
					t.Fatalf("sampling confirm %d: %v", i, fault)
				}
			}

			firstReq := service.BlindSplitRequest{
				OperationID: "op-blind-split-timeout",
				Generation:  1,
				Codes:       []blindcode.BlindCode{"BCODE-A", "BCODE-B"},
			}
			first, fault := svc.BlindSplit(ctx, taskID, firstReq)
			if fault != nil {
				t.Fatalf("first blind split: %v", fault)
			}
			if first.Status != inspection.StatusPlateOccupied {
				t.Fatalf("first status = %s, want %s", first.Status, inspection.StatusPlateOccupied)
			}

			got, fault := svc.BlindSplit(ctx, taskID, tc.retry(firstReq))
			if tc.wantFault != "" {
				if fault == nil || fault.Code != tc.wantFault {
					t.Fatalf("retry fault = %v, want %s", fault, tc.wantFault)
				}
				return
			}
			if fault != nil {
				t.Fatalf("idempotent retry returned fault after status advance: %v", fault)
			}
			if !reflect.DeepEqual(got, first) {
				t.Fatalf("retry result = %+v, want first result %+v", got, first)
			}
		})
	}
}
