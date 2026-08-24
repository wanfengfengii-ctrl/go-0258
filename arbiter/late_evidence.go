package arbiter

import "github.com/dairygate/raw-milk-tank-intake-inspection/evidence"

// Late-evidence filtering. A reading that arrives after the task has advanced
// past the generation it was taken at, or that carries an older generation
// than the current evidence, must never overwrite current evidence or change
// the current conclusion. The filter drops such readings deterministically.

// LateFilter selects evidence records whose generation is not older than the
// current generation. Records with an older generation are dropped so a late
// reading cannot supersede newer evidence.
type LateFilter struct {
	current int64
}

// NewLateFilter builds a filter for the given current generation.
func NewLateFilter(current int64) *LateFilter {
	return &LateFilter{current: current}
}

// Accept reports whether a record's generation is current (or newer).
func (f *LateFilter) Accept(record evidence.EvidenceRecord) bool {
	return record.Generation >= f.current
}

// Filter returns only the records accepted by the filter, preserving order.
func (f *LateFilter) Filter(records []evidence.EvidenceRecord) []evidence.EvidenceRecord {
	out := make([]evidence.EvidenceRecord, 0, len(records))
	for _, r := range records {
		if f.Accept(r) {
			out = append(out, r)
		}
	}
	return out
}

// LatestGeneration returns the maximum generation among the records, or 0 when
// empty.
func LatestGeneration(records []evidence.EvidenceRecord) int64 {
	var max int64
	for _, r := range records {
		if r.Generation > max {
			max = r.Generation
		}
	}
	return max
}
