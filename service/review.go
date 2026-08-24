package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// ReviewRequest records one independent qualified reviewer's conclusion.
type ReviewRequest struct {
	OperationID inspection.OperationID   `json:"operationId"`
	Generation  inspection.Generation    `json:"generation"`
	Reviewer    catalog.PersonID         `json:"reviewer"`
	Conclusion  arbiter.ReviewConclusion `json:"conclusion"`
}

// ReviewResult reports the review and the current review count.
type ReviewResult struct {
	TaskID      inspection.TaskID        `json:"taskId"`
	Generation  inspection.Generation    `json:"generation"`
	Reviewer    catalog.PersonID         `json:"reviewer"`
	Conclusion  arbiter.ReviewConclusion `json:"conclusion"`
	ReviewCount int                      `json:"reviewCount"`
}

// Review records an independent review, enforcing distinct qualified
// reviewers.
func (s *Service) Review(ctx context.Context, id inspection.TaskID, req ReviewRequest) (*ReviewResult, *Fault) {
	if req.Reviewer == "" || req.Conclusion == "" {
		return nil, NewFault(CodeBadRequest, "reviewer and conclusion are required")
	}
	if req.Conclusion != arbiter.ReviewPass && req.Conclusion != arbiter.ReviewFail {
		return nil, NewFault(CodeBadRequest, "conclusion must be pass or fail")
	}

	var result *ReviewResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if f := guardTask(task, req.Generation); f != nil {
			return f
		}
		if !inspection.Allows(task.Status, inspection.CmdReview) {
			return NewFault(CodeIllegalTransition, string(task.Status))
		}
		if !s.Catalog().Qualifies(task.RuleVersion, req.Reviewer, catalog.RoleReviewer) {
			return NewFault(CodeNotQualified, "reviewer "+string(req.Reviewer)+" not qualified")
		}
		if catalog.HoldsAny(s.Catalog(), task.RuleVersion, req.Reviewer, catalog.RoleSampler) {
			return NewFault(CodeRoleOverlap, "reviewer "+string(req.Reviewer)+" already sampled")
		}

		prior, exists, err := tx.GetIdempotency(ctx, task.ID, req.OperationID)
		if err != nil {
			return err
		}
		digest := inspection.DigestOf(req)
		if exists {
			if prior.ContentConflicts(digest) {
				return NewFault(CodeContentConflict, "retry with different content")
			}
			reviews, _ := tx.ListReviews(ctx, task.ID)
			result = &ReviewResult{TaskID: task.ID, Generation: task.Generation, Reviewer: req.Reviewer, Conclusion: req.Conclusion, ReviewCount: len(reviews)}
			return nil
		}

		r := arbiter.Review{TaskID: string(task.ID), Reviewer: req.Reviewer, Conclusion: req.Conclusion, Generation: int64(task.Generation)}
		if err := tx.PutReview(ctx, r); err != nil {
			if err == store.ErrConflict {
				return NewFault(CodeDuplicateReview, "reviewer "+string(req.Reviewer)+" already reviewed")
			}
			return err
		}
		now := s.clock.Now()
		if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
			TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpReview,
			RequestDigest: digest, LogicalTime: now,
		}); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID: task.ID, Generation: task.Generation,
			EventType: inspection.EventReviewed, Actor: req.Reviewer, LogicalTime: now,
			Detail: string(req.Conclusion),
		}); err != nil {
			return err
		}
		reviews, _ := tx.ListReviews(ctx, task.ID)
		result = &ReviewResult{TaskID: task.ID, Generation: task.Generation, Reviewer: req.Reviewer, Conclusion: req.Conclusion, ReviewCount: len(reviews)}
		return nil
	})
	if err != nil {
		if f, ok := err.(*Fault); ok {
			return nil, f
		}
		return nil, NewFault(CodeStoreError, err.Error())
	}
	return result, nil
}
