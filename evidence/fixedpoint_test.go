package evidence

import (
	"errors"
	"testing"
)

func TestFixedPointAddAlignsScales(t *testing.T) {
	a := FixedPoint{Value: 180, Scale: 1} // 18.0
	b := FixedPoint{Value: 15, Scale: 0}  // 15
	got, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Value != 330 || got.Scale != 1 {
		t.Fatalf("got %+v, want value 330 scale 1", got)
	}
}

func TestFixedPointDivByZero(t *testing.T) {
	a := FixedPoint{Value: 10, Scale: 1}
	z := FixedPoint{Value: 0, Scale: 0}
	if _, err := a.Div(z); !errors.Is(err, ErrDivideByZero) {
		t.Fatalf("err = %v, want ErrDivideByZero", err)
	}
}

func TestFixedPointMulOverflow(t *testing.T) {
	a := FixedPoint{Value: 1 << 62, Scale: 0}
	b := FixedPoint{Value: 4, Scale: 0}
	if _, err := a.Mul(b); !errors.Is(err, ErrOverflow) {
		t.Fatalf("err = %v, want ErrOverflow", err)
	}
}

func TestFixedPointCmp(t *testing.T) {
	a := FixedPoint{Value: 5, Scale: 1}  // 0.5
	b := FixedPoint{Value: 50, Scale: 1} // 5.0
	cmp, err := a.Cmp(b)
	if err != nil {
		t.Fatal(err)
	}
	if cmp != -1 {
		t.Fatalf("cmp = %d, want -1", cmp)
	}
}

func TestFixedPointString(t *testing.T) {
	got := (FixedPoint{Value: -512, Scale: 1}).String()
	if got != "-51.2" {
		t.Fatalf("String() = %q, want -51.2", got)
	}
}
