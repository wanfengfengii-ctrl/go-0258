// Package arbiter holds the rejudgement and independent-review rules that
// turn raw measurements into a single final quality decision.
package arbiter

import "github.com/dairygate/raw-milk-tank-intake-inspection/catalog"

// RejudgementReason classifies why a rejudgement was triggered.
type RejudgementReason string

const (
	ReasonSuspectPositive   RejudgementReason = "suspect_positive"   // 抗生素疑阳
	ReasonContamination     RejudgementReason = "contamination"      // 菌落污染
	ReasonColdChainBreak    RejudgementReason = "cold_chain_break"   // 冷链断点
	ReasonSplitDisagreement RejudgementReason = "split_disagreement" // 三联结果分歧
)

// Rejudgement is a single generation-stamped rejudgement covering the affected
// blind codes, compartments and wells. Only one may exist per generation.
type Rejudgement struct {
	TaskID       string                    `json:"taskId"`
	Generation   int64                     `json:"generation"`
	Reason       RejudgementReason         `json:"reason"`
	BlindCodes   []string                  `json:"blindCodes"`
	Compartments []catalog.CompartmentCode `json:"compartments"`
	Wells        []string                  `json:"wells"`
}

// Review is an independent review by a single qualified reviewer.
type Review struct {
	TaskID     string           `json:"taskId"`
	Reviewer   catalog.PersonID `json:"reviewer"`
	Conclusion ReviewConclusion `json:"conclusion"`
	Generation int64            `json:"generation"`
}

// ReviewConclusion is the reviewer's outcome.
type ReviewConclusion string

const (
	ReviewPass ReviewConclusion = "pass"
	ReviewFail ReviewConclusion = "fail"
)

// Arbiter evaluates measurements and produces rejudgements and decisions.
type Arbiter interface {
	// Rejudge creates the single rejudgement for a generation, or reports
	// that one already exists.
	Rejudge(r Rejudgement) error
	// Review records an independent review, enforcing distinct qualified
	// reviewers before finalization is allowed.
	Review(r Review) error
	// CanFinalize reports whether the preconditions for finalization are met.
	CanFinalize(taskID string) (bool, error)
}
