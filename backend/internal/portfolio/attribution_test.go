package portfolio

import (
	"testing"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func floatPtr(v float64) *float64 { return &v }

func amt(v float64) money.Amount { return money.AmountFromFloat64(v) }

func amtPtr(v float64) *money.Amount {
	a := money.AmountFromFloat64(v)
	return &a
}

// Standalone fees exclude the sale/purchase fees already netted into realized
// P&L and cost basis. Subtracting TotalFeesBase in full double-counts them.
func TestStandaloneFeesBaseExcludesEmbeddedTradeFees(t *testing.T) {
	fees := FeeMetrics{TotalFeesBase: amt(50), EmbeddedInRealizedPnLBase: amt(12)}
	assert.InDelta(t, 38.0, StandaloneFeesBase(fees).Float64(), 1e-9)
}

// The economic breakdown must be the SAME decomposition reconciliation checks:
// realized + unrealized + income - standalone fees. If they ever disagree the
// Performance tab would contradict the reconciliation status beside it.
func TestEconomicAttributionMatchesReconciliationDecomposition(t *testing.T) {
	open := OpenHoldingsMetrics{UnrealizedPnLBase: amt(400), CostBasisBase: amt(2000)}
	realized := RealizedMetrics{RealizedPnLBase: amt(150)}
	income := IncomeMetrics{TotalIncomeBase: amt(60)}
	fees := FeeMetrics{TotalFeesBase: amt(30), EmbeddedInRealizedPnLBase: amt(10)}
	// 400 + 150 + 60 - 20 = 590
	economic := EconomicPerformance{
		TotalPnLBase: amtPtr(590), CalculationStatus: "complete", IsComplete: true,
	}

	got := CalculateEconomicAttribution(open, realized, income, fees, economic)

	assert.InDelta(t, 150.0, got.RealizedPnLBase.Float64(), 1e-9)
	assert.InDelta(t, 400.0, got.UnrealizedPnLBase.Float64(), 1e-9)
	assert.InDelta(t, 60.0, got.NetIncomeBase.Float64(), 1e-9)
	assert.InDelta(t, 20.0, got.StandaloneFeesBase.Float64(), 1e-9)
	assert.InDelta(t, 590.0, got.AttributedTotalBase.Float64(), 1e-9)
	require.NotNil(t, got.TotalEconomicPnLBase)
	assert.InDelta(t, 590.0, got.TotalEconomicPnLBase.Float64(), 1e-9)
	require.NotNil(t, got.UnattributedBase)
	assert.InDelta(t, 0.0, got.UnattributedBase.Float64(), 1e-9)

	// The reconciliation status computed from the same inputs must agree.
	status := ReconcilePortfolioFinancials(
		RankedPerformanceView{Index: 100, ReturnPercentage: 0},
		PortfolioValuation{OpenHoldingsMarketValueBase: amt(2400), CashValueBase: amt(0), CurrentPortfolioValueBase: amt(2400)},
		OpenHoldingsMetrics{UnrealizedPnLBase: amt(400), CostBasisBase: amt(2000)},
		realized, income, fees, economic,
	)
	assert.True(t, status.IsConsistent, "reasons: %v", status.Reasons)
	assert.InDelta(t, 0.0, status.Difference.Float64(), 1e-9)
}

// An incomplete ledger must leave the total NIL. Rendering 0 would tell the
// user they broke even when the truth is "we do not know".
func TestEconomicAttributionLeavesIncompleteTotalNil(t *testing.T) {
	got := CalculateEconomicAttribution(
		OpenHoldingsMetrics{UnrealizedPnLBase: amt(100)},
		RealizedMetrics{}, IncomeMetrics{}, FeeMetrics{},
		EconomicPerformance{CalculationStatus: "legacy_estimate", IsComplete: false},
	)

	assert.Nil(t, got.TotalEconomicPnLBase)
	assert.Nil(t, got.UnattributedBase)
	assert.False(t, got.IsComplete)
	assert.Equal(t, "legacy_estimate", got.CalculationStatus)
	// The attributable part is still known and is still shown.
	assert.InDelta(t, 100.0, got.AttributedTotalBase.Float64(), 1e-9)
}

// A gap between the ledger total and the attributable parts is DISCLOSED.
func TestEconomicAttributionDisclosesUnattributedResidual(t *testing.T) {
	got := CalculateEconomicAttribution(
		OpenHoldingsMetrics{UnrealizedPnLBase: amt(100)},
		RealizedMetrics{RealizedPnLBase: amt(0)}, IncomeMetrics{}, FeeMetrics{},
		EconomicPerformance{TotalPnLBase: amtPtr(130), CalculationStatus: "complete", IsComplete: true},
	)

	require.NotNil(t, got.UnattributedBase)
	assert.InDelta(t, 30.0, got.UnattributedBase.Float64(), 1e-9)
}

// Contribution is weight x return, NOT standalone return. A tiny position with
// a spectacular return must not outrank a large position with a solid one.
func TestContributionsRankByWeightedContributionNotStandaloneReturn(t *testing.T) {
	instruments := []InstrumentEconomics{
		// 1% of capital, +100% return -> contributes 1.0 point.
		{Symbol: "MOON", CapitalBase: amt(100), UnrealizedPnLBase: amt(100)},
		// 90% of capital, +10% return -> contributes 9.0 points.
		{Symbol: "BIG", CapitalBase: amt(9000), UnrealizedPnLBase: amt(900)},
		{Symbol: "FLAT", CapitalBase: amt(900)},
	}
	got := CalculateContributions(instruments, amt(0))

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
		{Symbol: "DIVY", CapitalBase: amt(1000), IncomeBase: amt(50)},
		{Symbol: "SOLD", CapitalBase: amt(1000), RealizedPnLBase: amt(100)},
		{Symbol: "COSTLY", CapitalBase: amt(1000), FeesBase: amt(30)},
	}, amt(0))

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
		{Symbol: "A", CapitalBase: amt(1000), UnrealizedPnLBase: amt(40)},
		{Symbol: "B", CapitalBase: amt(1000), UnrealizedPnLBase: amt(30)},
		{Symbol: "C", CapitalBase: amt(1000), UnrealizedPnLBase: amt(20)},
		{Symbol: "D", CapitalBase: amt(1000), UnrealizedPnLBase: amt(10)},
		{Symbol: "W", CapitalBase: amt(1000), UnrealizedPnLBase: amt(-40)},
		{Symbol: "X", CapitalBase: amt(1000), UnrealizedPnLBase: amt(-30)},
		{Symbol: "Y", CapitalBase: amt(1000), UnrealizedPnLBase: amt(-20)},
		{Symbol: "Z", CapitalBase: amt(1000), UnrealizedPnLBase: amt(-10)},
	}
	got := CalculateContributions(instruments, amt(0))

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
		{Symbol: "AAPL", CapitalBase: amt(1000), UnrealizedPnLBase: amt(100)},
	}, amt(-25)) // a $25 management fee attached to no symbol

	require.True(t, got.Available)
	assert.InDelta(t, -2.5, got.UnattributedPercentagePoints, 1e-6)
	assert.Equal(t, "incomplete", got.CalculationStatus,
		"an unattributed remainder makes the decomposition incomplete")
}

// The scope limitation is explicit in the payload, never implied.
func TestContributionsDeclareSinceInceptionBasis(t *testing.T) {
	got := CalculateContributions([]InstrumentEconomics{
		{Symbol: "AAPL", CapitalBase: amt(1000), UnrealizedPnLBase: amt(100)},
	}, amt(0))
	assert.Equal(t, ContributionBasisSinceInception, got.Basis)
	assert.Equal(t, "complete", got.CalculationStatus)
}

// No committed capital means no defensible denominator: say so, do not divide.
func TestContributionsUnavailableWithoutCapital(t *testing.T) {
	got := CalculateContributions(nil, amt(0))

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
		"AAPL": {Symbol: "AAPL", RealizedPnLBase: amt(75), IncomeBase: amt(12), FeesBase: amt(3)},
		"":     {Symbol: "", RealizedPnLBase: amt(999)},
	}}
	got := buildInstrumentEconomics(
		[]PositionSummary{{Symbol: "AAPL", AssetType: "stock", CostBasisBase: money.AmountFromFloat64(1000), GainLossBase: money.AmountFromFloat64(200)}},
		[]ClosedPositionSummary{{Symbol: "AAPL", ClosedCostBasisBase: money.AmountFromFloat64(500)}},
		ledger,
	)

	require.Len(t, got, 1, "the empty-symbol ledger bucket is portfolio-level, not an instrument")
	assert.Equal(t, "AAPL", got[0].Symbol)
	assert.Equal(t, "stock", got[0].AssetType)
	assert.InDelta(t, 1500.0, got[0].CapitalBase.Float64(), 1e-9)
	assert.InDelta(t, 200.0, got[0].UnrealizedPnLBase.Float64(), 1e-9)
	assert.InDelta(t, 75.0, got[0].RealizedPnLBase.Float64(), 1e-9)
	assert.InDelta(t, 12.0, got[0].IncomeBase.Float64(), 1e-9)
	assert.InDelta(t, 3.0, got[0].FeesBase.Float64(), 1e-9)
	// 200 + 75 + 12 - 3
	assert.InDelta(t, 284.0, got[0].TotalPnLBase().Float64(), 1e-9)
}
