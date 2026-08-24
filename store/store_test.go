package store

import (
	"context"
	"errors"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
)

func newTestStore(t *testing.T) *MemoryStore {
	t.Helper()
	return NewMemoryStore(catalog.NewFixedCatalog())
}

func createTask(t *testing.T, s Store, task inspection.Task) inspection.Task {
	t.Helper()
	err := s.WithTx(context.Background(), func(tx Tx) error {
		return tx.CreateTask(context.Background(), task)
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	return task
}

func TestMemoryStoreCreateAndGet(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	task := inspection.Task{
		ID: "task-1", FarmID: catalog.FixedFarmID, TankBatch: "B-001",
		Status: inspection.StatusPendingBuild, CreatedAt: 1, Generation: 1,
	}
	createTask(t, s, task)
	got, err := s.GetTask(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TankBatch != "B-001" || got.Generation != 1 {
		t.Fatalf("unexpected task: %+v", got)
	}
}

func TestMemoryStoreCreateConflict(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	task := inspection.Task{ID: "task-1", Status: inspection.StatusPendingBuild, Generation: 1}
	createTask(t, s, task)
	err := s.WithTx(context.Background(), func(tx Tx) error {
		return tx.CreateTask(context.Background(), task)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestMemoryStoreNotFound(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	if _, err := s.GetTask(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestMemoryStoreListOrdered(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	createTask(t, s, inspection.Task{ID: "t2", Status: inspection.StatusPendingBuild, CreatedAt: 2, Generation: 1})
	createTask(t, s, inspection.Task{ID: "t1", Status: inspection.StatusPendingBuild, CreatedAt: 1, Generation: 1})
	tasks, err := s.ListTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].ID != "t1" || tasks[1].ID != "t2" {
		t.Fatalf("unexpected ordering: %+v", tasks)
	}
}

func TestOccupancyConflictRejected(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	well := func(taskID string) occupancy.Occupancy {
		return occupancy.Occupancy{
			TaskID: taskID, ResourceType: occupancy.ResourcePlateWell,
			PlateID: "P1", Well: "A1", StartAt: 0, EndAt: 100, Generation: 1,
		}
	}
	createTask(t, s, inspection.Task{ID: "t1", Status: inspection.StatusPendingBuild, Generation: 1})
	createTask(t, s, inspection.Task{ID: "t2", Status: inspection.StatusPendingBuild, Generation: 1})

	err := s.WithTx(context.Background(), func(tx Tx) error {
		return tx.AcquireOccupancy(context.Background(), well("t1"))
	})
	if err != nil {
		t.Fatal(err)
	}
	err = s.WithTx(context.Background(), func(tx Tx) error {
		return tx.AcquireOccupancy(context.Background(), well("t2"))
	})
	if !errors.Is(err, occupancy.ErrOccupied) {
		t.Fatalf("err = %v, want ErrOccupied", err)
	}
}

func TestIdempotencyRoundTrip(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	createTask(t, s, inspection.Task{ID: "t1", Status: inspection.StatusPendingBuild, Generation: 1})
	rec := inspection.IdempotencyRecord{
		TaskID: "t1", OperationID: "op-1", OperationType: inspection.OpSamplingConfirm,
		RequestDigest: "abc", LogicalTime: 5,
	}
	err := s.WithTx(context.Background(), func(tx Tx) error {
		return tx.PutIdempotency(context.Background(), rec)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := func() (inspection.IdempotencyRecord, bool, error) {
		var out inspection.IdempotencyRecord
		var ok bool
		err := s.WithTx(context.Background(), func(tx Tx) error {
			var e error
			out, ok, e = tx.GetIdempotency(context.Background(), "t1", "op-1")
			return e
		})
		return out, ok, err
	}()
	if err != nil || !ok {
		t.Fatalf("get idempotency: ok=%v err=%v", ok, err)
	}
	if got.RequestDigest != "abc" {
		t.Fatalf("digest = %s, want abc", got.RequestDigest)
	}
}

func TestEvidenceRoundTrip(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	createTask(t, s, inspection.Task{ID: "t1", Status: inspection.StatusPendingBuild, Generation: 1})
	rec := evidence.EvidenceRecord{
		TaskID: "t1", BlindCode: "BC1", Type: evidence.EvidenceAntibiotic,
		Raw: evidence.FixedPoint{Value: 190, Scale: 1}, RuleVersion: catalog.FixedRuleVersion,
		Generation: 1, Immutable: true,
	}
	err := s.WithTx(context.Background(), func(tx Tx) error {
		return tx.PutEvidence(context.Background(), rec)
	})
	if err != nil {
		t.Fatal(err)
	}
	recs, err := func() ([]evidence.EvidenceRecord, error) {
		var out []evidence.EvidenceRecord
		err := s.WithTx(context.Background(), func(tx Tx) error {
			var e error
			out, e = tx.ListEvidence(context.Background(), "t1")
			return e
		})
		return out, err
	}()
	if err != nil || len(recs) != 1 || recs[0].Raw.Value != 190 {
		t.Fatalf("evidence: %+v err=%v", recs, err)
	}
}

func TestUpdateTaskCASConflict(t *testing.T) {
	s := newTestStore(t)
	defer s.Close()
	task := inspection.Task{ID: "t1", Status: inspection.StatusPendingSampling, Generation: 1}
	createTask(t, s, task)
	updated := task
	updated.Status = inspection.StatusBlindSplitting
	// Wrong expected status must conflict.
	err := s.WithTx(context.Background(), func(tx Tx) error {
		return tx.UpdateTaskCAS(context.Background(), "t1", inspection.StatusPlateOccupied, 1, updated)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}
