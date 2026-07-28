package portfolio

import (
	"testing"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/stretchr/testify/assert"
)

func TestCalculatePositionSummary(t *testing.T) {
	pos := &Position{
		ID: "p1", Symbol: "AAPL", AssetType: "stock",
		Quantity: testQuantity("10"), AverageBuyPrice: testPrice("180"), Currency: "USD",
	}

	// USD position: base values equal local values.
	ps := CalculatePositionSummary(pos, 195, "USD", amt(1800), amt(1950), "USD")

	assert.Equal(t, "p1", ps.PositionID)
	assert.Equal(t, 1800.0, ps.CostBasis.Float64())
	assert.Equal(t, 1950.0, ps.CurrentValue.Float64())
	assert.Equal(t, 1800.0, ps.CostBasisBase.Float64())
	assert.Equal(t, 1950.0, ps.CurrentValueBase.Float64())
	assert.Equal(t, 150.0, ps.GainLossBase.Float64())
	assert.Equal(t, "USD", ps.CurrentPriceCurrency)
	assert.Equal(t, "USD", ps.BaseCurrency)
	assert.Equal(t, 150.0, ps.GainLoss.Float64())
	assert.InDelta(t, 8.33, ps.GainLossPercentage, 0.01)
}

func TestCalculatePositionSummary_PercentageUsesBaseCurrencyValues(t *testing.T) {
	pos := &Position{
		ID: "p1", Symbol: "AAPL", AssetType: "stock",
		Quantity: testQuantity("10"), AverageBuyPrice: testPrice("180"), Currency: "TRY",
	}

	// Purchase basis is TRY while the quote is USD. Local subtraction would be
	// meaningless; base values produce the financially valid performance.
	ps := CalculatePositionSummary(pos, 195, "USD", amt(55.8), amt(1950), "USD")

	assert.Equal(t, "TRY", ps.Currency)
	assert.Equal(t, "USD", ps.CurrentPriceCurrency)
	assert.Equal(t, 1894.2, ps.GainLossBase.Float64())
	assert.InDelta(t, 3394.62, ps.GainLossPercentage, 0.01)
}

func TestCalculatePortfolioSummary_AggregatesBaseValues(t *testing.T) {
	positions := []PositionSummary{
		{CostBasisBase: money.AmountFromFloat64(1800), CurrentValueBase: money.AmountFromFloat64(1950)},
		{CostBasisBase: money.AmountFromFloat64(775), CurrentValueBase: money.AmountFromFloat64(914.5)},
	}

	sum := CalculatePortfolioSummary("user-1", "pf-1", "USD", positions)

	assert.Equal(t, "USD", sum.BaseCurrency)
	assert.InDelta(t, 2575.0, sum.TotalCostBasis.Float64(), 0.01)
	assert.InDelta(t, 2864.5, sum.CurrentValue.Float64(), 0.01)
	assert.InDelta(t, 289.5, sum.GainLoss.Float64(), 0.01)
	assert.InDelta(t, 11.24, sum.GainLossPercentage, 0.05)
	assert.Equal(t, 100.0, sum.PortfolioIndex, "accounting calculator must not fabricate ranked performance")
}

func TestCalculatePortfolioSummary_ZeroCostBasis(t *testing.T) {
	sum := CalculatePortfolioSummary("user-1", "pf-1", "USD", nil)

	assert.Equal(t, 0.0, sum.TotalCostBasis.Float64())
	assert.Equal(t, 0.0, sum.GainLossPercentage)
	assert.Equal(t, 100.0, sum.PortfolioIndex)
}
