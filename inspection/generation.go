package inspection

import "strconv"

// GenerationGuard enforces the task-generation lock. Every command must
// present the current generation; a stale or future generation is rejected
// deterministically, and a command can advance the generation exactly once
// when its transition is accepted.
type GenerationGuard struct {
	current Generation
}

// NewGenerationGuard builds a guard starting at the given generation.
func NewGenerationGuard(g Generation) *GenerationGuard {
	return &GenerationGuard{current: g}
}

// Current returns the guard's generation.
func (g *GenerationGuard) Current() Generation { return g.current }

// Validate reports whether the presented generation matches the current one.
// It returns an error describing the mismatch.
func (g *GenerationGuard) Validate(presented Generation) error {
	switch {
	case presented < g.current:
		return &GenerationError{Current: g.current, Presented: presented, Future: false}
	case presented > g.current:
		return &GenerationError{Current: g.current, Presented: presented, Future: true}
	default:
		return nil
	}
}

// Advance moves the guard forward to next when next is exactly one greater
// than the current generation, reflecting a monotonic generation lock.
func (g *GenerationGuard) Advance(next Generation) bool {
	if next != g.current+1 {
		return false
	}
	g.current = next
	return true
}

// GenerationError describes a stale or future generation mismatch.
type GenerationError struct {
	Current   Generation
	Presented Generation
	Future    bool
}

func (e *GenerationError) Error() string {
	if e.Future {
		return "generation " + strconv.FormatInt(int64(e.Presented), 10) + " is ahead of current " + strconv.FormatInt(int64(e.Current), 10)
	}
	return "stale generation " + strconv.FormatInt(int64(e.Presented), 10) + " (current " + strconv.FormatInt(int64(e.Current), 10) + ")"
}
