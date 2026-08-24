package service

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_AuditLogScopedByTask(t *testing.T) {
	cases := []struct {
		name                string
		reopenBeforeTaskBOp bool
	}{
		{name: "live_snapshot_append"},
		{name: "restart_recovery_then_append", reopenBeforeTaskBOp: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat := catalog.NewFixedCatalog()
			clock := NewManualClock(1000)
			dbPath := ":memory:"
			if tc.reopenBeforeTaskBOp {
				dbPath = filepath.Join(t.TempDir(), "audit-scope.db")
			}

			st, err := store.OpenSQLite(dbPath, cat)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = st.Close() }()
			svc := NewService(st, clock)

			taskA := inspection.TaskID("task-a-" + tc.name)
			taskB := inspection.TaskID("task-b-" + tc.name)
			createAuditScopeTask(t, svc, taskA, inspection.TankBatch("BATCH-A-"+tc.name))
			advanceAuditScopeTaskToReading(t, svc, taskA, "A-"+tc.name)
			createAuditScopeTask(t, svc, taskB, inspection.TankBatch("BATCH-B-"+tc.name))

			if tc.reopenBeforeTaskBOp {
				if err := st.Close(); err != nil {
					t.Fatal(err)
				}
				st, err = store.OpenSQLite(dbPath, cat)
				if err != nil {
					t.Fatal(err)
				}
				svc = NewService(st, clock)
			}

			confirmAuditScopeSampling(t, svc, taskB, inspection.TankBatch("BATCH-B-"+tc.name))

			assertAuditScopeTrail(t, svc, taskA, []inspection.EventType{
				inspection.EventTaskCreated,
				inspection.EventSampled,
				inspection.EventSampled,
				inspection.EventBlindSplit,
				inspection.EventOccupied,
				inspection.EventColdChain,
				inspection.EventReading,
			})
			snapB := assertAuditScopeTrail(t, svc, taskB, []inspection.EventType{
				inspection.EventTaskCreated,
				inspection.EventSampled,
				inspection.EventSampled,
			})
			if snapB.Task.Status != inspection.StatusBlindSplitting {
				t.Fatalf("task B status = %s, want %s", snapB.Task.Status, inspection.StatusBlindSplitting)
			}
		})
	}
}

func createAuditScopeTask(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch) {
	t.Helper()
	_, fault := svc.CreateTask(context.Background(), CreateTaskRequest{
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
}

func advanceAuditScopeTaskToReading(t *testing.T, svc *Service, id inspection.TaskID, suffix string) {
	t.Helper()
	batch := inspection.TankBatch("BATCH-" + suffix)
	confirmAuditScopeSampling(t, svc, id, batch)
	_, fault := svc.BlindSplit(context.Background(), id, BlindSplitRequest{
		OperationID: inspection.OperationID(string(id) + "-split"),
		Generation:  1,
		Codes: []blindcode.BlindCode{
			blindcode.BlindCode(suffix + "-CODE-A"),
			blindcode.BlindCode(suffix + "-CODE-B"),
		},
	})
	if fault != nil {
		t.Fatalf("blind split %s: %v", id, fault)
	}
	_, fault = svc.AcquireOccupancy(context.Background(), id, OccupancyRequest{
		OperationID: inspection.OperationID(string(id) + "-occupancy"),
		Generation:  1,
		Occupancies: []occupancy.Occupancy{
			{ResourceType: occupancy.ResourcePlateWell, PlateID: "plate-" + string(id), Well: "A1", StartAt: 0, EndAt: 3600},
		},
	})
	if fault != nil {
		t.Fatalf("occupancy %s: %v", id, fault)
	}
	writeAuditScopeColdChain(t, svc, id)
	_, fault = svc.SubmitReading(context.Background(), id, ReadingRequest{
		OperationID: inspection.OperationID(string(id) + "-reading"),
		Generation:  1,
		Type:        evidence.EvidenceAntibiotic,
		BlindCode:   suffix + "-CODE-A",
		Well:        "A1",
		Value:       "20.0",
	})
	if fault != nil {
		t.Fatalf("reading %s: %v", id, fault)
	}
}

func confirmAuditScopeSampling(t *testing.T, svc *Service, id inspection.TaskID, batch inspection.TankBatch) {
	t.Helper()
	for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
		_, fault := svc.SamplingConfirm(context.Background(), id, SamplingConfirmationRequest{
			OperationID:  inspection.OperationID(string(id) + "-sample-" + strconv.Itoa(i)),
			Person:       sampler,
			FarmID:       catalog.FixedFarmID,
			TankBatch:    batch,
			Compartments: []catalog.CompartmentCode{"A", "B"},
			Seals:        []catalog.SealCode{"seal-0001", "seal-0002"},
			Generation:   1,
		})
		if fault != nil {
			t.Fatalf("sampling %s/%d: %v", id, i, fault)
		}
	}
}

func writeAuditScopeColdChain(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	rules, _ := svc.Catalog().Rules(catalog.FixedRuleVersion)
	cells := make([]evidence.TemperatureCell, 0, int(rules.Temperature.WindowSeconds/rules.Temperature.SampleEverySeconds)+1)
	for i := int64(0); i <= rules.Temperature.WindowSeconds; i += rules.Temperature.SampleEverySeconds {
		cells = append(cells, evidence.TemperatureCell{
			AtSeconds: i,
			Celsius:   evidence.FixedPoint{Value: 40, Scale: 1},
		})
	}
	_, fault := svc.ColdChainReadings(context.Background(), id, ColdChainReadingsRequest{
		OperationID: inspection.OperationID(string(id) + "-cold-chain"),
		Generation:  1,
		BaseTime:    0,
		RecorderID:  "recorder-x1",
		Cells:       cells,
	})
	if fault != nil {
		t.Fatalf("cold chain %s: %v", id, fault)
	}
}

func assertAuditScopeTrail(t *testing.T, svc *Service, id inspection.TaskID, want []inspection.EventType) *store.Snapshot {
	t.Helper()
	snap, fault := svc.GetSnapshot(context.Background(), id)
	if fault != nil {
		t.Fatalf("snapshot %s: %v", id, fault)
	}
	if len(snap.Audit) != len(want) {
		t.Fatalf("snapshot %s audit count = %d, want %d; got %s", id, len(snap.Audit), len(want), auditScopeSummary(snap.Audit))
	}
	for i, ev := range snap.Audit {
		if ev.TaskID != id {
			t.Fatalf("snapshot %s audit[%d] task = %s, want %s; got %s", id, i, ev.TaskID, id, auditScopeSummary(snap.Audit))
		}
		if ev.Sequence != int64(i+1) {
			t.Fatalf("snapshot %s audit[%d] sequence = %d, want %d; got %s", id, i, ev.Sequence, i+1, auditScopeSummary(snap.Audit))
		}
		if ev.EventType != want[i] {
			t.Fatalf("snapshot %s audit[%d] type = %s, want %s; got %s", id, i, ev.EventType, want[i], auditScopeSummary(snap.Audit))
		}
	}
	return snap
}

func auditScopeSummary(events []inspection.AuditEvent) string {
	out := ""
	for i, ev := range events {
		if i > 0 {
			out += ", "
		}
		out += string(ev.TaskID) + "#" + strconv.FormatInt(ev.Sequence, 10) + ":" + string(ev.EventType)
	}
	return out
}
