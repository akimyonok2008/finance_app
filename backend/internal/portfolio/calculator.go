package portfolio

import (
	"math"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// round2 rounds to two decimal places, matching the precision used for the
// percentage and index figures shown on the dashboard.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// round4 rounds to four decimal places. Fractional reinvestment quantities need
// finer precision than cash amounts.
func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// CalculatePositionSummary computes the derived figures for a single position.
// Local cost basis / current value are derived from quantity, average buy
// price, and the current price; the base-currency equivalents are supplied by
// the caller (which performs FX conversion).
//
//	cost_basis (local)   = quantity * average_buy_price
//	current_value (local)= quantity * current_price
//	gain_loss (local)    = current_value - cost_basis
//	gain_loss_percentage = gain_loss_base / cost_basis_base * 100
//
// Percentage performance is calculated from base-currency values. This keeps
// each position consistent with the mixed-currency portfolio total.
func CalculatePositionSummary(pos *Position, currentPrice float64, currentPriceCurrency string, costBasisBase, currentValueBase money.Amount, baseCurrency string) PositionSummary {
	// pos.Quantity/AverageBuyPrice are exact decimal; costBasis is computed
	// with exact MulPrice. currentPrice stays float64 (a live quote, not yet
	// part of this section's scope) and is wrapped via money.PriceFromFloat64
	// immediately so the local-currency figures stay in exact decimal space.
	costBasis := pos.Quantity.MulPrice(pos.AverageBuyPrice)
	currentValue := pos.Quantity.MulPrice(money.PriceFromFloat64(currentPrice))
	gainLoss := currentValue.Sub(costBasis)
	gainLossBase := currentValueBase.Sub(costBasisBase)

	gainLossPct := 0.0
	if costBasisBase.Sign() != 0 {
		gainLossPct = gainLossBase.Float64() / costBasisBase.Float64() * 100
	}

	return PositionSummary{
		PositionID:           pos.ID,
		Symbol:               pos.Symbol,
		AssetType:            pos.AssetType,
		Quantity:             pos.Quantity,
		AverageBuyPrice:      pos.AverageBuyPrice,
		CurrentPrice:         money.PriceFromFloat64(currentPrice),
		CurrentPriceCurrency: currentPriceCurrency,
		CostBasis:            money.QuantizeValue(costBasis),
		CurrentValue:         money.QuantizeValue(currentValue),
		GainLoss:             money.QuantizeValue(gainLoss),
		GainLossPercentage:   round2(gainLossPct),
		Currency:             pos.Currency,
		CostBasisBase:        money.QuantizeValue(costBasisBase),
		CurrentValueBase:     money.QuantizeValue(currentValueBase),
		GainLossBase:         money.QuantizeValue(gainLossBase),
		BaseCurrency:         baseCurrency,
	}
}

// CalculatePortfolioSummary aggregates per-position summaries into a portfolio
// total, using the base-currency values so mixed-currency portfolios are
// comparable.
//
// Compatibility aliases are deliberately scoped to OPEN holdings:
//
//	total_cost_basis     = open holdings cost basis
//	current_value        = open holdings market value (cash is added by Service)
//	gain_loss            = open holdings unrealized P&L
//	gain_loss_percentage = open holdings unrealized return
//
// portfolio_index is left at its neutral default here. Only the persistent
// ranked-performance service is allowed to populate it.
func CalculatePortfolioSummary(userID, portfolioID, baseCurrency string, positions []PositionSummary, closedInput ...[]ClosedPositionSummary) PortfolioSummary {
	var closed []ClosedPositionSummary
	if len(closedInput) > 0 {
		closed = closedInput[0]
	}
	activeCostBasis := money.ZeroAmount()
	activeCurrentValue := money.ZeroAmount()
	closedCostBasis := money.ZeroAmount()
	realizedGainLoss := money.ZeroAmount()
	for _, p := range positions {
		activeCostBasis = activeCostBasis.Add(p.CostBasisBase)
		activeCurrentValue = activeCurrentValue.Add(p.CurrentValueBase)
	}
	for _, p := range closed {
		closedCostBasis = closedCostBasis.Add(p.ClosedCostBasisBase)
		realizedGainLoss = realizedGainLoss.Add(p.RealizedGainLossBase)
	}
	unrealizedGainLoss := activeCurrentValue.Sub(activeCostBasis)
	// Closed proceeds are assets only when represented by cash. Do not synthesize
	// them here or realized gain would be counted twice once sale proceeds enter
	// portfolio_cash_balances.
	currentValue := activeCurrentValue

	gainLossPct := 0.0
	if activeCostBasis.Sign() != 0 {
		gainLossPct = unrealizedGainLoss.Float64() / activeCostBasis.Float64() * 100
	}

	return PortfolioSummary{
		UserID:                 userID,
		PortfolioID:            portfolioID,
		BaseCurrency:           baseCurrency,
		TotalCostBasis:         money.QuantizeValue(activeCostBasis),
		CurrentValue:           money.QuantizeValue(currentValue),
		GainLoss:               money.QuantizeValue(unrealizedGainLoss),
		GainLossPercentage:     round2(gainLossPct),
		PortfolioIndex:         100,
		Positions:              positions,
		ClosedPositions:        closed,
		ActiveCostBasisBase:    money.QuantizeValue(activeCostBasis),
		ActiveCurrentValueBase: money.QuantizeValue(activeCurrentValue),
		UnrealizedGainLossBase: money.QuantizeValue(unrealizedGainLoss),
		ClosedCostBasisBase:    money.QuantizeValue(closedCostBasis),
		RealizedGainLossBase:   money.QuantizeValue(realizedGainLoss),
	}
}
