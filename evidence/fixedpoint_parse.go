package evidence

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseFixedPoint converts a decimal string into a FixedPoint at the given
// scale. It rejects malformed input, out-of-range scales, fraction digits
// beyond the requested scale, and integer overflow. This is the only entry
// point for turning raw instrument text into fixed-point values, so every
// reading path shares one deterministic parser.
func ParseFixedPoint(text string, scale int) (FixedPoint, error) {
	if scale < 0 || scale > 9 {
		return FixedPoint{}, ErrScale
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return FixedPoint{}, fmt.Errorf("fixed point: empty value")
	}
	neg := false
	switch text[0] {
	case '-':
		neg = true
		text = text[1:]
	case '+':
		text = text[1:]
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
	if len(fracPart) > scale {
		return FixedPoint{}, fmt.Errorf("fixed point: fraction %q exceeds scale %d", fracPart, scale)
	}
	whole, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return FixedPoint{}, fmt.Errorf("fixed point: invalid integer part %q", intPart)
	}
	mult := int64(1)
	for i := 0; i < scale; i++ {
		mult *= 10
	}
	value, ok := mulChecked(whole, mult)
	if !ok {
		return FixedPoint{}, ErrOverflow
	}
	fracMult := mult
	for i := 0; i < len(fracPart); i++ {
		fracMult /= 10
		d := fracPart[i]
		if d < '0' || d > '9' {
			return FixedPoint{}, fmt.Errorf("fixed point: invalid fraction digit %q", string(d))
		}
		value += int64(d-'0') * fracMult
	}
	if neg {
		value = -value
	}
	return FixedPoint{Value: value, Scale: scale}, nil
}

// FormatFixedPoint renders a FixedPoint as a decimal string with its scale's
// fraction digits, including a leading sign for negative values.
func FormatFixedPoint(f FixedPoint) string {
	return f.String()
}
