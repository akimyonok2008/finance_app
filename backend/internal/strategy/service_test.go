package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
	"github.com/ardakimyonok/finance_app/internal/profile"
)

type fakePublicStrategies struct {
	byHandle map[string]profile.PublicStrategy
}

func (f fakePublicStrategies) PublicStrategyByHandle(_ context.Context, handle string) (profile.PublicStrategy, error) {
	if p, ok := f.byHandle[handle]; ok {
		return p, nil
	}
	return profile.PublicStrategy{}, profile.ErrNotFound
}

func copyTestService() (*Service, *portfolio.Service) {
	pf := portfolio.NewService(portfolio.NewInMemoryRepository(), prices.NewMockPriceProvider(), fx.NewMockFXProvider())
	profiles := fakePublicStrategies{byHandle: map[string]profile.PublicStrategy{
		"alpha_wolf": {
			UserID: "source", Handle: "alpha_wolf", DisplayName: "AlphaWolf", AvatarKey: "fox", StrategyTag: "growth",
			Weights: []profile.StrategyWeight{
				{Symbol: "NVDA", AssetType: "stock", WeightPercentage: 60},
				{Symbol: "SPY", AssetType: "etf", WeightPercentage: 40},
			},
		},
		"self": {
			UserID: "caller", Handle: "self", DisplayName: "Me", StrategyTag: "growth",
			Weights: []profile.StrategyWeight{{Symbol: "AAPL", AssetType: "stock", WeightPercentage: 100}},
		},
	}}
	return NewService(profiles, pf), pf
}

func TestCopyPreviewPrivacy(t *testing.T) {
	svc, _ := copyTestService()

	resp, err := svc.CopyPreview(context.Background(), "caller", "alpha_wolf")
	require.NoError(t, err)
	require.Len(t, resp.Weights, 2)
	assert.Equal(t, CopyDisclaimer, resp.Disclaimer)

	body, err := json.Marshal(resp)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"quantity", "average_buy_price", "cost_basis", "current_value", "portfolio_value",
		"user_id", "portfolio_id", "position_id", "email", "password", "baseline_value",
	} {
		assert.NotContains(t, string(body), `"`+forbidden+`":`)
	}
}

func TestCopyFromProfileCreatesFreshBaseline(t *testing.T) {
	svc, pf := copyTestService()
	ctx := context.Background()
	_, err := pf.AddPosition(ctx, "caller", portfolio.PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: money.QuantityFromFloat64(10)})
	require.NoError(t, err)

	resp, err := svc.CopyFromProfile(ctx, "caller", CopyFromProfileRequest{
		Handle:  "alpha_wolf",
		Weights: []Weight{{Symbol: "NVDA", AssetType: "stock", WeightPercentage: 60}, {Symbol: "SPY", AssetType: "etf", WeightPercentage: 40}},
	})
	require.NoError(t, err)
	assert.Equal(t, "alpha_wolf", resp.SourceProfile.Handle)

	summary, err := pf.Summary(ctx, "caller")
	require.NoError(t, err)
	assert.InDelta(t, 100.0, summary.PortfolioIndex, 0.001)
	assert.InDelta(t, 0.0, summary.GainLossPercentage, 0.001)
	require.Len(t, summary.Positions, 2)

	weights := map[string]float64{}
	for _, pos := range summary.Positions {
		weights[pos.Symbol] = pos.CurrentValueBase / summary.CurrentValue * 100
	}
	assert.InDelta(t, 60.0, weights["NVDA"], 0.01)
	assert.InDelta(t, 40.0, weights["SPY"], 0.01)
}

func TestCopyRejectsSelfCopyAndInvalidWeights(t *testing.T) {
	svc, _ := copyTestService()
	_, err := svc.CopyPreview(context.Background(), "caller", "self")
	assert.ErrorIs(t, err, ErrSelfCopy)

	_, err = svc.CopyFromProfile(context.Background(), "caller", CopyFromProfileRequest{
		Handle: "alpha_wolf",
		Weights: []Weight{
			{Symbol: "NVDA", AssetType: "stock", WeightPercentage: 80},
			{Symbol: "SPY", AssetType: "etf", WeightPercentage: 10},
		},
	})
	assert.ErrorIs(t, err, portfolio.ErrInvalidWeights)

	_, err = svc.CopyPreview(context.Background(), "caller", "missing")
	assert.True(t, errors.Is(err, ErrNotFound))
}

func TestCompareProfileUsesPublicWeights(t *testing.T) {
	svc, pf := copyTestService()
	ctx := context.Background()
	_, err := pf.AddPosition(ctx, "caller", portfolio.PositionInput{Symbol: "NVDA", AssetType: "stock", Quantity: money.QuantityFromFloat64(1)})
	require.NoError(t, err)
	_, err = pf.AddPosition(ctx, "caller", portfolio.PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: money.QuantityFromFloat64(1)})
	require.NoError(t, err)

	resp, err := svc.CompareProfile(ctx, "caller", "alpha_wolf")
	require.NoError(t, err)
	assert.Equal(t, "alpha_wolf", resp.TargetProfile.Handle)
	assert.Contains(t, resp.SharedSymbols, "NVDA")
	assert.Greater(t, resp.OverlapScore, 0.0)
	assert.NotEmpty(t, resp.WeightDifferences)
	assert.Equal(t, "This is educational comparison, not investment advice.", resp.Disclaimer)
}
