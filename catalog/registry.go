package catalog

import "fmt"

// Registry builds and validates a rule directory. It is the single place where
// farms, rule versions and people are assembled into a Catalog, and where
// threshold bounds are checked so a malformed rule snapshot never reaches an
// inspection.

// RuleViolation describes a threshold bound that is physically impossible or
// inconsistent.
type RuleViolation struct {
	Version string `json:"version"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (v *RuleViolation) Error() string {
	return fmt.Sprintf("rule %s %s: %s", v.Version, v.Field, v.Message)
}

// ValidateRules checks the threshold bounds of a rule snapshot. It returns the
// first violation, or nil when every bound is consistent.
func ValidateRules(r *RawMilkRules) *RuleViolation {
	if r == nil {
		return &RuleViolation{Field: "rules", Message: "nil rules"}
	}
	if r.Version == "" {
		return &RuleViolation{Field: "version", Message: "empty version"}
	}
	if r.Antibiotic.InhibitionZoneMM <= 0 {
		return &RuleViolation{Version: r.Version, Field: "antibiotic", Message: "inhibition zone must be positive"}
	}
	if r.Microbial.SomaticCells <= 0 || r.Microbial.ColonyCount <= 0 {
		return &RuleViolation{Version: r.Version, Field: "microbial", Message: "microbial limits must be positive"}
	}
	if r.Physicochemical.FatMin <= 0 || r.Physicochemical.ProteinMin <= 0 {
		return &RuleViolation{Version: r.Version, Field: "physicochemical", Message: "fat and protein lower bounds must be positive"}
	}
	if r.Temperature.MaxCelsius <= r.Temperature.MinCelsius {
		return &RuleViolation{Version: r.Version, Field: "temperature", Message: "max must exceed min"}
	}
	if r.Temperature.WindowSeconds <= 0 || r.Temperature.SampleEverySeconds <= 0 {
		return &RuleViolation{Version: r.Version, Field: "temperature", Message: "window and spacing must be positive"}
	}
	if r.Temperature.MaxConsecutiveOverSeconds <= 0 {
		return &RuleViolation{Version: r.Version, Field: "temperature", Message: "max consecutive over must be positive"}
	}
	if r.RequiredSamplers <= 0 || r.RequiredReviewers <= 0 {
		return &RuleViolation{Version: r.Version, Field: "staffing", Message: "sampler and reviewer counts must be positive"}
	}
	return nil
}

// BuildCatalog assembles a Catalog from farms, rules and people, validating
// every rule version first. It returns an error naming the first bad rule.
func BuildCatalog(farms map[FarmID]*Farm, rules map[string]*RawMilkRules, people map[PersonID]*Person) (*MemoryCatalog, error) {
	for version, r := range rules {
		if v := ValidateRules(r); v != nil {
			return nil, v
		}
		_ = version
	}
	// Every farm must reference a known rule version.
	for _, f := range farms {
		if f.RuleVersion != "" {
			if _, ok := rules[f.RuleVersion]; !ok {
				return nil, &RuleViolation{Version: f.RuleVersion, Field: "farm", Message: "farm " + string(f.ID) + " references unknown rule version"}
			}
		}
	}
	return NewMemoryCatalog(farms, rules, people), nil
}
