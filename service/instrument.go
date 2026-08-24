package service

import (
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// Instrument failure classification and retry planning. A rejected,
// disconnected, timed-out or malformed instrument call never forges a pass
// and never releases an occupancy early; it only appends an auditable call
// record with a deterministic retry plan.

// InstrumentErrorClass names the failure category of an instrument call.
const (
	ErrClassRejected   = "rejected"
	ErrClassTimeout    = "timeout"
	ErrClassDisconnect = "disconnect"
	ErrClassMalformed  = "malformed"
)

// RetryPlanner computes the next retry time for an instrument call using a
// deterministic exponential backoff over the clock.
type RetryPlanner struct {
	BaseSeconds int64
	MaxSeconds  int64
}

// NewRetryPlanner builds a planner with the given base and max backoff.
func NewRetryPlanner(base, max int64) *RetryPlanner {
	if base <= 0 {
		base = 1
	}
	if max <= 0 {
		max = 3600
	}
	return &RetryPlanner{BaseSeconds: base, MaxSeconds: max}
}

// Next returns the backoff delay for the given retry count.
func (p *RetryPlanner) Next(retryCount int) int64 {
	delay := p.BaseSeconds
	for i := 0; i < retryCount && delay < p.MaxSeconds; i++ {
		delay *= 2
		if delay > p.MaxSeconds {
			delay = p.MaxSeconds
		}
	}
	return delay
}

// Plan builds an InstrumentCall for a failed invocation.
func (p *RetryPlanner) Plan(callID, taskID, instrumentType, target, scriptResult, errorClass string, now int64) store.InstrumentCall {
	return store.InstrumentCall{
		CallID: callID, TaskID: inspection.TaskID(taskID), InstrumentType: instrumentType,
		Target: target, ScriptResult: scriptResult, RetryCount: 0,
		NextRetryAt: now + p.Next(0), ErrorClass: errorClass,
	}
}

// Reproject advances an existing call's retry count and next retry time.
func (p *RetryPlanner) Reproject(call store.InstrumentCall, now int64) store.InstrumentCall {
	call.RetryCount++
	call.NextRetryAt = now + p.Next(call.RetryCount)
	return call
}
