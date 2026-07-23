package benchmark

import (
	"context"
	"time"
)

// HistoricalPriceProvider supplies adjusted-close series for a symbol over a
// date range. Implement it with a licensed feed, a cache, or the bundled mock.
// Prices MUST be adjusted closes so splits/dividends do not distort returns.
type HistoricalPriceProvider interface {
	GetAdjustedCloseSeries(ctx context.Context, symbol string, start, end time.Time) ([]PricePoint, error)
}

// DynamicRecipeResolver resolves a dynamic recipe (e.g. a Berkshire 13F basket)
// into a concrete allocation as of a date. Implementations should resolve from a
// stored quarterly snapshot, never a live fetch inside evaluation.
type DynamicRecipeResolver interface {
	Resolve(ctx context.Context, recipe BenchmarkRecipe, asOf time.Time) (BenchmarkRecipe, error)
}
