package occupancy

import "errors"

// Stable occupancy errors. Acquisition conflicts and malformed leases map to
// these so the API can return deterministic codes.
var (
	// ErrOccupied is returned when another active lease already holds the
	// resource over an overlapping interval.
	ErrOccupied = errors.New("occupancy: resource already occupied")
	// ErrInvalidLease is returned when a lease has a non-positive interval,
	// mismatched resource fields, or a missing identity.
	ErrInvalidLease = errors.New("occupancy: invalid lease")
	// ErrNotHeld is returned when releasing a lease the task does not hold.
	ErrNotHeld = errors.New("occupancy: lease not held by task")
)
