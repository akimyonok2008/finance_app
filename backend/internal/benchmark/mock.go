package benchmark

import (
	"context"
	"time"
)

// MockHistoricalPriceProvider synthesizes a smooth adjusted-close series from a
// configured total return per symbol. It is deterministic and does no I/O.
//
// It is a stand-in until a real adjusted-close historical feed is integrated.
// Unknown symbols default to a flat (0%) series.
type MockHistoricalPriceProvider struct {
	totalReturnsPct map[string]float64
}

// NewMockHistoricalPriceProvider builds a mock provider from per-symbol total
// returns (in percentage points) over the requested window.
func NewMockHistoricalPriceProvider(totalReturnsPct map[string]float64) *MockHistoricalPriceProvider {
	if totalReturnsPct == nil {
		totalReturnsPct = map[string]float64{}
	}
	return &MockHistoricalPriceProvider{totalReturnsPct: totalReturnsPct}
}

// GetAdjustedCloseSeries returns a linear price path from 100 to the symbol's
// configured end price across daily dates in [start, end].
func (m *MockHistoricalPriceProvider) GetAdjustedCloseSeries(_ context.Context, symbol string, start, end time.Time) ([]PricePoint, error) {
	dates := dailyDates(start, end)
	totalReturnPct := m.totalReturnsPct[symbol]

	const startPrice = 100.0
	endPrice := startPrice * (1 + totalReturnPct/100)

	out := make([]PricePoint, 0, len(dates))
	for i, d := range dates {
		t := 1.0
		if len(dates) > 1 {
			t = float64(i) / float64(len(dates)-1)
		}
		price := startPrice + (endPrice-startPrice)*t
		out = append(out, PricePoint{Date: toDateKey(d), AdjustedClose: price})
	}
	return out, nil
}

// GetSeries implements HistoricalSeriesProvider, returning the synthetic path
// wrapped in metadata that labels it explicitly as synthetic. Synthetic data is
// admissible for progress previews and demo awards, never for verified awards —
// validateSeries enforces that from the metadata, not the caller's trust.
func (m *MockHistoricalPriceProvider) GetSeries(ctx context.Context, symbol string, start, end time.Time, _ SeriesRequirement) (BenchmarkPriceSeries, error) {
	points, err := m.GetAdjustedCloseSeries(ctx, symbol, start, end)
	if err != nil {
		return BenchmarkPriceSeries{}, err
	}
	now := time.Now().UTC()
	return BenchmarkPriceSeries{
		Symbol: symbol,
		Points: points,
		Metadata: BenchmarkDataMetadata{
			Provider:          "mock",
			ProviderMode:      "mock",
			PriceType:         PriceTypeTotalReturn, // synthetic path is total-return by construction
			IncludesDividends: true,
			IncludesSplits:    true,
			IsAdjusted:        true,
			IsTotalReturn:     true,
			IsSynthetic:       true,
			CorpActionsKnown:  true,
			Quality:           DataQualitySynthetic,
			RetrievedAt:       now,
			SourceAsOf:        now,
			ProviderDataset:   "deterministic_synthetic",
			CurrencyTreatment: "synthetic_same_currency",
			Currency:          "USD",
		},
	}, nil
}

// dailyDates returns each UTC day from start to end inclusive.
func dailyDates(start, end time.Time) []time.Time {
	var dates []time.Time
	cursor := start.UTC().Truncate(24 * time.Hour)
	last := end.UTC().Truncate(24 * time.Hour)
	for !cursor.After(last) {
		dates = append(dates, cursor)
		cursor = cursor.AddDate(0, 0, 1)
	}
	return dates
}

// DefaultMockReturns is a sample return table covering every symbol referenced
// by the recipe catalogue, used to make the engine demoable end-to-end until a
// real historical feed is wired in.
func DefaultMockReturns() map[string]float64 {
	return map[string]float64{
		"VOO": 4.5, "SPY": 4.4, "SGOV": 1.0, "GLD": 2.0, "BND": 0.5, "VT": 3.5,
		"SCHD": 3.2, "SIVR": 6.0, "XLE": 1.5, "URA": -2.0, "BRK.B": 4.0, "TLT": -1.0,
		"QUAL": 3.8, "VTV": 2.9, "IJR": 2.0, "QQQ": 5.0, "VXUS": 3.0, "VNQ": 1.0,
		"UUP": 0.2, "USMV": 2.8, "AAPL": 4.2, "AXP": 3.9, "KO": 2.1, "BAC": 3.1,
		"CVX": 2.8, "OXY": 3.4,
	}
}
