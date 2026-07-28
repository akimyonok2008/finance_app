package portfolio

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// This file covers Step 3: accurate user-recorded buys and sells — real
// execution details with provenance, automatic purchase funding, non-mutating
// previews, idempotency, and atomicity. There is no backdating: every trade
// is recorded against the live quote at entry time.

func seedCash(t *testing.T, svc *Service, user, currency, amount string) {
	t.Helper()
	_, err := svc.DepositCash(ctx(), user, "seed-"+user+"-"+currency, CashFlowInput{
		Currency: currency, Amount: testAmount(amount),
	})
	require.NoError(t, err)
}

func activitiesOf(t *testing.T, repo *InMemoryRepository, user string) []Activity {
	t.Helper()
	list, err := repo.ListActivities(ctx(), user, 200)
	require.NoError(t, err)
	return list
}

func findActivity(list []Activity, kind ActivityType) (Activity, bool) {
	for _, a := range list {
		if a.Type == kind {
			return a, true
		}
	}
	return Activity{}, false
}

func autoFundingActivities(list []Activity) []Activity {
	var out []Activity
	for _, a := range list {
		if a.Type == ActivityDeposit && a.Metadata["funding_reason"] == "purchase_shortfall" {
			out = append(out, a)
		}
	}
	return out
}

// withClockAt temporarily pins the coordinator's clock to `at` for the
// duration of fn, restoring the real clock afterward. There is no
// user-facing way to backdate a trade — this exists purely so a test can
// pin the OccurredAt/CreatedAt of a mutation without a real time.Sleep.
func withClockAt(svc *Service, at time.Time, fn func()) {
	svc.Coordinator().SetClock(func() time.Time { return at })
	defer svc.Coordinator().SetClock(func() time.Time { return time.Now().UTC() })
	fn()
}

func cashOf(t *testing.T, repo *InMemoryRepository, user, currency string) money.Amount {
	t.Helper()
	balances, err := repo.ListCashBalances(ctx(), user)
	require.NoError(t, err)
	for _, b := range balances {
		if b.Currency == currency {
			return b.Amount
		}
	}
	return testAmount("0")
}

// 1. Buy with no cash at all → the whole purchase is funded automatically.
func TestBuy_NoCash_FullAutomaticFunding(t *testing.T) {
	svc, repo, _, _ := newTxTestService()

	res, err := svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), Fee: testAmount("5"),
	})
	require.NoError(t, err)
	require.NotNil(t, res.Activity)

	list := activitiesOf(t, repo, "u1")
	funding := autoFundingActivities(list)
	require.Len(t, funding, 1)
	// 2 x 430 + 5 fee = 865, none of it available.
	assertAmountEqual(t, "865", funding[0].GrossAmount)
	assert.Equal(t, "USD", funding[0].Currency, "funding uses the instrument's quote currency")
	assert.Equal(t, true, funding[0].Metadata["automatic"])

	buy, ok := findActivity(list, ActivityBuy)
	require.True(t, ok)
	fee, ok := findActivity(list, ActivityBuyFee)
	require.True(t, ok)

	// One purchase = one activity group.
	assert.NotEmpty(t, buy.GroupID)
	assert.Equal(t, buy.GroupID, fee.GroupID)
	assert.Equal(t, buy.GroupID, funding[0].GroupID)
	assert.Equal(t, buy.GroupID, funding[0].Metadata["linked_purchase_group_id"])

	// Provenance: no price entered → provider estimate; fee entered → user.
	assert.Equal(t, PriceSourceProviderEstimate, buy.Metadata["execution_price_source"])
	assert.Equal(t, FeeSourceUserRecorded, buy.Metadata["fee_source"])
	assert.NotEmpty(t, buy.Metadata["effective_at"])
	assert.NotEmpty(t, buy.Metadata["recorded_at"])

	// Cash is exactly drained, never negative.
	assertAmountEqual(t, "0", cashOf(t, repo, "u1", "USD"))

	// Ranked: the funding and allocation are neutral; only the fee is a return.
	assert.Less(t, res.RankedIndexAfter.Cmp(res.RankedIndexBefore), 0,
		"the purchase fee is the only return-bearing part of the buy")
}

// 1b. A fee-free automatically-funded buy is fully ranked-neutral.
func TestBuy_NoCashNoFee_IsRankedNeutral(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	res, err := svc.BuyPosition(ctx(), "u1", "buy-neutral", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)
	assertIndexValuesEqual(t, res.RankedIndexBefore, res.RankedIndexAfter)
}

// 2. Buy with partial cash → existing cash is used first, the rest is funded.
func TestBuy_PartialCash_SplitsCorrectly(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "500")

	_, err := svc.BuyPosition(ctx(), "u1", "buy-partial", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), // 860 required
	})
	require.NoError(t, err)

	funding := autoFundingActivities(activitiesOf(t, repo, "u1"))
	require.Len(t, funding, 1)
	assertAmountEqual(t, "360", funding[0].GrossAmount, "860 required - 500 available")
	assertAmountEqual(t, "0", cashOf(t, repo, "u1", "USD"))
}

// 3. Buy with sufficient cash → no funding activity at all.
func TestBuy_SufficientCash_NoFundingActivity(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "2000")

	_, err := svc.BuyPosition(ctx(), "u1", "buy-funded", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)

	list := activitiesOf(t, repo, "u1")
	assert.Empty(t, autoFundingActivities(list))
	assertAmountEqual(t, "1140", cashOf(t, repo, "u1", "USD"))
}

// Entered execution price and fee are honoured verbatim, with user provenance.
func TestBuy_UserEnteredExecutionDetails(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "5000")
	when := time.Now().UTC().Add(-90 * time.Minute)

	withClockAt(svc, when, func() {
		_, err := svc.BuyPosition(ctx(), "u1", "buy-entered", BuyInput{
			Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("4"),
			ExecutionPrice: testPrice("400"), Fee: testAmount("12"),
		})
		require.NoError(t, err)
	})

	buy, ok := findActivity(activitiesOf(t, repo, "u1"), ActivityBuy)
	require.True(t, ok)
	require.NotNil(t, buy.UnitPrice)
	assertPriceEqual(t, "400", *buy.UnitPrice)
	assertAmountEqual(t, "1600", buy.GrossAmount)
	assertAmountEqual(t, "12", buy.FeeAmount)
	assert.Equal(t, PriceSourceUserRecorded, buy.Metadata["execution_price_source"])
	assert.Equal(t, FeeSourceUserRecorded, buy.Metadata["fee_source"])
	assert.True(t, buy.OccurredAt.Equal(when), "the trade is recorded at the moment it happened")
	assert.True(t, buy.CreatedAt.Equal(buy.OccurredAt), "there is no separate backdated effective_at: occurred_at and recorded_at are the same instant")

	// Basis includes price AND fee: (1600 + 12) / 4 = 403.
	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertPriceEqual(t, "403", positions[0].AverageBuyPrice)
	assertAmountEqual(t, "3388", cashOf(t, repo, "u1", "USD"))
}

// 7. Weighted-average basis across two buys (fees included).
func TestBuy_WeightedAverageBasisAcrossTwoBuys(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "10000")

	_, err := svc.BuyPosition(ctx(), "u1", "wa-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), ExecutionPrice: testPrice("400"),
	})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), "u1", "wa-2", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("3"), ExecutionPrice: testPrice("500"), Fee: testAmount("10"),
	})
	require.NoError(t, err)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertQuantityEqual(t, "5", positions[0].Quantity)
	// (2*400 + 3*500 + 10) / 5 = 462
	assertPriceEqual(t, "462", positions[0].AverageBuyPrice)
}

// 8. A rebuy after a full sale opens a NEW episode via the new buy path.
func TestBuy_RebuyAfterCloseCreatesNewEpisode(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "10000")

	first, err := svc.BuyPosition(ctx(), "u1", "ep-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)
	firstEpisode := first.Position.ID

	_, err = svc.SellPosition(ctx(), "u1", "ep-sell", SellInput{
		PositionID: firstEpisode, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)

	second, err := svc.BuyPosition(ctx(), "u1", "ep-2", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	assert.NotEqual(t, firstEpisode, second.Position.ID, "a rebuy starts a new episode")

	for _, a := range activitiesOf(t, repo, "u1") {
		if a.Type == ActivityBuy || a.Type == ActivitySell {
			assert.NotEmpty(t, a.PositionEpisodeID)
		}
	}
}

// 4. Buy preview is non-mutating and reports the funding split correctly.
func TestBuyPreview_NonMutatingAndCorrect(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "500")

	before := activitiesOf(t, repo, "u1")
	beforeCash := cashOf(t, repo, "u1", "USD")
	beforePositions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	beforePortfolio, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)

	in := BuyInput{Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), Fee: testAmount("5")}
	first, err := svc.PreviewBuy(ctx(), "u1", in)
	require.NoError(t, err)
	second, err := svc.PreviewBuy(ctx(), "u1", in)
	require.NoError(t, err)

	assert.InDelta(t, 860.0, first.GrossPurchaseAmount.Float64(), 1e-9)
	assert.InDelta(t, 5.0, first.Fee.Float64(), 1e-9)
	assert.InDelta(t, 865.0, first.TotalCashRequired.Float64(), 1e-9)
	assert.InDelta(t, 500.0, first.AvailableCash.Float64(), 1e-9)
	assert.InDelta(t, 500.0, first.CashUsed.Float64(), 1e-9)
	assert.InDelta(t, 365.0, first.AutomaticFunding.Float64(), 1e-9)
	assert.InDelta(t, 0.0, first.RemainingCash.Float64(), 1e-9)
	assert.True(t, first.CreatesNewEpisode)
	assert.Equal(t, PriceSourceProviderEstimate, first.ExecutionPriceSource)
	assert.Equal(t, FeeSourceUserRecorded, first.FeeSource)
	assert.Equal(t, "complete", first.CalculationStatus)
	assert.NotEmpty(t, first.EffectiveAt)

	// Repeating the preview changed nothing anywhere.
	first.EffectiveAt, second.EffectiveAt = "", ""
	assert.Equal(t, first, second)
	assert.Equal(t, len(before), len(activitiesOf(t, repo, "u1")))
	assertAmountValuesEqual(t, beforeCash, cashOf(t, repo, "u1", "USD"))
	afterPositions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, len(beforePositions), len(afterPositions))
	afterPortfolio, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, beforePortfolio.Version, afterPortfolio.Version,
		"a preview must never bump the aggregate version")
}

func TestBuyPreview_ExtendsExistingEpisode(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "10000")
	res, err := svc.BuyPosition(ctx(), "u1", "pv-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), ExecutionPrice: testPrice("400"),
	})
	require.NoError(t, err)

	preview, err := svc.PreviewBuy(ctx(), "u1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), ExecutionPrice: testPrice("500"),
	})
	require.NoError(t, err)
	assert.False(t, preview.CreatesNewEpisode)
	assert.Equal(t, res.Position.ID, preview.PositionEpisodeID)
	assert.InDelta(t, 4.0, preview.ResultingQuantity.Float64(), 1e-9)
	assert.InDelta(t, 450.0, preview.ResultingAverageCost.Float64(), 1e-9)
	assert.Equal(t, PriceSourceUserRecorded, preview.ExecutionPriceSource)
}

// 5. Idempotency: the same key applied twice produces exactly one effect.
func TestBuy_IdempotentRetryHasSingleEffect(t *testing.T) {
	svc, repo, _, _ := newTxTestService()

	first, err := svc.BuyPosition(ctx(), "u1", "idem-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), Fee: testAmount("5"),
	})
	require.NoError(t, err)
	retry, err := svc.BuyPosition(ctx(), "u1", "idem-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), Fee: testAmount("5"),
	})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
	assert.Equal(t, first.PortfolioVersion, retry.PortfolioVersion)

	list := activitiesOf(t, repo, "u1")
	buys, fees := 0, 0
	for _, a := range list {
		switch a.Type {
		case ActivityBuy:
			buys++
		case ActivityBuyFee:
			fees++
		}
	}
	assert.Equal(t, 1, buys)
	assert.Equal(t, 1, fees)
	assert.Len(t, autoFundingActivities(list), 1)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertQuantityEqual(t, "2", positions[0].Quantity)
}

func TestSell_IdempotentRetryHasSingleEffect(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "sell-idem-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("4"),
	})
	require.NoError(t, err)

	first, err := svc.SellPosition(ctx(), "u1", "idem-sell", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("2"), Fee: testAmount("3"),
	})
	require.NoError(t, err)
	retry, err := svc.SellPosition(ctx(), "u1", "idem-sell", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("2"), Fee: testAmount("3"),
	})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
	assert.Equal(t, first.PortfolioVersion, retry.PortfolioVersion)

	sells := 0
	for _, a := range activitiesOf(t, repo, "u1") {
		if a.Type == ActivitySell {
			sells++
		}
	}
	assert.Equal(t, 1, sells)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertQuantityEqual(t, "2", positions[0].Quantity)
}

// 6. Atomicity: a failure partway through a multi-activity automatically-funded
// buy commits nothing at all.
func TestBuy_RollbackCommitsNothing(t *testing.T) {
	boom := errors.New("injected failure")
	for name, faults := range map[string]Faults{
		"record activity": {RecordActivity: boom},
		"put cash":        {PutCash: boom},
		"create position": {CreatePosition: boom},
		"audit":           {RecordAudit: boom},
		"outbox":          {AppendOutbox: boom},
		"commit":          {Commit: boom},
	} {
		t.Run(name, func(t *testing.T) {
			svc, repo, _, _ := newTxTestService()
			seedCash(t, svc, "u1", "USD", "100")
			cashBefore := cashOf(t, repo, "u1", "USD")
			activitiesBefore := len(activitiesOf(t, repo, "u1"))
			pfBefore, err := repo.GetPortfolioByUser(ctx(), "u1")
			require.NoError(t, err)

			repo.SetFaults(faults)
			_, err = svc.BuyPosition(ctx(), "u1", "atomic-buy", BuyInput{
				Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), Fee: testAmount("5"),
			})
			require.Error(t, err)
			repo.SetFaults(Faults{})

			positions, err := svc.ListPositions(ctx(), "u1")
			require.NoError(t, err)
			assert.Empty(t, positions, "no position committed")
			assertAmountValuesEqual(t, cashBefore, cashOf(t, repo, "u1", "USD"), "no cash committed")
			assert.Equal(t, activitiesBefore, len(activitiesOf(t, repo, "u1")),
				"no activity (buy, fee, or automatic funding) committed")
			pfAfter, err := repo.GetPortfolioByUser(ctx(), "u1")
			require.NoError(t, err)
			assert.Equal(t, pfBefore.Version, pfAfter.Version, "no version bump")
		})
	}
}

// 9. Sell with entered execution price/date/fee.
func TestSell_UserEnteredExecutionDetails(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	bought := time.Now().UTC().Add(-2 * time.Hour)
	withClockAt(svc, bought, func() {
		_, err := svc.BuyPosition(ctx(), "u1", "s-buy", BuyInput{
			Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("4"), ExecutionPrice: testPrice("400"),
		})
		require.NoError(t, err)
	})
	cashAfterBuy := cashOf(t, repo, "u1", "USD")
	when := time.Now().UTC().Add(-30 * time.Minute)

	withClockAt(svc, when, func() {
		_, err := svc.SellPosition(ctx(), "u1", "s-sell", SellInput{
			Symbol: "MSFT", Quantity: testQuantity("2"), ExecutionPrice: testPrice("450"), Fee: testAmount("7"),
		})
		require.NoError(t, err)
	})

	sell, ok := findActivity(activitiesOf(t, repo, "u1"), ActivitySell)
	require.True(t, ok)
	assert.Equal(t, PriceSourceUserRecorded, sell.Metadata["execution_price_source"])
	assert.Equal(t, FeeSourceUserRecorded, sell.Metadata["fee_source"])
	assert.True(t, sell.OccurredAt.Equal(when))
	// gross = 2*450 = 900; net = 900 - 7 = 893; basis = 2*400 = 800; P&L = 93.
	assertAmountEqual(t, "900", sell.GrossAmount)
	assertAmountEqual(t, "893", sell.NetAmount)
	require.NotNil(t, sell.RealizedGainLossBase)
	assertAmountEqual(t, "93", *sell.RealizedGainLossBase)
	assertAmountValuesEqual(t, cashAfterBuy.Add(testAmount("893")), cashOf(t, repo, "u1", "USD"))
}

// 10. Sell with no entered price falls back to the provider estimate.
func TestSell_DefaultPriceIsLabelledProviderEstimate(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "est-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)
	_, err = svc.SellPosition(ctx(), "u1", "est-sell", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)

	sell, ok := findActivity(activitiesOf(t, repo, "u1"), ActivitySell)
	require.True(t, ok)
	assert.Equal(t, PriceSourceProviderEstimate, sell.Metadata["execution_price_source"])
	assert.Equal(t, FeeSourceDefaultZero, sell.Metadata["fee_source"])
	require.NotNil(t, sell.UnitPrice)
	assertPriceEqual(t, "430", *sell.UnitPrice)
}

// 11. Selling every remaining unit closes the episode with no explicit flag.
func TestSell_FullSaleAutoDetectsClosure(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "close-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("3"),
	})
	require.NoError(t, err)

	partial, err := svc.SellPosition(ctx(), "u1", "close-sell-1", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	assert.Nil(t, partial.Closed, "a partial sale does not close the episode")

	// One and the same sell action; closure is inferred from the remainder.
	full, err := svc.SellPosition(ctx(), "u1", "close-sell-2", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)
	require.NotNil(t, full.Closed, "selling the remainder closes the episode automatically")

	open, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	assert.Empty(t, open)
	closed, err := svc.ListClosedPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, closed, 1)
	assert.Equal(t, buy.Position.ID, closed[0].ID)
}

func TestSellPreview_ReportsProvenanceAndClosure(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "sp-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), ExecutionPrice: testPrice("400"),
	})
	require.NoError(t, err)
	pfBefore, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)

	in := SellInput{PositionID: buy.Position.ID, Quantity: testQuantity("2"), ExecutionPrice: testPrice("450"), Fee: testAmount("10")}
	first, err := svc.PreviewSell(ctx(), "u1", in)
	require.NoError(t, err)
	second, err := svc.PreviewSell(ctx(), "u1", in)
	require.NoError(t, err)

	assert.True(t, first.WillClosePosition)
	assert.InDelta(t, 900.0, first.GrossProceeds.Float64(), 1e-9)
	assert.InDelta(t, 890.0, first.NetProceeds.Float64(), 1e-9)
	assert.InDelta(t, 800.0, first.AllocatedBasis.Float64(), 1e-9)
	assert.InDelta(t, 90.0, first.EstimatedRealizedPnL.Float64(), 1e-9)
	assert.Equal(t, PriceSourceUserRecorded, first.ExecutionPriceSource)
	assert.Equal(t, FeeSourceUserRecorded, first.FeeSource)
	assert.Equal(t, "complete", first.CalculationStatus)

	first.EffectiveAt, second.EffectiveAt = "", ""
	assert.Equal(t, first, second)
	pfAfter, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, pfBefore.Version, pfAfter.Version, "a preview must not mutate")
	open, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, open, 1, "the position is untouched by the preview")
}

// TestSellPreview_ResolvesBySymbolLikeTheCommittedSell: the sell/preview API
// accepts either an explicit position_id or a bare symbol (SellInput/
// findSalePosition both support it), and the committed SellPosition path has
// always honored a symbol-only request. PreviewSell must resolve the same
// position the same way — a preview is presented to the user as an exact
// forecast of the commit, so a symbol-only preview must not fail merely
// because no position_id was supplied.
func TestSellPreview_ResolvesBySymbolLikeTheCommittedSell(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "sp-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), ExecutionPrice: testPrice("400"),
	})
	require.NoError(t, err)

	bySymbol, err := svc.PreviewSell(ctx(), "u1", SellInput{
		Symbol: "MSFT", Quantity: testQuantity("1"), ExecutionPrice: testPrice("450"), Fee: testAmount("10"),
	})
	require.NoError(t, err, "a symbol-only preview must resolve the open position, not report it missing")
	byID, err := svc.PreviewSell(ctx(), "u1", SellInput{
		PositionID: buy.Position.ID, Quantity: testQuantity("1"), ExecutionPrice: testPrice("450"), Fee: testAmount("10"),
	})
	require.NoError(t, err)

	assert.Equal(t, byID.PositionID, bySymbol.PositionID, "symbol and position_id must resolve to the identical position")
	assert.Equal(t, byID.EstimatedRealizedPnL, bySymbol.EstimatedRealizedPnL)

	// The committed sell accepts the identical symbol-only request and must
	// resolve to the same position the preview just forecast.
	committed, err := svc.SellPosition(ctx(), "u1", "sp-sell-by-symbol", SellInput{
		Symbol: "MSFT", Quantity: testQuantity("1"), ExecutionPrice: testPrice("450"), Fee: testAmount("10"),
	})
	require.NoError(t, err)
	require.NotNil(t, committed.Position)
	assert.Equal(t, byID.PositionID, committed.Position.ID)
}

// TestSellPreview_SymbolNotHeldReturnsPositionNotFound guards the boundary:
// a symbol with no open position must still report ErrPositionNotFound rather
// than some other error once the fallback lookup is in place.
func TestSellPreview_SymbolNotHeldReturnsPositionNotFound(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.PreviewSell(ctx(), "u1", SellInput{Symbol: "MSFT", Quantity: testQuantity("1")})
	assert.ErrorIs(t, err, ErrPositionNotFound)
}

// 12. A backdated sale is validated against the HISTORICAL quantity.
// 15. Multi-currency: a USD purchase never touches a GBP balance.
func TestBuy_MultiCurrencyNeverAutoConvertsFX(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "GBP", "5000")

	_, err := svc.BuyPosition(ctx(), "u1", "fx-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), // USD instrument
	})
	require.NoError(t, err)

	assertAmountEqual(t, "5000", cashOf(t, repo, "u1", "GBP"),
		"GBP cash is untouched — Alarvest never auto-converts currency")
	assertAmountEqual(t, "0", cashOf(t, repo, "u1", "USD"))

	funding := autoFundingActivities(activitiesOf(t, repo, "u1"))
	require.Len(t, funding, 1)
	assert.Equal(t, "USD", funding[0].Currency)
	assertAmountEqual(t, "860", funding[0].GrossAmount)
}

func TestBuy_ValidatesExecutionDetails(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.BuyPosition(ctx(), "u1", "bad-price", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"), ExecutionPrice: testPrice("-1"),
	})
	assert.ErrorIs(t, err, ErrInvalidBuyPrice)

	_, err = svc.BuyPosition(ctx(), "u1", "bad-fee", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"), Fee: testAmount("-5"),
	})
	assert.ErrorIs(t, err, ErrInvalidBuyFee)
}

// TestBuy_RejectsImplausibleLiveExecutionPrice: a buy claiming a fill wildly
// off the tracked quote (mock MSFT = 430 USD) must be rejected — otherwise a
// user could fabricate an instant, unbounded "gain" on the public
// open-position return that sits right next to the market-verified ranked
// index.
func TestBuy_RejectsImplausibleLiveExecutionPrice(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.BuyPosition(ctx(), "u1", "too-cheap", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"), ExecutionPrice: testPrice("1"),
	})
	assert.ErrorIs(t, err, ErrImplausibleExecutionPrice)

	_, err = svc.BuyPosition(ctx(), "u1", "too-expensive", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"), ExecutionPrice: testPrice("100000"),
	})
	assert.ErrorIs(t, err, ErrImplausibleExecutionPrice)
}

// TestBuy_AllowsOrdinaryLiveDeviation: normal volatility around the live
// quote must never trip the plausibility guard.
func TestBuy_AllowsOrdinaryLiveDeviation(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.BuyPosition(ctx(), "u1", "ordinary-deviation", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"), ExecutionPrice: testPrice("450"),
	})
	require.NoError(t, err, "a modest deviation from the live quote is ordinary volatility, not fabrication")
}

// TestBuy_RejectsImplausiblePriceEvenWhenRecordedInThePast: there is no
// backdating exemption. A trade recorded against an earlier clock (the
// portfolio's own test clock, not a user-supplied date) still gets the same
// plausibility check as any other trade, because every trade is priced
// against the live quote pinned at the moment it's entered.
func TestBuy_RejectsImplausiblePriceEvenWhenRecordedInThePast(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	past := time.Now().UTC().Add(-72 * time.Hour)
	withClockAt(svc, past, func() {
		_, err := svc.BuyPosition(ctx(), "u2", "far-off-price", BuyInput{
			Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"), ExecutionPrice: testPrice("10"),
		})
		assert.ErrorIs(t, err, ErrImplausibleExecutionPrice)
	})
}

// TestSell_RejectsImplausibleLiveExecutionPrice mirrors the buy-side guard
// for sells: no backdating exemption, no matter what clock the trade is
// recorded against.
func TestSell_RejectsImplausibleLiveExecutionPrice(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	bought := time.Now().UTC().Add(-2 * time.Hour)
	withClockAt(svc, bought, func() {
		_, err := svc.BuyPosition(ctx(), "u1", "buy-msft", BuyInput{
			Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("4"), ExecutionPrice: testPrice("400"),
		})
		require.NoError(t, err)
	})

	_, err := svc.SellPosition(ctx(), "u1", "sell-too-expensive", SellInput{
		Symbol: "MSFT", Quantity: testQuantity("1"), ExecutionPrice: testPrice("100000"),
	})
	assert.ErrorIs(t, err, ErrImplausibleExecutionPrice)

	sold := time.Now().UTC().Add(-30 * time.Minute)
	withClockAt(svc, sold, func() {
		_, err := svc.SellPosition(ctx(), "u1", "sell-still-checked", SellInput{
			Symbol: "MSFT", Quantity: testQuantity("1"), ExecutionPrice: testPrice("100000"),
		})
		assert.ErrorIs(t, err, ErrImplausibleExecutionPrice, "a trade recorded against an earlier clock still gets the same price check")
	})
}

// TestSetAutoFundPurchases_DisabledRejectsFundingBuy: disabling the
// portfolio-level auto-fund preference means a buy that would otherwise draw
// an implicit deposit is rejected instead — the "buys require sufficient
// cash" behavior for a user who wants it, rather than a silent deposit they
// never explicitly made.
func TestSetAutoFundPurchases_DisabledRejectsFundingBuy(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	require.NoError(t, svc.SetAutoFundPurchases(ctx(), "u1", false))

	_, err := svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), // no cash deposited
	})

	assert.ErrorIs(t, err, ErrInsufficientCash)
}

func TestSetAutoFundPurchases_EnabledIsUnaffected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	require.NoError(t, svc.SetAutoFundPurchases(ctx(), "u1", true)) // explicit, matches the default

	_, err := svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"),
	})

	require.NoError(t, err, "auto-funding must still work when explicitly left enabled")
}

// TestSetAutoFundPurchases_CreatesPortfolioForFirstTimeUser: a brand-new user
// with no portfolio yet must still be able to toggle this preference before
// ever making a deposit or buy.
func TestSetAutoFundPurchases_CreatesPortfolioForFirstTimeUser(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	require.NoError(t, svc.SetAutoFundPurchases(ctx(), "brand-new-user", false))

	pf, err := svc.GetOrCreateDefaultPortfolio(ctx(), "brand-new-user")
	require.NoError(t, err)
	assert.False(t, pf.AutoFundPurchases)
}

// TestSummary_FlagsSelfReportedExecutionPrice_WhenUserSuppliesPrice: the
// ranked index is always priced from tracked market quotes and immune to a
// fabricated execution price, but open/closed holdings P&L is built directly
// from it — so a public consumer of this data must be able to tell when it
// includes an unverifiable, user-entered price rather than a provider
// estimate.
func TestSummary_FlagsSelfReportedExecutionPrice_WhenUserSuppliesPrice(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), ExecutionPrice: testPrice("400"),
	})
	require.NoError(t, err)

	sum, err := svc.Summary(ctx(), "u1")
	require.NoError(t, err)
	assert.True(t, sum.HasSelfReportedExecutionPrice)
}

func TestSummary_DoesNotFlagSelfReportedExecutionPrice_WhenProviderEstimated(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), // no ExecutionPrice
	})
	require.NoError(t, err)

	sum, err := svc.Summary(ctx(), "u1")
	require.NoError(t, err)
	assert.False(t, sum.HasSelfReportedExecutionPrice)
}

// TestPublicWeightsSummary_MatchesSummaryOnPositionsAndCash: the leaderboard's
// cheap weights path (PublicWeightsSummary) must agree with the full Summary
// on exactly the fields a public allocation breakdown reads — open positions'
// current value/currency and cash — even though it skips the ledger scan that
// produces Summary's economic fields.
func TestPublicWeightsSummary_MatchesSummaryOnPositionsAndCash(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	seedCash(t, svc, "u1", "USD", "100000")
	seedCash(t, svc, "u1", "TRY", "100000")

	_, err := svc.BuyPosition(ctx(), "u1", "buy-aapl", BuyInput{Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), "u1", "buy-thyao", BuyInput{Symbol: "THYAO.IS", AssetType: AssetTypeStock, Quantity: testQuantity("50")})
	require.NoError(t, err)
	pp.Set("AAPL", 214.5, "USD")     // +10% since baseline
	pp.Set("THYAO.IS", 324.5, "TRY") // +10% since baseline

	full, err := svc.Summary(ctx(), "u1")
	require.NoError(t, err)
	cheap, err := svc.PublicWeightsSummary(ctx(), "u1")
	require.NoError(t, err)

	assert.InDelta(t, full.CurrentValue.Float64(), cheap.CurrentValue.Float64(), 0.01)
	assert.InDelta(t, full.TotalCashValueBase.Float64(), cheap.TotalCashValueBase.Float64(), 0.01)
	assert.Equal(t, len(full.CashBalances), len(cheap.CashBalances))
	require.Len(t, cheap.Positions, len(full.Positions))
	bySymbol := map[string]PositionSummary{}
	for _, p := range full.Positions {
		bySymbol[p.Symbol] = p
	}
	for _, p := range cheap.Positions {
		want, ok := bySymbol[p.Symbol]
		require.True(t, ok, "cheap summary listed a symbol the full summary did not: %s", p.Symbol)
		assert.InDelta(t, want.CurrentValueBase.Float64(), p.CurrentValueBase.Float64(), 0.01)
		assert.Equal(t, want.CurrentPriceCurrency, p.CurrentPriceCurrency)
	}

	// The cheap summary never touches the ledger, so its economic fields stay
	// at their zero value regardless of what Summary would report for them —
	// callers that need weights must not mistake this for "no income/fees".
	assert.Equal(t, 0.0, cheap.RealizedGainLossBase.Float64())
	assert.Empty(t, cheap.Income)
	assert.Empty(t, cheap.Fees)
}

// TestPublicWeightsSummary_ExcludesClosedPositions: weights are an allocation
// of OPEN holdings; a fully closed episode must not appear in Positions or
// contribute to CurrentValue, matching Summary's own contract.
func TestPublicWeightsSummary_ExcludesClosedPositions(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", "100000")
	_, err := svc.BuyPosition(ctx(), "u1", "buy-msft", BuyInput{Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("2"), ExecutionPrice: testPrice("400")})
	require.NoError(t, err)
	_, err = svc.SellPosition(ctx(), "u1", "sell-msft", SellInput{Symbol: "MSFT", Quantity: testQuantity("2"), ExecutionPrice: testPrice("450")})
	require.NoError(t, err)

	cheap, err := svc.PublicWeightsSummary(ctx(), "u1")
	require.NoError(t, err)
	assert.Empty(t, cheap.Positions)
	assert.Equal(t, cheap.TotalCashValueBase.Float64(), cheap.CurrentValue.Float64(), "with no open positions, current value is cash only")
}
