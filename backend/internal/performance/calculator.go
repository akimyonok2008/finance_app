package performance

import (
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// The functions in this file are PURE: no database access, deterministic output,
// no mutation of their inputs. All ranked-state transitions go through them, and
// persistence is owned by the portfolio aggregate transaction — so a checkpoint
// can never be committed without the position write it belongs to.

// CalculateCurrentIndex returns the live ranked index for a state given the
// current base-currency portfolio value. A paused state (or one with no valid
// segment) reports its preserved checkpoint index unchanged — an empty portfolio
// never divides by zero and never drifts.
func CalculateCurrentIndex(state State, currentValueBase money.Amount) money.IndexValue {
	if state.Status != StatusActive || state.SegmentStartValueBase == nil || state.SegmentStartValueBase.Sign() <= 0 {
		return state.CheckpointIndex
	}
	ratio, err := currentValueBase.DivExact(*state.SegmentStartValueBase, money.ScaleIndex+money.ScaleValue)
	if err != nil {
		return state.CheckpointIndex
	}
	return state.CheckpointIndex.MulRatio(ratio)
}

// CalculateIndexBeforeMutation computes the ranked index immediately before a
// mutation, from the pre-mutation value. For an active portfolio this is the live
// index; for a paused one it is the preserved checkpoint (an empty portfolio has
// no value to measure).
func CalculateIndexBeforeMutation(state State, in CheckpointInput) money.IndexValue {
	if state.Status == StatusActive && state.SegmentStartValueBase != nil && state.SegmentStartValueBase.Sign() > 0 && in.HasActiveBefore {
		return CalculateCurrentIndex(state, in.ValueBeforeBase)
	}
	return state.CheckpointIndex
}

// ActivateState builds the initial ranked state for a portfolio entering the
// ranked-tracking model for the first time. A non-empty portfolio starts at index
// 100 with its current value as the first segment start; an empty one starts
// paused at index 100. TrackingStartedAt is the ranking epoch.
func ActivateState(portfolioID, userID string, valueBase money.Amount, hasActive bool, at time.Time) State {
	st := State{
		PortfolioID:       portfolioID,
		UserID:            userID,
		CheckpointIndex:   money.MustIndexValue("100"),
		TrackingStartedAt: at,
		UpdatedAt:         at,
		Version:           1,
	}
	if hasActive {
		v := valueBase
		st.Status = StatusActive
		st.SegmentStartValueBase = &v
		st.SegmentStartedAt = &at
	} else {
		st.Status = StatusPaused
		st.SegmentStartValueBase = nil
	}
	return st
}

// ApplyCheckpoint returns the new state produced by a mutation, leaving prev
// untouched. The invariant it enforces: the ranked index is identical
// immediately before and immediately after the mutation (the mutation itself
// generates zero return). Only later market/FX movement, measured against the
// NEW segment start value, can move it.
//
//	index_before      = checkpoint * value_before / segment_start   (active)
//	new checkpoint    = index_before
//	new segment_start = value_after                                 (active after)
//	new status        = active if positions remain, else paused
//
// A paused → active transition resumes from the preserved checkpoint index
// rather than resetting to 100. The epoch (TrackingStartedAt) is preserved, and
// Version is incremented exactly once per committed mutation.
func ApplyCheckpoint(prev State, in CheckpointInput) State {
	index := CalculateIndexBeforeMutation(prev, in)

	next := prev
	next.SegmentStartValueBase = nil // detach from prev's pointer before reassigning
	next.UpdatedAt = in.At
	next.CheckpointIndex = money.QuantizeIndex(index)
	next.Version = prev.Version + 1
	if in.HasActiveAfter {
		v := in.ValueAfterBase
		next.Status = StatusActive
		next.SegmentStartValueBase = &v
		startedAt := in.At
		next.SegmentStartedAt = &startedAt
	} else {
		next.Status = StatusPaused
	}
	return next
}

// ApplyReturnCheckpoint returns the new state produced by a RETURN-BEARING event
// (cash dividend, distribution, interest income, fee, write-off). Unlike
// ApplyCheckpoint it deliberately does NOT re-baseline the segment: the segment
// start value is preserved, so the value change flows straight into the ranked
// index:
//
//	index_after = checkpoint * value_after / segment_start
//
// meaning income raises and fees lower the ranked index in exact proportion to
// the value change, exactly once.
//
// Fallbacks:
//   - No prior active value (paused/empty before): a return ratio is undefined
//     (division by zero), so it activates neutrally at value_after, like a
//     deposit into an empty portfolio.
//   - Drain to zero (no active value after, e.g. a full write-off with no cash
//     left): the loss is captured into the checkpoint index (floored to a tiny
//     positive so ValidateState still holds) and the state pauses.
//
// Version is incremented exactly once, the epoch is preserved.
func ApplyReturnCheckpoint(prev State, in CheckpointInput) State {
	activeBefore := prev.Status == StatusActive && prev.SegmentStartValueBase != nil &&
		prev.SegmentStartValueBase.Sign() > 0 && in.HasActiveBefore
	if !activeBefore {
		// Undefined return ratio → neutral activation.
		return ApplyCheckpoint(prev, in)
	}
	next := CloneState(prev)
	next.Version = prev.Version + 1
	next.UpdatedAt = in.At
	if in.HasActiveAfter && in.ValueAfterBase.Sign() > 0 {
		// Keep the segment baseline untouched: the index moves with value.
		next.Status = StatusActive
		return next
	}
	// Everything drained to (near) zero. Record the realized loss in the
	// checkpoint index, floored to a tiny positive, then pause.
	idxAfter := CalculateCurrentIndex(prev, in.ValueAfterBase)
	floor := money.MustIndexValue("0.000000001")
	if idxAfter.Cmp(floor) < 0 {
		idxAfter = floor
	}
	next.CheckpointIndex = money.QuantizeIndex(idxAfter)
	next.Status = StatusPaused
	next.SegmentStartValueBase = nil
	next.SegmentStartedAt = nil
	return next
}

// ValidateState enforces the ranked-state invariants that the database also
// constrains: a finite positive checkpoint index, a positive segment start when
// active, and no segment start when paused.
func ValidateState(state State) error {
	if state.PortfolioID == "" || state.UserID == "" {
		return ErrInvalidState
	}
	if state.CheckpointIndex.Sign() <= 0 {
		return ErrInvalidState
	}
	if state.Version <= 0 {
		return ErrInvalidState
	}
	switch state.Status {
	case StatusActive:
		if state.SegmentStartValueBase == nil || state.SegmentStartValueBase.Sign() <= 0 {
			return ErrInvalidState
		}
	case StatusPaused:
		if state.SegmentStartValueBase != nil {
			return ErrInvalidState
		}
	default:
		return ErrInvalidState
	}
	return nil
}

// CloneState returns a deep copy so callers can never alias another holder's
// pointer fields.
func CloneState(state State) State {
	copied := state
	if state.SegmentStartValueBase != nil {
		v := *state.SegmentStartValueBase
		copied.SegmentStartValueBase = &v
	}
	if state.SegmentStartedAt != nil {
		t := *state.SegmentStartedAt
		copied.SegmentStartedAt = &t
	}
	return copied
}

// Compatibility aliases retained while callers migrate to the exported pure
// transition API.
func CurrentIndex(state State, currentValueBase money.Amount) money.IndexValue {
	return CalculateCurrentIndex(state, currentValueBase)
}

func activate(portfolioID, userID string, valueBase money.Amount, hasActive bool, at time.Time) State {
	return ActivateState(portfolioID, userID, valueBase, hasActive, at)
}
