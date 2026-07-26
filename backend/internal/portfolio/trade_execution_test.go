package portfolio

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers Step 3: accurate user-recorded buys and sells — real
// execution details with provenance, automatic purchase funding, non-mutating
// previews, idempotency, atomicity, and the conservative historical-transaction
// policy.

func seedCash(t *testing.T, svc *Service, user, currency string, amount float64) {
	t.Helper()
	_, err := svc.DepositCash(ctx(), user, "seed-"+user+"-"+currency, CashFlowInput{
		Currency: currency, Amount: amount,
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

// sameDayBackdate returns an instant `ago` before now, clamped to the current
// UTC day so the test never accidentally crosses the trusted-boundary day line.
func sameDayBackdate(ago time.Duration) time.Time {
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	when := now.Add(-ago)
	if when.Before(startOfDay) {
		return startOfDay.Add(time.Second)
	}
	return when
}

func cashOf(t *testing.T, repo *InMemoryRepository, user, currency string) float64 {
	t.Helper()
	balances, err := repo.ListCashBalances(ctx(), user)
	require.NoError(t, err)
	for _, b := range balances {
		if b.Currency == currency {
			return b.Amount
		}
	}
	return 0
}

// 1. Buy with no cash at all → the whole purchase is funded automatically.
func TestBuy_NoCash_FullAutomaticFunding(t *testing.T) {
	svc, repo, _, _ := newTxTestService()

	res, err := svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, Fee: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Activity)

	list := activitiesOf(t, repo, "u1")
	funding := autoFundingActivities(list)
	require.Len(t, funding, 1)
	// 2 x 430 + 5 fee = 865, none of it available.
	assert.InDelta(t, 865.0, funding[0].GrossAmount, 1e-9)
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
	assert.InDelta(t, 0.0, cashOf(t, repo, "u1", "USD"), 1e-9)

	// Ranked: the funding and allocation are neutral; only the fee is a return.
	assert.Less(t, res.RankedIndexAfter, res.RankedIndexBefore,
		"the purchase fee is the only return-bearing part of the buy")
}

// 1b. A fee-free automatically-funded buy is fully ranked-neutral.
func TestBuy_NoCashNoFee_IsRankedNeutral(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	res, err := svc.BuyPosition(ctx(), "u1", "buy-neutral", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2,
	})
	require.NoError(t, err)
	assert.InDelta(t, res.RankedIndexBefore, res.RankedIndexAfter, 1e-9)
}

// 2. Buy with partial cash → existing cash is used first, the rest is funded.
func TestBuy_PartialCash_SplitsCorrectly(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", 500)

	_, err := svc.BuyPosition(ctx(), "u1", "buy-partial", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, // 860 required
	})
	require.NoError(t, err)

	funding := autoFundingActivities(activitiesOf(t, repo, "u1"))
	require.Len(t, funding, 1)
	assert.InDelta(t, 360.0, funding[0].GrossAmount, 1e-9, "860 required - 500 available")
	assert.InDelta(t, 0.0, cashOf(t, repo, "u1", "USD"), 1e-9)
}

// 3. Buy with sufficient cash → no funding activity at all.
func TestBuy_SufficientCash_NoFundingActivity(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", 2000)

	_, err := svc.BuyPosition(ctx(), "u1", "buy-funded", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2,
	})
	require.NoError(t, err)

	list := activitiesOf(t, repo, "u1")
	assert.Empty(t, autoFundingActivities(list))
	assert.InDelta(t, 1140.0, cashOf(t, repo, "u1", "USD"), 1e-9)
}

// Entered execution price and fee are honoured verbatim, with user provenance.
func TestBuy_UserEnteredExecutionDetails(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", 5000)
	when := sameDayBackdate(90 * time.Minute)

	_, err := svc.BuyPosition(ctx(), "u1", "buy-entered", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 4,
		ExecutionPrice: 400, Fee: 12, EffectiveAt: &when,
	})
	require.NoError(t, err)

	buy, ok := findActivity(activitiesOf(t, repo, "u1"), ActivityBuy)
	require.True(t, ok)
	require.NotNil(t, buy.UnitPrice)
	assert.InDelta(t, 400.0, *buy.UnitPrice, 1e-9)
	assert.InDelta(t, 1600.0, buy.GrossAmount, 1e-9)
	assert.InDelta(t, 12.0, buy.FeeAmount, 1e-9)
	assert.Equal(t, PriceSourceUserRecorded, buy.Metadata["execution_price_source"])
	assert.Equal(t, FeeSourceUserRecorded, buy.Metadata["fee_source"])
	assert.True(t, buy.OccurredAt.Equal(when), "effective_at is the real-world trade time")
	assert.True(t, buy.CreatedAt.After(buy.OccurredAt), "recorded_at is distinct from effective_at")

	// Basis includes price AND fee: (1600 + 12) / 4 = 403.
	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.InDelta(t, 403.0, positions[0].AverageBuyPrice, 1e-9)
	assert.InDelta(t, 3388.0, cashOf(t, repo, "u1", "USD"), 1e-9)
}

// 7. Weighted-average basis across two buys (fees included).
func TestBuy_WeightedAverageBasisAcrossTwoBuys(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", 10000)

	_, err := svc.BuyPosition(ctx(), "u1", "wa-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, ExecutionPrice: 400,
	})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), "u1", "wa-2", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 3, ExecutionPrice: 500, Fee: 10,
	})
	require.NoError(t, err)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.InDelta(t, 5.0, positions[0].Quantity, 1e-9)
	// (2*400 + 3*500 + 10) / 5 = 462
	assert.InDelta(t, 462.0, positions[0].AverageBuyPrice, 1e-9)
}

// 8. A rebuy after a full sale opens a NEW episode via the new buy path.
func TestBuy_RebuyAfterCloseCreatesNewEpisode(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "USD", 10000)

	first, err := svc.BuyPosition(ctx(), "u1", "ep-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2,
	})
	require.NoError(t, err)
	firstEpisode := first.Position.ID

	_, err = svc.SellPosition(ctx(), "u1", "ep-sell", SellInput{
		PositionID: firstEpisode, Quantity: 2,
	})
	require.NoError(t, err)

	second, err := svc.BuyPosition(ctx(), "u1", "ep-2", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 1,
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
	seedCash(t, svc, "u1", "USD", 500)

	before := activitiesOf(t, repo, "u1")
	beforeCash := cashOf(t, repo, "u1", "USD")
	beforePositions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	beforePortfolio, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)

	in := BuyInput{Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, Fee: 5}
	first, err := svc.PreviewBuy(ctx(), "u1", in)
	require.NoError(t, err)
	second, err := svc.PreviewBuy(ctx(), "u1", in)
	require.NoError(t, err)

	assert.InDelta(t, 860.0, first.GrossPurchaseAmount, 1e-9)
	assert.InDelta(t, 5.0, first.Fee, 1e-9)
	assert.InDelta(t, 865.0, first.TotalCashRequired, 1e-9)
	assert.InDelta(t, 500.0, first.AvailableCash, 1e-9)
	assert.InDelta(t, 500.0, first.CashUsed, 1e-9)
	assert.InDelta(t, 365.0, first.AutomaticFunding, 1e-9)
	assert.InDelta(t, 0.0, first.RemainingCash, 1e-9)
	assert.True(t, first.CreatesNewEpisode)
	assert.Equal(t, PriceSourceProviderEstimate, first.ExecutionPriceSource)
	assert.Equal(t, FeeSourceUserRecorded, first.FeeSource)
	assert.Equal(t, "complete", first.CalculationStatus)
	assert.NotEmpty(t, first.EffectiveAt)

	// Repeating the preview changed nothing anywhere.
	first.EffectiveAt, second.EffectiveAt = "", ""
	assert.Equal(t, first, second)
	assert.Equal(t, len(before), len(activitiesOf(t, repo, "u1")))
	assert.InDelta(t, beforeCash, cashOf(t, repo, "u1", "USD"), 1e-9)
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
	seedCash(t, svc, "u1", "USD", 10000)
	res, err := svc.BuyPosition(ctx(), "u1", "pv-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, ExecutionPrice: 400,
	})
	require.NoError(t, err)

	preview, err := svc.PreviewBuy(ctx(), "u1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, ExecutionPrice: 500,
	})
	require.NoError(t, err)
	assert.False(t, preview.CreatesNewEpisode)
	assert.Equal(t, res.Position.ID, preview.PositionEpisodeID)
	assert.InDelta(t, 4.0, preview.ResultingQuantity, 1e-9)
	assert.InDelta(t, 450.0, preview.ResultingAverageCost, 1e-9)
	assert.Equal(t, PriceSourceUserRecorded, preview.ExecutionPriceSource)
}

// 5. Idempotency: the same key applied twice produces exactly one effect.
func TestBuy_IdempotentRetryHasSingleEffect(t *testing.T) {
	svc, repo, _, _ := newTxTestService()

	first, err := svc.BuyPosition(ctx(), "u1", "idem-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, Fee: 5,
	})
	require.NoError(t, err)
	retry, err := svc.BuyPosition(ctx(), "u1", "idem-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, Fee: 5,
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
	assert.InDelta(t, 2.0, positions[0].Quantity, 1e-9)
}

func TestSell_IdempotentRetryHasSingleEffect(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "sell-idem-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 4,
	})
	require.NoError(t, err)

	first, err := svc.SellPosition(ctx(), "u1", "idem-sell", SellInput{
		PositionID: buy.Position.ID, Quantity: 2, Fee: 3,
	})
	require.NoError(t, err)
	retry, err := svc.SellPosition(ctx(), "u1", "idem-sell", SellInput{
		PositionID: buy.Position.ID, Quantity: 2, Fee: 3,
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
	assert.InDelta(t, 2.0, positions[0].Quantity, 1e-9)
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
			seedCash(t, svc, "u1", "USD", 100)
			cashBefore := cashOf(t, repo, "u1", "USD")
			activitiesBefore := len(activitiesOf(t, repo, "u1"))
			pfBefore, err := repo.GetPortfolioByUser(ctx(), "u1")
			require.NoError(t, err)

			repo.SetFaults(faults)
			_, err = svc.BuyPosition(ctx(), "u1", "atomic-buy", BuyInput{
				Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, Fee: 5,
			})
			require.Error(t, err)
			repo.SetFaults(Faults{})

			positions, err := svc.ListPositions(ctx(), "u1")
			require.NoError(t, err)
			assert.Empty(t, positions, "no position committed")
			assert.InDelta(t, cashBefore, cashOf(t, repo, "u1", "USD"), 1e-9, "no cash committed")
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
	bought := sameDayBackdate(2 * time.Hour)
	_, err := svc.BuyPosition(ctx(), "u1", "s-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 4, ExecutionPrice: 400,
		EffectiveAt: &bought,
	})
	require.NoError(t, err)
	cashAfterBuy := cashOf(t, repo, "u1", "USD")
	when := sameDayBackdate(30 * time.Minute)

	_, err = svc.SellPosition(ctx(), "u1", "s-sell", SellInput{
		Symbol: "MSFT", Quantity: 2, ExecutionPrice: 450, Fee: 7, EffectiveAt: &when,
	})
	require.NoError(t, err)

	sell, ok := findActivity(activitiesOf(t, repo, "u1"), ActivitySell)
	require.True(t, ok)
	assert.Equal(t, PriceSourceUserRecorded, sell.Metadata["execution_price_source"])
	assert.Equal(t, FeeSourceUserRecorded, sell.Metadata["fee_source"])
	assert.True(t, sell.OccurredAt.Equal(when))
	// gross = 2*450 = 900; net = 900 - 7 = 893; basis = 2*400 = 800; P&L = 93.
	assert.InDelta(t, 900.0, sell.GrossAmount, 1e-9)
	assert.InDelta(t, 893.0, sell.NetAmount, 1e-9)
	require.NotNil(t, sell.RealizedGainLossBase)
	assert.InDelta(t, 93.0, *sell.RealizedGainLossBase, 1e-9)
	assert.InDelta(t, cashAfterBuy+893.0, cashOf(t, repo, "u1", "USD"), 1e-9)
}

// 10. Sell with no entered price falls back to the provider estimate.
func TestSell_DefaultPriceIsLabelledProviderEstimate(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "est-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2,
	})
	require.NoError(t, err)
	_, err = svc.SellPosition(ctx(), "u1", "est-sell", SellInput{
		PositionID: buy.Position.ID, Quantity: 1,
	})
	require.NoError(t, err)

	sell, ok := findActivity(activitiesOf(t, repo, "u1"), ActivitySell)
	require.True(t, ok)
	assert.Equal(t, PriceSourceProviderEstimate, sell.Metadata["execution_price_source"])
	assert.Equal(t, FeeSourceDefaultZero, sell.Metadata["fee_source"])
	require.NotNil(t, sell.UnitPrice)
	assert.InDelta(t, 430.0, *sell.UnitPrice, 1e-9)
}

// 11. Selling every remaining unit closes the episode with no explicit flag.
func TestSell_FullSaleAutoDetectsClosure(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "close-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 3,
	})
	require.NoError(t, err)

	partial, err := svc.SellPosition(ctx(), "u1", "close-sell-1", SellInput{
		PositionID: buy.Position.ID, Quantity: 1,
	})
	require.NoError(t, err)
	assert.Nil(t, partial.Closed, "a partial sale does not close the episode")

	// One and the same sell action; closure is inferred from the remainder.
	full, err := svc.SellPosition(ctx(), "u1", "close-sell-2", SellInput{
		PositionID: buy.Position.ID, Quantity: 2,
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
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, ExecutionPrice: 400,
	})
	require.NoError(t, err)
	pfBefore, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)

	in := SellInput{PositionID: buy.Position.ID, Quantity: 2, ExecutionPrice: 450, Fee: 10}
	first, err := svc.PreviewSell(ctx(), "u1", in)
	require.NoError(t, err)
	second, err := svc.PreviewSell(ctx(), "u1", in)
	require.NoError(t, err)

	assert.True(t, first.WillClosePosition)
	assert.InDelta(t, 900.0, first.GrossProceeds, 1e-9)
	assert.InDelta(t, 890.0, first.NetProceeds, 1e-9)
	assert.InDelta(t, 800.0, first.AllocatedBasis, 1e-9)
	assert.InDelta(t, 90.0, first.EstimatedRealizedPnL, 1e-9)
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

// 12. A backdated sale is validated against the HISTORICAL quantity.
func TestHistorical_SellRejectedWhenHistoricalQuantityInsufficient(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	base := time.Now().UTC().Add(-10 * 24 * time.Hour)

	// 5 units bought 10 days ago...
	early := base
	_, err := svc.BuyPosition(ctx(), "u1", "hist-buy-1", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 5, EffectiveAt: &early,
	})
	require.NoError(t, err)
	// ...and 20 more bought today.
	_, err = svc.BuyPosition(ctx(), "u1", "hist-buy-2", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 20,
	})
	require.NoError(t, err)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.InDelta(t, 25.0, positions[0].Quantity, 1e-9)

	// Backdated to a day when only 5 units were held: selling 10 must fail even
	// though 25 are held right now.
	backdated := base.Add(24 * time.Hour)
	_, err = svc.SellPosition(ctx(), "u1", "hist-sell-bad", SellInput{
		PositionID: positions[0].ID, Quantity: 10, EffectiveAt: &backdated,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHistoricalQuantityInsufficient)
	assert.Contains(t, err.Error(), "held only 5")

	// Nothing was mutated by the rejection.
	after, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, 25.0, after[0].Quantity, 1e-9)
	sells := 0
	for _, a := range activitiesOf(t, repo, "u1") {
		if a.Type == ActivitySell {
			sells++
		}
	}
	assert.Zero(t, sells)
}

// 13. A historical buy that cannot change any captured ranked snapshot state is
// allowed and applied normally.
func TestHistorical_BuyBeforeTrustedBoundaryIsAllowed(t *testing.T) {
	svc, repo, _, _ := newTxTestService()

	// Establish a trusted ranked checkpoint by mutating now.
	_, err := svc.BuyPosition(ctx(), "u1", "boundary-setup", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 1,
	})
	require.NoError(t, err)

	// A backdated purchase of a DIFFERENT instrument with no captured ledger
	// history at that point: allowed, applied normally, snapshots not rebuilt.
	when := time.Now().UTC().Add(-30 * 24 * time.Hour)
	res, err := svc.BuyPosition(ctx(), "u1", "hist-allowed", BuyInput{
		Symbol: "NVDA", AssetType: AssetTypeStock, Quantity: 2, EffectiveAt: &when,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Position)

	found := false
	for _, a := range activitiesOf(t, repo, "u1") {
		if a.Type == ActivityBuy && a.Symbol == "NVDA" {
			found = true
			assert.True(t, a.OccurredAt.Equal(when))
			assert.True(t, a.CreatedAt.After(a.OccurredAt))
		}
	}
	assert.True(t, found)
}

// 14. A historical transaction that would invalidate trusted ranked history is
// rejected with a clear error and mutates nothing.
func TestHistorical_TransactionCorruptingRankedHistoryIsRejected(t *testing.T) {
	svc, repo, _, _ := newTxTestService()

	_, err := svc.BuyPosition(ctx(), "u1", "corrupt-setup", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 4,
	})
	require.NoError(t, err)

	pfBefore, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)
	activitiesBefore := len(activitiesOf(t, repo, "u1"))

	// A backdated BUY inserted before an instrument's existing ledger history
	// would retroactively rewrite the episode timeline a trusted snapshot
	// already captured.
	when := time.Now().UTC().Add(-7 * 24 * time.Hour)
	_, err = svc.BuyPosition(ctx(), "u1", "corrupt-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 1, EffectiveAt: &when,
	})
	assert.ErrorIs(t, err, ErrHistoricalRankedConflict)
	assert.Contains(t, err.Error(), "requires reconciliation")

	// No state changed at all.
	pfAfter, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, pfBefore.Version, pfAfter.Version)
	assert.Equal(t, activitiesBefore, len(activitiesOf(t, repo, "u1")))
	after, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.InDelta(t, 4.0, after[0].Quantity, 1e-9)
}

// 14b. A backdated SALE before the trusted boundary is always refused, even when
// the historical quantity was ample — Alarvest never rebuilds ranked history.
func TestHistorical_BackdatedSaleBeforeBoundaryIsRejected(t *testing.T) {
	svc, repo, _, _ := newTxTestService()

	bought := time.Now().UTC().Add(-10 * 24 * time.Hour)
	buy, err := svc.BuyPosition(ctx(), "u1", "hist-sale-buy", BuyInput{
		Symbol: "NVDA", AssetType: AssetTypeStock, Quantity: 10, EffectiveAt: &bought,
	})
	require.NoError(t, err, "a backdated buy of an untracked instrument is allowed")

	pfBefore, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)

	// 5 days ago the position genuinely held 10 units, so this is NOT a quantity
	// problem — it is refused purely because it predates trusted ranked history.
	when := time.Now().UTC().Add(-5 * 24 * time.Hour)
	_, err = svc.SellPosition(ctx(), "u1", "hist-sale", SellInput{
		PositionID: buy.Position.ID, Quantity: 1, EffectiveAt: &when,
	})
	assert.ErrorIs(t, err, ErrHistoricalRankedConflict)

	pfAfter, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)
	assert.Equal(t, pfBefore.Version, pfAfter.Version)
	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, 10.0, positions[0].Quantity, 1e-9)
}

// 15. Multi-currency: a USD purchase never touches a GBP balance.
func TestBuy_MultiCurrencyNeverAutoConvertsFX(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	seedCash(t, svc, "u1", "GBP", 5000)

	_, err := svc.BuyPosition(ctx(), "u1", "fx-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, // USD instrument
	})
	require.NoError(t, err)

	assert.InDelta(t, 5000.0, cashOf(t, repo, "u1", "GBP"), 1e-9,
		"GBP cash is untouched — Alarvest never auto-converts currency")
	assert.InDelta(t, 0.0, cashOf(t, repo, "u1", "USD"), 1e-9)

	funding := autoFundingActivities(activitiesOf(t, repo, "u1"))
	require.Len(t, funding, 1)
	assert.Equal(t, "USD", funding[0].Currency)
	assert.InDelta(t, 860.0, funding[0].GrossAmount, 1e-9)
}

func TestBuy_ValidatesExecutionDetails(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	_, err := svc.BuyPosition(ctx(), "u1", "bad-price", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 1, ExecutionPrice: -1,
	})
	assert.ErrorIs(t, err, ErrInvalidBuyPrice)

	_, err = svc.BuyPosition(ctx(), "u1", "bad-fee", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 1, Fee: -5,
	})
	assert.ErrorIs(t, err, ErrInvalidBuyFee)
}
