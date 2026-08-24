package arbiter

import (
	"errors"
	"sync"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

// Stable arbiter errors.
var (
	// ErrRejudgementExists is returned when a rejudgement already exists for
	// the generation, enforcing one rejudgement per generation.
	ErrRejudgementExists = errors.New("arbiter: rejudgement already exists for generation")
	// ErrDuplicateReview is returned when a reviewer already recorded a review
	// for the task.
	ErrDuplicateReview = errors.New("arbiter: reviewer already recorded")
	// ErrRejudgementIncomplete is returned when a rejudgement does not cover
	// every affected blind code, compartment and well.
	ErrRejudgementIncomplete = errors.New("arbiter: rejudgement does not cover all affected objects")
)

// MemoryArbiter is a concurrency-safe in-memory Arbiter. It enforces the
// one-rejudgement-per-generation and distinct-reviewer invariants used by
// tests and the in-memory demo.
type MemoryArbiter struct {
	mu           sync.Mutex
	rejudgements map[int64]Rejudgement
	reviews      map[string]map[catalog.PersonID]Review // task -> reviewer -> review
}

// NewMemoryArbiter builds an empty arbiter.
func NewMemoryArbiter() *MemoryArbiter {
	return &MemoryArbiter{
		rejudgements: make(map[int64]Rejudgement),
		reviews:      make(map[string]map[catalog.PersonID]Review),
	}
}

// Rejudge records a single rejudgement for its generation.
func (a *MemoryArbiter) Rejudge(r Rejudgement) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, exists := a.rejudgements[r.Generation]; exists {
		return ErrRejudgementExists
	}
	a.rejudgements[r.Generation] = r
	return nil
}

// Rejudgement returns the rejudgement for a generation, if any.
func (a *MemoryArbiter) Rejudgement(generation int64) (Rejudgement, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	r, ok := a.rejudgements[generation]
	return r, ok
}

// Review records an independent review, rejecting a duplicate reviewer.
func (a *MemoryArbiter) Review(r Review) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	byReviewer, ok := a.reviews[r.TaskID]
	if !ok {
		byReviewer = make(map[catalog.PersonID]Review)
		a.reviews[r.TaskID] = byReviewer
	}
	if _, exists := byReviewer[r.Reviewer]; exists {
		return ErrDuplicateReview
	}
	byReviewer[r.Reviewer] = r
	return nil
}

// Reviews returns the reviews recorded for a task.
func (a *MemoryArbiter) Reviews(taskID string) []Review {
	a.mu.Lock()
	defer a.mu.Unlock()
	byReviewer := a.reviews[taskID]
	out := make([]Review, 0, len(byReviewer))
	for _, r := range byReviewer {
		out = append(out, r)
	}
	return out
}

// CanFinalize reports whether a task has no unresolved rejudgement at the
// current generation. It is a coarse gate; the full decision lives in
// decision.Evaluate.
func (a *MemoryArbiter) CanFinalize(taskID string) (bool, error) {
	// The current generation rejudgement must be absent for finalization.
	a.mu.Lock()
	defer a.mu.Unlock()
	_ = taskID
	return len(a.rejudgements) == 0, nil
}
