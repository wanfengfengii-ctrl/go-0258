package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func TestModel_RejudgementPendingReviewDoesNotRegressStatus(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		reason arbiter.RejudgementReason
	}{
		{name: "cold_chain_break_remains_finalizeable_review_stage", reason: arbiter.ReasonColdChainBreak},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()
			id := createFixtureTask(t, svc)
			advanceToReview(t, svc, id)

			req := RejudgementRequest{
				OperationID:  inspection.OperationID("op-rejudge-" + string(tc.reason)),
				Generation:   1,
				Reason:       tc.reason,
				BlindCodes:   []string{"BCODE-A", "BCODE-B"},
				Compartments: []catalog.CompartmentCode{"A", "B"},
				Wells:        []string{"A1", "A2"},
			}
			res, fault := svc.Rejudge(ctx, id, req)
			if fault != nil {
				t.Fatalf("rejudge: %v", fault)
			}
			if res.Generation != 2 {
				t.Fatalf("rejudge generation = %d, want 2", res.Generation)
			}

			snap, fault := svc.GetSnapshot(ctx, id)
			if fault != nil {
				t.Fatalf("snapshot: %v", fault)
			}
			if snap.Task.Generation != 2 {
				t.Fatalf("task generation = %d, want 2", snap.Task.Generation)
			}
			if snap.Task.Status != inspection.StatusPendingReview {
				t.Fatalf("status after rejudge = %s, want %s", snap.Task.Status, inspection.StatusPendingReview)
			}
			if len(snap.Rejudgements) != 1 {
				t.Fatalf("rejudgements = %d, want 1", len(snap.Rejudgements))
			}
			if snap.Rejudgements[0].Generation != 1 || snap.Rejudgements[0].Reason != tc.reason {
				t.Fatalf("rejudgement = %+v, want generation 1 reason %s", snap.Rejudgements[0], tc.reason)
			}
			foundAudit := false
			for _, ev := range snap.Audit {
				if ev.EventType == inspection.EventRejudged && ev.Generation == 1 && ev.Detail == string(tc.reason) {
					foundAudit = true
					break
				}
			}
			if !foundAudit {
				t.Fatalf("missing rejudgement audit event for reason %s", tc.reason)
			}

			req.OperationID = inspection.OperationID("op-rejudge-duplicate-" + string(tc.reason))
			_, fault = svc.Rejudge(ctx, id, req)
			if fault == nil || fault.Code != CodeStaleGeneration {
				t.Fatalf("duplicate old-generation rejudge fault = %v, want %s", fault, CodeStaleGeneration)
			}
			_, fault = svc.SubmitReading(ctx, id, ReadingRequest{
				OperationID: inspection.OperationID("op-stale-reading-" + string(tc.reason)),
				Generation:  1,
				Type:        evidence.EvidenceAntibiotic,
				BlindCode:   "BCODE-A",
				Value:       "20.0",
			})
			if fault == nil || fault.Code != CodeStaleGeneration {
				t.Fatalf("old-generation reading fault = %v, want %s", fault, CodeStaleGeneration)
			}

			_, fault = svc.Review(ctx, id, ReviewRequest{
				OperationID: inspection.OperationID("op-review-after-rejudge-" + string(tc.reason)),
				Generation:  2,
				Reviewer:    catalog.FixedReviewerA,
				Conclusion:  arbiter.ReviewPass,
			})
			if fault != nil {
				t.Fatalf("review at advanced generation: %v", fault)
			}
			final, fault := svc.Finalize(ctx, id, FinalizeRequest{
				OperationID: inspection.OperationID("op-final-after-rejudge-" + string(tc.reason)),
				Generation:  2,
				Outcome:     inspection.FinalQuarantined,
			})
			if fault != nil {
				t.Fatalf("finalize quarantine at advanced generation: %v", fault)
			}
			if final.FinalType != inspection.FinalQuarantined {
				t.Fatalf("final type = %s, want %s", final.FinalType, inspection.FinalQuarantined)
			}
		})
	}
}
