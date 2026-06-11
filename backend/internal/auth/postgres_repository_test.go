package auth

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/db"
)

// testPool connects to the integration-test database, applying migrations.
// Tests are skipped when DATABASE_URL_TEST is unset so the suite stays green
// without local infrastructure.
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

func newPGUser(name string) *User {
	id := uuid.NewString()
	return &User{
		ID:           id,
		Email:        id + "@example.com", // unique per test run
		DisplayName:  name,
		AvatarKey:    "fox",
		PasswordHash: "bcrypt-hash",
	}
}

func TestPostgresUserRepository_CreateAndFind(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	require.NoError(t, repo.Create(u))

	byEmail, err := repo.FindByEmail(u.Email)
	require.NoError(t, err)
	assert.Equal(t, u.ID, byEmail.ID)
	assert.Equal(t, "Alpha", byEmail.DisplayName)

	byID, err := repo.FindByID(u.ID)
	require.NoError(t, err)
	assert.Equal(t, u.Email, byID.Email)
}

func TestPostgresUserRepository_DuplicateEmailFails(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	require.NoError(t, repo.Create(u))

	dup := newPGUser("Beta")
	dup.Email = u.Email
	assert.ErrorIs(t, repo.Create(dup), ErrEmailExists)
}

func TestPostgresUserRepository_FindMissingReturnsNotFound(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	_, err := repo.FindByID(uuid.NewString())
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, err = repo.FindByEmail("nobody-" + uuid.NewString() + "@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)
}

func TestPostgresUserRepository_ListUsersIncludesCreated(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	require.NoError(t, repo.Create(u))

	users, err := repo.ListUsers(context.Background())
	require.NoError(t, err)
	found := false
	for _, x := range users {
		if x.ID == u.ID {
			found = true
		}
	}
	assert.True(t, found)
}
