package arbiter

import "github.com/dairygate/raw-milk-tank-intake-inspection/catalog"

// Contamination routing. When a cultivation batch is contaminated, a single
// generation-stamped rejudgement is generated that covers every affected
// blind code, compartment and well. A later rejudgement for the same
// generation is rejected, so only one rejudgement ever exists per generation.

// ContaminationScope captures the affected sample scope of a contamination
// rejudgement.
type ContaminationScope struct {
	BlindCodes   []string                  `json:"blindCodes"`
	Compartments []catalog.CompartmentCode `json:"compartments"`
	Wells        []string                  `json:"wells"`
}

// BuildContaminationRejudgement assembles a contamination rejudgement for the
// given task, generation and scope.
func BuildContaminationRejudgement(taskID string, generation int64, scope ContaminationScope) Rejudgement {
	return Rejudgement{
		TaskID:       taskID,
		Generation:   generation,
		Reason:       ReasonContamination,
		BlindCodes:   append([]string(nil), scope.BlindCodes...),
		Compartments: append([]catalog.CompartmentCode(nil), scope.Compartments...),
		Wells:        append([]string(nil), scope.Wells...),
	}
}

// CoversAll reports whether the rejudgement covers every affected blind code,
// compartment and well in the scope. A rejudgement that misses any affected
// object is incomplete and must be rejected.
func (r Rejudgement) CoversAll(scope ContaminationScope) bool {
	if !stringSetContains(r.BlindCodes, scope.BlindCodes) {
		return false
	}
	if !compartmentSetContains(r.Compartments, scope.Compartments) {
		return false
	}
	if !stringSetContains(r.Wells, scope.Wells) {
		return false
	}
	return true
}

func stringSetContains(have, want []string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func compartmentSetContains(have, want []catalog.CompartmentCode) bool {
	set := make(map[catalog.CompartmentCode]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}
