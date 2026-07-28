package corpactions_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// testGateway adapts a real portfolio.Service to the pipeline gateway, so tests
// exercise the true aggregate coordinator (atomic, idempotent, ranked-neutral).
type testGateway struct{ svc *portfolio.Service }

func (g testGateway) ActiveSymbols(ctx context.Context) ([]string, error) {
	return g.svc.ActiveSymbols(ctx)
}
func (g testGateway) HoldersOfSymbol(ctx context.Context, symbol string) ([]corpactions.Holder, error) {
	hs, err := g.svc.HoldersOfSymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	out := make([]corpactions.Holder, 0, len(hs))
	for _, h := range hs {
		out = append(out, corpactions.Holder{UserID: h.UserID, PortfolioID: h.PortfolioID, AcquiredAt: h.AcquiredAt})
	}
	return out, nil
}
func (g testGateway) ApplySplit(ctx context.Context, userID, requestID, symbol string, num, den float64, at time.Time) error {
	sub := portfolio.CorpStockSplit
	if num < den {
		sub = portfolio.CorpReverseSplit
	}
	_, err := g.svc.RecordCorporateAction(ctx, userID, requestID, portfolio.CorpActionInput{
		Subtype: sub, Symbol: symbol, RatioNumerator: num, RatioDenominator: den,
		OccurredAt: &at, Provenance: portfolio.ProvenanceProviderReported,
	})
	return err
}
func (g testGateway) ApplySymbolChange(ctx context.Context, userID, requestID, oldSym, newSym string, at time.Time) error {
	_, err := g.svc.RecordCorporateAction(ctx, userID, requestID, portfolio.CorpActionInput{
		Subtype: portfolio.CorpSymbolChange, Symbol: oldSym, NewSymbol: newSym,
		OccurredAt: &at, Provenance: portfolio.ProvenanceProviderReported,
	})
	return err
}

func fundedService(t *testing.T, userID string) (*portfolio.Service, *portfolio.InMemoryRepository, *prices.MockPriceProvider) {
	t.Helper()
	quotes := prices.NewMockPriceProvider()
	repo := portfolio.NewInMemoryRepository()
	svc := portfolio.NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()
	_, err := svc.DepositCash(ctx, userID, "dep-"+userID, portfolio.CashFlowInput{Currency: "USD", Amount: money.AmountFromFloat64(10000)})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx, userID, "buy-"+userID, portfolio.BuyInput{Symbol: "AAPL", AssetType: portfolio.AssetTypeStock, Quantity: money.QuantityFromFloat64(10)})
	require.NoError(t, err)
	return svc, repo, quotes
}

func rankedIndex(t *testing.T, svc *portfolio.Service, repo *portfolio.InMemoryRepository, userID string) float64 {
	t.Helper()
	perf := performance.NewService(repo)
	perf.SetValuator(svc)
	rp, err := perf.CurrentRankedPerformance(context.Background(), userID)
	require.NoError(t, err)
	return rp.RankedIndex.Float64()
}

func fptr(f float64) *float64 { return &f }

func position(t *testing.T, svc *portfolio.Service, userID, symbol string) *portfolio.Position {
	t.Helper()
	positions, err := svc.ListPositions(context.Background(), userID)
	require.NoError(t, err)
	for _, p := range positions {
		if p.Symbol == symbol {
			return p
		}
	}
	return nil
}

func TestAutomaticSplitAppliedAndIdempotent(t *testing.T) {
	svc, repo, quotes := fundedService(t, "u1")
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})

	effective := time.Now().UTC().Add(time.Minute)
	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "SPLIT-AAPL-1", Type: corpactions.TypeSplit,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: effective, Effective: true,
		RatioNumerator: fptr(4), RatioDenominator: fptr(1),
	})

	before := position(t, svc, "u1", "AAPL")
	require.NotNil(t, before)
	beforeIdx := rankedIndex(t, svc, repo, "u1")

	require.NoError(t, pipeline.RunOnce(context.Background()))

	after := position(t, svc, "u1", "AAPL")
	require.NotNil(t, after)
	assert.InDelta(t, before.Quantity.Float64()*4, after.Quantity.Float64(), 1e-9, "quantity must quadruple")
	assert.InDelta(t, before.AverageBuyPrice.Float64()/4, after.AverageBuyPrice.Float64(), 1e-9, "basis must quarter")
	assert.InDelta(t, before.Quantity.Float64()*before.AverageBuyPrice.Float64(), after.Quantity.Float64()*after.AverageBuyPrice.Float64(), 1e-6, "total basis preserved")

	// A split-adjusted feed halves... quarters the quote; ranked index unchanged.
	quotes.Set("AAPL", before.AverageBuyPrice.Float64()/4, "USD")
	assert.InDelta(t, beforeIdx, rankedIndex(t, svc, repo, "u1"), 1e-6, "no phantom ranked gain")

	// Running again must be idempotent: no second application.
	require.NoError(t, pipeline.RunOnce(context.Background()))
	again := position(t, svc, "u1", "AAPL")
	assert.InDelta(t, after.Quantity.Float64(), again.Quantity.Float64(), 1e-9, "split must not apply twice")
}

func TestSymbolChangeAppliedAutomatically(t *testing.T) {
	svc, _, quotes := fundedService(t, "u1")
	quotes.Set("APLX", 195, "USD")
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})

	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "SYM-AAPL-APLX", Type: corpactions.TypeSymbolChange,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		Target:      &corpactions.InstrumentReference{Symbol: "APLX"},
		EffectiveAt: time.Now().UTC().Add(time.Minute), Effective: true,
	})
	require.NoError(t, pipeline.RunOnce(context.Background()))

	assert.Nil(t, position(t, svc, "u1", "AAPL"), "old ticker must be gone")
	newPos := position(t, svc, "u1", "APLX")
	require.NotNil(t, newPos, "position must be re-tickered")
	assert.InDelta(t, 10, newPos.Quantity.Float64(), 1e-9)

	views, err := pipeline.ListCorporateActionViews(context.Background(), "u1")
	require.NoError(t, err)
	require.NotEmpty(t, views)
	assert.Contains(t, views[0].Explanation, "changed ticker")
}

func TestIncompleteMergerStaysPending(t *testing.T) {
	svc, _, _ := fundedService(t, "u1")
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})

	// Stock merger missing the exchange ratio → incomplete, must not mutate.
	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "MERGER-AAPL", Type: corpactions.TypeStockMerger,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		Target:      &corpactions.InstrumentReference{Symbol: "NEWCO"},
		EffectiveAt: time.Now().UTC().Add(-time.Hour), Effective: true,
	})
	require.NoError(t, pipeline.RunOnce(context.Background()))

	pos := position(t, svc, "u1", "AAPL")
	require.NotNil(t, pos, "incomplete merger must leave the position visible")
	assert.InDelta(t, 10, pos.Quantity.Float64(), 1e-9)

	events, err := store.ListEventsByStatus(context.Background(), corpactions.StatusUnresolved)
	require.NoError(t, err)
	assert.Len(t, events, 1, "incomplete merger must be unresolved")
}

func TestDelistingDoesNotZeroPosition(t *testing.T) {
	svc, _, _ := fundedService(t, "u1")
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})

	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "DELIST-AAPL", Type: corpactions.TypeDelisting,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: time.Now().UTC().Add(-time.Hour), Effective: true,
	})
	require.NoError(t, pipeline.RunOnce(context.Background()))

	pos := position(t, svc, "u1", "AAPL")
	require.NotNil(t, pos, "delisting must never silently zero a position")
	assert.InDelta(t, 10, pos.Quantity.Float64(), 1e-9)
}

func TestBoughtAfterEffectiveIsSkipped(t *testing.T) {
	svc, _, _ := fundedService(t, "u1")
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})

	// Split effective BEFORE the user acquired the position (position CreatedAt is
	// "now"); they are not entitled to it.
	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "SPLIT-OLD", Type: corpactions.TypeSplit,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: time.Now().UTC().Add(-365 * 24 * time.Hour), Effective: true,
		RatioNumerator: fptr(2), RatioDenominator: fptr(1),
	})
	require.NoError(t, pipeline.RunOnce(context.Background()))
	pos := position(t, svc, "u1", "AAPL")
	assert.InDelta(t, 10, pos.Quantity.Float64(), 1e-9, "a split before acquisition must not apply")
}

func TestCancelledEventNotApplied(t *testing.T) {
	svc, _, _ := fundedService(t, "u1")
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})

	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "SPLIT-CANCELLED", Type: corpactions.TypeSplit,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: time.Now().UTC().Add(-time.Hour), Effective: true, Cancelled: true,
		RatioNumerator: fptr(2), RatioDenominator: fptr(1),
	})
	require.NoError(t, pipeline.RunOnce(context.Background()))
	assert.InDelta(t, 10, position(t, svc, "u1", "AAPL").Quantity.Float64(), 1e-9)
}

func TestWorkerAppliesToMultiplePortfolios(t *testing.T) {
	// One shared portfolio service holding two users, both holding AAPL.
	quotes := prices.NewMockPriceProvider()
	repo := portfolio.NewInMemoryRepository()
	svc := portfolio.NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()
	for _, u := range []string{"u1", "u2"} {
		_, err := svc.DepositCash(ctx, u, "d-"+u, portfolio.CashFlowInput{Currency: "USD", Amount: money.AmountFromFloat64(5000)})
		require.NoError(t, err)
		_, err = svc.BuyPosition(ctx, u, "b-"+u, portfolio.BuyInput{Symbol: "AAPL", AssetType: portfolio.AssetTypeStock, Quantity: money.QuantityFromFloat64(4)})
		require.NoError(t, err)
	}
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})
	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "SPLIT-MULTI", Type: corpactions.TypeSplit,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: time.Now().UTC().Add(time.Minute), Effective: true,
		RatioNumerator: fptr(2), RatioDenominator: fptr(1),
	})
	require.NoError(t, pipeline.RunOnce(ctx))
	for _, u := range []string{"u1", "u2"} {
		assert.InDelta(t, 8, position(t, svc, u, "AAPL").Quantity.Float64(), 1e-9, "each holder gets the split once")
	}
}

func TestProviderCorrectionDetected(t *testing.T) {
	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	svc, _, _ := fundedService(t, "u1")
	pipeline := corpactions.NewService(provider, store, testGateway{svc})

	base := corpactions.ProviderCorporateAction{
		ProviderEventID: "SPLIT-CORR", Type: corpactions.TypeSplit,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: time.Now().UTC().Add(-time.Hour), Effective: true,
		RatioNumerator: fptr(2), RatioDenominator: fptr(1),
	}
	provider.Seed(base)
	require.NoError(t, pipeline.Ingest(context.Background()))

	// The provider revises the ratio: an unapplied event's terms change.
	corrected := base
	corrected.RatioNumerator = fptr(3)
	provider.Seed(corrected)
	require.NoError(t, pipeline.Ingest(context.Background()))

	ev, ok, err := store.GetEvent(context.Background(), "manual_dev:SPLIT-CORR")
	require.NoError(t, err)
	require.True(t, ok)
	assert.NotNil(t, ev.RatioNumerator)
	assert.InDelta(t, 3, *ev.RatioNumerator, 1e-9, "correction must update the pending event terms")
}
