package evidence

import (
	"errors"
	"fmt"
	"math"
)

// FixedPoint is a fixed-point decimal: an integer Value scaled by 10^Scale.
// All arithmetic checks length, sign, division-by-zero and overflow and never
// silently truncates.
type FixedPoint struct {
	Value int64 `json:"value"`
	Scale int   `json:"scale"`
}

var (
	ErrDivideByZero = errors.New("fixed point: division by zero")
	ErrOverflow     = errors.New("fixed point: overflow")
	ErrScale        = errors.New("fixed point: scale out of range")
)

// New builds a FixedPoint after validating scale bounds.
func New(value int64, scale int) (FixedPoint, error) {
	if scale < 0 || scale > 9 {
		return FixedPoint{}, ErrScale
	}
	return FixedPoint{Value: value, Scale: scale}, nil
}

// rescale aligns two fixed points to the larger scale.
func (f FixedPoint) rescale(o FixedPoint) (a, b int64, scale int, err error) {
	scale = f.Scale
	if o.Scale > scale {
		scale = o.Scale
	}
	a, err = f.toScale(scale)
	if err != nil {
		return 0, 0, 0, err
	}
	b, err = o.toScale(scale)
	if err != nil {
		return 0, 0, 0, err
	}
	return a, b, scale, nil
}

// toScale converts the value to the target scale, checking overflow.
func (f FixedPoint) toScale(scale int) (int64, error) {
	if scale < f.Scale {
		return 0, ErrScale
	}
	diff := scale - f.Scale
	if diff > 9 {
		return 0, ErrScale
	}
	mult := int64(1)
	for i := 0; i < diff; i++ {
		mult *= 10
	}
	v, ok := mulChecked(f.Value, mult)
	if !ok {
		return 0, ErrOverflow
	}
	return v, nil
}

// Add returns f+o aligned to the larger scale.
func (f FixedPoint) Add(o FixedPoint) (FixedPoint, error) {
	a, b, scale, err := f.rescale(o)
	if err != nil {
		return FixedPoint{}, err
	}
	v, ok := addChecked(a, b)
	if !ok {
		return FixedPoint{}, ErrOverflow
	}
	return FixedPoint{Value: v, Scale: scale}, nil
}

// Sub returns f-o aligned to the larger scale.
func (f FixedPoint) Sub(o FixedPoint) (FixedPoint, error) {
	a, b, scale, err := f.rescale(o)
	if err != nil {
		return FixedPoint{}, err
	}
	v, ok := subChecked(a, b)
	if !ok {
		return FixedPoint{}, ErrOverflow
	}
	return FixedPoint{Value: v, Scale: scale}, nil
}

// Mul returns f*o with scales summed.
func (f FixedPoint) Mul(o FixedPoint) (FixedPoint, error) {
	v, ok := mulChecked(f.Value, o.Value)
	if !ok {
		return FixedPoint{}, ErrOverflow
	}
	return FixedPoint{Value: v, Scale: f.Scale + o.Scale}, nil
}

// Div returns f/o. The quotient keeps f's scale; o must not be zero.
func (f FixedPoint) Div(o FixedPoint) (FixedPoint, error) {
	if o.Value == 0 {
		return FixedPoint{}, ErrDivideByZero
	}
	diff := f.Scale - o.Scale
	numer := f.Value
	for i := 0; i < diff; i++ {
		v, ok := mulChecked(numer, 10)
		if !ok {
			return FixedPoint{}, ErrOverflow
		}
		numer = v
	}
	return FixedPoint{Value: numer / o.Value, Scale: f.Scale}, nil
}

// Cmp compares f and o after alignment; returns -1, 0 or 1.
func (f FixedPoint) Cmp(o FixedPoint) (int, error) {
	a, b, _, err := f.rescale(o)
	if err != nil {
		return 0, err
	}
	switch {
	case a < b:
		return -1, nil
	case a > b:
		return 1, nil
	default:
		return 0, nil
	}
}

func (f FixedPoint) String() string {
	if f.Scale == 0 {
		return fmt.Sprintf("%d", f.Value)
	}
	return fmt.Sprintf("%d.%0*d", f.Value/int64Pow10(f.Scale), f.Scale, absInt64(f.Value)%int64Pow10(f.Scale))
}

func int64Pow10(n int) int64 {
	v := int64(1)
	for i := 0; i < n; i++ {
		v *= 10
	}
	return v
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func addChecked(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}

func subChecked(a, b int64) (int64, bool) {
	if (b < 0 && a > math.MaxInt64+b) || (b > 0 && a < math.MinInt64+b) {
		return 0, false
	}
	return a - b, true
}

func mulChecked(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	c := a * b
	if c/b != a {
		return 0, false
	}
	return c, true
}
