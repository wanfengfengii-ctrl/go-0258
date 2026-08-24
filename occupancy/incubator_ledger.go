package occupancy

import "fmt"

// Incubator-slot leases. An incubator is addressed by an id and a
// half-open time interval [StartAt, EndAt). Two tasks may share an incubator
// only over disjoint intervals; the ledger enforces interval non-overlap.

// IncubatorSlot builds an incubator occupancy after validating the interval
// and the incubator id.
func IncubatorSlot(taskID, incubatorID string, startAt, endAt, generation int64) (Occupancy, error) {
	o := Occupancy{
		TaskID:       taskID,
		ResourceType: ResourceIncubator,
		IncubatorID:  incubatorID,
		StartAt:      startAt,
		EndAt:        endAt,
		Generation:   generation,
	}
	if err := o.Validate(); err != nil {
		return Occupancy{}, err
	}
	return o, nil
}

// IntervalSeconds returns the length of the lease interval in seconds.
func (o *Occupancy) IntervalSeconds() int64 {
	if o.EndAt <= o.StartAt {
		return 0
	}
	return o.EndAt - o.StartAt
}

// DurationError describes an incubator interval that is too short for the
// required cultivation time.
type DurationError struct {
	RequiredSeconds int64
	ActualSeconds   int64
}

func (e *DurationError) Error() string {
	return fmt.Sprintf("incubator interval %ds shorter than required %ds", e.ActualSeconds, e.RequiredSeconds)
}

// MeetsDuration reports whether the incubator lease satisfies the required
// cultivation duration, returning a DurationError otherwise.
func (o *Occupancy) MeetsDuration(requiredSeconds int64) error {
	if o.IntervalSeconds() < requiredSeconds {
		return &DurationError{RequiredSeconds: requiredSeconds, ActualSeconds: o.IntervalSeconds()}
	}
	return nil
}
