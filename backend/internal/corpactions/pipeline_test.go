package corpactions_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/instrument"
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
		out = append(out, corpactions.Holder{UserID: h.UserID, PortfolioID: h.PortfolioID, AcquiredAt: h.AcquiredAt, Symbol: h.Symbol})
	}
	return out, nil
}
func (g testGateway) HoldersOfInstrument(ctx context.Context, instrumentID string) ([]corpactions.Holder, error) {
	hs, err := g.svc.HoldersOfInstrument(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	out := make([]corpactions.Holder, 0, len(hs))
	for _, h := range hs {
		out = append(out, corpactions.Holder{UserID: h.UserID, PortfolioID: h.PortfolioID, AcquiredAt: h.AcquiredAt, Symbol: h.Symbol})
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

// TestIngest_MatchInstrumentResolvesSourceIdentity verifies that Ingest
// resolves ev.Source.InstrumentID via the wired resolver, and that an event
// still applies normally when it does (no regression to the existing
// symbol-only auto-apply path).
func TestIngest_MatchInstrumentResolvesSourceIdentity(t *testing.T) {
	svc, _, _ := fundedService(t, "u1")
	identities := instrument.NewInMemoryRepository()
	resolver := instrument.NewResolver(identities, nil)
	ctx := context.Background()
	in, err := identities.CreateInstrument(ctx, instrument.Instrument{
		CurrentSymbol: "AAPL", Status: instrument.StatusActive, IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	_, err = identities.CreateAlias(ctx, instrument.InstrumentAlias{
		InstrumentID: in.ID, AliasType: instrument.AliasTicker, AliasValue: "AAPL",
		ValidFrom: time.Now().UTC().Add(-time.Hour),
	})
	require.NoError(t, err)

	store := corpactions.NewInMemoryStore()
	provider := corpactions.NewManualDevelopmentProvider()
	pipeline := corpactions.NewService(provider, store, testGateway{svc})
	pipeline.SetInstrumentResolver(resolver)

	provider.Seed(corpactions.ProviderCorporateAction{
		ProviderEventID: "SPLIT-AAPL-MATCH", Type: corpactions.TypeSplit,
		Source:      corpactions.InstrumentReference{Symbol: "AAPL"},
		EffectiveAt: time.Now().UTC().Add(time.Minute), Effective: true,
		RatioNumerator: fptr(2), RatioDenominator: fptr(1),
	})
	require.NoError(t, pipeline.Ingest(ctx))

	ev, ok, err := store.GetEvent(ctx, "manual_dev:SPLIT-AAPL-MATCH")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, in.ID, ev.Source.InstrumentID, "Ingest must resolve the source instrument identity")

	// No regression: the event still applies normally.
	require.NoError(t, pipeline.Process(ctx))
	pos := position(t, svc, "u1", "AAPL")
	require.NotNil(t, pos)
	assert.InDelta(t, 20, pos.Quantity.Float64(), 1e-9)
}

// TestProcessEvent_UsesInstrumentBasedHolderDiscoveryOnSymbolDrift is the
// corporate-action identity-wiring regression: when a normalized event
// already carries a resolved Source.InstrumentID (see Service.
// matchInstrument, exercised separately in Ingest tests), processEvent must
// still find a holder whose position.Symbol no longer matches the event's
// symbol string — something HoldersOfSymbol alone cannot do. This isolates
// the holder-discovery step from Ingest's provider-fetch filtering by
// writing the normalized event directly into the store.
func TestProcessEvent_UsesInstrumentBasedHolderDiscoveryOnSymbolDrift(t *testing.T) {
	quotes := prices.NewMockPriceProvider()
	quotes.Set("OLDX", 100, "USD")
	repo := portfolio.NewInMemoryRepository()
	svc := portfolio.NewService(repo, quotes, fx.NewMockFXProvider())
	identities := instrument.NewInMemoryRepository()
	resolver := instrument.NewResolver(identities, nil)
	svc.SetInstrumentResolver(resolver)

	ctx := context.Background()
	in, err := identities.CreateInstrument(ctx, instrument.Instrument{
		CurrentSymbol: "OLDX", Status: instrument.StatusActive, IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	_, err = identities.CreateAlias(ctx, instrument.InstrumentAlias{
		InstrumentID: in.ID, AliasType: instrument.AliasTicker, AliasValue: "OLDX",
		ValidFrom: time.Now().UTC().Add(-time.Hour),
	})
	require.NoError(t, err)

	_, err = svc.DepositCash(ctx, "u1", "dep-u1", portfolio.CashFlowInput{Currency: "USD", Amount: money.AmountFromFloat64(10000)})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx, "u1", "buy-u1", portfolio.BuyInput{Symbol: "OLDX", AssetType: portfolio.AssetTypeStock, Quantity: money.QuantityFromFloat64(10)})
	require.NoError(t, err)
	before := position(t, svc, "u1", "OLDX")
	require.NotNil(t, before)
	require.Equal(t, in.ID, before.InstrumentID)

	store := corpactions.NewInMemoryStore()
	pipeline := corpactions.NewService(corpactions.NewManualDevelopmentProvider(), store, testGateway{svc})

	// A provider event under a DIFFERENT symbol string than the position's
	// stored symbol, but already resolved (by matchInstrument, upstream of
	// this test) to the SAME instrument identity as the held position.
	num, den := 2.0, 1.0
	_, err = store.UpsertEvent(ctx, corpactions.CorporateAction{
		ID: "manual_dev:SPLIT-DRIFT-1", Provider: "manual_dev", ProviderEventID: "SPLIT-DRIFT-1",
		Type:             corpactions.TypeSplit,
		Source:           corpactions.InstrumentReference{Symbol: "DRIFTEDX", InstrumentID: in.ID},
		EffectiveAt:      time.Now().UTC().Add(time.Minute),
		RatioNumerator:   &num,
		RatioDenominator: &den,
		Status:           corpactions.StatusValidated,
		Quality:          corpactions.QualityVerified,
		RawFingerprint:   "drift-fp-1",
	})
	require.NoError(t, err)

	require.NoError(t, pipeline.Process(ctx))

	after := position(t, svc, "u1", "OLDX")
	require.NotNil(t, after)
	assert.InDelta(t, before.Quantity.Float64()*2, after.Quantity.Float64(), 1e-9,
		"split must apply via instrument-based holder discovery despite the symbol string mismatch")
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
