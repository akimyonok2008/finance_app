package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresUserRepository is the durable implementation of UserRepository.
// It satisfies the exact same interface as InMemoryUserRepository, so the
// service layer cannot tell them apart.
type PostgresUserRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresUserRepository wires a Postgres-backed user repository.
func NewPostgresUserRepository(pool *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{pool: pool}
}

const userColumns = `id, email, password_hash, display_name, avatar_key,
	email_verified_at, auth_version, has_password,
	role, suspended_until, suspension_reason, banned_at, ban_reason`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	var passwordHash *string
	var suspensionReason, banReason *string
	if err := row.Scan(&u.ID, &u.Email, &passwordHash, &u.DisplayName, &u.AvatarKey,
		&u.EmailVerifiedAt, &u.AuthVersion, &u.HasPassword,
		&u.Role, &u.SuspendedUntil, &suspensionReason, &u.BannedAt, &banReason); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth repository: scan user: %w", err)
	}
	if passwordHash != nil {
		u.PasswordHash = *passwordHash
	}
	if suspensionReason != nil {
		u.SuspensionReason = *suspensionReason
	}
	if banReason != nil {
		u.BanReason = *banReason
	}
	return &u, nil
}

// Create persists a new user. A unique-violation on email maps to ErrEmailExists.
func (r *PostgresUserRepository) Create(user *User) error {
	authVersion := user.AuthVersion
	if authVersion == 0 {
		authVersion = 1
	}
	hasPassword := user.HasPassword || user.PasswordHash != ""
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO users (
			id, email, password_hash, display_name, avatar_key,
			email_verified_at, auth_version, has_password
		 ) VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8)`,
		user.ID, normalizeEmail(user.Email), user.PasswordHash, user.DisplayName, user.AvatarKey,
		user.EmailVerifiedAt, authVersion, hasPassword,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrEmailExists
		}
		return fmt.Errorf("auth repository: create user: %w", err)
	}
	return nil
}

// FindByEmail looks up a non-deleted user by normalized email.
func (r *PostgresUserRepository) FindByEmail(email string) (*User, error) {
	row := r.pool.QueryRow(context.Background(),
		`SELECT `+userColumns+` FROM users WHERE email = $1 AND deleted_at IS NULL`,
		normalizeEmail(email),
	)
	return scanUser(row)
}

// FindByID looks up a non-deleted user by id.
func (r *PostgresUserRepository) FindByID(id string) (*User, error) {
	row := r.pool.QueryRow(context.Background(),
		`SELECT `+userColumns+` FROM users WHERE id = $1 AND deleted_at IS NULL`, id,
	)
	return scanUser(row)
}

// ListUsers returns all non-deleted users.
func (r *PostgresUserRepository) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE deleted_at IS NULL ORDER BY created_at`,
	)
	if err != nil {
		return nil, fmt.Errorf("auth repository: list users: %w", err)
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, fmt.Errorf("auth repository: scan user row: %w", err)
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

// UpdatePassword overwrites the stored password hash for a non-deleted user.
func (r *PostgresUserRepository) UpdatePassword(userID, passwordHash string) error {
	tag, err := r.pool.Exec(context.Background(),
		`UPDATE users SET password_hash = $1, has_password = TRUE, updated_at = now()
		 WHERE id = $2 AND deleted_at IS NULL`,
		passwordHash, userID,
	)
	if err != nil {
		return fmt.Errorf("auth repository: update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SoftDelete stamps deleted_at, after which FindByEmail/FindByID/ListUsers all
// exclude the row (they already filter on deleted_at IS NULL). It also
// mangles the email to a synthetic, still-unique value: the email column has
// a plain UNIQUE constraint (not scoped to non-deleted rows), so leaving the
// original address in place would permanently block that email from ever
// registering again, even though the account is gone from every read path.
func (r *PostgresUserRepository) SoftDelete(userID string) error {
	tag, err := r.pool.Exec(context.Background(),
		`UPDATE users
		 SET deleted_at = now(), updated_at = now(),
		     email = 'deleted-' || id::text || '@deleted.invalid'
		 WHERE id = $1 AND deleted_at IS NULL`,
		userID,
	)
	if err != nil {
		return fmt.Errorf("auth repository: soft delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// SetRole assigns a moderation role for a non-deleted user.
func (r *PostgresUserRepository) SetRole(ctx context.Context, userID, role string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET role = $2, updated_at = now() WHERE id = $1 AND deleted_at IS NULL
		 RETURNING `+userColumns,
		userID, role,
	)
	return scanUser(row)
}

// Suspend sets or clears a temporary suspension window for a non-deleted user.
func (r *PostgresUserRepository) Suspend(ctx context.Context, userID string, until *time.Time, reason string) (*User, error) {
	var reasonArg *string
	if until != nil {
		reasonArg = &reason
	}
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET suspended_until = $2, suspension_reason = $3, updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING `+userColumns,
		userID, until, reasonArg,
	)
	return scanUser(row)
}

// Ban permanently bans a non-deleted user.
func (r *PostgresUserRepository) Ban(ctx context.Context, userID string, reason string) (*User, error) {
	row := r.pool.QueryRow(ctx,
		`UPDATE users SET banned_at = now(), ban_reason = $2, updated_at = now()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING `+userColumns,
		userID, reason,
	)
	return scanUser(row)
}

func (r *PostgresUserRepository) FindIdentity(provider AuthProvider, subject string) (*AuthIdentity, error) {
	var identity AuthIdentity
	err := r.pool.QueryRow(context.Background(), `
		SELECT id, user_id, provider, provider_subject, email, email_verified
		FROM auth_identities
		WHERE provider = $1 AND provider_subject = $2
	`, string(provider), subject).Scan(
		&identity.ID, &identity.UserID, &identity.Provider, &identity.ProviderSubject,
		&identity.Email, &identity.EmailVerified,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrIdentityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth repository: find identity: %w", err)
	}
	return &identity, nil
}

func (r *PostgresUserRepository) CreateIdentity(identity *AuthIdentity) error {
	_, err := r.pool.Exec(context.Background(), `
		INSERT INTO auth_identities (
			id, user_id, provider, provider_subject, email, email_verified
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, identity.ID, identity.UserID, string(identity.Provider), identity.ProviderSubject,
		normalizeEmail(identity.Email), identity.EmailVerified)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEmailExists
		}
		return fmt.Errorf("auth repository: create identity: %w", err)
	}
	return nil
}
