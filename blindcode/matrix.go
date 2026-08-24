package blindcode

import (
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// Matrix is the three-way split-tube consistency matrix for one batch. Every
// compartment must split into exactly three tubes with matching blind code
// and compartment; any missing, duplicated or mismatched tube is a defect.
type Matrix struct {
	Batch inspection.TankBatch `json:"batch"`
	Tubes []SplitTube          `json:"tubes"`
}

// Defect describes a specific consistency violation in a split matrix.
type Defect struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Validate checks the matrix for quantity, blind-code and compartment
// consistency. It returns the first defect found, or nil when consistent.
func (m *Matrix) Validate() *Defect {
	perCompartment := map[catalog.CompartmentCode][]SplitTube{}
	for _, t := range m.Tubes {
		perCompartment[t.Compartment] = append(perCompartment[t.Compartment], t)
	}
	for comp, tubes := range perCompartment {
		if len(tubes) != 3 {
			return &Defect{Code: "split_count", Message: "compartment " + string(comp) + " must split into three tubes"}
		}
		code := tubes[0].BlindCode
		for _, t := range tubes {
			if t.BlindCode != code {
				return &Defect{Code: "blind_mismatch", Message: "compartment " + string(comp) + " tubes disagree on blind code"}
			}
			if t.Compartment != comp {
				return &Defect{Code: "compartment_mismatch", Message: "tube written to wrong compartment"}
			}
		}
	}
	seen := map[BlindCode]catalog.CompartmentCode{}
	for _, t := range m.Tubes {
		if prev, ok := seen[t.BlindCode]; ok && prev != t.Compartment {
			return &Defect{Code: "blind_reuse", Message: "blind code reused across compartments"}
		}
		seen[t.BlindCode] = t.Compartment
	}
	return nil
}
