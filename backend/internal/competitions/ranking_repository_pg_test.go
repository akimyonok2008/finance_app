package competitions

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// pgActiveEntry inserts an EntryActive engine entry directly (bypassing
// join/baseline), ready to be claimed and valued by the ranking pipeline.
func pgActiveEntry(t *testing.T, repo *PostgresCompetitionRepository, competitionID, userID, symbol, eligibleStartingValue string) CompetitionEntry {
	t.Helper()
	entryID := uuid.NewString()
	entry := CompetitionEntry{
		ID: entryID, CompetitionID: competitionID, UserID: userID,
		StartingValue: money.MustAmount(eligibleStartingValue), StartingIndex: money.MustIndexValue("100"),
		JoinedAt: time.Now().UTC(),
		Snapshots: []CompetitionEntrySnapshotPosition{{
			ID: uuid.NewString(), CompetitionEntryID: entryID,
			Symbol: symbol, AssetType: "stock", Quantity: money.QuantityFromFloat64(1),
			Currency: "USD", StartingPrice: money.PriceFromFloat64(100), StartingPriceCurrency: "USD",
			StartingValueBase: money.MustAmount(eligibleStartingValue), IncludedInScore: true,
		}},
		EntryStatus: EntryActive, PortfolioVersion: 1,
		ScoringScope: "full_portfolio", BaselineStatus: BaselineCompleted,
		EligibleStartingValueBase: money.MustAmount(eligibleStartingValue),
	}
	require.NoError(t, repo.CreateEngineEntry(context.Background(), entry))
	return entry
}

func TestPostgresRanking_ClaimValuePromoteAndReadBack(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	u1, u2 := seedPGUser(t, pool), seedPGUser(t, pool)
	pgActiveEntry(t, repo, edition.ID, u1, "AAA", "100")
	pgActiveEntry(t, repo, edition.ID, u2, "BBB", "100")

	gen, err := repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), gen.Generation)

	// Idempotent: calling it again while still building returns the same row.
	again, err := repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, gen.Generation, again.Generation)

	claimed, err := repo.ClaimActiveEntries(ctx, edition.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, claimed, 2)

	now := time.Now().UTC()
	for _, e := range claimed {
		idx, retPct := money.MustIndexValue("120"), money.MustRatio("20")
		if e.UserID == u2 {
			idx, retPct = money.MustIndexValue("150"), money.MustRatio("50")
		}
		require.NoError(t, repo.UpsertRanking(ctx, edition.ID, gen.Generation, CompetitionRankingRow{
			EntryID: e.ID, UserID: e.UserID, Index: idx, ReturnPct: retPct, ValuedAt: now,
		}))
	}
	require.NoError(t, repo.AdvanceGeneration(ctx, edition.ID, gen.Generation, claimed[len(claimed)-1].ID, len(claimed), len(claimed), 0, false))
	require.NoError(t, repo.PromoteGeneration(ctx, edition.ID, gen.Generation, now))

	// A second promote attempt on the same (now non-building) generation must
	// fail rather than double-apply.
	assert.ErrorIs(t, repo.PromoteGeneration(ctx, edition.ID, gen.Generation, now), ErrGenerationConflict)

	activeGen, activatedAt, found, err := repo.ActiveGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, gen.Generation, activeGen)
	assert.WithinDuration(t, now, activatedAt, 5*time.Second)

	page, err := repo.LeaderboardPage(ctx, edition.ID, 0, 10)
	require.NoError(t, err)
	require.Len(t, page, 2)
	assert.Equal(t, 1, page[0].Rank)
	assert.Equal(t, 0, page[0].ReturnPercentage.Cmp(money.MustRatio("50")), "u2's +50% return ranks first")
	assert.Equal(t, 2, page[1].Rank)

	row, found, err := repo.UserRankRow(ctx, edition.ID, u1)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 2, row.Rank)

	// The next tick starts a fresh building generation on top of the now-
	// active one, never re-touching the promoted rows.
	next, err := repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, gen.Generation+1, next.Generation)
}

func TestPostgresRanking_PromotionSupersedesAndPrunesPreviousGeneration(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	u1 := seedPGUser(t, pool)
	entry := pgActiveEntry(t, repo, edition.ID, u1, "AAA", "100")

	gen1, err := repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertRanking(ctx, edition.ID, gen1.Generation, CompetitionRankingRow{
		EntryID: entry.ID, UserID: u1, Index: money.MustIndexValue("110"), ReturnPct: money.MustRatio("10"), ValuedAt: now,
	}))
	require.NoError(t, repo.AdvanceGeneration(ctx, edition.ID, gen1.Generation, entry.ID, 1, 1, 0, false))
	require.NoError(t, repo.PromoteGeneration(ctx, edition.ID, gen1.Generation, now))

	gen2, err := repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	require.Equal(t, gen1.Generation+1, gen2.Generation)
	require.NoError(t, repo.UpsertRanking(ctx, edition.ID, gen2.Generation, CompetitionRankingRow{
		EntryID: entry.ID, UserID: u1, Index: money.MustIndexValue("130"), ReturnPct: money.MustRatio("30"), ValuedAt: now,
	}))
	require.NoError(t, repo.AdvanceGeneration(ctx, edition.ID, gen2.Generation, entry.ID, 1, 1, 0, false))
	require.NoError(t, repo.PromoteGeneration(ctx, edition.ID, gen2.Generation, now))

	// The active generation must remain readable throughout — this reads the
	// NEW one and never observes a gap.
	activeGen, _, found, err := repo.ActiveGeneration(ctx, edition.ID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, gen2.Generation, activeGen)

	row, found, err := repo.UserRankRow(ctx, edition.ID, u1)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 0, row.ReturnPct.Cmp(money.MustRatio("30")), "reads only ever see the latest promoted generation")

	// The superseded generation's rows are pruned (bounded storage growth).
	var remaining int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM competition_rankings WHERE competition_id = $1 AND generation = $2
	`, edition.ID, gen1.Generation).Scan(&remaining))
	assert.Zero(t, remaining, "the previous generation's rows must be pruned once superseded")
}

func TestPostgresRanking_WriteFailureWithholdsPromotion(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	u1 := seedPGUser(t, pool)
	entry := pgActiveEntry(t, repo, edition.ID, u1, "AAA", "100")

	gen, err := repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, repo.UpsertRanking(ctx, edition.ID, gen.Generation, CompetitionRankingRow{
		EntryID: entry.ID, UserID: u1, Index: money.MustIndexValue("110"), ReturnPct: money.MustRatio("10"), ValuedAt: now,
	}))
	require.NoError(t, repo.AdvanceGeneration(ctx, edition.ID, gen.Generation, entry.ID, 1, 1, 0, true /* writeFailed */))

	assert.ErrorIs(t, repo.PromoteGeneration(ctx, edition.ID, gen.Generation, now), ErrGenerationConflict)
	_, _, found, err := repo.ActiveGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.False(t, found)

	retry, err := repo.EnsureBuildingGeneration(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, gen.Generation, retry.Generation, "the same generation is retried, not abandoned")
}
