package benchmark

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// BenchmarkConstructionService builds daily-rebalanced benchmark index returns
// from recipes. It performs no scoring — only return construction.
type BenchmarkConstructionService struct {
	prices          HistoricalPriceProvider
	recipes         map[string]BenchmarkRecipe
	dynamicResolver DynamicRecipeResolver // optional
}

// NewBenchmarkConstructionService wires the engine. dynamicResolver may be nil
// if no dynamic recipes are used.
func NewBenchmarkConstructionService(prices HistoricalPriceProvider, recipes map[string]BenchmarkRecipe, dynamicResolver DynamicRecipeResolver) *BenchmarkConstructionService {
	return &BenchmarkConstructionService{prices: prices, recipes: recipes, dynamicResolver: dynamicResolver}
}

// CalculateReturnPct returns the total benchmark return (percentage points) over
// [start, end], assuming daily rebalancing back to target weights.
//
// Daily benchmark return: R_day = Σ(weight_i × R_i_day)
// Index:                  index_t = index_{t-1} × (1 + R_day), starting at 100
// Result:                 index_end - 100
func (s *BenchmarkConstructionService) CalculateReturnPct(ctx context.Context, recipeID string, start, end time.Time) (float64, error) {
	recipe, err := s.resolveRecipe(ctx, recipeID, end)
	if err != nil {
		return 0, err
	}
	flattened, err := s.flattenRecipe(ctx, recipe, end, 1)
	if err != nil {
		return 0, err
	}
	if err := validateWeights(flattened); err != nil {
		return 0, fmt.Errorf("recipe %s: %w", recipeID, err)
	}

	pricesBySymbol := make(map[string][]PricePoint, len(flattened))
	for _, component := range flattened {
		series, err := s.prices.GetAdjustedCloseSeries(ctx, component.Symbol, start, end)
		if err != nil {
			return 0, fmt.Errorf("prices for %s: %w", component.Symbol, err)
		}
		if len(series) < 2 {
			return 0, fmt.Errorf("not enough price data for %s", component.Symbol)
		}
		pricesBySymbol[component.Symbol] = series
	}

	alignedDates := commonDates(pricesBySymbol)
	if len(alignedDates) < 2 {
		return 0, fmt.Errorf("not enough common dates across benchmark components")
	}

	priceMaps := make(map[string]map[string]float64, len(pricesBySymbol))
	for symbol, series := range pricesBySymbol {
		priceMaps[symbol] = toPriceMap(series)
	}

	index := 100.0
	for i := 1; i < len(alignedDates); i++ {
		prevDate := alignedDates[i-1]
		currDate := alignedDates[i]

		weightedDailyReturn := 0.0
		for _, component := range flattened {
			pm := priceMaps[component.Symbol]
			prev, okPrev := pm[prevDate]
			curr, okCurr := pm[currDate]
			if !okPrev || !okCurr || prev == 0 {
				return 0, fmt.Errorf("missing aligned price for %s", component.Symbol)
			}
			assetDailyReturn := curr/prev - 1
			weightedDailyReturn += component.Weight * assetDailyReturn
		}
		index *= 1 + weightedDailyReturn
	}

	return round(index-100, 4), nil
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
		m[p.Date] = p.AdjustedClose
	}
	return m
}
