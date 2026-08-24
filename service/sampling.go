package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// SamplingConfirmationRequest is a single operator's confirmation of the farm,
// tank batch, compartments, seals and operation number.
type SamplingConfirmationRequest struct {
	OperationID  inspection.OperationID    `json:"operationId"`
	Person       catalog.PersonID          `json:"person"`
	FarmID       catalog.FarmID            `json:"farmId"`
	TankBatch    inspection.TankBatch      `json:"tankBatch"`
	Compartments []catalog.CompartmentCode `json:"compartments"`
	Seals        []catalog.SealCode        `json:"seals"`
	Generation   inspection.Generation     `json:"generation"`
}

// SamplingConfirmationResult reports whether the dual confirmation is complete.
type SamplingConfirmationResult struct {
	TaskID     inspection.TaskID     `json:"taskId"`
	Generation inspection.Generation `json:"generation"`
	Status     inspection.Status     `json:"status"`
	Confirmed  []catalog.PersonID    `json:"confirmed"`
	Complete   bool                  `json:"complete"`
}

// SamplingConfirm records one operator's confirmation and, when two distinct,
// role-separated, qualified operators confirm identical content, advances the
// task to blind-splitting.
func (s *Service) SamplingConfirm(ctx context.Context, id inspection.TaskID, req SamplingConfirmationRequest) (*SamplingConfirmationResult, *Fault) {
	if req.Person == "" || req.OperationID == "" {
		return nil, NewFault(CodeBadRequest, "person and operationId are required")
	}

	var result *SamplingConfirmationResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if f := guardTask(task, req.Generation); f != nil {
			return f
		}
		now := s.clock.Now()
		rec := inspection.IdempotencyRecord{
			TaskID:        task.ID,
			OperationID:   req.OperationID,
			OperationType: inspection.OpSamplingConfirm,
			RequestDigest: inspection.DigestOf(req),
			LogicalTime:   now,
		}
		// Idempotency is checked before the state-machine guard so that a retry
		// of a command whose first attempt already advanced the task replays
		// the recorded outcome instead of being rejected for the new status.
		// The generation lock above still rejects a retry whose generation was
		// superseded.
		prior, exists, err := tx.GetIdempotency(ctx, task.ID, req.OperationID)
		if err != nil {
			return err
		}
		if exists {
			if prior.ContentConflicts(rec.RequestDigest) {
				return NewFault(CodeContentConflict, "retry with different content")
			}
			result, err = s.buildSamplingResult(ctx, tx, task)
			return err
		}

		if !inspection.Allows(task.Status, inspection.CmdSamplingConfirm) {
			return NewFault(CodeIllegalTransition, string(task.Status))
		}
		if !s.Catalog().Qualifies(task.RuleVersion, req.Person, catalog.RoleSampler) {
			return NewFault(CodeNotQualified, "person "+string(req.Person)+" not a sampler")
		}
		if catalog.HoldsAny(s.Catalog(), task.RuleVersion, req.Person, catalog.RoleReviewer, catalog.RoleRejudger) {
			return NewFault(CodeRoleOverlap, "person "+string(req.Person)+" holds overlapping role")
		}
		if req.FarmID != task.FarmID || req.TankBatch != task.TankBatch {
			return NewFault(CodeContentConflict, "farm or tank batch mismatch")
		}
		if !sameStringSet(req.Compartments, task.Compartments) || !sameStringSet(req.Seals, task.Seals) {
			return NewFault(CodeContentConflict, "compartment or seal mismatch")
		}

		existing, err := tx.ListSamplingConfirmations(ctx, task.ID)
		if err != nil {
			return err
		}
		if len(existing) > 0 {
			first := existing[0]
			if first.FarmID != req.FarmID || first.TankBatch != req.TankBatch ||
				!sameStringSet(first.Compartments, req.Compartments) || !sameStringSet(first.Seals, req.Seals) {
				return NewFault(CodeContentConflict, "confirmation content disagrees with prior operator")
			}
			if first.Person == req.Person {
				return NewFault(CodeRoleOverlap, "same operator already confirmed")
			}
		}

		if err := tx.PutSamplingConfirmation(ctx, store.SamplingConfirmation{
			TaskID: task.ID, Person: req.Person, FarmID: req.FarmID, TankBatch: req.TankBatch,
			Compartments: req.Compartments, Seals: req.Seals, OperationID: req.OperationID, LogicalTime: now,
		}); err != nil {
			if err == store.ErrConflict {
				return NewFault(CodeConflict, "person already confirmed")
			}
			return err
		}
		if err := tx.PutIdempotency(ctx, rec); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID: task.ID, Generation: task.Generation,
			EventType: inspection.EventSampled, Actor: req.Person, LogicalTime: now,
			Detail: "confirmed by " + string(req.Person),
		}); err != nil {
			return err
		}

		confirmations, _ := tx.ListSamplingConfirmations(ctx, task.ID)
		if len(confirmations) >= 2 {
			updated := task
			updated.Status = inspection.StatusBlindSplitting
			if err := tx.UpdateTaskCAS(ctx, task.ID, task.Status, task.Generation, updated); err != nil {
				return err
			}
		}
		result, err = s.buildSamplingResult(ctx, tx, task)
		return err
	})
	if err != nil {
		if f, ok := err.(*Fault); ok {
			return nil, f
		}
		return nil, NewFault(CodeStoreError, err.Error())
	}
	return result, nil
}

func (s *Service) buildSamplingResult(ctx context.Context, tx store.Tx, task inspection.Task) (*SamplingConfirmationResult, error) {
	confirmations, err := tx.ListSamplingConfirmations(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	people := make([]catalog.PersonID, 0, len(confirmations))
	for _, c := range confirmations {
		people = append(people, c.Person)
	}
	current, err := tx.GetTask(ctx, task.ID)
	if err != nil {
		return nil, err
	}
	return &SamplingConfirmationResult{
		TaskID: current.ID, Generation: current.Generation, Status: current.Status,
		Confirmed: people, Complete: len(people) >= 2,
	}, nil
}

func sameStringSet[T ~string](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[T]bool, len(a))
	for _, x := range a {
		set[x] = true
	}
	for _, x := range b {
		if !set[x] {
			return false
		}
	}
	return true
}
