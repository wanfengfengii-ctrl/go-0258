// Package store defines the persistence boundary for the inspection
// aggregate. The SQLite WAL implementation is layered behind this interface
// so the application and tests depend only on the stable contract.
//
// Every business command runs inside a single SQLite transaction (WithTx):
// the command reads the task generation, idempotency records, occupancy
// ledgers and status, and any failed validation rolls the whole transaction
// back. Unique indexes plus compare-and-set status updates arbitrate
// concurrent builds, well/slot acquisition and finalization.
package store

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/blindcode"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/occupancy"
)

// Store is the durable aggregate store.
type Store interface {
	// Catalog returns the read-only rule directory backing the store.
	Catalog() catalog.Catalog

	// WithTx runs fn inside one SQLite transaction. Any error returned by fn
	// rolls the transaction back; a nil error commits it.
	WithTx(ctx context.Context, fn func(tx Tx) error) error

	// GetTask returns the task by ID, or ErrNotFound.
	GetTask(ctx context.Context, id inspection.TaskID) (inspection.Task, error)
	// ListTasks returns all tasks ordered by creation time.
	ListTasks(ctx context.Context) ([]inspection.Task, error)
	// Snapshot returns the full, persisted state of one task for the console
	// and restart-recovery assertions.
	Snapshot(ctx context.Context, id inspection.TaskID) (*Snapshot, error)

	// Close releases the underlying resources.
	Close() error
}

// Tx is a single store transaction. Its methods read and write one aggregate
// each; callers compose them inside WithTx to keep a command atomic.
type Tx interface {
	// Tasks.
	CreateTask(ctx context.Context, t inspection.Task) error
	GetTask(ctx context.Context, id inspection.TaskID) (inspection.Task, error)
	// OpenTaskForBatch reports whether a non-terminal task already holds the
	// given tank batch. It is the cross-task uniqueness gate enforced at
	// build time so the same batch cannot drive two concurrent flows.
	OpenTaskForBatch(ctx context.Context, farmID catalog.FarmID, batch inspection.TankBatch) (inspection.TaskID, bool, error)
	// UpdateTaskCAS applies a compare-and-set status/generation update. It
	// returns ErrConflict when the expected status or generation no longer
	// matches.
	UpdateTaskCAS(ctx context.Context, id inspection.TaskID, wantStatus inspection.Status, wantGeneration inspection.Generation, update inspection.Task) error

	// Idempotency.
	GetIdempotency(ctx context.Context, taskID inspection.TaskID, opID inspection.OperationID) (inspection.IdempotencyRecord, bool, error)
	PutIdempotency(ctx context.Context, rec inspection.IdempotencyRecord) error

	// Sampling confirmations.
	PutSamplingConfirmation(ctx context.Context, c SamplingConfirmation) error
	ListSamplingConfirmations(ctx context.Context, taskID inspection.TaskID) ([]SamplingConfirmation, error)

	// Blind samples.
	PutBlindSample(ctx context.Context, sample blindcode.BlindSample) error
	GetBlindByCode(ctx context.Context, code blindcode.BlindCode) (blindcode.BlindSample, bool, error)
	GetBlindByBatch(ctx context.Context, batch inspection.TankBatch, comp catalog.CompartmentCode) (blindcode.BlindSample, bool, error)
	ListBlind(ctx context.Context, taskID inspection.TaskID) ([]blindcode.BlindSample, error)
	RevealBlind(ctx context.Context, code blindcode.BlindCode, generation int64) error

	// Occupancy.
	AcquireOccupancy(ctx context.Context, o occupancy.Occupancy) error
	ListOccupancy(ctx context.Context, taskID inspection.TaskID) ([]occupancy.Occupancy, error)
	ActiveOccupancyFor(ctx context.Context, o occupancy.Occupancy) ([]occupancy.Occupancy, error)
	ReleaseOccupancy(ctx context.Context, taskID inspection.TaskID, now int64) error

	// Evidence.
	PutEvidence(ctx context.Context, r evidence.EvidenceRecord) error
	ListEvidence(ctx context.Context, taskID inspection.TaskID) ([]evidence.EvidenceRecord, error)
	PutTemperature(ctx context.Context, cells []evidence.TemperatureCell) error
	ListTemperature(ctx context.Context, taskID inspection.TaskID) ([]evidence.TemperatureCell, error)

	// Instrument calls.
	PutInstrumentCall(ctx context.Context, call InstrumentCall) error
	ListInstrumentCalls(ctx context.Context, taskID inspection.TaskID) ([]InstrumentCall, error)

	// Reviews and rejudgements.
	PutRejudgement(ctx context.Context, r arbiter.Rejudgement) error
	GetRejudgement(ctx context.Context, taskID inspection.TaskID, generation int64) (arbiter.Rejudgement, bool, error)
	ListRejudgements(ctx context.Context, taskID inspection.TaskID) ([]arbiter.Rejudgement, error)
	PutReview(ctx context.Context, r arbiter.Review) error
	ListReviews(ctx context.Context, taskID inspection.TaskID) ([]arbiter.Review, error)

	// Audit.
	AppendAudit(ctx context.Context, ev inspection.AuditEvent) error
	ListAudit(ctx context.Context, taskID inspection.TaskID) ([]inspection.AuditEvent, error)

	// Finalization.
	PutFinalDecision(ctx context.Context, taskID inspection.TaskID, finalType inspection.FinalType, credential string, logicalTime int64) error
	GetFinalDecision(ctx context.Context, taskID inspection.TaskID) (FinalDecision, bool, error)
}

// Snapshot is the complete persisted state of one task, rebuilt on read for
// the console and for restart-recovery assertions.
type Snapshot struct {
	Task            inspection.Task            `json:"task"`
	BlindSamples    []blindcode.BlindSample    `json:"blindSamples"`
	Occupancies     []occupancy.Occupancy      `json:"occupancies"`
	Temperature     []evidence.TemperatureCell `json:"temperature"`
	Evidence        []evidence.EvidenceRecord  `json:"evidence"`
	InstrumentCalls []InstrumentCall           `json:"instrumentCalls"`
	Rejudgements    []arbiter.Rejudgement      `json:"rejudgements"`
	Reviews         []arbiter.Review           `json:"reviews"`
	Audit           []inspection.AuditEvent    `json:"audit"`
	FinalDecision   *FinalDecision             `json:"finalDecision,omitempty"`
}

// InstrumentCall is a persisted, auditable instrument invocation with a retry
// plan. Failures append a call record; they never forge a pass or release an
// occupancy early.
type InstrumentCall struct {
	CallID         string            `json:"callId"`
	TaskID         inspection.TaskID `json:"taskId"`
	InstrumentType string            `json:"instrumentType"`
	Target         string            `json:"target"`
	ScriptResult   string            `json:"scriptResult"`
	RetryCount     int               `json:"retryCount"`
	NextRetryAt    int64             `json:"nextRetryAt"`
	ErrorClass     string            `json:"errorClass"`
}

// FinalDecision is the unique terminal outcome credential for a task.
type FinalDecision struct {
	TaskID      inspection.TaskID    `json:"taskId"`
	FinalType   inspection.FinalType `json:"finalType"`
	Credential  string               `json:"credential"`
	LogicalTime int64                `json:"logicalTime"`
}
