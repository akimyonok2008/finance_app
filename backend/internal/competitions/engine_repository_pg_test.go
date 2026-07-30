package competitions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// pgEngineEdition creates a definition+version+edition ready for entries.
func pgEngineEdition(t *testing.T, repo *PostgresCompetitionRepository, defs *PostgresDefinitionRepository) Competition {
	t.Helper()
	ctx := context.Background()
	def := uniqueDefinition()
	require.NoError(t, defs.CreateDefinition(ctx, def))
	require.NoError(t, defs.CreateDefinitionVersion(ctx, versionFor(def.ID, 1)))
	v1, err := defs.GetDefinitionVersion(ctx, def.ID, 1)
	require.NoError(t, err)
	snapshot, err := BuildRulesSnapshot(*v1)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	opens, closes := now.Add(-time.Hour), now.Add(time.Hour)
	edition := Competition{
		ID: "test_engine_" + uuid.NewString(), Name: "Engine Edition", Type: "engine",
		StartsAt: now.Add(-time.Minute), EndsAt: now.Add(7 * 24 * time.Hour), CreatedAt: now,
		DefinitionID: def.ID, DefinitionVersion: 1,
		JoinOpensAt: &opens, JoinClosesAt: &closes,
		LifecycleStatus: LifecycleRegistrationClosed, RulesSnapshotJSON: snapshot,
		ScoringScope: "full_portfolio",
	}
	require.NoError(t, repo.CreateEdition(ctx, edition))
	return edition
}

func pgEngineEntry(competitionID, userID string) CompetitionEntry {
	entryID := uuid.NewString()
	captured := time.Now().UTC()
	return CompetitionEntry{
		ID: entryID, CompetitionID: competitionID, UserID: userID,
		StartingValue: money.MustAmount("100"), StartingIndex: money.MustIndexValue("100"),
		JoinedAt: captured,
		Snapshots: []CompetitionEntrySnapshotPosition{{
			ID: uuid.NewString(), CompetitionEntryID: entryID,
			Symbol: "BTC", AssetType: "crypto", Quantity: money.QuantityFromFloat64(2),
			Currency: "USD", StartingPrice: money.PriceFromFloat64(40), StartingPriceCurrency: "USD",
			StartingValueBase: money.MustAmount("80"),
			InstrumentID:      uuid.NewString(), VenueMIC: "",
			ClassificationSnapshotJSON: json.RawMessage(`{"asset_type":"crypto"}`),
			IncludedInScore:            true,
		}, {
			ID: uuid.NewString(), CompetitionEntryID: entryID,
			Symbol: "AAPL", AssetType: "stock", Quantity: money.QuantityFromFloat64(1),
			Currency: "USD", StartingPrice: money.PriceFromFloat64(20), StartingPriceCurrency: "USD",
			StartingValueBase:          money.MustAmount("20"),
			ClassificationSnapshotJSON: json.RawMessage(`{"asset_type":"stock","venue_mic":"XNAS"}`),
			VenueMIC:                   "XNAS",
			IncludedInScore:            false,
		}},
		CashSnapshots: []CompetitionEntrySnapshotCash{{
			ID: uuid.NewString(), CompetitionEntryID: entryID,
			Currency: "USD", Amount: money.MustAmount("15"), ValueBase: money.MustAmount("15"),
			IncludedInScore: true,
		}},
		EntryStatus: EntryAdmitted, PortfolioVersion: 42, SnapshotCapturedAt: &captured,
		EligibilityEvidenceJSON:   json.RawMessage(`{"eligible":true,"rules":[]}`),
		ScoringScope:              "full_portfolio",
		EligibleStartingValueBase: money.MustAmount("95"),
		BaselineStatus:            BaselinePending,
	}
}

func TestPostgresEngineEntry_AtomicCreateBaselineAndGuards(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	userID := seedPGUser(t, pool)
	entry := pgEngineEntry(edition.ID, userID)
	require.NoError(t, repo.CreateEngineEntry(ctx, entry))

	// Duplicate (competition, user) is the idempotency backstop.
	dup := pgEngineEntry(edition.ID, userID)
	assert.ErrorIs(t, repo.CreateEngineEntry(ctx, dup), ErrEntryExists)

	// The edition shows up for baselining, entries loaded with engine fields.
	due, err := repo.ListBaselineDueEditions(ctx, time.Now().UTC())
	require.NoError(t, err)
	found := false
	for _, c := range due {
		if c.ID == edition.ID {
			found = true
		}
	}
	require.True(t, found)
	pending, err := repo.ListPendingBaselineEntries(ctx, edition.ID, 10)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Len(t, pending[0].Snapshots, 2)
	require.Len(t, pending[0].CashSnapshots, 1)
	var includedID string
	for _, s := range pending[0].Snapshots {
		if s.IncludedInScore {
			includedID = s.ID
		}
	}
	require.NotEmpty(t, includedID)

	// Official baseline lands atomically and is guarded against replays.
	now := time.Now().UTC()
	baselines := []PositionBaseline{{
		SnapshotID: includedID, Price: money.PriceFromFloat64(50), PriceCurrency: "USD",
		ValueBase: money.MustAmount("100"), Weight: money.MustRatio("1"), ObservedAt: now,
	}}
	require.NoError(t, repo.CompleteBaseline(ctx, entry.ID, money.MustAmount("115"), baselines, now))
	assert.ErrorIs(t, repo.CompleteBaseline(ctx, entry.ID, money.MustAmount("115"), baselines, now),
		ErrEntryConflict, "a completed baseline can never be rewritten")

	pending, err = repo.ListPendingBaselineEntries(ctx, edition.ID, 10)
	require.NoError(t, err)
	assert.Empty(t, pending)

	// Withdrawal is impossible once the entry is active.
	assert.ErrorIs(t, repo.UpdateEntryStatus(ctx, entry.ID, EntryAdmitted, EntryWithdrawn, now), ErrEntryConflict)
}

func TestPostgresEngineEntry_FailBaselineIsExplicitAndGuarded(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	edition := pgEngineEdition(t, repo, defs)
	userID := seedPGUser(t, pool)
	entry := pgEngineEntry(edition.ID, userID)
	require.NoError(t, repo.CreateEngineEntry(ctx, entry))

	require.NoError(t, repo.FailBaseline(ctx, entry.ID, "start price unavailable for BTC"))
	assert.ErrorIs(t, repo.FailBaseline(ctx, entry.ID, "again"), ErrEntryConflict)

	pending, err := repo.ListPendingBaselineEntries(ctx, edition.ID, 10)
	require.NoError(t, err)
	assert.Empty(t, pending, "a failed entry leaves the pending set explicitly, never silently")
}
