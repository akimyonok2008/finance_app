package performancehistory

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/db"
	"github.com/ardakimyonok/finance_app/internal/performance"
)

// These tests exercise PostgreSQL's partial unique indexes, epoch filtering,
// evidence protection, and compaction. They are skipped unless
// DATABASE_URL_TEST points at a disposable database.
func historyTestPool(t *testing.T) *pgxpool.Pool {
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

func seedHistoryOwner(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	userID := uuid.NewString()
	portfolioID := uuid.NewString()
	ctx := context.Background()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name)
		 VALUES ($1, $2, 'h', 'History Test')`,
		userID, userID+"@example.com")
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO portfolios (id, user_id, name, currency)
		 VALUES ($1, $2, 'Default', 'USD')`,
		portfolioID, userID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, userID)
	})
	return userID, portfolioID
}

func pgHistorySnapshot(id, userID, portfolioID string, epoch, at time.Time, kind SnapshotKind) Snapshot {
	snapshot := Snapshot{
		ID: id, UserID: userID, PortfolioID: portfolioID,
		TrackingStartedAt: epoch, RankedIndex: 101.25,
		RankingStatus: performance.StatusActive, CapturedAt: at,
		Kind: kind, ValuationAsOf: at, DataQualityStatus: QualityComplete,
		CreatedAt: at,
	}
	switch kind {
	case KindIntraday:
		bucket := at.UTC().Truncate(4 * time.Hour)
		snapshot.BucketStart = &bucket
	case KindDaily:
		snapshot.SnapshotDate = at.UTC().Format("2006-01-02")
	}
	return snapshot
}

func TestPGConcurrentInsertIsIdempotentPerCanonicalBucket(t *testing.T) {
	pool := historyTestPool(t)
	repo := NewPostgresRepository(pool)
	userID, portfolioID := seedHistoryOwner(t, pool)
	now := time.Date(2026, 7, 24, 12, 30, 0, 0, time.UTC)
	epoch := now.AddDate(0, 0, -10)

	for _, kind := range []SnapshotKind{KindIntraday, KindDaily} {
		t.Run(string(kind), func(t *testing.T) {
			var inserted atomic.Int64
			var wg sync.WaitGroup
			for range 12 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					snapshot := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, epoch, now, kind)
					ok, err := repo.Insert(context.Background(), snapshot)
					assert.NoError(t, err)
					if ok {
						inserted.Add(1)
					}
				}()
			}
			wg.Wait()
			assert.EqualValues(t, 1, inserted.Load())

			var count int
			require.NoError(t, pool.QueryRow(context.Background(),
				`SELECT count(*) FROM ranked_performance_snapshots
				 WHERE portfolio_id=$1 AND tracking_started_at=$2 AND snapshot_kind=$3`,
				portfolioID, epoch, kind).Scan(&count))
			assert.Equal(t, 1, count)
		})
	}
}

func TestPGIndexAtOrBeforeIsEpochSafeAndCompleteOnly(t *testing.T) {
	pool := historyTestPool(t)
	repo := NewPostgresRepository(pool)
	userID, portfolioID := seedHistoryOwner(t, pool)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	oldEpoch := now.AddDate(0, 0, -60)
	currentEpoch := now.AddDate(0, 0, -30)

	old := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, oldEpoch, now.AddDate(0, 0, -20), KindTransition)
	old.RankedIndex = 999
	_, err := repo.Insert(context.Background(), old)
	require.NoError(t, err)

	complete := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, currentEpoch, now.Add(-2*time.Hour), KindTransition)
	complete.RankedIndex = 110
	_, err = repo.Insert(context.Background(), complete)
	require.NoError(t, err)

	stale := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, currentEpoch, now.Add(-time.Hour), KindTransition)
	stale.RankedIndex = 150
	stale.DataQualityStatus = QualityStale
	_, err = repo.Insert(context.Background(), stale)
	require.NoError(t, err)

	index, found, err := repo.IndexAtOrBefore(context.Background(), userID, now, currentEpoch)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, 110.0, index)
}

func TestPGCompactionRequiresDailyCoverageAndPreservesEvidence(t *testing.T) {
	pool := historyTestPool(t)
	repo := NewPostgresRepository(pool)
	userID, portfolioID := seedHistoryOwner(t, pool)
	epoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	day := time.Date(2026, 2, 1, 8, 0, 0, 0, time.UTC)

	removable := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, epoch, day, KindIntraday)
	protected := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, epoch, day.Add(4*time.Hour), KindIntraday)
	uncovered := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, epoch, day.AddDate(0, 0, 1), KindIntraday)
	daily := pgHistorySnapshot(uuid.NewString(), userID, portfolioID, epoch, day.Add(10*time.Hour), KindDaily)
	for _, snapshot := range []Snapshot{removable, protected, uncovered, daily} {
		_, err := repo.Insert(context.Background(), snapshot)
		require.NoError(t, err)
	}
	require.NoError(t, repo.Protect(context.Background(), protected.ID))

	removed, err := repo.Compact(context.Background(), day.AddDate(0, 0, 10))
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed)

	var remaining int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM ranked_performance_snapshots
		 WHERE id = ANY($1::uuid[])`, []string{protected.ID, uncovered.ID, daily.ID}).Scan(&remaining))
	assert.Equal(t, 3, remaining)
}
