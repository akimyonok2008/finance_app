package portfolio

import (
	"math"

	"github.com/ardakimyonok/finance_app/internal/money"
)

const reconciliationTolerance = 0.01

// CalculateUnrealizedMetrics owns the open-holdings accounting formula. A nil
// return percentage distinguishes zero basis (not applicable) from a real 0%.
func CalculateUnrealizedMetrics(marketValueBase, costBasisBase money.Amount) OpenHoldingsMetrics {
	pnl := marketValueBase.Sub(costBasisBase)
	var returnPercentage *float64
	if costBasisBase.Sign() > 0 {
		value := pnl.Float64() / costBasisBase.Float64() * 100
		returnPercentage = &value
	}
	return OpenHoldingsMetrics{
		CostBasisBase:              money.QuantizeValue(costBasisBase),
		UnrealizedPnLBase:          money.QuantizeValue(pnl),
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

	valueDifference := valuation.CurrentPortfolioValueBase.Sub(
		valuation.OpenHoldingsMarketValueBase.Add(valuation.CashValueBase))
	if math.Abs(valueDifference.Float64()) > reconciliationTolerance {
		consistent = false
		reasons = append(reasons, "current_value_mismatch")
	}

	unrealizedDifference := open.UnrealizedPnLBase.Sub(
		valuation.OpenHoldingsMarketValueBase.Sub(open.CostBasisBase))
	if math.Abs(unrealizedDifference.Float64()) > reconciliationTolerance {
		consistent = false
		reasons = append(reasons, "unrealized_pnl_mismatch")
	}

	rankedDifference := ranked.ReturnPercentage - (ranked.Index - 100)
	if math.Abs(rankedDifference) > 0.0001 {
		consistent = false
		reasons = append(reasons, "ranked_return_mismatch")
	}

	difference := money.ZeroAmount()
	if economic.IsComplete && economic.TotalPnLBase != nil {
		// Standalone fees are the only fees that still need to be deducted here:
		// sale fees embedded in RealizedMetrics.RealizedPnLBase (canonical sale
		// contract, see backend/README.md) are already netted out of that figure,
		// so subtracting fees.TotalFeesBase in full would double-count them.
		standaloneFees := StandaloneFeesBase(fees)
		attribution := open.UnrealizedPnLBase.Add(realized.RealizedPnLBase).
			Add(income.TotalIncomeBase).Sub(standaloneFees)
		difference = economic.TotalPnLBase.Sub(attribution)
		if math.Abs(difference.Float64()) > reconciliationTolerance {
			consistent = false
			reasons = append(reasons, "economic_attribution_mismatch")
		}
	}

	return ReconciliationStatus{
		IsComplete: economic.IsComplete, IsConsistent: consistent,
		Difference: money.QuantizeValue(difference), Reasons: reasons,
	}
}

func roundedPointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	rounded := round2(*value)
	return &rounded
}

// roundedAmountPointer is the money.Amount equivalent of roundedPointer, used
// at the same posting boundary for EconomicPerformance's *money.Amount
// fields.
func roundedAmountPointer(value *money.Amount) *money.Amount {
	if value == nil {
		return nil
	}
	rounded := money.QuantizeValue(*value)
	return &rounded
}
