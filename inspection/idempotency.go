package inspection

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// OperationType names the business commands that carry an idempotency key.
type OperationType string

const (
	OpCreateTask        OperationType = "create_task"
	OpSamplingConfirm   OperationType = "sampling_confirm"
	OpBlindSplit        OperationType = "blind_split"
	OpOccupancyAcquire  OperationType = "occupancy_acquire"
	OpColdChainReadings OperationType = "cold_chain_readings"
	OpReading           OperationType = "reading"
	OpRejudge           OperationType = "rejudge"
	OpReview            OperationType = "review"
	OpFinalize          OperationType = "finalize"
)

// OperationID is the client-supplied idempotency key for a command.
type OperationID string

// IdempotencyRecord captures one applied command so an identical retry returns
// the original outcome instead of being re-executed. A retry with the same
// operation id but a different request digest is a content conflict.
type IdempotencyRecord struct {
	TaskID        TaskID        `json:"taskId"`
	OperationID   OperationID   `json:"operationId"`
	OperationType OperationType `json:"operationType"`
	RequestDigest string        `json:"requestDigest"`
	Response      []byte        `json:"response"`
	ErrorCode     string        `json:"errorCode,omitempty"`
	LogicalTime   int64         `json:"logicalTime"`
}

// DigestOf computes a stable SHA-256 digest of the canonical JSON rendering of
// v, so equal requests produce equal digests and divergent requests do not.
func DigestOf(v any) string {
	h := sha256.New()
	h.Write([]byte(mustCanonicalJSON(v)))
	return hex.EncodeToString(h.Sum(nil))
}

// IdempotencyKey is the unique (task, operation) identity used to deduplicate.
type IdempotencyKey struct {
	TaskID      TaskID
	OperationID OperationID
}

// Matches reports whether two records share a key.
func (r IdempotencyRecord) Matches(o IdempotencyRecord) bool {
	return r.TaskID == o.TaskID && r.OperationID == o.OperationID
}

// ContentConflicts reports whether a retry carries a different request
// digest than the already-applied record.
func (r IdempotencyRecord) ContentConflicts(digest string) bool {
	return r.RequestDigest != "" && r.RequestDigest != digest
}

// SortRecords orders idempotency records by task then operation for a
// deterministic audit listing.
func SortRecords(recs []IdempotencyRecord) {
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].TaskID != recs[j].TaskID {
			return recs[i].TaskID < recs[j].TaskID
		}
		return recs[i].OperationID < recs[j].OperationID
	})
}
