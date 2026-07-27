package benchmark

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// BenchmarkConstructionService builds daily-rebalanced benchmark index returns
// from recipes. It performs no scoring — only return construction and
// provenance aggregation.
type BenchmarkConstructionService struct {
	prices          HistoricalPriceProvider
	series          HistoricalSeriesProvider
	recipes         map[string]BenchmarkRecipe
	dynamicResolver DynamicRecipeResolver // legacy, retained for compatibility
	versions        *VersionedRecipeStore
	now             func() time.Time
}

// NewBenchmarkConstructionService wires the engine. dynamicResolver may be nil.
// The provider is adapted to a HistoricalSeriesProvider: providers that only
// implement the legacy port are treated as raw-close/unverified data (they can
// power previews but never a verified award). A default version store is built
// from the catalogue plus the authoritative Berkshire 13F versions.
func NewBenchmarkConstructionService(prices HistoricalPriceProvider, recipes map[string]BenchmarkRecipe, dynamicResolver DynamicRecipeResolver) *BenchmarkConstructionService {
	store, _ := DefaultVersionStore(recipes)
	return &BenchmarkConstructionService{
		prices:          prices,
		series:          asSeriesProvider(prices),
		recipes:         recipes,
		dynamicResolver: dynamicResolver,
		versions:        store,
		now:             func() time.Time { return time.Now().UTC() },
	}
}

// SetVersionStore overrides the version store (e.g. a durable, DB-backed store).
func (s *BenchmarkConstructionService) SetVersionStore(store *VersionedRecipeStore) {
	if store != nil {
		s.versions = store
	}
}

// SetClock overrides the clock, for deterministic tests.
func (s *BenchmarkConstructionService) SetClock(now func() time.Time) { s.now = now }

// asSeriesProvider adapts any provider to a HistoricalSeriesProvider. A provider
// that already implements the richer port is used directly; a legacy provider is
// wrapped so its output is explicitly labelled raw-close with unknown corporate
// actions — which fails closed under an award-grade requirement.
func asSeriesProvider(p HistoricalPriceProvider) HistoricalSeriesProvider {
	if sp, ok := p.(HistoricalSeriesProvider); ok {
		return sp
	}
	if p == nil {
		return nil
	}
	return legacySeriesAdapter{p: p}
}

type legacySeriesAdapter struct{ p HistoricalPriceProvider }

func (a legacySeriesAdapter) GetSeries(ctx context.Context, symbol string, start, end time.Time, _ SeriesRequirement) (BenchmarkPriceSeries, error) {
	pts, err := a.p.GetAdjustedCloseSeries(ctx, symbol, start, end)
	if err != nil {
		return BenchmarkPriceSeries{}, err
	}
	now := time.Now().UTC()
	return BenchmarkPriceSeries{
		Symbol: symbol,
		Points: pts,
		Metadata: BenchmarkDataMetadata{
			Provider:         "legacy_provider",
			ProviderMode:     "real",
			PriceType:        PriceTypeRawClose,
			CorpActionsKnown: false,
			Quality:          DataQualityAcceptable,
			RetrievedAt:      now,
			SourceAsOf:       now,
		},
	}, nil
}

// CalculateReturnPct returns the total benchmark return (percentage points) over
// [start, end] under daily rebalancing. It is the preview-grade wrapper around
// CalculateReturn: it permits synthetic/stale/raw data (no permanent award is
// issued from a preview) and returns only the number. Award decisions must use
// CalculateReturn and inspect the provenance.
func (s *BenchmarkConstructionService) CalculateReturnPct(ctx context.Context, recipeID string, start, end time.Time) (float64, error) {
	res, err := s.CalculateReturn(ctx, recipeID, start, end, RequirementForPreview())
	if err != nil {
		return 0, err
	}
	return res.ReturnPercentage, nil
}

func (s *BenchmarkConstructionService) resolveRecipe(ctx context.Context, recipeID string, asOf time.Time) (BenchmarkRecipe, error) {
	recipe, ok := s.recipes[recipeID]
	if !ok {
		return BenchmarkRecipe{}, fmt.Errorf("unknown benchmark recipe: %s", recipeID)
	}
	if recipe.Dynamic {
		if s.dynamicResolver == nil {
			return BenchmarkRecipe{}, fmt.Errorf("recipe %s is dynamic but no resolver is configured", recipeID)
		}
		return s.dynamicResolver.Resolve(ctx, recipe, asOf)
	}
	return recipe, nil
}

// flattenRecipe expands nested recipeRef legs into a flat list of symbol legs,
// scaling child weights by the parent weight and merging duplicate symbols.
func (s *BenchmarkConstructionService) flattenRecipe(ctx context.Context, recipe BenchmarkRecipe, asOf time.Time, parentWeight float64) ([]AssetAllocation, error) {
	var result []AssetAllocation
	for _, component := range recipe.Components {
		effectiveWeight := parentWeight * component.Weight
		switch {
		case component.Symbol != "":
			result = append(result, AssetAllocation{Symbol: component.Symbol, Weight: effectiveWeight})
		case component.RecipeRef != "":
			child, err := s.resolveRecipe(ctx, component.RecipeRef, asOf)
			if err != nil {
				return nil, err
			}
			childComponents, err := s.flattenRecipe(ctx, child, asOf, effectiveWeight)
			if err != nil {
				return nil, err
			}
			result = append(result, childComponents...)
		default:
			return nil, fmt.Errorf("recipe %s has a component with neither symbol nor recipeRef", recipe.ID)
		}
	}
	return mergeDuplicateSymbols(result), nil
}

func mergeDuplicateSymbols(components []AssetAllocation) []AssetAllocation {
	merged := make(map[string]float64, len(components))
	order := make([]string, 0, len(components))
	for _, c := range components {
		if _, seen := merged[c.Symbol]; !seen {
			order = append(order, c.Symbol)
		}
		merged[c.Symbol] += c.Weight
	}
	out := make([]AssetAllocation, 0, len(order))
	for _, symbol := range order {
		out = append(out, AssetAllocation{Symbol: symbol, Weight: merged[symbol]})
	}
	return out
}

// commonDates returns the sorted set of dates present in every symbol's series.
func commonDates(pricesBySymbol map[string][]PricePoint) []string {
	if len(pricesBySymbol) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, series := range pricesBySymbol {
		seen := make(map[string]struct{}, len(series))
		for _, p := range series {
			if _, dup := seen[p.Date]; dup {
				continue
			}
			seen[p.Date] = struct{}{}
			counts[p.Date]++
		}
	}
	n := len(pricesBySymbol)
	out := make([]string, 0, len(counts))
	for date, c := range counts {
		if c == n {
			out = append(out, date)
		}
	}
	sort.Strings(out)
	return out
}

func toPriceMap(series []PricePoint) map[string]float64 {
	m := make(map[string]float64, len(series))
	for _, p := range series {
		if p.AdjustedClose != 0 {
			m[p.Date] = p.AdjustedClose
		} else {
			m[p.Date] = p.RawClose
		}
	}
	return m
}
