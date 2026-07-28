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
