package store

import (
	"context"
	"database/sql"

	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

func scanEvidence(rows interface{ Scan(...any) error }) (evidence.EvidenceRecord, error) {
	var (
		r                          evidence.EvidenceRecord
		derivedValue, derivedScale sql.NullInt64
	)
	err := rows.Scan(&r.TaskID, &r.BlindCode, &r.Compartment, &r.Well, &r.Type, &r.Raw.Value, &r.Raw.Scale, &derivedValue, &derivedScale, &r.RuleVersion, &r.Generation, &r.Immutable)
	if err != nil {
		return evidence.EvidenceRecord{}, err
	}
	if derivedValue.Valid {
		d, err := evidence.New(derivedValue.Int64, int(derivedScale.Int64))
		if err != nil {
			return evidence.EvidenceRecord{}, err
		}
		r.Derived = &d
	}
	return r, nil
}

const evidenceColumns = `task_id, blind_code, compartment, well, evidence_type, raw_value, raw_scale, derived_value, derived_scale, rule_version, generation, immutable`

// PutEvidence appends one evidence record. Immutability is enforced by the
// service's generation guard; the table itself is append-oriented.
func (s *sqliteTx) PutEvidence(ctx context.Context, r evidence.EvidenceRecord) error {
	var derivedValue, derivedScale any
	if r.Derived != nil {
		derivedValue = r.Derived.Value
		derivedScale = r.Derived.Scale
	}
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO evidence_records (task_id, blind_code, compartment, well, evidence_type, raw_value, raw_scale, derived_value, derived_scale, rule_version, generation, immutable)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.TaskID, r.BlindCode, string(r.Compartment), r.Well, r.Type, r.Raw.Value, r.Raw.Scale, derivedValue, derivedScale, r.RuleVersion, r.Generation, boolToInt(r.Immutable),
	)
	return err
}

// ListEvidence returns all evidence for a task, ordered by generation then
// type.
func (s *sqliteTx) ListEvidence(ctx context.Context, taskID inspection.TaskID) ([]evidence.EvidenceRecord, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT `+evidenceColumns+` FROM evidence_records ORDER BY generation, evidence_type, blind_code, compartment, well`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.EvidenceRecord
	for rows.Next() {
		r, err := scanEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutTemperature inserts a batch of temperature cells. A duplicate time point
// (task, recorder, at) yields a unique violation.
func (s *sqliteTx) PutTemperature(ctx context.Context, cells []evidence.TemperatureCell) error {
	for _, c := range cells {
		_, err := s.tx.ExecContext(ctx,
			`INSERT INTO temperature_cells (task_id, recorder_id, at_seconds, celsius_value, celsius_scale, window_seq, covered)
			 VALUES (?,?,?,?,?,?,?)`,
			c.TaskID, c.RecorderID, c.AtSeconds, c.Celsius.Value, c.Celsius.Scale, c.WindowSeq, boolToInt(c.Covered),
		)
		if err != nil {
			if isUniqueViolation(err) {
				return ErrConflict
			}
			return err
		}
	}
	return nil
}

// ListTemperature returns all temperature cells for a task ordered by time.
func (s *sqliteTx) ListTemperature(ctx context.Context, taskID inspection.TaskID) ([]evidence.TemperatureCell, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT task_id, recorder_id, at_seconds, celsius_value, celsius_scale, window_seq, covered
		 FROM temperature_cells WHERE task_id=? ORDER BY at_seconds`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []evidence.TemperatureCell
	for rows.Next() {
		var c evidence.TemperatureCell
		if err := rows.Scan(&c.TaskID, &c.RecorderID, &c.AtSeconds, &c.Celsius.Value, &c.Celsius.Scale, &c.WindowSeq, &c.Covered); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
