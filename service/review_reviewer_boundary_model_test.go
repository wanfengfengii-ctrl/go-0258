package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

func TestModel_ReviewConclusionCannotBeResubmittedBySameReviewer(t *testing.T) {
	tests := []struct {
		name            string
		first           ReviewRequest
		second          ReviewRequest
		wantFault       string
		wantReviewCount int
		wantReviews     map[catalog.PersonID]arbiter.ReviewConclusion
	}{
		{
			name: "same reviewer cannot turn fail into pass with new operation",
			first: ReviewRequest{
				OperationID: "op-review-a-fail", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewFail,
			},
			second: ReviewRequest{
				OperationID: "op-review-a-pass", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewPass,
			},
			wantFault:       CodeDuplicateReview,
			wantReviewCount: 1,
			wantReviews: map[catalog.PersonID]arbiter.ReviewConclusion{
				catalog.FixedReviewerA: arbiter.ReviewFail,
			},
		},
		{
			name: "same reviewer cannot repeat same conclusion with new operation",
			first: ReviewRequest{
				OperationID: "op-review-a-pass-1", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewPass,
			},
			second: ReviewRequest{
				OperationID: "op-review-a-pass-2", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewPass,
			},
			wantFault:       CodeDuplicateReview,
			wantReviewCount: 1,
			wantReviews: map[catalog.PersonID]arbiter.ReviewConclusion{
				catalog.FixedReviewerA: arbiter.ReviewPass,
			},
		},
		{
			name: "same operation retry remains idempotent",
			first: ReviewRequest{
				OperationID: "op-review-a-pass", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewPass,
			},
			second: ReviewRequest{
				OperationID: "op-review-a-pass", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewPass,
			},
			wantReviewCount: 1,
			wantReviews: map[catalog.PersonID]arbiter.ReviewConclusion{
				catalog.FixedReviewerA: arbiter.ReviewPass,
			},
		},
		{
			name: "same operation conflicting content remains content conflict",
			first: ReviewRequest{
				OperationID: "op-review-a-content", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewFail,
			},
			second: ReviewRequest{
				OperationID: "op-review-a-content", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewPass,
			},
			wantFault:       CodeContentConflict,
			wantReviewCount: 1,
			wantReviews: map[catalog.PersonID]arbiter.ReviewConclusion{
				catalog.FixedReviewerA: arbiter.ReviewFail,
			},
		},
		{
			name: "different qualified reviewer can submit normally",
			first: ReviewRequest{
				OperationID: "op-review-a-pass", Generation: 1,
				Reviewer: catalog.FixedReviewerA, Conclusion: arbiter.ReviewPass,
			},
			second: ReviewRequest{
				OperationID: "op-review-b-pass", Generation: 1,
				Reviewer: catalog.FixedReviewerB, Conclusion: arbiter.ReviewPass,
			},
			wantReviewCount: 2,
			wantReviews: map[catalog.PersonID]arbiter.ReviewConclusion{
				catalog.FixedReviewerA: arbiter.ReviewPass,
				catalog.FixedReviewerB: arbiter.ReviewPass,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc, st := newFixtureService(t)
			defer st.Close()
			id := createFixtureTask(t, svc)
			advanceToReview(t, svc, id)

			if _, fault := svc.Review(ctx, id, tt.first); fault != nil {
				t.Fatalf("first review: %v", fault)
			}

			result, fault := svc.Review(ctx, id, tt.second)
			if tt.wantFault == "" {
				if fault != nil {
					t.Fatalf("second review fault = %v, want nil", fault)
				}
				if result == nil {
					t.Fatal("second review result = nil, want result")
				}
				if result.ReviewCount != tt.wantReviewCount {
					t.Fatalf("review count = %d, want %d", result.ReviewCount, tt.wantReviewCount)
				}
			} else {
				if fault == nil || fault.Code != tt.wantFault {
					t.Fatalf("second review fault = %v, want %s", fault, tt.wantFault)
				}
			}

			report, reportFault := svc.BuildReport(ctx, id)
			if reportFault != nil {
				t.Fatalf("report: %v", reportFault)
			}
			if len(report.Reviews) != tt.wantReviewCount {
				t.Fatalf("report reviews = %d, want %d: %+v", len(report.Reviews), tt.wantReviewCount, report.Reviews)
			}

			gotReviews := make(map[catalog.PersonID]arbiter.ReviewConclusion)
			for _, review := range report.Reviews {
				if _, exists := gotReviews[review.Reviewer]; exists {
					t.Fatalf("report contains reviewer %s more than once: %+v", review.Reviewer, report.Reviews)
				}
				gotReviews[review.Reviewer] = review.Conclusion
			}
			if !reflect.DeepEqual(gotReviews, tt.wantReviews) {
				t.Fatalf("report reviews = %#v, want %#v", gotReviews, tt.wantReviews)
			}
		})
	}
}
