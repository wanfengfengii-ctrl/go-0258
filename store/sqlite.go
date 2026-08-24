package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	_ "modernc.org/sqlite"
)

// SQLiteStore is the durable WAL-backed aggregate store. Writes are
// serialized through a process-local mutex so interval-overlap arbitration is
// deterministic, while unique indexes and compare-and-set updates still
// enforce the cross-command invariants. On open it recreates the schema and
// rebuilds nothing in memory; every read hits the durable tables, so restart
// recovery is exact.
type SQLiteStore struct {
	db  *sql.DB
	cat catalog.Catalog
	// writeMu serializes write transactions so occupancy interval checks and
	// finalization CAS never race within this process.
	writeMu sync.Mutex
	closed  bool
}

// OpenSQLite opens (or creates) the SQLite database at path and applies the
// schema. Use path ":memory:" for a non-durable store.
func OpenSQLite(path string, cat catalog.Catalog) (*SQLiteStore, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	if path == ":memory:" {
		// A single connection keeps the in-memory database alive and shared.
		db.SetMaxOpenConns(1)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: apply schema: %w", err)
	}
	return &SQLiteStore{db: db, cat: cat}, nil
}

// Catalog returns the rule directory.
func (s *SQLiteStore) Catalog() catalog.Catalog { return s.cat }

// Close releases the database handle.
func (s *SQLiteStore) Close() error {
	if s.closed {
		return ErrClosed
	}
	s.closed = true
	return s.db.Close()
}

// WithTx runs fn inside one serialized SQLite transaction. A nil error
// commits; any error rolls back.
func (s *SQLiteStore) WithTx(ctx context.Context, fn func(tx Tx) error) error {
	if s.closed {
		return ErrClosed
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	sqlTx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin: %w", err)
	}
	tx := &sqliteTx{tx: sqlTx}
	if err := fn(tx); err != nil {
		_ = sqlTx.Rollback()
		return err
	}
	if err := sqlTx.Commit(); err != nil {
		return fmt.Errorf("store: commit: %w", err)
	}
	return nil
}

// GetTask returns a task by ID outside a transaction.
func (s *SQLiteStore) GetTask(ctx context.Context, id inspection.TaskID) (inspection.Task, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return inspection.Task{}, err
	}
	defer tx.Rollback()
	return (&sqliteTx{tx: tx}).GetTask(ctx, id)
}

// ListTasks returns all tasks ordered by creation time.
func (s *SQLiteStore) ListTasks(ctx context.Context) ([]inspection.Task, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, farm_id, tank_batch, compartments, seals, recorder_model, rule_version, generation, status, final_type, created_at, reviewers FROM inspection_tasks ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []inspection.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Snapshot assembles the complete persisted state of one task.
func (s *SQLiteStore) Snapshot(ctx context.Context, id inspection.TaskID) (*Snapshot, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	st := &sqliteTx{tx: tx}
	// The snapshot is a pure read; roll the transaction back on every exit
	// path so the pooled connection is released. Without this the first
	// snapshot holds its connection (and, under the single-connection
	// in-memory store, the only connection), so the next GET blocks on the
	// pool until the busy timeout. Match GetTask's read-transaction pattern.
	defer tx.Rollback()

	task, err := st.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{Task: task}

	if snap.BlindSamples, err = st.ListBlind(ctx, id); err != nil {
		return nil, err
	}
	if snap.Occupancies, err = st.ListOccupancy(ctx, id); err != nil {
		return nil, err
	}
	if snap.Temperature, err = st.ListTemperature(ctx, id); err != nil {
		return nil, err
	}
	if snap.Evidence, err = st.ListEvidence(ctx, id); err != nil {
		return nil, err
	}
	if snap.InstrumentCalls, err = st.ListInstrumentCalls(ctx, id); err != nil {
		return nil, err
	}
	if snap.Rejudgements, err = st.ListRejudgements(ctx, id); err != nil {
		return nil, err
	}
	if snap.Reviews, err = st.ListReviews(ctx, id); err != nil {
		return nil, err
	}
	if snap.Audit, err = st.ListAudit(ctx, id); err != nil {
		return nil, err
	}
	if fd, ok, err := st.GetFinalDecision(ctx, id); err != nil {
		return nil, err
	} else if ok {
		snap.FinalDecision = &fd
	}
	return snap, nil
}

// sqliteTx implements Tx against a *sql.Tx.
type sqliteTx struct {
	tx *sql.Tx
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return containsSQLiteConstraint(err)
}

func isNoRows(err error) bool {
	return err == sql.ErrNoRows
}

func containsSQLiteConstraint(err error) bool {
	msg := err.Error()
	return containsStr(msg, "UNIQUE constraint failed") ||
		containsStr(msg, "constraint failed") ||
		containsStr(msg, "PRIMARY KEY")
}

func containsStr(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
