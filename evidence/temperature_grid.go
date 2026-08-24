package evidence

import (
	"sort"
	"strconv"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
)

// TemperatureGrid validates a batch of temperature samples against the locked
// sampling window and computes the coverage plus the consecutive over-limit
// minutes using deterministic integer arithmetic. It accepts only samples
// inside the window, at unique time points, within the legal range; any
// missing, duplicated or out-of-range sample rejects the whole batch so no
// partial coverage is ever persisted.

// TempDefect describes a specific cold-chain validation failure.
type TempDefect struct {
	Code    string `json:"code"`
	At      int64  `json:"at,omitempty"`
	Message string `json:"message"`
}

// TempResult is the validated outcome of a cold-chain reading batch.
type TempResult struct {
	Cells            []TemperatureCell `json:"cells"`
	CoveredCount     int               `json:"coveredCount"`
	ExpectedCount    int               `json:"expectedCount"`
	ConsecutiveOver  int64             `json:"consecutiveOverSeconds"`
	Complete         bool              `json:"complete"`
	SummaryFragment  string            `json:"summaryFragment"`
	MaxObserved      FixedPoint        `json:"maxObserved"`
	MinObserved      FixedPoint        `json:"minObserved"`
	RecorderConflict bool              `json:"recorderConflict"`
}

// Grid validates and computes coverage for a temperature batch.
type Grid struct {
	window catalog.TemperatureWindow
}

// NewGrid builds a grid from a locked temperature window.
func NewGrid(window catalog.TemperatureWindow) *Grid {
	return &Grid{window: window}
}

// windowStart returns the locked window start (offset 0) for a task's
// sampling; callers supply the window base time.
type windowBounds struct {
	startAt int64
	endAt   int64
}

// ValidateCells checks the batch against the window anchored at baseTime. It
// returns the first defect, or nil when the batch is consistent and fully
// covers the window.
func (g *Grid) ValidateCells(baseTime int64, recorderID string, cells []TemperatureCell) *TempDefect {
	window := g.window
	bounds := windowBounds{startAt: baseTime, endAt: baseTime + window.WindowSeconds}

	if len(cells) == 0 {
		return &TempDefect{Code: "temperature_missing", Message: "no temperature samples"}
	}
	seen := make(map[int64]bool, len(cells))
	for _, c := range cells {
		if c.RecorderID != recorderID {
			return &TempDefect{Code: "recorder_mismatch", At: c.AtSeconds, Message: "recorder id mismatch"}
		}
		if c.AtSeconds < bounds.startAt || c.AtSeconds > bounds.endAt {
			return &TempDefect{Code: "temperature_out_of_window", At: c.AtSeconds, Message: "sample outside locked window"}
		}
		if seen[c.AtSeconds] {
			return &TempDefect{Code: "temperature_duplicate", At: c.AtSeconds, Message: "duplicate time point"}
		}
		seen[c.AtSeconds] = true
		cmp, err := c.Celsius.Cmp(FixedPoint{Value: window.MaxCelsius, Scale: window.Scale})
		if err != nil {
			return &TempDefect{Code: "temperature_compare", At: c.AtSeconds, Message: err.Error()}
		}
		if cmp > 0 {
			return &TempDefect{Code: "temperature_above_max", At: c.AtSeconds, Message: "temperature above maximum"}
		}
		cmp, err = c.Celsius.Cmp(FixedPoint{Value: window.MinCelsius, Scale: window.Scale})
		if err != nil {
			return &TempDefect{Code: "temperature_compare", At: c.AtSeconds, Message: err.Error()}
		}
		if cmp < 0 {
			return &TempDefect{Code: "temperature_below_min", At: c.AtSeconds, Message: "temperature below minimum"}
		}
	}

	expected := int(window.WindowSeconds/window.SampleEverySeconds) + 1
	if len(cells) != expected {
		return &TempDefect{Code: "temperature_missing", Message: "expected " + itoa(expected) + " samples, got " + itoa(len(cells))}
	}
	return nil
}

// Compute builds the full TempResult for a validated batch, including the
// consecutive over-limit minute count. It assumes ValidateCells already
// passed.
func (g *Grid) Compute(baseTime int64, recorderID string, cells []TemperatureCell) *TempResult {
	sort.Slice(cells, func(i, j int) bool { return cells[i].AtSeconds < cells[j].AtSeconds })
	window := g.window
	maxC := FixedPoint{Value: window.MaxCelsius, Scale: window.Scale}

	consecutive := int64(0)
	run := int64(0)
	for i, c := range cells {
		over := false
		if cmp, err := c.Celsius.Cmp(maxC); err == nil && cmp > 0 {
			over = true
		}
		step := int64(0)
		if i > 0 {
			step = c.AtSeconds - cells[i-1].AtSeconds
		} else {
			step = window.SampleEverySeconds
		}
		if over {
			run += step
			if run > consecutive {
				consecutive = run
			}
		} else {
			run = 0
		}
	}

	res := &TempResult{
		Cells:           cells,
		CoveredCount:    len(cells),
		ExpectedCount:   int(window.WindowSeconds/window.SampleEverySeconds) + 1,
		ConsecutiveOver: consecutive,
		Complete:        len(cells) == int(window.WindowSeconds/window.SampleEverySeconds)+1,
	}
	res.SummaryFragment = summarizeCells(cells)
	if len(cells) > 0 {
		res.MaxObserved = cells[0].Celsius
		res.MinObserved = cells[0].Celsius
		for _, c := range cells {
			if cmp, err := c.Celsius.Cmp(res.MaxObserved); err == nil && cmp > 0 {
				res.MaxObserved = c.Celsius
			}
			if cmp, err := c.Celsius.Cmp(res.MinObserved); err == nil && cmp < 0 {
				res.MinObserved = c.Celsius
			}
		}
	}
	return res
}

func summarizeCells(cells []TemperatureCell) string {
	if len(cells) == 0 {
		return ""
	}
	s := cells[0].Celsius.String()
	for _, c := range cells[1:] {
		s += "," + c.Celsius.String()
	}
	return s
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
