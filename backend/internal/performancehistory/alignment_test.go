package performancehistory

import (
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func alignedSnapshot(at time.Time, index float64, epoch time.Time) Snapshot {
	return Snapshot{
		ID: at.Format(time.RFC3339Nano), PortfolioID: "portfolio", UserID: "user",
		TrackingStartedAt: epoch, RankedIndex: money.IndexValueFromFloat64(index),
		RankingStatus: performance.StatusActive, CapturedAt: at, ValuationAsOf: at,
		Kind: KindIntraday, BucketStart: &at, DataQualityStatus: QualityComplete,
	}
}

func TestAlignWindowToBenchmarkClosesNeverSelectsAfterBoundary(t *testing.T) {
	epoch := day(2026, time.January, 1)
	points := []Snapshot{
		alignedSnapshot(day(2026, time.January, 2).Add(4*time.Hour), 100, epoch),
		alignedSnapshot(day(2026, time.January, 2).Add(14*time.Hour), 999, epoch),
		alignedSnapshot(day(2026, time.January, 5).Add(4*time.Hour), 110, epoch),
		alignedSnapshot(day(2026, time.January, 5).Add(14*time.Hour), 999, epoch),
	}
	window, err := AlignWindowToBenchmarkCloses(
		points, day(2026, time.January, 2), day(2026, time.January, 5),
	)
	require.NoError(t, err)
	assertIndexEqual(t, "100", window.StartSnapshot.RankedIndex)
	assertIndexEqual(t, "110", window.EndSnapshot.RankedIndex)
}

func TestAlignWindowToBenchmarkClosesRejectsDifferentEpochs(t *testing.T) {
	points := []Snapshot{
		alignedSnapshot(day(2026, time.January, 2), 100, day(2026, time.January, 1)),
		alignedSnapshot(day(2026, time.January, 5), 110, day(2026, time.January, 4)),
	}
	_, err := AlignWindowToBenchmarkCloses(
		points, day(2026, time.January, 2), day(2026, time.January, 5),
	)
	assert.ErrorIs(t, err, ErrBenchmarkDateAlignment)
}

func TestAlignWindowToBenchmarkClosesRejectsPausedOrUntrustedInterval(t *testing.T) {
	epoch := day(2026, time.January, 1)
	points := []Snapshot{
		alignedSnapshot(day(2026, time.January, 2), 100, epoch),
		alignedSnapshot(day(2026, time.January, 3), 101, epoch),
		alignedSnapshot(day(2026, time.January, 5), 110, epoch),
	}
	points[1].RankingStatus = performance.StatusPaused
	_, err := AlignWindowToBenchmarkCloses(
		points, day(2026, time.January, 2), day(2026, time.January, 5),
	)
	assert.ErrorIs(t, err, ErrBenchmarkDateAlignment)
}
