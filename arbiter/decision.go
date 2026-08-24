package arbiter

import (
	"sort"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

// Final decision preconditions. A task may only be finalized admissible when
// every measurement gate is satisfied and the required number of distinct
// qualified reviewers has independently passed. Any failure, contamination,
// cold-chain break or unresolved rejudgement forces quarantine; a manual
// cancel is always a distinct terminal option.

// DecisionInput is the complete set of facts a final decision is drawn from.
type DecisionInput struct {
	ColdChainComplete bool     `json:"coldChainComplete"`
	ColdChainOver     bool     `json:"coldChainOver"`
	AntibioticPass    bool     `json:"antibioticPass"`
	MicrobialPass     bool     `json:"microbialPass"`
	PhysicoPass       bool     `json:"physicoPass"`
	Contaminated      bool     `json:"contaminated"`
	SplitDisagreement bool     `json:"splitDisagreement"`
	RequiredReviewers int      `json:"requiredReviewers"`
	Reviews           []Review `json:"reviews"`
}

// Conclusion is the outcome of evaluating final decision preconditions.
type Conclusion struct {
	FinalType string   `json:"finalType"` // admissible or quarantined
	Reasons   []string `json:"reasons"`   // sorted, deterministic failure reasons
}

// Evaluate applies every precondition and returns the conclusion. The reasons
// are sorted for deterministic output.
func Evaluate(input DecisionInput) Conclusion {
	var reasons []string
	if !input.ColdChainComplete {
		reasons = append(reasons, "cold_chain_incomplete")
	}
	if input.ColdChainOver {
		reasons = append(reasons, "cold_chain_over_limit")
	}
	if !input.AntibioticPass {
		reasons = append(reasons, "antibiotic_suspect")
	}
	if !input.MicrobialPass {
		reasons = append(reasons, "microbial_fail")
	}
	if !input.PhysicoPass {
		reasons = append(reasons, "physicochemical_fail")
	}
	if input.Contaminated {
		reasons = append(reasons, "contaminated")
	}
	if input.SplitDisagreement {
		reasons = append(reasons, "split_disagreement")
	}
	if !distinctPassingReviewers(input.Reviews, input.RequiredReviewers) {
		reasons = append(reasons, "insufficient_reviews")
	}
	sort.Strings(reasons)

	if len(reasons) == 0 {
		return Conclusion{FinalType: "admissible"}
	}
	return Conclusion{FinalType: "quarantined", Reasons: reasons}
}

// distinctPassingReviewers reports whether at least n distinct reviewers have
// each passed the task.
func distinctPassingReviewers(reviews []Review, n int) bool {
	passers := make(map[catalog.PersonID]bool)
	for _, r := range reviews {
		if r.Conclusion == ReviewPass {
			passers[r.Reviewer] = true
		}
	}
	return len(passers) >= n
}

// RequiredReviewersMet is the standalone reviewer-count precondition.
func RequiredReviewersMet(reviews []Review, required int) bool {
	return distinctPassingReviewers(reviews, required)
}
