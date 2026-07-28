package portfolio

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/performance"
)

// This file holds the READ-side portfolio valuation consumed by the ranked
// engine, plus small pure set helpers used by the mutation coordinator.
//
// It deliberately contains no checkpoint orchestration. Ranked state is written
// only inside the aggregate transaction (see coordinator.go), so no API here can
// reintroduce a non-atomic mutation.

// PortfolioValueBase implements performance.Valuator: the user's default
// portfolio id, the current base-currency market value of the ACTIVE positions,
// and whether any active positions exist. An error means the portfolio cannot be
// valued consistently (missing prices/FX); the ranked engine then refuses to
// report rather than using zeros.
func (s *Service) PortfolioValueBase(ctx context.Context, userID string) (string, float64, bool, error) {
	observation, err := s.PortfolioValueObservation(ctx, userID)
	if err != nil {
		return "", 0, false, err
	}
	return observation.PortfolioID, observation.ValueBase, observation.HasActive, nil
}

// PortfolioValueObservation adds the valuation timestamp and quote-freshness
// quality to the private ranked valuation, so readers can distinguish a value
// derived from fresh quotes from one derived from allowed stale quotes.
func (s *Service) PortfolioValueObservation(ctx context.Context, userID string) (performance.ValuationObservation, error) {
	pf, err := s.GetOrCreateDefaultPortfolio(ctx, userID)
	if err != nil {
		return performance.ValuationObservation{}, err
	}
	positions, err := s.repo.ListPositionsByUser(ctx, userID)
	if err != nil {
		return performance.ValuationObservation{}, err
	}
	cash, err := s.repo.ListCashBalances(ctx, userID)
	if err != nil {
		return performance.ValuationObservation{}, err
	}

	value, hasActive, quality, asOf, err := s.valueOpenObserved(ctx, positions)
	if err != nil {
		return performance.ValuationObservation{}, err
	}
	for _, balance := range cash {
		if balance.Amount.Sign() <= 0 {
			continue
		}
		rate, err := s.fx.GetRate(ctx, balance.Currency, fx.BaseCurrency)
		if err != nil || !finitePositive(rate) {
			return performance.ValuationObservation{}, ErrUnsupportedCurrency
		}
		// balance.Amount.Float64() is a documented boundary conversion: value
		// and rate stay float64 because they feed internal/performance and
		// internal/fx, out of scope for this section.
		value += balance.Amount.Float64() * rate
		hasActive = true
	}
	return performance.ValuationObservation{
		PortfolioID: pf.ID, ValueBase: value, HasActive: hasActive,
		ValuationAsOf: asOf, DataQualityStatus: quality,
	}, nil
}

type observedQuote struct {
	price    float64
	currency string
}

// valueOpenObserved prices each distinct symbol once and reports the oldest
// observation time plus whether any quote was stale.
func (s *Service) valueOpenObserved(ctx context.Context, positions []*Position) (value float64, hasActive bool, quality string, asOf time.Time, err error) {
	quality = "complete"
	asOf = time.Now().UTC()

	quotes := map[string]observedQuote{}
	rates := map[string]float64{}

	for _, pos := range positions {
		if positionStatus(pos) != PositionStatusOpen {
			continue
		}
		hasActive = true
		q, ok := quotes[pos.Symbol]
		if !ok {
			price, perr := s.provider.GetLatestPrice(ctx, pos.Symbol)
			if perr != nil || price == nil || !finitePositive(price.Price) {
				return 0, false, "", time.Time{}, ErrPriceProvider
			}
			if price.IsStale {
				quality = "stale"
			}
			observed := price.FetchedAt
			if observed.IsZero() {
				observed = price.Timestamp
			}
			if !observed.IsZero() && observed.UTC().Before(asOf) {
				asOf = observed.UTC()
			}
			q = observedQuote{price: price.Price, currency: price.Currency}
			quotes[pos.Symbol] = q
		}
		rate, ok := rates[q.currency]
		if !ok {
			r, ferr := s.fx.GetRate(ctx, q.currency, fx.BaseCurrency)
			if ferr != nil || !finitePositive(r) {
				return 0, false, "", time.Time{}, ErrUnsupportedCurrency
			}
			rate = r
			rates[q.currency] = r
		}
		// pos.Quantity.Float64() is a documented boundary conversion: this
		// running valuation stays float64 (out of scope for this section).
		value += pos.Quantity.Float64() * q.price * rate
	}
	if !isFinite(value) || value < 0 {
		return 0, false, "", time.Time{}, ErrPriceProvider
	}
	return value, hasActive, quality, asOf, nil
}

// --- pure set helpers used by the mutation coordinator ------------------------

// openPositions returns only the open positions from a slice.
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
// updated. Non-matching positions are carried over unchanged.
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
