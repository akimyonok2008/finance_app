package portfolio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

func newTestService() (*Service, *prices.MockPriceProvider) {
	pp := prices.NewMockPriceProvider()
	return NewService(NewInMemoryRepository(), pp, fx.NewMockFXProvider()), pp
}

func validInput() PositionInput {
	return PositionInput{
		Symbol:    "aapl",
		AssetType: "stock",
		Quantity:  10,
	}
}

func ctx() context.Context { return context.Background() }

// --- portfolio / position service tests --------------------------------------

func TestGetOrCreateDefaultPortfolio_CreatesWhenMissing(t *testing.T) {
	svc, _ := newTestService()

	p, err := svc.GetOrCreateDefaultPortfolio("user-1")

	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, "user-1", p.UserID)
	assert.Equal(t, "Default Portfolio", p.Name)
}

func TestGetOrCreateDefaultPortfolio_ReturnsExisting(t *testing.T) {
	svc, _ := newTestService()

	first, err := svc.GetOrCreateDefaultPortfolio("user-1")
	require.NoError(t, err)
	second, err := svc.GetOrCreateDefaultPortfolio("user-1")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
}

// --- baseline locking (current-day price model) --------------------------------

func TestAddPosition_LocksBaselineAtCurrentPrice(t *testing.T) {
	svc, _ := newTestService()

	pos, err := svc.AddPosition(ctx(), "user-1", validInput())

	require.NoError(t, err)
	assert.NotEmpty(t, pos.ID)
	assert.Equal(t, "AAPL", pos.Symbol)
	assert.Equal(t, "user-1", pos.UserID)
	assert.NotEmpty(t, pos.PortfolioID)
	// Baseline = today's mock quote (AAPL 195 USD); the client never sends it.
	assert.Equal(t, 195.0, pos.AverageBuyPrice)
	assert.Equal(t, "USD", pos.Currency)
}

func TestAddPosition_BaselineUsesQuoteCurrency(t *testing.T) {
	svc, _ := newTestService()
	in := PositionInput{Symbol: "thyao.is", AssetType: "stock", Quantity: 100}

	pos, err := svc.AddPosition(ctx(), "user-1", in)

	require.NoError(t, err)
	assert.Equal(t, "THYAO.IS", pos.Symbol)
	assert.Equal(t, 295.0, pos.AverageBuyPrice) // mock THYAO.IS quote
	assert.Equal(t, "TRY", pos.Currency)        // quote currency, not user input
}

func TestAddPosition_ValidCrypto(t *testing.T) {
	svc, _ := newTestService()
	in := PositionInput{Symbol: "BTC-USD", AssetType: "crypto", Quantity: 0.1}

	pos, err := svc.AddPosition(ctx(), "user-1", in)

	require.NoError(t, err)
	assert.Equal(t, "BTC-USD", pos.Symbol)
	assert.Equal(t, 68000.0, pos.AverageBuyPrice)
}

func TestAddPosition_FreshPositionStartsAtIndex100(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	sum, err := svc.Summary(ctx(), "user-1")
	require.NoError(t, err)
	assert.Equal(t, 0.0, sum.GainLossPercentage)
	assert.Equal(t, 100.0, sum.PortfolioIndex)
}

func TestAddPosition_RejectsEmptySymbol(t *testing.T) {
	svc, _ := newTestService()
	in := validInput()
	in.Symbol = "   "
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrSymbolRequired)
}

// --- symbol validation ---------------------------------------------------------

func TestAddPosition_RejectsUnpriceableSymbol(t *testing.T) {
	svc, _ := newTestService()
	in := validInput()
	in.Symbol = "ZZZZ" // valid format, not in mock provider
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrUnsupportedSymbol)
}

func TestAddPosition_RejectsBadlyFormattedSymbols(t *testing.T) {
	svc, _ := newTestService()
	for _, sym := range []string{"XYZ_FAKE", "XYZ FAKE", "A/B", "A;DROP", `A"B`, "🚀MOON", "THISISWAYTOOLONGSYMBOL123"} {
		in := validInput()
		in.Symbol = sym
		_, err := svc.AddPosition(ctx(), "user-1", in)
		assert.ErrorIsf(t, err, ErrUnsupportedSymbol, "symbol %q must be rejected", sym)
	}
}

func TestAddPosition_InvalidSymbolNotPersisted(t *testing.T) {
	svc, _ := newTestService()
	in := validInput()
	in.Symbol = "ZZZZ"

	_, err := svc.AddPosition(ctx(), "user-1", in)
	require.Error(t, err)

	list, err := svc.ListPositions("user-1")
	require.NoError(t, err)
	assert.Empty(t, list, "an invalid symbol must never reach the repository")
}

func TestAddPosition_RejectsInvalidAssetType(t *testing.T) {
	svc, _ := newTestService()
	in := validInput()
	in.AssetType = "bond"
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrInvalidAssetType)
}

func TestAddPosition_RejectsNonPositiveQuantity(t *testing.T) {
	svc, _ := newTestService()
	in := validInput()
	in.Quantity = 0
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}

func TestAddPosition_RejectsUnsupportedQuoteCurrency(t *testing.T) {
	svc, pp := newTestService()
	pp.Set("TOKYO.T", 1500, "JPY") // priceable, but JPY is not in the mock FX
	in := PositionInput{Symbol: "TOKYO.T", AssetType: "stock", Quantity: 1}
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrUnsupportedCurrency)
}

func TestAddPosition_NormalizesSymbol(t *testing.T) {
	svc, _ := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)
	assert.Equal(t, "AAPL", pos.Symbol)
	assert.Equal(t, "USD", pos.Currency)
}

func TestListPositions_OnlyReturnsCurrentUsersPositions(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)
	_, err = svc.AddPosition(ctx(), "user-2", validInput())
	require.NoError(t, err)

	list1, err := svc.ListPositions("user-1")
	require.NoError(t, err)
	assert.Len(t, list1, 1)
	assert.Equal(t, "user-1", list1[0].UserID)
}

// --- update: quantity only, baseline immutable ---------------------------------

func TestUpdatePosition_QuantityOnly(t *testing.T) {
	svc, _ := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	updated, err := svc.UpdatePosition(ctx(), "user-1", pos.ID, 12)
	require.NoError(t, err)
	assert.Equal(t, 12.0, updated.Quantity)
	// Baseline price and symbol survive the edit untouched.
	assert.Equal(t, 195.0, updated.AverageBuyPrice)
	assert.Equal(t, "AAPL", updated.Symbol)
}

func TestUpdatePosition_BaselineSurvivesPriceMoves(t *testing.T) {
	svc, pp := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput()) // baseline 195
	require.NoError(t, err)

	pp.Set("AAPL", 250, "USD") // market moves
	updated, err := svc.UpdatePosition(ctx(), "user-1", pos.ID, 20)
	require.NoError(t, err)
	// Editing quantity must NOT re-lock the baseline at the new price.
	assert.Equal(t, 195.0, updated.AverageBuyPrice)
}

func TestUpdatePosition_RejectsNonPositiveQuantity(t *testing.T) {
	svc, _ := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	_, err = svc.UpdatePosition(ctx(), "user-1", pos.ID, 0)
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}

func TestUpdatePosition_OtherUsersFails(t *testing.T) {
	svc, _ := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	_, err = svc.UpdatePosition(ctx(), "user-2", pos.ID, 5)
	assert.ErrorIs(t, err, ErrPositionNotFound)
}

func TestDeletePosition_OwnSucceeds(t *testing.T) {
	svc, _ := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	require.NoError(t, svc.DeletePosition("user-1", pos.ID))
	list, _ := svc.ListPositions("user-1")
	assert.Empty(t, list)
}

func TestDeletePosition_OtherUsersFails(t *testing.T) {
	svc, _ := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	err = svc.DeletePosition("user-2", pos.ID)
	assert.ErrorIs(t, err, ErrPositionNotFound)
}

// --- summary (performance measured from the locked baseline) -------------------

func TestSummary_GainComesOnlyFromPostAddMoves(t *testing.T) {
	svc, pp := newTestService()
	_, err := svc.AddPosition(ctx(), "user-1", validInput()) // AAPL 10 @ baseline 195
	require.NoError(t, err)

	pp.Set("AAPL", 214.5, "USD") // +10% after the add

	sum, err := svc.Summary(ctx(), "user-1")
	require.NoError(t, err)

	assert.Equal(t, "USD", sum.BaseCurrency)
	require.Len(t, sum.Positions, 1)
	ps := sum.Positions[0]
	assert.Equal(t, 1950.0, ps.CostBasis) // 10 × baseline 195
	assert.Equal(t, 2145.0, ps.CurrentValue)
	assert.InDelta(t, 10.0, sum.GainLossPercentage, 0.01)
	assert.InDelta(t, 110.0, sum.PortfolioIndex, 0.01)
}

func TestSummary_MixedCurrencyNormalizedToUSD(t *testing.T) {
	svc, pp := newTestService()
	// Baselines lock at AAPL 195 USD and THYAO.IS 295 TRY (TRY=0.031).
	_, err := svc.AddPosition(ctx(), "user-1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 10})
	require.NoError(t, err)
	_, err = svc.AddPosition(ctx(), "user-1", PositionInput{Symbol: "THYAO.IS", AssetType: "stock", Quantity: 100})
	require.NoError(t, err)

	pp.Set("AAPL", 214.5, "USD")     // +10%
	pp.Set("THYAO.IS", 324.5, "TRY") // +10%

	sum, err := svc.Summary(ctx(), "user-1")
	require.NoError(t, err)

	// Baseline base: 1950 + 29500*0.031=914.5 -> 2864.5. Value: +10% across.
	assert.InDelta(t, 2864.5, sum.TotalCostBasis, 0.01)
	assert.InDelta(t, 3150.95, sum.CurrentValue, 0.01)
	assert.InDelta(t, 10.0, sum.GainLossPercentage, 0.05)
	assert.InDelta(t, 110.0, sum.PortfolioIndex, 0.05)

	// Per-position THYAO keeps local values AND exposes base values.
	var thyao PositionSummary
	for _, p := range sum.Positions {
		if p.Symbol == "THYAO.IS" {
			thyao = p
		}
	}
	assert.Equal(t, 29500.0, thyao.CostBasis)    // local TRY at baseline
	assert.Equal(t, 32450.0, thyao.CurrentValue) // local TRY now
	assert.InDelta(t, 914.5, thyao.CostBasisBase, 0.01)
	assert.InDelta(t, 1005.95, thyao.CurrentValueBase, 0.01)
}

func TestSummary_EmptyPortfolio(t *testing.T) {
	svc, _ := newTestService()

	sum, err := svc.Summary(ctx(), "user-1")
	require.NoError(t, err)

	assert.Empty(t, sum.Positions)
	assert.Equal(t, 0.0, sum.TotalCostBasis)
	assert.Equal(t, 0.0, sum.GainLossPercentage)
	assert.Equal(t, 100.0, sum.PortfolioIndex)
	assert.Equal(t, "USD", sum.BaseCurrency)
}

func TestRepository_GetPositionUnknownReturnsNotFound(t *testing.T) {
	repo := NewInMemoryRepository()
	_, err := repo.GetPosition("does-not-exist")
	assert.ErrorIs(t, err, ErrPositionNotFound)
}
