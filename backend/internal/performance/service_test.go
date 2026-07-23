package performance

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeValuator lets tests drive the current portfolio value deterministically.
type fakeValuator struct {
	portfolioID string
	valueBase   float64
	hasActive   bool
	err         error
}

func (f *fakeValuator) PortfolioValueBase(_ context.Context, _ string) (string, float64, bool, error) {
	if f.err != nil {
		return "", 0, false, f.err
	}
	return f.portfolioID, f.valueBase, f.hasActive, nil
}

func newService(t *testing.T) (*Service, *InMemoryRepository, *fakeValuator) {
	t.Helper()
	repo := NewInMemoryRepository()
	val := &fakeValuator{portfolioID: "pf-1", valueBase: 100, hasActive: true}
	svc := NewService(repo)
	svc.SetValuator(val)
	svc.SetClock(func() time.Time { return time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC) })
	return svc, repo, val
}

// index reads the current ranked index for the fixed portfolio at the given value.
func index(t *testing.T, svc *Service, val *fakeValuator, value float64, hasActive bool) float64 {
	t.Helper()
	val.valueBase = value
	val.hasActive = hasActive
	rp, err := svc.CurrentRankedPerformance(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("CurrentRankedPerformance: %v", err)
	}
	return rp.RankedIndex
}

func checkpoint(t *testing.T, svc *Service, before float64, hadActive bool, after float64, hasActive bool) {
	t.Helper()
	err := svc.Checkpoint(context.Background(), CheckpointInput{
		PortfolioID: "pf-1", UserID: "u-1",
		ValueBeforeBase: before, HasActiveBefore: hadActive,
		ValueAfterBase: after, HasActiveAfter: hasActive,
		At: time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
}

func TestFirstPositionStartsAt100(t *testing.T) {
	svc, _, val := newService(t)
	// First activation: portfolio worth 100.
	checkpoint(t, svc, 0, false, 100, true)
	if got := index(t, svc, val, 100, true); got != 100 {
		t.Fatalf("first position index = %v, want 100", got)
	}
}

func TestReadWithoutStateIsSynthetic(t *testing.T) {
	svc, _, val := newService(t)
	// No checkpoint written yet: a non-empty portfolio reads as active 100.
	if got := index(t, svc, val, 500, true); got != 100 {
		t.Fatalf("synthetic index = %v, want 100", got)
	}
	// An empty portfolio reads as paused 100.
	rp, err := svc.CurrentRankedPerformance(context.Background(), "u-1")
	_ = rp
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	val.hasActive = false
	rp, _ = svc.CurrentRankedPerformance(context.Background(), "u-1")
	if rp.Status != StatusPaused || rp.RankedIndex != 100 {
		t.Fatalf("empty synthetic = %+v, want paused 100", rp)
	}
}

func TestMarketGainAndLoss(t *testing.T) {
	svc, _, val := newService(t)
	checkpoint(t, svc, 0, false, 100, true)
	if got := index(t, svc, val, 110, true); got != 110 {
		t.Fatalf("+10%% -> %v, want 110", got)
	}
	if got := index(t, svc, val, 80, true); got != 80 {
		t.Fatalf("-20%% -> %v, want 80", got)
	}
	// Repeated reads must not mutate state.
	if got := index(t, svc, val, 80, true); got != 80 {
		t.Fatalf("repeated read -> %v, want 80", got)
	}
}

func TestCapitalInjectionDoesNotDilute(t *testing.T) {
	svc, _, val := newService(t)
	checkpoint(t, svc, 0, false, 100, true) // activate at 100
	// Market falls to 50.
	if got := index(t, svc, val, 50, true); got != 50 {
		t.Fatalf("pre-injection index = %v, want 50", got)
	}
	// Add a new position worth 950: value 50 -> 1000.
	checkpoint(t, svc, 50, true, 1000, true)
	if got := index(t, svc, val, 1000, true); got != 50 {
		t.Fatalf("post-injection index = %v, want 50 (NOT 95.24)", got)
	}
	// Later the 1000 portfolio rises to 1100 -> index 55 (still -45%).
	if got := index(t, svc, val, 1100, true); got != 55 {
		t.Fatalf("post-injection gain index = %v, want 55", got)
	}
}

func TestQuantityIncreaseOnWinnerNoRetroactiveGain(t *testing.T) {
	svc, _, val := newService(t)
	checkpoint(t, svc, 0, false, 100, true) // 1 unit @ 100
	// Price rises to 110.
	if got := index(t, svc, val, 110, true); got != 110 {
		t.Fatalf("winner index = %v, want 110", got)
	}
	// Quantity x1000: value 110 -> 110000, same current prices.
	checkpoint(t, svc, 110, true, 110000, true)
	if got := index(t, svc, val, 110000, true); got != 110 {
		t.Fatalf("post-resize index = %v, want 110", got)
	}
}

func TestQuantityDecreaseNoGainOrLoss(t *testing.T) {
	svc, _, val := newService(t)
	checkpoint(t, svc, 0, false, 200, true)
	_ = index(t, svc, val, 200, true)
	// Halve quantity: 200 -> 100 at current prices.
	checkpoint(t, svc, 200, true, 100, true)
	if got := index(t, svc, val, 100, true); got != 100 {
		t.Fatalf("post-decrease index = %v, want 100", got)
	}
}

func TestDeleteLosingPositionPreservesIndex(t *testing.T) {
	svc, _, val := newService(t)
	checkpoint(t, svc, 0, false, 100, true)
	// Two positions; total falls to 60. Delete one worth 20 -> 40 remains.
	if got := index(t, svc, val, 60, true); got != 60 {
		t.Fatalf("index = %v, want 60", got)
	}
	checkpoint(t, svc, 60, true, 40, true)
	if got := index(t, svc, val, 40, true); got != 60 {
		t.Fatalf("post-delete index = %v, want 60", got)
	}
}

func TestDeleteAndReAddDoesNotReset(t *testing.T) {
	svc, _, val := newService(t)
	checkpoint(t, svc, 0, false, 100, true)
	_ = index(t, svc, val, 80, true) // down to 80
	// Delete the only position -> empty -> paused at 80.
	checkpoint(t, svc, 80, true, 0, false)
	rp, _ := svc.CurrentRankedPerformance(context.Background(), "u-1")
	_ = rp
	// Re-add a fresh position worth 500.
	checkpoint(t, svc, 0, false, 500, true)
	if got := index(t, svc, val, 500, true); got != 80 {
		t.Fatalf("post re-add index = %v, want 80 (preserved, not reset to 100)", got)
	}
}

func TestEmptyPortfolioPausesAndResumes(t *testing.T) {
	svc, repo, val := newService(t)
	checkpoint(t, svc, 0, false, 100, true)
	_ = index(t, svc, val, 125, true) // +25%
	// Remove every position.
	checkpoint(t, svc, 125, true, 0, false)
	st, _ := repo.GetByPortfolio(context.Background(), "pf-1")
	if st.Status != StatusPaused {
		t.Fatalf("status = %v, want paused", st.Status)
	}
	if st.SegmentStartValueBase != nil {
		t.Fatalf("paused segment start = %v, want nil", *st.SegmentStartValueBase)
	}
	// Paused read reports preserved index regardless of value.
	val.hasActive = false
	rp, _ := svc.CurrentRankedPerformance(context.Background(), "u-1")
	if rp.RankedIndex != 125 || rp.Status != StatusPaused {
		t.Fatalf("paused perf = %+v, want 125 paused", rp)
	}
	// Resume with a new portfolio worth 3000.
	checkpoint(t, svc, 0, false, 3000, true)
	if got := index(t, svc, val, 3000, true); got != 125 {
		t.Fatalf("resumed index = %v, want 125", got)
	}
}

func TestStrategyReplacementPreservesIndex(t *testing.T) {
	svc, _, val := newService(t)
	checkpoint(t, svc, 0, false, 100, true)
	_ = index(t, svc, val, 90, true) // -10%
	// Replace whole portfolio (copy strategy): old value 90 -> new 5000.
	checkpoint(t, svc, 90, true, 5000, true)
	if got := index(t, svc, val, 5000, true); got != 90 {
		t.Fatalf("post strategy-copy index = %v, want 90 (not reset to 100)", got)
	}
}

func TestConcurrentCheckpointsSerialize(t *testing.T) {
	svc, repo, _ := newService(t)
	checkpoint(t, svc, 0, false, 100, true)
	ctx := context.Background()
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			errs <- svc.Checkpoint(ctx, CheckpointInput{
				PortfolioID: "pf-1", UserID: "u-1",
				ValueBeforeBase: 100, HasActiveBefore: true,
				ValueAfterBase: 200, HasActiveAfter: true,
				At: time.Now().UTC(),
			})
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent checkpoint failed: %v", err)
		}
	}
	st, _ := repo.GetByPortfolio(ctx, "pf-1")
	// Two successful serialized checkpoints after activation (version 1) -> 3.
	if st.Version != 3 {
		t.Fatalf("version = %d, want 3 (serialized, no lost update)", st.Version)
	}
}

func TestValuationFailurePropagates(t *testing.T) {
	svc, _, val := newService(t)
	val.err = errors.New("price provider down")
	if _, err := svc.CurrentRankedPerformance(context.Background(), "u-1"); err == nil {
		t.Fatal("expected error when valuation fails")
	}
}

func TestEnsureEpochIdempotent(t *testing.T) {
	svc, repo, val := newService(t)
	val.valueBase = 4200
	if err := svc.EnsureEpoch(context.Background(), "u-1"); err != nil {
		t.Fatalf("EnsureEpoch: %v", err)
	}
	st, _ := repo.GetByPortfolio(context.Background(), "pf-1")
	if st.CheckpointIndex != 100 || st.Status != StatusActive || *st.SegmentStartValueBase != 4200 {
		t.Fatalf("epoch state = %+v, want 100/active/4200", st)
	}
	epoch := st.TrackingStartedAt
	// Second call must not rewrite.
	val.valueBase = 9999
	if err := svc.EnsureEpoch(context.Background(), "u-1"); err != nil {
		t.Fatalf("EnsureEpoch 2: %v", err)
	}
	st2, _ := repo.GetByPortfolio(context.Background(), "pf-1")
	if !st2.TrackingStartedAt.Equal(epoch) || *st2.SegmentStartValueBase != 4200 {
		t.Fatalf("epoch rewritten: %+v", st2)
	}
}

func TestPausedStatePersistsInRepository(t *testing.T) {
	repo := NewInMemoryRepository()
	// Paused states carry a nil segment value; the repo must accept and return it.
	st := activate("pf-x", "u-x", 0, false, time.Now().UTC())
	if err := repo.Create(context.Background(), st); err != nil {
		t.Fatalf("create paused: %v", err)
	}
	got, err := repo.GetByPortfolio(context.Background(), "pf-x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusPaused || got.SegmentStartValueBase != nil {
		t.Fatalf("paused round-trip = %+v", got)
	}
}
