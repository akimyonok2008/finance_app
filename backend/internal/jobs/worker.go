// Package jobs runs the background maintenance tasks: ensuring the current
// weekly sprint exists and refreshing the Redis leaderboard caches. It is a
// simple ticker-based worker — deliberately no queue/broker for this phase.
package jobs

import (
	"context"
	"log/slog"
	"time"
)

// Leader reports whether this process currently holds leadership over the
// gated jobs (implemented by *leaderlock.Elector). Worker takes a narrow
// interface rather than importing leaderlock's concrete type so callers that
// never need coordination (a single-instance/dev/memory-mode deployment) can
// leave it unset — a nil Leader is treated as "always leader" — without
// jobs depending on leaderlock's pgx-specific dependencies.
type Leader interface {
	IsLeader() bool
}

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

// PortfolioSnapshotter records daily portfolio archive snapshots for all users,
// preserving a continuous private portfolio-summary archive for owner analytics.
// It is idempotent per UTC day, so calling it every tick writes at most one
// snapshot per user per day. Implemented by an adapter over the portfolio and
// auth services.
type PortfolioSnapshotter interface {
	SnapshotAllDaily(ctx context.Context) (recorded int, err error)
}

// ExploreProjectionRefresher builds privacy-safe public discovery cards after
// the ranking projection refresh. Implemented by *profile.Service.
type ExploreProjectionRefresher interface {
	RefreshExploreProjection(ctx context.Context) (rows int, err error)
}

// Worker periodically runs all maintenance jobs. Each job is independent: one
// failing job never prevents the others from running.
type Worker struct {
	global    GlobalLeaderboardRefresher
	sprints   SprintMaintainer
	snapshots PortfolioSnapshotter       // optional
	explore   ExploreProjectionRefresher // optional
	interval  time.Duration
	leader    Leader // optional; nil means "always leader"
}

// NewWorker wires a Worker that runs every interval.
func NewWorker(global GlobalLeaderboardRefresher, sprints SprintMaintainer, interval time.Duration) *Worker {
	return &Worker{global: global, sprints: sprints, interval: interval}
}

// SetPortfolioSnapshotter attaches an optional daily-snapshot job. When set, the
// worker records daily portfolio snapshots on every pass (idempotent per day).
func (w *Worker) SetPortfolioSnapshotter(s PortfolioSnapshotter) {
	w.snapshots = s
}

// SetExploreProjectionRefresher attaches the off-request Explore projection
// build. It runs after the global ranking refresh so cards use the newest
// complete ranking generation.
func (w *Worker) SetExploreProjectionRefresher(r ExploreProjectionRefresher) {
	w.explore = r
}

// SetLeaderElector attaches the coordination gate that keeps these jobs
// running on exactly one replica at a time. Every job this Worker runs does a
// full unpaginated pass over all users with no per-row claiming (unlike the
// outbox and ranked-snapshot workers), so two replicas both running it
// doubles leaderboard-cache writes and daily-snapshot valuation load for no
// benefit.
func (w *Worker) SetLeaderElector(l Leader) {
	w.leader = l
}

// Start runs the jobs immediately, then on every tick until ctx is cancelled.
// The returned channel closes when the worker has fully stopped. Each pass is
// skipped (logged, not executed) while this process is not the leader — see
// SetLeaderElector — so a non-leader replica just idles rather than doing
// redundant work.
func (w *Worker) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		slog.Info("background worker started", "interval", w.interval.String())
		w.runIfLeader(ctx)

		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("background worker stopped")
				return
			case <-ticker.C:
				w.runIfLeader(ctx)
			}
		}
	}()
	return done
}

func (w *Worker) runIfLeader(ctx context.Context) {
	if w.leader != nil && !w.leader.IsLeader() {
		slog.Debug("job: skipped, not leader")
		return
	}
	w.RunOnce(ctx)
}

// RunOnce executes every job a single time. Exported so startup and tests can
// trigger a full pass synchronously.
func (w *Worker) RunOnce(ctx context.Context) {
	w.ensureSprint(ctx)
	w.refreshGlobal(ctx)
	w.refreshExplore(ctx)
	w.refreshSprints(ctx)
	w.recordSnapshots(ctx)
}

func (w *Worker) refreshExplore(ctx context.Context) {
	if w.explore == nil {
		return
	}
	rows, err := w.explore.RefreshExploreProjection(ctx)
	if err != nil {
		slog.Error("job: explore projection refresh failed", "error", err)
		return
	}
	slog.Info("job: explore projection refreshed", "rows", rows)
}

func (w *Worker) recordSnapshots(ctx context.Context) {
	if w.snapshots == nil {
		return
	}
	recorded, err := w.snapshots.SnapshotAllDaily(ctx)
	if err != nil {
		slog.Error("job: daily portfolio snapshots failed", "error", err)
		return
	}
	if recorded > 0 {
		slog.Info("job: daily portfolio snapshots recorded", "count", recorded)
	}
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
