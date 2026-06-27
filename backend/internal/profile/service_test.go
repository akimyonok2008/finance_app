package profile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
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

func testService() *Service {
	return NewService(NewInMemoryRepository(), testUsers{
		"u1": {ID: "u1", Email: "private@example.com", DisplayName: "Alpha User", AvatarKey: "blue"},
		"u2": {ID: "u2", Email: "other@example.com", DisplayName: "Beta User", AvatarKey: "green"},
	}, testSummaries{
		"u1": {
			UserID: "u1", PortfolioID: "secret-portfolio", CurrentValue: 1000,
			TotalCostBasis: 800, GainLoss: 200, GainLossPercentage: 25, PortfolioIndex: 125,
			Positions: []portfolio.PositionSummary{
				{PositionID: "secret-position", Symbol: "AAPL", AssetType: "stock", Quantity: 10, AverageBuyPrice: 50, CurrentValueBase: 700, CurrentPriceCurrency: "USD"},
				{PositionID: "secret-position-2", Symbol: "BTC-USD", AssetType: "crypto", Quantity: 1, AverageBuyPrice: 100, CurrentValueBase: 300, CurrentPriceCurrency: "USD"},
			},
		},
	})
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

func TestPublicProfilePrivacyVisibilityAndHiddenWeights(t *testing.T) {
	ctx := context.Background()
	svc := testService()
	owner, err := svc.GetMe(ctx, "u1")
	require.NoError(t, err)
	_, err = svc.GetPublic(ctx, owner.Handle)
	assert.ErrorIs(t, err, ErrNotFound)

	public := true
	hidden := false
	owner, err = svc.UpdateMe(ctx, "u1", UpdateInput{IsPublic: &public, ShowPublicWeights: &hidden})
	require.NoError(t, err)
	out, err := svc.GetPublic(ctx, owner.Handle)
	require.NoError(t, err)
	assert.Empty(t, out.PublicWeights)
	assert.NotEmpty(t, out.AssetTypeExposure)
	assert.Equal(t, 70.0, out.Concentration.LargestPosition)

	body, err := json.Marshal(out)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"user_id", "portfolio_id", "position_id", "email", "password", "quantity",
		"average_buy_price", "cost_basis", "current_value", "gain_loss",
	} {
		assert.NotContains(t, string(body), `"`+forbidden+`":`)
	}
	assert.NotContains(t, string(body), "private@example.com")
	assert.NotContains(t, string(body), "secret-portfolio")
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
