package performance

import (
	"math"
	"testing"
	"time"
)

// These tests cover the PURE ranked-performance transitions. They touch no
// database: every scenario is expressed as state + checkpoint input, which is
// exactly the contract the portfolio aggregate transaction depends on.

var epoch = time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)

func active(t *testing.T, checkpoint, segmentStart float64) State {
	t.Helper()
	v := segmentStart
	return State{
		PortfolioID: "pf-1", UserID: "u-1",
		CheckpointIndex: checkpoint, SegmentStartValueBase: &v,
		Status: StatusActive, TrackingStartedAt: epoch, Version: 1,
	}
}

// mutate applies a checkpoint and returns (index before, index after).
func mutate(prev State, valueBefore float64, hadActive bool, valueAfter float64, hasActive bool) (State, float64, float64) {
	in := CheckpointInput{
		PortfolioID: "pf-1", UserID: "u-1",
		ValueBeforeBase: valueBefore, HasActiveBefore: hadActive,
		ValueAfterBase: valueAfter, HasActiveAfter: hasActive,
		At: epoch.Add(time.Hour),
	}
	before := CalculateIndexBeforeMutation(prev, in)
	next := ApplyCheckpoint(prev, in)
	after := CalculateCurrentIndex(next, valueAfter)
	return next, before, after
}

const tol = 1e-9

func assertNeutral(t *testing.T, label string, before, after float64) {
	t.Helper()
	if math.Abs(before-after) > tol*math.Max(1, math.Abs(before)) {
		t.Fatalf("%s: mutation generated ranked return: before=%v after=%v", label, before, after)
	}
}

func TestActivate_FirstNonEmptyPortfolioStartsAt100(t *testing.T) {
	st := ActivateState("pf-1", "u-1", 250, true, epoch)
	if st.Status != StatusActive || st.CheckpointIndex != 100 {
		t.Fatalf("state = %+v, want active @100", st)
	}
	if got := CalculateCurrentIndex(st, 250); got != 100 {
		t.Fatalf("index = %v, want 100", got)
	}
}

func TestActivate_EmptyPortfolioStartsPausedAt100(t *testing.T) {
	st := ActivateState("pf-1", "u-1", 0, false, epoch)
	if st.Status != StatusPaused || st.SegmentStartValueBase != nil {
		t.Fatalf("state = %+v, want paused with nil segment", st)
	}
	// No division by zero, whatever value is supplied.
	if got := CalculateCurrentIndex(st, 0); got != 100 {
		t.Fatalf("index = %v, want 100", got)
	}
}

func TestMarketMovesChangeIndex(t *testing.T) {
	st := active(t, 100, 100)
	if got := CalculateCurrentIndex(st, 110); got != 110 {
		t.Fatalf("+10%% -> %v, want 110", got)
	}
	if got := CalculateCurrentIndex(st, 80); got != 80 {
		t.Fatalf("-20%% -> %v, want 80", got)
	}
}

func TestPerformanceReadModelPreservesSnapshotPrecision(t *testing.T) {
	st := active(t, 100, 3)
	got := PerformanceFromObservation(st, ValuationObservation{
		PortfolioID: "pf-1", ValueBase: 3.333333333,
		ValuationAsOf: epoch, DataQualityStatus: "complete",
	})
	want := 100 * 3.333333333 / 3
	if got.RankedIndex != want {
		t.Fatalf("ranked index was rounded before snapshot persistence: got %.12f, want %.12f", got.RankedIndex, want)
	}
}

func TestCapitalInjectionPreservesIndex(t *testing.T) {
	st := active(t, 100, 100) // value fell to 50 => index 50
	next, before, after := mutate(st, 50, true, 1000, true)
	assertNeutral(t, "capital injection", before, after)
	if before != 50 {
		t.Fatalf("index before = %v, want 50", before)
	}
	// The classic exploit: 100*1000/(100+950) = 95.24. Must NOT happen.
	if after > 50.0001 {
		t.Fatalf("index after = %v, want 50 (not diluted to ~95.24)", after)
	}
	// Subsequent market gain measures against the NEW segment.
	if got := CalculateCurrentIndex(next, 1100); math.Abs(got-55) > 1e-9 {
		t.Fatalf("post-injection gain = %v, want 55", got)
	}
}

func TestQuantityIncreaseOnWinnerPreservesIndex(t *testing.T) {
	st := active(t, 100, 100)
	_, before, after := mutate(st, 110, true, 110000, true) // x1000 on a winner
	assertNeutral(t, "quantity increase", before, after)
	if before != 110 {
		t.Fatalf("index before = %v, want 110", before)
	}
}

func TestQuantityDecreasePreservesIndex(t *testing.T) {
	st := active(t, 100, 200)
	_, before, after := mutate(st, 200, true, 100, true)
	assertNeutral(t, "quantity decrease", before, after)
}

func TestDeleteWinnerAndLoserPreserveIndex(t *testing.T) {
	winner := active(t, 100, 100)
	_, wb, wa := mutate(winner, 150, true, 90, true) // drop a winner
	assertNeutral(t, "delete winner", wb, wa)
	if wb != 150 {
		t.Fatalf("winner index before = %v, want 150", wb)
	}

	loser := active(t, 100, 100)
	_, lb, la := mutate(loser, 60, true, 40, true) // drop a loser
	assertNeutral(t, "delete loser", lb, la)
	if lb != 60 {
		t.Fatalf("loser index before = %v, want 60", lb)
	}
}

func TestClosePositionPreservesIndex(t *testing.T) {
	// Closing one of several: the realized position leaves the active basket.
	st := active(t, 120, 500)
	_, before, after := mutate(st, 600, true, 400, true)
	assertNeutral(t, "close one of many", before, after)
}

func TestReplacePortfolioPreservesIndex(t *testing.T) {
	st := active(t, 100, 100)
	_, before, after := mutate(st, 90, true, 5000, true) // strategy copy
	assertNeutral(t, "strategy replacement", before, after)
	if before != 90 {
		t.Fatalf("index before = %v, want 90", before)
	}
}

func TestRemovingFinalPositionPausesAndPreserves(t *testing.T) {
	st := active(t, 100, 100)
	next, before, after := mutate(st, 125, true, 0, false)
	assertNeutral(t, "remove final position", before, after)
	if next.Status != StatusPaused {
		t.Fatalf("status = %v, want paused", next.Status)
	}
	if next.SegmentStartValueBase != nil {
		t.Fatalf("paused segment start = %v, want nil", *next.SegmentStartValueBase)
	}
	if next.CheckpointIndex != 125 {
		t.Fatalf("preserved index = %v, want 125", next.CheckpointIndex)
	}
	// Paused reads never divide by zero and never drift.
	if got := CalculateCurrentIndex(next, 0); got != 125 {
		t.Fatalf("paused index = %v, want 125", got)
	}
}

func TestResumeContinuesFromPreservedIndex(t *testing.T) {
	paused := State{
		PortfolioID: "pf-1", UserID: "u-1", CheckpointIndex: 80,
		Status: StatusPaused, TrackingStartedAt: epoch, Version: 4,
	}
	next, before, after := mutate(paused, 0, false, 3000, true)
	assertNeutral(t, "resume", before, after)
	if next.Status != StatusActive {
		t.Fatalf("status = %v, want active", next.Status)
	}
	if next.CheckpointIndex != 80 {
		t.Fatalf("resumed at %v, want 80 (not reset to 100)", next.CheckpointIndex)
	}
}

func TestEpochPreservedAndVersionIncrementsOnce(t *testing.T) {
	st := active(t, 100, 100)
	next, _, _ := mutate(st, 100, true, 200, true)
	if !next.TrackingStartedAt.Equal(epoch) {
		t.Fatalf("epoch changed: %v -> %v", epoch, next.TrackingStartedAt)
	}
	if next.Version != st.Version+1 {
		t.Fatalf("version = %d, want %d", next.Version, st.Version+1)
	}
}

func TestApplyCheckpointDoesNotMutateInput(t *testing.T) {
	st := active(t, 100, 100)
	originalSegment := *st.SegmentStartValueBase
	originalVersion := st.Version
	_, _, _ = mutate(st, 100, true, 999, true)
	if *st.SegmentStartValueBase != originalSegment {
		t.Fatalf("input segment mutated: %v", *st.SegmentStartValueBase)
	}
	if st.Version != originalVersion {
		t.Fatalf("input version mutated: %d", st.Version)
	}
}

// TestNoIntermediateRoundingDrift chain-links many mutations and asserts the
// accumulated index still matches the exact product of segment ratios. Rounding
// stored checkpoints to 2dp would visibly drift here.
func TestNoIntermediateRoundingDrift(t *testing.T) {
	st := ActivateState("pf-1", "u-1", 100, true, epoch)
	value := 100.0
	expected := 100.0
	for i := 0; i < 200; i++ {
		grown := value * 1.001 // +0.1% market move
		expected *= grown / value
		// Then a neutral mutation that resizes the basket to an awkward value.
		in := CheckpointInput{
			PortfolioID: "pf-1", UserID: "u-1",
			ValueBeforeBase: grown, HasActiveBefore: true,
			ValueAfterBase: grown * 3.7, HasActiveAfter: true,
			At: epoch.Add(time.Duration(i) * time.Minute),
		}
		before := CalculateIndexBeforeMutation(st, in)
		st = ApplyCheckpoint(st, in)
		after := CalculateCurrentIndex(st, in.ValueAfterBase)
		assertNeutral(t, "chained mutation", before, after)
		value = in.ValueAfterBase
	}
	got := CalculateCurrentIndex(st, value)
	if math.Abs(got-expected) > 1e-6 {
		t.Fatalf("chain-linked index drifted: got %v, want %v", got, expected)
	}
}

func TestValidateStateRejectsInvalidValues(t *testing.T) {
	neg := active(t, -1, 100)
	if err := ValidateState(neg); err == nil {
		t.Fatal("negative checkpoint index must be rejected")
	}
	nan := active(t, math.NaN(), 100)
	if err := ValidateState(nan); err == nil {
		t.Fatal("NaN checkpoint index must be rejected")
	}
	inf := active(t, math.Inf(1), 100)
	if err := ValidateState(inf); err == nil {
		t.Fatal("infinite checkpoint index must be rejected")
	}
	zeroSeg := active(t, 100, 0)
	if err := ValidateState(zeroSeg); err == nil {
		t.Fatal("active state with zero segment start must be rejected")
	}
	// Paused state must not carry a segment start.
	badPaused := active(t, 100, 50)
	badPaused.Status = StatusPaused
	if err := ValidateState(badPaused); err == nil {
		t.Fatal("paused state with a segment start must be rejected")
	}
	good := active(t, 100, 50)
	if err := ValidateState(good); err != nil {
		t.Fatalf("valid state rejected: %v", err)
	}
}
