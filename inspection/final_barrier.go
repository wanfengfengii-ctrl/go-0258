package inspection

// FinalBarrier is the terminal-state gate that runs ahead of every ordinary
// command. Once a task reaches a final outcome, no reading, rejudgement,
// review or status change may modify persisted state; late and duplicate
// commands are rejected without side effects.
type FinalBarrier struct {
	FinalType FinalType
	reached   bool
}

// NewFinalBarrier builds a barrier in the not-yet-final state.
func NewFinalBarrier() *FinalBarrier {
	return &FinalBarrier{}
}

// Reach marks the barrier as reached with the given terminal type.
func (b *FinalBarrier) Reach(t FinalType) {
	b.FinalType = t
	b.reached = true
}

// Reached reports whether a terminal outcome has been recorded.
func (b *FinalBarrier) Reached() bool { return b.reached }

// Guard returns a FinalBarrierError when the task is already terminal.
func (b *FinalBarrier) Guard() error {
	if b.reached {
		return &FinalBarrierError{FinalType: b.FinalType}
	}
	return nil
}

// FinalBarrierError describes a rejected command on a terminal task.
type FinalBarrierError struct {
	FinalType FinalType
}

func (e *FinalBarrierError) Error() string {
	return "task already finalized with outcome " + string(e.FinalType)
}
