package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// appendAudit appends an audit event with the next monotonic sequence for the
// task. It rebuilds the task's audit log from persisted events (inspection.
// RebuildAuditLog) so sequences are gap-free even across retries, restarts and
// a command that emits multiple events.
func (s *Service) appendAudit(ctx context.Context, tx store.Tx, ev inspection.AuditEvent) error {
	events, err := tx.ListAudit(ctx, ev.TaskID)
	if err != nil {
		return err
	}
	log := inspection.RebuildAuditLog(events)
	next := log.Append(ev.TaskID, ev.Generation, ev.EventType, ev.Actor, ev.Detail, ev.LogicalTime)
	return tx.AppendAudit(ctx, next)
}
