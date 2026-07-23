package portfolio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// newRankedTestService wires a portfolio service to a REAL performance service
// (in-memory) so the fairness invariants are exercised end-to-end through the
// actual mutation → checkpoint path.
func newRankedTestService() (*Service, *performance.Service, *prices.MockPriceProvider) {
	pp := prices.NewMockPriceProvider()
	svc := NewService(NewInMemoryRepository(), pp, fx.NewMockFXProvider())
	perf := performance.NewService(performance.NewInMemoryRepository())
	perf.SetValuator(svc)
	svc.SetCheckpointer(perf)
	return svc, perf, pp
}

func rankedIndex(t *testing.T, perf *performance.Service, userID string) float64 {
	t.Helper()
	rp, err := perf.CurrentRankedPerformance(context.Background(), userID)
	require.NoError(t, err)
	return rp.RankedIndex
}

func TestRanked_FirstPositionStartsAt100(t *testing.T) {
	svc, perf, _ := newRankedTestService()
	_, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	require.Equal(t, 100.0, rankedIndex(t, perf, "u1"))
}

func TestRanked_MarketMovePropagates(t *testing.T) {
	svc, perf, pp := newRankedTestService()
	_, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	// AAPL 195 -> 214.5 (+10%).
	pp.Set("AAPL", 214.5, "USD")
	require.Equal(t, 110.0, rankedIndex(t, perf, "u1"))
	// -> 156 (-20% from baseline).
	pp.Set("AAPL", 156.0, "USD")
	require.Equal(t, 80.0, rankedIndex(t, perf, "u1"))
}

func TestRanked_CapitalInjectionCannotDilute(t *testing.T) {
	svc, perf, pp := newRankedTestService()
	// Start: 1 AAPL @ 195 -> value 195, index 100.
	_, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	// AAPL halves: value 97.5, index 50.
	pp.Set("AAPL", 97.5, "USD")
	require.Equal(t, 50.0, rankedIndex(t, perf, "u1"))
	// Inject a large new position (SPY 540 x 100 = 54000). Index must stay 50.
	_, err = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "SPY", AssetType: "etf", Quantity: 100})
	require.NoError(t, err)
	require.Equal(t, 50.0, rankedIndex(t, perf, "u1"))
}

func TestRanked_QuantityIncreaseNoRetroactiveGain(t *testing.T) {
	svc, perf, pp := newRankedTestService()
	pos, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	pp.Set("AAPL", 214.5, "USD") // +10% -> index 110
	require.Equal(t, 110.0, rankedIndex(t, perf, "u1"))
	// Scale the winner x1000; index must remain 110.
	_, err = svc.UpdatePosition(ctx(), "u1", pos.ID, 1000)
	require.NoError(t, err)
	require.Equal(t, 110.0, rankedIndex(t, perf, "u1"))
}

func TestRanked_DeleteAndReAddDoesNotReset(t *testing.T) {
	svc, perf, pp := newRankedTestService()
	pos, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	pp.Set("AAPL", 156.0, "USD") // -20% -> index 80
	require.Equal(t, 80.0, rankedIndex(t, perf, "u1"))
	// Delete the only position -> paused at 80.
	require.NoError(t, svc.DeletePosition(ctx(), "u1", pos.ID))
	rp, err := perf.CurrentRankedPerformance(ctx(), "u1")
	require.NoError(t, err)
	require.Equal(t, performance.StatusPaused, rp.Status)
	require.Equal(t, 80.0, rp.RankedIndex)
	// Re-add the same symbol at its (now lower) price: index stays 80, not reset.
	_, err = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 5})
	require.NoError(t, err)
	require.Equal(t, 80.0, rankedIndex(t, perf, "u1"))
}

func TestRanked_StrategyReplaceDoesNotReset(t *testing.T) {
	svc, perf, pp := newRankedTestService()
	_, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	pp.Set("AAPL", 175.5, "USD") // -10% -> index 90
	require.Equal(t, 90.0, rankedIndex(t, perf, "u1"))
	// Replace the whole portfolio with a copied strategy (fresh baselines).
	err = svc.ReplaceWithStrategyWeights(ctx(), "u1", []StrategyWeightInput{
		{Symbol: "MSFT", AssetType: "stock", WeightPercentage: 60},
		{Symbol: "SPY", AssetType: "etf", WeightPercentage: 40},
	})
	require.NoError(t, err)
	// Index preserved at 90, new segment started at current prices.
	require.Equal(t, 90.0, rankedIndex(t, perf, "u1"))
}

func TestRanked_PriceFailureAbortsMutationAndState(t *testing.T) {
	svc, perf, pp := newRankedTestService()
	_, err := svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 1})
	require.NoError(t, err)
	require.Equal(t, 100.0, rankedIndex(t, perf, "u1"))

	// Make the EXISTING position unpriceable, then attempt to add another. The
	// add must fail (cannot value the portfolio), leaving positions and ranked
	// state unchanged.
	pp.Unset("AAPL")
	_, err = svc.AddPosition(ctx(), "u1", PositionInput{Symbol: "SPY", AssetType: "etf", Quantity: 1})
	require.Error(t, err)

	// Restore price; portfolio still has exactly the one original position and
	// the ranked index is still 100 (no partial mutation, no ranked drift).
	pp.Set("AAPL", 195.0, "USD")
	list, err := svc.ListPositions("u1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, 100.0, rankedIndex(t, perf, "u1"))
}
