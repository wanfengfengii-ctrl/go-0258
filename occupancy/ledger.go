package occupancy

import (
	"sync"
)

// MemoryLedger is a concurrency-safe in-memory Ledger. Acquire atomically
// rejects any lease that overlaps an active lease on the same resource, so a
// concurrent race yields exactly one winner and no partial reservation. It is
// used by tests and the in-memory demo; the SQLite ledger enforces the same
// invariant with a unique index.
type MemoryLedger struct {
	mu     sync.Mutex
	leases map[string][]Occupancy // keyed by ResourceKey
}

// NewMemoryLedger builds an empty ledger.
func NewMemoryLedger() *MemoryLedger {
	return &MemoryLedger{leases: make(map[string][]Occupancy)}
}

// Acquire atomically reserves the occupancy or reports a conflict.
func (m *MemoryLedger) Acquire(o Occupancy) (Occupancy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := o.Validate(); err != nil {
		return Occupancy{}, err
	}
	key := o.ResourceKey()
	for _, existing := range m.leases[key] {
		if existing.Active() && o.Overlaps(&existing) {
			return Occupancy{}, ErrOccupied
		}
	}
	m.leases[key] = append(m.leases[key], o)
	return o, nil
}

// Release marks every active occupancy for the task released at logical time
// now. It returns ErrNotHeld when the task holds nothing.
func (m *MemoryLedger) Release(taskID string, now int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	released := 0
	for key, list := range m.leases {
		for i := range list {
			if list[i].TaskID == taskID && list[i].Active() {
				list[i].ReleasedAt = now
				released++
			}
		}
		m.leases[key] = list
	}
	if released == 0 {
		return ErrNotHeld
	}
	return nil
}

// HeldBy returns all occupancies (active or released) for a task.
func (m *MemoryLedger) HeldBy(taskID string) []Occupancy {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Occupancy
	for _, list := range m.leases {
		for _, o := range list {
			if o.TaskID == taskID {
				out = append(out, o)
			}
		}
	}
	return out
}

// ActiveHeldBy returns only active occupancies for a task.
func (m *MemoryLedger) ActiveHeldBy(taskID string) []Occupancy {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Occupancy
	for _, list := range m.leases {
		for _, o := range list {
			if o.TaskID == taskID && o.Active() {
				out = append(out, o)
			}
		}
	}
	return out
}
