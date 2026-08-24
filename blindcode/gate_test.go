package blindcode

import (
	"errors"
	"testing"
)

func TestMapGateEstablishOnce(t *testing.T) {
	g := NewMapGate(1)
	if err := g.Establish("B-1", "A", "CODE-A"); err != nil {
		t.Fatal(err)
	}
	// Same batch/compartment cannot be remapped.
	if err := g.Establish("B-1", "A", "CODE-B"); !errors.Is(err, ErrAlreadyMapped) {
		t.Fatalf("err = %v, want ErrAlreadyMapped", err)
	}
	// The blind code cannot be reused elsewhere.
	if err := g.Establish("B-1", "B", "CODE-A"); !errors.Is(err, ErrBlindReuse) {
		t.Fatalf("err = %v, want ErrBlindReuse", err)
	}
	code, ok := g.Code("B-1", "A")
	if !ok || code != "CODE-A" {
		t.Fatalf("code = %q ok=%v, want CODE-A", code, ok)
	}
}

func TestMapGateRevealGating(t *testing.T) {
	g := NewMapGate(2)
	_ = g.Establish("B-1", "A", "CODE-A")
	// Not allowed yet.
	if err := g.Reveal("CODE-A", 2); !errors.Is(err, ErrPrematureReveal) {
		t.Fatalf("err = %v, want ErrPrematureReveal", err)
	}
	g.SetAllowed(true)
	// Stale generation rejected.
	if err := g.Reveal("CODE-A", 1); !errors.Is(err, ErrStaleReveal) {
		t.Fatalf("err = %v, want ErrStaleReveal", err)
	}
	if err := g.Reveal("CODE-A", 2); err != nil {
		t.Fatalf("reveal at current generation failed: %v", err)
	}
	if status, _ := g.StatusOf("CODE-A"); status != MappingRevealed {
		t.Fatalf("status = %s, want revealed", status)
	}
}
