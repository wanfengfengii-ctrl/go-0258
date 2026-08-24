package store

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// PutInstrumentCall appends an instrument invocation record with its retry
// plan. Failures append; they never forge a pass.
func (s *sqliteTx) PutInstrumentCall(ctx context.Context, call InstrumentCall) error {
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO instrument_calls (call_id, task_id, instrument_type, target, script_result, retry_count, next_retry_at, error_class)
		 VALUES (?,?,?,?,?,?,?,?)`,
		call.CallID, call.TaskID, call.InstrumentType, call.Target, call.ScriptResult, call.RetryCount, call.NextRetryAt, call.ErrorClass,
	)
	return err
}

// ListInstrumentCalls returns all instrument calls for a task, ordered by
// call id for determinism.
func (s *sqliteTx) ListInstrumentCalls(ctx context.Context, taskID inspection.TaskID) ([]InstrumentCall, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT call_id, task_id, instrument_type, target, script_result, retry_count, next_retry_at, error_class
		 FROM instrument_calls WHERE task_id=? ORDER BY call_id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InstrumentCall
	for rows.Next() {
		var c InstrumentCall
		if err := rows.Scan(&c.CallID, &c.TaskID, &c.InstrumentType, &c.Target, &c.ScriptResult, &c.RetryCount, &c.NextRetryAt, &c.ErrorClass); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
