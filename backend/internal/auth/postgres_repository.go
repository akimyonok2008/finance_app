package auth

import (
	"context"
	"errors"
	"fmt"

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

const userColumns = `id, email, password_hash, display_name, avatar_key`

func scanUser(row pgx.Row) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.AvatarKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("auth repository: scan user: %w", err)
	}
	return &u, nil
}

// Create persists a new user. A unique-violation on email maps to ErrEmailExists.
func (r *PostgresUserRepository) Create(user *User) error {
	_, err := r.pool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, display_name, avatar_key)
		 VALUES ($1, $2, $3, $4, $5)`,
		user.ID, normalizeEmail(user.Email), user.PasswordHash, user.DisplayName, user.AvatarKey,
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
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.AvatarKey); err != nil {
			return nil, fmt.Errorf("auth repository: scan user row: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
