package evidence

import (
	"errors"
	"sync"
)

// Evidence immutability errors.
var (
	// ErrImmutable is returned when a valid write would overwrite existing
	// evidence. Evidence can only be superseded by a newer generation.
	ErrImmutable = errors.New("evidence: record is immutable")
	// ErrInvalidEvidence is returned when a record lacks its required identity
	// fields.
	ErrInvalidEvidence = errors.New("evidence: invalid record")
)

// MemoryWriter is a concurrency-safe in-memory Writer that enforces the
// append-only evidence invariant: a valid record can be written once, and a
// later write for the same (type, target) at the same or older generation is
// rejected. It is used by tests and the in-memory demo; the SQLite writer
// enforces the same invariant with a uniqueness constraint.
type MemoryWriter struct {
	mu      sync.Mutex
	records []EvidenceRecord
	keys    map[string]int64 // key -> latest generation written
}

// NewMemoryWriter builds an empty writer.
func NewMemoryWriter() *MemoryWriter {
	return &MemoryWriter{keys: make(map[string]int64)}
}

// recordKey builds a stable identity for a record's evidence slot.
func recordKey(r EvidenceRecord) string {
	switch r.Type {
	case EvidenceTemperature:
		return string(r.Type) + ":" + string(r.Compartment)
	default:
		return string(r.Type) + ":" + r.BlindCode + ":" + r.Well + ":" + string(r.Compartment)
	}
}

// Write validates then appends one evidence record.
func (w *MemoryWriter) Write(r EvidenceRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.validate(r); err != nil {
		return err
	}
	key := recordKey(r)
	if prev, ok := w.keys[key]; ok && r.Generation <= prev {
		return ErrImmutable
	}
	w.keys[key] = r.Generation
	r.Immutable = true
	w.records = append(w.records, r)
	return nil
}

// WriteTemperature validates then appends one temperature cell batch. The
// whole batch is rejected if any cell duplicates a prior time point.
func (w *MemoryWriter) WriteTemperature(cells []TemperatureCell) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	seen := make(map[string]bool, len(cells))
	for _, c := range cells {
		if c.TaskID == "" || c.RecorderID == "" {
			return ErrInvalidEvidence
		}
		key := string(EvidenceTemperature) + ":" + c.RecorderID + ":" + itoa(int(c.AtSeconds))
		if seen[key] {
			return ErrImmutable
		}
		seen[key] = true
	}
	w.records = append(w.records, temperatureCellsToRecords(cells)...)
	return nil
}

func (w *MemoryWriter) validate(r EvidenceRecord) error {
	if r.TaskID == "" {
		return ErrInvalidEvidence
	}
	if r.Type == "" {
		return ErrInvalidEvidence
	}
	return nil
}

// Records returns a copy of all written evidence records.
func (w *MemoryWriter) Records() []EvidenceRecord {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]EvidenceRecord, len(w.records))
	copy(out, w.records)
	return out
}

func temperatureCellsToRecords(cells []TemperatureCell) []EvidenceRecord {
	out := make([]EvidenceRecord, 0, len(cells))
	for _, c := range cells {
		out = append(out, EvidenceRecord{
			TaskID:      c.TaskID,
			Type:        EvidenceTemperature,
			Raw:         c.Celsius,
			Immutable:   true,
			Compartment: "",
		})
	}
	return out
}
