package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculatePositionSummary(t *testing.T) {
	pos := &Position{
		ID: "p1", Symbol: "AAPL", AssetType: "stock",
		Quantity: 10, AverageBuyPrice: 180, Currency: "USD",
	}

	// USD position: base values equal local values.
	ps := CalculatePositionSummary(pos, 195, "USD", 1800, 1950, "USD")

	assert.Equal(t, "p1", ps.PositionID)
	assert.Equal(t, 1800.0, ps.CostBasis)
	assert.Equal(t, 1950.0, ps.CurrentValue)
	assert.Equal(t, 1800.0, ps.CostBasisBase)
	assert.Equal(t, 1950.0, ps.CurrentValueBase)
	assert.Equal(t, 150.0, ps.GainLossBase)
	assert.Equal(t, "USD", ps.CurrentPriceCurrency)
	assert.Equal(t, "USD", ps.BaseCurrency)
	assert.Equal(t, 150.0, ps.GainLoss)
	assert.InDelta(t, 8.33, ps.GainLossPercentage, 0.01)
}

func TestCalculatePositionSummary_PercentageUsesBaseCurrencyValues(t *testing.T) {
	pos := &Position{
		ID: "p1", Symbol: "AAPL", AssetType: "stock",
		Quantity: 10, AverageBuyPrice: 180, Currency: "TRY",
	}

	// Purchase basis is TRY while the quote is USD. Local subtraction would be
	// meaningless; base values produce the financially valid performance.
	ps := CalculatePositionSummary(pos, 195, "USD", 55.8, 1950, "USD")

	assert.Equal(t, "TRY", ps.Currency)
	assert.Equal(t, "USD", ps.CurrentPriceCurrency)
	assert.Equal(t, 1894.2, ps.GainLossBase)
	assert.InDelta(t, 3394.62, ps.GainLossPercentage, 0.01)
}

func TestCalculatePortfolioSummary_AggregatesBaseValues(t *testing.T) {
	positions := []PositionSummary{
		{CostBasisBase: 1800, CurrentValueBase: 1950},
		{CostBasisBase: 775, CurrentValueBase: 914.5},
	}

	sum := CalculatePortfolioSummary("user-1", "pf-1", "USD", positions)

	assert.Equal(t, "USD", sum.BaseCurrency)
	assert.InDelta(t, 2575.0, sum.TotalCostBasis, 0.01)
	assert.InDelta(t, 2864.5, sum.CurrentValue, 0.01)
	assert.InDelta(t, 289.5, sum.GainLoss, 0.01)
	assert.InDelta(t, 11.24, sum.GainLossPercentage, 0.05)
	assert.InDelta(t, 111.24, sum.PortfolioIndex, 0.05)
}

func TestCalculatePortfolioSummary_ZeroCostBasis(t *testing.T) {
	sum := CalculatePortfolioSummary("user-1", "pf-1", "USD", nil)

	assert.Equal(t, 0.0, sum.TotalCostBasis)
	assert.Equal(t, 0.0, sum.GainLossPercentage)
	assert.Equal(t, 100.0, sum.PortfolioIndex)
}
