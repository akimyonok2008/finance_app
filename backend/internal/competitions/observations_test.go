package competitions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// historicalMockProvider wraps prices.MockPriceProvider with a canned
// PriceAtOrBefore, so captureObservations' preference for historical pricing
// over a live quote can be exercised without a real Twelve Data client.
type historicalMockProvider struct {
	*prices.MockPriceProvider
	historical map[string]*prices.HistoricalPrice
}

func newHistoricalMockProvider() *historicalMockProvider {
	return &historicalMockProvider{
		MockPriceProvider: prices.NewMockPriceProvider(),
		historical:        map[string]*prices.HistoricalPrice{},
	}
}

func (h *historicalMockProvider) SetHistorical(symbol string, hp prices.HistoricalPrice) {
	h.historical[symbol] = &hp
}

func (h *historicalMockProvider) PriceAtOrBefore(_ context.Context, symbol string, _ time.Time) (*prices.HistoricalPrice, error) {
	hp, ok := h.historical[symbol]
	if !ok {
		return nil, prices.ErrPriceUnavailable
	}
	return hp, nil
}

var _ prices.HistoricalPriceProvider = (*historicalMockProvider)(nil)

// entryNeedingSymbol builds a minimal CompetitionEntry whose single included
// snapshot requires the given symbol, purely to drive captureObservations
// directly without going through the full baseline/finalize worker paths.
func entryNeedingSymbol(symbol string) CompetitionEntry {
	return CompetitionEntry{
		Snapshots: []CompetitionEntrySnapshotPosition{
			{Symbol: symbol, IncludedInScore: true},
		},
	}
}

// TestCaptureObservations_NeverSealsOffABatchThatIsntTheLastOne is the
// regression test for the bug where an observation set was marked
// complete/sealed as soon as ONE worker batch's own requested symbols were
// captured, regardless of whether more batches (with different symbols)
// remained for the same competition boundary. Two passes simulate two
// baseline batches for the same competition+boundary: the first captures
// everything it asked for but is NOT the last batch, so the set must stay
// pending; only the second pass, which both captures its symbol and is
// flagged as the last batch, may seal it.
func TestCaptureObservations_NeverSealsOffABatchThatIsntTheLastOne(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	now := time.Now().UTC()
	h.mp.Set("AAPL", 100, "USD")
	h.mp.Set("MSFT", 200, "USD")

	// Batch 1: only entry needing AAPL, NOT flagged as the last batch (more
	// entries — with potentially different symbols — remain to be swept).
	_, err := h.svc.captureObservations(ctx, h.repo, "comp-1", BoundaryStart, now, []CompetitionEntry{entryNeedingSymbol("AAPL")}, false, now)
	require.NoError(t, err)

	set, err := h.repo.EnsureObservationSet(ctx, "comp-1", BoundaryStart, now, observationProvider)
	require.NoError(t, err)
	assert.Equal(t, ObservationPending, set.Status,
		"a batch that fully captures its own symbols must not seal the set unless it is also the last batch")

	// Batch 2: a DIFFERENT symbol (MSFT), and this time it is the last batch.
	memo, err := h.svc.captureObservations(ctx, h.repo, "comp-1", BoundaryStart, now, []CompetitionEntry{entryNeedingSymbol("MSFT")}, true, now)
	require.NoError(t, err)

	set, err = h.repo.EnsureObservationSet(ctx, "comp-1", BoundaryStart, now, observationProvider)
	require.NoError(t, err)
	assert.Equal(t, ObservationSealed, set.Status)

	// Both batches' symbols must be visible in the returned view — sealing
	// never drops what earlier, non-final batches already captured.
	_, _, err = memo.priceOf(ctx, "", "AAPL")
	assert.NoError(t, err)
	_, _, err = memo.priceOf(ctx, "", "MSFT")
	assert.NoError(t, err)
}

// TestCaptureObservations_SealedSetNeverAcceptsANewSymbol guards requirement
// 5 of the fix: once a set is sealed, a later call that asks for a symbol
// never seen before (e.g. a basket adjustment surfacing a late corporate
// action) must not silently fetch and append it — the set is frozen.
func TestCaptureObservations_SealedSetNeverAcceptsANewSymbol(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	now := time.Now().UTC()
	h.mp.Set("AAPL", 100, "USD")
	h.mp.Set("NVDA", 300, "USD")

	_, err := h.svc.captureObservations(ctx, h.repo, "comp-2", BoundaryEnd, now, []CompetitionEntry{entryNeedingSymbol("AAPL")}, true, now)
	require.NoError(t, err)
	set, err := h.repo.EnsureObservationSet(ctx, "comp-2", BoundaryEnd, now, observationProvider)
	require.NoError(t, err)
	require.Equal(t, ObservationSealed, set.Status)

	memo, err := h.svc.captureObservations(ctx, h.repo, "comp-2", BoundaryEnd, now, []CompetitionEntry{entryNeedingSymbol("NVDA")}, true, now)
	require.NoError(t, err)

	_, _, err = memo.priceOf(ctx, "", "NVDA")
	assert.ErrorIs(t, err, errObservationNotCaptured,
		"a sealed set must never fetch a symbol it wasn't sealed with")
}

// TestCaptureObservations_PrefersHistoricalPriceOverLiveQuote is the
// regression test for the "boundary prices aren't true boundary prices" gap:
// when the wired price provider can resolve a price at-or-before the
// competition's declared boundary instant, captureObservations must use
// that — tagged prices.MethodologySessionClose — rather than whatever the
// live provider happens to be quoting at the moment the worker pass runs.
func TestCaptureObservations_PrefersHistoricalPriceOverLiveQuote(t *testing.T) {
	repo := NewInMemoryCompetitionRepository()
	hp := newHistoricalMockProvider()
	hp.Set("AAPL", 999, "USD") // the live quote at capture time — must NOT be used
	sessionClose := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	hp.SetHistorical("AAPL", prices.HistoricalPrice{
		Symbol: "AAPL", Price: 150, Currency: "USD",
		ProviderTimestamp: sessionClose, TradingSessionDate: "2026-01-05",
		Methodology: prices.MethodologySessionClose,
	})
	svc := NewService(repo, fakeUsers{}, &fakePositions{}, hp, fx.NewMockFXProvider(), &clock.FixedClock{Time: fixedTime})

	effectiveAt := time.Date(2026, 1, 5, 15, 30, 0, 0, time.UTC) // mid-session, well after the close used above
	now := time.Now().UTC()
	memo, err := svc.captureObservations(context.Background(), repo, "comp-hist", BoundaryStart, effectiveAt,
		[]CompetitionEntry{entryNeedingSymbol("AAPL")}, true, now)
	require.NoError(t, err)

	price, currency, err := memo.priceOf(context.Background(), "", "AAPL")
	require.NoError(t, err)
	assert.Equal(t, "USD", currency)
	assert.Equal(t, 0, price.Cmp(money.MustPrice("150")),
		"must use the historical session-close price, not the live quote")

	stored := memo.prices[observationKey("", "AAPL")]
	assert.Equal(t, prices.MethodologySessionClose, stored.Methodology)
	assert.Equal(t, "2026-01-05", stored.TradingSessionDate)
	assert.True(t, stored.ProviderTimestamp.Equal(sessionClose))
}

// TestCaptureObservations_FallsBackToLiveQuoteWhenHistoricalUnavailable
// covers the other half: a provider that implements HistoricalPriceProvider
// but fails for this particular symbol (outage, no coverage, predates
// history) must still fall back to GetLatestPrice — tagged
// prices.MethodologyFallbackLatest so the imprecision is auditable — rather
// than leaving the symbol uncaptured when a live quote was available.
func TestCaptureObservations_FallsBackToLiveQuoteWhenHistoricalUnavailable(t *testing.T) {
	repo := NewInMemoryCompetitionRepository()
	hp := newHistoricalMockProvider()
	hp.Set("MSFT", 400, "USD") // no SetHistorical call: historical lookup misses
	svc := NewService(repo, fakeUsers{}, &fakePositions{}, hp, fx.NewMockFXProvider(), &clock.FixedClock{Time: fixedTime})

	now := time.Now().UTC()
	memo, err := svc.captureObservations(context.Background(), repo, "comp-hist-2", BoundaryStart, now,
		[]CompetitionEntry{entryNeedingSymbol("MSFT")}, true, now)
	require.NoError(t, err)

	price, _, err := memo.priceOf(context.Background(), "", "MSFT")
	require.NoError(t, err)
	assert.Equal(t, 0, price.Cmp(money.MustPrice("400")))

	stored := memo.prices[observationKey("", "MSFT")]
	assert.Equal(t, prices.MethodologyFallbackLatest, stored.Methodology)
}
