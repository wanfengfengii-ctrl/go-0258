package store

import (
	"context"
	"database/sql"

	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func scanBlind(rows interface{ Scan(...any) error }) (blindcode.BlindSample, error) {
	var s blindcode.BlindSample
	err := rows.Scan(&s.TaskID, &s.TankBatch, &s.Compartment, &s.BlindCode, &s.MappingStatus, &s.RevealGeneration)
	if err != nil {
		return blindcode.BlindSample{}, err
	}
	return s, nil
}

const blindColumns = `task_id, tank_batch, compartment, blind_code, mapping_status, reveal_generation`

// PutBlindSample inserts a blind sample; unique constraints on (batch,
// compartment) and blind code yield ErrConflict on reuse.
func (s *sqliteTx) PutBlindSample(ctx context.Context, sample blindcode.BlindSample) error {
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO blind_samples (task_id, tank_batch, compartment, blind_code, mapping_status, reveal_generation)
		 VALUES (?,?,?,?,?,?)`,
		sample.TaskID, sample.TankBatch, sample.Compartment, sample.BlindCode, sample.MappingStatus, sample.RevealGeneration,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// GetBlindByCode resolves a blind sample by its blind code.
func (s *sqliteTx) GetBlindByCode(ctx context.Context, code blindcode.BlindCode) (blindcode.BlindSample, bool, error) {
	row := s.tx.QueryRowContext(ctx, `SELECT `+blindColumns+` FROM blind_samples WHERE blind_code=?`, code)
	sample, err := scanBlind(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return blindcode.BlindSample{}, false, nil
		}
		return blindcode.BlindSample{}, false, err
	}
	return sample, true, nil
}

// GetBlindByBatch resolves a blind sample by batch and compartment.
func (s *sqliteTx) GetBlindByBatch(ctx context.Context, batch inspection.TankBatch, comp catalog.CompartmentCode) (blindcode.BlindSample, bool, error) {
	row := s.tx.QueryRowContext(ctx, `SELECT `+blindColumns+` FROM blind_samples WHERE tank_batch=? AND compartment=?`, batch, comp)
	sample, err := scanBlind(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return blindcode.BlindSample{}, false, nil
		}
		return blindcode.BlindSample{}, false, err
	}
	return sample, true, nil
}

// ListBlind returns all blind samples for a task ordered by compartment.
func (s *sqliteTx) ListBlind(ctx context.Context, taskID inspection.TaskID) ([]blindcode.BlindSample, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT `+blindColumns+` FROM blind_samples WHERE task_id=? ORDER BY compartment`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []blindcode.BlindSample
	for rows.Next() {
		sample, err := scanBlind(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sample)
	}
	return out, rows.Err()
}

// RevealBlind marks a blind sample revealed at a generation.
func (s *sqliteTx) RevealBlind(ctx context.Context, code blindcode.BlindCode, generation int64) error {
	res, err := s.tx.ExecContext(ctx,
		`UPDATE blind_samples SET mapping_status=?, reveal_generation=? WHERE blind_code=?`,
		blindcode.MappingRevealed, generation, code)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
