package performancehistory

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
)

type rankedStub struct {
	value *performance.RankedPerformance
	err   error
}

func (s rankedStub) CurrentRankedPerformance(context.Context, string) (*performance.RankedPerformance, error) {
	if s.err != nil {
		return nil, s.err
	}
	copy := *s.value
	return &copy, nil
}

func rankedAt(epoch, at time.Time, index string, status performance.Status) *performance.RankedPerformance {
	rankedIndex := testIndex(index)
	return &performance.RankedPerformance{
		PortfolioID: "pf-1", RankedIndex: rankedIndex,
		RankedReturnPercentage: rankedIndex.Sub(money.MustIndexValue("100")), Status: status,
		TrackingStartedAt: epoch, ValuationAsOf: at,
		DataQualityStatus: "complete",
	}
}

func insertPoint(t *testing.T, repo Repository, id string, epoch, at time.Time, index string, status performance.Status, kind SnapshotKind, quality QualityStatus) {
	t.Helper()
	s := Snapshot{
		ID: id, PortfolioID: "pf-1", UserID: "u1",
		TrackingStartedAt: epoch, RankedIndex: testIndex(index), RankingStatus: status,
		CapturedAt: at, Kind: kind, ValuationAsOf: at,
		DataQualityStatus: quality, CreatedAt: at,
	}
	if kind == KindDaily {
		s.SnapshotDate = at.UTC().Format("2006-01-02")
	}
	if kind == KindIntraday {
		bucket := at.UTC().Truncate(4 * time.Hour)
		s.BucketStart = &bucket
	}
	_, err := repo.Insert(context.Background(), s)
	require.NoError(t, err)
}

func insertPointFloat(t *testing.T, repo Repository, id string, epoch, at time.Time, index float64, status performance.Status, kind SnapshotKind, quality QualityStatus) {
	t.Helper()
	insertPoint(t, repo, id, epoch, at, strconv.FormatFloat(index, 'g', -1, 64), status, kind, quality)
}

func TestRecordCurrentUsesPersistentRankedIndexAndIsBucketIdempotent(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 30, 0, 0, time.UTC)
	epoch := now.AddDate(0, 0, -1)
	repo := NewInMemoryRepository()
	svc := NewService(repo, rankedStub{value: rankedAt(epoch, now, "112.3456789", performance.StatusActive)}, DefaultConfig())
	svc.SetClock(func() time.Time { return now })

	written, quality, err := svc.RecordCurrent(context.Background(), "u1")
	require.NoError(t, err)
	assert.Equal(t, 2, written, "one intraday and one canonical daily point")
	assert.Equal(t, QualityComplete, quality)

	written, _, err = svc.RecordCurrent(context.Background(), "u1")
	require.NoError(t, err)
	assert.Zero(t, written, "same interval/day must be idempotent")

	points, err := repo.List(context.Background(), "u1", now.Add(-time.Hour), now.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, points, 2)
	for _, point := range points {
		assertIndexEqual(t, "112.3456789", point.RankedIndex)
		assert.Equal(t, epoch, point.TrackingStartedAt)
	}
}

func TestLatestTrustedSnapshotAtIgnoresNewerUntrustedPoint(t *testing.T) {
	epoch := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	trusted := epoch.Add(24 * time.Hour)
	repo := NewInMemoryRepository()
	insertPoint(t, repo, "trusted", epoch, trusted, "101",
		performance.StatusActive, KindTransition, QualityComplete)
	insertPoint(t, repo, "newer-stale", epoch, trusted.Add(time.Hour), "102",
		performance.StatusActive, KindTransition, QualityStale)
	svc := NewService(repo, rankedStub{value: rankedAt(epoch, trusted, "101", performance.StatusActive)}, DefaultConfig())

	got, found, err := svc.LatestTrustedSnapshotAt(context.Background(), "u1", "pf-1", epoch)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, trusted, got)
}

func TestRecordCurrentRejectsFailedOrInvalidValuation(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, rankedStub{err: errors.New("price unavailable")}, DefaultConfig())
	_, quality, err := svc.RecordCurrent(context.Background(), "u1")
	assert.Error(t, err)
	assert.Equal(t, QualityInvalid, quality)

	now := time.Now().UTC()
	svc = NewService(repo, rankedStub{value: rankedAt(now, now, "0", performance.StatusActive)}, DefaultConfig())
	_, _, err = svc.RecordCurrent(context.Background(), "u1")
	assert.ErrorIs(t, err, ErrInvalidSnapshot)
}

func TestConcurrentWorkersCreateOneIntradayAndOneDailyPoint(t *testing.T) {
	now := time.Date(2026, 7, 24, 8, 0, 0, 0, time.UTC)
	epoch := now.Add(-time.Hour)
	repo := NewInMemoryRepository()
	svc := NewService(repo, rankedStub{value: rankedAt(epoch, now, "101", performance.StatusActive)}, DefaultConfig())
	svc.SetClock(func() time.Time { return now })

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, _ = svc.RecordCurrent(context.Background(), "u1")
		}()
	}
	wg.Wait()
	points, _ := repo.List(context.Background(), "u1", now.Add(-time.Minute), now.Add(time.Minute))
	assert.Len(t, points, 2)
}

func TestWindowCalculatesRankedReturnBoundariesAndCoverage(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -30)
	epoch := start.Add(-time.Hour)
	repo := NewInMemoryRepository()
	for day := 0; day <= 30; day++ {
		at := start.AddDate(0, 0, day)
		index := 100 + float64(day)/3
		insertPointFloat(t, repo, "d-"+at.Format("20060102"), epoch, at, index, performance.StatusActive, KindDaily, QualityComplete)
	}
	svc := NewService(repo, rankedStub{}, Config{
		BoundaryTolerance: 36 * time.Hour, EndFreshness: 8 * time.Hour,
		MaxTrustedGap: 36 * time.Hour, EligibilityThreshold: .9,
	})
	window, err := svc.Window(context.Background(), "u1", start, now)
	require.NoError(t, err)
	assertIndexEqual(t, "100", window.StartSnapshot.RankedIndex)
	assertIndexEqual(t, "110", window.EndSnapshot.RankedIndex)
	assert.InDelta(t, 1, window.HistoryCoverage, .001)
	assert.InDelta(t, 1, window.ActiveCoverage, .001)
	assert.InDelta(t, 1, window.TrustedCoverage, .001)
	assert.True(t, window.Eligible(.9))
}

func TestWindowAllowsAfterBoundaryWithinToleranceAndRejectsOutside(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	target := now.AddDate(0, 0, -30)
	epoch := target.Add(-time.Hour)
	repo := NewInMemoryRepository()
	insertPoint(t, repo, "start", epoch, target.Add(3*time.Hour), "100", performance.StatusActive, KindIntraday, QualityComplete)
	for day := 1; day < 30; day++ {
		at := target.Add(3*time.Hour).AddDate(0, 0, day)
		if at.After(now) {
			break
		}
		insertPointFloat(t, repo, "p-"+at.Format("2006010215"), epoch, at, 100+float64(day), performance.StatusActive, KindDaily, QualityComplete)
	}
	insertPoint(t, repo, "end", epoch, now, "110", performance.StatusActive, KindIntraday, QualityComplete)
	svc := NewService(repo, rankedStub{}, Config{
		BoundaryTolerance: 4 * time.Hour, EndFreshness: 4 * time.Hour, MaxTrustedGap: 36 * time.Hour,
	})
	_, err := svc.Window(context.Background(), "u1", target, now)
	require.NoError(t, err)

	svc = NewService(repo, rankedStub{}, Config{
		BoundaryTolerance: 2 * time.Hour, EndFreshness: 4 * time.Hour, MaxTrustedGap: 36 * time.Hour,
	})
	_, err = svc.Window(context.Background(), "u1", target, now)
	assert.ErrorIs(t, err, ErrWindowNotReady)
}

func TestWindowRejectsStaleEndAndCrossEpochHistory(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -30)
	oldEpoch := start.AddDate(0, 0, -10)
	newEpoch := now.AddDate(0, 0, -5)
	repo := NewInMemoryRepository()
	insertPoint(t, repo, "old-start", oldEpoch, start, "100", performance.StatusActive, KindDaily, QualityComplete)
	insertPoint(t, repo, "new-start", newEpoch, newEpoch, "100", performance.StatusActive, KindTransition, QualityComplete)
	insertPoint(t, repo, "new-end", newEpoch, now.Add(-10*time.Hour), "105", performance.StatusActive, KindIntraday, QualityComplete)
	svc := NewService(repo, rankedStub{}, Config{
		BoundaryTolerance: 36 * time.Hour, EndFreshness: 8 * time.Hour, MaxTrustedGap: 36 * time.Hour,
	})
	_, err := svc.Window(context.Background(), "u1", start, now)
	assert.ErrorIs(t, err, ErrWindowNotReady)
}

func TestPausedCoveragePreventsGaming(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -30)
	epoch := start.Add(-time.Hour)
	repo := NewInMemoryRepository()
	for day := 0; day <= 30; day++ {
		at := start.AddDate(0, 0, day)
		status := performance.StatusPaused
		if day < 5 {
			status = performance.StatusActive
		}
		insertPointFloat(t, repo, "p-"+at.Format("20060102"), epoch, at, 100+float64(day)/3, status, KindDaily, QualityComplete)
	}
	svc := NewService(repo, rankedStub{}, DefaultConfig())
	window, err := svc.Window(context.Background(), "u1", start, now)
	require.NoError(t, err)
	assert.Less(t, window.ActiveCoverage, .2)
	assert.False(t, window.Eligible(.9))
}

func TestShortPauseWithinCoverageThresholdRemainsEligible(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	start := now.AddDate(0, 0, -30)
	epoch := start.Add(-time.Hour)
	repo := NewInMemoryRepository()
	for day := 0; day <= 30; day++ {
		at := start.AddDate(0, 0, day)
		status := performance.StatusActive
		if day == 10 || day == 11 {
			status = performance.StatusPaused
		}
		insertPointFloat(t, repo, "short-pause-"+at.Format("20060102"), epoch, at,
			100+float64(day)/3, status, KindDaily, QualityComplete)
	}
	svc := NewService(repo, rankedStub{}, DefaultConfig())
	window, err := svc.Window(context.Background(), "u1", start, now)
	require.NoError(t, err)
	assert.InDelta(t, 28.0/30.0, window.ActiveCoverage, .001)
	assert.True(t, window.Eligible(.9))
}

func TestTransitionSuppressionAndResumeBoundary(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	epoch := now.AddDate(0, 0, -1)
	repo := NewInMemoryRepository()
	svc := NewService(repo, rankedStub{}, DefaultConfig())
	input := performance.TransitionSnapshot{
		PortfolioID: "pf-1", UserID: "u1", TrackingStartedAt: epoch,
		RankedIndex: testIndex("100"), Status: performance.StatusActive,
		CapturedAt: now, ValuationAsOf: now, DataQualityStatus: "complete",
	}
	wrote, err := svc.RecordTransitionIfChanged(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, wrote)
	input.CapturedAt = now.Add(time.Minute)
	wrote, err = svc.RecordTransitionIfChanged(context.Background(), input)
	require.NoError(t, err)
	assert.False(t, wrote, "ordinary same-status mutation is not a transition")
	input.Status = performance.StatusPaused
	input.CapturedAt = now.Add(2 * time.Minute)
	wrote, err = svc.RecordTransitionIfChanged(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, wrote)
	input.Status = performance.StatusActive
	input.CapturedAt = now.Add(3 * time.Minute)
	wrote, err = svc.RecordTransitionIfChanged(context.Background(), input)
	require.NoError(t, err)
	assert.True(t, wrote)
}

func TestCompactionPreservesDailyTransitionAndEvidence(t *testing.T) {
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	epoch := now.AddDate(0, 0, -200)
	old := now.AddDate(0, 0, -150)
	repo := NewInMemoryRepository()
	insertPoint(t, repo, "daily", epoch, old, "101", performance.StatusActive, KindDaily, QualityComplete)
	insertPoint(t, repo, "drop", epoch, old.Add(time.Hour), "101", performance.StatusActive, KindIntraday, QualityComplete)
	insertPoint(t, repo, "keep-evidence", epoch, old.Add(5*time.Hour), "101", performance.StatusActive, KindIntraday, QualityComplete)
	insertPoint(t, repo, "transition", epoch, old.Add(6*time.Hour), "101", performance.StatusPaused, KindTransition, QualityComplete)
	require.NoError(t, repo.Protect(context.Background(), "keep-evidence"))
	removed, err := repo.Compact(context.Background(), now.AddDate(0, 0, -120))
	require.NoError(t, err)
	assert.EqualValues(t, 1, removed)
	points, _ := repo.List(context.Background(), "u1", old.Add(-time.Hour), old.Add(7*time.Hour))
	ids := map[string]bool{}
	for _, point := range points {
		ids[point.ID] = true
	}
	assert.True(t, ids["daily"])
	assert.True(t, ids["keep-evidence"])
	assert.True(t, ids["transition"])
	assert.False(t, ids["drop"])
}

func TestEvaluationFailureIsRetryable(t *testing.T) {
	now := time.Now().UTC()
	epoch := now.Add(-time.Hour)
	repo := NewInMemoryRepository()
	insertPoint(t, repo, "pending", epoch, now, "100", performance.StatusActive, KindTransition, QualityComplete)
	claims, err := repo.ClaimEvaluations(context.Background(), 10, now.Add(-time.Minute))
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.NoError(t, repo.FailEvaluation(context.Background(), claims[0].SnapshotID, "temporary"))
	claims, err = repo.ClaimEvaluations(context.Background(), 10, now.Add(-time.Minute))
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.NoError(t, repo.CompleteEvaluation(context.Background(), claims[0].SnapshotID))
	claims, err = repo.ClaimEvaluations(context.Background(), 10, now.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, claims)
}
