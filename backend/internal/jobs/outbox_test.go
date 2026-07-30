package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// fakeSource is a minimal outbox: claim-once semantics plus settle tracking.
type fakeSource struct {
	mu        sync.Mutex
	events    []portfolio.OutboxEvent
	claimed   map[string]bool
	processed map[string]bool
	failed    map[string]string
	claims    int
}

type fakeRankedRecorder struct {
	mu       sync.Mutex
	recorded map[string]portfolio.OutboxEvent
	fail     error
}

func (r *fakeRankedRecorder) RecordMutationSnapshot(_ context.Context, event portfolio.OutboxEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail != nil {
		return r.fail
	}
	if r.recorded == nil {
		r.recorded = map[string]portfolio.OutboxEvent{}
	}
	r.recorded[event.ID] = event
	return nil
}

func (r *fakeRankedRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.recorded)
}

func newFakeSource(events ...portfolio.OutboxEvent) *fakeSource {
	return &fakeSource{
		events: events, claimed: map[string]bool{},
		processed: map[string]bool{}, failed: map[string]string{},
	}
}

func (s *fakeSource) ClaimOutboxEvents(_ context.Context, limit int) ([]portfolio.OutboxEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claims++
	out := make([]portfolio.OutboxEvent, 0, limit)
	for _, ev := range s.events {
		if len(out) >= limit || s.claimed[ev.ID] || s.processed[ev.ID] {
			continue
		}
		s.claimed[ev.ID] = true
		out = append(out, ev)
	}
	return out, nil
}

func (s *fakeSource) MarkOutboxProcessed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processed[id] = true
	delete(s.claimed, id)
	return nil
}

func (s *fakeSource) MarkOutboxFailed(_ context.Context, id, cause string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed[id] = cause
	delete(s.claimed, id) // released for retry
	return nil
}

func mutatedEvent(id, userID string, index float64, status portfolio.RankingStatus) portfolio.OutboxEvent {
	return portfolio.OutboxEvent{
		ID: id, EventType: portfolio.EventPortfolioMutated,
		AggregateType: "portfolio", AggregateID: "pf-1", AggregateVersion: 1,
		UserID: userID, RankedIndex: money.IndexValueFromFloat64(index), RankingStatus: string(status),
	}
}

func TestOutbox_CommittedMutationProjectsRankedHistoryIdempotently(t *testing.T) {
	ev := mutatedEvent("ranked-projection", "u1", 142, portfolio.RankingStatusActive)
	src := newFakeSource(ev)
	recorder := &fakeRankedRecorder{}
	p := NewOutboxProcessor(src, 0)
	p.SetRankedSnapshotRecorder(recorder)

	processed, err := p.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.True(t, src.processed[ev.ID])
	require.Equal(t, 1, recorder.count())

	require.NoError(t, p.handle(context.Background(), ev))
	require.NoError(t, p.handle(context.Background(), ev))
	assert.Equal(t, 1, recorder.count(), "reprocessing converges on one canonical projection")
}

func TestOutbox_ProjectorFailureDoesNotCompleteEventAndRetries(t *testing.T) {
	ev := mutatedEvent("ranked-failure", "u1", 142, portfolio.RankingStatusActive)
	src := newFakeSource(ev)
	recorder := &fakeRankedRecorder{fail: errors.New("ranked history unavailable")}
	p := NewOutboxProcessor(src, 0)
	p.SetRankedSnapshotRecorder(recorder)

	processed, err := p.ProcessOnce(context.Background())
	require.NoError(t, err, "a projector outage must not fail the whole batch")
	assert.Zero(t, processed)
	assert.False(t, src.processed[ev.ID], "the event must NOT be settled")
	assert.Contains(t, src.failed[ev.ID], "ranked history unavailable")

	// The dependency recovers: the durable retry succeeds and no work is lost.
	recorder.fail = nil
	processed, err = p.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 1, recorder.count())
}

func TestOutbox_ConcurrentProcessorsNeverDoubleClaim(t *testing.T) {
	events := make([]portfolio.OutboxEvent, 0, 20)
	for i := 0; i < 20; i++ {
		events = append(events, mutatedEvent(string(rune('a'+i)), "u1", 110, portfolio.RankingStatusActive))
	}
	src := newFakeSource(events...)

	var wg sync.WaitGroup
	totals := make([]int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p := NewOutboxProcessor(src, 0)
			p.SetRankedSnapshotRecorder(&fakeRankedRecorder{})
			totals[i], _ = p.ProcessOnce(context.Background())
		}(i)
	}
	wg.Wait()

	sum := 0
	for _, n := range totals {
		sum += n
	}
	assert.Equal(t, len(events), sum, "each event must be processed exactly once across workers")
}

func TestOutbox_StartIsGuardedAgainstDuplicateLoops(t *testing.T) {
	src := newFakeSource()
	p := NewOutboxProcessor(src, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx)
	p.Start(ctx)

	deadline := time.After(time.Second)
	for {
		src.mu.Lock()
		claims := src.claims
		src.mu.Unlock()
		if p.Running() && claims > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("projector did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	cancel()

	deadline = time.After(time.Second)
	for p.Running() {
		select {
		case <-deadline:
			t.Fatal("projector did not stop")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	assert.Equal(t, 1, src.claims, "calling Start twice must not create a second loop")
}

type fixedBacklog struct {
	pending int64
	age     time.Duration
}

func (b fixedBacklog) OutboxBacklog(context.Context) (int64, time.Duration, error) {
	return b.pending, b.age, nil
}

func TestProjectorReadinessDegradesForBacklogThresholds(t *testing.T) {
	err := CheckProjectorReadiness(context.Background(), true,
		fixedBacklog{pending: 11, age: time.Minute}, 10, 15*time.Minute)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backlog degraded")

	err = CheckProjectorReadiness(context.Background(), true,
		fixedBacklog{pending: 1, age: 16 * time.Minute}, 10, 15*time.Minute)
	require.Error(t, err)

	require.NoError(t, CheckProjectorReadiness(context.Background(), true,
		fixedBacklog{pending: 1, age: time.Minute}, 10, 15*time.Minute))
	require.Error(t, CheckProjectorReadiness(context.Background(), false,
		fixedBacklog{}, 10, 15*time.Minute))
}
