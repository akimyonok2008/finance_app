// Package db holds PostgreSQL connection helpers and embedded SQL migrations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// ConnectPostgres opens a pgx connection pool and verifies connectivity.
func ConnectPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: invalid configuration: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: connection failed: %w", err)
	}
	return pool, nil
}

// migrationLockKey is an arbitrary, fixed advisory-lock key. It has no meaning
// beyond being unique to this app's migration step.
const migrationLockKey int64 = 0x46494E415050 // "FINAPP"

// RunMigrations applies the embedded SQL migrations in lexical order, tracking
// applied versions in a schema_migrations table. It is idempotent: already
// applied migrations are skipped. This deliberately avoids a heavy migration
// framework for the prototype; the files can be moved to golang-migrate later.
//
// The whole run holds a Postgres session-level advisory lock on one pinned
// connection: without it, two instances starting at once (a rolling deploy, a
// container restarting alongside a still-running old one) both pass the
// per-version "already applied?" check for the same not-yet-applied migration
// before either has committed it, and both try to apply and INSERT INTO
// schema_migrations, racing on the primary key. The lock serializes that
// startup race instead of letting it fail unpredictably.
func RunMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	return runMigrations(ctx, pool, migrationFS, "migrations")
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS, directory string) (returnErr error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migrations: acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationLockKey); err != nil {
		return fmt.Errorf("migrations: acquire advisory lock: %w", err)
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		unlockErr := conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock($1)`, migrationLockKey).Scan(&unlocked)
		if unlockErr != nil || !unlocked {
			// A session lock survives returning a connection to the pool. Close
			// the physical connection if explicit unlock failed.
			_ = conn.Conn().Close(unlockCtx)
			if returnErr == nil {
				if unlockErr != nil {
					returnErr = fmt.Errorf("migrations: release advisory lock: %w", unlockErr)
				} else {
					returnErr = fmt.Errorf("migrations: release advisory lock: lock was not held")
				}
			}
		}
	}()

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("migrations: could not create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations, directory)
	if err != nil {
		return fmt.Errorf("migrations: read embedded dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		sqlBytes, err := fs.ReadFile(migrations, directory+"/"+name)
		if err != nil {
			return fmt.Errorf("migrations: read %s: %w", name, err)
		}
		if err := applyMigration(ctx, conn, name, string(sqlBytes)); err != nil {
			return err
		}
	}
	return nil
}

type migrationBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func applyMigration(ctx context.Context, conn migrationBeginner, name, sql string) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("migrations: begin %s: %w", name, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, name,
	).Scan(&exists); err != nil {
		return fmt.Errorf("migrations: check %s: %w", name, err)
	}
	if exists {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrations: commit skipped %s: %w", name, err)
		}
		committed = true
		return nil
	}
	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("migrations: apply %s: %w", name, err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
		return fmt.Errorf("migrations: record %s: %w", name, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("migrations: commit %s: %w", name, err)
	}
	committed = true
	return nil
}
