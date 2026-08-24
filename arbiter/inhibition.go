package arbiter

import "github.com/dairygate/raw-milk-tank-intake-inspection/evidence"

// Inhibition-reader rules classify an antibiotic inhibition-zone reading
// against the locked threshold. The boundary is exact: a zone equal to the
// minimum is negative; anything below is suspect positive and routed to a
// rejudgement rather than a direct fail.

// InhibitionClass is the classification of a single inhibition reading.
type InhibitionClass string

const (
	InhibitionNegative        InhibitionClass = "negative"         // 阴性
	InhibitionSuspectPositive InhibitionClass = "suspect_positive" // 疑阳
	InhibitionInvalid         InhibitionClass = "invalid"          // 无法判读
)

// ClassifyInhibition compares the zone to the minimum inhibition threshold
// (in fixed point). It returns the class and the pass boolean.
func ClassifyInhibition(zone evidence.FixedPoint, minZone evidence.FixedPoint) InhibitionClass {
	cmp, err := zone.Cmp(minZone)
	if err != nil {
		return InhibitionInvalid
	}
	if cmp >= 0 {
		return InhibitionNegative
	}
	return InhibitionSuspectPositive
}

// IsSuspect reports whether the class routes the reading to a rejudgement.
func (c InhibitionClass) IsSuspect() bool { return c == InhibitionSuspectPositive }

// Borderline reports whether a zone is within margin of the threshold. It is
// used to flag readings that are negative but close to the boundary, which a
// reviewer may still want to see.
func Borderline(zone evidence.FixedPoint, minZone evidence.FixedPoint, margin evidence.FixedPoint) (bool, error) {
	cmp, err := zone.Cmp(minZone)
	if err != nil {
		return false, err
	}
	if cmp < 0 {
		return false, nil
	}
	diff, err := zone.Sub(minZone)
	if err != nil {
		return false, err
	}
	m, err := diff.Cmp(margin)
	if err != nil {
		return false, err
	}
	return m <= 0, nil
}
