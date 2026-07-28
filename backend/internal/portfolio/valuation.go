package portfolio

import (
	"context"
	"math"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/fx"
)

// Valuation is an immutable market observation used for ONE mutation. It pins
// exactly one price per symbol and one FX rate per currency, so the pre- and
// post-mutation valuations are computed against identical observations and the
// mutation itself can never manufacture a ranked return.
//
// Every observation is validated on entry: NaN, infinity, zero and negative
// prices/rates are rejected, so an unusable quote aborts the mutation before any
// database write rather than silently valuing a position at zero.
type Valuation struct {
	quotes            map[string]quoteObservation
	rates             map[string]float64
	ObservedAt        time.Time
	ValuationAsOf     time.Time
	DataQualityStatus string
}

type quoteObservation struct {
	Price    float64
	Currency string
}

func newValuation(at time.Time) *Valuation {
	return &Valuation{
		quotes:            map[string]quoteObservation{},
		rates:             map[string]float64{},
		ObservedAt:        at,
		ValuationAsOf:     at,
		DataQualityStatus: "complete",
	}
}

// Symbols returns the symbols covered by this observation, sorted for stable
// logging and test assertions.
func (v *Valuation) Symbols() []string {
	out := make([]string, 0, len(v.quotes))
	for s := range v.quotes {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Missing returns the subset of symbols this observation does not cover. The
// coordinator uses it after acquiring the aggregate lock: if the locked position
// set contains a symbol that was not priced, the transaction is abandoned and
// the whole calculation is rebuilt from current state rather than retried with
// stale inputs.
func (v *Valuation) Missing(symbols []string) []string {
	var missing []string
	for _, s := range symbols {
		if _, ok := v.quotes[s]; !ok {
			missing = append(missing, s)
		}
	}
	return missing
}

// Quote returns the pinned observation for a symbol.
func (v *Valuation) Quote(symbol string) (quoteObservation, bool) {
	q, ok := v.quotes[symbol]
	return q, ok
}

// pinCostBasisFallback records a NON-market observation for a symbol that has
// no live or cached price available. It exists only so a write-off's "value
// before" checkpoint has a number to subtract when the position disappears
// from "value after" — the position's own known cost basis, never an
// invented market price. Callers must never use this to VALUE a position for
// ordinary holdings/summary purposes; it is scoped to the one write-off
// mutation that is about to realize this exact basis as a loss anyway.
func (v *Valuation) pinCostBasisFallback(symbol string, costBasisPrice float64, currency string) {
	v.quotes[symbol] = quoteObservation{Price: costBasisPrice, Currency: currency}
	v.DataQualityStatus = "stale"
}

// ValueOpen returns the base-currency market value of the OPEN positions in the
// set and whether any exist. It is pure with respect to the pinned observation:
// no network calls, fully deterministic.
func (v *Valuation) ValueOpen(positions []*Position) (float64, bool, error) {
	var total float64
	hasActive := false
	for _, pos := range positions {
		if positionStatus(pos) != PositionStatusOpen {
			continue
		}
		hasActive = true
		q, ok := v.quotes[pos.Symbol]
		if !ok {
			return 0, false, ErrPriceProvider
		}
		rate, ok := v.rates[q.Currency]
		if !ok {
			return 0, false, ErrPriceProvider
		}
		if !finitePositive(pos.Quantity) {
			return 0, false, ErrInvalidQuantity
		}
		total += pos.Quantity * q.Price * rate
	}
	if !isFinite(total) || total < 0 {
		return 0, false, ErrPriceProvider
	}
	return total, hasActive, nil
}

func (v *Valuation) ValueTracked(positions []*Position, cash []CashBalance) (float64, bool, error) {
	total, active, err := v.ValueOpen(positions)
	if err != nil {
		return 0, false, err
	}
	for _, balance := range cash {
		// Exact decimal sign check: cash balances must never be negative. No
		// epsilon tolerance — decimal arithmetic makes exact comparison safe.
		if balance.Amount.Sign() < 0 {
			return 0, false, ErrInsufficientCash
		}
		if balance.Amount.IsZero() {
			continue
		}
		rate, ok := v.rates[balance.Currency]
		if !ok {
			return 0, false, ErrUnsupportedCurrency
		}
		// balance.Amount.Float64() is a documented boundary conversion: the
		// running valuation total and FX rate remain float64 because they feed
		// internal/performance and internal/fx, which are not part of this
		// section's scope.
		total += balance.Amount.Float64() * rate
		active = true
	}
	if !isFinite(total) || total < 0 {
		return 0, false, ErrPriceProvider
	}
	return total, active && total > 0, nil
}

func (c *MutationCoordinator) observeCurrency(ctx context.Context, v *Valuation, currency string) error {
	if _, ok := v.rates[currency]; ok {
		return nil
	}
	rate, err := c.fx.GetRate(ctx, currency, fx.BaseCurrency)
	if err != nil || !finitePositive(rate) {
		return ErrUnsupportedCurrency
	}
	v.rates[currency] = rate
	return nil
}

// observe fetches and validates the quote and FX rate for each requested symbol,
// skipping symbols already pinned so a symbol is never fetched twice within one
// mutation.
func (c *MutationCoordinator) observe(ctx context.Context, v *Valuation, symbols []string) error {
	for _, symbol := range symbols {
		if _, ok := v.quotes[symbol]; ok {
			continue
		}
		quote, err := c.provider.GetLatestPrice(ctx, symbol)
		if err != nil || quote == nil {
			return ErrPriceProvider
		}
		if !finitePositive(quote.Price) {
			return ErrPriceProvider
		}
		if quote.IsStale {
			v.DataQualityStatus = "stale"
		}
		asOf := quote.FetchedAt
		if asOf.IsZero() {
			asOf = quote.Timestamp
		}
		if !asOf.IsZero() && asOf.UTC().Before(v.ValuationAsOf) {
			v.ValuationAsOf = asOf.UTC()
		}
		if _, ok := v.rates[quote.Currency]; !ok {
			rate, err := c.fx.GetRate(ctx, quote.Currency, fx.BaseCurrency)
			if err != nil {
				return ErrUnsupportedCurrency
			}
			if !finitePositive(rate) {
				return ErrUnsupportedCurrency
			}
			v.rates[quote.Currency] = rate
		}
		v.quotes[symbol] = quoteObservation{Price: quote.Price, Currency: quote.Currency}
	}
	return nil
}

// openSymbols returns the distinct symbols of the OPEN positions in the set.
func openSymbols(positions []*Position) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(positions))
	for _, p := range positions {
		if positionStatus(p) != PositionStatusOpen || seen[p.Symbol] {
			continue
		}
		seen[p.Symbol] = true
		out = append(out, p.Symbol)
	}
	return out
}

// finitePositive rejects NaN, infinity, zero and negative values — the guard
// applied to every price, FX rate and quantity before it can influence a
// valuation or reach the database.
func finitePositive(v float64) bool { return isFinite(v) && v > 0 }

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// executionPriceMinFactor/executionPriceMaxFactor bound how far a LIVE
// (non-backdated) user-reported execution price may sit from the tracked
// market quote pinned for the same mutation. They are deliberately generous —
// ordinary volatility, wide spreads, and thinly-traded symbols must never trip
// this — the goal is only to catch obvious fabrication (e.g. claiming a fill
// at a small fraction, or a large multiple, of the real price).
const (
	executionPriceMinFactor = 0.2 // must be >= 20% of the live quote
	executionPriceMaxFactor = 5.0 // must be <= 500% of the live quote
)

// validateLiveExecutionPrice enforces that bound for a live trade with a
// user-supplied price. It is a no-op for a backdated trade (effectiveAt != nil
// — there is no live comparator for a claimed past price, and backdating a
// real historical trade is a deliberate, supported feature) and for a
// provider-estimated price (the user left the field blank, so the "execution
// price" IS the quote by construction).
func validateLiveExecutionPrice(effectiveAt *time.Time, source string, executionPrice, quotePrice float64) error {
	if effectiveAt != nil || source != PriceSourceUserRecorded {
		return nil
	}
	if !finitePositive(quotePrice) {
		return nil // nothing to compare against; other validation already covers this case
	}
	ratio := executionPrice / quotePrice
	if ratio < executionPriceMinFactor || ratio > executionPriceMaxFactor {
		return ErrImplausibleExecutionPrice
	}
	return nil
}
