package catalog

// MemoryCatalog is an in-memory Catalog backed by fixed maps. It satisfies the
// read-only directory contract and is used by the demo server and tests.
type MemoryCatalog struct {
	farms  map[FarmID]*Farm
	rules  map[string]*RawMilkRules
	people map[PersonID]*Person
}

// NewMemoryCatalog builds a catalog from the supplied maps.
func NewMemoryCatalog(farms map[FarmID]*Farm, rules map[string]*RawMilkRules, people map[PersonID]*Person) *MemoryCatalog {
	return &MemoryCatalog{farms: farms, rules: rules, people: people}
}

// NewFixedCatalog builds a catalog preloaded with the canonical fixtures.
func NewFixedCatalog() *MemoryCatalog {
	return NewMemoryCatalog(
		map[FarmID]*Farm{FixedFarmID: FixedFarm()},
		map[string]*RawMilkRules{FixedRuleVersion: FixedRules()},
		FixedPeople(),
	)
}

func (m *MemoryCatalog) Farm(id FarmID) (*Farm, bool) {
	f, ok := m.farms[id]
	return f, ok
}

func (m *MemoryCatalog) Rules(version string) (*RawMilkRules, bool) {
	r, ok := m.rules[version]
	return r, ok
}

func (m *MemoryCatalog) Person(id PersonID) (*Person, bool) {
	p, ok := m.people[id]
	return p, ok
}

func (m *MemoryCatalog) Qualifies(version string, id PersonID, required ...Role) bool {
	_, rulesOK := m.rules[version]
	if !rulesOK {
		return false
	}
	p, ok := m.people[id]
	if !ok {
		return false
	}
	held := make(map[Role]bool, len(p.Roles))
	for _, r := range p.Roles {
		held[r] = true
	}
	for _, r := range required {
		if !held[r] {
			return false
		}
	}
	return true
}
