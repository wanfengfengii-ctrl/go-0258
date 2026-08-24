// Package service orchestrates the DairyGate inspection business flows over
// the persistent store. Every command validates domain rules (state machine,
// generation lock, role qualification, threshold arithmetic) inside a single
// store transaction, and returns a Fault with a stable code plus a
// deterministically sorted reason list on rejection.
package service

import (
	"sort"
	"strings"
)

// Stable fault codes. Every rejection maps to exactly one code so the API can
// return deterministic error responses.
const (
	CodeNotFound             = "not_found"
	CodeConflict             = "conflict"
	CodeBadRequest           = "bad_request"
	CodeUnknownFarm          = "unknown_farm"
	CodeUnknownRules         = "unknown_rules"
	CodeStaleRules           = "stale_rules"
	CodeNotQualified         = "not_qualified"
	CodeRoleOverlap          = "role_overlap"
	CodeContentConflict      = "content_conflict"
	CodeStaleGeneration      = "stale_generation"
	CodeTerminalState        = "terminal_state"
	CodeIllegalTransition    = "illegal_transition"
	CodeDuplicateBlind       = "duplicate_blind"
	CodeBlindReuse           = "blind_reuse"
	CodeBlindUnknown         = "blind_unknown"
	CodePrematureReveal      = "premature_reveal"
	CodeSplitCount           = "split_count"
	CodeSplitMismatch        = "split_mismatch"
	CodeSplitDisagreement    = "split_disagreement"
	CodeOccupancyConflict    = "occupancy_conflict"
	CodeInvalidLease         = "invalid_lease"
	CodeTemperatureMissing   = "temperature_missing"
	CodeTemperatureDuplicate = "temperature_duplicate"
	CodeTemperatureRange     = "temperature_range"
	CodeSummaryConflict      = "summary_conflict"
	CodeArithmeticFailure    = "arithmetic_failure"
	CodeInstrumentFailure    = "instrument_failure"
	CodeRejudgementExists    = "rejudgement_exists"
	CodeRejudgementMissing   = "rejudgement_missing"
	CodeDuplicateReview      = "duplicate_review"
	CodeFinalizeConflict     = "finalize_conflict"
	CodeStoreError           = "store_error"
)

// Fault is a stable, deterministic rejection. Reasons are sorted so identical
// failures always serialize identically.
type Fault struct {
	Code    string   `json:"code"`
	Reasons []string `json:"reasons,omitempty"`
}

func (f *Fault) Error() string {
	if len(f.Reasons) == 0 {
		return f.Code
	}
	return f.Code + ": " + strings.Join(f.Reasons, ", ")
}

// NewFault builds a Fault from a code and a set of reasons, sorted for
// determinism.
func NewFault(code string, reasons ...string) *Fault {
	r := make([]string, 0, len(reasons))
	seen := map[string]bool{}
	for _, x := range reasons {
		if x != "" && !seen[x] {
			seen[x] = true
			r = append(r, x)
		}
	}
	sort.Strings(r)
	return &Fault{Code: code, Reasons: r}
}

// AsFault converts any error into a Fault, preserving an existing Fault and
// mapping generic errors to a store-error fault.
func AsFault(err error) *Fault {
	if err == nil {
		return nil
	}
	if f, ok := err.(*Fault); ok {
		return f
	}
	return NewFault(CodeStoreError, err.Error())
}
