package service

import (
	"context"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func TestModel_ReviewRetryReplaysFirstVisibleResult(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name          string
		retry         ReviewRequest
		wantFaultCode string
		wantReplay    bool
	}{
		{
			name: "same operation replays first review count after another reviewer passes",
			retry: ReviewRequest{
				OperationID: "op-review-a",
				Generation:  1,
				Reviewer:    catalog.FixedReviewerA,
				Conclusion:  arbiter.ReviewPass,
			},
			wantReplay: true,
		},
		{
			name: "changed content with same operation conflicts without extra records",
			retry: ReviewRequest{
				OperationID: "op-review-a",
				Generation:  1,
				Reviewer:    catalog.FixedReviewerA,
				Conclusion:  arbiter.ReviewFail,
			},
			wantFaultCode: CodeContentConflict,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, st := newFixtureService(t)
			defer st.Close()
			id := createFixtureTask(t, svc)
			advanceToReview(t, svc, id)

			firstReq := ReviewRequest{
				OperationID: "op-review-a",
				Generation:  1,
				Reviewer:    catalog.FixedReviewerA,
				Conclusion:  arbiter.ReviewPass,
			}
			first, fault := svc.Review(ctx, id, firstReq)
			if fault != nil {
				t.Fatalf("first review: %v", fault)
			}
			if first.ReviewCount != 1 {
				t.Fatalf("first review count = %d, want 1", first.ReviewCount)
			}

			second, fault := svc.Review(ctx, id, ReviewRequest{
				OperationID: "op-review-b",
				Generation:  1,
				Reviewer:    catalog.FixedReviewerB,
				Conclusion:  arbiter.ReviewPass,
			})
			if fault != nil {
				t.Fatalf("second review: %v", fault)
			}
			if second.ReviewCount != 2 {
				t.Fatalf("second review count = %d, want 2", second.ReviewCount)
			}

			before, err := svc.GetSnapshot(ctx, id)
			if err != nil {
				t.Fatalf("snapshot before retry: %v", err)
			}

			got, fault := svc.Review(ctx, id, tc.retry)
			if tc.wantFaultCode != "" {
				if fault == nil || fault.Code != tc.wantFaultCode {
					t.Fatalf("fault = %v, want %s", fault, tc.wantFaultCode)
				}
			} else if fault != nil {
				t.Fatalf("retry review: %v", fault)
			}
			if tc.wantReplay && *got != *first {
				t.Fatalf("retry response = %+v, want original response %+v", got, first)
			}

			after, err := svc.GetSnapshot(ctx, id)
			if err != nil {
				t.Fatalf("snapshot after retry: %v", err)
			}
			if len(after.Reviews) != len(before.Reviews) {
				t.Fatalf("reviews after retry = %d, want %d", len(after.Reviews), len(before.Reviews))
			}
			if len(after.Audit) != len(before.Audit) {
				t.Fatalf("audit events after retry = %d, want %d", len(after.Audit), len(before.Audit))
			}
			for i, ev := range after.Audit {
				if ev.EventType == inspection.EventReviewed && ev != before.Audit[i] {
					t.Fatalf("review audit event %d changed after retry: %+v vs %+v", i, ev, before.Audit[i])
				}
			}
		})
	}
}
