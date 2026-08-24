package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_InstrumentFailureLedgerConsistency(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "same task can append repeated failed instrument calls without advancing readings",
			run: func(t *testing.T) {
				svc, st := newFixtureService(t)
				defer st.Close()
				id := createFixtureTask(t, svc)
				confirmSampling(t, svc, id)
				splitBlind(t, svc, id)
				occupyResources(t, svc, id)
				writeColdChain(t, svc, id)

				requests := []ReadingRequest{
					{
						OperationID: "op-inst-fail-1", Generation: 1, Type: evidence.EvidenceAntibiotic,
						BlindCode: "BCODE-A", Well: "A1",
						InstrumentType: "plate-reader", ScriptResult: "timeout", ErrorClass: ErrClassTimeout,
					},
					{
						OperationID: "op-inst-fail-2", Generation: 1, Type: evidence.EvidenceAntibiotic,
						BlindCode: "BCODE-A", Well: "A1",
						InstrumentType: "plate-reader", ScriptResult: "disconnect", ErrorClass: ErrClassDisconnect,
					},
				}
				for _, req := range requests {
					result, fault := svc.SubmitReading(ctx, id, req)
					if fault != nil {
						t.Fatalf("submit %s: %v", req.OperationID, fault)
					}
					if result.Instrument == nil {
						t.Fatalf("submit %s returned no retry plan", req.OperationID)
					}
					if result.Instrument.ErrorClass != req.ErrorClass || result.Instrument.RetryCount != 0 || result.Instrument.NextRetryAt == 0 {
						t.Fatalf("submit %s retry plan = %+v", req.OperationID, result.Instrument)
					}
				}

				snap, fault := svc.GetSnapshot(ctx, id)
				if fault != nil {
					t.Fatal(fault)
				}
				if snap.Task.Status != inspection.StatusAntibioticReading {
					t.Fatalf("status = %s, want antibiotic_reading", snap.Task.Status)
				}
				if len(snap.Evidence) != 0 {
					t.Fatalf("evidence count = %d, want 0", len(snap.Evidence))
				}
				if len(snap.InstrumentCalls) != 2 {
					t.Fatalf("instrument call count = %d, want 2: %+v", len(snap.InstrumentCalls), snap.InstrumentCalls)
				}
				classes := map[string]int{}
				for _, call := range snap.InstrumentCalls {
					if call.TaskID != id || call.Target != "BCODE-A" || call.RetryCount != 0 || call.NextRetryAt == 0 {
						t.Fatalf("bad persisted call: %+v", call)
					}
					classes[call.ErrorClass]++
				}
				if classes[ErrClassTimeout] != 1 || classes[ErrClassDisconnect] != 1 {
					t.Fatalf("persisted error classes = %+v, want one timeout and one disconnect", classes)
				}
				if got := instrumentFailureAuditCount(snap.Audit); got != 2 {
					t.Fatalf("instrument failure audit count = %d, want 2", got)
				}
			},
		},
		{
			name: "instrument call persistence error aborts the whole reading command",
			run: func(t *testing.T) {
				base := store.NewMemoryStore(catalog.NewFixedCatalog())
				defer base.Close()
				svc := NewService(rejectingInstrumentCallStore{
					Store: base,
					err:   errors.New("forced instrument ledger write failure"),
				}, NewManualClock(1000))
				id := createFixtureTask(t, svc)
				confirmSampling(t, svc, id)
				splitBlind(t, svc, id)
				occupyResources(t, svc, id)
				writeColdChain(t, svc, id)

				before, fault := svc.GetSnapshot(ctx, id)
				if fault != nil {
					t.Fatal(fault)
				}
				_, fault = svc.SubmitReading(ctx, id, ReadingRequest{
					OperationID: "op-inst-persist-fails", Generation: 1, Type: evidence.EvidenceAntibiotic,
					BlindCode: "BCODE-A", Well: "A1",
					InstrumentType: "plate-reader", ScriptResult: "timeout", ErrorClass: ErrClassTimeout,
				})
				if fault == nil || fault.Code != CodeStoreError {
					t.Fatalf("fault = %v, want store_error", fault)
				}

				after, fault := svc.GetSnapshot(ctx, id)
				if fault != nil {
					t.Fatal(fault)
				}
				if len(after.InstrumentCalls) != len(before.InstrumentCalls) {
					t.Fatalf("instrument calls changed from %d to %d", len(before.InstrumentCalls), len(after.InstrumentCalls))
				}
				if len(after.Audit) != len(before.Audit) {
					t.Fatalf("audit count changed from %d to %d", len(before.Audit), len(after.Audit))
				}
				if after.Task.Status != inspection.StatusAntibioticReading || len(after.Evidence) != 0 {
					t.Fatalf("partial reading commit: status=%s evidence=%d", after.Task.Status, len(after.Evidence))
				}
				var exists bool
				if err := base.WithTx(ctx, func(tx store.Tx) error {
					_, got, err := tx.GetIdempotency(ctx, id, "op-inst-persist-fails")
					exists = got
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if exists {
					t.Fatal("idempotency record was committed for an unpersisted instrument failure")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, tc.run)
	}
}

func instrumentFailureAuditCount(events []inspection.AuditEvent) int {
	count := 0
	for _, ev := range events {
		if ev.EventType == inspection.EventReading && strings.HasPrefix(ev.Detail, "instrument failure ") {
			count++
		}
	}
	return count
}

type rejectingInstrumentCallStore struct {
	store.Store
	err error
}

func (s rejectingInstrumentCallStore) WithTx(ctx context.Context, fn func(store.Tx) error) error {
	return s.Store.WithTx(ctx, func(tx store.Tx) error {
		return fn(rejectingInstrumentCallTx{Tx: tx, err: s.err})
	})
}

type rejectingInstrumentCallTx struct {
	store.Tx
	err error
}

func (tx rejectingInstrumentCallTx) PutInstrumentCall(context.Context, store.InstrumentCall) error {
	return tx.err
}
