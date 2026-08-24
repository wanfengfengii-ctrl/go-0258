package service

import "github.com/dairygate/raw-milk-tank-intake-inspection/inspection"

// guardTask enforces the task-generation lock and the terminal-state barrier
// ahead of every command. It uses the inspection package's GenerationGuard and
// FinalBarrier so the in-memory gating and the persisted generation/outcome
// share one authoritative rule implementation.
func guardTask(task inspection.Task, presented inspection.Generation) *Fault {
	guard := inspection.NewGenerationGuard(task.Generation)
	if err := guard.Validate(presented); err != nil {
		return NewFault(CodeStaleGeneration, err.Error())
	}
	barrier := inspection.NewFinalBarrier()
	if task.FinalType != "" {
		barrier.Reach(task.FinalType)
	}
	if err := barrier.Guard(); err != nil {
		return NewFault(CodeTerminalState, string(task.FinalType))
	}
	return nil
}
