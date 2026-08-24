package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// TestInstrumentFailureRecordsRetry asserts a scripted instrument failure
// appends an auditable call with a deterministic retry plan and never forges
// a pass or advances the phase.
func TestInstrumentFailureRecordsRetry(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)
	confirmSampling(t, svc, id)
	splitBlind(t, svc, id)
	occupyResources(t, svc, id)
	writeColdChain(t, svc, id)

	_, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
		OperationID: "op-inst", Generation: 1, Type: evidence.EvidenceAntibiotic,
		BlindCode: "BCODE-A", Well: "A1",
		InstrumentType: "plate-reader", ScriptResult: "timeout", ErrorClass: ErrClassTimeout,
	})
	if fault != nil {
		t.Fatal(fault)
	}

	snap, fault := svc.GetSnapshot(context.Background(), id)
	if fault != nil {
		t.Fatal(fault)
	}
	if len(snap.InstrumentCalls) != 1 {
		t.Fatalf("instrument calls = %d, want 1", len(snap.InstrumentCalls))
	}
	call := snap.InstrumentCalls[0]
	if call.ErrorClass != ErrClassTimeout {
		t.Fatalf("error class = %s, want timeout", call.ErrorClass)
	}
	if call.RetryCount != 0 || call.NextRetryAt == 0 {
		t.Fatalf("retry plan not set: %+v", call)
	}
	// No evidence was written and status did not advance.
	if len(snap.Evidence) != 0 {
		t.Fatalf("evidence = %d, want 0 (failure must not forge evidence)", len(snap.Evidence))
	}
	if snap.Task.Status != inspection.StatusAntibioticReading {
		t.Fatalf("status = %s, want antibiotic_reading", snap.Task.Status)
	}
}

// TestRetryPlannerBackoff asserts the deterministic exponential backoff.
func TestRetryPlannerBackoff(t *testing.T) {
	p := NewRetryPlanner(1, 3600)
	prev := p.Next(0)
	for i := 1; i < 12; i++ {
		next := p.Next(i)
		if next < prev || next > 3600 {
			t.Fatalf("backoff not monotonic/bounded: %d -> %d", prev, next)
		}
		prev = next
	}
	if p.Next(20) != 3600 {
		t.Fatalf("backoff should cap at 3600, got %d", p.Next(20))
	}
}

// TestInstrumentCallRoundTrip exercises the store persistence of a call.
func TestInstrumentCallRoundTrip(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)

	_ = st.WithTx(context.Background(), func(tx store.Tx) error {
		return tx.PutInstrumentCall(context.Background(), store.InstrumentCall{
			CallID: "call-1", TaskID: id, InstrumentType: "counter",
			Target: "BCODE-A", ScriptResult: "rejected", RetryCount: 1,
			NextRetryAt: 42, ErrorClass: ErrClassRejected,
		})
	})

	snap, fault := svc.GetSnapshot(context.Background(), id)
	if fault != nil {
		t.Fatal(fault)
	}
	if len(snap.InstrumentCalls) != 1 || snap.InstrumentCalls[0].CallID != "call-1" {
		t.Fatalf("instrument calls not persisted: %+v", snap.InstrumentCalls)
	}
}
