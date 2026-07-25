package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// fakeCache is an in-memory stand-in for the Redis sorted set, with an
// injectable failure to prove Redis outages never corrupt core state.
type fakeCache struct {
	mu     sync.Mutex
	scores map[string]float64
	fail   error
}

func newFakeCache() *fakeCache { return &fakeCache{scores: map[string]float64{}} }

func (c *fakeCache) UpsertGlobalScore(_ context.Context, userID string, score float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	c.scores[userID] = score
	return nil
}

func (c *fakeCache) RemoveGlobalScore(_ context.Context, userID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail != nil {
		return c.fail
	}
	delete(c.scores, userID)
	return nil
}

func (c *fakeCache) has(userID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.scores[userID]
	return ok
}

type fakeRankedCacheState struct {
	mu     sync.Mutex
	states map[string]RankedCacheState
	err    error
}

func newFakeRankedCacheState() *fakeRankedCacheState {
	return &fakeRankedCacheState{states: map[string]RankedCacheState{}}
}

func (s *fakeRankedCacheState) CurrentRankedCacheState(_ context.Context, userID string) (RankedCacheState, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return RankedCacheState{}, false, s.err
	}
	state, found := s.states[userID]
	return state, found, nil
}

func (s *fakeRankedCacheState) set(userID string, active bool, score float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[userID] = RankedCacheState{Active: active, Score: score}
}

// fakeSource is a minimal outbox: claim-once semantics plus settle tracking.
type fakeSource struct {
	mu        sync.Mutex
	events    []portfolio.OutboxEvent
	claimed   map[string]bool
	processed map[string]bool
	failed    map[string]string
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
		UserID: userID, RankedIndex: index, RankingStatus: string(status),
	}
}

func TestOutbox_ActiveUserAppearsInCache(t *testing.T) {
	cache := newFakeCache()
	state := newFakeRankedCacheState()
	state.set("u1", true, 30)
	src := newFakeSource(mutatedEvent("e1", "u1", 130, portfolio.RankingStatusActive))
	p := NewOutboxProcessor(src, 0)
	p.SetCache(cache, state)

	n, err := p.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.True(t, cache.has("u1"))
	assert.Equal(t, 30.0, cache.scores["u1"], "score is the ranked return, not the index")
}

func TestOutbox_PausingRemovesUserAndResumingRestores(t *testing.T) {
	cache := newFakeCache()
	state := newFakeRankedCacheState()
	ctx := context.Background()

	// Active -> present.
	state.set("u1", true, 30)
	p := NewOutboxProcessor(newFakeSource(mutatedEvent("e1", "u1", 130, portfolio.RankingStatusActive)), 0)
	p.SetCache(cache, state)
	_, err := p.ProcessOnce(ctx)
	require.NoError(t, err)
	require.True(t, cache.has("u1"))

	// Paused (portfolio emptied) -> must be REMOVED, not left stale.
	state.set("u1", false, 30)
	p = NewOutboxProcessor(newFakeSource(mutatedEvent("e2", "u1", 130, portfolio.RankingStatusPaused)), 0)
	p.SetCache(cache, state)
	_, err = p.ProcessOnce(ctx)
	require.NoError(t, err)
	assert.False(t, cache.has("u1"), "a paused user must not remain visible on the board")

	// Resumed -> restored.
	state.set("u1", true, 30)
	p = NewOutboxProcessor(newFakeSource(mutatedEvent("e3", "u1", 130, portfolio.RankingStatusActive)), 0)
	p.SetCache(cache, state)
	_, err = p.ProcessOnce(ctx)
	require.NoError(t, err)
	assert.True(t, cache.has("u1"))
}

func TestOutbox_CacheFailureRetriesAndDoesNotLoseWork(t *testing.T) {
	cache := newFakeCache()
	state := newFakeRankedCacheState()
	state.set("u1", true, 30)
	cache.fail = errors.New("redis down")
	src := newFakeSource(mutatedEvent("e1", "u1", 130, portfolio.RankingStatusActive))
	p := NewOutboxProcessor(src, 0)
	p.SetCache(cache, state)

	n, err := p.ProcessOnce(context.Background())
	require.NoError(t, err, "a cache outage must not fail the whole batch")
	assert.Equal(t, 0, n)
	assert.NotEmpty(t, src.failed["e1"], "the event is recorded as failed for retry")
	assert.False(t, src.processed["e1"], "the event must NOT be settled")

	// Redis recovers: the retry succeeds and no work was lost.
	cache.fail = nil
	n, err = p.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.True(t, cache.has("u1"))
}

func TestOutbox_PauseRemovalFailureKeepsCommitAndRetryRepairsCache(t *testing.T) {
	cache := newFakeCache()
	cache.scores["u1"] = 12
	state := newFakeRankedCacheState()
	state.set("u1", false, 12) // database commit already paused successfully
	cache.fail = errors.New("redis down")
	src := newFakeSource(mutatedEvent("pause", "u1", 112, portfolio.RankingStatusPaused))
	p := NewOutboxProcessor(src, 0)
	p.SetCache(cache, state)

	n, err := p.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.True(t, cache.has("u1"), "failed Redis removal cannot alter committed database state")
	assert.NotEmpty(t, src.failed["pause"])

	cache.fail = nil
	n, err = p.ProcessOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.False(t, cache.has("u1"), "durable retry must eventually apply ZREM")
}

func TestOutbox_ReprocessingSameEventIsHarmless(t *testing.T) {
	cache := newFakeCache()
	state := newFakeRankedCacheState()
	state.set("u1", true, 42)
	ev := mutatedEvent("e1", "u1", 142, portfolio.RankingStatusActive)
	p := NewOutboxProcessor(newFakeSource(ev), 0)
	p.SetCache(cache, state)
	ctx := context.Background()

	require.NoError(t, p.handle(ctx, ev))
	first := cache.scores["u1"]
	require.NoError(t, p.handle(ctx, ev))
	require.NoError(t, p.handle(ctx, ev))
	assert.Equal(t, first, cache.scores["u1"], "replaying an event converges on the same state")
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
			state := newFakeRankedCacheState()
			state.set("u1", true, 10)
			p.SetCache(newFakeCache(), state)
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

func TestOutbox_OutOfOrderEventsAlwaysProjectCurrentState(t *testing.T) {
	cache := newFakeCache()
	state := newFakeRankedCacheState()
	ctx := context.Background()
	p := NewOutboxProcessor(newFakeSource(), 0)
	p.SetCache(cache, state)

	// Current database state is resumed/active. Even a later-delivered old pause
	// event must upsert the current score rather than remove the user.
	state.set("u1", true, 12)
	resume := mutatedEvent("resume-11", "u1", 112, portfolio.RankingStatusActive)
	resume.AggregateVersion = 11
	pause := mutatedEvent("pause-10", "u1", 112, portfolio.RankingStatusPaused)
	pause.AggregateVersion = 10
	require.NoError(t, p.handle(ctx, resume))
	require.NoError(t, p.handle(ctx, pause))
	assert.True(t, cache.has("u1"))
	assert.Equal(t, 12.0, cache.scores["u1"])

	// Current database state then advances to paused. A stale resume event cannot
	// re-add the member because the projector rereads current state.
	state.set("u1", false, 12)
	newPause := mutatedEvent("pause-12", "u1", 112, portfolio.RankingStatusPaused)
	newPause.AggregateVersion = 12
	require.NoError(t, p.handle(ctx, newPause))
	require.NoError(t, p.handle(ctx, resume))
	assert.False(t, cache.has("u1"))
}

func TestOutbox_DeletedUserEventRemovesCachedMember(t *testing.T) {
	cache := newFakeCache()
	cache.scores["deleted"] = 20
	state := newFakeRankedCacheState() // absent means deleted/nonexistent
	p := NewOutboxProcessor(newFakeSource(), 0)
	p.SetCache(cache, state)

	require.NoError(t, p.handle(context.Background(),
		mutatedEvent("old", "deleted", 120, portfolio.RankingStatusActive)))
	assert.False(t, cache.has("deleted"))
}
