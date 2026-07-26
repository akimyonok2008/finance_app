package performancehistory

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
}

func pts(values ...indexPoint) []indexPoint { return values }

// With no snapshots every metric must be nil with a reason. Reporting 0%
// drawdown / 0% positive weeks for a portfolio with no history is a lie.
func TestRiskConsistencyEmptyHistoryIsNilNotZero(t *testing.T) {
	got := CalculateRiskConsistency(nil, day(2026, time.July, 26))

	assert.Nil(t, got.MaxDrawdownPercentage)
	assert.Nil(t, got.CurrentDrawdownPercentage)
	assert.Nil(t, got.PositiveWeeksPercentage)
	assert.Nil(t, got.BestMonth)
	assert.Nil(t, got.WorstMonth)
	assert.NotEmpty(t, got.DrawdownReason)
	assert.NotEmpty(t, got.WeeksReason)
	assert.NotEmpty(t, got.MonthsReason)
	assert.Equal(t, "ranked_index", got.CalculationBase)
}

// Current drawdown is the FINAL index versus its running peak, which is a
// different number from the worst drawdown in the window.
func TestRiskConsistencyCurrentDrawdownIsFinalPointVersusPeak(t *testing.T) {
	series := pts(
		indexPoint{at: day(2026, time.January, 5), index: 100},
		indexPoint{at: day(2026, time.January, 6), index: 120},
		indexPoint{at: day(2026, time.January, 7), index: 90},  // worst: -25%
		indexPoint{at: day(2026, time.January, 8), index: 108}, // current: -10%
	)
	got := CalculateRiskConsistency(series, day(2026, time.March, 1))

	require.NotNil(t, got.MaxDrawdownPercentage)
	require.NotNil(t, got.CurrentDrawdownPercentage)
	assert.InDelta(t, -25.0, *got.MaxDrawdownPercentage, 1e-6)
	assert.InDelta(t, -10.0, *got.CurrentDrawdownPercentage, 1e-6)
	assert.NotEqual(t, *got.MaxDrawdownPercentage, *got.CurrentDrawdownPercentage)
}

// Max drawdown must be the SAME number MaxDrawdownPercent produces — the risk
// block reuses that calculation rather than duplicating it.
func TestRiskConsistencyMaxDrawdownReusesDrawdownCalculation(t *testing.T) {
	values := []float64{100, 130, 95, 110}
	series := make([]indexPoint, 0, len(values))
	for i, v := range values {
		series = append(series, indexPoint{at: day(2026, time.January, 5+i), index: v})
	}
	got := CalculateRiskConsistency(series, day(2026, time.March, 1))

	require.NotNil(t, got.MaxDrawdownPercentage)
	assert.InDelta(t, round4(MaxDrawdownPercent(values)), *got.MaxDrawdownPercentage, 1e-9)
}

// Complete calendar weeks only. 2026-01-05 is a Monday; the week of Jan 5-11
// and Jan 12-18 are complete when "now" is Jan 21, but the week of Jan 19 is
// still running and must be excluded from BOTH numerator and denominator.
func TestRiskConsistencyExcludesTheCurrentIncompleteWeek(t *testing.T) {
	series := pts(
		indexPoint{at: day(2026, time.January, 5), index: 100},
		indexPoint{at: day(2026, time.January, 9), index: 110}, // week 1 close: +10%
		indexPoint{at: day(2026, time.January, 16), index: 99}, // week 2 close: -10%
		indexPoint{at: day(2026, time.January, 20), index: 5},  // running week: ignored
	)
	got := CalculateRiskConsistency(series, day(2026, time.January, 21))

	assert.Equal(t, 2, got.CompleteWeeks)
	assert.Equal(t, 1, got.PositiveWeeks)
	require.NotNil(t, got.PositiveWeeksPercentage)
	assert.InDelta(t, 50.0, *got.PositiveWeeksPercentage, 1e-6)
}

// Fewer than one complete week must say "not enough history", not 0%.
func TestRiskConsistencyReportsNotEnoughWeeks(t *testing.T) {
	got := CalculateRiskConsistency(pts(
		indexPoint{at: day(2026, time.January, 20), index: 100},
		indexPoint{at: day(2026, time.January, 21), index: 104},
	), day(2026, time.January, 21))

	assert.Equal(t, 0, got.CompleteWeeks)
	assert.Nil(t, got.PositiveWeeksPercentage)
	assert.Contains(t, got.WeeksReason, "Not enough history")
}

// Best/worst month cover COMPLETE calendar months only, chained across month
// boundaries: February opens at January's close.
func TestRiskConsistencyBestAndWorstCompleteMonths(t *testing.T) {
	series := pts(
		indexPoint{at: day(2026, time.January, 2), index: 100},
		indexPoint{at: day(2026, time.January, 30), index: 110}, // Jan: +10%
		indexPoint{at: day(2026, time.February, 27), index: 99}, // Feb: -10%
		indexPoint{at: day(2026, time.March, 31), index: 108.9}, // Mar: +10%
		indexPoint{at: day(2026, time.April, 3), index: 1},      // running: ignored
	)
	got := CalculateRiskConsistency(series, day(2026, time.April, 3))

	assert.Equal(t, 3, got.CompleteMonths)
	require.NotNil(t, got.BestMonth)
	require.NotNil(t, got.WorstMonth)
	assert.InDelta(t, 10.0, got.BestMonth.ReturnPercentage, 1e-6)
	assert.Equal(t, "2026-02", got.WorstMonth.Label)
	assert.InDelta(t, -10.0, got.WorstMonth.ReturnPercentage, 1e-6)
}

func TestRiskConsistencyReportsNotEnoughMonths(t *testing.T) {
	got := CalculateRiskConsistency(pts(
		indexPoint{at: day(2026, time.July, 2), index: 100},
		indexPoint{at: day(2026, time.July, 20), index: 130},
	), day(2026, time.July, 26))

	assert.Equal(t, 0, got.CompleteMonths)
	assert.Nil(t, got.BestMonth)
	assert.Nil(t, got.WorstMonth)
	assert.Contains(t, got.MonthsReason, "Not enough history")
}

// A gap in coverage must not be chained across: anchoring March on January's
// close would invent a return for a period we did not observe.
func TestRiskConsistencyDoesNotChainAcrossAGapInMonths(t *testing.T) {
	series := pts(
		indexPoint{at: day(2026, time.January, 5), index: 100},
		indexPoint{at: day(2026, time.January, 30), index: 200},
		// No February observations at all.
		indexPoint{at: day(2026, time.March, 3), index: 210},
		indexPoint{at: day(2026, time.March, 30), index: 231}, // +10% from its own open
	)
	got := CalculateRiskConsistency(series, day(2026, time.April, 10))

	assert.Equal(t, 2, got.CompleteMonths)
	require.NotNil(t, got.BestMonth)
	// March is +10% from its own first observation, NOT 231/200-1 = +15.5%.
	assert.Equal(t, "2026-01", got.BestMonth.Label)
	assert.InDelta(t, 100.0, got.BestMonth.ReturnPercentage, 1e-6)
	require.NotNil(t, got.WorstMonth)
	assert.Equal(t, "2026-03", got.WorstMonth.Label)
	assert.InDelta(t, 10.0, got.WorstMonth.ReturnPercentage, 1e-6)
}

func TestWeekStartIsUTCMonday(t *testing.T) {
	// 2026-01-11 is a Sunday; its week starts Monday 2026-01-05.
	assert.Equal(t,
		time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
		weekStart(day(2026, time.January, 11)))
	assert.Equal(t,
		time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
		weekStart(day(2026, time.January, 5)))
}
