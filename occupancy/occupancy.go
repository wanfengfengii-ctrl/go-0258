// Package occupancy models the limited-time resource ledgers for inhibition
// plate wells and incubator slots, plus the concurrency arbitration that
// guarantees a single valid task holds each resource.
package occupancy

// ResourceType discriminates the two ledger kinds.
type ResourceType string

const (
	ResourcePlateWell ResourceType = "plate_well" // 抑制板孔位
	ResourceIncubator ResourceType = "incubator"  // 培养箱时段
)

// Occupancy is a lease on a single plate well or incubator slot interval.
type Occupancy struct {
	TaskID       string       `json:"taskId"`
	ResourceType ResourceType `json:"resourceType"`
	PlateID      string       `json:"plateId,omitempty"`
	Well         string       `json:"well,omitempty"`
	IncubatorID  string       `json:"incubatorId,omitempty"`
	StartAt      int64        `json:"startAt"`
	EndAt        int64        `json:"endAt"`
	Generation   int64        `json:"generation"`
	ReleasedAt   int64        `json:"releasedAt,omitempty"`
}

// Conflict reports whether two occupancies overlap on the same resource.
func (o *Occupancy) Conflict(other *Occupancy) bool {
	if o.ResourceType != other.ResourceType {
		return false
	}
	sameResource := false
	switch o.ResourceType {
	case ResourcePlateWell:
		sameResource = o.PlateID == other.PlateID && o.Well == other.Well
	case ResourceIncubator:
		sameResource = o.IncubatorID == other.IncubatorID
	}
	if !sameResource {
		return false
	}
	return o.StartAt < other.EndAt && other.StartAt < o.EndAt
}

// Ledger is the limited-time occupancy book. Every acquire is atomic: either
// the whole lease is recorded or nothing is.
type Ledger interface {
	// Acquire atomically reserves the occupancy or reports a conflict.
	Acquire(o Occupancy) (Occupancy, error)
	// Release marks the occupancy released at logical time now.
	Release(taskID string, now int64) error
	// HeldBy returns all active occupancies for a task.
	HeldBy(taskID string) []Occupancy
}
