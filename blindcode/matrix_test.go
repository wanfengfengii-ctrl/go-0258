package blindcode

import (
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

func tubesFor(comp catalog.CompartmentCode, code BlindCode) []SplitTube {
	return []SplitTube{
		{TubeSeq: 1, Compartment: comp, BlindCode: code},
		{TubeSeq: 2, Compartment: comp, BlindCode: code},
		{TubeSeq: 3, Compartment: comp, BlindCode: code},
	}
}

func TestMatrixValidateConsistent(t *testing.T) {
	m := &Matrix{
		Batch: "B-001",
		Tubes: append(tubesFor("A", "BCODE-A"), tubesFor("B", "BCODE-B")...),
	}
	if d := m.Validate(); d != nil {
		t.Fatalf("unexpected defect: %+v", d)
	}
}

func TestMatrixValidateSplitCount(t *testing.T) {
	m := &Matrix{
		Batch: "B-001",
		Tubes: []SplitTube{{TubeSeq: 1, Compartment: "A", BlindCode: "BCODE-A"}},
	}
	d := m.Validate()
	if d == nil || d.Code != "split_count" {
		t.Fatalf("defect = %+v, want split_count", d)
	}
}

func TestMatrixValidateBlindMismatch(t *testing.T) {
	m := &Matrix{
		Batch: "B-001",
		Tubes: []SplitTube{
			{TubeSeq: 1, Compartment: "A", BlindCode: "BCODE-A"},
			{TubeSeq: 2, Compartment: "A", BlindCode: "BCODE-B"},
			{TubeSeq: 3, Compartment: "A", BlindCode: "BCODE-A"},
		},
	}
	d := m.Validate()
	if d == nil || d.Code != "blind_mismatch" {
		t.Fatalf("defect = %+v, want blind_mismatch", d)
	}
}

func TestMatrixValidateBlindReuse(t *testing.T) {
	m := &Matrix{
		Batch: "B-001",
		Tubes: append(tubesFor("A", "BCODE-SHARED"), tubesFor("B", "BCODE-SHARED")...),
	}
	d := m.Validate()
	if d == nil || d.Code != "blind_reuse" {
		t.Fatalf("defect = %+v, want blind_reuse", d)
	}
}
