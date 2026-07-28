package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricSeparation_RankedPositiveWhileOpenHoldingsNegative(t *testing.T) {
	open := CalculateUnrealizedMetrics(amt(341266.27), amt(347437.07))
	require.NotNil(t, open.UnrealizedReturnPercentage)

	assert.InDelta(t, -6170.80, open.UnrealizedPnLBase.Float64(), 0.001)
	assert.InDelta(t, -1.7755, *open.UnrealizedReturnPercentage, 0.01)

	ranked := RankedPerformanceView{
		Index: 100.90, ReturnPercentage: 0.90, TrackingStatus: "active",
	}
	assert.Equal(t, 0.90, ranked.ReturnPercentage)
	assert.NotEqual(t, round2(347437.07*ranked.ReturnPercentage/100), open.UnrealizedPnLBase.Float64())
}

func TestReconciliation_MixedHistoryEconomicPnLIsIndependentOfRankedReturn(t *testing.T) {
	totalPnL := amt(3126.13)
	economic := EconomicPerformance{
		TotalPnLBase: &totalPnL, CalculationStatus: "complete", IsComplete: true,
	}
	status := ReconcilePortfolioFinancials(
		RankedPerformanceView{Index: 100.90, ReturnPercentage: 0.90, TrackingStatus: "active"},
		PortfolioValuation{
			OpenHoldingsMarketValueBase: amt(341266.27),
			CurrentPortfolioValueBase:   amt(341266.27),
		},
		OpenHoldingsMetrics{
			CostBasisBase: amt(347437.07), UnrealizedPnLBase: amt(-6170.80),
		},
		RealizedMetrics{RealizedPnLBase: amt(8000)},
		IncomeMetrics{TotalIncomeBase: amt(1500)},
		FeeMetrics{TotalFeesBase: amt(203.07)},
		economic,
	)

	assert.True(t, status.IsComplete)
	assert.True(t, status.IsConsistent)
	assert.InDelta(t, 0, status.Difference.Float64(), 0.001)
}

func TestCalculateUnrealizedMetrics_ZeroBasisIsNotApplicable(t *testing.T) {
	open := CalculateUnrealizedMetrics(amt(0), amt(0))
	assert.Nil(t, open.UnrealizedReturnPercentage)
	assert.Equal(t, 0.0, open.UnrealizedPnLBase.Float64())
}
