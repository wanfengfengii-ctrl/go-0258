package evidence

import (
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

func window() catalog.TemperatureWindow {
	return catalog.TemperatureWindow{
		SampleEverySeconds:        60,
		WindowSeconds:             3600,
		MaxCelsius:                60, // 6.0 C
		MinCelsius:                0,
		MaxConsecutiveOverSeconds: 300,
		Scale:                     1,
	}
}

func cells(values ...int64) []TemperatureCell {
	var out []TemperatureCell
	for i, v := range values {
		out = append(out, TemperatureCell{
			RecorderID: "rec", AtSeconds: int64(i * 60),
			Celsius: FixedPoint{Value: v, Scale: 1},
		})
	}
	return out
}

// TestGridConsecutiveOverMinutes computes the consecutive over-limit minutes
// with integer arithmetic.
func TestGridConsecutiveOverMinutes(t *testing.T) {
	g := NewGrid(window())
	// 0..60 min at 60s spacing = 61 samples; the middle three (index 20,21,22)
	// are over 6.0 C (value 70 = 7.0 C), giving 180 consecutive over seconds.
	vals := make([]int64, 61)
	for i := range vals {
		vals[i] = 40
	}
	vals[20], vals[21], vals[22] = 70, 70, 70
	res := g.Compute(0, "rec", cells(vals...))
	if res.ConsecutiveOver != 180 {
		t.Fatalf("consecutive over = %d, want 180", res.ConsecutiveOver)
	}
	if !res.Complete || res.CoveredCount != 61 {
		t.Fatalf("coverage = %+v, want complete 61", res)
	}
}

// TestGridRejectsMissingSample asserts a short batch is rejected.
func TestGridRejectsMissingSample(t *testing.T) {
	g := NewGrid(window())
	if d := g.ValidateCells(0, "rec", cells(40, 40)); d == nil || d.Code != "temperature_missing" {
		t.Fatalf("defect = %v, want temperature_missing", d)
	}
}

// TestGridRejectsDuplicateTimePoint asserts duplicate times are rejected.
func TestGridRejectsDuplicateTimePoint(t *testing.T) {
	g := NewGrid(window())
	dup := append(cells(40, 40), TemperatureCell{RecorderID: "rec", AtSeconds: 0, Celsius: FixedPoint{Value: 40, Scale: 1}})
	if d := g.ValidateCells(0, "rec", dup); d == nil || d.Code != "temperature_duplicate" {
		t.Fatalf("defect = %v, want temperature_duplicate", d)
	}
}

// TestGridRejectsOutOfRange asserts an above-maximum sample is rejected.
func TestGridRejectsOutOfRange(t *testing.T) {
	g := NewGrid(window())
	vals := make([]int64, 61)
	for i := range vals {
		vals[i] = 40
	}
	vals[0] = 999 // way above 6.0 C
	if d := g.ValidateCells(0, "rec", cells(vals...)); d == nil || d.Code != "temperature_above_max" {
		t.Fatalf("defect = %v, want temperature_above_max", d)
	}
}

// TestGridRejectsRecorderMismatch asserts a mismatched recorder is rejected.
func TestGridRejectsRecorderMismatch(t *testing.T) {
	g := NewGrid(window())
	bad := cells(40)
	bad[0].RecorderID = "other-recorder"
	if d := g.ValidateCells(0, "rec", bad); d == nil || d.Code != "recorder_mismatch" {
		t.Fatalf("defect = %v, want recorder_mismatch", d)
	}
}
