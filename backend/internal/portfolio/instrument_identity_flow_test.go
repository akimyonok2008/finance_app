package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/instrument"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type identityProviderFunc func(context.Context, instrument.IdentityQuery) ([]instrument.IdentityCandidate, error)

func (f identityProviderFunc) Resolve(ctx context.Context, q instrument.IdentityQuery) ([]instrument.IdentityCandidate, error) {
	return f(ctx, q)
}

func TestBuyFlow_ReusesExistingAliasWithoutProviderCall(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	pp.Set("FB", 100, "USD")
	pp.Set("META", 500, "USD")
	identities := instrument.NewInMemoryRepository()
	in, err := identities.CreateInstrument(ctx(), instrument.Instrument{
		ID: "stable-msft", CurrentSymbol: "MSFT", Status: instrument.StatusActive,
		IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	_, err = identities.CreateAlias(ctx(), instrument.InstrumentAlias{
		InstrumentID: in.ID, AliasType: instrument.AliasTicker, AliasValue: "MSFT",
		ExchangeCode: "UN", ValidFrom: time.Now().UTC().Add(-time.Hour),
	})
	require.NoError(t, err)
	calls := 0
	resolver := instrument.NewResolver(identities, identityProviderFunc(func(context.Context, instrument.IdentityQuery) ([]instrument.IdentityCandidate, error) {
		calls++
		return nil, nil
	}))
	svc.SetInstrumentResolver(resolver)

	buy, err := svc.BuyPosition(ctx(), "u1", "alias-hit", BuyInput{
		Symbol: "MSFT", ExchangeCode: "UN", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	assert.Equal(t, in.ID, buy.Position.InstrumentID)
	assert.Zero(t, calls)
}

// TestPriceLookupSymbol_ResolvesRenamedTickerViaInstrumentID is the
// provider-symbol-mapper regression: a position bought under an old ticker
// (FB) must be priced under the CURRENT alias (META) once the instrument's
// ticker changed, instead of the stale symbol string stored on the position.
func TestPriceLookupSymbol_ResolvesRenamedTickerViaInstrumentID(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	pp.Set("FB", 100, "USD")
	pp.Set("META", 350, "USD")
	identities := instrument.NewInMemoryRepository()
	in, err := identities.CreateInstrument(ctx(), instrument.Instrument{
		CurrentSymbol: "META", Status: instrument.StatusActive, IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	resolver := instrument.NewResolver(identities, nil)
	require.NoError(t, resolver.ChangeTicker(ctx(), in.ID, "META", "UN", "", time.Now().UTC().Add(-time.Hour), "test"))
	svc.SetInstrumentResolver(resolver)

	// Position still carries the pre-rename symbol (as a legacy row would),
	// but has the resolved instrument_id.
	resolved := svc.priceLookupSymbol(ctx(), "FB", in.ID)
	assert.Equal(t, "META", resolved, "must resolve the current alias, not the stale stored symbol")

	// No instrument_id: falls back to the stored symbol unchanged.
	assert.Equal(t, "FB", svc.priceLookupSymbol(ctx(), "FB", ""))
}

// TestBuyFlow_UnresolvedIdentityRejectedWhenResolutionRequired is the
// production-gate regression: with SetInstrumentResolutionRequired(true) (as
// config.InstrumentResolutionRequired forces under APP_ENV=production), a
// buy whose ticker resolves to nothing must be rejected instead of saving a
// ticker-only position.
func TestBuyFlow_UnresolvedIdentityRejectedWhenResolutionRequired(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	pp.Set("ZZZZ", 10, "USD")
	identities := instrument.NewInMemoryRepository()
	resolver := instrument.NewResolver(identities, identityProviderFunc(func(context.Context, instrument.IdentityQuery) ([]instrument.IdentityCandidate, error) {
		return nil, nil // no candidates: unresolved
	}))
	svc.SetInstrumentResolver(resolver)
	svc.SetInstrumentResolutionRequired(true)

	_, err := svc.BuyPosition(ctx(), "u1", "req-unresolved", BuyInput{
		Symbol: "ZZZZ", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.ErrorIs(t, err, ErrInstrumentIdentityUnresolved)

	// The same buy succeeds when resolution is not required (development
	// default): existing ticker-only-position behavior is unchanged.
	svc.SetInstrumentResolutionRequired(false)
	_, err = svc.BuyPosition(ctx(), "u1", "req-unresolved-2", BuyInput{
		Symbol: "ZZZZ", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
}

// TestOutboxEvent_CarriesInstrumentIdentitySnapshot is the outbox-instrument-
// fields regression: a buy's committed outbox event must carry the resolved
// instrument_id and the symbol as of that mutation, while a cash-only
// mutation (deposit) carries neither.
func TestOutboxEvent_CarriesInstrumentIdentitySnapshot(t *testing.T) {
	svc, repo, _, pp := newTxTestService()
	pp.Set("MSFT", 100, "USD")
	identities := instrument.NewInMemoryRepository()
	in, err := identities.CreateInstrument(ctx(), instrument.Instrument{
		CurrentSymbol: "MSFT", Status: instrument.StatusActive, IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	resolver := instrument.NewResolver(identities, nil)
	require.NoError(t, resolver.ChangeTicker(ctx(), in.ID, "MSFT", "UN", "", time.Now().UTC().Add(-time.Hour), "test"))
	svc.SetInstrumentResolver(resolver)

	_, err = svc.DepositCash(ctx(), "u1", "dep-1", CashFlowInput{Currency: "USD", Amount: testAmount("1000")})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), "u1", "buy-1", BuyInput{
		Symbol: "MSFT", ExchangeCode: "UN", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)

	events := repo.OutboxEvents()
	var depositEvent, buyEvent *OutboxEvent
	for i := range events {
		if depositEvent == nil {
			depositEvent = &events[i]
		}
		if events[i].InstrumentID != "" {
			buyEvent = &events[i]
		}
	}
	require.NotNil(t, depositEvent)
	require.NotNil(t, buyEvent)
	assert.Empty(t, depositEvent.InstrumentID, "a cash-only mutation must not carry an instrument identity")
	assert.Equal(t, in.ID, buyEvent.InstrumentID)
	assert.Equal(t, "MSFT", buyEvent.DisplaySymbolAtEventTime)
}

func TestBuyFlow_SameTickerDifferentExchangesDoesNotMerge(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	identities := instrument.NewInMemoryRepository()
	resolver := instrument.NewResolver(identities, identityProviderFunc(func(_ context.Context, q instrument.IdentityQuery) ([]instrument.IdentityCandidate, error) {
		return []instrument.IdentityCandidate{{
			FIGI: "FIGI-" + q.ExchangeCode, Ticker: q.Ticker, ExchangeCode: q.ExchangeCode,
			SecurityType: "Common Stock", Currency: "USD",
		}}, nil
	}))
	svc.SetInstrumentResolver(resolver)

	first, err := svc.BuyPosition(ctx(), "u1", "nyse", BuyInput{
		Symbol: "MSFT", ExchangeCode: "UN", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	second, err := svc.BuyPosition(ctx(), "u1", "xetra", BuyInput{
		Symbol: "MSFT", ExchangeCode: "GY", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	assert.NotEqual(t, first.Position.InstrumentID, second.Position.InstrumentID)
	positions, err := svc.ListPositions(ctx(), "u1")
	require.NoError(t, err)
	assert.Len(t, positions, 2)
}

func TestBuyFlow_UnresolvedScopedTickerDoesNotFallBackToTickerMerge(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	svc.SetInstrumentResolver(instrument.NewResolver(instrument.NewInMemoryRepository(), nil))

	first, err := svc.BuyPosition(ctx(), "u1", "unresolved-un", BuyInput{
		Symbol: "MSFT", ExchangeCode: "UN", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx(), "u1", "unresolved-gy", BuyInput{
		Symbol: "MSFT", ExchangeCode: "GY", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.ErrorIs(t, err, ErrInstrumentIdentityUnresolvedConflict)
	assert.Empty(t, first.Position.InstrumentID)
	positions, listErr := svc.ListPositions(ctx(), "u1")
	require.NoError(t, listErr)
	assert.Len(t, positions, 1)
}

func TestBuyFlow_AliasesAndRebuyPreserveStableInstrument(t *testing.T) {
	svc, _, _, pp := newTxTestService()
	pp.Set("FB", 100, "USD")
	pp.Set("META", 500, "USD")
	identities := instrument.NewInMemoryRepository()
	in, err := identities.CreateInstrument(ctx(), instrument.Instrument{
		ID: "meta-stable", CurrentSymbol: "META", Status: instrument.StatusActive,
		IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	for _, ticker := range []string{"FB", "META"} {
		_, err = identities.CreateAlias(ctx(), instrument.InstrumentAlias{
			InstrumentID: in.ID, AliasType: instrument.AliasTicker, AliasValue: ticker,
			ExchangeCode: "UW", ValidFrom: time.Now().UTC().Add(-time.Hour),
		})
		require.NoError(t, err)
	}
	svc.SetInstrumentResolver(instrument.NewResolver(identities, nil))

	first, err := svc.BuyPosition(ctx(), "u1", "fb-buy", BuyInput{
		Symbol: "FB", ExchangeCode: "UW", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	_, err = svc.SellPosition(ctx(), "u1", "fb-close", SellInput{
		PositionID: first.Position.ID, Quantity: testQuantity("1"), ExecutionPrice: testPrice("100"),
	})
	require.NoError(t, err)
	second, err := svc.BuyPosition(ctx(), "u1", "meta-rebuy", BuyInput{
		Symbol: "META", ExchangeCode: "UW", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.NoError(t, err)
	assert.NotEqual(t, first.Position.ID, second.Position.ID)
	assert.Equal(t, in.ID, first.Position.InstrumentID)
	assert.Equal(t, in.ID, second.Position.InstrumentID)
}

func TestBuyFlow_AmbiguousIdentityDoesNotSelectFirstCandidate(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	resolver := instrument.NewResolver(instrument.NewInMemoryRepository(),
		identityProviderFunc(func(context.Context, instrument.IdentityQuery) ([]instrument.IdentityCandidate, error) {
			return []instrument.IdentityCandidate{
				{FIGI: "candidate-one", Ticker: "MSFT", ExchangeCode: "UN"},
				{FIGI: "candidate-two", Ticker: "MSFT", ExchangeCode: "GY"},
			}, nil
		}))
	svc.SetInstrumentResolver(resolver)

	_, err := svc.BuyPosition(ctx(), "u1", "ambiguous-buy", BuyInput{
		Symbol: "MSFT", AssetType: AssetTypeStock, Quantity: testQuantity("1"),
	})
	require.ErrorIs(t, err, ErrInstrumentIdentityAmbiguous)
	positions, listErr := svc.ListPositions(ctx(), "u1")
	require.NoError(t, listErr)
	assert.Empty(t, positions)
}

func TestPositionSpecificIncomeKeepsInstrumentIdentityAndSymbolSnapshot(t *testing.T) {
	svc, _, _, _ := newTxTestService()
	buy, err := svc.BuyPosition(ctx(), "u1", "identity-income-buy", BuyInput{
		Symbol: "MSFT", InstrumentID: "stable-income-instrument",
		AssetType: AssetTypeStock, Quantity: testQuantity("2"),
	})
	require.NoError(t, err)

	income, err := svc.RecordIncome(ctx(), "u1", "identity-income", IncomeInput{
		Subtype: IncomeCashDividend, Symbol: "MSFT", Currency: "USD",
		Amount: testAmount("5"), Provenance: ProvenanceProviderReported,
	})
	require.NoError(t, err)
	require.NotNil(t, income.Activity)
	assert.Equal(t, buy.Position.InstrumentID, income.Activity.InstrumentID)
	assert.Equal(t, "MSFT", income.Activity.Symbol)
	assert.Equal(t, buy.Position.ID, income.Activity.PositionEpisodeID)
}
