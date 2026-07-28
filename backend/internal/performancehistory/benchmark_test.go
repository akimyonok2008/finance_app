package performancehistory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubBenchmark struct {
	result       BenchmarkReturn
	err          error
	requestStart time.Time
	requestEnd   time.Time
	calls        int
}

func (s *stubBenchmark) ReturnOver(_ context.Context, _ string, start, end time.Time) (BenchmarkReturn, error) {
	s.calls++
	s.requestStart, s.requestEnd = start, end
	if s.err != nil {
		return BenchmarkReturn{}, s.err
	}
	return s.result, nil
}

func snapshotAt(at time.Time, index string) Snapshot {
	return Snapshot{
		CapturedAt: at, ValuationAsOf: at, RankedIndex: testIndex(index),
		TrackingStartedAt: day(2025, time.January, 1),
		RankingStatus:     performance.StatusActive, DataQualityStatus: QualityComplete,
	}
}

// Without a benchmark source the block must say so, not report a 0-point
// difference (which would read as "you exactly matched the market").
func TestBenchmarkComparisonUnavailableWithoutASource(t *testing.T) {
	svc := &Service{}
	got := svc.benchmarkComparison(context.Background(), []Snapshot{
		snapshotAt(day(2026, time.January, 5), "100"),
		snapshotAt(day(2026, time.January, 9), "110"),
	})

	assert.False(t, got.Available)
	assert.Nil(t, got.DifferencePercentagePoints)
	assert.Contains(t, got.Reason, "no benchmark price source")
}

// The portfolio leg must be measured over the benchmark's OWN effective trading
// dates, not over the whole requested chart window — that is the timeframe
// mismatch this section exists to avoid.
func TestBenchmarkComparisonAlignsOnTheBenchmarksEffectiveDates(t *testing.T) {
	stub := &stubBenchmark{result: BenchmarkReturn{
		RecipeID:         DefaultBenchmarkRecipeID,
		Name:             "S&P 500",
		ReturnPercentage: 4,
		// The benchmark could only be measured Jan 6 -> Jan 8 (trading days).
		EffectiveStart: day(2026, time.January, 6),
		EffectiveEnd:   day(2026, time.January, 8),
		Quality:        "verified",
	}}
	svc := &Service{benchmark: stub}

	points := []Snapshot{
		snapshotAt(day(2026, time.January, 5), "50"),  // outside the aligned window
		snapshotAt(day(2026, time.January, 6), "100"), // aligned start
		snapshotAt(day(2026, time.January, 7), "120"),
		snapshotAt(day(2026, time.January, 8), "110"), // aligned end
		snapshotAt(day(2026, time.January, 9), "400"), // outside the aligned window
	}
	got := svc.benchmarkComparison(context.Background(), points)

	require.True(t, got.Available)
	assert.Equal(t, "2026-01-06", got.AlignedFrom)
	assert.Equal(t, "2026-01-08", got.AlignedTo)

	// Portfolio: 110/100 - 1 = +10% over the ALIGNED window. Measuring the full
	// window (400/50 - 1 = +700%) would be the mismatch.
	require.NotNil(t, got.PortfolioReturnPercentage)
	assert.InDelta(t, 10.0, *got.PortfolioReturnPercentage, 1e-6)

	require.NotNil(t, got.BenchmarkReturnPercentage)
	assert.InDelta(t, 4.0, *got.BenchmarkReturnPercentage, 1e-6)

	require.NotNil(t, got.DifferencePercentagePoints)
	assert.InDelta(t, 6.0, *got.DifferencePercentagePoints, 1e-6)

	// The engine is asked for the chart window's real bounds.
	assert.Equal(t, day(2026, time.January, 5).UTC(), stub.requestStart)
	assert.Equal(t, day(2026, time.January, 9).UTC(), stub.requestEnd)
}

// If no ranked snapshot exists at or before the benchmark's start date, the two
// legs cannot describe the same interval and the comparison must be withheld.
func TestBenchmarkComparisonWithheldWhenWindowsDoNotOverlap(t *testing.T) {
	svc := &Service{benchmark: &stubBenchmark{result: BenchmarkReturn{
		ReturnPercentage: 4,
		EffectiveStart:   day(2025, time.January, 6),
		EffectiveEnd:     day(2025, time.January, 8),
	}}}
	got := svc.benchmarkComparison(context.Background(), []Snapshot{
		snapshotAt(day(2026, time.January, 6), "100"),
		snapshotAt(day(2026, time.January, 8), "110"),
	})

	assert.False(t, got.Available)
	assert.Nil(t, got.DifferencePercentagePoints)
	assert.Contains(t, got.Reason, "same dates")
}

func TestBenchmarkComparisonWithheldOnProviderError(t *testing.T) {
	svc := &Service{benchmark: &stubBenchmark{err: errors.New("no series")}}
	got := svc.benchmarkComparison(context.Background(), []Snapshot{
		snapshotAt(day(2026, time.January, 6), "100"),
		snapshotAt(day(2026, time.January, 8), "110"),
	})

	assert.False(t, got.Available)
	assert.Nil(t, got.BenchmarkReturnPercentage)
	assert.NotEmpty(t, got.Reason)
}

func TestBenchmarkComparisonWithheldForRawCloseButPortfolioHistoryRemainsUsable(t *testing.T) {
	svc := &Service{benchmark: &stubBenchmark{result: BenchmarkReturn{
		RecipeID: "SPY", Name: "S&P 500", ReturnPercentage: 3,
		EffectiveStart: day(2026, time.January, 2),
		EffectiveEnd:   day(2026, time.January, 5),
		Quality:        "acceptable", DataType: "raw_close",
		CurrencyTreatment: "native_quote_currency_unhedged",
	}}}
	points := []Snapshot{
		{CapturedAt: day(2026, time.January, 2), RankedIndex: testIndex("100")},
		{CapturedAt: day(2026, time.January, 5), RankedIndex: testIndex("105")},
	}

	got := svc.benchmarkComparison(context.Background(), points)
	assert.False(t, got.Available)
	assert.Equal(t, "raw_close", got.DataType)
	assert.Equal(t, "insufficient_for_total_return_comparison", got.DataQualityStatus)
	assert.Contains(t, got.Reason, "not comparable")
	portfolioReturn, err := TimeframeReturnPercent(points[0].RankedIndex, points[1].RankedIndex)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, portfolioReturn, 1e-9)
}

// Provenance travels with the number so a synthetic/mock comparison is labelled.
func TestBenchmarkComparisonCarriesDataProvenance(t *testing.T) {
	svc := &Service{benchmark: &stubBenchmark{result: BenchmarkReturn{
		Name:             "S&P 500",
		ReturnPercentage: 1,
		EffectiveStart:   day(2026, time.January, 6),
		EffectiveEnd:     day(2026, time.January, 8),
		Quality:          "synthetic",
		Synthetic:        true,
	}}}
	got := svc.benchmarkComparison(context.Background(), []Snapshot{
		snapshotAt(day(2026, time.January, 6), "100"),
		snapshotAt(day(2026, time.January, 8), "110"),
	})

	require.True(t, got.Available)
	assert.Equal(t, "synthetic", got.DataQuality)
	assert.True(t, got.IsSynthetic)
	assert.Equal(t, "S&P 500", got.Name)
}

// A single snapshot cannot produce a return over any window.
func TestBenchmarkComparisonNeedsTwoSnapshots(t *testing.T) {
	stub := &stubBenchmark{}
	svc := &Service{benchmark: stub}
	got := svc.benchmarkComparison(context.Background(),
		[]Snapshot{snapshotAt(day(2026, time.January, 6), "100")})

	assert.False(t, got.Available)
	assert.Zero(t, stub.calls, "must not call the benchmark engine without a measurable window")
}
