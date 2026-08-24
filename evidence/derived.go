package evidence

import "github.com/dairygate/raw-milk-tank-intake-inspection/catalog"

// Derived metrics are computed from raw fixed-point readings using checked
// arithmetic. Every calculation checks length, sign, division-by-zero and
// overflow; an arithmetic failure is isolated so no derived evidence is
// written from a bad computation.

// DerivedCalculator computes pass/fail and derived values for the
// physicochemical and microbial readings against a locked rule snapshot.
type DerivedCalculator struct {
	rules *catalog.RawMilkRules
}

// NewDerivedCalculator builds a calculator bound to the given rules.
func NewDerivedCalculator(rules *catalog.RawMilkRules) *DerivedCalculator {
	return &DerivedCalculator{rules: rules}
}

// PassResult reports whether a reading passes its threshold plus the derived
// value, or an arithmetic failure.
type PassResult struct {
	Pass    bool        `json:"pass"`
	Derived *FixedPoint `json:"derived,omitempty"`
	Err     error       `json:"-"`
}

// AntibioticPass evaluates an inhibition-zone reading. A zone at or above the
// minimum is negative (pass); below is suspect positive (fail).
func (c *DerivedCalculator) AntibioticPass(zone FixedPoint) PassResult {
	threshold := FixedPoint{Value: c.rules.Antibiotic.InhibitionZoneMM, Scale: c.rules.Antibiotic.Scale}
	cmp, err := zone.Cmp(threshold)
	if err != nil {
		return PassResult{Err: err}
	}
	return PassResult{Pass: cmp >= 0}
}

// SomaticCellPass evaluates a somatic cell count against the limit.
func (c *DerivedCalculator) SomaticCellPass(count FixedPoint) PassResult {
	threshold := FixedPoint{Value: c.rules.Microbial.SomaticCells, Scale: c.rules.Microbial.SomaticScale}
	cmp, err := count.Cmp(threshold)
	if err != nil {
		return PassResult{Err: err}
	}
	return PassResult{Pass: cmp <= 0}
}

// ColonyPass evaluates a total plate colony count against the limit.
func (c *DerivedCalculator) ColonyPass(count FixedPoint) PassResult {
	threshold := FixedPoint{Value: c.rules.Microbial.ColonyCount, Scale: c.rules.Microbial.ColonyScale}
	cmp, err := count.Cmp(threshold)
	if err != nil {
		return PassResult{Err: err}
	}
	return PassResult{Pass: cmp <= 0}
}

// FreezingPointPass evaluates a freezing-point reading against the upper
// bound. A reading above the maximum indicates water adulteration (fail).
func (c *DerivedCalculator) FreezingPointPass(fp FixedPoint) PassResult {
	threshold := FixedPoint{Value: c.rules.Physicochemical.FreezingPointMax, Scale: c.rules.Physicochemical.Scale}
	cmp, err := fp.Cmp(threshold)
	if err != nil {
		return PassResult{Err: err}
	}
	return PassResult{Pass: cmp <= 0}
}

// FatPass evaluates a fat reading against the lower bound.
func (c *DerivedCalculator) FatPass(fat FixedPoint) PassResult {
	threshold := FixedPoint{Value: c.rules.Physicochemical.FatMin, Scale: c.rules.Physicochemical.Scale}
	cmp, err := fat.Cmp(threshold)
	if err != nil {
		return PassResult{Err: err}
	}
	return PassResult{Pass: cmp >= 0}
}

// ProteinPass evaluates a protein reading against the lower bound.
func (c *DerivedCalculator) ProteinPass(protein FixedPoint) PassResult {
	threshold := FixedPoint{Value: c.rules.Physicochemical.ProteinMin, Scale: c.rules.Physicochemical.Scale}
	cmp, err := protein.Cmp(threshold)
	if err != nil {
		return PassResult{Err: err}
	}
	return PassResult{Pass: cmp >= 0}
}
