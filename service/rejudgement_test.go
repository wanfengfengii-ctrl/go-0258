package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// TestRejudgementSinglePerGeneration asserts only one rejudgement exists per
// generation and a second is rejected.
func TestRejudgementSinglePerGeneration(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)
	advanceToReview(t, svc, id)

	req := RejudgementRequest{
		OperationID: "op-rej", Generation: 1, Reason: arbiter.ReasonSuspectPositive,
		BlindCodes: []string{"BCODE-A"}, Compartments: []catalog.CompartmentCode{"A"}, Wells: []string{"A1"},
	}
	res, fault := svc.Rejudge(context.Background(), id, req)
	if fault != nil {
		t.Fatal(fault)
	}
	if res.Generation != 2 {
		t.Fatalf("generation = %d, want 2 after rejudgement", res.Generation)
	}
	// A second rejudgement at the OLD generation must be stale.
	req.OperationID = "op-rej-2"
	_, fault = svc.Rejudge(context.Background(), id, req)
	if fault == nil || fault.Code != CodeStaleGeneration {
		t.Fatalf("fault = %v, want stale_generation", fault)
	}
	snap, _ := svc.GetSnapshot(context.Background(), id)
	if len(snap.Rejudgements) != 1 {
		t.Fatalf("rejudgements = %d, want 1", len(snap.Rejudgements))
	}
	if snap.Task.Generation != 2 {
		t.Fatalf("task generation = %d, want 2", snap.Task.Generation)
	}

	// A finalize submitted at the OLD (pre-rejudgement) generation must be
	// rejected as stale, never allowed to race the refreshed generation into
	// a terminal outcome.
	_, fault = svc.Finalize(context.Background(), id, FinalizeRequest{
		OperationID: "op-final-stale", Generation: 1, Outcome: inspection.FinalCancelled,
	})
	if fault == nil || fault.Code != CodeStaleGeneration {
		t.Fatalf("fault = %v, want stale_generation for stale finalize", fault)
	}
	snap, _ = svc.GetSnapshot(context.Background(), id)
	if snap.Task.Status.IsTerminal() {
		t.Fatalf("status = %s, stale finalize must not terminalize", snap.Task.Status)
	}
}

// TestContaminationForcesQuarantine asserts a contamination rejudgement blocks
// admissible finalization even with passing readings and reviews.
func TestContaminationForcesQuarantine(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)
	advanceToReview(t, svc, id)
	passReviews(t, svc, id)

	if _, fault := svc.Rejudge(context.Background(), id, RejudgementRequest{
		OperationID: "op-rej", Generation: 1, Reason: arbiter.ReasonContamination,
		BlindCodes: []string{"BCODE-A", "BCODE-B"}, Wells: []string{"A1", "A2"},
	}); fault != nil {
		t.Fatal(fault)
	}

	// A late reading at the old generation is rejected (stale).
	_, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
		OperationID: "op-late", Generation: 1, Type: "antibiotic", BlindCode: "BCODE-A", Value: "20.0",
	})
	if fault == nil || fault.Code != CodeStaleGeneration {
		t.Fatalf("fault = %v, want stale_generation for late reading", fault)
	}

	_, fault = svc.Finalize(context.Background(), id, FinalizeRequest{
		OperationID: "op-final", Generation: 2, Outcome: inspection.FinalAdmissible,
	})
	if fault == nil || fault.Code != CodeFinalizeConflict {
		t.Fatalf("fault = %v, want finalize_conflict due to contamination", fault)
	}
}

// TestReviewDuplicateRejected asserts a reviewer may not review twice.
func TestReviewDuplicateRejected(t *testing.T) {
	svc, st := newFixtureService(t)
	defer st.Close()
	id := createFixtureTask(t, svc)
	advanceToReview(t, svc, id)

	req := ReviewRequest{OperationID: "op-rev", Generation: 1, Reviewer: "person-reviewer-a", Conclusion: "pass"}
	if _, fault := svc.Review(context.Background(), id, req); fault != nil {
		t.Fatal(fault)
	}
	req.OperationID = "op-rev-2"
	_, fault := svc.Review(context.Background(), id, req)
	if fault == nil || fault.Code != CodeDuplicateReview {
		t.Fatalf("fault = %v, want duplicate_review", fault)
	}
}
