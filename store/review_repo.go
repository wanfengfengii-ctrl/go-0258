package store

import (
	"context"
	"encoding/json"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// PutRejudgement inserts one rejudgement; the (task, generation) primary key
// enforces one rejudgement per generation.
func (s *sqliteTx) PutRejudgement(ctx context.Context, r arbiter.Rejudgement) error {
	blind, _ := json.Marshal(r.BlindCodes)
	compartments, _ := json.Marshal(r.Compartments)
	wells, _ := json.Marshal(r.Wells)
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO rejudgements (task_id, generation, reason, blind_codes, compartments, wells)
		 VALUES (?,?,?,?,?,?)`,
		r.TaskID, r.Generation, r.Reason, string(blind), string(compartments), string(wells),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// GetRejudgement returns the rejudgement for a task/generation, if any.
func (s *sqliteTx) GetRejudgement(ctx context.Context, taskID inspection.TaskID, generation int64) (arbiter.Rejudgement, bool, error) {
	var (
		r                          arbiter.Rejudgement
		blind, compartments, wells string
	)
	err := s.tx.QueryRowContext(ctx,
		`SELECT task_id, generation, reason, blind_codes, compartments, wells FROM rejudgements WHERE task_id=? AND generation=?`,
		taskID, generation).
		Scan(&r.TaskID, &r.Generation, &r.Reason, &blind, &compartments, &wells)
	if err != nil {
		if isNoRows(err) {
			return arbiter.Rejudgement{}, false, nil
		}
		return arbiter.Rejudgement{}, false, err
	}
	_ = json.Unmarshal([]byte(blind), &r.BlindCodes)
	_ = json.Unmarshal([]byte(compartments), &r.Compartments)
	_ = json.Unmarshal([]byte(wells), &r.Wells)
	return r, true, nil
}

// ListRejudgements returns all rejudgements for a task ordered by generation.
func (s *sqliteTx) ListRejudgements(ctx context.Context, taskID inspection.TaskID) ([]arbiter.Rejudgement, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT task_id, generation, reason, blind_codes, compartments, wells FROM rejudgements WHERE task_id=? ORDER BY generation`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []arbiter.Rejudgement
	for rows.Next() {
		var (
			r                          arbiter.Rejudgement
			blind, compartments, wells string
		)
		if err := rows.Scan(&r.TaskID, &r.Generation, &r.Reason, &blind, &compartments, &wells); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(blind), &r.BlindCodes)
		_ = json.Unmarshal([]byte(compartments), &r.Compartments)
		_ = json.Unmarshal([]byte(wells), &r.Wells)
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutReview inserts a review; the (task, reviewer) primary key enforces one
// review per reviewer.
func (s *sqliteTx) PutReview(ctx context.Context, r arbiter.Review) error {
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO reviews (task_id, reviewer, conclusion, generation) VALUES (?,?,?,?)`,
		r.TaskID, r.Reviewer, r.Conclusion, r.Generation,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// ListReviews returns all reviews for a task ordered by reviewer.
func (s *sqliteTx) ListReviews(ctx context.Context, taskID inspection.TaskID) ([]arbiter.Review, error) {
	rows, err := s.tx.QueryContext(ctx,
		`SELECT task_id, reviewer, conclusion, generation FROM reviews WHERE task_id=? ORDER BY reviewer`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []arbiter.Review
	for rows.Next() {
		var r arbiter.Review
		if err := rows.Scan(&r.TaskID, &r.Reviewer, &r.Conclusion, &r.Generation); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutFinalDecision upserts the unique terminal credential. The first
// finalization (admissible/quarantined/cancelled) inserts a row; advancing an
// already-admissible task to entered updates that same row in place. A task
// holds exactly one terminal decision, so the credential, final type and
// logical time are replaced on conflict.
func (s *sqliteTx) PutFinalDecision(ctx context.Context, taskID inspection.TaskID, finalType inspection.FinalType, credential string, logicalTime int64) error {
	_, err := s.tx.ExecContext(ctx,
		`INSERT INTO final_decisions (task_id, final_type, credential, logical_time)
		 VALUES (?,?,?,?)
		 ON CONFLICT(task_id) DO UPDATE SET
		   final_type = excluded.final_type,
		   credential = excluded.credential,
		   logical_time = excluded.logical_time`,
		taskID, finalType, credential, logicalTime,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

// GetFinalDecision returns the terminal credential for a task, if any.
func (s *sqliteTx) GetFinalDecision(ctx context.Context, taskID inspection.TaskID) (FinalDecision, bool, error) {
	var d FinalDecision
	err := s.tx.QueryRowContext(ctx,
		`SELECT task_id, final_type, credential, logical_time FROM final_decisions WHERE task_id=?`, taskID).
		Scan(&d.TaskID, &d.FinalType, &d.Credential, &d.LogicalTime)
	if err != nil {
		if isNoRows(err) {
			return FinalDecision{}, false, nil
		}
		return FinalDecision{}, false, err
	}
	return d, true, nil
}
