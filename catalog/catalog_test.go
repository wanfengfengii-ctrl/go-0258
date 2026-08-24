package catalog

import "testing"

func TestFixedCatalogQualifies(t *testing.T) {
	c := NewFixedCatalog()
	if !c.Qualifies(FixedRuleVersion, FixedSamplerA, RoleSampler) {
		t.Fatal("sampler A should hold sampler role")
	}
	if c.Qualifies(FixedRuleVersion, FixedSamplerA, RoleReviewer) {
		t.Fatal("sampler A must not hold reviewer role")
	}
	if c.Qualifies("unknown-version", FixedSamplerA, RoleSampler) {
		t.Fatal("unknown rule version must not qualify")
	}
}

func TestFixedCatalogRules(t *testing.T) {
	c := NewFixedCatalog()
	rules, ok := c.Rules(FixedRuleVersion)
	if !ok {
		t.Fatal("fixed rules should exist")
	}
	if rules.RequiredSamplers != 2 || rules.RequiredReviewers != 2 {
		t.Fatalf("unexpected reviewer/sampler counts: %+v", rules)
	}
	if rules.Antibiotic.InhibitionZoneMM != 180 {
		t.Fatalf("inhibition zone = %d, want 180", rules.Antibiotic.InhibitionZoneMM)
	}
}

func TestFixedCatalogFarm(t *testing.T) {
	c := NewFixedCatalog()
	farm, ok := c.Farm(FixedFarmID)
	if !ok {
		t.Fatal("fixed farm should exist")
	}
	if len(farm.Compartments) != 2 {
		t.Fatalf("compartments = %d, want 2", len(farm.Compartments))
	}
}
