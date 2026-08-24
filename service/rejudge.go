package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// RejudgementRequest creates a single generation-stamped rejudgement covering
// the affected blind codes, compartments and wells.
type RejudgementRequest struct {
	OperationID  inspection.OperationID    `json:"operationId"`
	Generation   inspection.Generation     `json:"generation"`
	Reason       arbiter.RejudgementReason `json:"reason"`
	BlindCodes   []string                  `json:"blindCodes"`
	Compartments []catalog.CompartmentCode `json:"compartments"`
	Wells        []string                  `json:"wells"`
}

// RejudgementResult is the created rejudgement and the advanced generation.
type RejudgementResult struct {
	TaskID     inspection.TaskID         `json:"taskId"`
	Generation inspection.Generation     `json:"generation"`
	Reason     arbiter.RejudgementReason `json:"reason"`
}

// Rejudge records one rejudgement for the current generation and advances the
// task generation so late readings at the old generation are rejected.
func (s *Service) Rejudge(ctx context.Context, id inspection.TaskID, req RejudgementRequest) (*RejudgementResult, *Fault) {
	if req.Reason == "" {
		return nil, NewFault(CodeBadRequest, "reason is required")
	}

	var result *RejudgementResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if f := guardTask(task, req.Generation); f != nil {
			return f
		}
		if !inspection.Allows(task.Status, inspection.CmdRejudge) {
			return NewFault(CodeIllegalTransition, string(task.Status))
		}
		if _, exists, err := tx.GetRejudgement(ctx, task.ID, int64(task.Generation)); err != nil {
			return err
		} else if exists {
			return NewFault(CodeRejudgementExists, "generation "+strconvItoa(int(task.Generation)))
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
			result = &RejudgementResult{TaskID: task.ID, Generation: task.Generation, Reason: req.Reason}
			return nil
		}

		r := arbiter.Rejudgement{
			TaskID: string(task.ID), Generation: int64(task.Generation), Reason: req.Reason,
			BlindCodes: req.BlindCodes, Compartments: req.Compartments, Wells: req.Wells,
		}
		if err := tx.PutRejudgement(ctx, r); err != nil {
			if err == store.ErrConflict {
				return NewFault(CodeRejudgementExists, "generation "+strconvItoa(int(task.Generation)))
			}
			return err
		}
		now := s.clock.Now()
		if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
			TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpRejudge,
			RequestDigest: digest, LogicalTime: now,
		}); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID: task.ID, Generation: task.Generation,
			EventType: inspection.EventRejudged, LogicalTime: now, Detail: string(req.Reason),
		}); err != nil {
			return err
		}

		updated := task
		if req.Reason == arbiter.ReasonColdChainBreak {
			updated.Status = inspection.StatusColdChainVerifying
		}
		updated.Generation = task.Generation + 1
		if err := tx.UpdateTaskCAS(ctx, task.ID, task.Status, task.Generation, updated); err != nil {
			return err
		}
		result = &RejudgementResult{TaskID: task.ID, Generation: updated.Generation, Reason: req.Reason}
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
