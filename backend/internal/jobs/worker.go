// Package jobs runs the background maintenance tasks: ensuring the current
// weekly sprint exists and refreshing the Redis leaderboard caches. It is a
// simple ticker-based worker — deliberately no queue/broker for this phase.
package jobs

import (
	"context"
	"log/slog"
	"time"
)

// GlobalLeaderboardRefresher recomputes and caches the global ranking.
// Implemented by *leaderboard.Service.
type GlobalLeaderboardRefresher interface {
	RefreshCache(ctx context.Context) (skipped int, err error)
}

// SprintMaintainer ensures the current sprint exists and refreshes sprint
// rankings. Implemented by *competitions.Service.
type SprintMaintainer interface {
	EnsureCurrentSprint(ctx context.Context) error
	ListActiveCompetitionIDs(ctx context.Context) ([]string, error)
	RefreshCache(ctx context.Context, competitionID string) (skipped int, err error)
}

// Worker periodically runs all maintenance jobs. Each job is independent: one
// failing job never prevents the others from running.
type Worker struct {
	global   GlobalLeaderboardRefresher
	sprints  SprintMaintainer
	interval time.Duration
}

// NewWorker wires a Worker that runs every interval.
func NewWorker(global GlobalLeaderboardRefresher, sprints SprintMaintainer, interval time.Duration) *Worker {
	return &Worker{global: global, sprints: sprints, interval: interval}
}

// Start runs the jobs immediately, then on every tick until ctx is cancelled.
// The returned channel closes when the worker has fully stopped.
func (w *Worker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		slog.Info("background worker started", "interval", w.interval.String())
		w.RunOnce(ctx)

		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("background worker stopped")
				return
			case <-ticker.C:
				w.RunOnce(ctx)
			}
		}
	}()
	return done
}

// RunOnce executes every job a single time. Exported so startup and tests can
// trigger a full pass synchronously.
func (w *Worker) RunOnce(ctx context.Context) {
	w.ensureSprint(ctx)
	w.refreshGlobal(ctx)
	w.refreshSprints(ctx)
}

func (w *Worker) ensureSprint(ctx context.Context) {
	if err := w.sprints.EnsureCurrentSprint(ctx); err != nil {
		slog.Error("job: ensure current sprint failed", "error", err)
		return
	}
	slog.Debug("job: current sprint ensured")
}

func (w *Worker) refreshGlobal(ctx context.Context) {
	skipped, err := w.global.RefreshCache(ctx)
	if err != nil {
		slog.Error("job: global leaderboard refresh failed", "error", err)
		return
	}
	slog.Info("job: global leaderboard refreshed", "skipped_users", skipped)
}

func (w *Worker) refreshSprints(ctx context.Context) {
	ids, err := w.sprints.ListActiveCompetitionIDs(ctx)
	if err != nil {
		slog.Error("job: list active competitions failed", "error", err)
		return
	}
	for _, id := range ids {
		skipped, err := w.sprints.RefreshCache(ctx, id)
		if err != nil {
			slog.Error("job: sprint leaderboard refresh failed", "competition_id", id, "error", err)
			continue
		}
		slog.Info("job: sprint leaderboard refreshed", "competition_id", id, "skipped_entries", skipped)
	}
}
