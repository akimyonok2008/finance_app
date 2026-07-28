package performancehistory

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

// A timeframe return is ending/starting - 1, NOT ending - 100. A period that
// starts at 110 and ends at 121 returned 10%, not 21%.
func TestTimeframeReturnPercentUsesRatioNotIndexMinus100(t *testing.T) {
	got, err := TimeframeReturnPercent(testIndex("110"), testIndex("121"))
	require.NoError(t, err)
	assert.InDelta(t, 10.0, got, 1e-9)
	assert.Greater(t, 21.0-got, 1e-6, "must not be the ending-index-minus-100 shortcut")
}

func TestTimeframeReturnPercentFromBaseline(t *testing.T) {
	got, err := TimeframeReturnPercent(testIndex("100"), testIndex("121"))
	require.NoError(t, err)
	assert.InDelta(t, 21.0, got, 1e-9)
}

func TestTimeframeReturnPercentRejectsNonPositiveStart(t *testing.T) {
	_, err := TimeframeReturnPercent(testIndex("0"), testIndex("121"))
	require.ErrorIs(t, err, ErrNoStartingIndex)
}

// drawdown_t = index_t / running_peak_t - 1. For 100,110,105,120,90 the worst
// point is 90 against a running peak of 120 => -25%.
func TestMaxDrawdownPercent(t *testing.T) {
	series := []float64{100, 110, 105, 120, 90}
	assert.InDelta(t, -25.0, MaxDrawdownPercent(series), 1e-9)

	dd := DrawdownSeriesPercent(series)
	require.Len(t, dd, 5)
	assert.InDelta(t, 0.0, dd[0], 1e-9)
	assert.InDelta(t, 0.0, dd[1], 1e-9)
	assert.InDelta(t, -4.545454545, dd[2], 1e-6)
	assert.InDelta(t, 0.0, dd[3], 1e-9)
	assert.InDelta(t, -25.0, dd[4], 1e-9)
}

func TestMaxDrawdownPercentMonotonicSeriesHasNoDrawdown(t *testing.T) {
	assert.InDelta(t, 0.0, MaxDrawdownPercent([]float64{100, 101, 130}), 1e-9)
	assert.InDelta(t, 0.0, MaxDrawdownPercent(nil), 1e-9)
}

func TestParseTimeframeDefaultsTo1M(t *testing.T) {
	assert.Equal(t, Timeframe1M, ParseTimeframe(""))
	assert.Equal(t, Timeframe1M, ParseTimeframe("nonsense"))
	assert.Equal(t, TimeframeAll, ParseTimeframe("all"))
	assert.Equal(t, Timeframe1Y, ParseTimeframe(" 1y "))
}

func newAnalyticsService(now time.Time) (*Service, Repository) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, rankedStub{value: rankedAt(now, now, "100", performance.StatusActive)}, Config{})
	svc.SetClock(func() time.Time { return now })
	return svc, repo
}

// The endpoint's analytics must be derived from the canonical ranked snapshots.
func TestRankedHistoryDerivesReturnAndDrawdownFromCanonicalSnapshots(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	epoch := now.AddDate(0, 0, -10)
	svc, repo := newAnalyticsService(now)

	indexes := []float64{110, 121, 115, 132, 99}
	for i, idx := range indexes {
		at := epoch.AddDate(0, 0, i+1)
		insertPointFloat(t, repo, "s"+string(rune('a'+i)), epoch, at, idx,
			performance.StatusActive, KindDaily, QualityComplete)
	}

	history, err := svc.RankedHistory(context.Background(), "u1", "1M")
	require.NoError(t, err)
	require.True(t, history.Available)
	require.Len(t, history.Points, 5)

	require.NotNil(t, history.StartingIndex)
	require.NotNil(t, history.EndingIndex)
	assert.InDelta(t, 110.0, *history.StartingIndex, 1e-9)
	assert.InDelta(t, 99.0, *history.EndingIndex, 1e-9)

	// 99/110 - 1 = -10%, not 99 - 100 = -1%.
	require.NotNil(t, history.TimeframeReturnPercentage)
	assert.InDelta(t, -10.0, *history.TimeframeReturnPercentage, 1e-6)

	// Peak 132, trough 99 => 99/132 - 1 = -25%.
	require.NotNil(t, history.MaxDrawdownPercentage)
	assert.InDelta(t, -25.0, *history.MaxDrawdownPercentage, 1e-6)

	assert.InDelta(t, 0.0, history.Points[0].ReturnPercentage, 1e-9)
	assert.InDelta(t, 10.0, history.Points[1].ReturnPercentage, 1e-6)
	assert.InDelta(t, -25.0, history.Points[4].DrawdownPercentage, 1e-6)
}

// Missing analytics must be absent (truthful empty state), never zero.
func TestRankedHistoryEmptyIsUnavailableNotZero(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	svc, _ := newAnalyticsService(now)

	history, err := svc.RankedHistory(context.Background(), "u1", "1M")
	require.NoError(t, err)
	assert.False(t, history.Available)
	assert.Nil(t, history.TimeframeReturnPercentage)
	assert.Nil(t, history.MaxDrawdownPercentage)
	assert.Empty(t, history.Points)
	assert.NotEmpty(t, history.Reason)
}

// The endpoint must read the SAME canonical snapshot series that the
// achievement/leaderboard evidence path (Service.Window) treats as truth.
func TestRankedHistoryAgreesWithCanonicalEvidenceWindow(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	epoch := now.AddDate(0, 0, -6)
	svc, repo := newAnalyticsService(now)

	indexes := []float64{110, 112, 118, 121}
	for i, idx := range indexes {
		at := epoch.AddDate(0, 0, i+1)
		insertPointFloat(t, repo, "w"+string(rune('a'+i)), epoch, at, idx,
			performance.StatusActive, KindDaily, QualityComplete)
	}

	window, err := svc.Window(context.Background(), "u1",
		epoch.AddDate(0, 0, 1), epoch.AddDate(0, 0, 4))
	require.NoError(t, err)

	history, err := svc.RankedHistory(context.Background(), "u1", "1M")
	require.NoError(t, err)
	require.True(t, history.Available)

	// Same boundary indexes as the canonical evidence window.
	assert.InDelta(t, window.StartSnapshot.RankedIndex.Float64(), *history.StartingIndex, 1e-9)
	assert.InDelta(t, window.EndSnapshot.RankedIndex.Float64(), *history.EndingIndex, 1e-9)

	expected, err := TimeframeReturnPercent(
		window.StartSnapshot.RankedIndex, window.EndSnapshot.RankedIndex)
	require.NoError(t, err)
	assert.InDelta(t, expected, *history.TimeframeReturnPercentage, 1e-6)

	// And the same point set.
	require.Len(t, history.Points, len(window.Points))
	for i := range window.Points {
		assert.InDelta(t, window.Points[i].RankedIndex.Float64(), history.Points[i].RankedIndex, 1e-9)
	}
}
