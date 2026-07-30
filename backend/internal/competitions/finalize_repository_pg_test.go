package competitions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func TestPostgresFinalization_AtomicWriteAndIdempotentReplay(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	u1 := seedPGUser(t, pool)
	entry := pgActiveEntry(t, repo, edition.ID, u1, "AAA", "100")
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleRegistrationClosed, LifecycleActive, time.Now().UTC()))
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleActive, LifecycleFinalizing, time.Now().UTC()))

	entries, err := repo.ListActiveEntriesForFinalization(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	now := time.Now().UTC()
	results := []CompetitionResultRow{{
		EntryID: entry.ID, UserID: u1, Rank: 1,
		Index: money.MustIndexValue("130"), ReturnPct: money.MustRatio("30"),
	}}
	require.NoError(t, repo.FinalizeResults(ctx, edition.ID, results, now))

	comp, err := repo.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, LifecycleCompleted, comp.LifecycleStatus)
	require.NotNil(t, comp.FinalizedAt)

	read, err := repo.ListResults(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, read, 1)
	assert.Equal(t, 1, read[0].Rank)
	assert.Equal(t, 0, read[0].ReturnPercentage.Cmp(money.MustRatio("30")))

	// Idempotent replay with DIFFERENT values must not overwrite the
	// immutable rows: the competition is already completed.
	require.NoError(t, repo.FinalizeResults(ctx, edition.ID, []CompetitionResultRow{{
		EntryID: entry.ID, UserID: u1, Rank: 1, Index: money.MustIndexValue("999"), ReturnPct: money.MustRatio("999"),
	}}, now))
	replay, err := repo.ListResults(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, replay, 1)
	assert.Equal(t, 0, replay[0].ReturnPercentage.Cmp(money.MustRatio("30")), "results are immutable once written")

	// A second, unrelated user's disqualification path is guarded too.
	assert.ErrorIs(t, repo.DisqualifyEntry(ctx, entry.ID, "already finalized", now), ErrEntryConflict)
}

func TestPostgresFinalization_RefusesWhenNotInFinalizing(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs) // still registration_closed
	err := repo.FinalizeResults(ctx, edition.ID, nil, time.Now().UTC())
	assert.ErrorIs(t, err, ErrLifecycleConflict)
}
