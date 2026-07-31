package competitions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// TestPostgresObservationRepository_EnsureIsIdempotentAndCapturesIncrementally
// exercises migration 0050 directly: EnsureObservationSet must return the
// same row (same effective_at/provider) across calls for the same
// competition+boundary, RecordObservations must be idempotent per
// symbol/pair, and the set must flip to completed only once allCaptured is
// signalled true.
func TestPostgresObservationRepository_EnsureIsIdempotentAndCapturesIncrementally(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	effectiveAt := edition.StartsAt

	first, err := repo.EnsureObservationSet(ctx, edition.ID, BoundaryStart, effectiveAt, "live_quote")
	require.NoError(t, err)
	assert.Equal(t, ObservationPending, first.Status)
	assert.WithinDuration(t, effectiveAt, first.EffectiveAt, time.Second)

	// A second Ensure call (a later worker pass) must return the identical
	// set, not create a competing one — the (competition_id, boundary_type)
	// unique constraint is what makes this safe under concurrent workers.
	again, err := repo.EnsureObservationSet(ctx, edition.ID, BoundaryStart, effectiveAt.Add(5*time.Minute), "live_quote")
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID)
	assert.WithinDuration(t, effectiveAt, again.EffectiveAt, time.Second,
		"effective_at must not be overwritten by a later pass")

	now := time.Now().UTC()
	err = repo.RecordObservations(ctx, first.ID, []PriceObservation{
		{Symbol: "BTC-ID", Price: money.PriceFromFloat64(50), Currency: "USD", ObservedAt: now},
	}, nil, false, now)
	require.NoError(t, err)

	prices, fxRates, err := repo.LoadObservations(ctx, first.ID)
	require.NoError(t, err)
	require.Contains(t, prices, "BTC-ID")
	assert.Equal(t, 0, prices["BTC-ID"].Price.Cmp(money.MustPrice("50")))
	assert.Empty(t, fxRates)

	// Recording the same symbol again must never overwrite the frozen value —
	// this is the property the whole fix depends on.
	err = repo.RecordObservations(ctx, first.ID, []PriceObservation{
		{Symbol: "BTC-ID", Price: money.PriceFromFloat64(999), Currency: "USD", ObservedAt: now},
	}, nil, true, now)
	require.NoError(t, err)
	prices, _, err = repo.LoadObservations(ctx, first.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, prices["BTC-ID"].Price.Cmp(money.MustPrice("50")),
		"an already-captured observation must never be overwritten by a later pass")

	completed, err := repo.EnsureObservationSet(ctx, edition.ID, BoundaryStart, effectiveAt, "live_quote")
	require.NoError(t, err)
	assert.Equal(t, ObservationCompleted, completed.Status)
	require.NotNil(t, completed.CompletedAt)
}
