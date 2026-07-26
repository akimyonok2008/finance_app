package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func floatPtr(v float64) *float64 { return &v }

// Standalone fees exclude the sale/purchase fees already netted into realized
// P&L and cost basis. Subtracting TotalFeesBase in full double-counts them.
func TestStandaloneFeesBaseExcludesEmbeddedTradeFees(t *testing.T) {
	fees := FeeMetrics{TotalFeesBase: 50, EmbeddedInRealizedPnLBase: 12}
	assert.InDelta(t, 38.0, StandaloneFeesBase(fees), 1e-9)
}

// The economic breakdown must be the SAME decomposition reconciliation checks:
// realized + unrealized + income - standalone fees. If they ever disagree the
// Performance tab would contradict the reconciliation status beside it.
func TestEconomicAttributionMatchesReconciliationDecomposition(t *testing.T) {
	open := OpenHoldingsMetrics{UnrealizedPnLBase: 400, CostBasisBase: 2000}
	realized := RealizedMetrics{RealizedPnLBase: 150}
	income := IncomeMetrics{TotalIncomeBase: 60}
	fees := FeeMetrics{TotalFeesBase: 30, EmbeddedInRealizedPnLBase: 10}
	// 400 + 150 + 60 - 20 = 590
	economic := EconomicPerformance{
		TotalPnLBase: floatPtr(590), CalculationStatus: "complete", IsComplete: true,
	}

	got := CalculateEconomicAttribution(open, realized, income, fees, economic)

	assert.InDelta(t, 150.0, got.RealizedPnLBase, 1e-9)
	assert.InDelta(t, 400.0, got.UnrealizedPnLBase, 1e-9)
	assert.InDelta(t, 60.0, got.NetIncomeBase, 1e-9)
	assert.InDelta(t, 20.0, got.StandaloneFeesBase, 1e-9)
	assert.InDelta(t, 590.0, got.AttributedTotalBase, 1e-9)
	require.NotNil(t, got.TotalEconomicPnLBase)
	assert.InDelta(t, 590.0, *got.TotalEconomicPnLBase, 1e-9)
	require.NotNil(t, got.UnattributedBase)
	assert.InDelta(t, 0.0, *got.UnattributedBase, 1e-9)

	// The reconciliation status computed from the same inputs must agree.
	status := ReconcilePortfolioFinancials(
		RankedPerformanceView{Index: 100, ReturnPercentage: 0},
		PortfolioValuation{OpenHoldingsMarketValueBase: 2400, CashValueBase: 0, CurrentPortfolioValueBase: 2400},
		OpenHoldingsMetrics{UnrealizedPnLBase: 400, CostBasisBase: 2000},
		realized, income, fees, economic,
	)
	assert.True(t, status.IsConsistent, "reasons: %v", status.Reasons)
	assert.InDelta(t, 0.0, status.Difference, 1e-9)
}

// An incomplete ledger must leave the total NIL. Rendering 0 would tell the
// user they broke even when the truth is "we do not know".
func TestEconomicAttributionLeavesIncompleteTotalNil(t *testing.T) {
	got := CalculateEconomicAttribution(
		OpenHoldingsMetrics{UnrealizedPnLBase: 100},
		RealizedMetrics{}, IncomeMetrics{}, FeeMetrics{},
		EconomicPerformance{CalculationStatus: "legacy_estimate", IsComplete: false},
	)

	assert.Nil(t, got.TotalEconomicPnLBase)
	assert.Nil(t, got.UnattributedBase)
	assert.False(t, got.IsComplete)
	assert.Equal(t, "legacy_estimate", got.CalculationStatus)
	// The attributable part is still known and is still shown.
	assert.InDelta(t, 100.0, got.AttributedTotalBase, 1e-9)
}

// A gap between the ledger total and the attributable parts is DISCLOSED.
func TestEconomicAttributionDisclosesUnattributedResidual(t *testing.T) {
	got := CalculateEconomicAttribution(
		OpenHoldingsMetrics{UnrealizedPnLBase: 100},
		RealizedMetrics{RealizedPnLBase: 0}, IncomeMetrics{}, FeeMetrics{},
		EconomicPerformance{TotalPnLBase: floatPtr(130), CalculationStatus: "complete", IsComplete: true},
	)

	require.NotNil(t, got.UnattributedBase)
	assert.InDelta(t, 30.0, *got.UnattributedBase, 1e-9)
}

// Contribution is weight x return, NOT standalone return. A tiny position with
// a spectacular return must not outrank a large position with a solid one.
func TestContributionsRankByWeightedContributionNotStandaloneReturn(t *testing.T) {
	instruments := []InstrumentEconomics{
		// 1% of capital, +100% return -> contributes 1.0 point.
		{Symbol: "MOON", CapitalBase: 100, UnrealizedPnLBase: 100},
		// 90% of capital, +10% return -> contributes 9.0 points.
		{Symbol: "BIG", CapitalBase: 9000, UnrealizedPnLBase: 900},
		{Symbol: "FLAT", CapitalBase: 900},
	}
	got := CalculateContributions(instruments, 0)

	require.True(t, got.Available)
	require.Len(t, got.Contributors, 2)
	assert.Equal(t, "BIG", got.Contributors[0].Symbol)
	assert.InDelta(t, 9.0, got.Contributors[0].ContributionPercentagePoints, 1e-6)
	assert.Equal(t, "MOON", got.Contributors[1].Symbol)
	assert.InDelta(t, 1.0, got.Contributors[1].ContributionPercentagePoints, 1e-6)

	// MOON's standalone return is the larger number — proof the ranking is not
	// using it.
	require.NotNil(t, got.Contributors[1].InstrumentReturnPercentage)
	require.NotNil(t, got.Contributors[0].InstrumentReturnPercentage)
	assert.Greater(t,
		*got.Contributors[1].InstrumentReturnPercentage,
		*got.Contributors[0].InstrumentReturnPercentage)
}

// Realized P&L and instrument income are part of the contribution; a position
// that is flat on price but paid dividends is a real contributor.
func TestContributionsIncludeRealizedAndIncome(t *testing.T) {
	got := CalculateContributions([]InstrumentEconomics{
		{Symbol: "DIVY", CapitalBase: 1000, IncomeBase: 50},
		{Symbol: "SOLD", CapitalBase: 1000, RealizedPnLBase: 100},
		{Symbol: "COSTLY", CapitalBase: 1000, FeesBase: 30},
	}, 0)

	require.True(t, got.Available)
	require.Len(t, got.Contributors, 2)
	assert.Equal(t, "SOLD", got.Contributors[0].Symbol)
	assert.Equal(t, "DIVY", got.Contributors[1].Symbol)
	require.Len(t, got.Detractors, 1)
	assert.Equal(t, "COSTLY", got.Detractors[0].Symbol)
	assert.InDelta(t, -1.0, got.Detractors[0].ContributionPercentagePoints, 1e-6)
}

// At most three of each, worst detractor first.
func TestContributionsCapAtTopThreeAndOrderDetractorsWorstFirst(t *testing.T) {
	instruments := []InstrumentEconomics{
		{Symbol: "A", CapitalBase: 1000, UnrealizedPnLBase: 40},
		{Symbol: "B", CapitalBase: 1000, UnrealizedPnLBase: 30},
		{Symbol: "C", CapitalBase: 1000, UnrealizedPnLBase: 20},
		{Symbol: "D", CapitalBase: 1000, UnrealizedPnLBase: 10},
		{Symbol: "W", CapitalBase: 1000, UnrealizedPnLBase: -40},
		{Symbol: "X", CapitalBase: 1000, UnrealizedPnLBase: -30},
		{Symbol: "Y", CapitalBase: 1000, UnrealizedPnLBase: -20},
		{Symbol: "Z", CapitalBase: 1000, UnrealizedPnLBase: -10},
	}
	got := CalculateContributions(instruments, 0)

	require.Len(t, got.Contributors, TopContributionCount)
	require.Len(t, got.Detractors, TopContributionCount)
	assert.Equal(t, []string{"A", "B", "C"},
		[]string{got.Contributors[0].Symbol, got.Contributors[1].Symbol, got.Contributors[2].Symbol})
	assert.Equal(t, []string{"W", "X", "Y"},
		[]string{got.Detractors[0].Symbol, got.Detractors[1].Symbol, got.Detractors[2].Symbol})
}

// Portfolio-level results (cash interest, management fees) belong to no
// instrument and are reported as unattributed rather than folded into one.
func TestContributionsDiscloseUnattributedPortfolioLevelResult(t *testing.T) {
	got := CalculateContributions([]InstrumentEconomics{
		{Symbol: "AAPL", CapitalBase: 1000, UnrealizedPnLBase: 100},
	}, -25) // a $25 management fee attached to no symbol

	require.True(t, got.Available)
	assert.InDelta(t, -2.5, got.UnattributedPercentagePoints, 1e-6)
	assert.Equal(t, "incomplete", got.CalculationStatus,
		"an unattributed remainder makes the decomposition incomplete")
}

// The scope limitation is explicit in the payload, never implied.
func TestContributionsDeclareSinceInceptionBasis(t *testing.T) {
	got := CalculateContributions([]InstrumentEconomics{
		{Symbol: "AAPL", CapitalBase: 1000, UnrealizedPnLBase: 100},
	}, 0)
	assert.Equal(t, ContributionBasisSinceInception, got.Basis)
	assert.Equal(t, "complete", got.CalculationStatus)
}

// No committed capital means no defensible denominator: say so, do not divide.
func TestContributionsUnavailableWithoutCapital(t *testing.T) {
	got := CalculateContributions(nil, 0)

	assert.False(t, got.Available)
	assert.Equal(t, "incomplete", got.CalculationStatus)
	assert.NotEmpty(t, got.Reason)
	assert.Empty(t, got.Contributors)
	assert.Empty(t, got.Detractors)
}

// Open and closed episodes of the same symbol are one instrument; realized P&L
// comes from the ledger (a partial sell leaves the position open).
func TestBuildInstrumentEconomicsMergesOpenClosedAndLedger(t *testing.T) {
	ledger := ledgerMetrics{bySymbol: map[string]*InstrumentEconomics{
		"AAPL": {Symbol: "AAPL", RealizedPnLBase: 75, IncomeBase: 12, FeesBase: 3},
		"":     {Symbol: "", RealizedPnLBase: 999},
	}}
	got := buildInstrumentEconomics(
		[]PositionSummary{{Symbol: "AAPL", AssetType: "stock", CostBasisBase: 1000, GainLossBase: 200}},
		[]ClosedPositionSummary{{Symbol: "AAPL", ClosedCostBasisBase: 500}},
		ledger,
	)

	require.Len(t, got, 1, "the empty-symbol ledger bucket is portfolio-level, not an instrument")
	assert.Equal(t, "AAPL", got[0].Symbol)
	assert.Equal(t, "stock", got[0].AssetType)
	assert.InDelta(t, 1500.0, got[0].CapitalBase, 1e-9)
	assert.InDelta(t, 200.0, got[0].UnrealizedPnLBase, 1e-9)
	assert.InDelta(t, 75.0, got[0].RealizedPnLBase, 1e-9)
	assert.InDelta(t, 12.0, got[0].IncomeBase, 1e-9)
	assert.InDelta(t, 3.0, got[0].FeesBase, 1e-9)
	// 200 + 75 + 12 - 3
	assert.InDelta(t, 284.0, got[0].TotalPnLBase(), 1e-9)
}
