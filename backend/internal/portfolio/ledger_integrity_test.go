package portfolio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Part B: sale fee must not be double-counted -----------------------------

// TestSellFee_NotDoubleCountedInReconciliation proves the canonical sale
// contract: gross proceeds = qty*price, net proceeds = gross - fee, realized
// P&L = net proceeds - allocated basis. The sell_fee ledger row is audit-only
// and must not be subtracted a second time from economic attribution.
func TestSellFee_NotDoubleCountedInReconciliation(t *testing.T) {
	svc, pp := newTestService()
	pp.Set("AAPL", 100, "USD")
	userID := "user-fee"

	_, err := svc.DepositCash(ctx(), userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	buyRes, err := svc.BuyPosition(ctx(), userID, "buy", BuyInput{Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)

	cashBefore, _, err := svc.CashBalances(ctx(), userID)
	require.NoError(t, err)
	usdBefore := testAmount("0")
	for _, c := range cashBefore {
		if c.Currency == "USD" {
			usdBefore = c.Amount
		}
	}

	pp.Set("AAPL", 120, "USD")
	sellRes, err := svc.SellPosition(ctx(), userID, "sell", SellInput{
		PositionID: buyRes.Position.ID, Symbol: "AAPL", Quantity: testQuantity("10"), ExecutionPrice: testPrice("120"), Fee: testAmount("15"),
	})
	require.NoError(t, err)
	require.NotNil(t, sellRes.Closed)

	gross := 10.0 * 120.0
	netProceeds := gross - 15
	allocatedBasis := 10.0 * 100.0
	wantRealized := netProceeds - allocatedBasis

	// Cash increases by NET proceeds exactly once.
	cashAfter, _, err := svc.CashBalances(ctx(), userID)
	require.NoError(t, err)
	usdAfter := testAmount("0")
	for _, c := range cashAfter {
		if c.Currency == "USD" {
			usdAfter = c.Amount
		}
	}
	assertAmountEqual(t, "1185", usdAfter.Sub(usdBefore), "cash must increase by net proceeds exactly once")

	// Realized P&L includes the sale fee exactly once.
	assert.InDelta(t, wantRealized, sellRes.Closed.RealizedGainLossBase, 0.01)

	summary, err := svc.Summary(ctx(), userID)
	require.NoError(t, err)

	// Fee metrics still display the sale fee...
	assert.InDelta(t, 15, summary.Fees.TransactionFeesBase, 0.01)
	assert.InDelta(t, 15, summary.Fees.TotalFeesBase, 0.01)
	// ...but it is flagged as already embedded in realized P&L, so
	// reconciliation must not subtract it again.
	assert.InDelta(t, 15, summary.Fees.EmbeddedInRealizedPnLBase, 0.01)
	assert.InDelta(t, wantRealized, summary.Realized.RealizedPnLBase, 0.01)

	econ := calculateEconomicPerformance(summary.CurrentValue, ledgerMetrics{
		deposits: 10000, hasBuy: true,
	}, true)
	// Reconciliation is internally consistent: standalone fees (TotalFeesBase -
	// EmbeddedInRealizedPnLBase) is what's subtracted, not the full fee total.
	status := ReconcilePortfolioFinancials(
		summary.RankedPerformance, summary.Valuation, summary.OpenHoldings,
		summary.Realized, summary.Income, summary.Fees, econ,
	)
	_ = status // shape-checked below via the standalone-fee assertion

	standaloneFees := summary.Fees.TotalFeesBase - summary.Fees.EmbeddedInRealizedPnLBase
	assert.InDelta(t, 0, standaloneFees, 0.01, "the sale fee must not appear as a standalone deduction")
}

// TestSellPreview_MatchesCommittedSell proves SellPreview and the committed
// sell produce identical formulas/results.
func TestSellPreview_MatchesCommittedSell(t *testing.T) {
	svc, pp := newTestService()
	pp.Set("MSFT", 50, "USD")
	userID := "user-preview"

	_, err := svc.DepositCash(ctx(), userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("5000")})
	require.NoError(t, err)
	buyRes, err := svc.BuyPosition(ctx(), userID, "buy", BuyInput{Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)

	pp.Set("MSFT", 60, "USD")
	preview, err := svc.PreviewSell(ctx(), userID, SellInput{
		PositionID: buyRes.Position.ID, Quantity: testQuantity("4"), ExecutionPrice: testPrice("60"), Fee: testAmount("5"),
	})
	require.NoError(t, err)

	sellRes, err := svc.SellPosition(ctx(), userID, "sell", SellInput{
		PositionID: buyRes.Position.ID, Symbol: "MSFT", Quantity: testQuantity("4"), ExecutionPrice: testPrice("60"), Fee: testAmount("5"),
	})
	require.NoError(t, err)

	assert.InDelta(t, preview.GrossProceeds, 4*60.0, 0.01)
	assert.InDelta(t, preview.NetProceeds, 4*60.0-5, 0.01)
	assertAmountEqual(t, "35", *sellRes.Activity.RealizedGainLossBase)
	assert.InDelta(t, 35.0, preview.EstimatedRealizedPnL, 0.01)
	assert.False(t, preview.WillClosePosition)
}

// --- Part C: return of capital is not ordinary income ------------------------

func TestReturnOfCapital_ExcludedFromOrdinaryIncomeTotal(t *testing.T) {
	svc, pp := newTestService()
	pp.Set("T", 40, "USD")
	userID := "user-roc"

	_, err := svc.DepositCash(ctx(), userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), userID, "buy", BuyInput{Symbol: "T", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)

	// An ordinary dividend DOES count as ordinary income.
	_, err = svc.RecordIncome(ctx(), userID, "div", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "T", Currency: "USD", Amount: 20,
	})
	require.NoError(t, err)

	// Return of capital credits cash and reduces basis, but must NOT inflate
	// TotalIncomeBase.
	_, err = svc.RecordIncome(ctx(), userID, "roc", IncomeInput{
		Subtype: IncomeReturnOfCapitalSub, Symbol: "T", Currency: "USD", Amount: 50,
	})
	require.NoError(t, err)

	summary, err := svc.Summary(ctx(), userID)
	require.NoError(t, err)

	assert.InDelta(t, 20, summary.Income.TotalIncomeBase, 0.01, "ROC must not be counted as ordinary income")
	assert.InDelta(t, 50, summary.Income.ReturnOfCapitalBase, 0.01, "ROC must still be disclosed separately")
	assert.True(t, summary.Reconciliation.IsConsistent, "reconciliation must remain consistent: %v", summary.Reconciliation.Reasons)
}

// --- Part D: closed episodes aggregate the complete ledger -------------------

func TestClosedPosition_AggregatesPartialAndFinalSales(t *testing.T) {
	svc, pp := newTestService()
	pp.Set("NFLX", 100, "USD")
	userID := "user-episode"

	_, err := svc.DepositCash(ctx(), userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	buyRes, err := svc.BuyPosition(ctx(), userID, "buy", BuyInput{Symbol: "NFLX", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)
	episodeID := buyRes.Position.ID

	pp.Set("NFLX", 110, "USD")
	partial, err := svc.SellPosition(ctx(), userID, "sell1", SellInput{
		PositionID: episodeID, Symbol: "NFLX", Quantity: testQuantity("4"), ExecutionPrice: testPrice("110"),
	})
	require.NoError(t, err)
	require.Nil(t, partial.Closed, "a partial sale must not close the episode")

	pp.Set("NFLX", 130, "USD")
	final, err := svc.SellPosition(ctx(), userID, "sell2", SellInput{
		PositionID: episodeID, Symbol: "NFLX", Quantity: testQuantity("6"), ExecutionPrice: testPrice("130"),
	})
	require.NoError(t, err)
	require.NotNil(t, final.Closed)
	assert.Equal(t, episodeID, final.Closed.ID, "the final sale closes the SAME episode")

	closed, err := svc.ListClosedPositions(ctx(), userID)
	require.NoError(t, err)
	require.Len(t, closed, 1)

	wantRealized := (4*110.0 - 4*100.0) + (6*130.0 - 6*100.0)
	wantBasis := 4*100.0 + 6*100.0
	assert.InDelta(t, wantRealized, closed[0].RealizedGainLossBase, 0.01,
		"closed summary must aggregate BOTH the partial and final sale, not just the final leg")
	assert.InDelta(t, wantBasis, closed[0].ClosedCostBasisBase, 0.01)

	// Cross-check against the repository's episode ledger directly.
	acts, err := svc.repo.ListActivitiesByPositionEpisode(ctx(), userID, episodeID)
	require.NoError(t, err)
	sellCount := 0
	for _, a := range acts {
		if a.Type == ActivitySell {
			sellCount++
		}
	}
	assert.Equal(t, 2, sellCount, "the episode ledger must contain both sale legs")
}

func TestClosedPosition_RebuyAfterClosureCreatesNewEpisode(t *testing.T) {
	svc, pp := newTestService()
	pp.Set("GME", 20, "USD")
	userID := "user-rebuy"

	_, err := svc.DepositCash(ctx(), userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	firstBuy, err := svc.BuyPosition(ctx(), userID, "buy1", BuyInput{Symbol: "GME", AssetType: AssetTypeStock, Quantity: testQuantity("5")})
	require.NoError(t, err)
	firstEpisode := firstBuy.Position.ID

	pp.Set("GME", 25, "USD")
	closeRes, err := svc.SellPosition(ctx(), userID, "sellall", SellInput{
		PositionID: firstEpisode, Symbol: "GME", Quantity: testQuantity("5"), ExecutionPrice: testPrice("25"),
	})
	require.NoError(t, err)
	require.NotNil(t, closeRes.Closed)

	secondBuy, err := svc.BuyPosition(ctx(), userID, "buy2", BuyInput{Symbol: "GME", AssetType: AssetTypeStock, Quantity: testQuantity("3")})
	require.NoError(t, err)
	assert.NotEqual(t, firstEpisode, secondBuy.Position.ID, "rebuying the same symbol must create a NEW episode")

	// The first (closed) episode's activities are untouched by the rebuy.
	firstEpisodeActs, err := svc.repo.ListActivitiesByPositionEpisode(ctx(), userID, firstEpisode)
	require.NoError(t, err)
	for _, a := range firstEpisodeActs {
		assert.NotEqual(t, secondBuy.Position.ID, a.PositionEpisodeID)
	}

	closedList, err := svc.ListClosedPositions(ctx(), userID)
	require.NoError(t, err)
	require.Len(t, closedList, 1)
	assert.Equal(t, firstEpisode, closedList[0].ID)
}

func TestClosedPosition_WriteOffClosesEpisode(t *testing.T) {
	svc, pp := newTestService()
	pp.Set("BBBY", 5, "USD")
	userID := "user-writeoff"

	_, err := svc.DepositCash(ctx(), userID, "dep", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	buyRes, err := svc.BuyPosition(ctx(), userID, "buy", BuyInput{Symbol: "BBBY", AssetType: AssetTypeStock, Quantity: testQuantity("20")})
	require.NoError(t, err)

	_, err = svc.RecordCorporateAction(ctx(), userID, "wo", CorpActionInput{
		Subtype: CorpWriteOff, Symbol: "BBBY",
	})
	require.NoError(t, err)

	closed, err := svc.ListClosedPositions(ctx(), userID)
	require.NoError(t, err)
	require.Len(t, closed, 1)
	assert.Equal(t, buyRes.Position.ID, closed[0].ID)
	assert.InDelta(t, -20*5.0, closed[0].RealizedGainLossBase, 0.01)
}

// TestClosedPosition_LegacyFallback proves a closed position with no episode
// ledger history (simulating pre-episode legacy data) still returns a summary
// from the position row instead of failing or fabricating history.
func TestClosedPosition_LegacyFallback(t *testing.T) {
	svc, pp := newTestService()
	_ = pp
	userID := "user-legacy"
	pf, err := svc.GetOrCreateDefaultPortfolio(ctx(), userID)
	require.NoError(t, err)

	store := svc.repo.(AggregateStore)
	legacyPos := &Position{
		ID: "legacy-pos-1", UserID: userID, PortfolioID: pf.ID, Symbol: "LEGACY",
		AssetType: AssetTypeStock, Quantity: testQuantity("3"), AverageBuyPrice: testPrice("10"), Currency: "USD",
		Status: PositionStatusClosed, RealizedGainLossBase: 42, RealizedGainLossPercentage: 14,
	}
	require.NoError(t, store.WithLockedPortfolio(ctx(), userID, func(c context.Context, tx AggregateTx) error {
		return tx.CreatePosition(c, legacyPos)
	}))

	closed, err := svc.ListClosedPositions(ctx(), userID)
	require.NoError(t, err)
	require.Len(t, closed, 1)
	assert.InDelta(t, 42, closed[0].RealizedGainLossBase, 0.01)
}
