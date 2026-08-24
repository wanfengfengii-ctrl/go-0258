package service

import "time"

// Clock supplies the logical time used by commands and audit events. The
// inspection flow is fully deterministic under a manual clock, so tests can
// script retry timing and window boundaries.
type Clock interface {
	// Now returns the current logical time in seconds.
	Now() int64
}

// SystemClock returns wall-clock time in seconds.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() int64 { return time.Now().Unix() }

// ManualClock returns a fixed value until advanced.
type ManualClock struct {
	now int64
}

// NewManualClock builds a manual clock at the given time.
func NewManualClock(start int64) *ManualClock { return &ManualClock{now: start} }

// Now implements Clock.
func (m *ManualClock) Now() int64 { return m.now }

// Advance moves the clock forward by seconds.
func (m *ManualClock) Advance(seconds int64) { m.now += seconds }

// Set pins the clock to a specific time.
func (m *ManualClock) Set(now int64) { m.now = now }
