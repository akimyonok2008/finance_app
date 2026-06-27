package profile

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type fakeGlobalRanks map[string]int

func (f fakeGlobalRanks) GetUserRank(_ context.Context, userID string) (int, error) {
	if r, ok := f[userID]; ok {
		return r, nil
	}
	return 0, nil
}

// exploreTestService seeds a small public dataset:
//
//	alpha_wolf (u1) public, weights shown: NVDA 40 / AAPL 35 / BTC-USD 25, rank 2
//	beta_bear  (u2) public, weights shown: NVDA 50 / MSFT 50,             rank 1
//	gamma_goat (u3) public, weights HIDDEN (show_public_weights=false)
//	delta_duck (u4) PRIVATE (is_public=false) -> excluded everywhere
func exploreTestService(t *testing.T) *Service {
	t.Helper()
	ctx := context.Background()
	repo := NewInMemoryRepository()
	now := time.Now().UTC()

	mk := func(id, handle, name string, public, weights bool, ageHours int) {
		require.NoError(t, repo.Create(ctx, Profile{
			UserID: id, Handle: handle, DisplayName: name, StrategyTag: DefaultStrategyTag,
			IsPublic: public, ShowPublicWeights: weights,
			CreatedAt: now.Add(time.Duration(-ageHours) * time.Hour),
			UpdatedAt: now.Add(time.Duration(-ageHours) * time.Hour),
		}))
	}
	mk("u1", "alpha_wolf", "AlphaWolf", true, true, 1)
	mk("u2", "beta_bear", "BetaBear", true, true, 2)
	mk("u3", "gamma_goat", "GammaGoat", true, false, 3)
	mk("u4", "delta_duck", "DeltaDuck", false, true, 4)

	summaries := testSummaries{
		"u1": {
			UserID: "u1", PortfolioID: "p1", CurrentValue: 100, GainLossPercentage: 24.6, PortfolioIndex: 124.6,
			Positions: []portfolio.PositionSummary{
				{PositionID: "x1", Symbol: "NVDA", AssetType: "stock", CurrentValueBase: 40, CurrentPriceCurrency: "USD"},
				{PositionID: "x2", Symbol: "AAPL", AssetType: "stock", CurrentValueBase: 35, CurrentPriceCurrency: "USD"},
				{PositionID: "x3", Symbol: "BTC-USD", AssetType: "crypto", CurrentValueBase: 25, CurrentPriceCurrency: "USD"},
			},
		},
		"u2": {
			UserID: "u2", PortfolioID: "p2", CurrentValue: 100, GainLossPercentage: 10, PortfolioIndex: 110,
			Positions: []portfolio.PositionSummary{
				{PositionID: "y1", Symbol: "NVDA", AssetType: "stock", CurrentValueBase: 50, CurrentPriceCurrency: "USD"},
				{PositionID: "y2", Symbol: "MSFT", AssetType: "stock", CurrentValueBase: 50, CurrentPriceCurrency: "USD"},
			},
		},
		"u3": {
			UserID: "u3", PortfolioID: "p3", CurrentValue: 100, GainLossPercentage: 5, PortfolioIndex: 105,
			Positions: []portfolio.PositionSummary{
				{PositionID: "z1", Symbol: "NVDA", AssetType: "stock", CurrentValueBase: 60, CurrentPriceCurrency: "USD"},
				{PositionID: "z2", Symbol: "AAPL", AssetType: "stock", CurrentValueBase: 40, CurrentPriceCurrency: "USD"},
			},
		},
	}
	users := testUsers{
		"u1": {ID: "u1"}, "u2": {ID: "u2"}, "u3": {ID: "u3"}, "u4": {ID: "u4"},
	}
	svc := NewService(repo, users, summaries)
	svc.SetGlobalRankProvider(fakeGlobalRanks{"u1": 2, "u2": 1})
	return svc
}

func TestExploreDefaultResponse(t *testing.T) {
	svc := exploreTestService(t)
	out, err := svc.Explore(context.Background(), "", defaultFilter())
	require.NoError(t, err)

	// Default sort=top: ranked profiles first (beta_bear rank1, alpha_wolf rank2),
	// then unranked gamma_goat. delta_duck (private) is excluded.
	require.Len(t, out.TopPerformers, 3)
	assert.Equal(t, "beta_bear", out.TopPerformers[0].Handle)
	assert.Equal(t, "alpha_wolf", out.TopPerformers[1].Handle)
	assert.Equal(t, "gamma_goat", out.TopPerformers[2].Handle)

	assert.Len(t, out.Featured, 3)
	assert.NotEmpty(t, out.TrendingHoldings)
	assert.Equal(t, 3, out.Pagination.Total)
	assert.False(t, out.Pagination.HasMore)
}

func TestExplorePrivacyForbiddenKeys(t *testing.T) {
	svc := exploreTestService(t)
	out, err := svc.Explore(context.Background(), "", defaultFilter())
	require.NoError(t, err)

	body, err := json.Marshal(out)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"quantity", "quantities", "average_buy_price", "avg_buy_price", "cost_basis",
		"current_value", "portfolio_value", "total_cost_basis", "gain_loss",
		"absolute_gain_loss", "user_id", "portfolio_id", "position_id", "email",
		"password", "password_hash", "brokerage", "deposits", "withdrawals",
		"starting_value", "baseline_value", "sprint_snapshot", "snapshot_details",
	} {
		assert.NotContainsf(t, string(body), `"`+forbidden+`"`, "forbidden key %q present", forbidden)
	}
}

func TestExploreExcludesPrivateProfiles(t *testing.T) {
	svc := exploreTestService(t)
	out, err := svc.Explore(context.Background(), "", defaultFilter())
	require.NoError(t, err)
	for _, c := range out.TopPerformers {
		assert.NotEqual(t, "delta_duck", c.Handle)
	}
	assert.Equal(t, 3, out.Pagination.Total)
}

func TestExploreHiddenWeights(t *testing.T) {
	svc := exploreTestService(t)
	out, err := svc.Explore(context.Background(), "", defaultFilter())
	require.NoError(t, err)

	var gamma *PublicProfile
	for i := range out.TopPerformers {
		if out.TopPerformers[i].Handle == "gamma_goat" {
			gamma = &out.TopPerformers[i]
		}
	}
	require.NotNil(t, gamma, "hidden-weight profile should still appear")
	assert.Empty(t, gamma.PublicWeights)

	// gamma_goat holds NVDA & AAPL privately; those must not inflate trending.
	nvda := findHolding(out.TrendingHoldings, "NVDA")
	require.NotNil(t, nvda)
	assert.Equal(t, 2, nvda.ProfileCount) // alpha_wolf + beta_bear only
	aapl := findHolding(out.TrendingHoldings, "AAPL")
	require.NotNil(t, aapl)
	assert.Equal(t, 1, aapl.ProfileCount) // alpha_wolf only
}

func TestExploreSearch(t *testing.T) {
	svc := exploreTestService(t)
	ctx := context.Background()

	byHandle, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Query = "alpha" }))
	require.NoError(t, err)
	require.Len(t, byHandle.TopPerformers, 1)
	assert.Equal(t, "alpha_wolf", byHandle.TopPerformers[0].Handle)

	byName, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Query = "betabear" }))
	require.NoError(t, err)
	require.Len(t, byName.TopPerformers, 1)
	assert.Equal(t, "beta_bear", byName.TopPerformers[0].Handle)

	none, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Query = "zzzznomatch" }))
	require.NoError(t, err)
	assert.Empty(t, none.TopPerformers)
	assert.Empty(t, none.Featured)
}

func TestExploreSymbolFilter(t *testing.T) {
	svc := exploreTestService(t)
	ctx := context.Background()

	nvda, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Symbol = "NVDA" }))
	require.NoError(t, err)
	handles := handlesOf(nvda.TopPerformers)
	assert.ElementsMatch(t, []string{"alpha_wolf", "beta_bear"}, handles)
	assert.NotContains(t, handles, "gamma_goat") // holds NVDA privately

	msft, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Symbol = "MSFT" }))
	require.NoError(t, err)
	assert.Equal(t, []string{"beta_bear"}, handlesOf(msft.TopPerformers))
}

func TestExploreInvalidSymbolRejected(t *testing.T) {
	_, err := ParseExploreFilter(mapGet(map[string]string{"symbol": "bad symbol!"}))
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestExploreSorting(t *testing.T) {
	svc := exploreTestService(t)
	ctx := context.Background()

	byReturn, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Sort = SortReturn }))
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha_wolf", "beta_bear", "gamma_goat"}, handlesOf(byReturn.TopPerformers))

	byRank, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Sort = SortRank }))
	require.NoError(t, err)
	// beta_bear rank1, alpha_wolf rank2, gamma_goat unranked (last).
	assert.Equal(t, []string{"beta_bear", "alpha_wolf", "gamma_goat"}, handlesOf(byRank.TopPerformers))

	byTop, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Sort = SortTop }))
	require.NoError(t, err)
	assert.Equal(t, "beta_bear", byTop.TopPerformers[0].Handle)
}

func TestExploreInvalidSortRejected(t *testing.T) {
	_, err := ParseExploreFilter(mapGet(map[string]string{"sort": "wealthiest"}))
	assert.ErrorIs(t, err, ErrInvalid)
}

func TestExplorePagination(t *testing.T) {
	svc := exploreTestService(t)
	ctx := context.Background()

	first, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Limit = 1 }))
	require.NoError(t, err)
	require.Len(t, first.TopPerformers, 1)
	assert.Equal(t, "beta_bear", first.TopPerformers[0].Handle)
	assert.Equal(t, 3, first.Pagination.Total)
	assert.True(t, first.Pagination.HasMore)

	second, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Limit = 1; f.Offset = 1 }))
	require.NoError(t, err)
	require.Len(t, second.TopPerformers, 1)
	assert.Equal(t, "alpha_wolf", second.TopPerformers[0].Handle)
	assert.True(t, second.Pagination.HasMore)

	last, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Limit = 1; f.Offset = 2 }))
	require.NoError(t, err)
	require.Len(t, last.TopPerformers, 1)
	assert.False(t, last.Pagination.HasMore)

	beyond, err := svc.Explore(ctx, "", filterWith(func(f *ExploreFilter) { f.Offset = 50 }))
	require.NoError(t, err)
	assert.Empty(t, beyond.TopPerformers)
	assert.Equal(t, 3, beyond.Pagination.Total)
}

func TestExploreLimitClampedToMax(t *testing.T) {
	f, err := ParseExploreFilter(mapGet(map[string]string{"limit": "999"}))
	require.NoError(t, err)
	assert.Equal(t, maxExploreLimit, f.Limit)
}

func TestExploreTrendingHoldings(t *testing.T) {
	svc := exploreTestService(t)
	out, err := svc.Explore(context.Background(), "", defaultFilter())
	require.NoError(t, err)

	// Expected ordering: NVDA (count 2) first, then count-1 symbols by avg weight
	// desc: MSFT(50) > AAPL(35) > BTC-USD(25).
	require.Len(t, out.TrendingHoldings, 4)
	assert.Equal(t, "NVDA", out.TrendingHoldings[0].Symbol)
	assert.Equal(t, 2, out.TrendingHoldings[0].ProfileCount)
	assert.InDelta(t, 45.0, out.TrendingHoldings[0].AverageWeight, 0.001) // (40+50)/2
	assert.Equal(t, 2, out.TrendingHoldings[0].Top10Count)                // held by both ranked profiles
	assert.Equal(t, "stock", out.TrendingHoldings[0].AssetType)

	assert.Equal(t, []string{"NVDA", "MSFT", "AAPL", "BTC-USD"}, symbolsOf(out.TrendingHoldings))
}

func TestExploreSimilarRanksByWeightOverlap(t *testing.T) {
	svc := exploreTestService(t)
	// Caller u2 (beta_bear) holds NVDA 50 / MSFT 50. alpha_wolf shares NVDA
	// (overlap 40); gamma_goat shares only the strategy_tag (hidden weights).
	out, err := svc.Explore(context.Background(), "u2", defaultFilter())
	require.NoError(t, err)

	handles := handlesOf(out.Similar)
	assert.NotContains(t, handles, "beta_bear", "caller is never their own 'similar'")
	require.NotEmpty(t, out.Similar)
	assert.Equal(t, "alpha_wolf", out.Similar[0].Handle, "weight overlap ranks first")
}

func TestExploreSimilarEmptyForAnonymousCaller(t *testing.T) {
	svc := exploreTestService(t)
	out, err := svc.Explore(context.Background(), "", defaultFilter())
	require.NoError(t, err)
	assert.Empty(t, out.Similar)
}

func TestExploreEmptyData(t *testing.T) {
	svc := NewService(NewInMemoryRepository(), testUsers{}, testSummaries{})
	out, err := svc.Explore(context.Background(), "", defaultFilter())
	require.NoError(t, err)
	assert.Empty(t, out.Featured)
	assert.Empty(t, out.TopPerformers)
	assert.Empty(t, out.TrendingHoldings)
	assert.Equal(t, 0, out.Pagination.Total)
	assert.False(t, out.Pagination.HasMore)
}

// --- helpers ---

func defaultFilter() ExploreFilter {
	return ExploreFilter{Sort: SortTop, Limit: defaultExploreLimit}
}

func filterWith(mut func(*ExploreFilter)) ExploreFilter {
	f := defaultFilter()
	mut(&f)
	return f
}

func mapGet(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func handlesOf(cards []PublicProfile) []string {
	out := make([]string, 0, len(cards))
	for _, c := range cards {
		out = append(out, c.Handle)
	}
	return out
}

func symbolsOf(holdings []TrendingHolding) []string {
	out := make([]string, 0, len(holdings))
	for _, h := range holdings {
		out = append(out, h.Symbol)
	}
	return out
}

func findHolding(holdings []TrendingHolding, symbol string) *TrendingHolding {
	for i := range holdings {
		if holdings[i].Symbol == symbol {
			return &holdings[i]
		}
	}
	return nil
}
