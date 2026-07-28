package money

import (
	"fmt"

	"github.com/shopspring/decimal"
)

// Each domain type below wraps Decimal so that values with different
// economic meaning cannot be combined by accident (e.g. Amount + Quantity
// does not compile). Conversions between types must go through an explicit
// operation (e.g. Quantity.MulPrice -> Amount) rather than a bare cast.

// Amount represents a cash value in some currency (cost basis, market
// value, P&L, fees, dividends, etc. all use Amount at the appropriate
// scale).
type Amount struct{ Decimal }

// Quantity represents a security/share/unit quantity.
type Quantity struct{ Decimal }

// Price represents a per-unit price.
type Price struct{ Decimal }

// FXRate represents a historical or spot foreign-exchange conversion rate.
type FXRate struct{ Decimal }

// Ratio represents a dimensionless ratio (e.g. a return, a growth factor).
type Ratio struct{ Decimal }

// IndexValue represents a ranked-performance index value or benchmark NAV.
type IndexValue struct{ Decimal }

// Weight represents a benchmark component weight (expected to sum to 1).
type Weight struct{ Decimal }

// AmountFromFloat64 converts a legacy float64 cash value into an Amount,
// exactly (via the float64's own decimal representation, not a truncated
// string). It exists ONLY for interop with packages not yet migrated off
// float64 (e.g. internal/fx, internal/performance) and must never be used to
// silently re-introduce float64 arithmetic inside already-converted domain
// code — convert once at the boundary, then use Amount arithmetic.
func AmountFromFloat64(f float64) Amount {
	return Amount{Decimal{d: decimal.NewFromFloat(f)}}
}

// Float64 converts an Amount back to float64, for interop with a legacy
// float64 API at a package boundary (e.g. calling internal/fx.Convert).
// Precision beyond float64's ~15-17 significant digits is lost; do not use
// this for authoritative comparisons or further arithmetic within
// already-converted decimal code.
func (a Amount) Float64() float64 {
	f, _ := a.Decimal.d.Float64()
	return f
}

// QuantityFromFloat64 / Float64: same documented boundary-conversion
// contract as AmountFromFloat64/Amount.Float64, for security/unit
// quantities.
func QuantityFromFloat64(f float64) Quantity {
	return Quantity{Decimal{d: decimal.NewFromFloat(f)}}
}

func (q Quantity) Float64() float64 {
	f, _ := q.Decimal.d.Float64()
	return f
}

// PriceFromFloat64 / Float64: same documented boundary-conversion contract,
// for per-unit prices.
func PriceFromFloat64(f float64) Price {
	return Price{Decimal{d: decimal.NewFromFloat(f)}}
}

func (p Price) Float64() float64 {
	f, _ := p.Decimal.d.Float64()
	return f
}

// RatioFromFloat64 / Float64: same documented boundary-conversion contract,
// for dimensionless ratios/factors.
func RatioFromFloat64(f float64) Ratio {
	return Ratio{Decimal{d: decimal.NewFromFloat(f)}}
}

func (r Ratio) Float64() float64 {
	f, _ := r.Decimal.d.Float64()
	return f
}

func mustNew(s string) Decimal {
	d, err := newDecimal(s)
	if err != nil {
		panic(err)
	}
	return d
}

// Zero constructors.
func ZeroAmount() Amount         { return Amount{mustNew("0")} }
func ZeroQuantity() Quantity     { return Quantity{mustNew("0")} }
func ZeroPrice() Price           { return Price{mustNew("0")} }
func ZeroFXRate() FXRate         { return FXRate{mustNew("0")} }
func ZeroRatio() Ratio           { return Ratio{mustNew("0")} }
func ZeroIndexValue() IndexValue { return IndexValue{mustNew("0")} }
func ZeroWeight() Weight         { return Weight{mustNew("0")} }

// Parsing constructors: exact string parsing only, never through float64.
func ParseAmount(s string) (Amount, error) {
	d, err := newDecimal(s)
	if err != nil {
		return Amount{}, fmt.Errorf("money: parse amount: %w", err)
	}
	return Amount{d}, nil
}

func ParseQuantity(s string) (Quantity, error) {
	d, err := newDecimal(s)
	if err != nil {
		return Quantity{}, fmt.Errorf("money: parse quantity: %w", err)
	}
	return Quantity{d}, nil
}

func ParsePrice(s string) (Price, error) {
	d, err := newDecimal(s)
	if err != nil {
		return Price{}, fmt.Errorf("money: parse price: %w", err)
	}
	return Price{d}, nil
}

func ParseFXRate(s string) (FXRate, error) {
	d, err := newDecimal(s)
	if err != nil {
		return FXRate{}, fmt.Errorf("money: parse fx rate: %w", err)
	}
	return FXRate{d}, nil
}

func ParseRatio(s string) (Ratio, error) {
	d, err := newDecimal(s)
	if err != nil {
		return Ratio{}, fmt.Errorf("money: parse ratio: %w", err)
	}
	return Ratio{d}, nil
}

func ParseIndexValue(s string) (IndexValue, error) {
	d, err := newDecimal(s)
	if err != nil {
		return IndexValue{}, fmt.Errorf("money: parse index value: %w", err)
	}
	return IndexValue{d}, nil
}

func ParseWeight(s string) (Weight, error) {
	d, err := newDecimal(s)
	if err != nil {
		return Weight{}, fmt.Errorf("money: parse weight: %w", err)
	}
	return Weight{d}, nil
}

// Must* constructors parse a literal decimal string and panic on failure. They
// are intended for compile-time-constant literals (catalogue data, test
// fixtures) where the string is known-good, never for untrusted input.
func MustAmount(s string) Amount         { return Amount{mustNew(s)} }
func MustQuantity(s string) Quantity     { return Quantity{mustNew(s)} }
func MustPrice(s string) Price           { return Price{mustNew(s)} }
func MustFXRate(s string) FXRate         { return FXRate{mustNew(s)} }
func MustRatio(s string) Ratio           { return Ratio{mustNew(s)} }
func MustIndexValue(s string) IndexValue { return IndexValue{mustNew(s)} }
func MustWeight(s string) Weight         { return Weight{mustNew(s)} }

// ---- Amount arithmetic ----

// Cmp compares two Amounts: -1, 0, 1. Exact decimal comparison, no epsilon.
func (a Amount) Cmp(b Amount) int { return a.Decimal.Cmp(b.Decimal) }

// EqualAmount reports exact equality between two Amounts (no epsilon).
func (a Amount) EqualAmount(b Amount) bool { return a.Decimal.Equal(b.Decimal) }

func (a Amount) Add(b Amount) Amount { return Amount{a.Decimal.add(b.Decimal)} }
func (a Amount) Sub(b Amount) Amount { return Amount{a.Decimal.sub(b.Decimal)} }
func (a Amount) Neg() Amount         { return Amount{Decimal{d: a.Decimal.d.Neg()}} }

// StringFixedCurrency renders a (typically already-quantized) Amount at the
// given currency's minor-unit scale, e.g. "1.00" for USD.
func (a Amount) StringFixedCurrency(currency string) (string, error) {
	scale, err := CurrencyScale(currency)
	if err != nil {
		return "", err
	}
	return a.Decimal.StringFixed(scale), nil
}

// MulRatio scales an amount by a dimensionless ratio (e.g. applying a
// weight or a return factor), keeping full precision.
func (a Amount) MulRatio(r Ratio) Amount { return Amount{a.Decimal.mul(r.Decimal)} }

// DivExact divides one amount by another producing a dimensionless Ratio,
// at the given precision (round-half-even), e.g. for return calculations.
func (a Amount) DivExact(b Amount, places int32) (Ratio, error) {
	d, err := a.Decimal.divPrecise(b.Decimal, places)
	if err != nil {
		return Ratio{}, err
	}
	return Ratio{d}, nil
}

// ---- Quantity arithmetic ----

// Cmp compares two Quantities: -1, 0, 1. Exact decimal comparison, no
// epsilon.
func (q Quantity) Cmp(o Quantity) int { return q.Decimal.Cmp(o.Decimal) }

// EqualQuantity reports exact equality between two Quantities (no epsilon).
func (q Quantity) EqualQuantity(o Quantity) bool { return q.Decimal.Equal(o.Decimal) }

func (q Quantity) Add(o Quantity) Quantity { return Quantity{q.Decimal.add(o.Decimal)} }
func (q Quantity) Sub(o Quantity) Quantity { return Quantity{q.Decimal.sub(o.Decimal)} }
func (q Quantity) Neg() Quantity           { return Quantity{Decimal{d: q.Decimal.d.Neg()}} }

// MulPrice computes quantity * price -> cash Amount (full precision, no
// implicit rounding; caller quantizes at the posting boundary).
func (q Quantity) MulPrice(p Price) Amount { return Amount{q.Decimal.mul(p.Decimal)} }

// MulRatio scales a quantity by a dimensionless ratio/factor (e.g. a split
// or stock-dividend factor), keeping full precision.
func (q Quantity) MulRatio(r Ratio) Quantity { return Quantity{q.Decimal.mul(r.Decimal)} }

// DivExact divides one quantity by another producing a dimensionless Ratio
// (e.g. fractional fill ratio), at the given precision.
func (q Quantity) DivExact(o Quantity, places int32) (Ratio, error) {
	d, err := q.Decimal.divPrecise(o.Decimal, places)
	if err != nil {
		return Ratio{}, err
	}
	return Ratio{d}, nil
}

// ---- Price / FXRate ----

// Cmp compares two Prices: -1, 0, 1. Exact decimal comparison, no epsilon.
func (p Price) Cmp(o Price) int { return p.Decimal.Cmp(o.Decimal) }

// Convert applies an FX rate to a price/amount pairing: price * rate ->
// Price in the destination currency.
func (p Price) Convert(rate FXRate) Price { return Price{p.Decimal.mul(rate.Decimal)} }

// DivRatio divides a Price by a dimensionless ratio/factor (e.g. undoing a
// split or stock-dividend factor on the per-share baseline), at the given
// explicit precision.
func (p Price) DivRatio(r Ratio, places int32) (Price, error) {
	d, err := p.Decimal.divPrecise(r.Decimal, places)
	if err != nil {
		return Price{}, err
	}
	return Price{d}, nil
}

func (a Amount) Convert(rate FXRate) Amount { return Amount{a.Decimal.mul(rate.Decimal)} }

// DivByQuantity divides an Amount by a Quantity to produce a per-unit
// Price (e.g. weighted-average cost basis / total quantity), at the given
// explicit precision. This is the one place cost-basis Amounts become a
// Price; callers quantize with QuantizePrice only at the posting boundary.
func (a Amount) DivByQuantity(q Quantity, places int32) (Price, error) {
	d, err := a.Decimal.divPrecise(q.Decimal, places)
	if err != nil {
		return Price{}, err
	}
	return Price{d}, nil
}

// DivByPrice divides an Amount by a Price to produce a Quantity (e.g. a
// reinvested cash amount at a per-share price -> reinvested share count),
// at the given explicit precision. Callers quantize with QuantizeQuantity
// only at the posting boundary.
func (a Amount) DivByPrice(p Price, places int32) (Quantity, error) {
	d, err := a.Decimal.divPrecise(p.Decimal, places)
	if err != nil {
		return Quantity{}, err
	}
	return Quantity{d}, nil
}

func (p Price) Add(o Price) Price      { return Price{p.Decimal.add(o.Decimal)} }
func (p Price) Sub(o Price) Price      { return Price{p.Decimal.sub(o.Decimal)} }
func (p Price) MulRatio(r Ratio) Price { return Price{p.Decimal.mul(r.Decimal)} }

// DivExact divides one price by another, producing a dimensionless Ratio
// (e.g. curr/prev for a period return), at the given precision
// (round-half-even). It is not a posting-boundary rounding; callers
// requantize at persistence time if the result is itself a Price/Index.
func (p Price) DivExact(o Price, places int32) (Ratio, error) {
	d, err := p.Decimal.divPrecise(o.Decimal, places)
	if err != nil {
		return Ratio{}, err
	}
	return Ratio{d}, nil
}

// ---- IndexValue / Ratio ----

// MulRatio scales an index value by a ratio (e.g. checkpoint index *
// current/segment-start ratio in ranked performance).
func (i IndexValue) MulRatio(r Ratio) IndexValue { return IndexValue{i.Decimal.mul(r.Decimal)} }

// Sub subtracts two index values, producing a dimensionless Ratio (e.g. an
// index expressed against a 100-base yields percentage points directly).
func (i IndexValue) Sub(o IndexValue) Ratio { return Ratio{i.Decimal.sub(o.Decimal)} }

// DivExact divides one index value by another at the given precision
// (round-half-even), producing a dimensionless Ratio (e.g. last/first for a
// period return).
func (i IndexValue) DivExact(o IndexValue, places int32) (Ratio, error) {
	d, err := i.Decimal.divPrecise(o.Decimal, places)
	if err != nil {
		return Ratio{}, err
	}
	return Ratio{d}, nil
}

func (r Ratio) Mul(o Ratio) Ratio { return Ratio{r.Decimal.mul(o.Decimal)} }
func (r Ratio) Add(o Ratio) Ratio { return Ratio{r.Decimal.add(o.Decimal)} }
func (r Ratio) Sub(o Ratio) Ratio { return Ratio{r.Decimal.sub(o.Decimal)} }

// DivExact divides one ratio by another at the given precision
// (round-half-even), for explicit-precision intermediate math (e.g. a
// fractional position within a series).
func (r Ratio) DivExact(o Ratio, places int32) (Ratio, error) {
	d, err := r.Decimal.divPrecise(o.Decimal, places)
	if err != nil {
		return Ratio{}, err
	}
	return Ratio{d}, nil
}

// ---- Weight ----

func (w Weight) Add(o Weight) Weight { return Weight{w.Decimal.add(o.Decimal)} }
func (w Weight) Sub(o Weight) Weight { return Weight{w.Decimal.sub(o.Decimal)} }

// Mul multiplies two weights (e.g. a nested recipe leg's weight scaled by its
// parent's weight), producing an effective Weight.
func (w Weight) Mul(o Weight) Weight { return Weight{w.Decimal.mul(o.Decimal)} }

// MulRatio scales a weight by a dimensionless ratio (e.g. a leg's weight
// times its per-period return), producing a Ratio contribution.
func (w Weight) MulRatio(r Ratio) Ratio { return Ratio{w.Decimal.mul(r.Decimal)} }

// DivExact divides one weight by another at the given precision
// (round-half-even), e.g. renormalizing a raw weight against the sum of all
// raw weights so the result sums to exactly 1.
func (w Weight) DivExact(o Weight, places int32) (Weight, error) {
	d, err := w.Decimal.divPrecise(o.Decimal, places)
	if err != nil {
		return Weight{}, err
	}
	return Weight{d}, nil
}

// ---- Quantization helpers (posting/persistence boundary only) ----

// QuantizeCash rounds an Amount to the given currency's minor-unit scale
// using round-half-even. Returns an error for unknown currencies.
func QuantizeCash(a Amount, currency string) (Amount, error) {
	scale, err := CurrencyScale(currency)
	if err != nil {
		return Amount{}, err
	}
	return Amount{a.Decimal.quantize(scale)}, nil
}

// QuantizeQuantity rounds a Quantity to the policy scale (18dp).
func QuantizeQuantity(q Quantity) Quantity {
	return Quantity{q.Decimal.quantize(ScaleQuantity)}
}

// QuantizePrice rounds a Price to the policy scale (12dp).
func QuantizePrice(p Price) Price {
	return Price{p.Decimal.quantize(ScalePrice)}
}

// QuantizeFX rounds an FXRate to the policy scale (12dp).
func QuantizeFX(r FXRate) FXRate {
	return FXRate{r.Decimal.quantize(ScaleFX)}
}

// QuantizeIndex rounds an IndexValue to the policy scale (18dp).
func QuantizeIndex(i IndexValue) IndexValue {
	return IndexValue{i.Decimal.quantize(ScaleIndex)}
}

// QuantizeValue rounds a market-value Amount to the policy scale (18dp).
func QuantizeValue(a Amount) Amount {
	return Amount{a.Decimal.quantize(ScaleValue)}
}

// QuantizeWeight rounds a Weight to the policy scale (18dp).
func QuantizeWeight(w Weight) Weight {
	return Weight{w.Decimal.quantize(ScaleWeight)}
}

// QuantizeCostBasis rounds a cost-basis Amount to the policy scale (18dp).
func QuantizeCostBasis(a Amount) Amount {
	return Amount{a.Decimal.quantize(ScaleCostBasis)}
}
