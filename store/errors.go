package store

import "errors"

// Stable store errors.
var (
	// ErrNotFound is returned when a task does not exist.
	ErrNotFound = errors.New("store: task not found")

	// ErrConflict is returned when a command conflicts with a prior generation,
	// an already-bound resource, or a unique constraint.
	ErrConflict = errors.New("store: conflict")

	// ErrClosed is returned when an operation runs after the store is closed.
	ErrClosed = errors.New("store: closed")
)
