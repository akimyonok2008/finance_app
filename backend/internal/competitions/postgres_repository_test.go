package competitions

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/db"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres integration test")
	}
	pool, err := db.ConnectPostgres(context.Background(), url)
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(context.Background(), pool))
	t.Cleanup(pool.Close)
	return pool
}

func seedPGUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'h', 'Test')`,
		id, id+"@example.com")
	require.NoError(t, err)
	return id
}

// uniqueSprint avoids id collisions across test runs against a shared database.
func uniqueSprint() Competition {
	now := time.Now().UTC()
	return Competition{
		ID: "test_" + uuid.NewString(), Name: "Test Sprint", Type: TypeWeeklySprint,
		StartsAt: now, EndsAt: now.Add(7 * 24 * time.Hour), Status: StatusActive, CreatedAt: now,
	}
}

func TestPostgresCompetitionRepository_CreateGetList(t *testing.T) {
	repo := NewPostgresCompetitionRepository(testPool(t))
	ctx := context.Background()
	c := uniqueSprint()
	require.NoError(t, repo.CreateCompetition(ctx, c))
	// Creating again is idempotent (ON CONFLICT DO NOTHING).
	require.NoError(t, repo.CreateCompetition(ctx, c))

	got, err := repo.GetCompetition(ctx, c.ID)
	require.NoError(t, err)
	assert.Equal(t, c.Name, got.Name)

	list, err := repo.ListCompetitions(ctx)
	require.NoError(t, err)
	found := false
	for _, x := range list {
		if x.ID == c.ID {
			found = true
		}
	}
	assert.True(t, found)
}

func TestPostgresCompetitionRepository_GetMissing(t *testing.T) {
	repo := NewPostgresCompetitionRepository(testPool(t))
	_, err := repo.GetCompetition(context.Background(), "missing_"+uuid.NewString())
	assert.ErrorIs(t, err, ErrCompetitionNotFound)
}

func TestPostgresCompetitionRepository_EntryWithSnapshots(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	ctx := context.Background()
	c := uniqueSprint()
	require.NoError(t, repo.CreateCompetition(ctx, c))
	userID := seedPGUser(t, pool)

	entryID := uuid.NewString()
	entry := CompetitionEntry{
		ID: entryID, CompetitionID: c.ID, UserID: userID,
		StartingValue: 1950, StartingIndex: 100, JoinedAt: time.Now().UTC(),
		Snapshots: []CompetitionEntrySnapshotPosition{{
			ID: uuid.NewString(), CompetitionEntryID: entryID,
			Symbol: "AAPL", AssetType: "stock", Quantity: 10, Currency: "USD",
			StartingPrice: 195, StartingPriceCurrency: "USD", StartingValueBase: 1950,
		}},
	}
	require.NoError(t, repo.CreateEntry(ctx, entry))

	// Duplicate entry for the same user is silently idempotent.
	dup := entry
	dup.ID = uuid.NewString()
	require.NoError(t, repo.CreateEntry(ctx, dup))

	got, err := repo.GetEntry(ctx, c.ID, userID)
	require.NoError(t, err)
	assert.Equal(t, 1950.0, got.StartingValue)
	require.Len(t, got.Snapshots, 1)
	assert.Equal(t, "AAPL", got.Snapshots[0].Symbol)
	assert.Equal(t, 195.0, got.Snapshots[0].StartingPrice)

	entries, err := repo.ListEntries(ctx, c.ID)
	require.NoError(t, err)
	require.Len(t, entries, 1, "duplicate join must not create a second entry")
	require.Len(t, entries[0].Snapshots, 1, "ListEntries must load snapshots")
}

func TestPostgresCompetitionRepository_GetEntryMissing(t *testing.T) {
	repo := NewPostgresCompetitionRepository(testPool(t))
	_, err := repo.GetEntry(context.Background(), "missing_"+uuid.NewString(), uuid.NewString())
	assert.ErrorIs(t, err, ErrEntryNotFound)
}
