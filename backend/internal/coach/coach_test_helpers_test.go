package coach

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// --- test doubles ------------------------------------------------------------

type fakeUsers struct{ users []auth.User }

func (f fakeUsers) ListUsers(_ context.Context) ([]auth.User, error) {
	return f.users, nil
}

type fakeSummaries struct {
	m map[string]*portfolio.PortfolioSummary
}

func (f fakeSummaries) Summary(_ context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	return f.m[userID], nil
}

// --- builders ----------------------------------------------------------------

func newCoachService(users []auth.User, summaries map[string]*portfolio.PortfolioSummary) *Service {
	svc := NewService(fakeUsers{users}, fakeSummaries{summaries}, NewMockProvider())
	svc.SetClock(&clock.FixedClock{Time: time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)})
	return svc
}

// summaryWith builds a portfolio summary from (symbol, assetType, currency,
// weightBase, returnPct) tuples. Base values are derived so weights and the
// portfolio return come out as specified, without exposing raw figures in tests.
func summaryWith(userID string, positions []testPosition) *portfolio.PortfolioSummary {
	var totalValue, totalCost float64
	ps := make([]portfolio.PositionSummary, 0, len(positions))
	for _, p := range positions {
		costBase := p.valueBase / (1 + p.returnFraction)
		ps = append(ps, portfolio.PositionSummary{
			Symbol:             p.symbol,
			AssetType:          p.assetType,
			Currency:           p.currency,
			CurrentValueBase:   p.valueBase,
			CostBasisBase:      costBase,
			GainLossBase:       p.valueBase - costBase,
			GainLossPercentage: p.returnFraction * 100,
			BaseCurrency:       "USD",
		})
		totalValue += p.valueBase
		totalCost += costBase
	}
	gainPct := 0.0
	if totalCost > 0 {
		gainPct = (totalValue - totalCost) / totalCost * 100
	}
	return &portfolio.PortfolioSummary{
		UserID:             userID,
		PortfolioID:        "pf-" + userID,
		BaseCurrency:       "USD",
		TotalCostBasis:     totalCost,
		CurrentValue:       totalValue,
		GainLoss:           totalValue - totalCost,
		GainLossPercentage: gainPct,
		PortfolioIndex:     100 + gainPct,
		Positions:          ps,
	}
}

type testPosition struct {
	symbol         string
	assetType      string
	currency       string
	valueBase      float64
	returnFraction float64 // 0.10 == +10%
}

func user(id, name string) auth.User {
	return auth.User{ID: id, DisplayName: name, AvatarKey: "default", Email: id + "@example.com"}
}
