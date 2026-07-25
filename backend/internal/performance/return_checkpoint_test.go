package performance

import (
	"math"
	"testing"
	"time"
)

// returnMutate applies a return-bearing checkpoint and returns (before, after).
func returnMutate(prev State, valueBefore float64, hadActive bool, valueAfter float64, hasActive bool) (State, float64, float64) {
	in := CheckpointInput{
		PortfolioID: "pf-1", UserID: "u-1",
		ValueBeforeBase: valueBefore, HasActiveBefore: hadActive,
		ValueAfterBase: valueAfter, HasActiveAfter: hasActive,
		At: epoch.Add(time.Hour),
	}
	before := CalculateIndexBeforeMutation(prev, in)
	next := ApplyReturnCheckpoint(prev, in)
	after := CalculateCurrentIndex(next, valueAfter)
	return next, before, after
}

func TestReturnCheckpoint_DividendRaisesIndex(t *testing.T) {
	// Segment started at 10,000, index 100. Value now 10,000; a 100 dividend
	// makes it 10,100 → index should rise ~1%.
	st := active(t, 100, 10000)
	next, before, after := returnMutate(st, 10000, true, 10100, true)
	if math.Abs(before-100) > tol {
		t.Fatalf("before should be 100, got %v", before)
	}
	if math.Abs(after-101) > 1e-6 {
		t.Fatalf("dividend should raise index to ~101, got %v", after)
	}
	if next.Version != 2 {
		t.Fatalf("version must advance once, got %d", next.Version)
	}
	// Segment baseline must be UNCHANGED (that is what lets the value change show).
	if next.SegmentStartValueBase == nil || *next.SegmentStartValueBase != 10000 {
		t.Fatalf("segment baseline must be preserved for a return event")
	}
}

func TestReturnCheckpoint_FeeLowersIndex(t *testing.T) {
	st := active(t, 120, 10000)                                   // already up 20% from market
	_, before, after := returnMutate(st, 10000, true, 9975, true) // 25 fee
	if after >= before {
		t.Fatalf("a fee must lower the index: before=%v after=%v", before, after)
	}
	want := 120.0 * 9975 / 10000
	if math.Abs(after-want) > 1e-6 {
		t.Fatalf("fee index want %v, got %v", want, after)
	}
}

func TestReturnCheckpoint_NoPriorActiveValueActivatesNeutrally(t *testing.T) {
	// Paused/empty before: a dividend into an empty portfolio cannot express a
	// return ratio, so it activates neutrally (index preserved).
	paused := State{
		PortfolioID: "pf-1", UserID: "u-1", CheckpointIndex: 100,
		Status: StatusPaused, TrackingStartedAt: epoch, Version: 1,
	}
	_, before, after := returnMutate(paused, 0, false, 50, true)
	assertNeutral(t, "dividend into empty portfolio", before, after)
}

func TestReturnCheckpoint_DrainToZeroCapturesLossAndPauses(t *testing.T) {
	st := active(t, 100, 10000)
	next, _, after := returnMutate(st, 10000, true, 0, false)
	if next.Status != StatusPaused {
		t.Fatalf("draining to zero should pause, got %s", next.Status)
	}
	if next.SegmentStartValueBase != nil {
		t.Fatalf("paused state must not carry a segment start")
	}
	if err := ValidateState(next); err != nil {
		t.Fatalf("drained state must remain valid: %v", err)
	}
	if after >= 100 {
		t.Fatalf("a total write-off must record a loss, got index %v", after)
	}
}
