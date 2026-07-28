package benchmark

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeSeriesProvider returns a deterministic adjusted series per symbol and lets
// tests override per-symbol metadata to exercise mixed-quality aggregation.
type fakeSeriesProvider struct {
	returns  map[string]float64
	metaBySy map[string]BenchmarkDataMetadata
	base     BenchmarkDataMetadata
}

func newFakeProvider(returns map[string]float64) *fakeSeriesProvider {
	return &fakeSeriesProvider{
		returns:  returns,
		metaBySy: map[string]BenchmarkDataMetadata{},
		base: BenchmarkDataMetadata{
			Provider: "feed", ProviderMode: "real", PriceType: PriceTypeAdjustedClose,
			IsAdjusted: true, IsTotalReturn: true, IncludesDividends: true, IncludesSplits: true,
			CorpActionsKnown: true, Quality: DataQualityVerified, Currency: "USD",
		},
	}
}

func (f *fakeSeriesProvider) GetAdjustedCloseSeries(_ context.Context, symbol string, start, end time.Time) ([]PricePoint, error) {
	dates := dailyDates(start, end)
	ret := f.returns[symbol]
	out := make([]PricePoint, 0, len(dates))
	for i, d := range dates {
		t := 0.0
		if len(dates) > 1 {
			t = float64(i) / float64(len(dates)-1)
		}
		out = append(out, PricePoint{Date: toDateKey(d), AdjustedClose: 100 * (1 + (ret/100)*t)})
	}
	return out, nil
}

func (f *fakeSeriesProvider) GetSeries(ctx context.Context, symbol string, start, end time.Time, _ SeriesRequirement) (BenchmarkPriceSeries, error) {
	pts, err := f.GetAdjustedCloseSeries(ctx, symbol, start, end)
	if err != nil {
		return BenchmarkPriceSeries{}, err
	}
	meta := f.base
	if m, ok := f.metaBySy[symbol]; ok {
		meta = m
	}
	return BenchmarkPriceSeries{Symbol: symbol, Points: pts, Metadata: meta}, nil
}

func newProvEngine(t *testing.T, p HistoricalPriceProvider) *BenchmarkConstructionService {
	t.Helper()
	e := NewBenchmarkConstructionService(p, Recipes, nil)
	return e
}

func TestCalculateReturn_ReturnsProvenance(t *testing.T) {
	p := newFakeProvider(map[string]float64{"SPY": 10, "BND": 2})
	e := newProvEngine(t, p)
	res, err := e.CalculateReturn(context.Background(), "BALANCED_60_40",
		mustTime("2026-01-02"), mustTime("2026-03-02"), RequirementForAwards())
	if err != nil {
		t.Fatalf("CalculateReturn: %v", err)
	}
	if res.RecipeVersion.VersionID != "BALANCED_60_40_v1" {
		t.Fatalf("expected versioned recipe, got %q", res.RecipeVersion.VersionID)
	}
	if res.DataMetadata.Quality != DataQualityVerified {
		t.Fatalf("expected verified aggregate quality, got %s", res.DataMetadata.Quality)
	}
	if len(res.DataMetadata.Symbols) != 2 {
		t.Fatalf("expected 2 symbols in provenance, got %v", res.DataMetadata.Symbols)
	}
	if res.Fingerprint == "" {
		t.Fatal("fingerprint must be set")
	}
	if res.ReturnPercentage == 0 {
		t.Fatal("expected a non-zero benchmark return")
	}
}

func TestCalculateReturn_MixedQualityDowngradesAndFailsClosed(t *testing.T) {
	p := newFakeProvider(map[string]float64{"SPY": 10, "BND": 2})
	// BND leg is synthetic; the aggregate must be no better than synthetic and an
	// award-grade evaluation must fail closed.
	synthetic := p.base
	synthetic.IsSynthetic = true
	synthetic.Quality = DataQualitySynthetic
	p.metaBySy["BND"] = synthetic

	e := newProvEngine(t, p)
	if _, err := e.CalculateReturn(context.Background(), "BALANCED_60_40",
		mustTime("2026-01-02"), mustTime("2026-03-02"), RequirementForAwards()); !errors.Is(err, ErrSyntheticDataNotAllowed) {
		t.Fatalf("expected synthetic rejection, got %v", err)
	}
	// Under preview the evaluation succeeds but aggregate quality is synthetic.
	res, err := e.CalculateReturn(context.Background(), "BALANCED_60_40",
		mustTime("2026-01-02"), mustTime("2026-03-02"), RequirementForPreview())
	if err != nil {
		t.Fatalf("preview should succeed: %v", err)
	}
	if res.DataMetadata.Quality != DataQualitySynthetic {
		t.Fatalf("aggregate quality should downgrade to synthetic, got %s", res.DataMetadata.Quality)
	}
	if !res.DataMetadata.IsSynthetic {
		t.Fatal("aggregate must be flagged synthetic")
	}
}

func TestFingerprint_DeterministicAndSensitive(t *testing.T) {
	p := newFakeProvider(map[string]float64{"SPY": 10, "BND": 2})
	e := newProvEngine(t, p)
	a, err := e.CalculateReturn(context.Background(), "BALANCED_60_40",
		mustTime("2026-01-02"), mustTime("2026-03-02"), RequirementForAwards())
	if err != nil {
		t.Fatalf("a: %v", err)
	}
	b, err := e.CalculateReturn(context.Background(), "BALANCED_60_40",
		mustTime("2026-01-02"), mustTime("2026-03-02"), RequirementForAwards())
	if err != nil {
		t.Fatalf("b: %v", err)
	}
	if a.Fingerprint != b.Fingerprint {
		t.Fatal("identical inputs must produce identical fingerprints")
	}

	// Changing a price series changes the fingerprint.
	p2 := newFakeProvider(map[string]float64{"SPY": 12, "BND": 2})
	e2 := newProvEngine(t, p2)
	c, err := e2.CalculateReturn(context.Background(), "BALANCED_60_40",
		mustTime("2026-01-02"), mustTime("2026-03-02"), RequirementForAwards())
	if err != nil {
		t.Fatalf("c: %v", err)
	}
	if a.Fingerprint == c.Fingerprint {
		t.Fatal("different price data must produce a different fingerprint")
	}
}

func TestCircularRecipeReferenceRejected(t *testing.T) {
	// A -> B -> A cycle must be detected, not recursed into forever.
	store, err := NewVersionedRecipeStore([]BenchmarkRecipeVersion{
		{RecipeID: "A", VersionID: "A_v1", PubliclyKnownAt: epoch, EffectiveFrom: epoch,
			Components: []AssetAllocation{{RecipeRef: "B", Weight: 1}}, SourceType: "static_model"},
		{RecipeID: "B", VersionID: "B_v1", PubliclyKnownAt: epoch, EffectiveFrom: epoch,
			Components: []AssetAllocation{{RecipeRef: "A", Weight: 1}}, SourceType: "static_model"},
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	p := newFakeProvider(map[string]float64{"SPY": 5})
	e := newProvEngine(t, p)
	e.SetVersionStore(store)
	if _, err := e.CalculateReturn(context.Background(), "A",
		mustTime("2026-01-02"), mustTime("2026-02-02"), RequirementForPreview()); !errors.Is(err, ErrCircularRecipe) {
		t.Fatalf("expected ErrCircularRecipe, got %v", err)
	}
}

func TestDuplicateSymbolsMerged(t *testing.T) {
	store, err := NewVersionedRecipeStore([]BenchmarkRecipeVersion{
		{RecipeID: "DUP", VersionID: "DUP_v1", PubliclyKnownAt: epoch, EffectiveFrom: epoch,
			Components: []AssetAllocation{{Symbol: "SPY", Weight: 0.5}, {Symbol: "SPY", Weight: 0.5}},
			SourceType: "static_model", RebalancingPolicy: RebalanceBuyAndHold},
	})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	p := newFakeProvider(map[string]float64{"SPY": 8})
	e := newProvEngine(t, p)
	e.SetVersionStore(store)
	res, err := e.CalculateReturn(context.Background(), "DUP",
		mustTime("2026-01-02"), mustTime("2026-02-02"), RequirementForAwards())
	if err != nil {
		t.Fatalf("CalculateReturn: %v", err)
	}
	if len(res.DataMetadata.Symbols) != 1 {
		t.Fatalf("duplicate symbols must merge to one leg, got %v", res.DataMetadata.Symbols)
	}
}
