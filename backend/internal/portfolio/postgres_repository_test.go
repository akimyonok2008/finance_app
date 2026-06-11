package portfolio

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

// seedUser inserts a user row directly (positions/portfolios have FK to users).
func seedUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	id := uuid.NewString()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, display_name) VALUES ($1, $2, 'h', 'Test')`,
		id, id+"@example.com")
	require.NoError(t, err)
	return id
}

func seedPortfolio(t *testing.T, repo *PostgresRepository, userID string) *Portfolio {
	t.Helper()
	now := time.Now().UTC()
	p := &Portfolio{ID: uuid.NewString(), UserID: userID, Name: DefaultPortfolioName, Currency: "USD", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreatePortfolio(p))
	return p
}

func newPGPosition(userID, portfolioID string) *Position {
	now := time.Now().UTC()
	return &Position{
		ID: uuid.NewString(), UserID: userID, PortfolioID: portfolioID,
		Symbol: "AAPL", AssetType: "stock", Quantity: 10, AverageBuyPrice: 180,
		Currency: "USD", CreatedAt: now, UpdatedAt: now,
	}
}

func TestPostgresRepository_PortfolioCreateAndGet(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	userID := seedUser(t, pool)

	created := seedPortfolio(t, repo, userID)
	got, err := repo.GetPortfolioByUser(userID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, got.ID)
	assert.Equal(t, DefaultPortfolioName, got.Name)
}

func TestPostgresRepository_GetPortfolioMissing(t *testing.T) {
	repo := NewPostgresRepository(testPool(t))
	_, err := repo.GetPortfolioByUser(uuid.NewString())
	assert.ErrorIs(t, err, ErrPortfolioNotFound)
}

func TestPostgresRepository_PositionCRUDAndIsolation(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	user1 := seedUser(t, pool)
	user2 := seedUser(t, pool)
	pf1 := seedPortfolio(t, repo, user1)
	pf2 := seedPortfolio(t, repo, user2)

	pos := newPGPosition(user1, pf1.ID)
	require.NoError(t, repo.CreatePosition(pos))
	other := newPGPosition(user2, pf2.ID)
	other.Symbol = "MSFT"
	require.NoError(t, repo.CreatePosition(other))

	// Get by id, float fields survive the NUMERIC round-trip.
	got, err := repo.GetPosition(pos.ID)
	require.NoError(t, err)
	assert.Equal(t, "AAPL", got.Symbol)
	assert.Equal(t, 10.0, got.Quantity)
	assert.Equal(t, 180.0, got.AverageBuyPrice)

	// User isolation in listing.
	list1, err := repo.ListPositionsByUser(user1)
	require.NoError(t, err)
	require.Len(t, list1, 1)
	assert.Equal(t, "AAPL", list1[0].Symbol)

	// Update.
	got.Quantity = 12
	got.AverageBuyPrice = 175
	require.NoError(t, repo.UpdatePosition(got))
	updated, err := repo.GetPosition(pos.ID)
	require.NoError(t, err)
	assert.Equal(t, 12.0, updated.Quantity)

	// Delete.
	require.NoError(t, repo.DeletePosition(pos.ID))
	_, err = repo.GetPosition(pos.ID)
	assert.ErrorIs(t, err, ErrPositionNotFound)
	assert.ErrorIs(t, repo.DeletePosition(pos.ID), ErrPositionNotFound)
}
