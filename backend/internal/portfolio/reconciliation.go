package portfolio

import "math"

const reconciliationTolerance = 0.01

// CalculateUnrealizedMetrics owns the open-holdings accounting formula. A nil
// return percentage distinguishes zero basis (not applicable) from a real 0%.
func CalculateUnrealizedMetrics(marketValueBase, costBasisBase float64) OpenHoldingsMetrics {
	pnl := marketValueBase - costBasisBase
	var returnPercentage *float64
	if costBasisBase > 0 {
		value := pnl / costBasisBase * 100
		returnPercentage = &value
	}
	return OpenHoldingsMetrics{
		CostBasisBase:              round2(costBasisBase),
		UnrealizedPnLBase:          round2(pnl),
		UnrealizedReturnPercentage: roundedPointer(returnPercentage),
	}
}

// ReconcilePortfolioFinancials verifies accounting identities independently of
// ranked performance. Ranked index and ranked return are checked only against
// each other and never participate in a dollar calculation.
func ReconcilePortfolioFinancials(
	ranked RankedPerformanceView,
	valuation PortfolioValuation,
	open OpenHoldingsMetrics,
	realized RealizedMetrics,
	income IncomeMetrics,
	fees FeeMetrics,
	economic EconomicPerformance,
) ReconciliationStatus {
	reasons := make([]string, 0, 3)
	consistent := true

	valueDifference := valuation.CurrentPortfolioValueBase -
		(valuation.OpenHoldingsMarketValueBase + valuation.CashValueBase)
	if math.Abs(valueDifference) > reconciliationTolerance {
		consistent = false
		reasons = append(reasons, "current_value_mismatch")
	}

	unrealizedDifference := open.UnrealizedPnLBase -
		(valuation.OpenHoldingsMarketValueBase - open.CostBasisBase)
	if math.Abs(unrealizedDifference) > reconciliationTolerance {
		consistent = false
		reasons = append(reasons, "unrealized_pnl_mismatch")
	}

	rankedDifference := ranked.ReturnPercentage - (ranked.Index - 100)
	if math.Abs(rankedDifference) > 0.0001 {
		consistent = false
		reasons = append(reasons, "ranked_return_mismatch")
	}

	difference := 0.0
	if economic.IsComplete && economic.TotalPnLBase != nil {
		// Standalone fees are the only fees that still need to be deducted here:
		// sale fees embedded in RealizedMetrics.RealizedPnLBase (canonical sale
		// contract, see backend/README.md) are already netted out of that figure,
		// so subtracting fees.TotalFeesBase in full would double-count them.
		standaloneFees := fees.TotalFeesBase - fees.EmbeddedInRealizedPnLBase
		attribution := open.UnrealizedPnLBase + realized.RealizedPnLBase +
			income.TotalIncomeBase - standaloneFees
		difference = *economic.TotalPnLBase - attribution
		if math.Abs(difference) > reconciliationTolerance {
			consistent = false
			reasons = append(reasons, "economic_attribution_mismatch")
		}
	}

	return ReconciliationStatus{
		IsComplete: economic.IsComplete, IsConsistent: consistent,
		Difference: round2(difference), Reasons: reasons,
	}
}

func roundedPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	rounded := round2(*value)
	return &rounded
}
