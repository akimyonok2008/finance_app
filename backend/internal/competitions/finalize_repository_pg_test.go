package competitions

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

func TestPostgresFinalization_ClaimValuePromoteAndReadBack(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	u1 := seedPGUser(t, pool)
	entry := pgActiveEntry(t, repo, edition.ID, u1, "AAA", "100")
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleRegistrationClosed, LifecycleActive, time.Now().UTC()))
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleActive, LifecycleFinalizing, time.Now().UTC()))

	gen, err := repo.EnsureBuildingFinalizationGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), gen.Generation)

	claimed, err := repo.ClaimEntriesForFinalization(ctx, edition.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	now := time.Now().UTC()
	require.NoError(t, repo.UpsertFinalizationRow(ctx, edition.ID, gen.Generation, CompetitionFinalizationRow{
		EntryID: entry.ID, UserID: u1, Index: money.MustIndexValue("130"), ReturnPct: money.MustRatio("30"), ValuedAt: now,
	}))
	require.NoError(t, repo.AdvanceFinalizationGeneration(ctx, edition.ID, gen.Generation, entry.ID, 1, 1, 0, false))
	require.NoError(t, repo.PromoteFinalizationGeneration(ctx, edition.ID, gen.Generation, now))

	comp, err := repo.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, LifecycleCompleted, comp.LifecycleStatus)
	require.NotNil(t, comp.FinalizedAt)

	read, err := repo.ListResults(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, read, 1)
	assert.Equal(t, 1, read[0].Rank)
	assert.Equal(t, 0, read[0].ReturnPercentage.Cmp(money.MustRatio("30")))

	var finalEntryStatus string
	require.NoError(t, pool.QueryRow(ctx, `SELECT entry_status FROM competition_entries WHERE id = $1`, entry.ID).Scan(&finalEntryStatus))
	assert.Equal(t, EntryFinalized, finalEntryStatus)

	// Idempotent replay on an already-completed competition must not
	// double-apply or error, regardless of which generation number is passed.
	require.NoError(t, repo.PromoteFinalizationGeneration(ctx, edition.ID, gen.Generation, now))
	replay, err := repo.ListResults(ctx, edition.ID)
	require.NoError(t, err)
	require.Len(t, replay, 1)
	assert.Equal(t, 0, replay[0].ReturnPercentage.Cmp(money.MustRatio("30")), "results are immutable once written")

	// A second, unrelated disqualification path is guarded too: the entry is
	// terminally finalized, never active.
	assert.ErrorIs(t, repo.DisqualifyEntry(ctx, entry.ID, "already finalized", now), ErrEntryConflict)
}

func TestPostgresFinalization_WriteFailureWithholdsPromotion(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	u1 := seedPGUser(t, pool)
	entry := pgActiveEntry(t, repo, edition.ID, u1, "AAA", "100")
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleRegistrationClosed, LifecycleActive, time.Now().UTC()))
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleActive, LifecycleFinalizing, time.Now().UTC()))

	gen, err := repo.EnsureBuildingFinalizationGeneration(ctx, edition.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertFinalizationRow(ctx, edition.ID, gen.Generation, CompetitionFinalizationRow{
		EntryID: entry.ID, UserID: u1, Index: money.MustIndexValue("110"), ReturnPct: money.MustRatio("10"), ValuedAt: now,
	}))
	require.NoError(t, repo.AdvanceFinalizationGeneration(ctx, edition.ID, gen.Generation, entry.ID, 1, 1, 0, true /* writeFailed */))

	assert.ErrorIs(t, repo.PromoteFinalizationGeneration(ctx, edition.ID, gen.Generation, now), ErrGenerationConflict)
	comp, err := repo.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, LifecycleFinalizing, comp.LifecycleStatus)

	require.NoError(t, repo.FailFinalizationGeneration(ctx, edition.ID, gen.Generation, "write failed", now))
	retry, err := repo.EnsureBuildingFinalizationGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.Greater(t, retry.Generation, gen.Generation, "the retry uses a clean generation")
}

func TestPostgresFinalization_RefusesWhenNotInFinalizing(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs) // still registration_closed
	gen, err := repo.EnsureBuildingFinalizationGeneration(ctx, edition.ID)
	require.NoError(t, err)
	err = repo.PromoteFinalizationGeneration(ctx, edition.ID, gen.Generation, time.Now().UTC())
	assert.ErrorIs(t, err, ErrLifecycleConflict)
}
