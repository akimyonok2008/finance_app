package achievements

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
)

// stubPerformance returns a two-point index series describing a total return
// over whatever window is requested.
type stubPerformance struct {
	returnPct map[string]float64 // userID -> total return %
	hasData   map[string]bool
}

func (s stubPerformance) GetPortfolioIndexSeries(_ context.Context, userID string, start, end time.Time) ([]benchmark.IndexPoint, error) {
	if s.hasData != nil && !s.hasData[userID] {
		return nil, nil
	}
	ret := s.returnPct[userID]
	return []benchmark.IndexPoint{
		{Date: start.UTC().Format("2006-01-02"), Index: 100},
		{Date: end.UTC().Format("2006-01-02"), Index: 100 * (1 + ret/100)},
	}, nil
}

// shortHistoryPerformance returns a strong-return series that only spans a few
// days ending now, regardless of the requested window — simulating a portfolio
// too young to cover any badge period.
type shortHistoryPerformance struct{}

func (shortHistoryPerformance) GetPortfolioIndexSeries(_ context.Context, _ string, _, end time.Time) ([]benchmark.IndexPoint, error) {
	return []benchmark.IndexPoint{
		{Date: end.AddDate(0, 0, -5).UTC().Format("2006-01-02"), Index: 100},
		{Date: end.UTC().Format("2006-01-02"), Index: 130},
	}, nil
}

func TestInsufficientCoverageUnlocksNothing(t *testing.T) {
	// A +30% portfolio would beat every benchmark, but with only 5 days of
	// history it can't cover even the 30D badge window, so nothing unlocks.
	svc := newTestService(shortHistoryPerformance{})
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	for _, a := range list {
		assert.Falsef(t, a.Unlocked, "badge %s must not unlock without period coverage", a.Key)
		require.NotNil(t, a.Progress)
		assert.Equal(t, "tracking", a.Progress.State)
		assert.Greater(t, a.Progress.HistoryCoveragePercentage, 0.0)
		assert.Greater(t, a.Progress.ProgressPercentage, 0.0)
		assert.NotNil(t, a.Progress.CurrentEdgePoints)
	}
}

func newTestService(perf PortfolioPerformanceProvider) *Service {
	engine := benchmark.NewBenchmarkConstructionService(
		benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns()),
		benchmark.Recipes,
		benchmark.NewSnapshotRecipeResolver(benchmark.DefaultRecipeSnapshots()),
	)
	rules := benchmark.NewRulesEngine(benchmark.DefaultEvaluators())
	return NewService(NewInMemoryAchievementRepository(), perf, engine, rules)
}

func TestListCatalogueMetadata(t *testing.T) {
	svc := newTestService(stubPerformance{returnPct: map[string]float64{}})
	list, err := svc.ListAchievementsForUser(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, list, len(benchmark.Badges))
	for _, a := range list {
		assert.False(t, a.Unlocked)
		assert.NotEmpty(t, a.Difficulty)
		assert.NotEmpty(t, a.Period)
		assert.Nil(t, a.Progress, "catalogue reads must remain lightweight")
	}
}

func TestBeatingBenchmarksUnlocksBadges(t *testing.T) {
	// A strong +25% portfolio beats every benchmark in the mock table.
	svc := newTestService(stubPerformance{returnPct: map[string]float64{"u1": 25}})
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)

	byKey := map[string]AchievementResponse{}
	for _, a := range list {
		byKey[a.Key] = a
	}
	assert.True(t, byKey["cash_plus_30d"].Unlocked)
	assert.True(t, byKey["first_market_edge_30d"].Unlocked)

	ev := byKey["cash_plus_30d"].Evidence
	require.NotNil(t, ev)
	assert.Greater(t, ev.EdgePoints, 0.0)
	assert.Equal(t, "CASH", ev.BenchmarkRecipeID)
}

func TestLosingPortfolioUnlocksNothing(t *testing.T) {
	svc := newTestService(stubPerformance{returnPct: map[string]float64{"u1": -10}})
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	for _, a := range list {
		assert.Falsef(t, a.Unlocked, "badge %s should be locked for a losing portfolio", a.Key)
		require.NotNil(t, a.Progress)
		assert.Equal(t, "tracking", a.Progress.State)
		assert.GreaterOrEqual(t, a.Progress.HistoryCoveragePercentage, 99.0)
		assert.NotNil(t, a.Progress.PortfolioReturnPercentage)
		assert.NotNil(t, a.Progress.BenchmarkReturnPercentage)
		assert.NotNil(t, a.Progress.CurrentEdgePoints)
	}
}

func TestNoPortfolioHistoryUnlocksNothing(t *testing.T) {
	svc := newTestService(stubPerformance{
		returnPct: map[string]float64{"u1": 25},
		hasData:   map[string]bool{"u1": false},
	})
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	for _, a := range list {
		assert.False(t, a.Unlocked)
		require.NotNil(t, a.Progress)
		assert.Equal(t, "building_history", a.Progress.State)
		assert.Zero(t, a.Progress.ProgressPercentage)
		assert.Zero(t, a.Progress.HistoryCoveragePercentage)
		assert.Contains(t, a.Progress.Reason, "tracking is active")
	}
}

func TestAwardIsIdempotent(t *testing.T) {
	svc := newTestService(stubPerformance{returnPct: map[string]float64{"u1": 25}})

	first, err := svc.checkAndAwardBadges(context.Background(), "u1")
	require.NoError(t, err)
	require.NotEmpty(t, first)

	second, err := svc.checkAndAwardBadges(context.Background(), "u1")
	require.NoError(t, err)
	assert.Empty(t, second)
}

func TestEvidenceIsPrivacySafe(t *testing.T) {
	svc := newTestService(stubPerformance{returnPct: map[string]float64{"u1": 25}})
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)

	body, err := json.Marshal(list)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"portfolio_id", "position_id", "email", "quantity",
		"cost_basis", "current_value", "average_buy_price", "baseline_price",
	} {
		assert.NotContainsf(t, string(body), `"`+forbidden+`"`, "response leaked %s", forbidden)
	}
}
