package store

// schema is the complete SQLite DDL. All tables are created idempotently on
// open; unique indexes and a partial active-occupancy index encode the
// concurrency invariants that the Go layer also enforces.
const schema = `
CREATE TABLE IF NOT EXISTS inspection_tasks (
    id             TEXT PRIMARY KEY,
    farm_id        TEXT NOT NULL,
    tank_batch     TEXT NOT NULL,
    compartments   TEXT NOT NULL,
    seals          TEXT NOT NULL,
    recorder_model TEXT NOT NULL,
    rule_version   TEXT NOT NULL,
    generation     INTEGER NOT NULL,
    status         TEXT NOT NULL,
    final_type     TEXT NOT NULL DEFAULT '',
    created_at     INTEGER NOT NULL,
    reviewers      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS idempotency_records (
    task_id        TEXT NOT NULL,
    operation_id   TEXT NOT NULL,
    operation_type TEXT NOT NULL,
    request_digest TEXT NOT NULL,
    response       BLOB,
    error_code     TEXT NOT NULL DEFAULT '',
    logical_time   INTEGER NOT NULL,
    PRIMARY KEY (task_id, operation_id)
);

CREATE TABLE IF NOT EXISTS blind_samples (
    task_id           TEXT NOT NULL,
    tank_batch        TEXT NOT NULL,
    compartment       TEXT NOT NULL,
    blind_code        TEXT NOT NULL,
    mapping_status    TEXT NOT NULL,
    reveal_generation INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (tank_batch, compartment),
    UNIQUE (blind_code)
);
CREATE INDEX IF NOT EXISTS idx_blind_task ON blind_samples(task_id);

CREATE TABLE IF NOT EXISTS resource_occupancies (
    task_id       TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_key  TEXT NOT NULL,
    plate_id      TEXT NOT NULL DEFAULT '',
    well          TEXT NOT NULL DEFAULT '',
    incubator_id  TEXT NOT NULL DEFAULT '',
    start_at      INTEGER NOT NULL,
    end_at        INTEGER NOT NULL,
    generation    INTEGER NOT NULL,
    released_at   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (task_id, resource_key, start_at)
);
CREATE INDEX IF NOT EXISTS idx_occupancy_resource
    ON resource_occupancies(resource_key, start_at, end_at)
    WHERE released_at = 0;

CREATE TABLE IF NOT EXISTS temperature_cells (
    task_id        TEXT NOT NULL,
    recorder_id    TEXT NOT NULL,
    at_seconds     INTEGER NOT NULL,
    celsius_value  INTEGER NOT NULL,
    celsius_scale  INTEGER NOT NULL,
    window_seq     INTEGER NOT NULL DEFAULT 0,
    covered        INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (task_id, recorder_id, at_seconds)
);

CREATE TABLE IF NOT EXISTS evidence_records (
    task_id        TEXT NOT NULL,
    blind_code     TEXT NOT NULL DEFAULT '',
    compartment    TEXT NOT NULL DEFAULT '',
    well           TEXT NOT NULL DEFAULT '',
    evidence_type  TEXT NOT NULL,
    raw_value      INTEGER NOT NULL,
    raw_scale      INTEGER NOT NULL,
    derived_value  INTEGER,
    derived_scale  INTEGER,
    rule_version   TEXT NOT NULL,
    generation     INTEGER NOT NULL,
    immutable      INTEGER NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_evidence_task ON evidence_records(task_id);

CREATE TABLE IF NOT EXISTS instrument_calls (
    call_id          TEXT NOT NULL,
    task_id          TEXT PRIMARY KEY,
    instrument_type  TEXT NOT NULL,
    target           TEXT NOT NULL,
    script_result    TEXT NOT NULL,
    retry_count      INTEGER NOT NULL,
    next_retry_at    INTEGER NOT NULL,
    error_class      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rejudgements (
    task_id      TEXT NOT NULL,
    generation   INTEGER NOT NULL,
    reason       TEXT NOT NULL,
    blind_codes  TEXT NOT NULL,
    compartments TEXT NOT NULL,
    wells        TEXT NOT NULL,
    PRIMARY KEY (task_id, generation)
);

CREATE TABLE IF NOT EXISTS reviews (
    task_id     TEXT NOT NULL,
    reviewer    TEXT NOT NULL,
    conclusion  TEXT NOT NULL,
    generation  INTEGER NOT NULL,
    PRIMARY KEY (task_id, reviewer)
);

CREATE TABLE IF NOT EXISTS audit_events (
    sequence     INTEGER NOT NULL,
    task_id      TEXT NOT NULL,
    generation   INTEGER NOT NULL,
    event_type   TEXT NOT NULL,
    actor        TEXT NOT NULL DEFAULT '',
    detail       TEXT NOT NULL DEFAULT '',
    logical_time INTEGER NOT NULL,
    PRIMARY KEY (task_id, sequence)
);

CREATE TABLE IF NOT EXISTS sampling_confirmations (
    task_id       TEXT NOT NULL,
    person        TEXT NOT NULL,
    farm_id       TEXT NOT NULL,
    tank_batch    TEXT NOT NULL,
    compartments  TEXT NOT NULL,
    seals         TEXT NOT NULL,
    operation_id  TEXT NOT NULL,
    logical_time  INTEGER NOT NULL,
    PRIMARY KEY (task_id, person)
);

CREATE TABLE IF NOT EXISTS final_decisions (
    task_id      TEXT PRIMARY KEY,
    final_type   TEXT NOT NULL,
    credential   TEXT NOT NULL,
    logical_time INTEGER NOT NULL
);
`
