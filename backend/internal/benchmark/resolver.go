package benchmark

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// SnapshotRecipeResolver is the legacy single-snapshot resolver for dynamic
// recipes. It is superseded by the versioned recipe store (see
// recipe_version.go): BenchmarkConstructionService now selects an immutable
// recipe version by the no-look-ahead policy (newest version whose
// PubliclyKnownAt <= evaluation start) rather than always returning one stored
// snapshot. This type is retained only for backward-compatible wiring and is not
// consulted by CalculateReturn.
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

// DefaultRecipeSnapshots returns a legacy, approximate Berkshire basket for the
// deprecated SnapshotRecipeResolver. It is NOT the source of truth: the
// authoritative, immutably versioned Berkshire baskets (transcribed from the SEC
// 13F-HR filings) live in berkshireVersions() and are what CalculateReturn uses.
// These approximate weights are never used for a verified award and are kept
// only so legacy wiring still compiles.
func DefaultRecipeSnapshots() map[string]BenchmarkRecipe {
	return map[string]BenchmarkRecipe{
		"BUFFETT_13F": normalizedRecipe(
			"BUFFETT_13F_legacy_approx",
			"Berkshire 13F Equity Basket (legacy approximate)",
			"Legacy approximate Berkshire basket; superseded by versioned SEC 13F baskets.",
			[]AssetAllocation{
				{Symbol: "AAPL", Weight: money.MustWeight("26")},
				{Symbol: "AXP", Weight: money.MustWeight("16")},
				{Symbol: "BAC", Weight: money.MustWeight("11")},
				{Symbol: "KO", Weight: money.MustWeight("10")},
				{Symbol: "CVX", Weight: money.MustWeight("6")},
				{Symbol: "OXY", Weight: money.MustWeight("5")},
				{Symbol: "MCO", Weight: money.MustWeight("4")},
				{Symbol: "KHC", Weight: money.MustWeight("4")},
				{Symbol: "CB", Weight: money.MustWeight("3")},
				{Symbol: "DVA", Weight: money.MustWeight("2")},
			},
		),
	}
}

// normalizedRecipe builds a benchmark recipe whose component weights are
// rescaled to sum to exactly 1, so the engine's weight validation always passes.
func normalizedRecipe(id, name, description string, raw []AssetAllocation) BenchmarkRecipe {
	total := money.ZeroWeight()
	for _, c := range raw {
		if c.Weight.Sign() > 0 {
			total = total.Add(c.Weight)
		}
	}
	components := make([]AssetAllocation, 0, len(raw))
	if total.Sign() > 0 {
		for _, c := range raw {
			if c.Weight.Sign() <= 0 {
				continue
			}
			normalized, err := c.Weight.DivExact(total, intermediatePrecision)
			if err != nil {
				continue
			}
			components = append(components, AssetAllocation{
				Symbol:    c.Symbol,
				RecipeRef: c.RecipeRef,
				Weight:    normalized,
			})
		}
	}
	return BenchmarkRecipe{ID: id, Name: name, Description: description, Components: components}
}
