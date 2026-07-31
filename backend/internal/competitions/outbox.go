package competitions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const CompetitionFinalizedEvent = "competition.finalized"
const competitionOutboxMaxAttempts = 10

type CompetitionOutboxEvent struct {
	ID, EventType, CompetitionID string
	ParticipantIDs               []string
	AttemptCount                 int
}

type CompetitionOutboxStore struct{ pool *pgxpool.Pool }

func NewCompetitionOutboxStore(pool *pgxpool.Pool) *CompetitionOutboxStore {
	return &CompetitionOutboxStore{pool: pool}
}

func (s *CompetitionOutboxStore) Claim(ctx context.Context, limit int) ([]CompetitionOutboxEvent, error) {
	rows, err := s.pool.Query(ctx, `UPDATE competition_outbox SET claimed_at=now(),attempt_count=attempt_count+1
		WHERE id IN (SELECT id FROM competition_outbox WHERE processed_at IS NULL AND dead_lettered_at IS NULL
		AND (next_attempt_at IS NULL OR next_attempt_at<=now()) AND (claimed_at IS NULL OR claimed_at<now()-interval '5 minutes')
		ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $1)
		RETURNING id::text,event_type,competition_id,participant_ids,attempt_count`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CompetitionOutboxEvent
	for rows.Next() {
		var e CompetitionOutboxEvent
		var raw []byte
		if err := rows.Scan(&e.ID, &e.EventType, &e.CompetitionID, &raw, &e.AttemptCount); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &e.ParticipantIDs); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *CompetitionOutboxStore) Processed(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE competition_outbox SET processed_at=now(),claimed_at=NULL,last_error=NULL WHERE id=$1`, id)
	return err
}
func (s *CompetitionOutboxStore) Failed(ctx context.Context, id, cause string) error {
	_, err := s.pool.Exec(ctx, `UPDATE competition_outbox SET claimed_at=NULL,last_error=$2,
	dead_lettered_at=CASE WHEN attempt_count >= $3 THEN now() ELSE dead_lettered_at END,
	next_attempt_at=CASE WHEN attempt_count >= $3 THEN NULL ELSE now()+least(interval '1 hour', interval '1 second'*power(2,attempt_count)) END WHERE id=$1 AND processed_at IS NULL`, id, cause, competitionOutboxMaxAttempts)
	return err
}
func (s *CompetitionOutboxStore) Backlog(ctx context.Context) (int64, time.Duration, error) {
	var n int64
	var oldest *time.Time
	if err := s.pool.QueryRow(ctx, `SELECT count(*),min(created_at) FROM competition_outbox WHERE processed_at IS NULL AND dead_lettered_at IS NULL`).Scan(&n, &oldest); err != nil {
		return 0, 0, err
	}
	if oldest == nil {
		return n, 0, nil
	}
	return n, time.Since(*oldest), nil
}

type competitionOutboxSource interface {
	Claim(context.Context, int) ([]CompetitionOutboxEvent, error)
	Processed(context.Context, string) error
	Failed(context.Context, string, string) error
}

type CompetitionAchievementProjector struct {
	store        competitionOutboxSource
	achievements FinalizationAchievementTrigger
	interval     time.Duration
	once         sync.Once
	running      atomic.Bool
}

func NewCompetitionAchievementProjector(store competitionOutboxSource, a FinalizationAchievementTrigger, interval time.Duration) *CompetitionAchievementProjector {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &CompetitionAchievementProjector{store: store, achievements: a, interval: interval}
}
func (p *CompetitionAchievementProjector) Start(ctx context.Context) {
	p.once.Do(func() {
		go func() {
			p.running.Store(true)
			defer p.running.Store(false)
			t := time.NewTicker(p.interval)
			defer t.Stop()
			for {
				if err := p.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("competition achievement outbox failed", "error", err)
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}()
	})
}
func (p *CompetitionAchievementProjector) Running() bool { return p.running.Load() }
func (p *CompetitionAchievementProjector) ProcessOnce(ctx context.Context) error {
	events, err := p.store.Claim(ctx, 100)
	if err != nil {
		return err
	}
	for _, e := range events {
		err = p.project(ctx, e)
		if err != nil {
			if markErr := p.store.Failed(ctx, e.ID, err.Error()); markErr != nil {
				slog.Error("settle competition outbox failure", "error", markErr)
			}
			continue
		}
		if err = p.store.Processed(ctx, e.ID); err != nil {
			return err
		}
	}
	return nil
}
func (p *CompetitionAchievementProjector) project(ctx context.Context, e CompetitionOutboxEvent) error {
	if e.EventType != CompetitionFinalizedEvent {
		return nil
	}
	for _, userID := range e.ParticipantIDs {
		if err := p.achievements.EvaluateCompetitionFinalizationAchievements(ctx, userID, e.CompetitionID); err != nil {
			return fmt.Errorf("participant %s: %w", userID, err)
		}
	}
	return nil
}
