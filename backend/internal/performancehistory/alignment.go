package performancehistory

import (
	"errors"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
)

var ErrBenchmarkDateAlignment = errors.New("ranked snapshots could not be aligned to benchmark closes")

const RankedBenchmarkAlignmentMethod = "latest_trusted_snapshot_not_after_utc_close_v1"

type AlignedBenchmarkComparison struct {
	Window                    Window
	PortfolioReturnPercentage money.Ratio
	BenchmarkReturnPercentage money.Ratio
	EdgePercentagePoints      money.Ratio
}

// CompareWithBenchmarkCloses is the canonical comparison calculation used by
// both product surfaces: alignment, ranked return, benchmark return, and edge.
func CompareWithBenchmarkCloses(
	points []Snapshot,
	startDate, endDate time.Time,
	benchmarkReturn money.Ratio,
) (AlignedBenchmarkComparison, error) {
	window, err := AlignWindowToBenchmarkCloses(points, startDate, endDate)
	if err != nil {
		return AlignedBenchmarkComparison{}, err
	}
	portfolioReturn, err := TimeframeReturnRatio(
		window.StartSnapshot.RankedIndex, window.EndSnapshot.RankedIndex,
	)
	if err != nil {
		return AlignedBenchmarkComparison{}, ErrBenchmarkDateAlignment
	}
	return AlignedBenchmarkComparison{
		Window: window, PortfolioReturnPercentage: portfolioReturn,
		BenchmarkReturnPercentage: benchmarkReturn,
		EdgePercentagePoints:      portfolioReturn.Sub(benchmarkReturn),
	}, nil
}

// AlignWindowToBenchmarkCloses is the one boundary-selection implementation
// shared by Performance and Achievement comparisons. Because benchmark
// providers expose trading dates rather than exchange close timestamps, a
// boundary close is represented conservatively as the end of that UTC date.
// The newest complete snapshot not later than each boundary is selected.
func AlignWindowToBenchmarkCloses(points []Snapshot, startDate, endDate time.Time) (Window, error) {
	if !endDate.After(startDate) {
		return Window{}, ErrBenchmarkDateAlignment
	}
	sorted := append([]Snapshot(nil), points...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CapturedAt.Before(sorted[j].CapturedAt)
	})
	start, startOK := trustedAtOrBeforeClose(sorted, startDate)
	end, endOK := trustedAtOrBeforeClose(sorted, endDate)
	if !startOK || !endOK || !end.CapturedAt.After(start.CapturedAt) ||
		!start.TrackingStartedAt.Equal(end.TrackingStartedAt) {
		return Window{}, ErrBenchmarkDateAlignment
	}
	aligned := make([]Snapshot, 0)
	for _, point := range sorted {
		if point.CapturedAt.Before(start.CapturedAt) || point.CapturedAt.After(end.CapturedAt) {
			continue
		}
		if point.DataQualityStatus != QualityComplete ||
			!point.TrackingStartedAt.Equal(start.TrackingStartedAt) ||
			point.RankingStatus != performance.StatusActive {
			return Window{}, ErrBenchmarkDateAlignment
		}
		aligned = append(aligned, point)
	}
	if len(aligned) < 2 {
		return Window{}, ErrBenchmarkDateAlignment
	}
	return Window{
		StartSnapshot: start, EndSnapshot: end, Points: aligned,
		HistoryCoverage: 1, ActiveCoverage: 1, TrustedCoverage: 1,
	}, nil
}

func trustedAtOrBeforeClose(points []Snapshot, day time.Time) (Snapshot, bool) {
	cutoff := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, 1)
	for index := len(points) - 1; index >= 0; index-- {
		point := points[index]
		if !point.CapturedAt.UTC().Before(cutoff) {
			continue
		}
		if point.DataQualityStatus != QualityComplete ||
			point.RankingStatus != performance.StatusActive ||
			point.RankedIndex.Cmp(money.ZeroIndexValue()) <= 0 {
			continue
		}
		return point, true
	}
	return Snapshot{}, false
}
