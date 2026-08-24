// Package blindcode models the one-time blind-code gate and the three-way
// split-tube matrix that unlink a tank batch from its measured samples.
package blindcode

import (
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/inspection"
)

// BlindCode is the anonymous sample code assigned to a compartment sample.
type BlindCode string

// MappingStatus is the reveal state of a blind-code mapping.
type MappingStatus string

const (
	MappingMapped   MappingStatus = "mapped"   // 已建立映射
	MappingRevealed MappingStatus = "revealed" // 已揭盲
)

// SplitTube is one tube of the three-way split for a single compartment.
type SplitTube struct {
	TubeSeq     int                     `json:"tubeSeq"` // 1..3
	BlindCode   BlindCode               `json:"blindCode"`
	Compartment catalog.CompartmentCode `json:"compartment"`
}

// BlindSample records the one-time tank-batch -> blind-code mapping gate.
type BlindSample struct {
	TaskID           inspection.TaskID       `json:"taskId"`
	TankBatch        inspection.TankBatch    `json:"tankBatch"`
	Compartment      catalog.CompartmentCode `json:"compartment"`
	BlindCode        BlindCode               `json:"blindCode"`
	MappingStatus    MappingStatus           `json:"mappingStatus"`
	RevealGeneration int64                   `json:"revealGeneration,omitempty"`
}

// Gate is the one-time mapping gate. A (tankBatch, compartment) pair may map
// to exactly one blind code, and each blind code may be used exactly once.
type Gate interface {
	// Establish binds batch/compartment to code exactly once.
	Establish(batch inspection.TankBatch, comp catalog.CompartmentCode, code BlindCode) error
	// Reveal permits unveiling only under an allowed status and generation.
	Reveal(code BlindCode, generation int64) error
	// Code resolves the blind code for a batch/compartment pair.
	Code(batch inspection.TankBatch, comp catalog.CompartmentCode) (BlindCode, bool)
}
