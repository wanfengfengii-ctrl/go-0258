package blindcode

import (
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// MapGate is an in-memory Gate that enforces the one-time mapping invariant:
// each (tankBatch, compartment) pair maps to exactly one blind code, and each
// blind code is used exactly once. Reveal is gated on the current generation
// and an allowed status. The service layers MapGate over the persisted blind
// samples, so the uniqueness rules are testable without SQLite.
type MapGate struct {
	batchToCode map[batchComp]BlindCode
	codeToBatch map[BlindCode]batchComp
	status      map[BlindCode]MappingStatus
	revealGen   map[BlindCode]int64
	allowed     bool
	generation  int64
}

type batchComp struct {
	batch inspection.TankBatch
	comp  catalog.CompartmentCode
}

// NewMapGate builds an empty gate bound to the current generation.
func NewMapGate(generation int64) *MapGate {
	return &MapGate{
		batchToCode: make(map[batchComp]BlindCode),
		codeToBatch: make(map[BlindCode]batchComp),
		status:      make(map[BlindCode]MappingStatus),
		revealGen:   make(map[BlindCode]int64),
		generation:  generation,
	}
}

// SetAllowed controls whether reveal is permitted (the task has advanced far
// enough). It is a pure gate, so the caller owns the status decision.
func (g *MapGate) SetAllowed(allowed bool) { g.allowed = allowed }

// SetGeneration updates the gate's current generation.
func (g *MapGate) SetGeneration(gen int64) { g.generation = gen }

// Establish binds batch/compartment to code exactly once.
func (g *MapGate) Establish(batch inspection.TankBatch, comp catalog.CompartmentCode, code BlindCode) error {
	key := batchComp{batch: batch, comp: comp}
	if _, exists := g.batchToCode[key]; exists {
		return ErrAlreadyMapped
	}
	if _, exists := g.codeToBatch[code]; exists {
		return ErrBlindReuse
	}
	g.batchToCode[key] = code
	g.codeToBatch[code] = key
	g.status[code] = MappingMapped
	return nil
}

// Reveal permits unveiling under the allowed status and current generation.
func (g *MapGate) Reveal(code BlindCode, generation int64) error {
	_, exists := g.codeToBatch[code]
	if !exists {
		return ErrBlindReuse
	}
	if !g.allowed {
		return ErrPrematureReveal
	}
	if generation != g.generation {
		return ErrStaleReveal
	}
	g.status[code] = MappingRevealed
	g.revealGen[code] = generation
	return nil
}

// Code resolves the blind code for a batch/compartment pair.
func (g *MapGate) Code(batch inspection.TankBatch, comp catalog.CompartmentCode) (BlindCode, bool) {
	code, ok := g.batchToCode[batchComp{batch: batch, comp: comp}]
	return code, ok
}

// StatusOf returns the mapping status of a blind code.
func (g *MapGate) StatusOf(code BlindCode) (MappingStatus, bool) {
	s, ok := g.status[code]
	return s, ok
}
