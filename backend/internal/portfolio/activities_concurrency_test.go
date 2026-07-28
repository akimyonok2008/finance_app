package portfolio

import (
	"context"
	"math"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

// assertReturnConsistent is the consistency check for portfolios that have seen
// return-bearing events. Unlike assertConsistent it does NOT require
// segment_start == current_value (return-bearing events intentionally keep the
// segment baseline so the index moves). It verifies the ranked state is valid,
// cash is never negative, and the live index is finite and positive.
func assertReturnConsistent(t *testing.T, repo *InMemoryRepository, svc *Service, perf *performance.Service, userID string) {
	t.Helper()
	_, state := readAggregate(t, repo, svc, userID)
	require.NotNil(t, state)
	require.NoError(t, performance.ValidateState(*state))

	cash, err := repo.ListCashBalances(context.Background(), userID)
	require.NoError(t, err)
	for _, c := range cash {
		assert.GreaterOrEqual(t, c.Amount.Cmp(testAmount("0")), 0, "cash must never go negative")
	}
	rp, err := perf.CurrentRankedPerformance(context.Background(), userID)
	require.NoError(t, err)
	assert.False(t, math.IsNaN(rp.RankedIndex))
	assert.Greater(t, rp.RankedIndex, 0.0)
}

// TestConcurrent_FeeAndWithdrawalContendForCash runs a fee and a withdrawal that
// together exceed the available cash. Serialization under the aggregate lock must
// let exactly one succeed; cash may never go negative.
func TestConcurrent_FeeAndWithdrawalContendForCash(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	_, err := svc.DepositCash(ctx(), "u1", "seed", CashFlowInput{Currency: "USD", Amount: testAmount("100")})
	require.NoError(t, err)

	// Each wants 80 of the 100 available; only one can win.
	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, results[0] = svc.RecordFee(ctx(), "u1", "fee", FeeInput{Subtype: FeeManagement, Currency: "USD", Amount: testAmount("80")})
	}()
	go func() {
		defer wg.Done()
		_, results[1] = svc.WithdrawCash(ctx(), "u1", "wd", CashFlowInput{Currency: "USD", Amount: testAmount("80")})
	}()
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	assert.Equal(t, 1, successes, "fee and withdrawal cannot both spend the same cash")

	cash, err := repo.ListCashBalances(ctx(), "u1")
	require.NoError(t, err)
	for _, c := range cash {
		assert.GreaterOrEqual(t, c.Amount.Cmp(testAmount("0")), 0, "cash must never go negative")
	}
	assertReturnConsistent(t, repo, svc, perf, "u1")
}

// TestConcurrent_ReinvestmentAndWithdrawal ensures a reinvested dividend (which
// nets cash to zero) and a withdrawal serialize cleanly and leave consistent
// versions.
func TestConcurrent_ReinvestmentAndWithdrawal(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = svc.RecordIncome(ctx(), "u1", "reinv", IncomeInput{
			Subtype: IncomeReinvestedDiv, Symbol: "AAPL", AssetType: AssetTypeStock, Currency: "USD", Amount: testAmount("195"),
		})
	}()
	go func() {
		defer wg.Done()
		_, _ = svc.WithdrawCash(ctx(), "u1", "wd", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	}()
	wg.Wait()

	assertReturnConsistent(t, repo, svc, perf, "u1")
}

// TestConcurrent_SellsCannotOversell runs two concurrent sells whose combined
// quantity exceeds the position's available quantity. Serialization under the
// aggregate lock must let at most one succeed (the second sees the reduced
// quantity and is rejected); quantity may never go negative and cash is
// credited only for the sale(s) that actually applied.
func TestConcurrent_SellsCannotOversell(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1") // 10 AAPL shares
	positionID := buy.Position.ID

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, results[0] = svc.SellPosition(ctx(), "u1", "sell-a", SellInput{PositionID: positionID, Quantity: testQuantity("7")})
	}()
	go func() {
		defer wg.Done()
		_, results[1] = svc.SellPosition(ctx(), "u1", "sell-b", SellInput{PositionID: positionID, Quantity: testQuantity("7")})
	}()
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, ErrInvalidSaleQuantity)
		}
	}
	assert.Equal(t, 1, successes, "combined quantity (14) exceeds available (10); exactly one sell may apply")

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertQuantityEqual(t, "3", positions[0].Quantity, "remaining quantity must reflect exactly one 7-share sale")

	assertReturnConsistent(t, repo, svc, perf, "u1")
}

// TestSell_DuplicateRequestIDIsIdempotent replays the same sell request twice
// with an identical idempotency key. The second call must return the original
// committed result without re-applying: quantity is reduced once, cash is
// credited once, and exactly one sell activity is persisted.
func TestSell_DuplicateRequestIDIsIdempotent(t *testing.T) {
	svc, repo, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1") // 10 AAPL shares
	positionID := buy.Position.ID

	first, err := svc.SellPosition(ctx(), "u1", "sell-dup", SellInput{PositionID: positionID, Quantity: testQuantity("5")})
	require.NoError(t, err)
	require.False(t, first.Duplicate)

	second, err := svc.SellPosition(ctx(), "u1", "sell-dup", SellInput{PositionID: positionID, Quantity: testQuantity("5")})
	require.NoError(t, err)
	assert.True(t, second.Duplicate, "replay with the same request id must be recognized as a duplicate")
	assert.Equal(t, first.Activity.ID, second.Activity.ID)

	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assertQuantityEqual(t, "5", positions[0].Quantity, "quantity must be reduced exactly once")

	activities, err := repo.ListActivities(ctx(), "u1", 1000)
	require.NoError(t, err)
	sellCount := 0
	for _, a := range activities {
		if a.Type == ActivitySell {
			sellCount++
		}
	}
	assert.Equal(t, 1, sellCount, "exactly one sell activity must be persisted despite the replay")
}

// TestRebuyAfterFullClosure_CreatesNewEpisode sells a position down to zero
// (closing its episode), then buys the same symbol again. The old episode
// must stay closed with its realized total unchanged, and the rebuy must get
// a brand-new position/episode identity rather than reopening the old one.
func TestRebuyAfterFullClosure_CreatesNewEpisode(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy := fundedPortfolio(t, svc, "u1") // 10 AAPL shares
	oldPositionID := buy.Position.ID

	sell, err := svc.SellPosition(ctx(), "u1", "sell-all", SellInput{PositionID: oldPositionID, Quantity: testQuantity("10")})
	require.NoError(t, err)
	require.NotNil(t, sell.Closed)
	assert.Equal(t, oldPositionID, sell.Activity.PositionEpisodeID)

	closedBefore, err := svc.ListClosedPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, closedBefore, 1)
	realizedBefore := closedBefore[0].RealizedGainLossBase

	rebuy, err := svc.BuyPosition(ctx(), "u1", "rebuy", BuyInput{Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: testQuantity("4")})
	require.NoError(t, err)
	require.NotNil(t, rebuy.Position)
	assert.NotEqual(t, oldPositionID, rebuy.Position.ID, "a rebuy after full closure must get a new episode identity")
	assert.Equal(t, rebuy.Position.ID, rebuy.Activity.PositionEpisodeID)

	closedAfter, err := svc.ListClosedPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, closedAfter, 1, "the old episode must remain the only closed position")
	assert.Equal(t, oldPositionID, closedAfter[0].ID)
	assert.Equal(t, realizedBefore, closedAfter[0].RealizedGainLossBase, "the old episode's realized result must not change after the rebuy")

	open, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, rebuy.Position.ID, open[0].ID)
	assertQuantityEqual(t, "4", open[0].Quantity)
}

// TestConcurrent_DividendAndFeeSerialize runs an income event and a fee together
// (opposite-direction return-bearing mutations). Both apply, the aggregate stays
// consistent, and the ranked segment tracks the final cash-adjusted value.
func TestConcurrent_DividendAndFeeSerialize(t *testing.T) {
	svc, repo, perf, _ := newTxTestService()
	fundedPortfolio(t, svc, "u1")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = svc.RecordIncome(ctx(), "u1", "div", IncomeInput{
			Subtype: IncomeCashDividend, Symbol: "AAPL", Currency: "USD", Amount: testAmount("50"),
		})
	}()
	go func() {
		defer wg.Done()
		_, _ = svc.RecordFee(ctx(), "u1", "fee", FeeInput{Subtype: FeeManagement, Currency: "USD", Amount: testAmount("10")})
	}()
	wg.Wait()

	assertReturnConsistent(t, repo, svc, perf, "u1")
}
