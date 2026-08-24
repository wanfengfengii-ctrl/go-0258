package service

import (
	"context"
	"testing"
	"time"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_SnapshotReadOnlyTransactionsReleaseConnections(t *testing.T) {
	const taskID inspection.TaskID = "task-snapshot-read-tx-release"

	newSQLiteService := func(t *testing.T) (*Service, *store.SQLiteStore) {
		t.Helper()
		st, err := store.OpenSQLite(":memory:", catalog.NewFixedCatalog())
		if err != nil {
			t.Fatalf("open sqlite store: %v", err)
		}
		return NewService(st, NewManualClock(1000)), st
	}

	createTask := func(t *testing.T, svc *Service) {
		t.Helper()
		_, fault := svc.CreateTask(context.Background(), CreateTaskRequest{
			TaskID:        taskID,
			FarmID:        catalog.FixedFarmID,
			TankBatch:     "BATCH-SNAPSHOT-TX",
			Compartments:  []catalog.CompartmentCode{"A", "B"},
			Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
			RecorderModel: "recorder-x1",
			RuleVersion:   catalog.FixedRuleVersion,
			Reviewers:     []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
		})
		if fault != nil {
			t.Fatalf("create task: %v", fault)
		}
	}

	tests := []struct {
		name               string
		firstID            inspection.TaskID
		wantFirstFaultCode string
	}{
		{name: "success releases read transaction", firstID: taskID},
		{name: "not found releases read transaction", firstID: "missing-task", wantFirstFaultCode: CodeNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, st := newSQLiteService(t)
			defer st.Close()
			createTask(t, svc)

			first, fault := svc.GetSnapshot(context.Background(), tt.firstID)
			if tt.wantFirstFaultCode == "" {
				if fault != nil {
					t.Fatalf("first snapshot returned fault: %v", fault)
				}
				if first == nil || first.Task.ID != taskID {
					t.Fatalf("first snapshot task = %+v, want %s", first, taskID)
				}
			} else {
				if fault == nil || fault.Code != tt.wantFirstFaultCode {
					t.Fatalf("first snapshot fault = %v, want code %s", fault, tt.wantFirstFaultCode)
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			second, fault := svc.GetSnapshot(ctx, taskID)
			if fault != nil {
				t.Fatalf("second snapshot after %q returned fault: %v", tt.name, fault)
			}
			if second == nil || second.Task.ID != taskID {
				t.Fatalf("second snapshot task = %+v, want %s", second, taskID)
			}
		})
	}
}
