package portfolio

import "errors"

// Typed domain errors for income, fees, and corporate actions. Handlers map
// these to HTTP status codes. None of them reveal another user's holdings.
var (
	ErrInsufficientCashForFee = errors.New("insufficient cash to cover fee")
	ErrInvalidIncomeAmount    = errors.New("income amount must be a finite positive number")
	ErrUnsupportedIncomeType  = errors.New("unsupported income type")
	ErrUnsupportedFeeType     = errors.New("unsupported fee type")
	ErrInvalidFeeAmount       = errors.New("fee amount must be a finite positive number")
	ErrInvalidCorporateAction = errors.New("invalid corporate action")
	ErrInvalidSplitRatio      = errors.New("split ratio numerator and denominator must be finite and positive")
	ErrInvalidReinvestment    = errors.New("reinvestment quantity and price must be finite and positive")
	ErrInstrumentNotFound     = errors.New("instrument not found in portfolio")
	ErrSymbolChangeConflict   = errors.New("target symbol already held; cannot convert into an existing distinct position")

	// Income-correction errors (the constrained, account-specific correction
	// path — never arbitrary income creation).
	ErrIncomeEventNotFound     = errors.New("income event not found")
	ErrIncomeNotApplied        = errors.New("income event has no application to correct")
	ErrInvalidIncomeCorrection = errors.New("invalid income correction")
)
