// Package catalog holds the farm and raw-milk inspection rule directory.
//
// It is the source of truth for farm identity, tank compartments, seal
// scopes, recorder models, personnel qualifications, instrument capability
// and threshold rule versions. All other packages read this directory; none
// may mutate it outside a versioned rule snapshot.
package catalog

// FarmID identifies a supplying dairy farm.
type FarmID string

// CompartmentCode identifies a tank compartment (罐仓隔舱) inside a farm tank.
type CompartmentCode string

// SealCode identifies a tamper-evident seal bound to a compartment scope.
type SealCode string

// RecorderModel identifies a cold-chain temperature recorder model.
type RecorderModel string

// PersonID identifies a qualified laboratory operator.
type PersonID string

// Role is a qualification role a person may hold for a rule version.
type Role string

const (
	RoleSampler  Role = "sampler"
	RoleReviewer Role = "reviewer"
	RoleRejudger Role = "rejudger"
)

// Farm is a supplying farm with its permitted compartments and seal scope.
type Farm struct {
	ID           FarmID        `json:"id"`
	Name         string        `json:"name"`
	ValidUntil   int64         `json:"validUntil"` // logical clock, seconds
	Compartments []Compartment `json:"compartments"`
	Seals        []SealScope   `json:"seals"`
	RuleVersion  string        `json:"ruleVersion"`
}

// Compartment is a single tank compartment with a fixed capacity.
type Compartment struct {
	Code       CompartmentCode `json:"code"`
	CapacityML int64           `json:"capacityMl"`
}

// SealScope binds a seal code to the compartment codes it protects.
type SealScope struct {
	Code         SealCode          `json:"code"`
	Compartments []CompartmentCode `json:"compartments"`
}

// Person is a laboratory operator with qualifications under a rule version.
type Person struct {
	ID    PersonID `json:"id"`
	Name  string   `json:"name"`
	Roles []Role   `json:"roles"`
}

// Catalog is the read-only directory used to freeze rules at build time.
type Catalog interface {
	// Farm returns the farm by ID, or (nil, false) when unknown.
	Farm(id FarmID) (*Farm, bool)
	// Rules returns the locked raw-milk rules for a version.
	Rules(version string) (*RawMilkRules, bool)
	// Person returns the operator qualifications by ID.
	Person(id PersonID) (*Person, bool)
	// Qualifies reports whether p holds every required role under version.
	Qualifies(version string, id PersonID, required ...Role) bool
}
