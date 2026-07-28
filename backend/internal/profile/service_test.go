package profile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type testUsers map[string]*auth.User

func (u testUsers) GetUserByID(_ context.Context, id string) (*auth.User, error) {
	user, ok := u[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return user, nil
}

type testSummaries map[string]*portfolio.PortfolioSummary

func (s testSummaries) GetSummary(_ context.Context, id string) (*portfolio.PortfolioSummary, error) {
	if summary, ok := s[id]; ok {
		return summary, nil
	}
	return &portfolio.PortfolioSummary{PortfolioIndex: 100, Positions: []portfolio.PositionSummary{}}, nil
}

// countingSummaries counts SummaryProvider calls so tests can prove the
// expensive full summary is (or is not) invoked.
type countingSummaries struct {
	calls *int
	data  map[string]*portfolio.PortfolioSummary
}

func (s countingSummaries) GetSummary(_ context.Context, id string) (*portfolio.PortfolioSummary, error) {
	*s.calls++
	if summary, ok := s.data[id]; ok {
		return summary, nil
	}
	return &portfolio.PortfolioSummary{PortfolioIndex: 100, Positions: []portfolio.PositionSummary{}}, nil
}

// countingWeights is a PublicWeightsProvider stub with its own call counter.
type countingWeights struct {
	calls *int
	data  map[string]*portfolio.PortfolioSummary
}

func (w countingWeights) GetPublicWeights(_ context.Context, id string) (*portfolio.PortfolioSummary, error) {
	*w.calls++
	if summary, ok := w.data[id]; ok {
		return summary, nil
	}
	return &portfolio.PortfolioSummary{PortfolioIndex: 100, Positions: []portfolio.PositionSummary{}}, nil
}

type testHistory map[string][]PublicPerformancePoint

func (h testHistory) RankedHistory(_ context.Context, userID string, _, _ time.Time) ([]PublicPerformancePoint, error) {
	return append([]PublicPerformancePoint(nil), h[userID]...), nil
}

type testRanked map[string]RankedPerformance

func (r testRanked) CurrentRankedPerformance(_ context.Context, userID string) (RankedPerformance, error) {
	if performance, ok := r[userID]; ok {
		return performance, nil
	}
	return RankedPerformance{RankedIndex: 100}, nil
}

type failingRanked struct{ err error }

func (f failingRanked) CurrentRankedPerformance(context.Context, string) (RankedPerformance, error) {
	return RankedPerformance{}, f.err
}

func useTestRanked(svc *Service) {
	svc.SetRankedPerformanceProvider(testRanked{
		"u1": {RankedIndex: 121.5, RankedReturnPercentage: 21.5},
		"u2": {RankedIndex: 100, RankedReturnPercentage: 0},
	})
}

func testService() *Service {
	svc := NewService(NewInMemoryRepository(), testUsers{
		"u1": {ID: "u1", Email: "private@example.com", DisplayName: "Alpha User", AvatarKey: "blue"},
		"u2": {ID: "u2", Email: "other@example.com", DisplayName: "Beta User", AvatarKey: "green"},
	}, testSummaries{
		"u1": {
			UserID: "u1", PortfolioID: "secret-portfolio", CurrentValue: money.AmountFromFloat64(1000),
			TotalCostBasis: money.AmountFromFloat64(800), GainLoss: money.AmountFromFloat64(200), GainLossPercentage: 25, PortfolioIndex: 125,
			Positions: []portfolio.PositionSummary{
				{PositionID: "secret-position", Symbol: "AAPL", AssetType: "stock", Quantity: money.QuantityFromFloat64(10), AverageBuyPrice: money.PriceFromFloat64(50), CostBasisBase: money.AmountFromFloat64(480), CurrentValueBase: money.AmountFromFloat64(700), GainLossBase: money.AmountFromFloat64(220), CurrentPriceCurrency: "USD"},
				{PositionID: "secret-position-2", Symbol: "BTC-USD", AssetType: "crypto", Quantity: money.QuantityFromFloat64(1), AverageBuyPrice: money.PriceFromFloat64(100), CostBasisBase: money.AmountFromFloat64(320), CurrentValueBase: money.AmountFromFloat64(300), GainLossBase: money.AmountFromFloat64(-20), CurrentPriceCurrency: "USD"},
			},
			ActiveCostBasisBase:    money.AmountFromFloat64(800),
			ActiveCurrentValueBase: money.AmountFromFloat64(1000),
			UnrealizedGainLossBase: money.AmountFromFloat64(200),
			RealizedGainLossBase:   money.AmountFromFloat64(100),
			ClosedPositions: []portfolio.ClosedPositionSummary{
				{
					ID: "secret-closed-position", Symbol: "MSFT", AssetType: "stock",
					Quantity: money.QuantityFromFloat64(5), BaselinePrice: money.PriceFromFloat64(90), BaselineCurrency: "USD",
					ClosePrice: money.PriceFromFloat64(110), ClosePriceCurrency: "USD", ClosedAt: "2026-07-06T00:00:00Z",
					RealizedGainLossBase: money.AmountFromFloat64(100), RealizedGainLossPercentage: 22.22,
					ClosedCostBasisBase: money.AmountFromFloat64(450), BaseCurrency: "USD",
				},
			},
		},
	})
	useTestRanked(svc)
	svc.SetPerformanceHistoryProvider(testHistory{
		"u1": {
			{CapturedAt: "2026-07-01T00:00:00Z", PortfolioIndex: 118.12, ReturnPercentage: 18.12},
			{CapturedAt: "2026-07-07T00:00:00Z", PortfolioIndex: 125, ReturnPercentage: 25},
		},
	})
	return svc
}

func TestValidation(t *testing.T) {
	valid := Profile{Handle: "alpha_user", DisplayName: "Alpha", StrategyTag: DefaultStrategyTag}
	require.NoError(t, ValidateProfile(valid))

	tests := []struct {
		name   string
		mutate func(*Profile)
	}{
		{"reserved handle", func(p *Profile) { p.Handle = "admin" }},
		{"invalid handle", func(p *Profile) { p.Handle = "Bad Handle" }},
		{"short name", func(p *Profile) { p.DisplayName = "A" }},
		{"long bio", func(p *Profile) { p.Bio = strings.Repeat("x", 161) }},
		{"long avatar", func(p *Profile) { p.AvatarKey = strings.Repeat("x", 41) }},
		{"invalid strategy", func(p *Profile) { p.StrategyTag = "all_in" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := valid
			tt.mutate(&p)
			assert.Error(t, ValidateProfile(p))
		})
	}
}

func TestProfilesDefaultPrivateUpdateAndHandleConflict(t *testing.T) {
	ctx := context.Background()
	svc := testService()

	first, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, first.IsPublic)
	assert.False(t, first.ShowPublicWeights)
	assert.Equal(t, "alpha_user", first.Handle)

	second, err := svc.GetMe(ctx, "u2")
	require.NoError(t, err)
	public := true
	weights := true
	handle := second.Handle
	_, err = svc.UpdateMe(ctx, "u1", UpdateInput{Handle: &handle, IsPublic: &public, ShowPublicWeights: &weights})
	assert.ErrorIs(t, err, ErrHandleExists)

	handle = "alpha_investor"
	updated, err := svc.UpdateMe(ctx, "u1", UpdateInput{Handle: &handle, IsPublic: &public, ShowPublicWeights: &weights})
	require.NoError(t, err)
	assert.Equal(t, "alpha_investor", updated.Handle)
	assert.True(t, updated.IsPublic)
	assert.Len(t, updated.PublicPreview.PublicWeights, 2)
}

// TestOnAccountDeleted_UnpublishesProfile: deleting an account must stop its
// profile from resolving via a direct handle lookup (GetPublic reads the
// profile repository directly and has no idea whether the account still
// exists), even though the profile row itself is left in place.
func TestOnAccountDeleted_UnpublishesProfile(t *testing.T) {
	ctx := context.Background()
	svc := testService()
	owner, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	public, weights := true, true
	owner, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public, ShowPublicWeights: &weights})
	require.NoError(t, err)
	_, err = svc.GetPublic(ctx, "", owner.Handle)
	require.NoError(t, err, "sanity check: the profile is public before deletion")

	require.NoError(t, svc.OnAccountDeleted(ctx, "u1"))

	_, err = svc.GetPublic(ctx, "", owner.Handle)
	assert.ErrorIs(t, err, ErrNotFound, "a deleted account's profile must no longer be publicly resolvable")
}

// TestOnAccountDeleted_NoProfileIsNoOp covers a user who never created a
// profile: deletion must not error just because there's nothing to unpublish.
func TestOnAccountDeleted_NoProfileIsNoOp(t *testing.T) {
	svc := testService()
	assert.NoError(t, svc.OnAccountDeleted(context.Background(), "never-had-a-profile"))
}

func TestPublicProfilePrivacyVisibilityAndHiddenWeights(t *testing.T) {
	ctx := context.Background()
	svc := testService()
	owner, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	_, err = svc.GetPublic(ctx, "", owner.Handle)
	assert.ErrorIs(t, err, ErrNotFound)

	public := true
	hidden := false
	owner, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public, ShowPublicWeights: &hidden})
	require.NoError(t, err)
	out, err := svc.GetPublic(ctx, "", owner.Handle)
	require.NoError(t, err)
	assert.Empty(t, out.PublicWeights)
	assert.Empty(t, out.PublicClosedPositions)
	require.Len(t, out.PerformanceHistory, 2)
	assert.Equal(t, 118.12, out.PerformanceHistory[0].PortfolioIndex)
	assert.Equal(t, 18.12, out.PerformanceHistory[0].ReturnPercentage)
	assert.NotEmpty(t, out.AssetTypeExposure)
	assert.Equal(t, 70.0, out.Concentration.LargestPosition)
	assert.NotEmpty(t, out.Insights.FocusAreas)
	assert.Empty(t, out.Insights.Contributors)
	assert.Empty(t, out.Insights.Detractors)
	assert.False(t, out.Insights.OpenClosedPerformance.CompositionVisible)

	body, err := json.Marshal(out)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"user_id", "portfolio_id", "position_id", "email", "password", "quantity",
		"average_buy_price", "baseline_price", "close_price", "cost_basis",
		"current_value", "gain_loss", "realized_gain_loss",
	} {
		assert.NotContains(t, string(body), `"`+forbidden+`":`)
	}
	assert.NotContains(t, string(body), "private@example.com")
	assert.NotContains(t, string(body), "secret-portfolio")
}

func TestPublicProfilePerformanceComesOnlyFromRankedProvider(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	require.NoError(t, repo.Create(ctx, Profile{
		UserID: "u1", Handle: "ranked_user", DisplayName: "Ranked User",
		StrategyTag: DefaultStrategyTag, IsPublic: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	svc := NewService(repo, testUsers{"u1": {ID: "u1"}}, testSummaries{
		"u1": {PortfolioIndex: 187, GainLossPercentage: 87},
	})
	svc.SetRankedPerformanceProvider(testRanked{
		"u1": {RankedIndex: 112.34, RankedReturnPercentage: 12.34},
	})

	out, err := svc.GetPublic(ctx, "", "ranked_user")
	require.NoError(t, err)
	assert.Equal(t, 112.34, out.PortfolioIndex)
	assert.Equal(t, 12.34, out.ReturnPercentage)
}

func TestPublicProfileFailsClosedWhenRankedDataUnavailable(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	require.NoError(t, repo.Create(ctx, Profile{
		UserID: "u1", Handle: "ranked_user", DisplayName: "Ranked User",
		StrategyTag: DefaultStrategyTag, IsPublic: true,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}))
	summaries := testSummaries{
		"u1": {PortfolioIndex: 187, GainLossPercentage: 87},
	}

	t.Run("provider error", func(t *testing.T) {
		svc := NewService(repo, testUsers{"u1": {ID: "u1"}}, summaries)
		svc.SetRankedPerformanceProvider(failingRanked{err: errors.New("ranked store offline")})

		out, err := svc.GetPublic(ctx, "", "ranked_user")
		assert.ErrorIs(t, err, ErrRankedDataUnavailable)
		assert.Equal(t, PublicProfile{}, out)
	})

	t.Run("provider not configured", func(t *testing.T) {
		svc := NewService(repo, testUsers{"u1": {ID: "u1"}}, summaries)

		out, err := svc.GetPublic(ctx, "", "ranked_user")
		assert.ErrorIs(t, err, ErrRankedDataUnavailable)
		assert.Equal(t, PublicProfile{}, out)
	})
}

// TestPublicInfoForUser_PrefersCheapWeightsProvider is the leaderboard
// enrichment path (PublicInfoForUser). When a PublicWeightsProvider is wired,
// it must be used instead of the full ledger-scanning SummaryProvider: that
// full scan is what makes enriching every public-weights leaderboard row
// expensive, and it computes nothing PublicInfoForUser actually needs.
func TestPublicInfoForUser_PrefersCheapWeightsProvider(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	users := testUsers{"u1": {ID: "u1", Email: "a@example.com", DisplayName: "Alpha User", AvatarKey: "blue"}}

	var fullCalls int
	full := countingSummaries{calls: &fullCalls, data: map[string]*portfolio.PortfolioSummary{
		"u1": {CurrentValue: money.AmountFromFloat64(1000), Positions: []portfolio.PositionSummary{
			{Symbol: "AAPL", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(1000), CurrentPriceCurrency: "USD"},
		}},
	}}
	var cheapCalls int
	cheap := countingWeights{calls: &cheapCalls, data: map[string]*portfolio.PortfolioSummary{
		"u1": {CurrentValue: money.AmountFromFloat64(500), Positions: []portfolio.PositionSummary{
			{Symbol: "MSFT", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(500), CurrentPriceCurrency: "USD"},
		}},
	}}

	svc := NewService(repo, users, full)
	useTestRanked(svc)
	svc.SetPublicWeightsProvider(cheap)

	_, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	public, weights := true, true
	_, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public, ShowPublicWeights: &weights})
	require.NoError(t, err)

	fullCalls, cheapCalls = 0, 0 // isolate PublicInfoForUser's own calls from UpdateMe's preview
	info, ok, err := svc.PublicInfoForUser(ctx, "u1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, info.Weights, 1)
	assert.Equal(t, "MSFT", info.Weights[0].Symbol, "the cheap weights provider's data must win when it is wired")
	assert.Equal(t, 1, cheapCalls)
	assert.Equal(t, 0, fullCalls, "the full ledger-scanning SummaryProvider must not run when the cheap provider is available")
}

// TestPublicInfoForUser_FallsBackToSummaryProviderWhenUnwired covers a
// PublicWeightsProvider never being set (e.g. an older wiring): the leaderboard
// enrichment must still work correctly via the original SummaryProvider path.
func TestPublicInfoForUser_FallsBackToSummaryProviderWhenUnwired(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	users := testUsers{"u1": {ID: "u1", Email: "a@example.com", DisplayName: "Alpha User", AvatarKey: "blue"}}
	full := testSummaries{"u1": {CurrentValue: money.AmountFromFloat64(1000), Positions: []portfolio.PositionSummary{
		{Symbol: "AAPL", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(1000), CurrentPriceCurrency: "USD"},
	}}}
	svc := NewService(repo, users, full)
	useTestRanked(svc)
	// SetPublicWeightsProvider intentionally left unset.

	_, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	public, weights := true, true
	_, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public, ShowPublicWeights: &weights})
	require.NoError(t, err)

	info, ok, err := svc.PublicInfoForUser(ctx, "u1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, info.Weights, 1)
	assert.Equal(t, "AAPL", info.Weights[0].Symbol)
}

// TestExploreSummaryProvider_IsIsolatedFromOwnerAndPublicViews: a caching
// decorator in front of exploreSummaryProvider must only affect Explore's own
// caller-comparison lookup — GetMe (the owner's own preview) and GetPublic (a
// public profile page) must keep reading the UNCACHED shared SummaryProvider,
// since both are reasonably expected to reflect a caller's own just-made
// changes immediately.
func TestExploreSummaryProvider_IsIsolatedFromOwnerAndPublicViews(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	users := testUsers{"u1": {ID: "u1", Email: "a@example.com", DisplayName: "Alpha User", AvatarKey: "blue"}}

	shared := testSummaries{"u1": {CurrentValue: money.AmountFromFloat64(1000), PortfolioIndex: 100, Positions: []portfolio.PositionSummary{
		{Symbol: "AAPL", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(1000), CurrentPriceCurrency: "USD"},
	}}}
	exploreOnly := testSummaries{"u1": {CurrentValue: money.AmountFromFloat64(2000), PortfolioIndex: 100, Positions: []portfolio.PositionSummary{
		{Symbol: "MSFT", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(2000), CurrentPriceCurrency: "USD"},
	}}}

	svc := NewService(repo, users, shared)
	useTestRanked(svc)
	svc.SetExploreSummaryProvider(exploreOnly)

	_, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	public, weights := true, true
	_, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public, ShowPublicWeights: &weights})
	require.NoError(t, err)

	// exploreSummaryProvider() must resolve to the dedicated provider...
	resolved, err := svc.exploreSummaryProvider().GetSummary(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, resolved.Positions, 1)
	assert.Equal(t, "MSFT", resolved.Positions[0].Symbol)

	// ...while GetMe (owner's own preview) still reads the SHARED provider.
	me, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, me.PublicPreview.PublicWeights, 1)
	assert.Equal(t, "AAPL", me.PublicPreview.PublicWeights[0].Symbol)
}

// TestExploreSummaryProvider_DefaultsToSharedProviderWhenUnset covers the
// backward-compatible default: no dedicated Explore provider means Explore
// uses the same SummaryProvider as everything else.
func TestExploreSummaryProvider_DefaultsToSharedProviderWhenUnset(t *testing.T) {
	svc := testService()
	// SetExploreSummaryProvider intentionally left unset.
	resolved, err := svc.exploreSummaryProvider().GetSummary(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, 1000.0, resolved.CurrentValue.Float64())
}

// TestPublicProfile_DisclosesSelfReportedExecutionPrices: the ranked index
// (PortfolioIndex/ReturnPercentage) is always priced from tracked market
// quotes, but open/closed holdings P&L is built directly from whatever
// execution price the user entered — a public viewer must be able to tell
// when those two figures include an unverifiable, self-reported price.
func TestPublicProfile_DisclosesSelfReportedExecutionPrices(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	users := testUsers{"u1": {ID: "u1", Email: "a@example.com", DisplayName: "Alpha User", AvatarKey: "blue"}}
	flagged := testSummaries{"u1": {
		CurrentValue: money.AmountFromFloat64(1000), PortfolioIndex: 110, ActiveCostBasisBase: money.AmountFromFloat64(800), ActiveCurrentValueBase: money.AmountFromFloat64(1000),
		UnrealizedGainLossBase: money.AmountFromFloat64(200), HasSelfReportedExecutionPrice: true,
		Positions: []portfolio.PositionSummary{
			{Symbol: "AAPL", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(1000), CurrentPriceCurrency: "USD"},
		},
	}}
	svc := NewService(repo, users, flagged)
	useTestRanked(svc)

	_, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	public := true
	_, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public})
	require.NoError(t, err)

	out, err := svc.GetPublic(ctx, "", "alpha_user")
	require.NoError(t, err)
	assert.True(t, out.Insights.OpenClosedPerformance.IncludesSelfReportedPrices)
}

func TestPublicProfile_DoesNotFlagWhenNoSelfReportedPrices(t *testing.T) {
	ctx := context.Background()
	svc := testService() // default stub summaries never set HasSelfReportedExecutionPrice
	owner, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	public := true
	_, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public})
	require.NoError(t, err)

	out, err := svc.GetPublic(ctx, "", owner.Handle)
	require.NoError(t, err)
	assert.False(t, out.Insights.OpenClosedPerformance.IncludesSelfReportedPrices)
}

func TestPublicProfileClosedPositionsAreSafeWhenWeightsVisible(t *testing.T) {
	ctx := context.Background()
	svc := testService()
	owner, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)

	public := true
	weights := true
	owner, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public, ShowPublicWeights: &weights})
	require.NoError(t, err)
	out, err := svc.GetPublic(ctx, "", owner.Handle)
	require.NoError(t, err)

	require.Len(t, out.PublicClosedPositions, 1)
	closed := out.PublicClosedPositions[0]
	assert.Equal(t, "MSFT", closed.Symbol)
	assert.Equal(t, "stock", closed.AssetType)
	assert.Equal(t, "2026-07-06T00:00:00Z", closed.ClosedAt)
	assert.Equal(t, 22.22, closed.ReturnPercentage)
	assert.True(t, out.Insights.OpenClosedPerformance.CompositionVisible)
	require.NotEmpty(t, out.Insights.Contributors)
	assert.Equal(t, "AAPL", out.Insights.Contributors[0].Symbol)
	require.NotEmpty(t, out.Insights.Detractors)
	assert.Equal(t, "BTC-USD", out.Insights.Detractors[0].Symbol)

	body, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(body), "public_closed_positions")
	assert.NotContains(t, string(body), "secret-closed-position")
	assert.NotContains(t, string(body), `"quantity":`)
	assert.NotContains(t, string(body), `"close_price":`)
	assert.NotContains(t, string(body), `"realized_gain_loss_base":`)
}

func TestEmptyPortfolioProjectionIsStable(t *testing.T) {
	svc := testService()
	out, err := svc.GetMe(context.Background(), "u2")
	require.NoError(t, err)
	assert.Equal(t, 100.0, out.PublicPreview.PortfolioIndex)
	assert.NotNil(t, out.PublicPreview.PublicWeights)
	assert.NotNil(t, out.PublicPreview.AssetTypeExposure)
	assert.NotNil(t, out.PublicPreview.CurrencyExposure)
}
