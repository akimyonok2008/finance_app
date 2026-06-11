package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGlobal struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeGlobal) RefreshCache(_ context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return 1, f.err
}

func (f *fakeGlobal) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

type fakeSprints struct {
	mu       sync.Mutex
	ensured  int
	refreshs map[string]int
	active   []string
}

func (f *fakeSprints) EnsureCurrentSprint(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensured++
	return nil
}

func (f *fakeSprints) ListActiveCompetitionIDs(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active, nil
}

func (f *fakeSprints) RefreshCache(_ context.Context, competitionID string) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.refreshs == nil {
		f.refreshs = map[string]int{}
	}
	f.refreshs[competitionID]++
	return 0, nil
}

func TestWorker_RunOnceExecutesAllJobs(t *testing.T) {
	global := &fakeGlobal{}
	sprints := &fakeSprints{active: []string{"weekly_2026_24", "weekly_2026_25"}}
	w := NewWorker(global, sprints, time.Minute)

	w.RunOnce(context.Background())

	assert.Equal(t, 1, sprints.ensured, "sprint existence job must run")
	assert.Equal(t, 1, global.count(), "global leaderboard job must run")
	assert.Equal(t, 1, sprints.refreshs["weekly_2026_24"])
	assert.Equal(t, 1, sprints.refreshs["weekly_2026_25"])
}

func TestWorker_RunOnceSurvivesJobErrors(t *testing.T) {
	global := &fakeGlobal{err: errors.New("redis down")}
	sprints := &fakeSprints{active: []string{"weekly_2026_24"}}
	w := NewWorker(global, sprints, time.Minute)

	// Must not panic, and later jobs must still run despite the earlier error.
	w.RunOnce(context.Background())
	assert.Equal(t, 1, sprints.refreshs["weekly_2026_24"])
}

func TestWorker_StartTicksAndStops(t *testing.T) {
	global := &fakeGlobal{}
	sprints := &fakeSprints{}
	w := NewWorker(global, sprints, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	done := w.Start(ctx)

	require.Eventually(t, func() bool { return global.count() >= 2 },
		2*time.Second, 5*time.Millisecond, "worker should tick repeatedly")

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after context cancellation")
	}
}
