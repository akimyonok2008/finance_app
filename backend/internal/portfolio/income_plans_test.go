package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests exercise the new provider-driven income plans directly on the
// coordinator: separated gross/withholding/net, return of capital (basis
// reduction), and stock dividends (neutral quantity transformation).

func TestIncome_WithholdingReducesNetCash(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	_, err := svc.RecordIncome(ctx(), "u1", "div-wh", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD",
		Amount: 100, Withholding: 15,
	})
	require.NoError(t, err)

	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usd float64
	for _, c := range cash {
		if c.Currency == "USD" {
			usd = c.Amount
		}
	}
	// Seed cash = 10000 deposit - 1950 (10 AAPL @ 195) = 8050. Net dividend =
	// 100 gross - 15 withholding = 85.
	assert.InDelta(t, 8050+85, usd, 0.001)
}

func TestReturnOfCapital_ReducesBasisAndIsReturnBearing(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	res, err := svc.RecordIncome(ctx(), "u1", "roc-1", IncomeInput{
		Subtype: IncomeReturnOfCapitalSub, Symbol: "AAPL", Currency: "USD", Amount: 50,
	})
	require.NoError(t, err)
	// Return-bearing: the cash credit raises the index.
	assert.Greater(t, res.RankedIndexAfter, res.RankedIndexBefore)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	// 10 shares, baseline 195 → total basis 1950. RoC 50 reduces basis to 1900 →
	// per-share 190.
	assert.InDelta(t, 190.0, positions[0].AverageBuyPrice, 0.01)

	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usd float64
	for _, c := range cash {
		if c.Currency == "USD" {
			usd = c.Amount
		}
	}
	assert.InDelta(t, 8050+50, usd, 0.001)
}

func TestReturnOfCapital_BasisNeverBelowZero(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	// A RoC larger than the entire basis (1950). Basis floors at zero; excess is
	// recorded, not silently dropped.
	_, err := svc.RecordIncome(ctx(), "u1", "roc-big", IncomeInput{
		Subtype: IncomeReturnOfCapitalSub, Symbol: "AAPL", Currency: "USD", Amount: 3000,
	})
	require.NoError(t, err)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.InDelta(t, 0.0, positions[0].AverageBuyPrice, 0.001) // basis exhausted, not negative

	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	var usd float64
	for _, c := range cash {
		if c.Currency == "USD" {
			usd = c.Amount
		}
	}
	assert.InDelta(t, 8050+3000, usd, 0.001) // full cash credited
}

func TestStockDividend_PreservesBasisAndValueNeutral(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	res, err := svc.RecordIncome(ctx(), "u1", "sd-1", IncomeInput{
		Subtype: IncomeStockDividendSub, Symbol: "AAPL", Currency: "USD",
		StockRatioNum: 1, StockRatioDen: 10, // 10% stock dividend
	})
	require.NoError(t, err)
	// Neutral: the index is unchanged (no artificial ranked jump).
	assert.InDelta(t, res.RankedIndexBefore, res.RankedIndexAfter, res.RankedIndexBefore*0.0005)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	// 10 → 11 shares; per-share basis 195 → 177.27; total basis preserved at 1950.
	assert.InDelta(t, 11.0, positions[0].Quantity, 0.001)
	assert.InDelta(t, 1950.0, positions[0].Quantity*positions[0].AverageBuyPrice, 0.01)
}

func TestIncome_RejectsWithholdingExceedingGross(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	_, err := svc.RecordIncome(ctx(), "u1", "bad-1", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD",
		Amount: 100, Withholding: 150, // deductions exceed gross
	})
	assert.ErrorIs(t, err, ErrInvalidIncomeAmount)
}
