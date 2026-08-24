package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// ReadingRequest submits one antibiotic, somatic-cell, colony, freezing-point,
// fat or protein reading, or an instrument-call receipt on failure.
type ReadingRequest struct {
	OperationID inspection.OperationID  `json:"operationId"`
	Generation  inspection.Generation   `json:"generation"`
	Type        evidence.EvidenceType   `json:"type"`
	BlindCode   string                  `json:"blindCode"`
	Compartment catalog.CompartmentCode `json:"compartment"`
	Well        string                  `json:"well"`
	Value       string                  `json:"value"`
	// Instrument failure fields; when set, the reading is a failed call.
	InstrumentType string `json:"instrumentType,omitempty"`
	ScriptResult   string `json:"scriptResult,omitempty"`
	ErrorClass     string `json:"errorClass,omitempty"`
}

// ReadingResult reports the pass/fail and whether the phase advanced.
type ReadingResult struct {
	TaskID     inspection.TaskID     `json:"taskId"`
	Generation inspection.Generation `json:"generation"`
	Status     inspection.Status     `json:"status"`
	Pass       bool                  `json:"pass"`
	Type       evidence.EvidenceType `json:"type"`
	Instrument *store.InstrumentCall `json:"instrument,omitempty"`
}

// SubmitReading validates and persists a reading, or records an instrument
// failure, then advances the phase when its required readings are complete.
func (s *Service) SubmitReading(ctx context.Context, id inspection.TaskID, req ReadingRequest) (*ReadingResult, *Fault) {
	if req.Type == "" {
		return nil, NewFault(CodeBadRequest, "type is required")
	}

	var result *ReadingResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if f := guardTask(task, req.Generation); f != nil {
			return f
		}
		phase := phaseForType(req.Type)
		if phase != task.Status {
			return NewFault(CodeIllegalTransition, "reading type "+string(req.Type)+" not allowed at "+string(task.Status))
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
			current, _ := tx.GetTask(ctx, task.ID)
			result = &ReadingResult{TaskID: task.ID, Generation: current.Generation, Status: current.Status, Type: req.Type}
			return nil
		}

		if req.InstrumentType != "" || req.ErrorClass != "" {
			call := s.recordInstrumentFailure(ctx, tx, task, req)
			status := task.Status
			generation := task.Generation
			if call.ErrorClass == ErrClassRejected {
				updated := task
				updated.Status = nextStatus(task.Status)
				if err := tx.UpdateTaskCAS(ctx, task.ID, task.Status, task.Generation, updated); err != nil {
					return err
				}
				status = updated.Status
				generation = updated.Generation
			}
			if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
				TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpReading,
				RequestDigest: digest, LogicalTime: s.clock.Now(), ErrorCode: CodeInstrumentFailure,
			}); err != nil {
				return err
			}
			result = &ReadingResult{TaskID: task.ID, Generation: generation, Status: status, Type: req.Type, Instrument: &call}
			return nil
		}

		rules, ok := s.Catalog().Rules(task.RuleVersion)
		if !ok {
			return NewFault(CodeUnknownRules, task.RuleVersion)
		}
		calc := evidence.NewDerivedCalculator(rules)
		scale := scaleForType(req.Type, rules)

		fp, err := evidence.ParseFixedPoint(req.Value, scale)
		if err != nil {
			return NewFault(CodeArithmeticFailure, err.Error())
		}
		pass := calcPass(calc, req.Type, fp)
		if pass.Err != nil {
			return NewFault(CodeArithmeticFailure, pass.Err.Error())
		}

		rec := evidence.EvidenceRecord{
			TaskID: string(task.ID), BlindCode: req.BlindCode, Compartment: req.Compartment,
			Well: req.Well, Type: req.Type, Raw: fp, RuleVersion: task.RuleVersion,
			Generation: int64(task.Generation), Immutable: true,
		}
		if pass.Derived != nil {
			rec.Derived = pass.Derived
		}
		if err := tx.PutEvidence(ctx, rec); err != nil {
			return err
		}

		now := s.clock.Now()
		if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
			TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpReading,
			RequestDigest: digest, LogicalTime: now,
		}); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID: task.ID, Generation: task.Generation,
			EventType: inspection.EventReading, LogicalTime: now,
			Detail: string(req.Type) + "=" + fp.String(),
		}); err != nil {
			return err
		}

		updated := task
		if s.phaseComplete(ctx, tx, task) {
			updated.Status = nextStatus(task.Status)
		}
		if updated.Status != task.Status {
			if err := tx.UpdateTaskCAS(ctx, task.ID, task.Status, task.Generation, updated); err != nil {
				return err
			}
		}
		result = &ReadingResult{TaskID: task.ID, Generation: updated.Generation, Status: updated.Status, Type: req.Type, Pass: pass.Pass}
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

func (s *Service) recordInstrumentFailure(ctx context.Context, tx store.Tx, task inspection.Task, req ReadingRequest) store.InstrumentCall {
	planner := NewRetryPlanner(1, 3600)
	call := planner.Plan(
		NewID("call"), string(task.ID), req.InstrumentType, req.BlindCode,
		req.ScriptResult, classifyError(req), s.clock.Now(),
	)
	_ = tx.PutInstrumentCall(ctx, call)
	if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
		TaskID: task.ID, Generation: task.Generation,
		EventType: inspection.EventReading, LogicalTime: s.clock.Now(),
		Detail: "instrument failure " + call.ErrorClass,
	}); err != nil {
		return call
	}
	return call
}

func classifyError(req ReadingRequest) string {
	if req.ErrorClass != "" {
		return req.ErrorClass
	}
	if req.ScriptResult == "timeout" {
		return ErrClassTimeout
	}
	return ErrClassRejected
}

func (s *Service) phaseComplete(ctx context.Context, tx store.Tx, task inspection.Task) bool {
	blinds, _ := tx.ListBlind(ctx, task.ID)
	if len(blinds) == 0 {
		return false
	}
	records, _ := tx.ListEvidence(ctx, task.ID)
	required := typesForPhase(task.Status)
	count := 0
	for _, r := range records {
		for _, t := range required {
			if r.Type == t {
				count++
			}
		}
	}
	return count >= len(blinds)*len(required)
}

func phaseForType(t evidence.EvidenceType) inspection.Status {
	switch t {
	case evidence.EvidenceAntibiotic:
		return inspection.StatusAntibioticReading
	case evidence.EvidenceSomaticCell, evidence.EvidenceColony:
		return inspection.StatusMicrobialCulturing
	default:
		return inspection.StatusPhysicochemical
	}
}

func typesForPhase(status inspection.Status) []evidence.EvidenceType {
	switch status {
	case inspection.StatusAntibioticReading:
		return []evidence.EvidenceType{evidence.EvidenceAntibiotic}
	case inspection.StatusMicrobialCulturing:
		return []evidence.EvidenceType{evidence.EvidenceSomaticCell, evidence.EvidenceColony}
	case inspection.StatusPhysicochemical:
		return []evidence.EvidenceType{evidence.EvidenceFreezingPoint, evidence.EvidenceFat, evidence.EvidenceProtein}
	default:
		return nil
	}
}

func nextStatus(status inspection.Status) inspection.Status {
	switch status {
	case inspection.StatusAntibioticReading:
		return inspection.StatusMicrobialCulturing
	case inspection.StatusMicrobialCulturing:
		return inspection.StatusPhysicochemical
	case inspection.StatusPhysicochemical:
		return inspection.StatusPendingReview
	default:
		return status
	}
}

func scaleForType(t evidence.EvidenceType, rules *catalog.RawMilkRules) int {
	switch t {
	case evidence.EvidenceAntibiotic:
		return rules.Antibiotic.Scale
	case evidence.EvidenceSomaticCell:
		return rules.Microbial.SomaticScale
	case evidence.EvidenceColony:
		return rules.Microbial.ColonyScale
	default:
		return rules.Physicochemical.Scale
	}
}

func calcPass(calc *evidence.DerivedCalculator, t evidence.EvidenceType, fp evidence.FixedPoint) evidence.PassResult {
	switch t {
	case evidence.EvidenceAntibiotic:
		return calc.AntibioticPass(fp)
	case evidence.EvidenceSomaticCell:
		return calc.SomaticCellPass(fp)
	case evidence.EvidenceColony:
		return calc.ColonyPass(fp)
	case evidence.EvidenceFreezingPoint:
		return calc.FreezingPointPass(fp)
	case evidence.EvidenceFat:
		return calc.FatPass(fp)
	case evidence.EvidenceProtein:
		return calc.ProteinPass(fp)
	default:
		return evidence.PassResult{}
	}
}
