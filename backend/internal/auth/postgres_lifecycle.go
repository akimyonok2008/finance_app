package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

func (r *PostgresUserRepository) UpdatePasswordAndRevoke(userID, passwordHash string) (*User, error) {
	return scanUser(r.pool.QueryRow(context.Background(), `
		UPDATE users
		SET password_hash = $1, has_password = TRUE,
		    auth_version = auth_version + 1, updated_at = now()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING `+userColumns,
		passwordHash, userID,
	))
}

func (r *PostgresUserRepository) IncrementAuthVersion(userID string) (*User, error) {
	return scanUser(r.pool.QueryRow(context.Background(), `
		UPDATE users
		SET auth_version = auth_version + 1, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userColumns,
		userID,
	))
}

func (r *PostgresUserRepository) SaveEmailVerificationToken(ctx context.Context, token LifecycleToken) error {
	return r.saveLifecycleToken(ctx, "email_verification_tokens", token)
}

func (r *PostgresUserRepository) SavePasswordResetToken(ctx context.Context, token LifecycleToken) error {
	return r.saveLifecycleToken(ctx, "password_reset_tokens", token)
}

func (r *PostgresUserRepository) SaveReauthenticationToken(ctx context.Context, token LifecycleToken) error {
	return r.saveLifecycleToken(ctx, "reauthentication_tokens", token)
}

func (r *PostgresUserRepository) saveLifecycleToken(ctx context.Context, table string, token LifecycleToken) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth repository: begin token save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if table != "reauthentication_tokens" {
		if _, err := tx.Exec(ctx, `UPDATE `+table+`
			SET consumed_at = now()
			WHERE user_id = $1 AND consumed_at IS NULL`, token.UserID); err != nil {
			return fmt.Errorf("auth repository: invalidate tokens: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO `+table+`
		(id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		token.ID, token.UserID, token.TokenHash, token.ExpiresAt, token.CreatedAt); err != nil {
		return fmt.Errorf("auth repository: save token: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *PostgresUserRepository) VerifyEmailToken(ctx context.Context, tokenHash string, now time.Time) (*User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth repository: begin email verification: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE email_verification_tokens
		SET consumed_at = $2
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING user_id
	`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidLifecycleToken
	}
	if err != nil {
		return nil, fmt.Errorf("auth repository: consume email token: %w", err)
	}
	user, err := scanUser(tx.QueryRow(ctx, `
		UPDATE users
		SET email_verified_at = COALESCE(email_verified_at, $2), updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
		RETURNING `+userColumns,
		userID, now,
	))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("auth repository: commit email verification: %w", err)
	}
	return user, nil
}

func (r *PostgresUserRepository) ResetPasswordToken(
	ctx context.Context,
	tokenHash, passwordHash string,
	now time.Time,
) (*User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("auth repository: begin password reset: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE password_reset_tokens
		SET consumed_at = $2
		WHERE token_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING user_id
	`, tokenHash, now).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrInvalidLifecycleToken
	}
	if err != nil {
		return nil, fmt.Errorf("auth repository: consume reset token: %w", err)
	}
	user, err := scanUser(tx.QueryRow(ctx, `
		UPDATE users
		SET password_hash = $2, has_password = TRUE,
		    auth_version = auth_version + 1, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL AND has_password = TRUE
		RETURNING `+userColumns,
		userID, passwordHash,
	))
	if errors.Is(err, ErrUserNotFound) {
		return nil, ErrInvalidLifecycleToken
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE password_reset_tokens
		SET consumed_at = $2
		WHERE user_id = $1 AND consumed_at IS NULL
	`, userID, now); err != nil {
		return nil, fmt.Errorf("auth repository: invalidate reset tokens: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("auth repository: commit password reset: %w", err)
	}
	return user, nil
}

func (r *PostgresUserRepository) ConsumeReauthenticationToken(
	ctx context.Context,
	userID, tokenHash string,
	now time.Time,
) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE reauthentication_tokens
		SET consumed_at = $3
		WHERE user_id = $1 AND token_hash = $2
		  AND consumed_at IS NULL AND expires_at > $3
	`, userID, tokenHash, now)
	if err != nil {
		return fmt.Errorf("auth repository: consume reauthentication token: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrReauthenticationRequired
	}
	return nil
}

// DeleteAccount performs the durable deletion in one transaction. Every
// user-owned table has an ON DELETE CASCADE foreign key to users (directly or
// through portfolios/conversations), so this removes authentication, financial,
// ranking, profile, achievement, competition, and social data atomically.
func (r *PostgresUserRepository) DeleteAccount(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("auth repository: begin account deletion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE users
		SET auth_version = auth_version + 1, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL
	`, userID)
	if err != nil {
		return fmt.Errorf("auth repository: revoke account sessions: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrUserNotFound
	}
	if _, err := tx.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		return fmt.Errorf("auth repository: delete account: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("auth repository: commit account deletion: %w", err)
	}
	return nil
}
