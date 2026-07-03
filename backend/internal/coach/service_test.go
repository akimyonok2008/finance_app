package coach

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/profile"
)

func TestAnalyze_SupportedModesPass(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Bravo"), user("u3", "Charlie")}
	sums := map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", []testPosition{
			{"AAPL", "stock", "USD", 600, 0.08},
			{"MSFT", "stock", "USD", 400, 0.05},
		}),
		"u2": summaryWith("u2", []testPosition{{"NVDA", "stock", "USD", 1000, 0.20}}),
		"u3": summaryWith("u3", []testPosition{{"SPY", "etf", "USD", 1000, 0.12}}),
	}
	svc := newCoachService(users, sums)

	for mode := range supportedModes {
		t.Run(mode, func(t *testing.T) {
			resp, err := svc.Analyze(context.Background(), "u1", mode)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, mode, resp.Mode)
			assert.Equal(t, Disclaimer, resp.Disclaimer)
			assert.NotEmpty(t, resp.Title)
			assert.NotEmpty(t, resp.Summary)
		})
	}
}

func TestAnalyze_UnsupportedModeRejected(t *testing.T) {
	svc := newCoachService([]auth.User{user("u1", "Alpha")}, map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", []testPosition{{"AAPL", "stock", "USD", 100, 0.1}}),
	})
	_, err := svc.Analyze(context.Background(), "u1", "do_my_taxes")
	assert.ErrorIs(t, err, ErrUnsupportedMode)
}

func TestAnalyze_EmptyPortfolioReturnsErrorWithoutProvider(t *testing.T) {
	users := []auth.User{user("u1", "Alpha")}
	sums := map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", nil), // no positions
	}
	// A provider that fails the test if it is ever called.
	svc := NewService(fakeUsers{users}, fakeSummaries{sums}, panicProvider{t})
	_, err := svc.Analyze(context.Background(), "u1", ModeAnalyzePortfolio)
	assert.ErrorIs(t, err, ErrEmptyPortfolio)
}

type panicProvider struct{ t *testing.T }

func (p panicProvider) GeneratePortfolioCoachAnalysis(_ context.Context, _ CoachProviderInput) (CoachProviderOutput, error) {
	p.t.Fatal("provider must not be called for an empty portfolio")
	return CoachProviderOutput{}, nil
}

func TestAnalyze_MockOutputIsDeterministicAndAdviceFree(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Bravo")}
	sums := map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", []testPosition{
			{"AAPL", "stock", "USD", 700, 0.08},
			{"BTC-USD", "crypto", "USD", 300, -0.04},
		}),
		"u2": summaryWith("u2", []testPosition{{"NVDA", "stock", "USD", 1000, 0.25}}),
	}
	svc := newCoachService(users, sums)

	r1, err := svc.Analyze(context.Background(), "u1", ModeAnalyzePortfolio)
	require.NoError(t, err)
	r2, err := svc.Analyze(context.Background(), "u1", ModeAnalyzePortfolio)
	require.NoError(t, err)

	assert.Equal(t, r1.Summary, r2.Summary, "mock output must be deterministic")
	assert.Equal(t, r1.Title, r2.Title)

	// No advice/guarantee language anywhere in the response text.
	blob := strings.ToLower(r1.Title + " " + r1.Summary + " " +
		strings.Join(r1.TechnicalNotes, " ") + " " +
		strings.Join(r1.FundamentalNotes, " ") + " " +
		strings.Join(r1.LearningPoints, " ") + " " +
		strings.Join(r1.QuestionsToConsider, " "))
	for _, o := range r1.Observations {
		blob += " " + strings.ToLower(o.Text)
	}
	assert.False(t, ContainsForbiddenAdvice(blob), "mock output must not contain advice language: %q", blob)
	assert.NotContains(t, blob, "you should buy")
	assert.NotContains(t, blob, "you should sell")
}

func TestAnalyze_CompareTop10_AvailableFalseWhenAlone(t *testing.T) {
	// Only the requesting user has a portfolio -> no benchmark.
	users := []auth.User{user("u1", "Alpha")}
	sums := map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", []testPosition{{"AAPL", "stock", "USD", 1000, 0.1}}),
	}
	svc := newCoachService(users, sums)

	resp, err := svc.Analyze(context.Background(), "u1", ModeCompareTop10)
	require.NoError(t, err)
	assert.False(t, resp.Top10Comparison.Available)
	assert.NotEmpty(t, resp.Top10Comparison.Notes)
}

func TestAnalyze_CompareTop10_UsefulComparisonWhenDataExists(t *testing.T) {
	users := []auth.User{user("u1", "Alpha"), user("u2", "Bravo"), user("u3", "Charlie")}
	sums := map[string]*portfolio.PortfolioSummary{
		// User u1 is concentrated and behind.
		"u1": summaryWith("u1", []testPosition{
			{"AAPL", "stock", "USD", 900, 0.02},
			{"MSFT", "stock", "USD", 100, 0.01},
		}),
		"u2": summaryWith("u2", []testPosition{
			{"AAPL", "stock", "USD", 500, 0.20},
			{"NVDA", "stock", "USD", 500, 0.20},
		}),
		"u3": summaryWith("u3", []testPosition{
			{"SPY", "etf", "USD", 600, 0.15},
			{"MSFT", "stock", "USD", 400, 0.15},
		}),
	}
	svc := newCoachService(users, sums)

	resp, err := svc.Analyze(context.Background(), "u1", ModeCompareTop10)
	require.NoError(t, err)

	c := resp.Top10Comparison
	assert.True(t, c.Available)
	assert.Equal(t, 2, c.SampleSize, "two other portfolios benchmark u1")
	assert.True(t, c.Limited, "fewer than 10 -> limited benchmark")
	// u1 ~+1.9% vs others' median ~17.5% -> a clearly negative gap.
	assert.Less(t, c.ReturnGapPercentagePoints, 0.0)
	// u1's largest weight (~90%) exceeds the top-10 median.
	assert.Greater(t, c.UserLargestWeightPercentage, c.Top10MedianLargestWeightPercentage)
	// u1 shares AAPL and MSFT with the others.
	assert.Equal(t, 2, c.SharedSymbolsCount)
}

type fakeStrategyProfiles map[string]profile.PublicStrategy

func (f fakeStrategyProfiles) PublicStrategyByHandle(_ context.Context, handle string) (profile.PublicStrategy, error) {
	if p, ok := f[handle]; ok {
		return p, nil
	}
	return profile.PublicStrategy{}, profile.ErrNotFound
}

func TestCompareProfileStructuredAndPrivacySafe(t *testing.T) {
	svc := newCoachService([]auth.User{user("u1", "Alpha")}, map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", []testPosition{
			{"NVDA", "stock", "USD", 120, 0},
			{"SPY", "etf", "USD", 300, 0},
			{"AAPL", "stock", "USD", 80, 0},
		}),
	})
	svc.SetProfileProvider(fakeStrategyProfiles{
		"alpha_wolf": {
			Handle: "alpha_wolf", DisplayName: "AlphaWolf", AvatarKey: "fox", StrategyTag: "growth",
			Weights: []profile.StrategyWeight{
				{Symbol: "NVDA", AssetType: "stock", WeightPercentage: 50},
				{Symbol: "AAPL", AssetType: "stock", WeightPercentage: 25},
				{Symbol: "MSFT", AssetType: "stock", WeightPercentage: 25},
			},
		},
	})

	resp, err := svc.CompareProfile(context.Background(), "u1", "alpha_wolf")
	require.NoError(t, err)
	assert.Equal(t, "alpha_wolf", resp.TargetProfile.Handle)
	assert.InDelta(t, 40.0, resp.OverlapScore, 0.001) // min(NVDA 24,50)+min(AAPL 16,25)
	assert.Equal(t, []string{"AAPL", "NVDA"}, resp.SharedSymbols)
	assert.NotEmpty(t, resp.WeightDifferences)
	assert.NotEmpty(t, resp.LearningPoints)
	assert.Equal(t, "This is educational comparison, not investment advice.", resp.Disclaimer)

	body, err := json.Marshal(resp)
	require.NoError(t, err)
	for _, forbidden := range []string{
		"quantity", "average_buy_price", "cost_basis", "current_value", "portfolio_value",
		"user_id", "portfolio_id", "position_id", "email", "password", "baseline_value",
	} {
		assert.NotContains(t, string(body), `"`+forbidden+`":`)
	}
}

func TestCompareProfileEmptyPortfolioAndMissingTarget(t *testing.T) {
	svc := newCoachService([]auth.User{user("u1", "Alpha")}, map[string]*portfolio.PortfolioSummary{
		"u1": summaryWith("u1", nil),
	})
	svc.SetProfileProvider(fakeStrategyProfiles{})

	_, err := svc.CompareProfile(context.Background(), "u1", "missing")
	assert.ErrorIs(t, err, profile.ErrNotFound)

	svc.SetProfileProvider(fakeStrategyProfiles{
		"alpha_wolf": {Handle: "alpha_wolf", Weights: []profile.StrategyWeight{{Symbol: "NVDA", AssetType: "stock", WeightPercentage: 100}}},
	})
	_, err = svc.CompareProfile(context.Background(), "u1", "alpha_wolf")
	assert.ErrorIs(t, err, ErrEmptyPortfolio)
}
