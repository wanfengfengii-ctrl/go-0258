package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// BlindSplitRequest carries one blind code per compartment, in compartment
// order.
type BlindSplitRequest struct {
	OperationID inspection.OperationID `json:"operationId"`
	Generation  inspection.Generation  `json:"generation"`
	Codes       []blindcode.BlindCode  `json:"codes"`
}

// BlindSplitResult is the established split matrix and gate summary.
type BlindSplitResult struct {
	TaskID      inspection.TaskID     `json:"taskId"`
	Generation  inspection.Generation `json:"generation"`
	Status      inspection.Status     `json:"status"`
	BlindCodes  []blindcode.BlindCode `json:"blindCodes"`
	SampleCount int                   `json:"sampleCount"`
}

// BlindSplit writes the three-way split matrix and establishes the one-time
// blind-code gate for each compartment, then advances the task.
func (s *Service) BlindSplit(ctx context.Context, id inspection.TaskID, req BlindSplitRequest) (*BlindSplitResult, *Fault) {
	if len(req.Codes) == 0 {
		return nil, NewFault(CodeBadRequest, "codes are required")
	}

	var result *BlindSplitResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if f := guardTask(task, req.Generation); f != nil {
			return f
		}
		if !inspection.Allows(task.Status, inspection.CmdBlindSplit) {
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
			blinds, _ := tx.ListBlind(ctx, task.ID)
			result = blindSplitResult(task, blinds)
			return nil
		}

		splitter := blindcode.NewSplitter(req.Codes)
		matrix, defect := splitter.Split(task.TankBatch, task.Compartments)
		if defect != nil {
			return NewFault(codeForDefect(defect.Code), defect.Message)
		}
		if d := matrix.Validate(); d != nil {
			return NewFault(codeForDefect(d.Code), d.Message)
		}

		now := s.clock.Now()
		for _, comp := range task.Compartments {
			code, ok := splitterCodesFor(matrix, comp)
			if !ok {
				return NewFault(CodeBlindReuse, "no blind code for compartment "+string(comp))
			}
			if _, exists, err := tx.GetBlindByCode(ctx, code); err != nil {
				return err
			} else if exists {
				return NewFault(CodeBlindReuse, "blind code "+string(code)+" already used")
			}
			if err := tx.PutBlindSample(ctx, blindcode.BlindSample{
				TaskID: task.ID, TankBatch: task.TankBatch, Compartment: comp,
				BlindCode: code, MappingStatus: blindcode.MappingMapped,
			}); err != nil {
				if err == store.ErrConflict {
					return NewFault(CodeDuplicateBlind, "batch/compartment already mapped")
				}
				return err
			}
		}

		if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
			TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpBlindSplit,
			RequestDigest: digest, LogicalTime: now,
		}); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID: task.ID, Generation: task.Generation,
			EventType: inspection.EventBlindSplit, LogicalTime: now,
			Detail: "split into " + strconvItoa(len(task.Compartments)) + " compartments",
		}); err != nil {
			return err
		}

		updated := task
		updated.Status = inspection.StatusPlateOccupied
		if err := tx.UpdateTaskCAS(ctx, task.ID, task.Status, task.Generation, updated); err != nil {
			return err
		}

		blinds, _ := tx.ListBlind(ctx, task.ID)
		result = blindSplitResult(updated, blinds)
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

func splitterCodesFor(m *blindcode.Matrix, comp catalog.CompartmentCode) (blindcode.BlindCode, bool) {
	for _, t := range m.Tubes {
		if t.Compartment == comp {
			return t.BlindCode, true
		}
	}
	return "", false
}

func blindSplitResult(task inspection.Task, blinds []blindcode.BlindSample) *BlindSplitResult {
	codes := make([]blindcode.BlindCode, 0, len(blinds))
	for _, b := range blinds {
		codes = append(codes, b.BlindCode)
	}
	return &BlindSplitResult{
		TaskID: task.ID, Generation: task.Generation, Status: task.Status,
		BlindCodes: codes, SampleCount: len(blinds) * 3,
	}
}

func codeForDefect(code string) string {
	switch code {
	case "split_count", "split_mismatch":
		return CodeSplitCount
	case "blind_shortage", "blank_blind":
		return CodeBadRequest
	case "blind_mismatch", "compartment_mismatch":
		return CodeSplitMismatch
	case "blind_reuse":
		return CodeBlindReuse
	default:
		return CodeBadRequest
	}
}
