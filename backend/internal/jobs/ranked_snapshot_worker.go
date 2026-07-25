package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/ardakimyonok/finance_app/internal/performancehistory"
)

type RankedSnapshotJob interface {
	SnapshotAll(ctx context.Context) performancehistory.BatchResult
	ProcessEvaluations(ctx context.Context) (processed, failed int)
	Compact(ctx context.Context) (int64, error)
}

// RankedSnapshotWorker runs independently from the leaderboard-cache cadence.
// Repository uniqueness and evaluation claims make it safe across instances.
type RankedSnapshotWorker struct {
	job      RankedSnapshotJob
	interval time.Duration
}

func NewRankedSnapshotWorker(job RankedSnapshotJob, interval time.Duration) *RankedSnapshotWorker {
	if interval <= 0 {
		interval = 4 * time.Hour
	}
	return &RankedSnapshotWorker{job: job, interval: interval}
}

func (w *RankedSnapshotWorker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.RunOnce(ctx)
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
	return done
}

func (w *RankedSnapshotWorker) RunOnce(ctx context.Context) {
	result := w.job.SnapshotAll(ctx)
	processed, evaluationFailures := w.job.ProcessEvaluations(ctx)
	compacted, err := w.job.Compact(ctx)
	if err != nil {
		slog.Error("ranked snapshot compaction failed", "error", err)
	}
	slog.Info("ranked snapshot pass completed",
		"users_processed", result.UsersProcessed,
		"snapshots_created", result.SnapshotsCreated,
		"skipped_valuations", result.SkippedValuations,
		"stale_snapshots", result.StaleSnapshots,
		"failures", result.Failures,
		"evaluations_processed", processed,
		"evaluation_failures", evaluationFailures,
		"intraday_compacted", compacted,
	)
}
