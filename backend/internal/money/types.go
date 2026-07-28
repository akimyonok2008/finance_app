package money

import "fmt"

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

// ---- Amount arithmetic ----

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

func (q Quantity) Add(o Quantity) Quantity { return Quantity{q.Decimal.add(o.Decimal)} }
func (q Quantity) Sub(o Quantity) Quantity { return Quantity{q.Decimal.sub(o.Decimal)} }
func (q Quantity) Neg() Quantity           { return Quantity{Decimal{d: q.Decimal.d.Neg()}} }

// MulPrice computes quantity * price -> cash Amount (full precision, no
// implicit rounding; caller quantizes at the posting boundary).
func (q Quantity) MulPrice(p Price) Amount { return Amount{q.Decimal.mul(p.Decimal)} }

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

// Convert applies an FX rate to a price/amount pairing: price * rate ->
// Price in the destination currency.
func (p Price) Convert(rate FXRate) Price { return Price{p.Decimal.mul(rate.Decimal)} }

func (a Amount) Convert(rate FXRate) Amount { return Amount{a.Decimal.mul(rate.Decimal)} }

// ---- IndexValue / Ratio ----

// MulRatio scales an index value by a ratio (e.g. checkpoint index *
// current/segment-start ratio in ranked performance).
func (i IndexValue) MulRatio(r Ratio) IndexValue { return IndexValue{i.Decimal.mul(r.Decimal)} }

func (r Ratio) Mul(o Ratio) Ratio { return Ratio{r.Decimal.mul(o.Decimal)} }
func (r Ratio) Add(o Ratio) Ratio { return Ratio{r.Decimal.add(o.Decimal)} }
func (r Ratio) Sub(o Ratio) Ratio { return Ratio{r.Decimal.sub(o.Decimal)} }

// ---- Weight ----

func (w Weight) Add(o Weight) Weight { return Weight{w.Decimal.add(o.Decimal)} }

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
