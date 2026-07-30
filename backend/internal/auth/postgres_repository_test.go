package auth

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

// TestPostgresUserRepository_ListUsersPageWalksWholePopulation proves the
// keyset page query behind ListRankableUsersPage: starting from the empty
// cursor (mapped to the nil UUID), successive pages are strictly ascending by
// id, terminate, and collectively cover every non-deleted user exactly once.
func TestPostgresUserRepository_ListUsersPageWalksWholePopulation(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	created := map[string]bool{}
	for i := 0; i < 5; i++ {
		u := newPGUser("Paged")
		require.NoError(t, repo.Create(u))
		created[u.ID] = true
	}

	seen := map[string]int{}
	cursor := ""
	prev := ""
	for {
		page, err := repo.ListUsersPage(context.Background(), cursor, 3)
		require.NoError(t, err)
		if len(page) == 0 {
			break
		}
		for _, u := range page {
			require.Greater(t, u.ID, prev, "pages must be strictly ascending by id")
			prev = u.ID
			seen[u.ID]++
		}
		cursor = page[len(page)-1].ID
	}
	for id := range created {
		assert.Equal(t, 1, seen[id], "every created user must appear exactly once")
	}
}

func TestPostgresUserRepository_DuplicateEmailFails(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	require.NoError(t, repo.Create(u))

	dup := newPGUser("Beta")
	dup.Email = u.Email
	assert.ErrorIs(t, repo.Create(dup), ErrEmailExists)
}

func TestPostgresUserRepository_CreateWithVerificationIsAtomic(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	ctx := context.Background()
	user := newPGUser("Atomic Registration")
	now := time.Now().UTC()

	err := repo.CreateWithVerification(ctx, user, LifecycleToken{
		ID: uuid.NewString(), UserID: user.ID, TokenHash: "token-hash",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}, EmailOutboxMessage{
		ID: uuid.NewString(), UserID: user.ID,
		Kind: "invalid-kind", Recipient: user.Email,
		VerificationURL: "https://example.test/verify?token=secret",
		CreatedAt:       now, AvailableAt: now,
	})
	require.Error(t, err)

	_, err = repo.FindByID(user.ID)
	assert.ErrorIs(t, err, ErrUserNotFound,
		"outbox insertion failure must roll back the user and token")
	var tokenCount int
	require.NoError(t, repo.pool.QueryRow(ctx,
		`SELECT count(*) FROM email_verification_tokens WHERE user_id = $1`, user.ID,
	).Scan(&tokenCount))
	assert.Zero(t, tokenCount)
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

func TestPostgresUserRepository_UpdatePassword(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	require.NoError(t, repo.Create(u))

	require.NoError(t, repo.UpdatePassword(u.ID, "new-bcrypt-hash"))

	byID, err := repo.FindByID(u.ID)
	require.NoError(t, err)
	assert.Equal(t, "new-bcrypt-hash", byID.PasswordHash)
}

func TestPostgresUserRepository_UpdatePasswordMissingUserFails(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	assert.ErrorIs(t, repo.UpdatePassword(uuid.NewString(), "hash"), ErrUserNotFound)
}

// TestPostgresUserRepository_SoftDeleteExcludesFromEveryReadPath is the
// property that matters: once soft-deleted, FindByEmail, FindByID, and
// ListUsers must all treat the row as gone (they already filter
// deleted_at IS NULL; SoftDelete is what actually sets that column).
func TestPostgresUserRepository_SoftDeleteExcludesFromEveryReadPath(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	require.NoError(t, repo.Create(u))

	require.NoError(t, repo.SoftDelete(u.ID))

	_, err := repo.FindByID(u.ID)
	assert.ErrorIs(t, err, ErrUserNotFound)
	_, err = repo.FindByEmail(u.Email)
	assert.ErrorIs(t, err, ErrUserNotFound)
	users, err := repo.ListUsers(context.Background())
	require.NoError(t, err)
	for _, x := range users {
		assert.NotEqual(t, u.ID, x.ID)
	}
}

func TestPostgresUserRepository_SoftDeleteMissingUserFails(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	assert.ErrorIs(t, repo.SoftDelete(uuid.NewString()), ErrUserNotFound)
}

// TestPostgresUserRepository_SoftDeleteReleasesEmailForReuse: the email
// column has a plain UNIQUE constraint with no exception for deleted rows, so
// SoftDelete must mangle the stored email — otherwise a deleted account's
// email would be permanently blocked from ever registering again.
func TestPostgresUserRepository_SoftDeleteReleasesEmailForReuse(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	originalEmail := u.Email
	require.NoError(t, repo.Create(u))
	require.NoError(t, repo.SoftDelete(u.ID))

	reregistered := newPGUser("Alpha Reborn")
	reregistered.Email = originalEmail
	assert.NoError(t, repo.Create(reregistered), "the original email must be free to reuse after deletion")

	byEmail, err := repo.FindByEmail(originalEmail)
	require.NoError(t, err)
	assert.Equal(t, reregistered.ID, byEmail.ID, "the email must resolve to the NEW account, not the deleted one")
}

func TestPostgresUserRepository_SoftDeleteIsNotReapplicable(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	u := newPGUser("Alpha")
	require.NoError(t, repo.Create(u))
	require.NoError(t, repo.SoftDelete(u.ID))

	// Deleting an already-deleted row affects no rows (deleted_at IS NULL no
	// longer matches), so it must report the same not-found error rather than
	// silently succeeding a second time.
	assert.ErrorIs(t, repo.SoftDelete(u.ID), ErrUserNotFound)
}

func TestPostgresLifecycleTokensAreSingleUse(t *testing.T) {
	repo := NewPostgresUserRepository(testPool(t))
	user := newPGUser("Lifecycle")
	require.NoError(t, repo.Create(user))
	now := time.Now().UTC()
	raw := "postgres-verification-token"
	require.NoError(t, repo.SaveEmailVerificationToken(context.Background(), LifecycleToken{
		ID: uuid.NewString(), UserID: user.ID, TokenHash: hashLifecycleToken(raw),
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}))

	verified, err := repo.VerifyEmailToken(context.Background(), hashLifecycleToken(raw), now)
	require.NoError(t, err)
	assert.NotNil(t, verified.EmailVerifiedAt)
	_, err = repo.VerifyEmailToken(context.Background(), hashLifecycleToken(raw), now)
	assert.ErrorIs(t, err, ErrInvalidLifecycleToken)
}

func TestPostgresDeleteAccountCascadesOwnedData(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresUserRepository(pool)
	user := newPGUser("Delete Cascade")
	require.NoError(t, repo.Create(user))
	portfolioID := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO portfolios (id, user_id, name, currency)
		VALUES ($1, $2, 'Default', 'USD')
	`, portfolioID, user.ID)
	require.NoError(t, err)
	require.NoError(t, repo.CreateIdentity(&AuthIdentity{
		ID: uuid.NewString(), UserID: user.ID, Provider: ProviderGoogle,
		ProviderSubject: uuid.NewString(), Email: user.Email, EmailVerified: true,
	}))
	require.NoError(t, repo.SavePasswordResetToken(context.Background(), LifecycleToken{
		ID: uuid.NewString(), UserID: user.ID,
		TokenHash: hashLifecycleToken("delete-reset"), CreatedAt: time.Now().UTC(),
		ExpiresAt: time.Now().Add(time.Hour),
	}))

	require.NoError(t, repo.DeleteAccount(context.Background(), user.ID))

	for table := range map[string]bool{
		"users": true, "portfolios": true, "auth_identities": true,
		"password_reset_tokens": true,
	} {
		var count int
		column := "user_id"
		if table == "users" {
			column = "id"
		}
		require.NoError(t, pool.QueryRow(context.Background(),
			`SELECT count(*) FROM `+table+` WHERE `+column+` = $1`, user.ID).Scan(&count))
		assert.Zero(t, count, table)
	}
}
