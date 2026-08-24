package service

import (
	"context"

	"github.com/dairygate/raw-milk-tank-intake-inspection/arbiter"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

// Report is a human- and machine-readable summary of an inspection: every
// reading with its pass/fail, the cold-chain coverage, the decision and the
// audit trail. It is produced from the persisted snapshot and is used by the
// browser console and operators.
type Report struct {
	TaskID       inspection.TaskID     `json:"taskId"`
	TankBatch    inspection.TankBatch  `json:"tankBatch"`
	FarmID       catalog.FarmID        `json:"farmId"`
	Status       inspection.Status     `json:"status"`
	Generation   inspection.Generation `json:"generation"`
	Readings     []ReadingSummary      `json:"readings"`
	ColdChain    ColdChainSummary      `json:"coldChain"`
	Reviews      []ReviewSummary       `json:"reviews"`
	Rejudgements []arbiter.Rejudgement `json:"rejudgements"`
	Decision     string                `json:"decision,omitempty"`
	AuditCount   int                   `json:"auditCount"`
}

// ReadingSummary is one reading with its pass/fail result.
type ReadingSummary struct {
	Type  evidence.EvidenceType `json:"type"`
	Blind string                `json:"blindCode"`
	Value string                `json:"value"`
	Pass  bool                  `json:"pass"`
}

// ColdChainSummary is the cold-chain coverage summary.
type ColdChainSummary struct {
	CoveredCount    int   `json:"coveredCount"`
	ExpectedCount   int   `json:"expectedCount"`
	ConsecutiveOver int64 `json:"consecutiveOverSeconds"`
	Complete        bool  `json:"complete"`
}

// ReviewSummary is one review with its conclusion.
type ReviewSummary struct {
	Reviewer   catalog.PersonID         `json:"reviewer"`
	Conclusion arbiter.ReviewConclusion `json:"conclusion"`
}

// BuildReport assembles the report for a task from its persisted snapshot.
func (s *Service) BuildReport(ctx context.Context, id inspection.TaskID) (*Report, *Fault) {
	snap, fault := s.GetSnapshot(ctx, id)
	if fault != nil {
		return nil, fault
	}
	return s.reportFromSnapshot(snap), nil
}

func (s *Service) reportFromSnapshot(snap *store.Snapshot) *Report {
	rules, ok := s.Catalog().Rules(snap.Task.RuleVersion)
	var calc *evidence.DerivedCalculator
	if ok {
		calc = evidence.NewDerivedCalculator(rules)
	}
	report := &Report{
		TaskID: snap.Task.ID, TankBatch: snap.Task.TankBatch, FarmID: snap.Task.FarmID,
		Status: snap.Task.Status, Generation: snap.Task.Generation, AuditCount: len(snap.Audit),
	}

	for _, rec := range snap.Evidence {
		pass := true
		if calc != nil {
			pass = readingPass(calc, rec)
		}
		report.Readings = append(report.Readings, ReadingSummary{
			Type: rec.Type, Blind: rec.BlindCode, Value: rec.Raw.String(), Pass: pass,
		})
	}

	grid := evidence.NewGrid(rules.Temperature)
	baseTime := int64(0)
	if len(snap.Temperature) > 0 {
		baseTime = snap.Temperature[0].AtSeconds
	}
	computed := grid.Compute(baseTime, "", snap.Temperature)
	report.ColdChain = ColdChainSummary{
		CoveredCount: computed.CoveredCount, ExpectedCount: computed.ExpectedCount,
		ConsecutiveOver: computed.ConsecutiveOver, Complete: computed.Complete,
	}

	for _, rv := range snap.Reviews {
		report.Reviews = append(report.Reviews, ReviewSummary{Reviewer: rv.Reviewer, Conclusion: rv.Conclusion})
	}
	report.Rejudgements = snap.Rejudgements

	if snap.FinalDecision != nil {
		report.Decision = string(snap.FinalDecision.FinalType)
	} else if snap.Task.FinalType != "" {
		report.Decision = string(snap.Task.FinalType)
	}
	return report
}

func readingPass(calc *evidence.DerivedCalculator, rec evidence.EvidenceRecord) bool {
	switch rec.Type {
	case evidence.EvidenceAntibiotic:
		return calc.AntibioticPass(rec.Raw).Pass
	case evidence.EvidenceSomaticCell:
		return calc.SomaticCellPass(rec.Raw).Pass
	case evidence.EvidenceColony:
		return calc.ColonyPass(rec.Raw).Pass
	case evidence.EvidenceFreezingPoint:
		return calc.FreezingPointPass(rec.Raw).Pass
	case evidence.EvidenceFat:
		return calc.FatPass(rec.Raw).Pass
	case evidence.EvidenceProtein:
		return calc.ProteinPass(rec.Raw).Pass
	default:
		return true
	}
}
