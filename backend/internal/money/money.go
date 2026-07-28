// Package money provides exact decimal arithmetic types for financial
// domain code. It wraps github.com/shopspring/decimal with named types
// (Amount, Quantity, Price, FXRate, Ratio, IndexValue) so that values with
// different economic meaning cannot be accidentally combined (e.g. adding a
// share Quantity to a cash Amount), while keeping arithmetic readable.
//
// Precision policy (see backend/docs/PRECISION_POLICY.md):
//   - Cash posting: currency-specific minor-unit scale (USD/EUR/GBP=2, JPY=0,
//     KWD-style=3). Unknown currencies fail validation rather than guessing.
//   - Cost basis: >=18 decimal places internally.
//   - Quantity: up to 18 decimal places.
//   - Price: up to 12 decimal places.
//   - Historical FX rate: up to 12 decimal places.
//   - Portfolio market value: >=18 decimal places internally.
//   - Benchmark weight: >=18 decimal places.
//   - Ranked index / Benchmark NAV: >=18 decimal places internally.
//
// Rounding is round-half-even (banker's rounding) and is applied ONLY at
// explicit posting/persistence boundaries via the QuantizeXxx helpers below.
// Intermediate arithmetic never rounds implicitly.
package money

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/shopspring/decimal"
)

// Scales used by QuantizeXxx helpers, per precision policy.
const (
	ScaleCostBasis = 18
	ScaleQuantity  = 18
	ScalePrice     = 12
	ScaleFX        = 12
	ScaleValue     = 18
	ScaleWeight    = 18
	ScaleIndex     = 18
)

// currencyMinorUnits lists supported ISO-4217 currencies and their minor
// unit (decimal) scale. Unknown currencies must fail validation rather than
// silently guessing a scale.
var currencyMinorUnits = map[string]int32{
	"USD": 2, "EUR": 2, "GBP": 2, "CAD": 2, "AUD": 2, "CHF": 2,
	"JPY": 0, "KRW": 0,
	"KWD": 3, "BHD": 3, "OMR": 3, "TND": 3,
}

// CurrencyScale returns the minor-unit decimal scale for currency, or an
// error if the currency is unknown. Callers must not guess a default.
func CurrencyScale(currency string) (int32, error) {
	c := strings.ToUpper(strings.TrimSpace(currency))
	scale, ok := currencyMinorUnits[c]
	if !ok {
		return 0, fmt.Errorf("money: unknown currency %q: cannot determine minor-unit scale", currency)
	}
	return scale, nil
}

// Decimal is the shared underlying exact-precision value embedded by every
// domain type in this package. It is not meant to be used directly outside
// this package's type constructors.
type Decimal struct {
	d decimal.Decimal
}

// ---- parsing ----

// parseExact parses s as an exact decimal string. It rejects NaN, Inf,
// locale-formatted numbers (thousands separators), exponent notation is
// allowed by the underlying library but explicitly rejected here to keep
// canonical forms unambiguous, and oversized/malformed input.
func parseExact(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return decimal.Decimal{}, errors.New("money: empty decimal string")
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "nan") || strings.Contains(lower, "inf") {
		return decimal.Decimal{}, fmt.Errorf("money: invalid decimal value %q (NaN/Inf not allowed)", s)
	}
	if strings.ContainsAny(s, ",_ ") {
		return decimal.Decimal{}, fmt.Errorf("money: locale-formatted or malformed decimal %q", s)
	}
	if strings.ContainsAny(lower, "e") {
		return decimal.Decimal{}, fmt.Errorf("money: exponent notation not allowed %q", s)
	}
	if len(s) > 80 {
		return decimal.Decimal{}, fmt.Errorf("money: decimal string too large (%d chars)", len(s))
	}
	// Validate character set strictly: optional leading '-', digits, optional single '.'.
	body := s
	if strings.HasPrefix(body, "-") {
		body = body[1:]
	}
	if body == "" {
		return decimal.Decimal{}, fmt.Errorf("money: malformed decimal %q", s)
	}
	dotSeen := false
	for _, r := range body {
		if r == '.' {
			if dotSeen {
				return decimal.Decimal{}, fmt.Errorf("money: malformed decimal %q", s)
			}
			dotSeen = true
			continue
		}
		if r < '0' || r > '9' {
			return decimal.Decimal{}, fmt.Errorf("money: malformed decimal %q", s)
		}
	}
	d, err := decimal.NewFromString(s)
	if err != nil {
		return decimal.Decimal{}, fmt.Errorf("money: malformed decimal %q: %w", s, err)
	}
	return d, nil
}

func newDecimal(s string) (Decimal, error) {
	d, err := parseExact(s)
	if err != nil {
		return Decimal{}, err
	}
	return Decimal{d: d}, nil
}

// IsZero reports whether the value is exactly zero.
func (d Decimal) IsZero() bool { return d.d.IsZero() }

// Sign returns -1, 0, or 1.
func (d Decimal) Sign() int { return d.d.Sign() }

// Cmp compares d to other: -1, 0, 1.
func (d Decimal) Cmp(other Decimal) int { return d.d.Cmp(other.d) }

// Equal reports exact decimal equality (mathematical, not scale-sensitive:
// e.g. 1 == 1.0).
func (d Decimal) Equal(other Decimal) bool { return d.d.Equal(other.d) }

func (d Decimal) add(other Decimal) Decimal { return Decimal{d: d.d.Add(other.d)} }
func (d Decimal) sub(other Decimal) Decimal { return Decimal{d: d.d.Sub(other.d)} }
func (d Decimal) mul(other Decimal) Decimal { return Decimal{d: d.d.Mul(other.d)} }

// divPrecise divides d by other to the requested number of decimal places
// using round-half-even, for use where an explicit intermediate precision
// is required (e.g. ranked index computation). It is not a posting-boundary
// rounding by itself; callers still requantize at persistence time.
func (d Decimal) divPrecise(other Decimal, places int32) (Decimal, error) {
	if other.d.IsZero() {
		return Decimal{}, errors.New("money: division by zero")
	}
	return Decimal{d: d.d.DivRound(other.d, places)}, nil
}

// quantize rounds to `places` decimal places using round-half-even.
func (d Decimal) quantize(places int32) Decimal {
	return Decimal{d: d.d.RoundBank(places)}
}

// canonical returns the canonical string form: fixed-point, no exponent, no
// leading/trailing zero noise, no negative zero. Values that are
// mathematically equal (1, 1.0, 1.000000) canonicalize identically.
func (d Decimal) canonical() string {
	s := d.d.String()
	if s == "-0" {
		return "0"
	}
	if strings.Contains(s, ".") {
		s = strings.TrimRight(s, "0")
		s = strings.TrimRight(s, ".")
	}
	if s == "" || s == "-" {
		return "0"
	}
	if s == "-0" {
		return "0"
	}
	return s
}

func (d Decimal) String() string { return d.canonical() }

// StringFixed renders the value with exactly `places` decimal places
// (zero-padded), for contexts that need scale-preserving display (e.g. a
// quantized cash amount shown with its currency's minor-unit scale). This
// is distinct from the trimmed canonical() form used for fingerprinting.
func (d Decimal) StringFixed(places int32) string {
	return d.d.StringFixed(places)
}

// Rat exposes the value as a *big.Rat for callers needing arbitrary further
// exact math outside this package (e.g. fingerprinting).
func (d Decimal) Rat() *big.Rat {
	r := new(big.Rat)
	r.SetString(d.d.String())
	return r
}

// ---- JSON ----

func (d Decimal) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.canonical())
}

func (d *Decimal) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*d = Decimal{}
		return nil
	}
	var s string
	if len(b) > 0 && b[0] == '"' {
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("money: unmarshal decimal: %w", err)
		}
	} else {
		// Accept bare JSON numbers transitionally, routed through the exact
		// textual representation (never float64).
		s = string(b)
	}
	parsed, err := newDecimal(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// ---- SQL ----

func (d Decimal) Value() (driver.Value, error) {
	return d.d.Value()
}

func (d *Decimal) Scan(v interface{}) error {
	var inner decimal.Decimal
	if err := inner.Scan(v); err != nil {
		return fmt.Errorf("money: scan decimal: %w", err)
	}
	*d = Decimal{d: inner}
	return nil
}

// FromDecimal wraps a shopspring/decimal.Decimal (for interop at the SQL /
// legacy boundary only).
func FromDecimal(v decimal.Decimal) Decimal { return Decimal{d: v} }

// Underlying exposes the shopspring/decimal.Decimal for interop.
func (d Decimal) Underlying() decimal.Decimal { return d.d }
