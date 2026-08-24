package blindcode

import "errors"

// Stable blind-code errors. Every rejection maps to one of these so the API
// can return a deterministic error code.
var (
	// ErrAlreadyMapped is returned when a (batch, compartment) pair already has
	// a one-time blind-code mapping.
	ErrAlreadyMapped = errors.New("blindcode: batch/compartment already mapped")
	// ErrBlindReuse is returned when a blind code is reused for a second sample.
	ErrBlindReuse = errors.New("blindcode: blind code already used")
	// ErrPrematureReveal is returned when a reveal is attempted before the
	// allowed status.
	ErrPrematureReveal = errors.New("blindcode: reveal not allowed yet")
	// ErrStaleReveal is returned when a reveal carries an old generation.
	ErrStaleReveal = errors.New("blindcode: stale reveal generation")
)
