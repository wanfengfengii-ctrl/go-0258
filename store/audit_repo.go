package store

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// AppendAudit appends an audit event. The (task, sequence) primary key keeps
// the trail ordered and gap-free per task.
func (s *sqliteTx) AppendAudit(ctx context.Context, ev inspection.AuditEvent) error {
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO audit_events (sequence, task_id, generation, event_type, actor, detail, logical_time)
		 VALUES (?,?,?,?,?,?,?)`,
		ev.Sequence, ev.TaskID, ev.Generation, ev.EventType, ev.Actor, ev.Detail, ev.LogicalTime,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// ListAudit returns all audit events for a task ordered by sequence.
func (s *sqliteTx) ListAudit(ctx context.Context, taskID inspection.TaskID) ([]inspection.AuditEvent, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT sequence, task_id, generation, event_type, actor, detail, logical_time
		 FROM audit_events ORDER BY sequence`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []inspection.AuditEvent
	for rows.Next() {
		var ev inspection.AuditEvent
		if err := rows.Scan(&ev.Sequence, &ev.TaskID, &ev.Generation, &ev.EventType, &ev.Actor, &ev.Detail, &ev.LogicalTime); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}
