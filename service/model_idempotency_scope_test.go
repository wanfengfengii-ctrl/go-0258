package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func TestModel_SamplingConfirmationIdempotencyScopesByTaskAndOperation(t *testing.T) {
	ctx := context.Background()
	operationID := inspection.OperationID("op-reused-by-frontend")
	compartments := []catalog.CompartmentCode{"A", "B"}
	seals := []catalog.SealCode{"seal-0001", "seal-0002"}

	type issueCase struct {
		name string
		run  func(t *testing.T, svc *Service, taskA, taskB inspection.TaskID)
	}

	cases := []issueCase{
		{
			name: "same task same content retry returns existing result",
			run: func(t *testing.T, svc *Service, taskA, _ inspection.TaskID) {
				req := SamplingConfirmationRequest{
					OperationID:  operationID,
					Person:       catalog.FixedSamplerA,
					FarmID:       catalog.FixedFarmID,
					TankBatch:    "BATCH-2026-101",
					Compartments: compartments,
					Seals:        seals,
					Generation:   1,
				}

				first, fault := svc.SamplingConfirm(ctx, taskA, req)
				if fault != nil {
					t.Fatalf("first confirmation fault = %v", fault)
				}
				again, fault := svc.SamplingConfirm(ctx, taskA, req)
				if fault != nil {
					t.Fatalf("retry fault = %v", fault)
				}
				if first.TaskID != again.TaskID || first.Status != again.Status ||
					first.Generation != again.Generation || first.Complete != again.Complete ||
					len(first.Confirmed) != len(again.Confirmed) {
					t.Fatalf("retry result = %+v, want existing result %+v", again, first)
				}
			},
		},
		{
			name: "same task same operation with different content conflicts",
			run: func(t *testing.T, svc *Service, taskA, _ inspection.TaskID) {
				req := SamplingConfirmationRequest{
					OperationID:  operationID,
					Person:       catalog.FixedSamplerA,
					FarmID:       catalog.FixedFarmID,
					TankBatch:    "BATCH-2026-101",
					Compartments: compartments,
					Seals:        seals,
					Generation:   1,
				}
				if _, fault := svc.SamplingConfirm(ctx, taskA, req); fault != nil {
					t.Fatalf("first confirmation fault = %v", fault)
				}

				req.Person = catalog.FixedSamplerB
				_, fault := svc.SamplingConfirm(ctx, taskA, req)
				if fault == nil || fault.Code != CodeContentConflict {
					t.Fatalf("fault = %v, want %s", fault, CodeContentConflict)
				}
			},
		},
		{
			name: "different tasks may reuse operation id independently",
			run: func(t *testing.T, svc *Service, taskA, taskB inspection.TaskID) {
				firstTaskReq := SamplingConfirmationRequest{
					OperationID:  operationID,
					Person:       catalog.FixedSamplerA,
					FarmID:       catalog.FixedFarmID,
					TankBatch:    "BATCH-2026-101",
					Compartments: compartments,
					Seals:        seals,
					Generation:   1,
				}
				if _, fault := svc.SamplingConfirm(ctx, taskA, firstTaskReq); fault != nil {
					t.Fatalf("first task confirmation fault = %v", fault)
				}

				secondTaskReq := firstTaskReq
				secondTaskReq.TankBatch = "BATCH-2026-202"
				second, fault := svc.SamplingConfirm(ctx, taskB, secondTaskReq)
				if fault != nil {
					t.Fatalf("second task reused operation id fault = %v", fault)
				}
				if second.TaskID != taskB || len(second.Confirmed) != 1 || second.Confirmed[0] != catalog.FixedSamplerA {
					t.Fatalf("second task result = %+v, want its own first confirmation", second)
				}

				again, fault := svc.SamplingConfirm(ctx, taskB, secondTaskReq)
				if fault != nil {
					t.Fatalf("second task retry fault = %v", fault)
				}
				if again.TaskID != second.TaskID || again.Status != second.Status ||
					again.Generation != second.Generation || again.Complete != second.Complete ||
					len(again.Confirmed) != len(second.Confirmed) {
					t.Fatalf("second task retry result = %+v, want existing result %+v", again, second)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()

			for _, task := range []struct {
				id    inspection.TaskID
				batch inspection.TankBatch
			}{
				{id: "task-idempotency-a", batch: "BATCH-2026-101"},
				{id: "task-idempotency-b", batch: "BATCH-2026-202"},
			} {
				_, fault := svc.CreateTask(ctx, CreateTaskRequest{
					TaskID:        task.id,
					FarmID:        catalog.FixedFarmID,
					TankBatch:     task.batch,
					Compartments:  compartments,
					Seals:         seals,
					RecorderModel: "recorder-x1",
					RuleVersion:   catalog.FixedRuleVersion,
					Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
				})
				if fault != nil {
					t.Fatalf("create %s: %v", task.id, fault)
				}
			}

			tc.run(t, svc, "task-idempotency-a", "task-idempotency-b")
		})
	}
}
