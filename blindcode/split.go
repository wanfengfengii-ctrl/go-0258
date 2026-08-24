package blindcode

import (
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// Splitter produces the three-way split-tube matrix for a tank batch. Every
// compartment yields exactly three tubes carrying one shared blind code; the
// codes are drawn from a deterministic supplier so the same batch produces
// the same matrix on retry.
type Splitter struct {
	codes []BlindCode
}

// NewSplitter builds a splitter from the supplied blind codes, one per
// compartment, in compartment order.
func NewSplitter(codes []BlindCode) *Splitter {
	return &Splitter{codes: append([]BlindCode(nil), codes...)}
}

// Split builds the matrix for the given batch and compartments. It returns a
// defect when there are not enough codes (one per compartment) or when a code
// is empty.
func (s *Splitter) Split(batch inspection.TankBatch, compartments []catalog.CompartmentCode) (*Matrix, *Defect) {
	if len(s.codes) < len(compartments) {
		return nil, &Defect{Code: "blind_shortage", Message: "not enough blind codes for compartments"}
	}
	m := &Matrix{Batch: batch}
	for i, comp := range compartments {
		code := s.codes[i]
		if code == "" {
			return nil, &Defect{Code: "blank_blind", Message: "empty blind code for compartment " + string(comp)}
		}
		for seq := 1; seq <= 3; seq++ {
			m.Tubes = append(m.Tubes, SplitTube{TubeSeq: seq, BlindCode: code, Compartment: comp})
		}
	}
	return m, nil
}

// SampleSet returns the unique blind codes present in a matrix, in first-seen
// order.
func SampleSet(m *Matrix) []BlindCode {
	if m == nil {
		return nil
	}
	seen := make(map[BlindCode]bool)
	var out []BlindCode
	for _, t := range m.Tubes {
		if !seen[t.BlindCode] {
			seen[t.BlindCode] = true
			out = append(out, t.BlindCode)
		}
	}
	return out
}

// CompartmentsFor returns the ordered compartments of a matrix.
func CompartmentsFor(m *Matrix) []catalog.CompartmentCode {
	if m == nil {
		return nil
	}
	seen := make(map[catalog.CompartmentCode]bool)
	var out []catalog.CompartmentCode
	for _, t := range m.Tubes {
		if !seen[t.Compartment] {
			seen[t.Compartment] = true
			out = append(out, t.Compartment)
		}
	}
	return out
}
