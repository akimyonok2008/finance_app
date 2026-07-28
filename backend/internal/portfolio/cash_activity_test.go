package portfolio

import (
	"sync"
	"testing"

	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCashFundedBuyWeightedAverageAndPartialSale(t *testing.T) {
	svc, repo, _, quotes := newTxTestService()

	deposit, err := svc.DepositCash(ctx(), "u1", "deposit-1", CashFlowInput{Currency: "usd", Amount: testAmount("5000")})
	require.NoError(t, err)
	assertIndexValuesEqual(t, deposit.RankedIndexBefore, deposit.RankedIndexAfter)

	first, err := svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("10"),
	})
	require.NoError(t, err)
	require.NotNil(t, first.Position)
	assertPriceEqual(t, "195", first.Position.AverageBuyPrice)
	assertIndexValuesEqual(t, first.RankedIndexBefore, first.RankedIndexAfter)

	quotes.Set("AAPL", 100, "USD")
	second, err := svc.BuyPosition(ctx(), "u1", "buy-2", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("10"),
	})
	require.NoError(t, err)
	require.NotNil(t, second.Position)
	assertQuantityEqual(t, "20", second.Position.Quantity)
	assertPriceEqual(t, "147.5", second.Position.AverageBuyPrice)

	quotes.Set("AAPL", 200, "USD")
	sale, err := svc.SellPosition(ctx(), "u1", "sell-1", SellInput{
		PositionID: second.Position.ID, Quantity: testQuantity("5"),
	})
	require.NoError(t, err)
	require.NotNil(t, sale.Position)
	assertQuantityEqual(t, "15", sale.Position.Quantity)
	assertPriceEqual(t, "147.5", sale.Position.AverageBuyPrice)
	require.NotNil(t, sale.Activity.RealizedGainLossBase)
	assertAmountEqual(t, "262.5", *sale.Activity.RealizedGainLossBase)
	assertIndexValuesEqual(t, sale.RankedIndexBefore, sale.RankedIndexAfter)

	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, cash, 1)
	assertAmountEqual(t, "3050", cash[0].Amount)

	summary, err := svc.Summary(ctx(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, 3000, summary.Valuation.OpenHoldingsMarketValueBase, 0.01)
	assert.InDelta(t, 3050, summary.Valuation.CashValueBase, 0.01)
	assert.InDelta(t, 6050, summary.Valuation.CurrentPortfolioValueBase, 0.01)
	assert.InDelta(t, 2212.5, summary.OpenHoldings.CostBasisBase, 0.01)
	assert.InDelta(t, 787.5, summary.OpenHoldings.UnrealizedPnLBase, 0.01)
	assert.InDelta(t, 262.5, summary.Realized.RealizedPnLBase, 0.01)
	require.NotNil(t, summary.EconomicPerformance.TotalPnLBase)
	assert.InDelta(t, 1050, *summary.EconomicPerformance.TotalPnLBase, 0.01)
	assert.True(t, summary.EconomicPerformance.IsComplete)
	assert.True(t, summary.Reconciliation.IsConsistent)
}

func TestSellPreview_IsReadOnlyAndCalculatesNetEconomics(t *testing.T) {
	svc, repo, _, quotes := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")
	quotes.Set("AAPL", 200, "USD")

	beforeCash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	beforeActivities, err := repo.ListActivities(ctx(), "u1", 100)
	require.NoError(t, err)

	preview, err := svc.PreviewSell(ctx(), "u1", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("5"), ExecutionPrice: testPrice("200"), Fee: testAmount("5"),
	})
	require.NoError(t, err)
	assert.Equal(t, buy.Position.ID, preview.PositionEpisodeID)
	assert.Equal(t, 10.0, preview.AvailableQuantity.Float64())
	assert.Equal(t, 5.0, preview.RemainingQuantity.Float64())
	assert.Equal(t, 1000.0, preview.GrossProceeds.Float64())
	assert.Equal(t, 995.0, preview.NetProceeds.Float64())
	assert.Equal(t, 975.0, preview.AllocatedBasis.Float64())
	assert.Equal(t, 20.0, preview.EstimatedRealizedPnL.Float64())
	assert.False(t, preview.WillClosePosition)

	afterCash, _ := repo.ListCashBalances(ctx(), "u1")
	afterActivities, _ := repo.ListActivities(ctx(), "u1", 100)
	assert.Equal(t, beforeCash, afterCash)
	assert.Equal(t, beforeActivities, afterActivities)
}

func TestSellFee_IsCountedOnceAndPartialEpisodeRemainsOpen(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")

	sale, err := svc.SellPosition(ctx(), "u1", "sale-with-fee", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("1"), ExecutionPrice: testPrice("195"), Fee: testAmount("5"),
	})
	require.NoError(t, err)
	require.NotNil(t, sale.Position)
	assertQuantityEqual(t, "9", sale.Position.Quantity)
	assertPriceEqual(t, "195", sale.Position.AverageBuyPrice)
	assert.Less(t, sale.RankedIndexAfter.Cmp(sale.RankedIndexBefore), 0)

	cash, _ := repo.ListCashBalances(ctx(), "u1")
	assertAmountEqual(t, "8240", cash[0].Amount)
	activities, _ := repo.ListActivities(ctx(), "u1", 100)
	var sells, sellFees int
	for _, activity := range activities {
		if activity.Type == ActivitySell {
			sells++
			assert.Equal(t, buy.Position.ID, activity.PositionEpisodeID)
			assertAmountEqual(t, "190", activity.NetAmount)
		}
		if activity.Type == ActivitySellFee {
			sellFees++
		}
	}
	assert.Equal(t, 1, sells)
	assert.Equal(t, 1, sellFees)

	summary, err := svc.Summary(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, 5.0, summary.Fees.TransactionFeesBase)
	require.NotNil(t, summary.EconomicPerformance.TotalPnLBase)
	assert.Equal(t, -5.0, *summary.EconomicPerformance.TotalPnLBase)
}

func TestFullSaleClosesEpisodeAndRebuyCreatesNewEpisode(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	_, err := svc.DepositCash(ctx(), "u1", "episode-deposit", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	first, err := svc.BuyPosition(ctx(), "u1", "episode-buy-1", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	_, err = svc.SellPosition(ctx(), "u1", "episode-sell", SellInput{
		PositionID: first.Position.ID, Quantity: testQuantity("1"), ExecutionPrice: testPrice("195"),
	})
	require.NoError(t, err)
	second, err := svc.BuyPosition(ctx(), "u1", "episode-buy-2", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)

	assert.NotEqual(t, first.Position.ID, second.Position.ID)
	positions, err := repo.ListPositionsByUser(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 2)
	assert.Equal(t, PositionStatusClosed, positionStatus(positions[0]))
	assert.Equal(t, PositionStatusOpen, positionStatus(positions[1]))
}

func TestFullSaleLeavesCashPortfolioActiveAndFinalWithdrawalPauses(t *testing.T) {
	svc, _, perf, _ := newTxTestService()
	_, err := svc.DepositCash(ctx(), "u1", "d1", CashFlowInput{Currency: "USD", Amount: testAmount("195")})
	require.NoError(t, err)
	buy, err := svc.BuyPosition(ctx(), "u1", "b1", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	_, err = svc.SellPosition(ctx(), "u1", "s1", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	ranked, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, performance.StatusActive, ranked.Status)

	withdrawal, err := svc.WithdrawCash(ctx(), "u1", "w1", CashFlowInput{Currency: "USD", Amount: testAmount("195")})
	require.NoError(t, err)
	assertIndexValuesEqual(t, withdrawal.RankedIndexBefore, withdrawal.RankedIndexAfter)
	ranked, err = perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, performance.StatusPaused, ranked.Status)
}

func TestCashMutationIdempotencyAndConcurrentOverspend(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	first, err := svc.DepositCash(ctx(), "u1", "same-deposit", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	retry, err := svc.DepositCash(ctx(), "u1", "same-deposit", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
	assert.Equal(t, first.PortfolioVersion, retry.PortfolioVersion)
	require.NotNil(t, retry.Activity)
	assert.Equal(t, first.Activity.ID, retry.Activity.ID)
	otherUser, err := svc.DepositCash(ctx(), "u2", "same-deposit", CashFlowInput{Currency: "USD", Amount: testAmount("50")})
	require.NoError(t, err)
	assert.False(t, otherUser.Duplicate, "idempotency keys are scoped per portfolio")

	// Concurrent buys against one cash balance. Automatic purchase funding is
	// the DEFAULT: neither buy is rejected for insufficient cash — the shortfall
	// is funded automatically, in the instrument's quote currency, and cash can
	// never go negative.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, symbol := range []string{"MSFT", "NVDA"} {
		wg.Add(1)
		go func(index int, symbol string) {
			defer wg.Done()
			_, errs[index] = svc.BuyPosition(ctx(), "u1", "buy-"+symbol, BuyInput{
				Symbol: symbol, AssetType: AssetTypeStock, Quantity: testQuantity("3"),
			})
		}(i, symbol)
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err, "automatic funding must not reject a purchase")
	}
	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cash[0].Amount.Cmp(testAmount("0")), 0)

	activities, err := repo.ListActivities(ctx(), "u1", 100)
	require.NoError(t, err)
	userDeposits, autoFunding := 0, 0
	for _, activity := range activities {
		if activity.Type != ActivityDeposit {
			continue
		}
		if activity.Metadata["funding_reason"] == "purchase_shortfall" {
			autoFunding++
			assert.Equal(t, true, activity.Metadata["automatic"])
			assert.NotEmpty(t, activity.GroupID, "funding must be grouped with its purchase")
		} else {
			userDeposits++
		}
	}
	assert.Equal(t, 1, userDeposits, "the user's own deposit is recorded exactly once")
	assert.GreaterOrEqual(t, autoFunding, 1, "at least one purchase needed automatic funding")
}

// TestBuyRejectsWhenAutoFundingDisabled proves the opt-out still works: with the
// portfolio preference off, a purchase beyond available cash is refused.
func TestBuyRejectsWhenAutoFundingDisabled(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	_, err := svc.DepositCash(ctx(), "u1", "seed", CashFlowInput{Currency: "USD", Amount: testAmount("10")})
	require.NoError(t, err)
	pf, err := repo.EnsureDefaultPortfolio(ctx(), "u1")
	require.NoError(t, err)
	require.True(t, pf.AutoFundPurchases, "automatic funding is on by default")
	require.NoError(t, repo.SetAutoFundPurchases(ctx(), "u1", false))

	_, err = svc.BuyPosition(ctx(), "u1", "blocked", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("3"),
	})
	assert.ErrorIs(t, err, ErrInsufficientCash)
}

func TestCashValidation(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.DepositCash(ctx(), "u1", "bad-currency", CashFlowInput{Currency: "JPY", Amount: testAmount("10")})
	assert.ErrorIs(t, err, ErrUnsupportedCurrency)
	_, err = svc.DepositCash(ctx(), "u1", "bad-amount", CashFlowInput{Currency: "USD", Amount: testAmount("0")})
	assert.ErrorIs(t, err, ErrInvalidCashAmount)
	_, err = svc.WithdrawCash(ctx(), "u1", "too-much", CashFlowInput{Currency: "USD", Amount: testAmount("1")})
	assert.ErrorIs(t, err, ErrInsufficientCash)
}
