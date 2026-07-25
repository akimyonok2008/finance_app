package benchmark

import (
	"errors"
	"testing"
	"time"
)

func mustStore(t *testing.T) *VersionedRecipeStore {
	t.Helper()
	store, err := DefaultVersionStore(Recipes)
	if err != nil {
		t.Fatalf("DefaultVersionStore: %v", err)
	}
	return store
}

func TestVersionSelection_NoLookAhead(t *testing.T) {
	store := mustStore(t)

	// Before the first filing is public: no version exists.
	if _, err := store.SelectVersion("BUFFETT_13F", mustTime("2024-06-01")); !errors.Is(err, ErrRecipeVersionUnavailable) {
		t.Fatalf("expected unavailable before first filing, got %v", err)
	}

	// After 2025 filing, before 2026 filing: 2025Q1 version.
	v, err := store.SelectVersion("BUFFETT_13F", mustTime("2025-08-01"))
	if err != nil {
		t.Fatalf("select 2025: %v", err)
	}
	if v.VersionID != "BUFFETT_13F_2025Q1" {
		t.Fatalf("expected 2025Q1, got %s", v.VersionID)
	}

	// On/after the 2026 filing date: 2026Q1 version.
	v, err = store.SelectVersion("BUFFETT_13F", mustTime("2026-06-01"))
	if err != nil {
		t.Fatalf("select 2026: %v", err)
	}
	if v.VersionID != "BUFFETT_13F_2026Q1" {
		t.Fatalf("expected 2026Q1, got %s", v.VersionID)
	}
}

func TestVersionSelection_BoundaryIsFilingDate(t *testing.T) {
	store := mustStore(t)
	// Exactly on the 2026 filing date the newer version becomes selectable.
	v, err := store.SelectVersion("BUFFETT_13F", mustTime("2026-05-15"))
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if v.VersionID != "BUFFETT_13F_2026Q1" {
		t.Fatalf("expected 2026Q1 on filing date, got %s", v.VersionID)
	}
	// One day earlier still the older version.
	v, _ = store.SelectVersion("BUFFETT_13F", mustTime("2026-05-14"))
	if v.VersionID != "BUFFETT_13F_2025Q1" {
		t.Fatalf("expected 2025Q1 the day before filing, got %s", v.VersionID)
	}
}

func TestVersionSelection_CompositionDiffersAcrossVersions(t *testing.T) {
	store := mustStore(t)
	older := store.Versions("BUFFETT_13F")[0]
	newer := store.Versions("BUFFETT_13F")[1]
	has := func(v BenchmarkRecipeVersion, sym string) bool {
		for _, c := range v.Components {
			if c.Symbol == sym {
				return true
			}
		}
		return false
	}
	if has(older, "GOOGL") {
		t.Fatal("2025Q1 must not contain GOOGL (Alphabet not yet held)")
	}
	if !has(newer, "GOOGL") {
		t.Fatal("2026Q1 must contain GOOGL")
	}
}

func TestStaticRecipesHaveVersions(t *testing.T) {
	store := mustStore(t)
	for _, id := range []string{"SPY", "BALANCED_60_40", "ALL_WEATHER", "SOROS_MACRO", "CASH"} {
		v, err := store.SelectVersion(id, time.Now().UTC())
		if err != nil {
			t.Fatalf("static recipe %s has no version: %v", id, err)
		}
		if v.VersionID != id+"_v1" {
			t.Fatalf("expected %s_v1, got %s", id, v.VersionID)
		}
	}
}

func TestStore_RejectsOverlappingEffectiveRanges(t *testing.T) {
	until := mustTime("2025-06-01")
	overlap := mustTime("2025-05-01")
	_, err := NewVersionedRecipeStore([]BenchmarkRecipeVersion{
		{RecipeID: "X", VersionID: "X_a", PubliclyKnownAt: mustTime("2025-01-01"),
			EffectiveFrom: mustTime("2025-01-01"), EffectiveUntil: &until,
			Components: []AssetAllocation{{Symbol: "SPY", Weight: 1}}, SourceType: "static_model"},
		{RecipeID: "X", VersionID: "X_b", PubliclyKnownAt: mustTime("2025-04-01"),
			EffectiveFrom: overlap, // starts before the previous version ends
			Components:    []AssetAllocation{{Symbol: "SPY", Weight: 1}}, SourceType: "static_model"},
	})
	if err == nil {
		t.Fatal("expected overlapping effective ranges to be rejected")
	}
}

func TestVersion_RejectsInvalidSourceMetadata(t *testing.T) {
	rpe := mustTime("2026-03-31")
	cov := 0.5 // below MinMappingCoverage
	v := BenchmarkRecipeVersion{
		RecipeID: "BUFFETT_13F", VersionID: "bad", PubliclyKnownAt: mustTime("2026-05-15"),
		EffectiveFrom: mustTime("2026-05-15"), ReportPeriodEnd: &rpe,
		Components: []AssetAllocation{{Symbol: "AAPL", Weight: 1}},
		SourceType: "sec_13f_hr",
		SourceURL:  "https://sec.gov/x", SourceAccession: "0001-25-1",
		MappingCoverage: &cov,
	}
	if err := v.Validate(); !errors.Is(err, ErrRecipeMappingCoverage) {
		t.Fatalf("expected coverage rejection, got %v", err)
	}
}

func TestVersion_RejectsMissingAccession(t *testing.T) {
	rpe := mustTime("2026-03-31")
	cov := 0.95
	v := BenchmarkRecipeVersion{
		RecipeID: "BUFFETT_13F", VersionID: "bad", PubliclyKnownAt: mustTime("2026-05-15"),
		EffectiveFrom: mustTime("2026-05-15"), ReportPeriodEnd: &rpe,
		Components: []AssetAllocation{{Symbol: "AAPL", Weight: 1}},
		SourceType: "sec_13f_hr", MappingCoverage: &cov, // no URL/accession
	}
	if err := v.Validate(); err == nil {
		t.Fatal("expected rejection for 13F version without accession/url")
	}
}

func TestBerkshireVersionsMeetCoverageThreshold(t *testing.T) {
	for _, v := range berkshireVersions() {
		if err := v.Validate(); err != nil {
			t.Fatalf("berkshire version %s invalid: %v", v.VersionID, err)
		}
		if v.MappingCoverage == nil || *v.MappingCoverage < MinMappingCoverage {
			t.Fatalf("berkshire version %s coverage below threshold", v.VersionID)
		}
		if v.SourceAccession == "" {
			t.Fatalf("berkshire version %s missing accession", v.VersionID)
		}
	}
}
