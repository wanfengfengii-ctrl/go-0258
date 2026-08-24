package catalog

import (
	"fmt"
	"strconv"
	"strings"
)

// ThresholdParser converts a compact, human-authored threshold specification
// into a RawMilkRules snapshot. It is the only place where threshold text is
// interpreted, so rule versions loaded from configuration or fixtures share a
// single, deterministic grammar. All numeric thresholds are fixed-point and
// are parsed with explicit scale and overflow checks.

// ParseThresholdError records the position and reason a threshold string
// failed to parse.
type ParseThresholdError struct {
	Field string `json:"field"`
	Text  string `json:"text"`
	Err   string `json:"err"`
}

func (e *ParseThresholdError) Error() string {
	return fmt.Sprintf("threshold parse %s=%q: %s", e.Field, e.Text, e.Err)
}

// ThresholdParser holds an optional named set of threshold fields keyed by
// their canonical names. Parsing is positional within the fixed grammar.
type ThresholdParser struct {
	scale int // default fixed-point scale for decimal values without one
}

// NewThresholdParser builds a parser with the given default scale.
func NewThresholdParser(defaultScale int) *ThresholdParser {
	if defaultScale < 0 {
		defaultScale = 0
	}
	return &ThresholdParser{scale: defaultScale}
}

// parseFixed parses a decimal string into a scaled fixed-point integer. The
// input may carry an explicit decimal point; the result is value*10^scale.
func (p *ThresholdParser) parseFixed(text string) (int64, error) {
	text = strings.TrimSpace(text)
	neg := false
	if strings.HasPrefix(text, "-") {
		neg = true
		text = text[1:]
	} else if strings.HasPrefix(text, "+") {
		text = text[1:]
	}
	if text == "" {
		return 0, fmt.Errorf("empty value")
	}
	intPart := text
	fracPart := ""
	if dot := strings.IndexByte(text, '.'); dot >= 0 {
		intPart = text[:dot]
		fracPart = text[dot+1:]
	}
	if intPart == "" {
		intPart = "0"
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer part %q", intPart)
	}
	if len(fracPart) > p.scale {
		return 0, fmt.Errorf("fraction %q exceeds scale %d", fracPart, p.scale)
	}
	scale := int64(1)
	for i := 0; i < p.scale; i++ {
		scale *= 10
	}
	if whole > 0 && (whole > (1<<63-1)/scale) {
		return 0, fmt.Errorf("overflow")
	}
	value := whole * scale
	fracScale := scale
	for i := 0; i < len(fracPart); i++ {
		fracScale /= 10
		if fracPart[i] < '0' || fracPart[i] > '9' {
			return 0, fmt.Errorf("invalid fraction digit %q", string(fracPart[i]))
		}
		value += int64(fracPart[i]-'0') * fracScale
	}
	if neg {
		value = -value
	}
	return value, nil
}

// ParseRules parses a full rule spec map into a RawMilkRules. Unknown fields
// are ignored so that forward-compatible specs remain loadable. A missing
// mandatory field yields a ParseThresholdError.
func ParseRules(version, summary string, scale int, fields map[string]string) (*RawMilkRules, error) {
	p := NewThresholdParser(scale)
	rules := &RawMilkRules{
		Version: version,
		Summary: summary,
	}

	parse := func(name string, dst *int64) error {
		text, ok := fields[name]
		if !ok {
			return &ParseThresholdError{Field: name, Err: "missing"}
		}
		v, err := p.parseFixed(text)
		if err != nil {
			return &ParseThresholdError{Field: name, Text: text, Err: err.Error()}
		}
		*dst = v
		return nil
	}

	anti := AntibioticThresholds{Scale: scale}
	if err := parse("antibiotic", &anti.InhibitionZoneMM); err != nil {
		return nil, err
	}
	rules.Antibiotic = anti

	micro := MicrobialThresholds{SomaticScale: scale, ColonyScale: 0}
	if err := parse("somatic", &micro.SomaticCells); err != nil {
		return nil, err
	}
	if err := parse("colony", &micro.ColonyCount); err != nil {
		return nil, err
	}
	rules.Microbial = micro

	phys := PhysicochemicalThresholds{Scale: scale}
	if err := parse("freezing_point", &phys.FreezingPointMax); err != nil {
		return nil, err
	}
	if err := parse("fat", &phys.FatMin); err != nil {
		return nil, err
	}
	if err := parse("protein", &phys.ProteinMin); err != nil {
		return nil, err
	}
	rules.Physicochemical = phys

	win := TemperatureWindow{Scale: scale}
	if err := parse("sample_every", &win.SampleEverySeconds); err != nil {
		return nil, err
	}
	if err := parse("window", &win.WindowSeconds); err != nil {
		return nil, err
	}
	if err := parse("max_celsius", &win.MaxCelsius); err != nil {
		return nil, err
	}
	if err := parse("min_celsius", &win.MinCelsius); err != nil {
		return nil, err
	}
	if err := parse("max_over", &win.MaxConsecutiveOverSeconds); err != nil {
		return nil, err
	}
	rules.Temperature = win

	if text, ok := fields["samplers"]; ok {
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 0 {
			return nil, &ParseThresholdError{Field: "samplers", Text: text, Err: "invalid count"}
		}
		rules.RequiredSamplers = n
	}
	if text, ok := fields["reviewers"]; ok {
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 0 {
			return nil, &ParseThresholdError{Field: "reviewers", Text: text, Err: "invalid count"}
		}
		rules.RequiredReviewers = n
	}

	return rules, nil
}
