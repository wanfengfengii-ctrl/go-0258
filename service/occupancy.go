package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// OccupancyRequest atomically acquires or replaces plate wells and incubator
// slots for a task.
type OccupancyRequest struct {
	OperationID inspection.OperationID `json:"operationId"`
	Generation  inspection.Generation  `json:"generation"`
	Occupancies []occupancy.Occupancy  `json:"occupancies"`
}

// OccupancyResult reports which resources were acquired.
type OccupancyResult struct {
	TaskID     inspection.TaskID     `json:"taskId"`
	Generation inspection.Generation `json:"generation"`
	Status     inspection.Status     `json:"status"`
	Acquired   []occupancy.Occupancy `json:"acquired"`
}

// AcquireOccupancy atomically reserves the requested resources. A conflict on
// any resource rolls back the whole transaction so no partial occupancy is
// left behind.
func (s *Service) AcquireOccupancy(ctx context.Context, id inspection.TaskID, req OccupancyRequest) (*OccupancyResult, *Fault) {
	if len(req.Occupancies) == 0 {
		return nil, NewFault(CodeBadRequest, "occupancies are required")
	}

	var result *OccupancyResult
	err := s.store.WithTx(ctx, func(tx store.Tx) error {
		task, fault := s.taskFrom(ctx, tx, id)
		if fault != nil {
			return fault
		}
		if f := guardTask(task, req.Generation); f != nil {
			return f
		}
		if !inspection.Allows(task.Status, inspection.CmdOccupancyAcquire) {
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
			held, _ := tx.ListOccupancy(ctx, task.ID)
			result = &OccupancyResult{TaskID: task.ID, Generation: task.Generation, Status: task.Status, Acquired: held}
			return nil
		}

		for i := range req.Occupancies {
			o := req.Occupancies[i]
			if o.TaskID == "" {
				o.TaskID = string(task.ID)
			}
			o.Generation = int64(task.Generation)
			if err := tx.AcquireOccupancy(ctx, o); err != nil {
				switch err {
				case occupancy.ErrOccupied:
					return NewFault(CodeOccupancyConflict, o.ResourceKey())
				case occupancy.ErrInvalidLease:
					return NewFault(CodeInvalidLease, o.ResourceKey())
				default:
					return err
				}
			}
		}

		now := s.clock.Now()
		if err := tx.PutIdempotency(ctx, inspection.IdempotencyRecord{
			TaskID: task.ID, OperationID: req.OperationID, OperationType: inspection.OpOccupancyAcquire,
			RequestDigest: digest, LogicalTime: now,
		}); err != nil {
			return err
		}
		if err := s.appendAudit(ctx, tx, inspection.AuditEvent{
			TaskID: task.ID, Generation: task.Generation,
			EventType: inspection.EventOccupied, LogicalTime: now,
			Detail: "occupied " + strconvItoa(len(req.Occupancies)) + " resources",
		}); err != nil {
			return err
		}

		updated := task
		updated.Status = inspection.StatusColdChainVerifying
		if err := tx.UpdateTaskCAS(ctx, task.ID, task.Status, task.Generation, updated); err != nil {
			return err
		}
		held, _ := tx.ListOccupancy(ctx, task.ID)
		result = &OccupancyResult{TaskID: task.ID, Generation: updated.Generation, Status: updated.Status, Acquired: held}
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
