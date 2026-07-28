package income

import (
	"strings"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// WithholdingProfile is an OPTIONAL account-level estimate of withholding tax
// applied to gross income. It is portfolio TRACKING, not tax advice: providers
// report gross distributions, and a broker's actual withholding may differ. When
// no profile is configured the pipeline credits the gross amount, marked
// estimated, and allows a later broker-based correction.
//
// The zero value withholds nothing.
type WithholdingProfile struct {
	// DefaultRate is applied to ordinary cash income when no more specific rule
	// matches. Expressed as a fraction (0.15 == 15%).
	DefaultRate money.Ratio
	// TypeRates overrides the default for a specific income type.
	TypeRates map[Type]money.Ratio
	// SymbolRates overrides everything for a specific instrument symbol.
	SymbolRates map[string]money.Ratio
}

// Rate returns the withholding fraction for an event under this profile,
// clamped to [0, 1). Stock dividends and return of capital never withhold in
// this tracking model.
func (p WithholdingProfile) Rate(t Type, symbol string) money.Ratio {
	if t == TypeStockDividend || t == TypeReturnOfCapital {
		return money.ZeroRatio()
	}
	rate := p.DefaultRate
	if p.TypeRates != nil {
		if r, ok := p.TypeRates[t]; ok {
			rate = r
		}
	}
	if p.SymbolRates != nil {
		if r, ok := p.SymbolRates[strings.ToUpper(strings.TrimSpace(symbol))]; ok {
			rate = r
		}
	}
	if rate.Sign() < 0 {
		return money.ZeroRatio()
	}
	if rate.Cmp(money.MustRatio("1")) >= 0 {
		return money.MustRatio("0.999999")
	}
	return rate
}

// HasAny reports whether the profile withholds anything at all.
func (p WithholdingProfile) HasAny() bool {
	if p.DefaultRate.Sign() > 0 {
		return true
	}
	for _, r := range p.TypeRates {
		if r.Sign() > 0 {
			return true
		}
	}
	for _, r := range p.SymbolRates {
		if r.Sign() > 0 {
			return true
		}
	}
	return false
}
