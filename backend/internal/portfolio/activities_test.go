package portfolio

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
)

// readRankedIndex computes the live ranked index for a user from committed
// state, valuing positions and cash at current mock prices.
func readRankedIndex(t *testing.T, svc *Service, userID string) money.IndexValue {
	t.Helper()
	sr, ok := svc.repo.(performance.StateReader)
	require.True(t, ok)
	perf := performance.NewService(sr)
	perf.SetValuator(svc)
	rp, err := perf.CurrentRankedPerformance(context.Background(), userID)
	require.NoError(t, err)
	return rp.RankedIndex
}

// fundedPortfolio deposits cash and buys a position so the portfolio is active
// with a known value. Returns the buy result.
func fundedPortfolio(t *testing.T, svc *Service, userID string) MutationResult {
	t.Helper()
	_, err := svc.DepositCash(ctx(), userID, "seed-deposit", CashFlowInput{Currency: "USD", Amount: testAmount("10000")})
	require.NoError(t, err)
	buy, err := svc.BuyPosition(ctx(), userID, "seed-buy", BuyInput{Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("10")})
	require.NoError(t, err)
	require.NotNil(t, buy.Position)
	return buy
}

// --- income -------------------------------------------------------------------

func TestCashDividend_IncreasesCashAndRankedReturn(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	before, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)

	res, err := svc.RecordIncome(ctx(), "u1", "div-1", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: testAmount("100"),
	})
	require.NoError(t, err)
	// Return-bearing: the index must RISE (not be neutralized).
	assert.Greater(t, res.RankedIndexAfter.Cmp(res.RankedIndexBefore), 0)

	after, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	assert.Greater(t, after.RankedIndex.Cmp(before.RankedIndex), 0)

	// Portfolio value was 10000 (8050 position + 1950 cash). A 100 dividend is
	// +1% → index ~ +1%.
	assertIndexValuesEqual(t, before.RankedIndex.MulRatio(testRatio("1.01")), after.RankedIndex)

	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	usd := testAmount("0")
	for _, c := range cash {
		if c.Currency == "USD" {
			usd = c.Amount
		}
	}
	assertAmountEqual(t, "8150", usd) // 8050 leftover (10000-1950) + 100 dividend
}

func TestDividend_DoesNotResetCheckpoint(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordIncome(ctx(), "u1", "div-1", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: testAmount("100"),
	})
	require.NoError(t, err)

	pf, err := repo.GetPortfolioByUser(ctx(), "u1")
	require.NoError(t, err)
	state, err := repo.GetByPortfolio(ctx(), pf.ID)
	require.NoError(t, err)
	// A return-bearing event must NOT re-baseline: the segment start stays 10000.
	require.NotNil(t, state.SegmentStartValueBase)
	assertAmountEqual(t, "10000", *state.SegmentStartValueBase)
}

func TestETFDistributionAndInterest_AreReturnBearing(t *testing.T) {
	for _, sub := range []IncomeSubtype{IncomeETFDistribution, IncomeInterest} {
		svc, _, _, _ := newTxTestService()
		fundedPortfolio(t, svc, "u1")
		in := IncomeInput{Subtype: sub, Symbol: "AAPL", Currency: "USD", Amount: testAmount("50")}
		if sub == IncomeInterest {
			in.Symbol = "" // interest may be recorded without a symbol
		}
		res, err := svc.RecordIncome(ctx(), "u1", "inc-"+string(sub), in)
		require.NoError(t, err)
		assert.Greaterf(t, res.RankedIndexAfter.Cmp(res.RankedIndexBefore), 0, "%s must raise the index", sub)
	}
}

func TestForeignCurrencyDividend_StaysInDeclaredCurrency(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordIncome(ctx(), "u1", "eur-div", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "EUR", Amount: testAmount("30"),
	})
	require.NoError(t, err)
	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	eur := testAmount("0")
	for _, c := range cash {
		if c.Currency == "EUR" {
			eur = c.Amount
		}
	}
	assertAmountEqual(t, "30", eur) // not converted to base
}

func TestReinvestedDividend_IncomeOncePlusNeutralBuy(t *testing.T) {
	svc, repo, _, quotes := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")
	startQty := buy.Position.Quantity

	before := readRankedIndex(t, svc, "u1")
	quotes.Set("AAPL", 200, "USD") // reinvest at 200
	res, err := svc.RecordIncome(ctx(), "u1", "reinv-1", IncomeInput{
		Subtype: IncomeReinvestedDiv, Symbol: "AAPL", AssetType: AssetTypeStock,
		Currency: "USD", Amount: testAmount("400"),
	})
	require.NoError(t, err)
	require.NotNil(t, res.Position)

	// Quantity increases by income/price = 400/200 = 2 shares.
	assert.True(t, startQty.Add(testQuantity("2")).EqualQuantity(res.Position.Quantity))

	// The income is reflected once: index rises, but only by the dividend amount,
	// not by dividend + a spurious buy return.
	after := readRankedIndex(t, svc, "u1")
	assert.Greater(t, after.Cmp(before), 0)

	// Two grouped activities were recorded: the income leg and the buy leg.
	acts, err := repo.ListActivities(ctx(), "u1", 50)
	require.NoError(t, err)
	var incomeLegs, buyLegs int
	for _, a := range acts {
		if a.Type == ActivityReinvestedDividend {
			incomeLegs++
		}
		if a.Type == ActivityBuy {
			buyLegs++
		}
	}
	assert.Equal(t, 1, incomeLegs)
	assert.GreaterOrEqual(t, buyLegs, 2) // seed buy + reinvestment buy leg
}

func TestDividend_DuplicatePrevented(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	first, err := svc.RecordIncome(ctx(), "u1", "same-div", IncomeInput{Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: testAmount("100")})
	require.NoError(t, err)
	retry, err := svc.RecordIncome(ctx(), "u1", "same-div", IncomeInput{Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: testAmount("100")})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
	assert.Equal(t, first.PortfolioVersion, retry.PortfolioVersion)
}

func TestIncome_RejectsInvalidAmountAndType(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordIncome(ctx(), "u1", "bad-amt", IncomeInput{Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: testAmount("0")})
	require.ErrorIs(t, err, ErrInvalidIncomeAmount)
	_, err = svc.RecordIncome(ctx(), "u1", "bad-cur", IncomeInput{Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "JPY", Amount: testAmount("10")})
	require.ErrorIs(t, err, ErrUnsupportedCurrency)
}

// --- fees ---------------------------------------------------------------------

func TestManagementFee_ReducesCashAndRankedReturn(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	before := readRankedIndex(t, svc, "u1")
	res, err := svc.RecordFee(ctx(), "u1", "fee-1", FeeInput{Subtype: FeeManagement, Currency: "USD", Amount: testAmount("100")})
	require.NoError(t, err)
	assert.Less(t, res.RankedIndexAfter.Cmp(res.RankedIndexBefore), 0)
	after := readRankedIndex(t, svc, "u1")
	assert.Less(t, after.Cmp(before), 0)
}

func TestFee_InsufficientCashRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1") // 1950 USD cash left
	_, err := svc.RecordFee(ctx(), "u1", "big-fee", FeeInput{Subtype: FeeManagement, Currency: "USD", Amount: testAmount("99999")})
	require.ErrorIs(t, err, ErrInsufficientCashForFee)
}

func TestFee_NeverCreatesNegativeCash(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordFee(ctx(), "u1", "f", FeeInput{Subtype: FeeCustody, Currency: "USD", Amount: testAmount("1950")})
	require.NoError(t, err)
	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	for _, c := range cash {
		assert.GreaterOrEqual(t, c.Amount.Cmp(testAmount("0")), 0)
	}
}

func TestFee_DuplicatePrevented(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordFee(ctx(), "u1", "same-fee", FeeInput{Subtype: FeeOther, Currency: "USD", Amount: testAmount("25")})
	require.NoError(t, err)
	retry, err := svc.RecordFee(ctx(), "u1", "same-fee", FeeInput{Subtype: FeeOther, Currency: "USD", Amount: testAmount("25")})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
}

// --- splits -------------------------------------------------------------------

func TestStockSplit_PreservesValueBasisAndIndex(t *testing.T) {
	svc, _, _, quotes := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")
	beforeIdx := readRankedIndex(t, svc, "u1")
	beforeBasis := buy.Position.Quantity.MulPrice(buy.Position.AverageBuyPrice)

	res, err := svc.RecordCorporateAction(ctx(), "u1", "split-1", CorpActionInput{
		Subtype: CorpStockSplit, Symbol: "AAPL", RatioNumerator: 2, RatioDenominator: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Position)

	assert.True(t, buy.Position.Quantity.MulRatio(testRatio("2")).EqualQuantity(res.Position.Quantity))
	expectedPrice, err := buy.Position.AverageBuyPrice.DivRatio(testRatio("2"), 18)
	require.NoError(t, err)
	assert.Equal(t, 0, expectedPrice.Cmp(res.Position.AverageBuyPrice))
	// Total basis unchanged.
	assertAmountValuesEqual(t, beforeBasis, res.Position.Quantity.MulPrice(res.Position.AverageBuyPrice))
	// Ranked index unchanged AT THE ACTION (value-invariant transformation).
	assertIndexValuesEqual(t, res.RankedIndexBefore, res.RankedIndexAfter)
	// A real split-adjusted feed halves the quote; simulate that, then confirm the
	// index is still unchanged (no phantom gain from the doubled share count).
	quotes.Set("AAPL", expectedPrice.Float64(), "USD")
	afterIdx := readRankedIndex(t, svc, "u1")
	assertIndexValuesEqual(t, beforeIdx, afterIdx)
}

func TestReverseSplit_PreservesValueAndIndex(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")
	beforeBasis := buy.Position.Quantity.MulPrice(buy.Position.AverageBuyPrice)
	res, err := svc.RecordCorporateAction(ctx(), "u1", "rsplit", CorpActionInput{
		Subtype: CorpReverseSplit, Symbol: "AAPL", RatioNumerator: 1, RatioDenominator: 10,
	})
	require.NoError(t, err)
	expectedQuantity := buy.Position.Quantity.MulRatio(testRatio("0.1"))
	assert.True(t, expectedQuantity.EqualQuantity(res.Position.Quantity))
	assertAmountValuesEqual(t, beforeBasis, res.Position.Quantity.MulPrice(res.Position.AverageBuyPrice))
	assertIndexValuesEqual(t, res.RankedIndexBefore, res.RankedIndexAfter)
}

func TestSplit_InvalidRatioRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordCorporateAction(ctx(), "u1", "bad", CorpActionInput{
		Subtype: CorpStockSplit, Symbol: "AAPL", RatioNumerator: 0, RatioDenominator: 1,
	})
	require.ErrorIs(t, err, ErrInvalidSplitRatio)
}

// --- symbol change ------------------------------------------------------------

func TestSymbolChange_PreservesPositionAndIndex(t *testing.T) {
	svc, repo, _, quotes := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1")
	quotes.Set("APLX", 195, "USD") // new ticker priceable
	res, err := svc.RecordCorporateAction(ctx(), "u1", "sym-1", CorpActionInput{
		Subtype: CorpSymbolChange, Symbol: "AAPL", NewSymbol: "APLX",
	})
	require.NoError(t, err)
	require.NotNil(t, res.Position)
	assert.Equal(t, "APLX", res.Position.Symbol)
	assert.True(t, buy.Position.Quantity.EqualQuantity(res.Position.Quantity))
	assert.Equal(t, 0, buy.Position.AverageBuyPrice.Cmp(res.Position.AverageBuyPrice))
	assertIndexValuesEqual(t, res.RankedIndexBefore, res.RankedIndexAfter)

	// The immutable history retains the old symbol.
	acts, err := repo.ListActivities(ctx(), "u1", 50)
	require.NoError(t, err)
	var found bool
	for _, a := range acts {
		if a.Type == ActivitySymbolChange {
			found = true
		}
	}
	assert.True(t, found)
}

func TestSymbolChange_DuplicateRejected(t *testing.T) {
	svc, _, _, quotes := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	quotes.Set("APLX", 195, "USD")
	_, err := svc.RecordCorporateAction(ctx(), "u1", "sc", CorpActionInput{Subtype: CorpSymbolChange, Symbol: "AAPL", NewSymbol: "APLX"})
	require.NoError(t, err)
	retry, err := svc.RecordCorporateAction(ctx(), "u1", "sc", CorpActionInput{Subtype: CorpSymbolChange, Symbol: "AAPL", NewSymbol: "APLX"})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
}

// --- write-off ----------------------------------------------------------------

func TestWriteOff_ProducesNegativeReturnAndClosesPosition(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	before := readRankedIndex(t, svc, "u1")
	res, err := svc.RecordCorporateAction(ctx(), "u1", "wo-1", CorpActionInput{
		Subtype: CorpWriteOff, Symbol: "AAPL",
	})
	require.NoError(t, err)
	assert.Less(t, res.RankedIndexAfter.Cmp(res.RankedIndexBefore), 0)
	after := readRankedIndex(t, svc, "u1")
	assert.Less(t, after.Cmp(before), 0)

	// Position is closed; cash and any other holdings remain intact.
	open, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	for _, p := range open {
		assert.NotEqual(t, "AAPL", p.Symbol, "written-off position must not remain open")
	}
	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	assert.NotEmpty(t, cash, "cash must remain after a write-off")
}

func TestWriteOff_DuplicateRejected(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordCorporateAction(ctx(), "u1", "wo", CorpActionInput{Subtype: CorpWriteOff, Symbol: "AAPL"})
	require.NoError(t, err)
	retry, err := svc.RecordCorporateAction(ctx(), "u1", "wo", CorpActionInput{Subtype: CorpWriteOff, Symbol: "AAPL"})
	require.NoError(t, err)
	assert.True(t, retry.Duplicate)
}

// --- privacy ------------------------------------------------------------------

func TestActivityMetadata_RecordsEffectAndProvenance(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")
	_, err := svc.RecordFee(ctx(), "u1", "fee-meta", FeeInput{Subtype: FeeManagement, Currency: "USD", Amount: testAmount("10")})
	require.NoError(t, err)
	acts, err := repo.ListActivities(ctx(), "u1", 50)
	require.NoError(t, err)
	var fee *Activity
	for i := range acts {
		if acts[i].Type == ActivityManagementFee {
			fee = &acts[i]
		}
	}
	require.NotNil(t, fee)
	assert.Equal(t, string(PerformanceEffectReturn), fee.Metadata["performance_effect"])
	assert.Equal(t, string(ProvenanceUserReported), fee.Metadata["provenance"])
	// Sanity: metadata is JSON-round-trippable and carries no private totals.
	raw, err := json.Marshal(fee.Metadata)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "segment_start")
}
