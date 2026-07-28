package portfolio

import "math"

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
func CalculatePositionSummary(pos *Position, currentPrice float64, currentPriceCurrency string, costBasisBase, currentValueBase float64, baseCurrency string) PositionSummary {
	// pos.Quantity/AverageBuyPrice are exact decimal; costBasis is computed
	// with exact MulPrice and only converted to float64 (documented boundary)
	// for this display-only local-currency figure. currentPrice stays float64
	// (a live quote, not yet part of this section's scope).
	costBasis := pos.Quantity.MulPrice(pos.AverageBuyPrice).Float64()
	currentValue := pos.Quantity.Float64() * currentPrice
	gainLoss := currentValue - costBasis
	gainLossBase := currentValueBase - costBasisBase

	gainLossPct := 0.0
	if costBasisBase != 0 {
		gainLossPct = gainLossBase / costBasisBase * 100
	}

	return PositionSummary{
		PositionID:           pos.ID,
		Symbol:               pos.Symbol,
		AssetType:            pos.AssetType,
		Quantity:             pos.Quantity.Float64(),
		AverageBuyPrice:      pos.AverageBuyPrice.Float64(),
		CurrentPrice:         currentPrice,
		CurrentPriceCurrency: currentPriceCurrency,
		CostBasis:            round2(costBasis),
		CurrentValue:         round2(currentValue),
		GainLoss:             round2(gainLoss),
		GainLossPercentage:   round2(gainLossPct),
		Currency:             pos.Currency,
		CostBasisBase:        round2(costBasisBase),
		CurrentValueBase:     round2(currentValueBase),
		GainLossBase:         round2(gainLossBase),
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
	var activeCostBasis, activeCurrentValue, closedCostBasis, realizedGainLoss float64
	for _, p := range positions {
		activeCostBasis += p.CostBasisBase
		activeCurrentValue += p.CurrentValueBase
	}
	for _, p := range closed {
		closedCostBasis += p.ClosedCostBasisBase
		realizedGainLoss += p.RealizedGainLossBase
	}
	unrealizedGainLoss := activeCurrentValue - activeCostBasis
	// Closed proceeds are assets only when represented by cash. Do not synthesize
	// them here or realized gain would be counted twice once sale proceeds enter
	// portfolio_cash_balances.
	currentValue := activeCurrentValue

	gainLossPct := 0.0
	if activeCostBasis != 0 {
		gainLossPct = unrealizedGainLoss / activeCostBasis * 100
	}

	return PortfolioSummary{
		UserID:                 userID,
		PortfolioID:            portfolioID,
		BaseCurrency:           baseCurrency,
		TotalCostBasis:         round2(activeCostBasis),
		CurrentValue:           round2(currentValue),
		GainLoss:               round2(unrealizedGainLoss),
		GainLossPercentage:     round2(gainLossPct),
		PortfolioIndex:         100,
		Positions:              positions,
		ClosedPositions:        closed,
		ActiveCostBasisBase:    round2(activeCostBasis),
		ActiveCurrentValueBase: round2(activeCurrentValue),
		UnrealizedGainLossBase: round2(unrealizedGainLoss),
		ClosedCostBasisBase:    round2(closedCostBasis),
		RealizedGainLossBase:   round2(realizedGainLoss),
	}
}
