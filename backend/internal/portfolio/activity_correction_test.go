package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func cashUSD(t *testing.T, repo *InMemoryRepository, userID string) money.Amount {
	t.Helper()
	cash, err := repo.ListCashBalances(ctx(), userID)
	require.NoError(t, err)
	for _, c := range cash {
		if c.Currency == "USD" {
			return c.Amount
		}
	}
	return testAmount("0")
}

func TestCorrectActivity_DepositUnderRecorded_CreditsDelta(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	deposit, err := svc.DepositCash(ctx(), "u1", "dep-1", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)

	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: deposit.Activity.ID, ActualAmount: testAmount("1200"), Reason: "bank statement shows 1200",
	})
	require.NoError(t, err)
	assert.Equal(t, ActivityDeposit, res.Activity.Type)
	assertAmountEqual(t, "200", res.Activity.GrossAmount)
	assert.Equal(t, deposit.Activity.ID, res.Activity.Metadata["correction_of_activity_id"])
	assertAmountEqual(t, "1200", cashUSD(t, repo, "u1"))
}

func TestCorrectActivity_DepositOverRecorded_DebitsDelta(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	deposit, err := svc.DepositCash(ctx(), "u1", "dep-1", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)

	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: deposit.Activity.ID, ActualAmount: testAmount("700"),
	})
	require.NoError(t, err)
	assert.Equal(t, ActivityWithdrawal, res.Activity.Type)
	assertAmountEqual(t, "300", res.Activity.GrossAmount)
	assertAmountEqual(t, "700", cashUSD(t, repo, "u1"))
}

func TestCorrectActivity_WithdrawalOverRecorded_CreditsDelta(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	_, err := svc.DepositCash(ctx(), "u1", "dep-1", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	withdrawal, err := svc.WithdrawCash(ctx(), "u1", "wd-1", CashFlowInput{Currency: "USD", Amount: testAmount("400")})
	require.NoError(t, err)

	// Actual withdrawal was only 250, so 150 should be restored to cash.
	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: withdrawal.Activity.ID, ActualAmount: testAmount("250"),
	})
	require.NoError(t, err)
	assert.Equal(t, ActivityDeposit, res.Activity.Type)
	assertAmountEqual(t, "150", res.Activity.GrossAmount)
	assertAmountEqual(t, "750", cashUSD(t, repo, "u1"))
}

func TestCorrectActivity_WithdrawalUnderRecorded_DebitsDelta(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	_, err := svc.DepositCash(ctx(), "u1", "dep-1", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	withdrawal, err := svc.WithdrawCash(ctx(), "u1", "wd-1", CashFlowInput{Currency: "USD", Amount: testAmount("400")})
	require.NoError(t, err)

	// Actual withdrawal was 550, more than recorded — an additional 150 must
	// come out.
	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: withdrawal.Activity.ID, ActualAmount: testAmount("550"),
	})
	require.NoError(t, err)
	assert.Equal(t, ActivityWithdrawal, res.Activity.Type)
	assertAmountEqual(t, "150", res.Activity.GrossAmount)
	assertAmountEqual(t, "450", cashUSD(t, repo, "u1"))
}

// TestCorrectActivity_UnsupportedTypeRejected: activity types with no
// correction path of their own (or their own dedicated one, e.g. income)
// still return ErrCorrectionNotSupported. Buy/sell are correctable now — see
// the tests below.
func TestCorrectActivity_UnsupportedTypeRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	fee, err := svc.RecordFee(ctx(), "u1", "fee-1", FeeInput{
		Subtype: FeeOther, Currency: "USD", Amount: testAmount("10"),
	})
	require.NoError(t, err)

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: fee.Activity.ID, ActualAmount: testAmount("100"),
	})
	assert.ErrorIs(t, err, ErrCorrectionNotSupported)
}

// --- buy/sell correction ------------------------------------------------------

func TestCorrectActivity_BuyQuantityTypo_AdjustsQuantityAndCash(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195, cash 8050

	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: buy.Activity.ID, Reason: "typo: sold 12 not 10",
		CorrectedQuantity: testQuantity("12"), CorrectedExecutionPrice: testPrice("195"),
	})
	require.NoError(t, err)
	assert.Equal(t, buy.Activity.ID, res.Activity.Metadata["correction_of_activity_id"])
	assertQuantityEqual(t, "12", res.Position.Quantity)
	assertPriceEqual(t, "195", res.Position.AverageBuyPrice)
	assertAmountEqual(t, "7660", cashUSD(t, repo, "u1")) // 8050 - 2*195
}

func TestCorrectActivity_BuyPriceTypo_AdjustsAverageCostAndCash(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195, cash 8050

	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("10"), CorrectedExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)
	assertQuantityEqual(t, "10", res.Position.Quantity)
	assertPriceEqual(t, "200", res.Position.AverageBuyPrice)
	assertAmountEqual(t, "8000", cashUSD(t, repo, "u1")) // 8050 - (2000-1950)
}

func TestCorrectActivity_BuyFeeTypo_AdjustsCashAndMovesRankedIndex(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195, cash 8050
	before := readRankedIndex(t, svc, "u1")

	_, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("10"), CorrectedExecutionPrice: testPrice("195"),
		CorrectedFee: testAmount("5"),
	})
	require.NoError(t, err)
	assertAmountEqual(t, "8045", cashUSD(t, repo, "u1"))
	after := readRankedIndex(t, svc, "u1")
	assert.True(t, after.Cmp(before) < 0, "a newly-recorded fee must lower the ranked index")
}

func TestCorrectActivity_SellQuantityTypo_PartialToPartial(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195, cash 8050
	sell, err := svc.SellPosition(ctx(), "u1", "sell-1", SellInput{
		Symbol: "AAPL", Quantity: testQuantity("3"), ExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)
	assertAmountEqual(t, "8650", cashUSD(t, repo, "u1")) // 8050 + 600

	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: sell.Activity.ID, Reason: "typo: sold 4 not 3",
		CorrectedQuantity: testQuantity("4"), CorrectedExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)
	assertQuantityEqual(t, "6", res.Position.Quantity)
	assertAmountEqual(t, "8850", cashUSD(t, repo, "u1")) // 8650 + (800-600)
}

func TestCorrectActivity_SellQuantityTypo_ReopensFullyClosedEpisode(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195, cash 8050
	sell, err := svc.SellPosition(ctx(), "u1", "sell-1", SellInput{
		Symbol: "AAPL", Quantity: testQuantity("10"), ExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)
	require.NotNil(t, sell.Closed, "selling everything held must close the episode")
	assertAmountEqual(t, "10050", cashUSD(t, repo, "u1")) // 8050 + 2000

	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: sell.Activity.ID, Reason: "typo: sold 8 not 10",
		CorrectedQuantity: testQuantity("8"), CorrectedExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)
	require.NotNil(t, res.Position, "the episode must reopen as a partial sale")
	assert.Equal(t, sell.Closed.ID, res.Position.ID, "same durable episode identity")
	assertQuantityEqual(t, "2", res.Position.Quantity)
	assertAmountEqual(t, "9650", cashUSD(t, repo, "u1")) // 10050 - (2000-1600)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, PositionStatusOpen, positions[0].Status)
	assert.Nil(t, positions[0].ClosedAt)
}

func TestCorrectActivity_SellPriceTypo_RecomputesRealizedPnL(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195, cash 8050
	sell, err := svc.SellPosition(ctx(), "u1", "sell-1", SellInput{
		Symbol: "AAPL", Quantity: testQuantity("10"), ExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)
	assertAmountEqual(t, "50", sell.Closed.RealizedGainLossBase) // (2000-1950)

	res, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        sell.Activity.ID,
		CorrectedQuantity: testQuantity("10"), CorrectedExecutionPrice: testPrice("210"),
	})
	require.NoError(t, err)
	require.NotNil(t, res.Closed)
	assertAmountEqual(t, "150", res.Closed.RealizedGainLossBase) // (2100-1950)
	assertAmountEqual(t, "10150", cashUSD(t, repo, "u1"))        // 10050 + (2100-2000)
}

func TestCorrectActivity_BuyRejectedWhenLaterBuyExistsInEpisode(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195
	_, err := svc.BuyPosition(ctx(), "u1", "buy-2", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("5"),
	})
	require.NoError(t, err)
	cashBefore := cashUSD(t, repo, "u1")

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("12"), CorrectedExecutionPrice: testPrice("195"),
	})
	assert.ErrorIs(t, err, ErrCorrectionSupersededByLaterActivity)
	assertAmountEqual(t, cashBefore.String(), cashUSD(t, repo, "u1"))
}

func TestCorrectActivity_SellRejectedWhenLaterActivityExistsInEpisode(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195
	sell, err := svc.SellPosition(ctx(), "u1", "sell-1", SellInput{
		Symbol: "AAPL", Quantity: testQuantity("3"), ExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), "u1", "buy-2", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        sell.Activity.ID,
		CorrectedQuantity: testQuantity("4"), CorrectedExecutionPrice: testPrice("200"),
	})
	assert.ErrorIs(t, err, ErrCorrectionSupersededByLaterActivity)
}

func TestCorrectActivity_SellRejectedWhenExceedsQuantityHeldBeforeOriginal(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1") // 10 AAPL @ 195
	sell, err := svc.SellPosition(ctx(), "u1", "sell-1", SellInput{
		Symbol: "AAPL", Quantity: testQuantity("3"), ExecutionPrice: testPrice("200"),
	})
	require.NoError(t, err)

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        sell.Activity.ID, // only 10 were ever held
		CorrectedQuantity: testQuantity("11"), CorrectedExecutionPrice: testPrice("200"),
	})
	assert.ErrorIs(t, err, ErrInvalidSaleQuantity)
}

// fixedTrustedBoundary is a minimal TrustedSnapshotBoundary test double.
type fixedTrustedBoundary struct {
	at    time.Time
	found bool
}

func (f fixedTrustedBoundary) LatestTrustedSnapshotAt(context.Context, string, string, time.Time) (time.Time, bool, error) {
	return f.at, f.found, nil
}

func TestCorrectActivity_RejectedAcrossTrustedRankedBoundary(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")
	before, err := repo.ListPositionsByUser(ctx(), "u1")
	require.NoError(t, err)
	cashBefore := cashUSD(t, repo, "u1")

	svc.Coordinator().SetTrustedSnapshotBoundary(fixedTrustedBoundary{
		at: time.Now().Add(time.Hour), found: true,
	})

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("12"), CorrectedExecutionPrice: testPrice("195"),
	})
	assert.ErrorIs(t, err, ErrCorrectionRankedConflict)

	after, err := repo.ListPositionsByUser(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, after, 1)
	assertQuantityEqual(t, before[0].Quantity.String(), after[0].Quantity)
	assertAmountEqual(t, cashBefore.String(), cashUSD(t, repo, "u1"))
}

func TestCorrectActivity_InvalidCorrectedPriceRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")

	_, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("10"), CorrectedExecutionPrice: testPrice("0"),
	})
	assert.ErrorIs(t, err, ErrInvalidBuyPrice)
}

func TestCorrectActivity_BuyNoOpRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")

	_, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("10"), CorrectedExecutionPrice: testPrice("195"),
	})
	assert.ErrorIs(t, err, ErrNothingToCorrect)
}

func TestCorrectActivity_CannotCorrectBuyTwice(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")

	_, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("12"), CorrectedExecutionPrice: testPrice("195"),
	})
	require.NoError(t, err)

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-2", ActivityCorrectionInput{
		ActivityID:        buy.Activity.ID,
		CorrectedQuantity: testQuantity("13"), CorrectedExecutionPrice: testPrice("195"),
	})
	assert.ErrorIs(t, err, ErrActivityAlreadyCorrected)
}

func TestCorrectActivity_UnknownActivity(t *testing.T) {
	svc, _, _, _ := newTxTestService()

	_, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: "does-not-exist", ActualAmount: testAmount("100"),
	})
	assert.ErrorIs(t, err, ErrActivityNotFound)
}

func TestCorrectActivity_CannotCorrectTwice(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	deposit, err := svc.DepositCash(ctx(), "u1", "dep-1", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: deposit.Activity.ID, ActualAmount: testAmount("1200"),
	})
	require.NoError(t, err)

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-2", ActivityCorrectionInput{
		ActivityID: deposit.Activity.ID, ActualAmount: testAmount("900"),
	})
	assert.ErrorIs(t, err, ErrActivityAlreadyCorrected)
}

func TestCorrectActivity_SameAmountRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	deposit, err := svc.DepositCash(ctx(), "u1", "dep-1", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)

	_, err = svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: deposit.Activity.ID, ActualAmount: testAmount("1000"),
	})
	assert.ErrorIs(t, err, ErrNothingToCorrect)
}
