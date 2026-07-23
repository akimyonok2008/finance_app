package benchmark

import (
	"context"
	"time"
)

// SnapshotRecipeResolver resolves dynamic recipes (e.g. the Berkshire 13F
// equity basket) from stored, real disclosed holdings. It performs no I/O:
// resolving a live 13F on every evaluation is neither cheap nor reliable
// (13F filings report CUSIPs, not tickers), so the product policy is to store
// each quarter's disclosed basket and refresh it when a new 13F is published.
type SnapshotRecipeResolver struct {
	snapshots map[string]BenchmarkRecipe
}

// NewSnapshotRecipeResolver wires a resolver over the given recipe snapshots,
// keyed by the dynamic recipe id they resolve.
func NewSnapshotRecipeResolver(snapshots map[string]BenchmarkRecipe) *SnapshotRecipeResolver {
	if snapshots == nil {
		snapshots = map[string]BenchmarkRecipe{}
	}
	return &SnapshotRecipeResolver{snapshots: snapshots}
}

// Resolve returns the stored snapshot for a dynamic recipe, or the recipe
// unchanged when no snapshot is registered for it.
func (r *SnapshotRecipeResolver) Resolve(_ context.Context, recipe BenchmarkRecipe, _ time.Time) (BenchmarkRecipe, error) {
	if snap, ok := r.snapshots[recipe.ID]; ok {
		return snap, nil
	}
	return recipe, nil
}

// DefaultRecipeSnapshots returns the current stored baskets for dynamic recipes.
// The Berkshire basket holds the real, publicly disclosed top equity positions
// from Berkshire Hathaway's most recent 13F; weights are normalized to sum to 1.
// Refresh this snapshot (id and weights) each quarter when a new 13F is filed.
func DefaultRecipeSnapshots() map[string]BenchmarkRecipe {
	return map[string]BenchmarkRecipe{
		"BUFFETT_13F": normalizedRecipe(
			"BUFFETT_13F_2025Q1",
			"Berkshire 13F Equity Basket",
			"Berkshire Hathaway's disclosed top public equity holdings (13F, 2025 Q1).",
			[]AssetAllocation{
				{Symbol: "AAPL", Weight: 26},
				{Symbol: "AXP", Weight: 16},
				{Symbol: "BAC", Weight: 11},
				{Symbol: "KO", Weight: 10},
				{Symbol: "CVX", Weight: 6},
				{Symbol: "OXY", Weight: 5},
				{Symbol: "MCO", Weight: 4},
				{Symbol: "KHC", Weight: 4},
				{Symbol: "CB", Weight: 3},
				{Symbol: "DVA", Weight: 2},
			},
		),
	}
}

// normalizedRecipe builds a benchmark recipe whose component weights are
// rescaled to sum to exactly 1, so the engine's weight validation always passes.
func normalizedRecipe(id, name, description string, raw []AssetAllocation) BenchmarkRecipe {
	total := 0.0
	for _, c := range raw {
		if c.Weight > 0 {
			total += c.Weight
		}
	}
	components := make([]AssetAllocation, 0, len(raw))
	if total > 0 {
		for _, c := range raw {
			if c.Weight <= 0 {
				continue
			}
			components = append(components, AssetAllocation{
				Symbol:    c.Symbol,
				RecipeRef: c.RecipeRef,
				Weight:    c.Weight / total,
			})
		}
	}
	return BenchmarkRecipe{ID: id, Name: name, Description: description, Components: components}
}
