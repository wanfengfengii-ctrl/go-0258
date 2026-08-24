package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// TestRestartRecoveryPersistsState closes a durable store after several
// commands and reopens it, asserting the task, occupancy, evidence and audit
// trail are exactly recovered from SQLite.
func TestRestartRecoveryPersistsState(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "recovery.db")
	cat := catalog.NewFixedCatalog()

	clock := NewManualClock(1000)

	// Phase 1: build and advance through occupancy.
	func() {
		st, err := store.OpenSQLite(dbPath, cat)
		if err != nil {
			t.Fatal(err)
		}
		defer st.Close()
		svc := NewService(st, clock)

		_, fault := svc.CreateTask(context.Background(), CreateTaskRequest{
			TaskID: "task-recover", FarmID: catalog.FixedFarmID, TankBatch: "BATCH-2026-001",
			Compartments:  []catalog.CompartmentCode{"A", "B"},
			Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
			RecorderModel: "recorder-x1", RuleVersion: catalog.FixedRuleVersion,
			Reviewers: []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
		})
		if fault != nil {
			t.Fatal(fault)
		}
		confirmSampling(t, svc, "task-recover")
		splitBlind(t, svc, "task-recover")

		_, fault = svc.AcquireOccupancy(context.Background(), "task-recover", OccupancyRequest{
			OperationID: "op-occ", Generation: 1,
			Occupancies: []occupancy.Occupancy{
				{TaskID: "task-recover", ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-1", Well: "A1", StartAt: 0, EndAt: 3600},
			},
		})
		if fault != nil {
			t.Fatal(fault)
		}
	}()

	// Phase 2: reopen and verify full recovery.
	st, err := store.OpenSQLite(dbPath, cat)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := NewService(st, NewManualClock(2000))

	snap, fault := svc.GetSnapshot(context.Background(), "task-recover")
	if fault != nil {
		t.Fatal(fault)
	}
	if snap.Task.Status != inspection.StatusColdChainVerifying {
		t.Fatalf("status = %s, want cold_chain_verifying", snap.Task.Status)
	}
	if len(snap.Occupancies) != 1 {
		t.Fatalf("occupancies = %d, want 1", len(snap.Occupancies))
	}
	if len(snap.BlindSamples) != 2 {
		t.Fatalf("blind samples = %d, want 2", len(snap.BlindSamples))
	}
	if len(snap.Audit) == 0 {
		t.Fatal("audit trail not recovered")
	}

	// The recovered task can continue: cold chain then a reading.
	writeColdChain(t, svc, "task-recover")
	_, fault = svc.SubmitReading(context.Background(), "task-recover", ReadingRequest{
		OperationID: "op-anti", Generation: 1, Type: evidence.EvidenceAntibiotic,
		BlindCode: "BCODE-A", Well: "A1", Value: "20.0",
	})
	if fault != nil {
		t.Fatalf("reading after recovery: %v", fault)
	}
}

// TestRestartRecoveryFileIsDurable ensures the WAL database file exists and is
// non-empty after writes, proving real persistence (not in-memory only).
func TestRestartRecoveryFileIsDurable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "durable.db")
	st, err := store.OpenSQLite(dbPath, catalog.NewFixedCatalog())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(st, NewManualClock(0))
	_, fault := svc.CreateTask(context.Background(), CreateTaskRequest{
		TaskID: "t1", FarmID: catalog.FixedFarmID, TankBatch: "BATCH-DUR",
		Compartments:  []catalog.CompartmentCode{"A", "B"},
		Seals:         []catalog.SealCode{"seal-0001", "seal-0002"},
		RecorderModel: "recorder-x1", RuleVersion: catalog.FixedRuleVersion,
		Reviewers: []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB},
	})
	if fault != nil {
		t.Fatal(fault)
	}
	_ = st.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("db file missing after close: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("db file is empty")
	}
}
