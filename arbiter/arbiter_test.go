package arbiter

import (
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/evidence"
)

func TestClassifyInhibitionBoundary(t *testing.T) {
	min := evidence.FixedPoint{Value: 180, Scale: 1} // 18.0 mm
	if c := ClassifyInhibition(evidence.FixedPoint{Value: 180, Scale: 1}, min); c != InhibitionNegative {
		t.Fatalf("equal zone = %s, want negative", c)
	}
	if c := ClassifyInhibition(evidence.FixedPoint{Value: 179, Scale: 1}, min); c != InhibitionSuspectPositive {
		t.Fatalf("below zone = %s, want suspect_positive", c)
	}
}

func TestEvaluateMicrobialContamination(t *testing.T) {
	somatic := evidence.FixedPoint{Value: 350000, Scale: 3}
	colony := evidence.FixedPoint{Value: 50000, Scale: 0}
	d := EvaluateMicrobial(somatic, colony,
		evidence.FixedPoint{Value: 400000, Scale: 3}, evidence.FixedPoint{Value: 100000, Scale: 0})
	if !d.Pass() {
		t.Fatalf("expected pass, got %+v", d)
	}
	d.MarkContamination()
	if d.Pass() {
		t.Fatal("contaminated decision must not pass")
	}
}

func TestEvaluateDecisionPreconditions(t *testing.T) {
	// All gates pass + two distinct reviewers -> admissible.
	input := DecisionInput{
		ColdChainComplete: true, ColdChainOver: false,
		AntibioticPass: true, MicrobialPass: true, PhysicoPass: true,
		EvidenceComplete: true,
		RequiredReviewers: 2,
		Reviews: []Review{
			{Reviewer: "r1", Conclusion: ReviewPass},
			{Reviewer: "r2", Conclusion: ReviewPass},
		},
	}
	if c := Evaluate(input); c.FinalType != "admissible" {
		t.Fatalf("conclusion = %+v, want admissible", c)
	}

	// One gate fails -> quarantined with sorted reasons.
	input.AntibioticPass = false
	input.ColdChainOver = true
	if c := Evaluate(input); c.FinalType != "quarantined" {
		t.Fatalf("conclusion = %+v, want quarantined", c)
	}
}

func TestLateFilterDropsOldGeneration(t *testing.T) {
	f := NewLateFilter(2)
	records := []evidence.EvidenceRecord{
		{Generation: 1}, {Generation: 2}, {Generation: 3},
	}
	kept := f.Filter(records)
	if len(kept) != 2 || kept[0].Generation != 2 || kept[1].Generation != 3 {
		t.Fatalf("filtered = %+v, want generations 2 and 3", kept)
	}
}

func TestMemoryArbiterDistinctReviewers(t *testing.T) {
	a := NewMemoryArbiter()
	if err := a.Review(Review{TaskID: "t", Reviewer: "r1", Conclusion: ReviewPass}); err != nil {
		t.Fatal(err)
	}
	if err := a.Review(Review{TaskID: "t", Reviewer: "r1", Conclusion: ReviewPass}); err != ErrDuplicateReview {
		t.Fatalf("err = %v, want ErrDuplicateReview", err)
	}
	if err := a.Review(Review{TaskID: "t", Reviewer: "r2", Conclusion: ReviewPass}); err != nil {
		t.Fatal(err)
	}
	if got := a.Reviews("t"); len(got) != 2 {
		t.Fatalf("reviews = %d, want 2", len(got))
	}
}

func TestRequiredReviewersMet(t *testing.T) {
	// A single reviewer repeating does not satisfy a two-reviewer requirement.
	one := []Review{{Reviewer: catalog.PersonID("a"), Conclusion: ReviewPass}}
	if RequiredReviewersMet(one, 2) {
		t.Fatal("one reviewer must not satisfy a two-reviewer requirement")
	}
	// Two distinct reviewers do satisfy it.
	two := []Review{
		{Reviewer: catalog.PersonID("a"), Conclusion: ReviewPass},
		{Reviewer: catalog.PersonID("b"), Conclusion: ReviewPass},
	}
	if !RequiredReviewersMet(two, 2) {
		t.Fatal("two distinct reviewers must satisfy a two-reviewer requirement")
	}
}
