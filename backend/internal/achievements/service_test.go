package achievements

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
)

// stubPerformance returns a two-point index series describing a total return
// over whatever window is requested.
type stubPerformance struct {
	returnPct map[string]float64 // userID -> total return %
	hasData   map[string]bool
}

// shortHistoryPerformance returns a strong-return series that only spans a few
// days ending now, regardless of the requested window — simulating a portfolio
// too young to cover any badge period.
type shortHistoryPerformance struct{}

type fixedHistory struct {
	window performancehistory.Window
}

func (f fixedHistory) Window(context.Context, string, time.Time, time.Time) (performancehistory.Window, error) {
	return f.window, nil
}
func (fixedHistory) ProtectEvidence(context.Context, ...string) error { return nil }
func (fixedHistory) EligibilityThreshold() float64                    { return .9 }
func (fixedHistory) SnapshotFrequency() string                        { return "4h0m0s" }

// --- adapters to the ranked-history contract ---------------------------------
// These stubs describe a ranked-index series; the helpers below project it into
// the performancehistory.Window shape the achievements service consumes.

func windowFrom(startIdx, endIdx float64, start, end time.Time) performancehistory.Window {
	epoch := start.Add(-24 * time.Hour)
	mk := func(id string, at time.Time, idx float64) performancehistory.Snapshot {
		return performancehistory.Snapshot{
			ID: id, UserID: "u1", PortfolioID: "pf1", CapturedAt: at,
			RankedIndex: idx, RankingStatus: performance.StatusActive,
			TrackingStartedAt: epoch, DataQualityStatus: "complete",
		}
	}
	startSnap := mk("s-start", start, startIdx)
	endSnap := mk("s-end", end, endIdx)
	return performancehistory.Window{
		StartSnapshot: startSnap, EndSnapshot: endSnap,
		Points:          []performancehistory.Snapshot{startSnap, endSnap},
		HistoryCoverage: 1, ActiveCoverage: 1, TrustedCoverage: 1,
	}
}

func (stubPerformance) ProtectEvidence(_ context.Context, _ ...string) error { return nil }
func (stubPerformance) EligibilityThreshold() float64                        { return 0.9 }
func (stubPerformance) SnapshotFrequency() string                            { return "4h0m0s" }

func (s stubPerformance) Window(_ context.Context, userID string, start, end time.Time) (performancehistory.Window, error) {
	if s.hasData != nil && !s.hasData[userID] {
		return performancehistory.Window{}, nil
	}
	ret := s.returnPct[userID]
	return windowFrom(100, 100*(1+ret/100), start, end), nil
}

func (shortHistoryPerformance) ProtectEvidence(_ context.Context, _ ...string) error { return nil }
func (shortHistoryPerformance) EligibilityThreshold() float64                        { return 0.9 }
func (shortHistoryPerformance) SnapshotFrequency() string                            { return "24h0m0s" }

// shortHistoryPerformance has only 5 days of history: coverage falls far below
// the eligibility threshold, so no badge may unlock however good the return is.
func (shortHistoryPerformance) Window(_ context.Context, _ string, _, end time.Time) (performancehistory.Window, error) {
	w := windowFrom(100, 130, end.AddDate(0, 0, -5), end)
	w.HistoryCoverage, w.ActiveCoverage, w.TrustedCoverage = 0.05, 0.05, 0.05
	return w, nil
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
		// Coverage is below the eligibility threshold, so evaluation stops before
		// any benchmark comparison: the badge reports that it is still building
		// ranked history and exposes no edge figure.
		assert.Equal(t, "building_history", a.Progress.State)
		assert.Greater(t, a.Progress.HistoryCoveragePercentage, 0.0)
		assert.Nil(t, a.Progress.CurrentEdgePoints,
			"no edge may be published until the coverage threshold is met")
	}
}

// verifiedTestPrices is a verified-grade in-test price provider. It reuses the
// deterministic mock price path but labels the data as real, adjusted /
// total-return and fresh, so award-grade evaluation can legitimately produce
// verified awards in tests. It implements both provider ports.
type verifiedTestPrices struct {
	inner *benchmark.MockHistoricalPriceProvider
}

func newVerifiedTestPrices() verifiedTestPrices {
	return verifiedTestPrices{inner: benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns())}
}

func (v verifiedTestPrices) GetAdjustedCloseSeries(ctx context.Context, symbol string, start, end time.Time) ([]benchmark.PricePoint, error) {
	return v.inner.GetAdjustedCloseSeries(ctx, symbol, start, end)
}

func (v verifiedTestPrices) GetSeries(ctx context.Context, symbol string, start, end time.Time, _ benchmark.SeriesRequirement) (benchmark.BenchmarkPriceSeries, error) {
	pts, err := v.inner.GetAdjustedCloseSeries(ctx, symbol, start, end)
	if err != nil {
		return benchmark.BenchmarkPriceSeries{}, err
	}
	now := time.Now().UTC()
	return benchmark.BenchmarkPriceSeries{
		Symbol: symbol, Points: pts,
		Metadata: benchmark.BenchmarkDataMetadata{
			Provider: "test_feed", ProviderMode: "real",
			PriceType:         benchmark.PriceTypeAdjustedClose,
			IncludesDividends: true, IncludesSplits: true,
			IsAdjusted: true, IsTotalReturn: true, CorpActionsKnown: true,
			Quality:     benchmark.DataQualityVerified,
			RetrievedAt: now, SourceAsOf: now,
			Currency: "USD",
		},
	}, nil
}

func newTestService(perf RankedPerformanceHistoryProvider) *Service {
	engine := benchmark.NewBenchmarkConstructionService(
		newVerifiedTestPrices(),
		benchmark.Recipes,
		nil,
	)
	rules := benchmark.NewRulesEngine(benchmark.DefaultEvaluators())
	svc := NewService(NewInMemoryAchievementRepository(), perf, engine, rules)
	svc.SetBenchmarkDataSource("test_feed")
	svc.SetAwardPolicy(benchmark.AwardModeVerifiedOnly, benchmark.EnvironmentProduction)
	return svc
}

// newMockTestService builds a service whose benchmark engine is backed by the
// synthetic mock provider, used to prove synthetic data fails closed.
func newMockTestService(perf RankedPerformanceHistoryProvider, mode benchmark.AwardMode) *Service {
	engine := benchmark.NewBenchmarkConstructionService(
		benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns()),
		benchmark.Recipes,
		nil,
	)
	rules := benchmark.NewRulesEngine(benchmark.DefaultEvaluators())
	svc := NewService(NewInMemoryAchievementRepository(), perf, engine, rules)
	svc.SetBenchmarkDataSource("mock")
	svc.SetAwardPolicy(mode, benchmark.EnvironmentDevelopment)
	return svc
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
		assert.Equal(t, "eligible_but_rule_not_met", a.Progress.State)
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
		assert.Contains(t, a.Progress.Reason, "Building ranked history")
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

func TestMockBenchmarkDataNeverCreatesPermanentAwards(t *testing.T) {
	// Synthetic mock benchmark data under verified_only mode must fail closed: no
	// badge unlocks and progress explains that the data is preview-only.
	svc := newMockTestService(stubPerformance{returnPct: map[string]float64{"u1": 25}}, benchmark.AwardModeVerifiedOnly)

	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	for _, item := range list {
		assert.False(t, item.Unlocked)
		require.NotNil(t, item.Progress)
		assert.Equal(t, "benchmark_unverified", item.Progress.State)
		assert.Contains(t, item.Progress.Reason, "preview-only")
	}
}

func TestMockBenchmarkDataCreatesDemoAwardsInDemoMode(t *testing.T) {
	// In demo mode synthetic data may create awards, but they are persistently
	// marked demo and never verified.
	svc := newMockTestService(stubPerformance{returnPct: map[string]float64{"u1": 25}}, benchmark.AwardModeDemo)

	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	var demoAward *benchmark.AchievementEvidence
	for _, item := range list {
		if item.Unlocked && item.Evidence != nil {
			demoAward = item.Evidence
			break
		}
	}
	require.NotNil(t, demoAward, "demo mode should unlock at least one badge")
	assert.Equal(t, benchmark.AwardVerificationDemo, demoAward.Verification)
	assert.NotEqual(t, benchmark.AwardVerificationVerified, demoAward.Verification)
}

func TestDisabledModeWritesNoAward(t *testing.T) {
	svc := newMockTestService(stubPerformance{returnPct: map[string]float64{"u1": 25}}, benchmark.AwardModeDisabled)
	awarded, err := svc.checkAndAwardBadges(context.Background(), "u1")
	require.NoError(t, err)
	assert.Empty(t, awarded)
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

func TestRankedSnapshotEvidenceIsVersioned(t *testing.T) {
	svc := newTestService(stubPerformance{returnPct: map[string]float64{"u1": 25}})
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	var evidence *benchmark.AchievementEvidence
	for _, item := range list {
		if item.Unlocked && item.Evidence != nil {
			evidence = item.Evidence
			break
		}
	}
	require.NotNil(t, evidence)
	assert.Equal(t, "ranked_snapshot_benchmark_aligned_v2", evidence.EvaluationModel)
	// Evidence v2 carries benchmark data-integrity provenance.
	assert.Equal(t, 3, evidence.EvidenceVersion)
	assert.Equal(t, 100.0, evidence.StartRankedIndex)
	assert.Equal(t, 125.0, evidence.EndRankedIndex)
	assert.Equal(t, "4h0m0s", evidence.SnapshotFrequency)
	assert.NotEmpty(t, evidence.TrackingEpoch)
	assert.NotEmpty(t, evidence.StartSnapshotAt)
	assert.NotEmpty(t, evidence.EndSnapshotAt)
	// Benchmark provenance: verified award, recorded recipe version, price
	// methodology, quality and a deterministic fingerprint.
	assert.Equal(t, benchmark.AwardVerificationVerified, evidence.Verification)
	assert.NotEmpty(t, evidence.BenchmarkRecipeVersion)
	assert.NotEmpty(t, evidence.BenchmarkInputHash)
	require.NotNil(t, evidence.DataSourceSummary)
	assert.Equal(t, "verified", evidence.DataSourceSummary.Quality)
	assert.False(t, evidence.DataSourceSummary.IsSynthetic)
}

func TestPausedPeriodCannotUnlockBadge(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	window := windowFrom(100, 125, now.AddDate(0, 0, -30), now)
	window.ActiveCoverage = .72
	svc := newTestService(fixedHistory{window: window})
	svc.SetClock(func() time.Time { return now })
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	for _, item := range list {
		assert.False(t, item.Unlocked)
		require.NotNil(t, item.Progress)
		assert.Equal(t, "insufficient_active_coverage", item.Progress.State)
		assert.Contains(t, item.Progress.Reason, "72%")
	}
}

func TestUntrustedCoverageFailsClosed(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	window := windowFrom(100, 125, now.AddDate(0, 0, -30), now)
	window.TrustedCoverage = .5
	svc := newTestService(fixedHistory{window: window})
	svc.SetClock(func() time.Time { return now })
	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	for _, item := range list {
		assert.False(t, item.Unlocked)
		require.NotNil(t, item.Progress)
		assert.Equal(t, "insufficient_trusted_data", item.Progress.State)
	}
}

func TestLegacyAwardIsPreservedAndNotRelabelled(t *testing.T) {
	repo := NewInMemoryAchievementRepository()
	require.NoError(t, repo.Award(context.Background(), AwardedAchievement{
		UserID: "u1", BadgeKey: "cash_plus_30d", UnlockedAt: time.Now().UTC(),
		Evidence: benchmark.AchievementEvidence{
			Period: benchmark.Period30D, PortfolioReturnPct: 5,
			BenchmarkReturnPct: 2, EdgePoints: 3,
		},
	}))
	engine := benchmark.NewBenchmarkConstructionService(
		benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns()),
		benchmark.Recipes,
		benchmark.NewSnapshotRecipeResolver(benchmark.DefaultRecipeSnapshots()),
	)
	svc := NewService(repo, stubPerformance{returnPct: map[string]float64{"u1": 25}},
		engine, benchmark.NewRulesEngine(benchmark.DefaultEvaluators()))
	list, err := svc.ListAchievementsForUser(context.Background(), "u1")
	require.NoError(t, err)
	for _, item := range list {
		if item.Key != "cash_plus_30d" {
			continue
		}
		require.True(t, item.Unlocked)
		require.NotNil(t, item.Evidence)
		assert.True(t, item.LegacyEvidence)
		assert.Equal(t, "archive_model_v0", item.Evidence.EvaluationModel)
	}
}
