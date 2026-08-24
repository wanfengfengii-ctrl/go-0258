package inspection

import "github.com/dairygate/raw-milk-tank-intake-inspection/catalog"

// TaskID identifies one tank-batch inspection aggregate.
type TaskID string

// TankBatch is the farm's milk tank batch number (乳罐批号).
type TankBatch string

// Generation is the monotonic task generation (任务代次). Every read and
// write is bound to a generation; stale generations are rejected.
type Generation int64

// FinalType is the unique terminal outcome of a completed inspection.
type FinalType string

const (
	FinalAdmissible  FinalType = "admissible"  // 可入厂
	FinalEntered     FinalType = "entered"     // 已入厂
	FinalQuarantined FinalType = "quarantined" // 质量隔离
	FinalCancelled   FinalType = "cancelled"   // 已取消
)

// Task is the tank-batch inspection aggregate. It freezes farm identity,
// compartments, seals, recorder, thresholds and reviewers at build time.
type Task struct {
	ID            TaskID                    `json:"id"`
	FarmID        catalog.FarmID            `json:"farmId"`
	TankBatch     TankBatch                 `json:"tankBatch"`
	Compartments  []catalog.CompartmentCode `json:"compartments"`
	Seals         []catalog.SealCode        `json:"seals"`
	RecorderModel catalog.RecorderModel     `json:"recorderModel"`
	RuleVersion   string                    `json:"ruleVersion"`
	Generation    Generation                `json:"generation"`
	Status        Status                    `json:"status"`
	FinalType     FinalType                 `json:"finalType,omitempty"`
	CreatedAt     int64                     `json:"createdAt"`
	Reviewers     []catalog.PersonID        `json:"reviewers"`
}

// Advance moves the task forward to next when the transition is legal. It
// returns false and leaves the task unchanged otherwise.
func (t *Task) Advance(next Status) bool {
	if !t.Status.CanAdvanceTo(next) {
		return false
	}
	t.Status = next
	return true
}
