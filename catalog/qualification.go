package catalog

// Qualification rules bind a person's held roles to the requirements of a
// rule version. The inspection workflow forbids one person from advancing a
// task across roles that must remain independent (sampler, reviewer,
// rejudger), so the directory exposes a positive "holds every role" check and
// a negative "holds any conflicting role" check.

// HoldsAny reports whether id holds at least one of the supplied roles under
// the version. It is used to reject role overlap: if a person already
// sampled, they must not also review or rejudge the same task.
func HoldsAny(cat Catalog, version string, id PersonID, roles ...Role) bool {
	p, ok := cat.Person(id)
	if !ok {
		return false
	}
	if _, ok := cat.Rules(version); !ok {
		return false
	}
	for _, want := range roles {
		for _, held := range p.Roles {
			if held == want {
				return true
			}
		}
	}
	return false
}

// HoldsEvery reports whether id holds every one of the supplied roles under
// the version, and the version is known. It is a strict counterpart to the
// Catalog.Qualifies check that also verifies role membership explicitly.
func HoldsEvery(cat Catalog, version string, id PersonID, roles ...Role) bool {
	p, ok := cat.Person(id)
	if !ok {
		return false
	}
	if _, ok := cat.Rules(version); !ok {
		return false
	}
	held := make(map[Role]bool, len(p.Roles))
	for _, r := range p.Roles {
		held[r] = true
	}
	for _, want := range roles {
		if !held[want] {
			return false
		}
	}
	return true
}
