package benchmark

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func mustTime(s string) time.Time {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

func newTestEngine(returns map[string]float64) *BenchmarkConstructionService {
	return NewBenchmarkConstructionService(
		NewMockHistoricalPriceProvider(returns),
		Recipes,
		NewSnapshotRecipeResolver(DefaultRecipeSnapshots()),
	)
}

func TestSingleSymbolRecipeReturn(t *testing.T) {
	engine := newTestEngine(map[string]float64{"VOO": 4.5})
	start := mustTime("2026-01-01")
	end := mustTime("2026-03-31")

	got, err := engine.CalculateReturnPct(context.Background(), "VOO", start, end)
	if err != nil {
		t.Fatalf("CalculateReturnPct: %v", err)
	}
	// A smooth linear path from 100 to 104.5, daily rebalanced (single asset),
	// reproduces ~4.5% within rounding tolerance.
	if math.Abs(got-4.5) > 0.05 {
		t.Errorf("expected ~4.5%%, got %.4f", got)
	}
}

func TestTwoAssetRecipeReturnBetweenComponents(t *testing.T) {
	engine := newTestEngine(map[string]float64{"SPY": 10, "BND": 0})
	start := mustTime("2026-01-01")
	end := mustTime("2026-04-01")

	got, err := engine.CalculateReturnPct(context.Background(), "BALANCED_60_40", start, end)
	if err != nil {
		t.Fatalf("CalculateReturnPct: %v", err)
	}
	// 60% at +10% and 40% at 0% should land between 0 and 10, near 6.
	if got <= 0 || got >= 10 {
		t.Errorf("expected return between 0 and 10, got %.4f", got)
	}
}

func TestNestedRecipeFlattening(t *testing.T) {
	engine := newTestEngine(DefaultMockReturns())
	recipe := Recipes["ALL_WEATHER"]
	flattened, err := engine.flattenRecipe(context.Background(), recipe, mustTime("2026-06-01"), money.MustWeight("1"))
	if err != nil {
		t.Fatalf("flattenRecipe: %v", err)
	}
	if err := validateWeights(flattened); err != nil {
		t.Fatalf("flattened weights invalid: %v", err)
	}
	// GLD appears directly (0.15) and inside CMDTY (0.075 * 0.25 = 0.01875).
	var gld money.Weight
	seen := map[string]bool{}
	for _, c := range flattened {
		if seen[c.Symbol] {
			t.Fatalf("duplicate symbol after merge: %s", c.Symbol)
		}
		seen[c.Symbol] = true
		if c.Symbol == "GLD" {
			gld = c.Weight
		}
	}
	want := money.MustWeight("0.15").Add(money.MustWeight("0.075").Mul(money.MustWeight("0.25")))
	if !gld.Equal(want.Decimal) {
		t.Errorf("expected merged GLD weight %s, got %s", want.String(), gld.String())
	}
}

func TestDynamicRecipeResolved(t *testing.T) {
	engine := newTestEngine(DefaultMockReturns())
	got, err := engine.CalculateReturnPct(context.Background(), "BUFFETT_13F",
		mustTime("2026-01-01"), mustTime("2026-06-30"))
	if err != nil {
		t.Fatalf("dynamic recipe should resolve: %v", err)
	}
	if got == 0 {
		t.Errorf("expected non-zero return for resolved 13F basket")
	}
}

// TestDynamicRecipeLookAheadFailsClosed proves the no-look-ahead policy: a
// dynamic 13F recipe evaluated over a window that starts before any version was
// publicly filed must fail closed rather than borrow future holdings. The
// earliest Berkshire version becomes public on 2025-05-15, so a 2024 window has
// no valid recipe version.
func TestDynamicRecipeLookAheadFailsClosed(t *testing.T) {
	engine := NewBenchmarkConstructionService(
		NewMockHistoricalPriceProvider(DefaultMockReturns()), Recipes, nil,
	)
	_, err := engine.CalculateReturnPct(context.Background(), "BUFFETT_13F",
		mustTime("2024-06-01"), mustTime("2024-12-31"))
	if !errors.Is(err, ErrRecipeVersionUnavailable) {
		t.Fatalf("expected ErrRecipeVersionUnavailable for pre-filing window, got %v", err)
	}
}

func TestUnknownRecipeErrors(t *testing.T) {
	engine := newTestEngine(DefaultMockReturns())
	_, err := engine.CalculateReturnPct(context.Background(), "NOPE",
		mustTime("2026-01-01"), mustTime("2026-02-01"))
	if err == nil {
		t.Fatalf("expected error for unknown recipe")
	}
}

func TestAllCatalogRecipesResolveAndScore(t *testing.T) {
	engine := newTestEngine(DefaultMockReturns())
	start := mustTime("2026-01-01")
	end := mustTime("2026-12-31")
	for id := range Recipes {
		if _, err := engine.CalculateReturnPct(context.Background(), id, start, end); err != nil {
			t.Errorf("recipe %s failed to score: %v", id, err)
		}
	}
}

func TestSubtractPeriod(t *testing.T) {
	end := mustTime("2026-07-22")
	cases := map[PeriodCode]string{
		Period30D: "2026-06-22",
		Period90D: "2026-04-23",
		Period6M:  "2026-01-22",
		Period1Y:  "2025-07-22",
	}
	for period, want := range cases {
		got, err := SubtractPeriod(end, period)
		if err != nil {
			t.Fatalf("SubtractPeriod(%s): %v", period, err)
		}
		if toDateKey(got) != want {
			t.Errorf("SubtractPeriod(%s) = %s, want %s", period, toDateKey(got), want)
		}
	}
}

func TestCalculateIndexReturnPct(t *testing.T) {
	points := []IndexPoint{
		{Date: "2026-01-01", Index: money.MustIndexValue("100")},
		{Date: "2026-02-01", Index: money.MustIndexValue("107")},
	}
	got, err := CalculateIndexReturnPct(points)
	if err != nil {
		t.Fatalf("CalculateIndexReturnPct: %v", err)
	}
	if got.String() != "7" {
		t.Errorf("expected 7, got %s", got.String())
	}
}

func TestValidateWeightsCatalog(t *testing.T) {
	for id, recipe := range Recipes {
		if recipe.Dynamic {
			continue
		}
		if err := validateWeights(recipe.Components); err != nil {
			t.Errorf("recipe %s has invalid weights: %v", id, err)
		}
	}
}
