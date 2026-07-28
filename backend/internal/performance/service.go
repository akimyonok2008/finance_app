package performance

import (
	"context"
	"errors"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// Valuator supplies the current base-currency market value of a user's ACTIVE
// positions. Implemented by the portfolio service. An error means the portfolio
// cannot be valued consistently (missing prices/FX); the ranked engine then
// refuses to report rather than inventing zeros.
type Valuator interface {
	PortfolioValueBase(ctx context.Context, userID string) (portfolioID string, valueBase money.Amount, hasActive bool, err error)
}

type ObservedValuator interface {
	PortfolioValueObservation(ctx context.Context, userID string) (ValuationObservation, error)
}

type TransitionRecorder interface {
	RecordTransition(ctx context.Context, snapshot TransitionSnapshot) error
}

// Service is the READ side of ranked performance — the single trusted source of
// global ranked standing for the leaderboard, profiles and explore.
//
// It has no write API. Ranked state changes only inside the portfolio aggregate
// transaction (see portfolio.MutationCoordinator), which guarantees a checkpoint
// can never be committed apart from the position write that caused it.
type Service struct {
	states   StateReader
	mutable  MutableRepository
	valuator Valuator
	recorder TransitionRecorder
	now      func() time.Time
}

// NewService wires the ranked-performance read service.
func NewService(states StateReader) *Service {
	s := &Service{states: states, now: func() time.Time { return time.Now().UTC() }}
	if mutable, ok := states.(MutableRepository); ok {
		s.mutable = mutable
	}
	return s
}

// SetValuator attaches the portfolio valuator (the portfolio and performance
// services are mutually dependent at construction time).
func (s *Service) SetValuator(v Valuator) { s.valuator = v }

// SetClock overrides the time source (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

func (s *Service) SetTransitionRecorder(recorder TransitionRecorder) {
	s.recorder = recorder
}

// CurrentRankedPerformance values the user's active portfolio and applies the
// persistent ranked state. A portfolio that has never been checkpointed reads as
// a fresh epoch (active at 100 when it holds positions, else paused at 100) so a
// first read is well-defined without performing a write.
func (s *Service) CurrentRankedPerformance(ctx context.Context, userID string) (*RankedPerformance, error) {
	observation := ValuationObservation{
		ValuationAsOf:     s.now(),
		DataQualityStatus: "complete",
	}
	if observed, ok := s.valuator.(ObservedValuator); ok {
		var err error
		observation, err = observed.PortfolioValueObservation(ctx, userID)
		if err != nil {
			return nil, err
		}
	} else {
		portfolioID, valueBase, hasActive, err := s.valuator.PortfolioValueBase(ctx, userID)
		if err != nil {
			return nil, err
		}
		observation.PortfolioID = portfolioID
		observation.ValueBase = valueBase
		observation.HasActive = hasActive
	}
	state, err := s.states.GetByPortfolio(ctx, observation.PortfolioID)
	if errors.Is(err, ErrStateNotFound) {
		synthetic := ActivateState(observation.PortfolioID, userID, observation.ValueBase, observation.HasActive, s.now())
		return PerformanceFromObservation(synthetic, observation), nil
	}
	if err != nil {
		return nil, err
	}
	return PerformanceFromObservation(*state, observation), nil
}

// PerformanceFrom projects a ranked state plus a current valuation into the
// privacy-safe read model. It preserves full precision so canonical snapshot
// persistence never receives presentation-rounded data.
func PerformanceFrom(state State, valueBase money.Amount) *RankedPerformance {
	return PerformanceFromObservation(state, ValuationObservation{
		PortfolioID: state.PortfolioID, ValueBase: valueBase,
		ValuationAsOf: time.Now().UTC(), DataQualityStatus: "complete",
	})
}

func PerformanceFromObservation(state State, observation ValuationObservation) *RankedPerformance {
	idx := CalculateCurrentIndex(state, observation.ValueBase)
	return &RankedPerformance{
		PortfolioID:            state.PortfolioID,
		RankedIndex:            idx,
		RankedReturnPercentage: idx.Sub(money.MustIndexValue("100")),
		Status:                 state.Status,
		TrackingStartedAt:      state.TrackingStartedAt,
		ValuationAsOf:          observation.ValuationAsOf,
		DataQualityStatus:      observation.DataQualityStatus,
	}
}

// Checkpoint advances ranked state after a portfolio mutation. The in-progress
// aggregate transaction refactor will move this write behind AggregateStore;
// this compatibility path preserves existing behavior meanwhile.
func (s *Service) Checkpoint(ctx context.Context, input CheckpointInput) error {
	if s.mutable == nil {
		return ErrInvalidState
	}
	at := input.At.UTC()
	if at.IsZero() {
		at = s.now()
	}
	current, err := s.mutable.GetByPortfolio(ctx, input.PortfolioID)
	isNew := errors.Is(err, ErrStateNotFound)
	if err != nil && !isNew {
		return err
	}
	var next State
	var previousStatus Status
	if isNew {
		next = ActivateState(input.PortfolioID, input.UserID, input.ValueAfterBase, input.HasActiveAfter, at)
		if err := s.mutable.Create(ctx, next); err != nil {
			return err
		}
	} else {
		previousStatus = current.Status
		next = ApplyCheckpoint(*current, input)
		if err := s.mutable.Update(ctx, next, current.Version); err != nil {
			return err
		}
	}
	if s.recorder != nil && (isNew || previousStatus != next.Status) {
		_ = s.recorder.RecordTransition(ctx, TransitionSnapshot{
			PortfolioID: next.PortfolioID, UserID: next.UserID,
			TrackingStartedAt: next.TrackingStartedAt,
			RankedIndex:       next.CheckpointIndex, Status: next.Status,
			CapturedAt: at, ValuationAsOf: at, DataQualityStatus: "complete",
		})
	}
	return nil
}

// EnsureEpoch initializes a persisted epoch without importing legacy history.
func (s *Service) EnsureEpoch(ctx context.Context, userID string) error {
	if s.mutable == nil {
		return ErrInvalidState
	}
	observation, err := s.currentObservation(ctx, userID)
	if err != nil {
		return err
	}
	if _, err := s.mutable.GetByPortfolio(ctx, observation.PortfolioID); err == nil {
		return nil
	} else if !errors.Is(err, ErrStateNotFound) {
		return err
	}
	at := s.now()
	state := ActivateState(observation.PortfolioID, userID, observation.ValueBase, observation.HasActive, at)
	if err := s.mutable.Create(ctx, state); err != nil && !errors.Is(err, ErrVersionConflict) {
		return err
	}
	if s.recorder != nil {
		_ = s.recorder.RecordTransition(ctx, TransitionSnapshot{
			PortfolioID: state.PortfolioID, UserID: state.UserID,
			TrackingStartedAt: state.TrackingStartedAt,
			RankedIndex:       state.CheckpointIndex, Status: state.Status,
			CapturedAt: at, ValuationAsOf: observation.ValuationAsOf,
			DataQualityStatus: observation.DataQualityStatus,
		})
	}
	return nil
}

func (s *Service) currentObservation(ctx context.Context, userID string) (ValuationObservation, error) {
	if observed, ok := s.valuator.(ObservedValuator); ok {
		return observed.PortfolioValueObservation(ctx, userID)
	}
	portfolioID, value, active, err := s.valuator.PortfolioValueBase(ctx, userID)
	return ValuationObservation{
		PortfolioID: portfolioID, ValueBase: value, HasActive: active,
		ValuationAsOf: s.now(), DataQualityStatus: "complete",
	}, err
}
