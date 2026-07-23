package portfolio

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// RankedCheckpointer is the ranked-performance side of a portfolio mutation.
// The portfolio service calls it after applying a position change so the
// persistent ranked index is checkpointed atomically-in-intent with the
// mutation. Implemented by *performance.Service. Optional: when nil, mutations
// still succeed (used by isolated portfolio unit tests) and ranked tracking is
// simply not updated.
type RankedCheckpointer interface {
	Checkpoint(ctx context.Context, in performance.CheckpointInput) error
}

// SetCheckpointer attaches the ranked-performance checkpoint engine. Wired in
// main so every position mutation updates ranked state.
func (s *Service) SetCheckpointer(c RankedCheckpointer) { s.checkpointer = c }

// PortfolioValueBase implements performance.Valuator: it returns the user's
// default portfolio id, the current base-currency market value of the ACTIVE
// positions, and whether any active positions exist. An error means the
// portfolio cannot be valued consistently (missing prices/FX) — the ranked
// engine then refuses to read or checkpoint rather than using zeros.
func (s *Service) PortfolioValueBase(ctx context.Context, userID string) (string, float64, bool, error) {
	pf, err := s.GetOrCreateDefaultPortfolio(userID)
	if err != nil {
		return "", 0, false, err
	}
	positions, err := s.repo.ListPositionsByUser(userID)
	if err != nil {
		return "", 0, false, err
	}
	value, hasActive, err := s.valueOpenSet(ctx, positions, newQuoteCache())
	if err != nil {
		return "", 0, false, err
	}
	return pf.ID, value, hasActive, nil
}

// quoteCache memoizes one quote per symbol for the duration of a single mutation
// so the same symbol is valued identically before and after the change (the
// consistent valuation snapshot required for a fair checkpoint) and the same
// quote is never fetched twice.
type quoteCache struct {
	prices map[string]*prices.Price
	rates  map[string]float64
}

func newQuoteCache() *quoteCache {
	return &quoteCache{prices: map[string]*prices.Price{}, rates: map[string]float64{}}
}

func (c *quoteCache) seedPrice(symbol string, p *prices.Price) {
	if p != nil {
		c.prices[symbol] = p
	}
}

func (s *Service) quoteFor(ctx context.Context, cache *quoteCache, symbol string) (*prices.Price, error) {
	if p, ok := cache.prices[symbol]; ok {
		return p, nil
	}
	p, err := s.provider.GetLatestPrice(ctx, symbol)
	if err != nil || p == nil {
		return nil, ErrPriceProvider
	}
	cache.prices[symbol] = p
	return p, nil
}

func (s *Service) rateFor(ctx context.Context, cache *quoteCache, currency string) (float64, error) {
	if r, ok := cache.rates[currency]; ok {
		return r, nil
	}
	r, err := s.fx.GetRate(ctx, currency, fx.BaseCurrency)
	if err != nil {
		return 0, ErrPriceProvider
	}
	cache.rates[currency] = r
	return r, nil
}

// valueOpenSet returns the base-currency market value of the OPEN positions in
// the set and whether any exist, pricing each symbol once via the cache. Any
// pricing/FX failure aborts with ErrPriceProvider so no mutation proceeds on an
// unvaluable portfolio.
func (s *Service) valueOpenSet(ctx context.Context, positions []*Position, cache *quoteCache) (float64, bool, error) {
	var total float64
	hasActive := false
	for _, pos := range positions {
		if positionStatus(pos) != PositionStatusOpen {
			continue
		}
		hasActive = true
		price, err := s.quoteFor(ctx, cache, pos.Symbol)
		if err != nil {
			return 0, false, err
		}
		rate, err := s.rateFor(ctx, cache, price.Currency)
		if err != nil {
			return 0, false, err
		}
		total += pos.Quantity * price.Price * rate
	}
	return total, hasActive, nil
}

// rankedInput values the pre-mutation open set (oldOpen) and the post-mutation
// open set (newOpen) using ONE shared quote snapshot and returns the checkpoint
// to record. It is called BEFORE the position write is persisted, so a
// price/FX failure aborts the whole mutation with nothing changed (value-first
// ordering). Returns (nil, nil) when no checkpointer is attached.
func (s *Service) rankedInput(ctx context.Context, portfolioID, userID string, oldOpen, newOpen []*Position, cache *quoteCache) (*performance.CheckpointInput, error) {
	if s.checkpointer == nil {
		return nil, nil
	}
	valueBefore, hadBefore, err := s.valueOpenSet(ctx, oldOpen, cache)
	if err != nil {
		return nil, err
	}
	valueAfter, hasAfter, err := s.valueOpenSet(ctx, newOpen, cache)
	if err != nil {
		return nil, err
	}
	return &performance.CheckpointInput{
		PortfolioID:     portfolioID,
		UserID:          userID,
		ValueBeforeBase: valueBefore,
		HasActiveBefore: hadBefore,
		ValueAfterBase:  valueAfter,
		HasActiveAfter:  hasAfter,
		At:              time.Now().UTC(),
	}, nil
}

// applyCheckpoint records a prepared checkpoint after the position write. A nil
// input (no checkpointer) is a no-op.
func (s *Service) applyRankedCheckpoint(ctx context.Context, input *performance.CheckpointInput) error {
	if input == nil {
		return nil
	}
	return s.checkpointer.Checkpoint(ctx, *input)
}

// openPositions returns only the open positions from a slice (copies of the
// pointers, not deep copies).
func openPositions(positions []*Position) []*Position {
	out := make([]*Position, 0, len(positions))
	for _, p := range positions {
		if positionStatus(p) == PositionStatusOpen {
			out = append(out, p)
		}
	}
	return out
}

// replaceInSet returns a new slice with the position matching id replaced by
// updated. Positions that do not match are carried over unchanged.
func replaceInSet(positions []*Position, id string, updated *Position) []*Position {
	out := make([]*Position, 0, len(positions))
	for _, p := range positions {
		if p.ID == id {
			out = append(out, updated)
			continue
		}
		out = append(out, p)
	}
	return out
}

// removeFromSet returns a new slice without the position matching id.
func removeFromSet(positions []*Position, id string) []*Position {
	out := make([]*Position, 0, len(positions))
	for _, p := range positions {
		if p.ID == id {
			continue
		}
		out = append(out, p)
	}
	return out
}
