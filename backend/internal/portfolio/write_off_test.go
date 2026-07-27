package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These cover the write-off escape hatch: a position whose symbol has no
// available market price (never cached, delisted, provider stopped covering
// it) used to make EVERY mutation on the account fail, because every
// mutation re-prices all currently-held symbols for the ranked-index
// checkpoint. WriteOffUnpriceablePosition is the narrow, abuse-resistant way
// out — it only works when the symbol genuinely cannot be priced.

func TestWriteOffUnpriceablePosition_RejectsStillPriceableSymbol(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "buy-aapl", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 10,
	})
	require.NoError(t, err)

	_, err = svc.WriteOffUnpriceablePosition(ctx(), "u1", "wo-1", buy.Position.ID)

	assert.ErrorIs(t, err, ErrPositionIsPriceable,
		"a priceable position must be sold, not written off — this guard prevents erasing a losing but tradeable position")

	// Nothing changed: the position is still open.
	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, PositionStatusOpen, positionStatus(positions[0]))
}

func TestWriteOffUnpriceablePosition_SucceedsAndRealizesFullBasisAsLoss(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "buy-aapl", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 10, ExecutionPrice: 195,
	})
	require.NoError(t, err)
	rankedBefore := buy.RankedIndexAfter

	// The provider stops covering the symbol entirely (delisting, coverage
	// gap, or a provider switch where this symbol was never cached).
	pp.Unset("AAPL")

	res, err := svc.WriteOffUnpriceablePosition(ctx(), "u1", "wo-2", buy.Position.ID)
	require.NoError(t, err)
	require.NotNil(t, res.Closed)
	assert.Equal(t, -100.0, res.Closed.RealizedGainLossPercentage)
	assert.InDelta(t, -1950.0, res.Closed.RealizedGainLossBase, 0.01, "the full cost basis (10 x 195) is realized as a loss")

	// Ranked index drops by exactly the known cost basis — never a fabricated
	// market move, since no market price for this symbol ever existed.
	assert.Less(t, res.RankedIndexAfter, rankedBefore)

	open, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	assert.Empty(t, open)
	closed, err := svc.ListClosedPositions(ctx(), "u1")
	require.NoError(t, err)
	require.Len(t, closed, 1)
	assert.Equal(t, buy.Position.ID, closed[0].ID)
}

// TestWriteOffUnpriceablePosition_UnblocksEverythingElse is the actual
// end-to-end proof: before the write-off, every mutation on the account
// failed because of the one broken symbol; after it, everything works again.
func TestWriteOffUnpriceablePosition_UnblocksEverythingElse(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	seedCash(t, svc, "u1", "USD", 100000)
	broken, err := svc.BuyPosition(ctx(), "u1", "buy-aapl", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 10,
	})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), "u1", "buy-msft", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 5,
	})
	require.NoError(t, err)

	pp.Unset("AAPL")

	// Sanity check the bug this fixes: an unrelated deposit is currently
	// blocked solely because AAPL can't be priced.
	_, err = svc.DepositCash(ctx(), "u1", "dep-blocked", CashFlowInput{Currency: "USD", Amount: 100})
	require.Error(t, err, "sanity check: the broken symbol blocks unrelated mutations before write-off")

	_, err = svc.WriteOffUnpriceablePosition(ctx(), "u1", "wo-3", broken.Position.ID)
	require.NoError(t, err)

	_, err = svc.DepositCash(ctx(), "u1", "dep-unblocked", CashFlowInput{Currency: "USD", Amount: 100})
	require.NoError(t, err, "an unrelated deposit must work once the broken position is written off")

	_, err = svc.BuyPosition(ctx(), "u1", "buy-nvda", BuyInput{Symbol: "NVDA", AssetType: AssetTypeStock, Quantity: 1})
	require.NoError(t, err, "buying a healthy symbol must work once the broken position is written off")

	_, err = svc.SellPosition(ctx(), "u1", "sell-msft", SellInput{Symbol: "MSFT", Quantity: 1})
	require.NoError(t, err, "selling a healthy symbol must work once the broken position is written off")

	_, err = svc.Summary(ctx(), "u1")
	require.NoError(t, err, "Summary must work once the broken position is written off")
}

func TestWriteOffUnpriceablePosition_RejectsClosedPosition(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "buy-msft", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: 2, ExecutionPrice: 400,
	})
	require.NoError(t, err)
	_, err = svc.SellPosition(ctx(), "u1", "sell-msft", SellInput{Symbol: "MSFT", Quantity: 2, ExecutionPrice: 450})
	require.NoError(t, err)

	_, err = svc.WriteOffUnpriceablePosition(ctx(), "u1", "wo-4", buy.Position.ID)

	assert.ErrorIs(t, err, ErrPositionClosed)
}

// TestWriteOffUnpriceablePosition_ReconcilesCleanly guards against a real bug
// this feature exposed: ActivityWriteOff sets RealizedGainLossBase but
// summarizeLedger's realized-P&L accumulation only ever read it for
// ActivitySell (write-off had no caller before this endpoint, so the gap was
// latent). A silently-dropped realized loss would make the accounting
// summary (and its independent reconciliation check) wrong for anyone using
// this feature.
func TestWriteOffUnpriceablePosition_ReconcilesCleanly(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	seedCash(t, svc, "u1", "USD", 100000)
	broken, err := svc.BuyPosition(ctx(), "u1", "buy-aapl", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 10, ExecutionPrice: 195,
	})
	require.NoError(t, err)
	pp.Unset("AAPL")

	_, err = svc.WriteOffUnpriceablePosition(ctx(), "u1", "wo-6", broken.Position.ID)
	require.NoError(t, err)

	sum, err := svc.Summary(ctx(), "u1")
	require.NoError(t, err)
	assert.InDelta(t, -1950.0, sum.Realized.RealizedPnLBase, 0.01,
		"the write-off's realized loss must be reflected in the accounting summary")
	assert.True(t, sum.Reconciliation.IsConsistent, "reasons: %v", sum.Reconciliation.Reasons)
	assert.Equal(t, 0.0, sum.Reconciliation.Difference)
}

func TestWriteOffUnpriceablePosition_RejectsOtherUsersPosition(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "buy-aapl", BuyInput{
		Symbol: "AAPL", AssetType: AssetTypeStock, Quantity: 10,
	})
	require.NoError(t, err)

	_, err = svc.WriteOffUnpriceablePosition(ctx(), "u2", "wo-5", buy.Position.ID)

	assert.ErrorIs(t, err, ErrPositionNotFound)
}
