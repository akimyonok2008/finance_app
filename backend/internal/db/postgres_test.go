package db

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationOrdering pins the deterministic apply order down as an
// explicit, tested contract — see migrations/README.md for why two numeric
// prefixes (0014, 0015) are intentionally duplicated and must NEVER be
// "fixed" by renaming an already-shipped migration file: schema_migrations
// tracks the applied set by exact filename, so a rename makes a live database
// try to re-run SQL it has already applied, which fails for any non-
// idempotent statement. This test would catch a regression in the ordering
// itself (a stray file, an accidental duplicate) without anyone needing to
// touch the historical files to "clean up" the numbering.
func TestMigrationOrdering(t *testing.T) {
	entries, err := migrationFS.ReadDir("migrations")
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	seen := make(map[string]bool, len(entries))
	for _, e := range entries {
		require.False(t, seen[e.Name()], "duplicate filename in migrations/: %s", e.Name())
		seen[e.Name()] = true
		names = append(names, e.Name())
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	assert.Equal(t, sorted, names, "migrationFS.ReadDir must already return entries in the exact apply order")

	// The two intentionally-duplicated prefixes must sort in this specific
	// order — if this ever fails, someone renamed a file in migrations/ and
	// needs to read migrations/README.md before going further.
	requireBefore(t, names, "0014_archive_cash.sql", "0014_income_fees_corporate_actions.sql")
	requireBefore(t, names, "0015_corporate_actions.sql", "0015_portfolio_idempotency_scope.sql")
}

func requireBefore(t *testing.T, names []string, first, second string) {
	t.Helper()
	firstIdx, secondIdx := -1, -1
	for i, n := range names {
		if n == first {
			firstIdx = i
		}
		if n == second {
			secondIdx = i
		}
	}
	require.NotEqual(t, -1, firstIdx, "%s must exist in migrations/ — do not rename it", first)
	require.NotEqual(t, -1, secondIdx, "%s must exist in migrations/ — do not rename it", second)
	require.Less(t, firstIdx, secondIdx, "%s must apply before %s", first, second)
}

// testPool connects to the integration-test database. Tests are skipped when
// DATABASE_URL_TEST is unset so the suite stays green without local
// infrastructure.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres integration test")
	}
	pool, err := ConnectPostgres(context.Background(), url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func migrationFixture(t *testing.T, sql string) (fstest.MapFS, string, string) {
	t.Helper()
	suffix := "t" + uuid.NewString()[:8]
	table := "migration_" + suffix
	name := "9000_" + suffix + ".sql"
	if sql == "" {
		sql = fmt.Sprintf(`CREATE TABLE %s (id BIGINT PRIMARY KEY)`, table)
	}
	files := fstest.MapFS{"migrations/" + name: {Data: []byte(sql)}}
	return files, name, table
}

func cleanupMigrationFixture(t *testing.T, pool *pgxpool.Pool, table string, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table+` CASCADE`)
		for _, name := range names {
			_, _ = pool.Exec(context.Background(), `DELETE FROM schema_migrations WHERE version=$1`, name)
		}
	})
}

func TestRunMigrations_RecordsVersionOnlyAfterSuccessfulCommit(t *testing.T) {
	pool := testPool(t)
	files, name, table := migrationFixture(t, "")
	cleanupMigrationFixture(t, pool, table, name)

	require.NoError(t, runMigrations(context.Background(), pool, files, "migrations"))
	var tableExists, versionExists bool
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&tableExists))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, name).Scan(&versionExists))
	assert.True(t, tableExists)
	assert.True(t, versionExists)
}

func TestRunMigrations_FailureRollsBackAndCanRetry(t *testing.T) {
	pool := testPool(t)
	_, name, table := migrationFixture(t, "")
	cleanupMigrationFixture(t, pool, table, name)
	failing := fstest.MapFS{"migrations/" + name: {Data: []byte(fmt.Sprintf(
		`CREATE TABLE %s (id BIGINT); INSERT INTO definitely_missing_table VALUES (1)`, table,
	))}}

	err := runMigrations(context.Background(), pool, failing, "migrations")
	require.Error(t, err)
	assert.Contains(t, err.Error(), name)
	assert.NotContains(t, err.Error(), os.Getenv("DATABASE_URL_TEST"))
	var tableExists, versionExists bool
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&tableExists))
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, name).Scan(&versionExists))
	assert.False(t, tableExists, "DDL must roll back with the failed migration")
	assert.False(t, versionExists, "a failed migration must not be recorded")

	corrected := fstest.MapFS{"migrations/" + name: {Data: []byte(fmt.Sprintf(`CREATE TABLE %s (id BIGINT)`, table))}}
	require.NoError(t, runMigrations(context.Background(), pool, corrected, "migrations"))
}

func TestRunMigrations_AlreadyAppliedMigrationIsSkipped(t *testing.T) {
	pool := testPool(t)
	files, name, table := migrationFixture(t, "")
	cleanupMigrationFixture(t, pool, table, name)
	// CREATE TABLE intentionally lacks IF NOT EXISTS: a second execution would
	// fail, proving the recorded version causes a real skip.
	require.NoError(t, runMigrations(context.Background(), pool, files, "migrations"))
	require.NoError(t, runMigrations(context.Background(), pool, files, "migrations"))
}

func TestRunMigrations_AdvisoryLockReleasedAfterSuccessAndFailure(t *testing.T) {
	pool := testPool(t)
	success, successName, successTable := migrationFixture(t, "")
	cleanupMigrationFixture(t, pool, successTable, successName)
	require.NoError(t, runMigrations(context.Background(), pool, success, "migrations"))
	assertMigrationLockAvailable(t, pool)

	_, failureName, failureTable := migrationFixture(t, "")
	cleanupMigrationFixture(t, pool, failureTable, failureName)
	failure := fstest.MapFS{"migrations/" + failureName: {Data: []byte("SELECT * FROM table_that_does_not_exist")}}
	require.Error(t, runMigrations(context.Background(), pool, failure, "migrations"))
	assertMigrationLockAvailable(t, pool)
}

func assertMigrationLockAvailable(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	conn, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	defer conn.Release()
	var locked bool
	require.NoError(t, conn.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, migrationLockKey).Scan(&locked))
	require.True(t, locked, "migration advisory lock should have been released")
	var unlocked bool
	require.NoError(t, conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockKey).Scan(&unlocked))
	require.True(t, unlocked)
}

func TestRunMigrations_UpgradesPreviousFixtureSchemaToLatest(t *testing.T) {
	pool := testPool(t)
	suffix := "t" + uuid.NewString()[:8]
	table := "migration_" + suffix
	firstName := "9000_" + suffix + "_previous.sql"
	latestName := "9001_" + suffix + "_latest.sql"
	cleanupMigrationFixture(t, pool, table, firstName, latestName)

	previous := fstest.MapFS{"migrations/" + firstName: {Data: []byte(fmt.Sprintf(`CREATE TABLE %s (id BIGINT)`, table))}}
	require.NoError(t, runMigrations(context.Background(), pool, previous, "migrations"))
	latest := fstest.MapFS{
		"migrations/" + firstName:  previous["migrations/"+firstName],
		"migrations/" + latestName: {Data: []byte(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN stable_value TEXT NOT NULL DEFAULT ''`, table))},
	}
	require.NoError(t, runMigrations(context.Background(), pool, latest, "migrations"))

	var columnExists bool
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema='public' AND table_name=$1 AND column_name='stable_value'
		)`, table).Scan(&columnExists))
	assert.True(t, columnExists)
}

func TestRunMigrations_IsIdempotent(t *testing.T) {
	pool := testPool(t)

	require.NoError(t, RunMigrations(context.Background(), pool))
	require.NoError(t, RunMigrations(context.Background(), pool), "re-running migrations must be a no-op, not an error")
}

// TestRunMigrations_ConcurrentInstancesDoNotRace reproduces the scenario the
// advisory lock exists for: multiple instances (a rolling deploy, a container
// restarting alongside a still-running old one) all calling RunMigrations at
// startup against the same database. Without serializing them, two instances
// can both see a migration as "not yet applied" and race to INSERT the same
// schema_migrations primary key.
func TestRunMigrations_ConcurrentInstancesDoNotRace(t *testing.T) {
	pool := testPool(t)
	files, name, table := migrationFixture(t, "")
	cleanupMigrationFixture(t, pool, table, name)

	const instances = 8
	var wg sync.WaitGroup
	errs := make(chan error, instances)
	for i := 0; i < instances; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- runMigrations(context.Background(), pool, files, "migrations")
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	var versions int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM schema_migrations WHERE version=$1`, name).Scan(&versions))
	assert.Equal(t, 1, versions)
}
