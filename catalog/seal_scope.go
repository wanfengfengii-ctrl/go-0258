package catalog

// Seal scope validation binds a task's requested seals to the compartments
// they are allowed to protect. A task may only reference seals that exist in
// the farm's scope, and the union of the requested seals must cover every
// requested compartment; duplicate seals are rejected.

// SealScopeViolation describes a specific seal-scope problem with a stable
// code used across the API surface.
type SealScopeViolation struct {
	Code    string   `json:"code"`
	Seal    SealCode `json:"seal,omitempty"`
	Message string   `json:"message"`
}

// ValidateSealScope checks the requested seals against the farm's seal scope
// and the requested compartments. It returns the first violation, or nil when
// the seal set is consistent and collectively covers every compartment.
func ValidateSealScope(farm *Farm, compartments []CompartmentCode, seals []SealCode) *SealScopeViolation {
	if farm == nil {
		return &SealScopeViolation{Code: "unknown_farm", Message: "farm is required"}
	}
	scope := make(map[SealCode]map[CompartmentCode]bool, len(farm.Seals))
	for _, s := range farm.Seals {
		covers := make(map[CompartmentCode]bool, len(s.Compartments))
		for _, c := range s.Compartments {
			covers[c] = true
		}
		scope[s.Code] = covers
	}

	requested := make(map[CompartmentCode]bool, len(compartments))
	for _, c := range compartments {
		if !farmHasCompartment(farm, c) {
			return &SealScopeViolation{Code: "unknown_compartment", Message: "compartment " + string(c) + " not in farm"}
		}
		requested[c] = true
	}

	seen := make(map[SealCode]bool, len(seals))
	covered := make(map[CompartmentCode]bool, len(compartments))
	for _, seal := range seals {
		if seen[seal] {
			return &SealScopeViolation{Code: "duplicate_seal", Seal: seal, Message: "duplicate seal " + string(seal)}
		}
		seen[seal] = true
		covers, ok := scope[seal]
		if !ok {
			return &SealScopeViolation{Code: "unknown_seal", Seal: seal, Message: "seal " + string(seal) + " not in farm scope"}
		}
		for c := range covers {
			covered[c] = true
		}
	}

	for c := range requested {
		if !covered[c] {
			return &SealScopeViolation{Code: "seal_scope_gap", Message: "no seal covers compartment " + string(c)}
		}
	}
	return nil
}

func farmHasCompartment(farm *Farm, c CompartmentCode) bool {
	for _, comp := range farm.Compartments {
		if comp.Code == c {
			return true
		}
	}
	return false
}

// CompartmentCodes returns the ordered compartment codes of a farm.
func CompartmentCodes(farm *Farm) []CompartmentCode {
	if farm == nil {
		return nil
	}
	out := make([]CompartmentCode, len(farm.Compartments))
	for i, c := range farm.Compartments {
		out[i] = c.Code
	}
	return out
}
