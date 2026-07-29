package portfolio_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/instrument"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// buyWithoutResolver simulates a legacy position: bought with no instrument
// resolver wired, so it lands with instrument_id NULL exactly like data
// written before this identity layer existed.
func buyWithoutResolver(t *testing.T, symbol string) (*portfolio.Service, *portfolio.InMemoryRepository) {
	t.Helper()
	quotes := prices.NewMockPriceProvider()
	quotes.Set(symbol, 100, "USD")
	repo := portfolio.NewInMemoryRepository()
	svc := portfolio.NewService(repo, quotes, fx.NewMockFXProvider())
	ctx := context.Background()
	_, err := svc.DepositCash(ctx, "u1", "dep-1", portfolio.CashFlowInput{Currency: "USD", Amount: money.AmountFromFloat64(10000)})
	require.NoError(t, err)
	_, err = svc.BuyPosition(ctx, "u1", "buy-1", portfolio.BuyInput{Symbol: symbol, AssetType: portfolio.AssetTypeStock, Quantity: money.QuantityFromFloat64(1)})
	require.NoError(t, err)
	return svc, repo
}

func TestBackfillJob_ResolvesUnambiguousLegacyTicker(t *testing.T) {
	svc, repo := buyWithoutResolver(t, "LEGACY1")
	ctx := context.Background()

	positions, err := svc.ListPositions(ctx, "u1")
	require.NoError(t, err)
	require.Len(t, positions, 1)
	require.Empty(t, positions[0].InstrumentID, "legacy position must start unresolved")

	identities := instrument.NewInMemoryRepository()
	in, err := identities.CreateInstrument(ctx, instrument.Instrument{
		CurrentSymbol: "LEGACY1", Status: instrument.StatusActive, IdentityQuality: instrument.QualityResolved,
	})
	require.NoError(t, err)
	_, err = identities.CreateAlias(ctx, instrument.InstrumentAlias{
		InstrumentID: in.ID, AliasType: instrument.AliasTicker, AliasValue: "LEGACY1",
		ValidFrom: time.Now().UTC().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	resolver := instrument.NewResolver(identities, nil)

	job := portfolio.NewBackfillJob(repo, resolver)
	summary, err := job.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.PositionsResolved)
	assert.Equal(t, 0, summary.PositionsQueued)

	after, err := svc.ListPositions(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, in.ID, after[0].InstrumentID)

	pending, err := repo.ListPendingReconciliation(ctx, 0)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestBackfillJob_QueuesAmbiguousAndUnresolvedTickers(t *testing.T) {
	svc, repo := buyWithoutResolver(t, "AMBIG1")
	ctx := context.Background()

	identities := instrument.NewInMemoryRepository()
	// Two DIFFERENT instruments both claim the ticker "AMBIG1" on different
	// exchanges — a ticker-only lookup cannot disambiguate.
	inA, err := identities.CreateInstrument(ctx, instrument.Instrument{CurrentSymbol: "AMBIG1", Status: instrument.StatusActive})
	require.NoError(t, err)
	inB, err := identities.CreateInstrument(ctx, instrument.Instrument{CurrentSymbol: "AMBIG1", Status: instrument.StatusActive})
	require.NoError(t, err)
	_, err = identities.CreateAlias(ctx, instrument.InstrumentAlias{
		InstrumentID: inA.ID, AliasType: instrument.AliasTicker, AliasValue: "AMBIG1", ExchangeCode: "US",
		ValidFrom: time.Now().UTC().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	_, err = identities.CreateAlias(ctx, instrument.InstrumentAlias{
		InstrumentID: inB.ID, AliasType: instrument.AliasTicker, AliasValue: "AMBIG1", ExchangeCode: "LN",
		ValidFrom: time.Now().UTC().Add(-24 * time.Hour),
	})
	require.NoError(t, err)
	resolver := instrument.NewResolver(identities, nil)

	job := portfolio.NewBackfillJob(repo, resolver)
	summary, err := job.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, summary.PositionsResolved)
	assert.Equal(t, 1, summary.PositionsQueued)
	assert.Equal(t, 1, summary.ActivitiesQueued, "the buy activity carries the same ambiguous ticker")

	after, err := svc.ListPositions(ctx, "u1")
	require.NoError(t, err)
	assert.Empty(t, after[0].InstrumentID, "an ambiguous match must never be guessed at")

	// Both the position and its buy activity are queued for review.
	pending, err := repo.ListPendingReconciliation(ctx, 0)
	require.NoError(t, err)
	require.Len(t, pending, 2)
	for _, item := range pending {
		assert.Equal(t, "AMBIG1", item.Symbol)
		assert.Equal(t, portfolio.ReconciliationConfidenceLow, item.Confidence)
	}

	// Re-running the job must not duplicate either queue entry.
	_, err = job.Run(ctx)
	require.NoError(t, err)
	pending, err = repo.ListPendingReconciliation(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, pending, 2)
}

func TestBackfillJob_AdminResolvesQueuedItem(t *testing.T) {
	svc, repo := buyWithoutResolver(t, "REVIEW1")
	ctx := context.Background()

	identities := instrument.NewInMemoryRepository()
	resolver := instrument.NewResolver(identities, nil) // no alias exists: unresolved
	job := portfolio.NewBackfillJob(repo, resolver)
	summary, err := job.Run(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.PositionsQueued)

	pending, err := repo.ListPendingReconciliation(ctx, 0)
	require.NoError(t, err)
	require.Len(t, pending, 2) // the position row and its buy activity

	var positionItem portfolio.ReconciliationItem
	for _, item := range pending {
		if item.TableName == "positions" {
			positionItem = item
		}
	}
	require.NotEmpty(t, positionItem.ID, "expected a queued positions row")

	in, err := identities.CreateInstrument(ctx, instrument.Instrument{CurrentSymbol: "REVIEW1", Status: instrument.StatusActive})
	require.NoError(t, err)
	require.NoError(t, repo.ResolveReconciliation(ctx, positionItem.ID, in.ID, "admin-1"))

	after, err := svc.ListPositions(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, in.ID, after[0].InstrumentID)

	stillPending, err := repo.ListPendingReconciliation(ctx, 0)
	require.NoError(t, err)
	assert.Len(t, stillPending, 1, "only the activity row remains queued")

	// Resolving an already-resolved item must fail cleanly.
	err = repo.ResolveReconciliation(ctx, positionItem.ID, in.ID, "admin-1")
	assert.ErrorIs(t, err, portfolio.ErrReconciliationItemNotFound)
}
