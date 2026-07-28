package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// EventSource claims and settles transactional-outbox events. Implemented by the
// portfolio repositories (Postgres uses FOR UPDATE SKIP LOCKED, so multiple
// application instances never process the same event concurrently).
type EventSource interface {
	ClaimOutboxEvents(ctx context.Context, limit int) ([]portfolio.OutboxEvent, error)
	MarkOutboxProcessed(ctx context.Context, id string) error
	MarkOutboxFailed(ctx context.Context, id string, cause string) error
}

type BacklogSource interface {
	OutboxBacklog(ctx context.Context) (pending int64, oldestAge time.Duration, err error)
}

func CheckProjectorReadiness(ctx context.Context, running bool, backlog BacklogSource, maxPending int64, maxAge time.Duration) error {
	if !running {
		return fmt.Errorf("projector is not running")
	}
	pending, age, err := backlog.OutboxBacklog(ctx)
	if err != nil {
		return err
	}
	if pending > maxPending || age > maxAge {
		return fmt.Errorf("outbox backlog degraded: pending=%d oldest_age=%s", pending, age)
	}
	return nil
}

// RankedCache is the leaderboard cache surface the processor keeps in sync.
type RankedCache interface {
	UpsertGlobalScore(ctx context.Context, userID string, score money.Ratio) error
	RemoveGlobalScore(ctx context.Context, userID string) error
}

type RankedCacheState struct {
	Active bool
	Score  money.Ratio
}

// RankedCacheStateProvider rereads the current database-backed ranking state.
// Outbox delivery order is deliberately not authoritative: an old pause event
// processed after a newer resume must still project the current active state.
type RankedCacheStateProvider interface {
	CurrentRankedCacheState(ctx context.Context, userID string) (RankedCacheState, bool, error)
}

// SnapshotRecorder records the derived daily archive snapshot. Insertion is
// idempotent per portfolio per UTC day (database-enforced), so replaying an
// event is harmless.
type SnapshotRecorder interface {
	RecordDailySnapshot(ctx context.Context, userID string) (bool, error)
}

type RankedSnapshotRecorder interface {
	RecordMutationSnapshot(ctx context.Context, event portfolio.OutboxEvent) error
}

// AchievementEvaluator is retained as an optional compatibility projection.
// Canonical badge evaluation is queued by newly committed ranked snapshots.
type AchievementEvaluator interface {
	EvaluateAllQuiet(ctx context.Context, userID string) error
}

// OutboxProcessor performs the DERIVED work of a committed portfolio mutation:
// leaderboard cache sync, lifecycle snapshots, and private daily archives.
//
// This work is deliberately outside the mutation transaction — a failing
// projection must never fail or roll back a portfolio mutation — but it is not
// best-effort either: every event is durable, claimed exclusively, retried on
// failure, and safe to reprocess.
type OutboxProcessor struct {
	source          EventSource
	cache           RankedCache // optional (Redis only)
	cacheState      RankedCacheStateProvider
	snapshots       SnapshotRecorder       // optional
	rankedSnapshots RankedSnapshotRecorder // optional
	achievements    AchievementEvaluator   // optional
	batchSize       int
	interval        time.Duration
	startOnce       sync.Once
	running         atomic.Bool
}

func NewOutboxProcessor(source EventSource, interval time.Duration) *OutboxProcessor {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &OutboxProcessor{source: source, batchSize: 100, interval: interval}
}

func (p *OutboxProcessor) SetCache(c RankedCache, state RankedCacheStateProvider) {
	p.cache, p.cacheState = c, state
}
func (p *OutboxProcessor) SetSnapshotRecorder(s SnapshotRecorder) { p.snapshots = s }
func (p *OutboxProcessor) SetRankedSnapshotRecorder(s RankedSnapshotRecorder) {
	p.rankedSnapshots = s
}
func (p *OutboxProcessor) SetAchievements(a AchievementEvaluator) { p.achievements = a }

// Start runs the processor until ctx is cancelled.
func (p *OutboxProcessor) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		go func() {
			p.running.Store(true)
			defer p.running.Store(false)
			ticker := time.NewTicker(p.interval)
			defer ticker.Stop()
			for {
				if _, err := p.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("outbox processing failed", "error", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (p *OutboxProcessor) Running() bool { return p.running.Load() }

// ProcessOnce claims a batch and processes it, returning how many events were
// settled successfully. A failing event is marked for retry and does not block
// the rest of the batch.
func (p *OutboxProcessor) ProcessOnce(ctx context.Context) (int, error) {
	events, err := p.source.ClaimOutboxEvents(ctx, p.batchSize)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, ev := range events {
		if err := p.handle(ctx, ev); err != nil {
			// Durable retry: the event stays unprocessed and is claimed again.
			if markErr := p.source.MarkOutboxFailed(ctx, ev.ID, err.Error()); markErr != nil {
				slog.Warn("marking outbox event failed", "id", ev.ID, "error", markErr)
			}
			continue
		}
		if err := p.source.MarkOutboxProcessed(ctx, ev.ID); err != nil {
			slog.Warn("marking outbox event processed", "id", ev.ID, "error", err)
			continue
		}
		processed++
	}
	return processed, nil
}

// handle applies one event's derived effects. Every step is idempotent, so
// reprocessing the same event converges on the same state.
func (p *OutboxProcessor) handle(ctx context.Context, ev portfolio.OutboxEvent) error {
	if ev.EventType != portfolio.EventPortfolioMutated {
		return nil // unknown event types are settled, not retried forever
	}

	// 1. Leaderboard cache. A paused portfolio must be REMOVED, not merely left
	//    stale, or an unrankable user stays visible on the board.
	if p.cache != nil && p.cacheState != nil {
		current, found, stateErr := p.cacheState.CurrentRankedCacheState(ctx, ev.UserID)
		if stateErr != nil {
			slog.Error("leaderboard_cache_update_failed",
				"user_id", ev.UserID, "aggregate_version", ev.AggregateVersion, "error", stateErr)
			return stateErr
		}
		var err error
		if !found || !current.Active {
			err = p.cache.RemoveGlobalScore(ctx, ev.UserID)
			if err == nil {
				slog.Info("leaderboard_cache_score_removed",
					"user_id", ev.UserID, "aggregate_version", ev.AggregateVersion)
			}
		} else {
			err = p.cache.UpsertGlobalScore(ctx, ev.UserID, current.Score)
			if err == nil {
				slog.Info("leaderboard_cache_score_upserted",
					"user_id", ev.UserID, "aggregate_version", ev.AggregateVersion)
			}
		}
		if err != nil {
			// Redis is a cache, never a source of truth: the committed database
			// state stands and the event is retried.
			slog.Error("leaderboard_cache_update_failed",
				"user_id", ev.UserID, "aggregate_version", ev.AggregateVersion, "error", err)
			return err
		}
	}

	// 2. Derived projections. Their failure is logged and retried via the event,
	//    but it can never undo the committed mutation.
	if p.rankedSnapshots != nil {
		if err := p.rankedSnapshots.RecordMutationSnapshot(ctx, ev); err != nil {
			return err
		}
	}
	if p.snapshots != nil {
		if _, err := p.snapshots.RecordDailySnapshot(ctx, ev.UserID); err != nil {
			return err
		}
	}
	if p.achievements != nil {
		if err := p.achievements.EvaluateAllQuiet(ctx, ev.UserID); err != nil {
			return err
		}
	}
	return nil
}
