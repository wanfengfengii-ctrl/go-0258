package store

import (
	"context"
	"database/sql"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// GetIdempotency returns the record for a task/operation, if any.
func (s *sqliteTx) GetIdempotency(ctx context.Context, taskID inspection.TaskID, opID inspection.OperationID) (inspection.IdempotencyRecord, bool, error) {
	var (
		rec      inspection.IdempotencyRecord
		response []byte
	)
	err := s.tx.QueryRowContext(ctx,
		`SELECT task_id, operation_id, operation_type, request_digest, response, error_code, logical_time
		 FROM idempotency_records WHERE task_id=? AND operation_id=?`, taskID, opID).
		Scan(&rec.TaskID, &rec.OperationID, &rec.OperationType, &rec.RequestDigest, &response, &rec.ErrorCode, &rec.LogicalTime)
	if err != nil {
		if err == sql.ErrNoRows {
			return inspection.IdempotencyRecord{}, false, nil
		}
		return inspection.IdempotencyRecord{}, false, err
	}
	rec.Response = response
	return rec, true, nil
}

// PutIdempotency inserts a record; a duplicate key yields ErrConflict.
func (s *sqliteTx) PutIdempotency(ctx context.Context, rec inspection.IdempotencyRecord) error {
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO idempotency_records (task_id, operation_id, operation_type, request_digest, response, error_code, logical_time)
		 VALUES (?,?,?,?,?,?,?)`,
		rec.TaskID, rec.OperationID, rec.OperationType, rec.RequestDigest, rec.Response, rec.ErrorCode, rec.LogicalTime,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}
