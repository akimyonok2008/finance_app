package benchmark

import (
	"context"
	"time"
)

// HistoricalPriceProvider supplies a close series for a symbol over a date
// range. This is the legacy port: it returns bare points with no provenance, so
// the engine wraps any such provider as raw-close/unverified data that cannot
// back a verified award. Prefer HistoricalSeriesProvider for new providers.
type HistoricalPriceProvider interface {
	GetAdjustedCloseSeries(ctx context.Context, symbol string, start, end time.Time) ([]PricePoint, error)
}

// HistoricalSeriesProvider supplies a price series together with its provenance
// (provider, price type, adjustment/total-return capability, freshness). The
// engine requests the data quality it needs via SeriesRequirement and the
// provider must fail rather than silently downgrade — e.g. it must not relabel a
// raw close as an adjusted close.
type HistoricalSeriesProvider interface {
	GetSeries(ctx context.Context, symbol string, start, end time.Time, req SeriesRequirement) (BenchmarkPriceSeries, error)
}

// DynamicRecipeResolver resolves a dynamic recipe (e.g. a Berkshire 13F basket)
// into a concrete allocation as of a date. Implementations should resolve from a
// stored quarterly snapshot, never a live fetch inside evaluation.
type DynamicRecipeResolver interface {
	Resolve(ctx context.Context, recipe BenchmarkRecipe, asOf time.Time) (BenchmarkRecipe, error)
}

type HistoricalFXRate struct {
	Rate     float64
	Date     time.Time
	Provider string
}

// HistoricalFXProvider supplies valuation-date FX. A current/static quote is
// never an acceptable substitute for a verified historical evaluation.
type HistoricalFXProvider interface {
	Rate(ctx context.Context, fromCurrency, toCurrency string, date time.Time) (HistoricalFXRate, error)
}
