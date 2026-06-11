package portfolio

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

func newTestService() *Service {
	return NewService(NewInMemoryRepository(), prices.NewMockPriceProvider(), fx.NewMockFXProvider())
}

func validInput() PositionInput {
	return PositionInput{
		Symbol:          "aapl",
		AssetType:       "stock",
		Quantity:        10,
		AverageBuyPrice: 180,
		Currency:        "usd",
	}
}

func ctx() context.Context { return context.Background() }

// --- portfolio / position service tests --------------------------------------

func TestGetOrCreateDefaultPortfolio_CreatesWhenMissing(t *testing.T) {
	svc := newTestService()

	p, err := svc.GetOrCreateDefaultPortfolio("user-1")

	require.NoError(t, err)
	assert.NotEmpty(t, p.ID)
	assert.Equal(t, "user-1", p.UserID)
	assert.Equal(t, "Default Portfolio", p.Name)
}

func TestGetOrCreateDefaultPortfolio_ReturnsExisting(t *testing.T) {
	svc := newTestService()

	first, err := svc.GetOrCreateDefaultPortfolio("user-1")
	require.NoError(t, err)
	second, err := svc.GetOrCreateDefaultPortfolio("user-1")
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
}

func TestAddPosition_ValidStock(t *testing.T) {
	svc := newTestService()

	pos, err := svc.AddPosition(ctx(), "user-1", validInput())

	require.NoError(t, err)
	assert.NotEmpty(t, pos.ID)
	assert.Equal(t, "AAPL", pos.Symbol)
	assert.Equal(t, "user-1", pos.UserID)
	assert.NotEmpty(t, pos.PortfolioID)
}

func TestAddPosition_ValidCrypto(t *testing.T) {
	svc := newTestService()
	in := PositionInput{Symbol: "BTC-USD", AssetType: "crypto", Quantity: 0.1, AverageBuyPrice: 65000, Currency: "USD"}

	pos, err := svc.AddPosition(ctx(), "user-1", in)

	require.NoError(t, err)
	assert.Equal(t, "BTC-USD", pos.Symbol)
}

func TestAddPosition_ValidTurkishStock(t *testing.T) {
	svc := newTestService()
	in := PositionInput{Symbol: "thyao.is", AssetType: "stock", Quantity: 100, AverageBuyPrice: 250, Currency: "try"}

	pos, err := svc.AddPosition(ctx(), "user-1", in)

	require.NoError(t, err)
	assert.Equal(t, "THYAO.IS", pos.Symbol)
	assert.Equal(t, "TRY", pos.Currency)
}

func TestAddPosition_RejectsEmptySymbol(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.Symbol = "   "
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrSymbolRequired)
}

// --- symbol validation (Problem 1) -------------------------------------------

func TestAddPosition_RejectsUnpriceableSymbol(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.Symbol = "ZZZZ" // valid format, not in mock provider
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrUnsupportedSymbol)
}

func TestAddPosition_RejectsBadlyFormattedSymbols(t *testing.T) {
	svc := newTestService()
	for _, sym := range []string{"XYZ_FAKE", "XYZ FAKE", "A/B", "A;DROP", `A"B`, "🚀MOON", "THISISWAYTOOLONGSYMBOL123"} {
		in := validInput()
		in.Symbol = sym
		_, err := svc.AddPosition(ctx(), "user-1", in)
		assert.ErrorIsf(t, err, ErrUnsupportedSymbol, "symbol %q must be rejected", sym)
	}
}

func TestAddPosition_InvalidSymbolNotPersisted(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, prices.NewMockPriceProvider(), fx.NewMockFXProvider())
	in := validInput()
	in.Symbol = "ZZZZ"

	_, err := svc.AddPosition(ctx(), "user-1", in)
	require.Error(t, err)

	list, err := svc.ListPositions("user-1")
	require.NoError(t, err)
	assert.Empty(t, list, "an invalid symbol must never reach the repository")
}

func TestUpdatePosition_RejectsInvalidSymbol(t *testing.T) {
	svc := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	bad := validInput()
	bad.Symbol = "NOPE_FAKE"
	_, err = svc.UpdatePosition(ctx(), "user-1", pos.ID, bad)
	assert.ErrorIs(t, err, ErrUnsupportedSymbol)

	// Original symbol must remain unchanged.
	list, _ := svc.ListPositions("user-1")
	require.Len(t, list, 1)
	assert.Equal(t, "AAPL", list[0].Symbol)
}

func TestAddPosition_RejectsInvalidAssetType(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.AssetType = "bond"
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrInvalidAssetType)
}

func TestAddPosition_RejectsNonPositiveQuantity(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.Quantity = 0
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrInvalidQuantity)
}

func TestAddPosition_RejectsNonPositiveBuyPrice(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.AverageBuyPrice = 0
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrInvalidPrice)
}

func TestAddPosition_RejectsMissingCurrency(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.Currency = ""
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrCurrencyRequired)
}

func TestAddPosition_RejectsUnsupportedCurrency(t *testing.T) {
	svc := newTestService()
	in := validInput()
	in.Currency = "JPY" // not in mock FX
	_, err := svc.AddPosition(ctx(), "user-1", in)
	assert.ErrorIs(t, err, ErrUnsupportedCurrency)
}

func TestAddPosition_NormalizesSymbolAndCurrency(t *testing.T) {
	svc := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)
	assert.Equal(t, "AAPL", pos.Symbol)
	assert.Equal(t, "USD", pos.Currency)
}

func TestListPositions_OnlyReturnsCurrentUsersPositions(t *testing.T) {
	svc := newTestService()
	_, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)
	_, err = svc.AddPosition(ctx(), "user-2", validInput())
	require.NoError(t, err)

	list1, err := svc.ListPositions("user-1")
	require.NoError(t, err)
	assert.Len(t, list1, 1)
	assert.Equal(t, "user-1", list1[0].UserID)
}

func TestUpdatePosition_OwnSucceeds(t *testing.T) {
	svc := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	updated, err := svc.UpdatePosition(ctx(), "user-1", pos.ID, PositionInput{
		Symbol: "AAPL", AssetType: "stock", Quantity: 12, AverageBuyPrice: 175, Currency: "USD",
	})
	require.NoError(t, err)
	assert.Equal(t, 12.0, updated.Quantity)
	assert.Equal(t, 175.0, updated.AverageBuyPrice)
}

func TestUpdatePosition_OtherUsersFails(t *testing.T) {
	svc := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	_, err = svc.UpdatePosition(ctx(), "user-2", pos.ID, validInput())
	assert.ErrorIs(t, err, ErrPositionNotFound)
}

func TestDeletePosition_OwnSucceeds(t *testing.T) {
	svc := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	require.NoError(t, svc.DeletePosition("user-1", pos.ID))
	list, _ := svc.ListPositions("user-1")
	assert.Empty(t, list)
}

func TestDeletePosition_OtherUsersFails(t *testing.T) {
	svc := newTestService()
	pos, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	err = svc.DeletePosition("user-2", pos.ID)
	assert.ErrorIs(t, err, ErrPositionNotFound)
}

// --- summary (base-currency, Problem 2) --------------------------------------

func TestSummary_SinglePositionUSD(t *testing.T) {
	svc := newTestService()
	_, err := svc.AddPosition(ctx(), "user-1", validInput()) // AAPL 10 @ 180 USD, price 195
	require.NoError(t, err)

	sum, err := svc.Summary(ctx(), "user-1")
	require.NoError(t, err)

	assert.Equal(t, "USD", sum.BaseCurrency)
	require.Len(t, sum.Positions, 1)
	ps := sum.Positions[0]
	assert.Equal(t, 1800.0, ps.CostBasis)
	assert.Equal(t, 1950.0, ps.CurrentValue)
	assert.Equal(t, 1800.0, ps.CostBasisBase)
	assert.Equal(t, 1950.0, ps.CurrentValueBase)
	assert.Equal(t, 150.0, ps.GainLossBase)
	assert.Equal(t, "USD", ps.CurrentPriceCurrency)
	assert.Equal(t, "USD", ps.BaseCurrency)

	assert.Equal(t, 1800.0, sum.TotalCostBasis)
	assert.Equal(t, 1950.0, sum.CurrentValue)
	assert.Equal(t, 150.0, sum.GainLoss)
	assert.InDelta(t, 8.33, sum.GainLossPercentage, 0.01)
	assert.InDelta(t, 108.33, sum.PortfolioIndex, 0.01)
}

func TestSummary_MixedCurrencyNormalizedToUSD(t *testing.T) {
	svc := newTestService()
	// AAPL 10@180 USD (price 195) and THYAO.IS 100@250 TRY (price 295), TRY=0.031.
	_, err := svc.AddPosition(ctx(), "user-1", PositionInput{Symbol: "AAPL", AssetType: "stock", Quantity: 10, AverageBuyPrice: 180, Currency: "USD"})
	require.NoError(t, err)
	_, err = svc.AddPosition(ctx(), "user-1", PositionInput{Symbol: "THYAO.IS", AssetType: "stock", Quantity: 100, AverageBuyPrice: 250, Currency: "TRY"})
	require.NoError(t, err)

	sum, err := svc.Summary(ctx(), "user-1")
	require.NoError(t, err)

	// AAPL base: 1800 / 1950. THYAO base: 25000*0.031=775 / 29500*0.031=914.5.
	assert.InDelta(t, 2575.0, sum.TotalCostBasis, 0.01)
	assert.InDelta(t, 2864.5, sum.CurrentValue, 0.01)
	assert.InDelta(t, 289.5, sum.GainLoss, 0.01)
	assert.InDelta(t, 11.24, sum.GainLossPercentage, 0.05)
	assert.InDelta(t, 111.24, sum.PortfolioIndex, 0.05)

	// Per-position THYAO keeps local values AND exposes base values.
	var thyao PositionSummary
	for _, p := range sum.Positions {
		if p.Symbol == "THYAO.IS" {
			thyao = p
		}
	}
	assert.Equal(t, 25000.0, thyao.CostBasis)    // local TRY
	assert.Equal(t, 29500.0, thyao.CurrentValue) // local TRY
	assert.InDelta(t, 775.0, thyao.CostBasisBase, 0.01)
	assert.InDelta(t, 914.5, thyao.CurrentValueBase, 0.01)
	assert.InDelta(t, 139.5, thyao.GainLossBase, 0.01)
}

func TestSummary_EmptyPortfolio(t *testing.T) {
	svc := newTestService()

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
