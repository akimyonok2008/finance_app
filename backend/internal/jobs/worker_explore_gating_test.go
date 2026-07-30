package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// promotingGlobal is a GlobalLeaderboardRefresher that also implements
// GenerationPromotionReporter, so tests can drive the Explore gating.
type promotingGlobal struct {
	calls    int
	promoted bool
}

func (f *promotingGlobal) RefreshCache(context.Context) (int, error) {
	f.calls++
	return 0, nil
}

func (f *promotingGlobal) LastRefreshPromotedGeneration() bool { return f.promoted }

func TestWorker_ExploreRefreshGatedOnGenerationPromotion(t *testing.T) {
	global := &promotingGlobal{}
	w := NewWorker(global, &fakeSprints{}, time.Minute)
	explore := &fakeExplore{}
	w.SetExploreProjectionRefresher(explore)
	ctx := context.Background()

	// First pass after startup always rebuilds, promotion or not: the
	// previous process may have promoted a generation this one never saw.
	w.RunOnce(ctx)
	assert.Equal(t, 1, explore.calls, "first pass must rebuild Explore")

	// No new generation: the population-wide rebuild is skipped.
	w.RunOnce(ctx)
	w.RunOnce(ctx)
	assert.Equal(t, 1, explore.calls, "no promotion means no rebuild")

	// A promoted generation is new input: rebuild once.
	global.promoted = true
	w.RunOnce(ctx)
	assert.Equal(t, 2, explore.calls, "promotion must trigger a rebuild")
	global.promoted = false
	w.RunOnce(ctx)
	assert.Equal(t, 2, explore.calls)

	// A failed rebuild retries on the next tick even without a fresh
	// promotion, instead of waiting a full cycle.
	global.promoted = true
	explore.err = errors.New("db down")
	w.RunOnce(ctx)
	assert.Equal(t, 3, explore.calls)
	global.promoted = false
	explore.err = nil
	w.RunOnce(ctx)
	assert.Equal(t, 4, explore.calls, "failure must retry next tick")
	w.RunOnce(ctx)
	assert.Equal(t, 4, explore.calls, "successful retry clears the pending flag")
}
