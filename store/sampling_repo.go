package store

import (
	"context"
	"encoding/json"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// SamplingConfirmation is a persisted dual-sampling confirmation from one
// operator. Two distinct, role-separated operators must confirm identical
// content before a task advances.
type SamplingConfirmation struct {
	TaskID       inspection.TaskID         `json:"taskId"`
	Person       catalog.PersonID          `json:"person"`
	FarmID       catalog.FarmID            `json:"farmId"`
	TankBatch    inspection.TankBatch      `json:"tankBatch"`
	Compartments []catalog.CompartmentCode `json:"compartments"`
	Seals        []catalog.SealCode        `json:"seals"`
	OperationID  inspection.OperationID    `json:"operationId"`
	LogicalTime  int64                     `json:"logicalTime"`
}

// PutSamplingConfirmation inserts a confirmation; the (task, person) primary
// key makes a repeat from the same person a conflict.
func (s *sqliteTx) PutSamplingConfirmation(ctx context.Context, c SamplingConfirmation) error {
	compartments, _ := json.Marshal(c.Compartments)
	seals, _ := json.Marshal(c.Seals)
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO sampling_confirmations (task_id, person, farm_id, tank_batch, compartments, seals, operation_id, logical_time)
		 VALUES (?,?,?,?,?,?,?,?)`,
		c.TaskID, c.Person, c.FarmID, c.TankBatch, string(compartments), string(seals), c.OperationID, c.LogicalTime,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// ListSamplingConfirmations returns all confirmations for a task ordered by
// person.
func (s *sqliteTx) ListSamplingConfirmations(ctx context.Context, taskID inspection.TaskID) ([]SamplingConfirmation, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT task_id, person, farm_id, tank_batch, compartments, seals, operation_id, logical_time
		 FROM sampling_confirmations WHERE task_id=? ORDER BY person`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SamplingConfirmation
	for rows.Next() {
		var (
			c                   SamplingConfirmation
			compartments, seals string
		)
		if err := rows.Scan(&c.TaskID, &c.Person, &c.FarmID, &c.TankBatch, &compartments, &seals, &c.OperationID, &c.LogicalTime); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(compartments), &c.Compartments)
		_ = json.Unmarshal([]byte(seals), &c.Seals)
		out = append(out, c)
	}
	return out, rows.Err()
}
