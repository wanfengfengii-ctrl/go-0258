package service

import (
	"context"
	"sync"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// TestFinalizationRaceSingleWinner concurrently competes admissible,
// quarantined and cancelled outcomes and asserts exactly one wins.
func TestFinalizationRaceSingleWinner(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)
	advanceToReview(t, svc, id)
	passReviews(t, svc, id)

	outcomes := []inspection.FinalType{
		inspection.FinalAdmissible, inspection.FinalQuarantined, inspection.FinalCancelled,
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make([]*FinalizeResult, len(outcomes))
	faults := make([]*Fault, len(outcomes))

	for i, oc := range outcomes {
		wg.Add(1)
		go func(i int, oc inspection.FinalType) {
			defer wg.Done()
			<-start
			results[i], faults[i] = svc.Finalize(context.Background(), id, FinalizeRequest{
				OperationID: inspection.OperationID("op-final-" + string(oc)),
				Generation:  1, Outcome: oc,
			})
		}(i, oc)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i := range outcomes {
		if faults[i] == nil {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners = %d, want exactly 1 (faults=%v)", winners, faults)
	}

	snap, fault := svc.GetSnapshot(context.Background(), id)
	if fault != nil {
		t.Fatal(fault)
	}
	if !snap.Task.Status.IsTerminal() {
		t.Fatalf("status = %s, want terminal", snap.Task.Status)
	}
}

// TestTerminalStateRejectsCommands asserts post-finalize commands are refused.
func TestTerminalStateRejectsCommands(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)
	advanceToReview(t, svc, id)
	passReviews(t, svc, id)

	if _, fault := svc.Finalize(context.Background(), id, FinalizeRequest{
		OperationID: "op-final", Generation: 1, Outcome: inspection.FinalAdmissible,
	}); fault != nil {
		t.Fatal(fault)
	}

	// Any further reading/review must be rejected without changing state.
	_, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
		OperationID: "op-late", Generation: 1, Type: "antibiotic", BlindCode: "BCODE-A", Value: "20.0",
	})
	if fault == nil || fault.Code != CodeTerminalState {
		t.Fatalf("fault = %v, want terminal_state", fault)
	}
	_, fault = svc.Review(context.Background(), id, ReviewRequest{
		OperationID: "op-rev-late", Generation: 1, Reviewer: "person-reviewer-a", Conclusion: "pass",
	})
	if fault == nil || fault.Code != CodeTerminalState {
		t.Fatalf("fault = %v, want terminal_state", fault)
	}
}
