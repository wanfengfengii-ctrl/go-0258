package catalog

// FixedFarmID and friends are the deterministic fixture identifiers used by
// the public "plant-entry" test path and the browser demo.
const (
	FixedFarmID      FarmID   = "farm-dairy-001"
	FixedRuleVersion string   = "rules-v2026-1"
	FixedSamplerA    PersonID = "person-sampler-a"
	FixedSamplerB    PersonID = "person-sampler-b"
	FixedReviewerA   PersonID = "person-reviewer-a"
	FixedReviewerB   PersonID = "person-reviewer-b"
)

// FixedFarm builds the canonical fixture farm with two compartments and a
// two-compartment seal scope.
func FixedFarm() *Farm {
	return &Farm{
		ID:          FixedFarmID,
		Name:        "示范牧场 001",
		ValidUntil:  1 << 40,
		RuleVersion: FixedRuleVersion,
		Compartments: []Compartment{
			{Code: "A", CapacityML: 5000},
			{Code: "B", CapacityML: 5000},
		},
		Seals: []SealScope{
			{Code: "seal-0001", Compartments: []CompartmentCode{"A"}},
			{Code: "seal-0002", Compartments: []CompartmentCode{"B"}},
		},
	}
}

// FixedRules builds the canonical locked rule snapshot.
func FixedRules() *RawMilkRules {
	return &RawMilkRules{
		Version: FixedRuleVersion,
		Summary: "抗生素/微生物/理化阈值 v2026.1",
		Antibiotic: AntibioticThresholds{
			InhibitionZoneMM: 180, // 18.0 mm
			Scale:            1,
		},
		Microbial: MicrobialThresholds{
			SomaticCells: 400000,
			ColonyCount:  100000,
			SomaticScale: 3,
			ColonyScale:  0,
		},
		Physicochemical: PhysicochemicalThresholds{
			FreezingPointMax: -512, // -51.2 m°C
			FatMin:           31,
			ProteinMin:       28,
			Scale:            1,
		},
		Temperature: TemperatureWindow{
			SampleEverySeconds:        60,
			WindowSeconds:             3600,
			MaxCelsius:                60,
			MinCelsius:                0,
			MaxConsecutiveOverSeconds: 300,
			Scale:                     1,
		},
		RequiredSamplers:  2,
		RequiredReviewers: 2,
	}
}

// FixedPeople builds the canonical qualified operators.
func FixedPeople() map[PersonID]*Person {
	return map[PersonID]*Person{
		FixedSamplerA:  {ID: FixedSamplerA, Name: "采样甲", Roles: []Role{RoleSampler}},
		FixedSamplerB:  {ID: FixedSamplerB, Name: "采样乙", Roles: []Role{RoleSampler}},
		FixedReviewerA: {ID: FixedReviewerA, Name: "复核甲", Roles: []Role{RoleReviewer}},
		FixedReviewerB: {ID: FixedReviewerB, Name: "复核乙", Roles: []Role{RoleReviewer}},
	}
}
