package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

type taskScanner interface {
	Scan(dest ...any) error
}

const taskColumns = `id, farm_id, tank_batch, compartments, seals, recorder_model, rule_version, generation, status, final_type, created_at, reviewers`

func scanTask(rows taskScanner) (inspection.Task, error) {
	var (
		t                       inspection.Task
		compartments, seals, rv string
	)
	if err := rows.Scan(&t.ID, &t.FarmID, &t.TankBatch, &compartments, &seals, &t.RecorderModel, &t.RuleVersion, &t.Generation, &t.Status, &t.FinalType, &t.CreatedAt, &rv); err != nil {
		return inspection.Task{}, err
	}
	if err := json.Unmarshal([]byte(compartments), &t.Compartments); err != nil {
		return inspection.Task{}, err
	}
	if err := json.Unmarshal([]byte(seals), &t.Seals); err != nil {
		return inspection.Task{}, err
	}
	if err := json.Unmarshal([]byte(rv), &t.Reviewers); err != nil {
		return inspection.Task{}, err
	}
	return t, nil
}

// CreateTask inserts a new task at generation 0 (the service bumps it to 1
// before persist where appropriate).
func (s *sqliteTx) CreateTask(ctx context.Context, t inspection.Task) error {
	compartments, _ := json.Marshal(t.Compartments)
	seals, _ := json.Marshal(t.Seals)
	reviewers, _ := json.Marshal(t.Reviewers)
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO inspection_tasks (id, farm_id, tank_batch, compartments, seals, recorder_model, rule_version, generation, status, final_type, created_at, reviewers)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.FarmID, t.TankBatch, string(compartments), string(seals), t.RecorderModel, t.RuleVersion, t.Generation, t.Status, t.FinalType, t.CreatedAt, string(reviewers),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// GetTask loads a task inside a transaction.
func (s *sqliteTx) GetTask(ctx context.Context, id inspection.TaskID) (inspection.Task, error) {
	row := s.tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM inspection_tasks WHERE id = ?`, id)
	t, err := scanTask(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return inspection.Task{}, ErrNotFound
		}
		return inspection.Task{}, err
	}
	return t, nil
}

// UpdateTaskCAS applies a compare-and-set update: it only writes when the
// task still has wantStatus and wantGeneration, and returns ErrConflict
// otherwise. This is the atomic terminal-state and status-advance gate.
func (s *sqliteTx) UpdateTaskCAS(ctx context.Context, id inspection.TaskID, wantStatus inspection.Status, wantGeneration inspection.Generation, update inspection.Task) error {
	compartments, _ := json.Marshal(update.Compartments)
	seals, _ := json.Marshal(update.Seals)
	reviewers, _ := json.Marshal(update.Reviewers)
	res, err := s.tx.ExecContext(ctx,
		`UPDATE inspection_tasks SET
		   farm_id=?, tank_batch=?, compartments=?, seals=?, recorder_model=?, rule_version=?,
		   generation=?, status=?, final_type=?, created_at=?, reviewers=?
		 WHERE id=? AND status=? AND generation=?`,
		update.FarmID, update.TankBatch, string(compartments), string(seals), update.RecorderModel, update.RuleVersion,
		update.Generation, update.Status, update.FinalType, update.CreatedAt, string(reviewers),
		id, wantStatus, wantGeneration,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}
