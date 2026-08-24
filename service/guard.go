package service

import "github.com/dairygate/raw-milk-tank-intake-inspection/inspection"

// guardTask enforces the task-generation lock and the terminal-state barrier
// ahead of every ordinary command. It uses the inspection package's
// GenerationGuard and FinalBarrier so the in-memory gating and the persisted
// generation/outcome share one authoritative rule implementation.
func guardTask(task inspection.Task, presented inspection.Generation) *Fault {
	if f := guardGeneration(task, presented); f != nil {
		return f
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

// guardGeneration enforces only the task-generation lock. Finalize uses it
// because FinalEntered advances an already-terminal (admissible) task that
// the FinalBarrier would otherwise reject; the generation lock still must
// hold, so a stale-generation finalize from an old page can never race a
// refreshed generation into a terminal outcome.
func guardGeneration(task inspection.Task, presented inspection.Generation) *Fault {
	guard := inspection.NewGenerationGuard(task.Generation)
	if err := guard.Validate(presented); err != nil {
		return NewFault(CodeStaleGeneration, err.Error())
	}
	return nil
}
