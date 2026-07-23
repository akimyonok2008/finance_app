package performance

import (
	"context"
	"errors"
	"time"
)

// Valuator supplies the current base-currency market value of a user's ACTIVE
// positions. It is implemented by the portfolio service. Returning an error
// means the portfolio cannot be valued consistently (missing prices/FX); the
// ranked engine then refuses to read or checkpoint rather than inventing zeros.
type Valuator interface {
	// PortfolioValueBase returns the portfolio id, the current base-currency
	// value of active positions, and whether any active positions exist.
	PortfolioValueBase(ctx context.Context, userID string) (portfolioID string, valueBase float64, hasActive bool, err error)
}

// Service is the ranked-performance domain service: the single trusted source of
// global ranked performance. It owns persistent ranked state and the checkpoint
// engine; it never stores or exposes absolute monetary values publicly.
type Service struct {
	repo     Repository
	valuator Valuator
	now      func() time.Time
}

// NewService wires a ranked-performance service. The valuator may be attached
// later via SetValuator (the portfolio service and this service are mutually
// dependent at construction time).
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

// SetValuator attaches the portfolio valuator used by the read path.
func (s *Service) SetValuator(v Valuator) { s.valuator = v }

// SetClock overrides the time source (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Checkpoint atomically advances the ranked state for a single portfolio
// mutation. It is idempotent w.r.t. the ranked invariant: the ranked index is
// identical immediately before and after, so no mutation (add capital, resize,
// delete, rebalance, strategy copy, empty, resume) generates ranked return.
//
// The caller must have already valued the portfolio before and after the
// mutation using ONE consistent price/FX snapshot. On a version conflict the
// method re-reads and retries a bounded number of times so concurrent mutations
// serialize rather than clobbering each other.
func (s *Service) Checkpoint(ctx context.Context, in CheckpointInput) error {
	if in.At.IsZero() {
		in.At = s.now()
	}
	const maxAttempts = 5
	for attempt := 0; attempt < maxAttempts; attempt++ {
		prev, err := s.repo.GetByPortfolio(ctx, in.PortfolioID)
		if errors.Is(err, ErrStateNotFound) {
			// First time this portfolio enters ranked tracking: activate at 100.
			st := activate(in.PortfolioID, in.UserID, in.ValueAfterBase, in.HasActiveAfter, in.At)
			if err := s.repo.Create(ctx, st); err != nil {
				if errors.Is(err, ErrVersionConflict) {
					continue // lost the create race; re-read and update instead
				}
				return err
			}
			return nil
		}
		if err != nil {
			return err
		}
		next := applyCheckpoint(*prev, in)
		if err := s.repo.Update(ctx, next, prev.Version); err != nil {
			if errors.Is(err, ErrVersionConflict) {
				continue
			}
			return err
		}
		return nil
	}
	return ErrVersionConflict
}

// EnsureEpoch initializes ranked tracking for an existing portfolio if it has no
// state yet, WITHOUT rewriting any existing state. This implements the migration
// policy: a non-empty portfolio starts a new epoch at index 100 with its current
// value as the first segment; an empty portfolio starts paused at index 100.
// Idempotent: a portfolio that already has state is left untouched.
func (s *Service) EnsureEpoch(ctx context.Context, userID string) error {
	portfolioID, valueBase, hasActive, err := s.valuator.PortfolioValueBase(ctx, userID)
	if err != nil {
		return err
	}
	if _, err := s.repo.GetByPortfolio(ctx, portfolioID); err == nil {
		return nil // already tracked
	} else if !errors.Is(err, ErrStateNotFound) {
		return err
	}
	st := activate(portfolioID, userID, valueBase, hasActive, s.now())
	if err := s.repo.Create(ctx, st); err != nil && !errors.Is(err, ErrVersionConflict) {
		return err
	}
	return nil
}

// CurrentRankedPerformance is the read path consumed by the leaderboard,
// profiles, and achievements. It values the user's active portfolio and applies
// the persistent ranked state. A portfolio that has never been checkpointed is
// treated as a fresh epoch (active at 100 if it holds positions, else paused at
// 100) so a first read is always well-defined without a write.
func (s *Service) CurrentRankedPerformance(ctx context.Context, userID string) (*RankedPerformance, error) {
	portfolioID, valueBase, hasActive, err := s.valuator.PortfolioValueBase(ctx, userID)
	if err != nil {
		return nil, err
	}
	state, err := s.repo.GetByPortfolio(ctx, portfolioID)
	if errors.Is(err, ErrStateNotFound) {
		synthetic := activate(portfolioID, userID, valueBase, hasActive, s.now())
		return performanceFrom(synthetic, valueBase), nil
	}
	if err != nil {
		return nil, err
	}
	return performanceFrom(*state, valueBase), nil
}

func performanceFrom(state State, valueBase float64) *RankedPerformance {
	idx := round2(CurrentIndex(state, valueBase))
	return &RankedPerformance{
		RankedIndex:            idx,
		RankedReturnPercentage: round2(idx - 100),
		Status:                 state.Status,
		TrackingStartedAt:      state.TrackingStartedAt,
	}
}
