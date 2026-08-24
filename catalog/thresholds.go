package catalog

// RawMilkRules is a versioned snapshot of every threshold the inspection
// freezes at build time. All numeric thresholds are fixed-point integers;
// each carries the Scale (number of decimal places) it is expressed in.
type RawMilkRules struct {
	Version           string                    `json:"version"`
	Summary           string                    `json:"summary"`
	Antibiotic        AntibioticThresholds      `json:"antibiotic"`
	Microbial         MicrobialThresholds       `json:"microbial"`
	Physicochemical   PhysicochemicalThresholds `json:"physicochemical"`
	Temperature       TemperatureWindow         `json:"temperature"`
	RequiredSamplers  int                       `json:"requiredSamplers"`
	RequiredReviewers int                       `json:"requiredReviewers"`
}

// AntibioticThresholds describes the inhibition-zone pass/fail boundary.
type AntibioticThresholds struct {
	// InhibitionZoneMM is the minimum inhibition ring in fixed point (scale 1).
	InhibitionZoneMM int64 `json:"inhibitionZoneMm"`
	Scale            int   `json:"scale"`
}

// MicrobialThresholds describes somatic-cell and colony-count limits.
type MicrobialThresholds struct {
	// SomaticCells is the somatic cell count limit (scale 3).
	SomaticCells int64 `json:"somaticCells"`
	// ColonyCount is the total plate colony count limit (scale 0).
	ColonyCount  int64 `json:"colonyCount"`
	SomaticScale int   `json:"somaticScale"`
	ColonyScale  int   `json:"colonyScale"`
}

// PhysicochemicalThresholds describes freezing-point, fat and protein limits.
type PhysicochemicalThresholds struct {
	// FreezingPointMax is the upper freezing-point bound in m°C (scale 1).
	FreezingPointMax int64 `json:"freezingPointMax"`
	// FatMin and ProteinMin are lower bounds (scale 1).
	FatMin     int64 `json:"fatMin"`
	ProteinMin int64 `json:"proteinMin"`
	Scale      int   `json:"scale"`
}

// TemperatureWindow is the locked cold-chain sampling window.
type TemperatureWindow struct {
	// SampleEverySeconds is the fixed spacing between temperature samples.
	SampleEverySeconds int64 `json:"sampleEverySeconds"`
	// WindowSeconds is the total locked window length.
	WindowSeconds int64 `json:"windowSeconds"`
	// MaxCelsius is the upper legal temperature bound (scale 1).
	MaxCelsius int64 `json:"maxCelsius"`
	// MinCelsius is the lower legal temperature bound (scale 1).
	MinCelsius int64 `json:"minCelsius"`
	// MaxConsecutiveOverSeconds before a cold-chain break is flagged.
	MaxConsecutiveOverSeconds int64 `json:"maxConsecutiveOverSeconds"`
	Scale                     int   `json:"scale"`
}
