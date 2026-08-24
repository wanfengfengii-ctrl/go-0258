package catalog

import "testing"

func TestValidateSealScopeCollectiveCoverage(t *testing.T) {
	farm := FixedFarm()
	// Two seals each covering one compartment collectively cover both.
	if v := ValidateSealScope(farm, []CompartmentCode{"A", "B"}, []SealCode{"seal-0001", "seal-0002"}); v != nil {
		t.Fatalf("collective coverage rejected: %+v", v)
	}
	// A gap: only seal-0001 requested, compartment B uncovered.
	if v := ValidateSealScope(farm, []CompartmentCode{"A", "B"}, []SealCode{"seal-0001"}); v == nil || v.Code != "seal_scope_gap" {
		t.Fatalf("gap = %+v, want seal_scope_gap", v)
	}
	// Duplicate seal rejected.
	if v := ValidateSealScope(farm, []CompartmentCode{"A"}, []SealCode{"seal-0001", "seal-0001"}); v == nil || v.Code != "duplicate_seal" {
		t.Fatalf("dup = %+v, want duplicate_seal", v)
	}
	// Unknown seal rejected.
	if v := ValidateSealScope(farm, []CompartmentCode{"A"}, []SealCode{"seal-9999"}); v == nil || v.Code != "unknown_seal" {
		t.Fatalf("unknown = %+v, want unknown_seal", v)
	}
}

func TestThresholdParserFixedPoint(t *testing.T) {
	p := NewThresholdParser(1)
	v, err := p.parseFixed("18.0")
	if err != nil || v != 180 {
		t.Fatalf("parse 18.0 = %d, err %v; want 180", v, err)
	}
	v, err = p.parseFixed("-51.2")
	if err != nil || v != -512 {
		t.Fatalf("parse -51.2 = %d, err %v; want -512", v, err)
	}
	if _, err := p.parseFixed("1.234"); err == nil {
		t.Fatal("fraction exceeding scale must fail")
	}
	if _, err := p.parseFixed("abc"); err == nil {
		t.Fatal("non-numeric must fail")
	}
}

func TestQualificationRoles(t *testing.T) {
	c := NewFixedCatalog()
	if !HoldsEvery(c, FixedRuleVersion, FixedSamplerA, RoleSampler) {
		t.Fatal("sampler should hold sampler role")
	}
	if HoldsEvery(c, FixedRuleVersion, FixedSamplerA, RoleReviewer) {
		t.Fatal("sampler must not hold reviewer role")
	}
	if !HoldsAny(c, FixedRuleVersion, FixedReviewerA, RoleReviewer) {
		t.Fatal("reviewer should hold reviewer role")
	}
	if HoldsAny(c, FixedRuleVersion, FixedReviewerA, RoleSampler) {
		t.Fatal("reviewer must not hold sampler role")
	}
}

func TestValidateRulesBounds(t *testing.T) {
	if v := ValidateRules(FixedRules()); v != nil {
		t.Fatalf("fixed rules invalid: %+v", v)
	}
	bad := FixedRules()
	bad.Temperature.MaxCelsius = 0
	bad.Temperature.MinCelsius = 100
	if v := ValidateRules(bad); v == nil || v.Field != "temperature" {
		t.Fatalf("violation = %+v, want temperature", v)
	}
}
