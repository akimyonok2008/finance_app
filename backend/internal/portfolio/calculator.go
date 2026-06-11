package portfolio

import "math"

// round2 rounds to two decimal places, matching the precision used for the
// percentage and index figures shown on the dashboard.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
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
	costBasis := pos.Quantity * pos.AverageBuyPrice
	currentValue := pos.Quantity * currentPrice
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
		Quantity:             pos.Quantity,
		AverageBuyPrice:      pos.AverageBuyPrice,
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
//	total_cost_basis     = sum(cost_basis_base)
//	current_value        = sum(current_value_base)
//	gain_loss            = current_value - total_cost_basis
//	gain_loss_percentage = gain_loss / total_cost_basis * 100   (0 if zero basis)
//	portfolio_index      = 100 * current_value / total_cost_basis (100 if zero basis)
func CalculatePortfolioSummary(userID, portfolioID, baseCurrency string, positions []PositionSummary) PortfolioSummary {
	var totalCostBasis, currentValue float64
	for _, p := range positions {
		totalCostBasis += p.CostBasisBase
		currentValue += p.CurrentValueBase
	}
	gainLoss := currentValue - totalCostBasis

	gainLossPct := 0.0
	portfolioIndex := 100.0
	if totalCostBasis != 0 {
		gainLossPct = gainLoss / totalCostBasis * 100
		portfolioIndex = 100 * currentValue / totalCostBasis
	}

	return PortfolioSummary{
		UserID:             userID,
		PortfolioID:        portfolioID,
		BaseCurrency:       baseCurrency,
		TotalCostBasis:     round2(totalCostBasis),
		CurrentValue:       round2(currentValue),
		GainLoss:           round2(gainLoss),
		GainLossPercentage: round2(gainLossPct),
		PortfolioIndex:     round2(portfolioIndex),
		Positions:          positions,
	}
}
