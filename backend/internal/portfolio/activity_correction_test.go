package portfolio

import (
	"testing"

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

func TestCorrectActivity_BuySellRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")

	_, err := svc.CorrectActivity(ctx(), "u1", "correct-1", ActivityCorrectionInput{
		ActivityID: buy.Activity.ID, ActualAmount: testAmount("100"),
	})
	assert.ErrorIs(t, err, ErrCorrectionNotSupported)
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
