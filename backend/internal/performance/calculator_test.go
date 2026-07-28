package performance

import (
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// These tests cover the PURE ranked-performance transitions. They touch no
// database: every scenario is expressed as state + checkpoint input, which is
// exactly the contract the portfolio aggregate transaction depends on.

var epoch = time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)

func active(t *testing.T, checkpoint, segmentStart string) State {
	t.Helper()
	return State{
		PortfolioID: "pf-1", UserID: "u-1",
		CheckpointIndex: testIndex(checkpoint), SegmentStartValueBase: testAmountPtr(segmentStart),
		Status: StatusActive, TrackingStartedAt: epoch, Version: 1,
	}
}

// mutate applies a checkpoint and returns (index before, index after).
func mutate(prev State, valueBefore string, hadActive bool, valueAfter string, hasActive bool) (State, money.IndexValue, money.IndexValue) {
	in := CheckpointInput{
		PortfolioID: "pf-1", UserID: "u-1",
		ValueBeforeBase: testAmount(valueBefore), HasActiveBefore: hadActive,
		ValueAfterBase: testAmount(valueAfter), HasActiveAfter: hasActive,
		At: epoch.Add(time.Hour),
	}
	before := CalculateIndexBeforeMutation(prev, in)
	next := ApplyCheckpoint(prev, in)
	after := CalculateCurrentIndex(next, testAmount(valueAfter))
	return next, before, after
}

func assertNeutral(t *testing.T, label string, before, after money.IndexValue) {
	t.Helper()
	if !money.QuantizeIndex(before).EqualIndex(money.QuantizeIndex(after)) {
		t.Fatalf("%s: mutation generated ranked return: before=%v after=%v", label, before, after)
	}
}

func TestActivate_FirstNonEmptyPortfolioStartsAt100(t *testing.T) {
	st := ActivateState("pf-1", "u-1", testAmount("250"), true, epoch)
	if st.Status != StatusActive || st.CheckpointIndex.Cmp(testIndex("100")) != 0 {
		t.Fatalf("state = %+v, want active @100", st)
	}
	if got := CalculateCurrentIndex(st, testAmount("250")); got.Cmp(testIndex("100")) != 0 {
		t.Fatalf("index = %v, want 100", got)
	}
}

func TestActivate_EmptyPortfolioStartsPausedAt100(t *testing.T) {
	st := ActivateState("pf-1", "u-1", testAmount("0"), false, epoch)
	if st.Status != StatusPaused || st.SegmentStartValueBase != nil {
		t.Fatalf("state = %+v, want paused with nil segment", st)
	}
	// No division by zero, whatever value is supplied.
	if got := CalculateCurrentIndex(st, testAmount("0")); got.Cmp(testIndex("100")) != 0 {
		t.Fatalf("index = %v, want 100", got)
	}
}

func TestMarketMovesChangeIndex(t *testing.T) {
	st := active(t, "100", "100")
	if got := CalculateCurrentIndex(st, testAmount("110")); got.Cmp(testIndex("110")) != 0 {
		t.Fatalf("+10%% -> %v, want 110", got)
	}
	if got := CalculateCurrentIndex(st, testAmount("80")); got.Cmp(testIndex("80")) != 0 {
		t.Fatalf("-20%% -> %v, want 80", got)
	}
}

func TestPerformanceReadModelPreservesSnapshotPrecision(t *testing.T) {
	st := active(t, "100", "3")
	got := PerformanceFromObservation(st, ValuationObservation{
		PortfolioID: "pf-1", ValueBase: testAmount("3.333333333"),
		ValuationAsOf: epoch, DataQualityStatus: "complete",
	})
	assertIndexEqual(t, "111.1111111", got.RankedIndex)
}

func TestCapitalInjectionPreservesIndex(t *testing.T) {
	st := active(t, "100", "100") // value fell to 50 => index 50
	next, before, after := mutate(st, "50", true, "1000", true)
	assertNeutral(t, "capital injection", before, after)
	if before.Cmp(testIndex("50")) != 0 {
		t.Fatalf("index before = %v, want 50", before)
	}
	// The classic exploit: 100*1000/(100+950) = 95.24. Must NOT happen.
	if after.Cmp(testIndex("50")) != 0 {
		t.Fatalf("index after = %v, want 50 (not diluted to ~95.24)", after)
	}
	// Subsequent market gain measures against the NEW segment.
	if got := CalculateCurrentIndex(next, testAmount("1100")); got.Cmp(testIndex("55")) != 0 {
		t.Fatalf("post-injection gain = %v, want 55", got)
	}
}

func TestQuantityIncreaseOnWinnerPreservesIndex(t *testing.T) {
	st := active(t, "100", "100")
	_, before, after := mutate(st, "110", true, "110000", true) // x1000 on a winner
	assertNeutral(t, "quantity increase", before, after)
	if before.Cmp(testIndex("110")) != 0 {
		t.Fatalf("index before = %v, want 110", before)
	}
}

func TestQuantityDecreasePreservesIndex(t *testing.T) {
	st := active(t, "100", "200")
	_, before, after := mutate(st, "200", true, "100", true)
	assertNeutral(t, "quantity decrease", before, after)
}

func TestDeleteWinnerAndLoserPreserveIndex(t *testing.T) {
	winner := active(t, "100", "100")
	_, wb, wa := mutate(winner, "150", true, "90", true) // drop a winner
	assertNeutral(t, "delete winner", wb, wa)
	if wb.Cmp(testIndex("150")) != 0 {
		t.Fatalf("winner index before = %v, want 150", wb)
	}

	loser := active(t, "100", "100")
	_, lb, la := mutate(loser, "60", true, "40", true) // drop a loser
	assertNeutral(t, "delete loser", lb, la)
	if lb.Cmp(testIndex("60")) != 0 {
		t.Fatalf("loser index before = %v, want 60", lb)
	}
}

func TestClosePositionPreservesIndex(t *testing.T) {
	// Closing one of several: the realized position leaves the active basket.
	st := active(t, "120", "500")
	_, before, after := mutate(st, "600", true, "400", true)
	assertNeutral(t, "close one of many", before, after)
}

func TestReplacePortfolioPreservesIndex(t *testing.T) {
	st := active(t, "100", "100")
	_, before, after := mutate(st, "90", true, "5000", true) // strategy copy
	assertNeutral(t, "strategy replacement", before, after)
	if before.Cmp(testIndex("90")) != 0 {
		t.Fatalf("index before = %v, want 90", before)
	}
}

func TestRemovingFinalPositionPausesAndPreserves(t *testing.T) {
	st := active(t, "100", "100")
	next, before, after := mutate(st, "125", true, "0", false)
	assertNeutral(t, "remove final position", before, after)
	if next.Status != StatusPaused {
		t.Fatalf("status = %v, want paused", next.Status)
	}
	if next.SegmentStartValueBase != nil {
		t.Fatalf("paused segment start = %v, want nil", *next.SegmentStartValueBase)
	}
	if next.CheckpointIndex.Cmp(testIndex("125")) != 0 {
		t.Fatalf("preserved index = %v, want 125", next.CheckpointIndex)
	}
	// Paused reads never divide by zero and never drift.
	if got := CalculateCurrentIndex(next, testAmount("0")); got.Cmp(testIndex("125")) != 0 {
		t.Fatalf("paused index = %v, want 125", got)
	}
}

func TestResumeContinuesFromPreservedIndex(t *testing.T) {
	paused := State{
		PortfolioID: "pf-1", UserID: "u-1", CheckpointIndex: testIndex("80"),
		Status: StatusPaused, TrackingStartedAt: epoch, Version: 4,
	}
	next, before, after := mutate(paused, "0", false, "3000", true)
	assertNeutral(t, "resume", before, after)
	if next.Status != StatusActive {
		t.Fatalf("status = %v, want active", next.Status)
	}
	if next.CheckpointIndex.Cmp(testIndex("80")) != 0 {
		t.Fatalf("resumed at %v, want 80 (not reset to 100)", next.CheckpointIndex)
	}
}

func TestEpochPreservedAndVersionIncrementsOnce(t *testing.T) {
	st := active(t, "100", "100")
	next, _, _ := mutate(st, "100", true, "200", true)
	if !next.TrackingStartedAt.Equal(epoch) {
		t.Fatalf("epoch changed: %v -> %v", epoch, next.TrackingStartedAt)
	}
	if next.Version != st.Version+1 {
		t.Fatalf("version = %d, want %d", next.Version, st.Version+1)
	}
}

func TestApplyCheckpointDoesNotMutateInput(t *testing.T) {
	st := active(t, "100", "100")
	originalSegment := *st.SegmentStartValueBase
	originalVersion := st.Version
	_, _, _ = mutate(st, "100", true, "999", true)
	if !st.SegmentStartValueBase.EqualAmount(originalSegment) {
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
	st := ActivateState("pf-1", "u-1", testAmount("100"), true, epoch)
	value := testAmount("100")
	expected := testIndex("100")
	growth := money.MustRatio("1.001")
	resize := money.MustRatio("3.7")
	for i := 0; i < 200; i++ {
		grown := value.MulRatio(growth) // +0.1% market move
		expected = money.QuantizeIndex(expected.MulRatio(growth))
		// Then a neutral mutation that resizes the basket to an awkward value.
		in := CheckpointInput{
			PortfolioID: "pf-1", UserID: "u-1",
			ValueBeforeBase: grown, HasActiveBefore: true,
			ValueAfterBase: grown.MulRatio(resize), HasActiveAfter: true,
			At: epoch.Add(time.Duration(i) * time.Minute),
		}
		before := CalculateIndexBeforeMutation(st, in)
		st = ApplyCheckpoint(st, in)
		after := CalculateCurrentIndex(st, in.ValueAfterBase)
		assertNeutral(t, "chained mutation", before, after)
		value = in.ValueAfterBase
	}
	got := CalculateCurrentIndex(st, value)
	if got.Cmp(expected) != 0 {
		t.Fatalf("chain-linked index drifted: got %v, want %v", got, expected)
	}
}

func TestValidateStateRejectsInvalidValues(t *testing.T) {
	neg := active(t, "-1", "100")
	if err := ValidateState(neg); err == nil {
		t.Fatal("negative checkpoint index must be rejected")
	}
	for _, invalid := range []string{"NaN", "Inf", "-Inf"} {
		if _, err := money.ParseIndexValue(invalid); err == nil {
			t.Fatalf("%s index must be rejected while parsing", invalid)
		}
	}
	zeroSeg := active(t, "100", "0")
	if err := ValidateState(zeroSeg); err == nil {
		t.Fatal("active state with zero segment start must be rejected")
	}
	// Paused state must not carry a segment start.
	badPaused := active(t, "100", "50")
	badPaused.Status = StatusPaused
	if err := ValidateState(badPaused); err == nil {
		t.Fatal("paused state with a segment start must be rejected")
	}
	good := active(t, "100", "50")
	if err := ValidateState(good); err != nil {
		t.Fatalf("valid state rejected: %v", err)
	}
}
