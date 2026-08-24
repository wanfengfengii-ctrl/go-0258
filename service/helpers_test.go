package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// advanceToReview runs the full happy path through the reading phases, leaving
// the task at pending_review (ready for reviews and finalization).
func advanceToReview(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	confirmSampling(t, svc, id)
	splitBlind(t, svc, id)
	occupyResources(t, svc, id)
	writeColdChain(t, svc, id)

	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		mustRead(t, svc, id, "op-anti-"+strconvItoa(i), evidence.EvidenceAntibiotic, code, "20.0")
	}
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		mustRead(t, svc, id, "op-som-"+strconvItoa(i), evidence.EvidenceSomaticCell, code, "350")
		mustRead(t, svc, id, "op-col-"+strconvItoa(i), evidence.EvidenceColony, code, "50000")
	}
	for i, code := range []string{"BCODE-A", "BCODE-B"} {
		mustRead(t, svc, id, "op-fp-"+strconvItoa(i), evidence.EvidenceFreezingPoint, code, "-53.0")
		mustRead(t, svc, id, "op-fat-"+strconvItoa(i), evidence.EvidenceFat, code, "3.5")
		mustRead(t, svc, id, "op-prot-"+strconvItoa(i), evidence.EvidenceProtein, code, "3.1")
	}
}

func mustRead(t *testing.T, svc *Service, id inspection.TaskID, op string, typ evidence.EvidenceType, code, value string) {
	t.Helper()
	_, fault := svc.SubmitReading(context.Background(), id, ReadingRequest{
		OperationID: inspection.OperationID(op), Generation: 1, Type: typ,
		BlindCode: code, Value: value,
	})
	if fault != nil {
		t.Fatalf("reading %s: %v", op, fault)
	}
}

func passReviews(t *testing.T, svc *Service, id inspection.TaskID) {
	t.Helper()
	for _, reviewer := range []catalog.PersonID{catalog.FixedReviewerA, catalog.FixedReviewerB} {
		_, fault := svc.Review(context.Background(), id, ReviewRequest{
			OperationID: inspection.OperationID("op-rev-" + string(reviewer)),
			Generation:  1, Reviewer: reviewer, Conclusion: "pass",
		})
		if fault != nil {
			t.Fatalf("review: %v", fault)
		}
	}
}
