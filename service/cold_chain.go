package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// ColdChainReadingsRequest is a batch of temperature samples against the
// locked window.
type ColdChainReadingsRequest struct {
	OperationID inspection.OperationID     `json:"operationId"`
	Generation  inspection.Generation      `json:"generation"`
	BaseTime    int64                      `json:"baseTime"`
	RecorderID  string                     `json:"recorderId"`
	Cells       []evidence.TemperatureCell `json:"cells"`
}

// ColdChainResult reports coverage and the consecutive over-limit minutes.
type ColdChainResult struct {
	TaskID          inspection.TaskID     `json:"taskId"`
	Generation      inspection.Generation `json:"generation"`
	Status          inspection.Status     `json:"status"`
	CoveredCount    int                   `json:"coveredCount"`
	ExpectedCount   int                   `json:"expectedCount"`
	ConsecutiveOver int64                 `json:"consecutiveOverSeconds"`
	Complete        bool                  `json:"complete"`
	OverLimit       bool                  `json:"overLimit"`
}

// ColdChainReadings validates and persists a temperature batch, then advances
// the task. A missing, duplicated or out-of-range sample rejects the whole
// batch.
func (s *Service) ColdChainReadings(ctx context.Context, id inspection.TaskID, req ColdChainReadingsRequest) (*ColdChainResult, *Fault) {
	if req.RecorderID == "" || len(req.Cells) == 0 {
		return nil, NewFault(CodeBadRequest, "recorderId and cells are required")
	}

	var result *ColdChainResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if f := guardTask(task, req.Generation); f != nil {
			return f
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
		digest := inspection.DigestOf(req)
		if exists {
			if prior.ContentConflicts(digest) {
				return NewFault(CodeContentConflict, "retry with different content")
			}
			return s.fillColdChainResult(ctx, tx, task, result)
		}

		if !inspection.Allows(task.Status, inspection.CmdColdChainReadings) {
			return NewFault(CodeIllegalTransition, string(task.Status))
		}

		rules, ok := s.Catalog().Rules(task.RuleVersion)
		if !ok {
			return NewFault(CodeUnknownRules, task.RuleVersion)
		}

		cells := make([]evidence.TemperatureCell, len(req.Cells))
		for i, c := range req.Cells {
			c.TaskID = string(task.ID)
			c.RecorderID = req.RecorderID
			cells[i] = c
		}
		grid := evidence.NewGrid(rules.Temperature)
		if d := grid.ValidateCells(req.BaseTime, req.RecorderID, cells); d != nil {
			return NewFault(codeForTemp(d.Code), d.Message)
		}
		computed := grid.Compute(req.BaseTime, req.RecorderID, cells)

		if err := tx.PutTemperature(ctx, cells); err != nil {
			if err == store.ErrConflict {
				return NewFault(CodeTemperatureDuplicate, "duplicate time point")
			}
			return err
		}
		now := s.clock.Now()
		if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
			TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpColdChainReadings,
			RequestDigest: digest, LogicalTime: now,
		}); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID: task.ID, Generation: task.Generation,
			EventType: inspection.EventColdChain, LogicalTime: now,
			Detail: computed.SummaryFragment,
		}); err != nil {
			return err
		}

		updated := task
		updated.Status = inspection.StatusAntibioticReading
		if err := tx.UpdateTaskCAS(ctx, task.ID, task.Status, task.Generation, updated); err != nil {
			return err
		}
		result = &ColdChainResult{
			TaskID: task.ID, Generation: updated.Generation, Status: updated.Status,
			CoveredCount: computed.CoveredCount, ExpectedCount: computed.ExpectedCount,
			ConsecutiveOver: computed.ConsecutiveOver, Complete: computed.Complete,
			OverLimit: computed.ConsecutiveOver > rules.Temperature.MaxConsecutiveOverSeconds,
		}
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

func (s *Service) fillColdChainResult(ctx context.Context, tx store.Tx, task inspection.Task, out *ColdChainResult) error {
	cells, err := tx.ListTemperature(ctx, task.ID)
	if err != nil {
		return err
	}
	rules, _ := s.Catalog().Rules(task.RuleVersion)
	grid := evidence.NewGrid(rules.Temperature)
	baseTime := int64(0)
	if len(cells) > 0 {
		baseTime = cells[0].AtSeconds
	}
	computed := grid.Compute(baseTime, "", cells)
	*out = ColdChainResult{
		TaskID: task.ID, Generation: task.Generation, Status: task.Status,
		CoveredCount: computed.CoveredCount, ExpectedCount: computed.ExpectedCount,
		ConsecutiveOver: computed.ConsecutiveOver, Complete: computed.Complete,
		OverLimit: computed.ConsecutiveOver > rules.Temperature.MaxConsecutiveOverSeconds,
	}
	return nil
}

func codeForTemp(code string) string {
	switch code {
	case "temperature_missing":
		return CodeTemperatureMissing
	case "temperature_duplicate":
		return CodeTemperatureDuplicate
	case "temperature_out_of_window", "temperature_above_max", "temperature_below_min":
		return CodeTemperatureRange
	default:
		return CodeBadRequest
	}
}
