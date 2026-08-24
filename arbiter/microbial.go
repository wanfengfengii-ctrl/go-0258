package arbiter

import "github.com/dairygate/raw-milk-tank-intake-inspection/evidence"

// Microbial recount rules evaluate somatic-cell and colony counts and detect
// batch contamination. A contamination is signalled when a control/blank
// plate grows colonies, meaning the whole cultivation batch is invalid and
// every affected sample must be rejudged.

// MicrobialDecision is the outcome of evaluating a microbial reading set.
type MicrobialDecision struct {
	SomaticPass  bool              `json:"somaticPass"`
	ColonyPass   bool              `json:"colonyPass"`
	Contaminated bool              `json:"contaminated"`
	Reason       RejudgementReason `json:"reason,omitempty"`
}

// EvaluateMicrobial applies the somatic-cell and colony-count limits to a
// pair of readings, returning a pass/fail/contamination decision.
func EvaluateMicrobial(somatic, colony evidence.FixedPoint, somaticLimit, colonyLimit evidence.FixedPoint) *MicrobialDecision {
	d := &MicrobialDecision{}
	if cmp, err := somatic.Cmp(somaticLimit); err == nil {
		d.SomaticPass = cmp <= 0
	} else {
		d.SomaticPass = false
	}
	if cmp, err := colony.Cmp(colonyLimit); err == nil {
		d.ColonyPass = cmp <= 0
	} else {
		d.ColonyPass = false
	}
	return d
}

// ContaminationLimit is the maximum colony count a control plate may show
// before the batch is declared contaminated. A control plate above this limit
// invalidates the cultivation batch regardless of sample counts.
func ContaminationLimit(controlColony, controlLimit evidence.FixedPoint) bool {
	cmp, err := controlColony.Cmp(controlLimit)
	if err != nil {
		return true // unreadable control plate is treated as contaminated
	}
	return cmp > 0
}

// MarkContamination sets the contamination flag and reason on a decision.
func (d *MicrobialDecision) MarkContamination() {
	d.Contaminated = true
	d.Reason = ReasonContamination
}

// Pass reports whether the microbial decision is clean (no contamination and
// both counts within limits).
func (d *MicrobialDecision) Pass() bool {
	return d.SomaticPass && d.ColonyPass && !d.Contaminated
}
