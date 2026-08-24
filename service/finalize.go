package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// FinalizeRequest competes for the unique terminal outcome.
type FinalizeRequest struct {
	OperationID inspection.OperationID `json:"operationId"`
	Generation  inspection.Generation  `json:"generation"`
	Outcome     inspection.FinalType   `json:"outcome"`
}

// FinalizeResult is the winning terminal credential.
type FinalizeResult struct {
	TaskID     inspection.TaskID     `json:"taskId"`
	Generation inspection.Generation `json:"generation"`
	FinalType  inspection.FinalType  `json:"finalType"`
	Credential string                `json:"credential"`
}

// Finalize competes to produce the unique terminal outcome. Admissible
// requires every gate and two distinct qualified reviewers to pass; entered
// advances an already-admissible task; quarantined and cancelled are manual
// overrides. Only one competitor wins.
func (s *Service) Finalize(ctx context.Context, id inspection.TaskID, req FinalizeRequest) (*FinalizeResult, *Fault) {
	switch req.Outcome {
	case inspection.FinalAdmissible, inspection.FinalEntered, inspection.FinalQuarantined, inspection.FinalCancelled:
	default:
		return nil, NewFault(CodeBadRequest, "outcome must be admissible, entered, quarantined or cancelled")
	}

	var result *FinalizeResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if req.Generation != task.Generation {
			return NewFault(CodeStaleGeneration, "presented "+strconvItoa(int(req.Generation))+" current "+strconvItoa(int(task.Generation)))
		}

		// Entered: only from admissible (already terminal).
		if req.Outcome == inspection.FinalEntered {
			if task.Status != inspection.StatusAdmissible {
				return NewFault(CodeFinalizeConflict, "entered requires admissible")
			}
			var e error
			result, e = s.commitFinal(ctx, tx, task, req)
			return e
		}

		if task.FinalType != "" {
			return NewFault(CodeTerminalState, string(task.FinalType))
		}
		if !inspection.Allows(task.Status, inspection.CmdFinalize) {
			return NewFault(CodeIllegalTransition, string(task.Status))
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
			fd, ok, _ := tx.GetFinalDecision(ctx, task.ID)
			if ok {
				result = &FinalizeResult{TaskID: task.ID, Generation: task.Generation, FinalType: fd.FinalType, Credential: fd.Credential}
			}
			return nil
		}

		input, err := s.buildDecisionInput(ctx, tx, task)
		if err != nil {
			return err
		}
		conclusion := arbiter.Evaluate(input)

		switch req.Outcome {
		case inspection.FinalAdmissible:
			if conclusion.FinalType != "admissible" {
				return NewFault(CodeFinalizeConflict, conclusion.Reasons...)
			}
		case inspection.FinalQuarantined, inspection.FinalCancelled:
			// Manual quarantine/cancel always allowed before a terminal outcome.
		}

		var e error
		result, e = s.commitFinal(ctx, tx, task, req)
		return e
	})
	if err != nil {
		if f, ok := err.(*Fault); ok {
			return nil, f
		}
		return nil, NewFault(CodeStoreError, err.Error())
	}
	return result, nil
}

// commitFinal performs the compare-and-set terminal transition and writes the
// unique credential.
func (s *Service) commitFinal(ctx context.Context, tx store.Tx, task inspection.Task, req FinalizeRequest) (*FinalizeResult, error) {
	targetStatus := statusForFinal(req.Outcome)
	updated := task
	updated.Status = targetStatus
	updated.FinalType = req.Outcome

	wantStatus := task.Status
	if req.Outcome == inspection.FinalEntered {
		wantStatus = inspection.StatusAdmissible
	}
	if err := tx.UpdateTaskCAS(ctx, task.ID, wantStatus, task.Generation, updated); err != nil {
		if err == store.ErrConflict {
			return nil, NewFault(CodeFinalizeConflict, "another finalization won")
		}
		return nil, err
	}

	// A terminal task is no longer an active occupant of its plate wells and
	// incubator slots, so its leases are released now: the resources become
	// available for a later task that reuses the same well and time interval.
	// ReleaseOccupancy is idempotent (it updates only released_at=0 rows), so
	// re-finalizing an already-terminal task (e.g. entered after admissible)
	// is a safe no-op.
	credential := NewID("cred")
	now := s.clock.Now()
	if err := tx.ReleaseOccupancy(ctx, task.ID, now); err != nil {
		return nil, err
	}
	if err := tx.PutFinalDecision(ctx, task.ID, req.Outcome, credential, now); err != nil {
		return nil, err
	}
	if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
		TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpFinalize,
		RequestDigest: inspection.DigestOf(req), LogicalTime: now,
	}); err != nil {
		return nil, err
	}
	if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
		TaskID: task.ID, Generation: task.Generation,
		EventType: inspection.EventFinalized, LogicalTime: now,
		Detail: string(req.Outcome),
	}); err != nil {
		return nil, err
	}
	if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
		TaskID: task.ID, Generation: task.Generation,
		EventType: inspection.EventReleased, LogicalTime: now,
		Detail: string(req.Outcome),
	}); err != nil {
		return nil, err
	}
	return &FinalizeResult{TaskID: task.ID, Generation: task.Generation, FinalType: req.Outcome, Credential: credential}, nil
}

func statusForFinal(t inspection.FinalType) inspection.Status {
	switch t {
	case inspection.FinalAdmissible:
		return inspection.StatusAdmissible
	case inspection.FinalEntered:
		return inspection.StatusEntered
	case inspection.FinalQuarantined:
		return inspection.StatusQuarantined
	default:
		return inspection.StatusCancelled
	}
}

// buildDecisionInput reconstructs the decision facts from persisted state.
func (s *Service) buildDecisionInput(ctx context.Context, tx store.Tx, task inspection.Task) (arbiter.DecisionInput, error) {
	rules, ok := s.Catalog().Rules(task.RuleVersion)
	if !ok {
		return arbiter.DecisionInput{}, NewFault(CodeUnknownRules, task.RuleVersion)
	}
	calc := evidence.NewDerivedCalculator(rules)

	cells, err := tx.ListTemperature(ctx, task.ID)
	if err != nil {
		return arbiter.DecisionInput{}, err
	}
	grid := evidence.NewGrid(rules.Temperature)
	baseTime := int64(0)
	if len(cells) > 0 {
		baseTime = cells[0].AtSeconds
	}
	computed := grid.Compute(baseTime, "", cells)
	input := arbiter.DecisionInput{
		ColdChainComplete: computed.Complete,
		ColdChainOver:     computed.ConsecutiveOver > rules.Temperature.MaxConsecutiveOverSeconds,
		RequiredReviewers: rules.RequiredReviewers,
	}

	records, err := tx.ListEvidence(ctx, task.ID)
	if err != nil {
		return arbiter.DecisionInput{}, err
	}
	input.AntibioticPass = true
	input.MicrobialPass = true
	input.PhysicoPass = true
	for _, r := range records {
		switch r.Type {
		case evidence.EvidenceAntibiotic:
			if !calc.AntibioticPass(r.Raw).Pass {
				input.AntibioticPass = false
			}
		case evidence.EvidenceSomaticCell:
			if !calc.SomaticCellPass(r.Raw).Pass {
				input.MicrobialPass = false
			}
		case evidence.EvidenceColony:
			if !calc.ColonyPass(r.Raw).Pass {
				input.MicrobialPass = false
			}
		case evidence.EvidenceFreezingPoint:
			if !calc.FreezingPointPass(r.Raw).Pass {
				input.PhysicoPass = false
			}
		case evidence.EvidenceFat:
			if !calc.FatPass(r.Raw).Pass {
				input.PhysicoPass = false
			}
		case evidence.EvidenceProtein:
			if !calc.ProteinPass(r.Raw).Pass {
				input.PhysicoPass = false
			}
		}
	}

	rejudgements, err := tx.ListRejudgements(ctx, task.ID)
	if err != nil {
		return arbiter.DecisionInput{}, err
	}
	for _, rj := range rejudgements {
		switch rj.Reason {
		case arbiter.ReasonContamination:
			input.Contaminated = true
		case arbiter.ReasonSplitDisagreement:
			input.SplitDisagreement = true
		}
	}

	reviews, err := tx.ListReviews(ctx, task.ID)
	if err != nil {
		return arbiter.DecisionInput{}, err
	}
	input.Reviews = reviews
	return input, nil
}
