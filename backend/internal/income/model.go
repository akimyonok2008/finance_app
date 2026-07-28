// Package income implements the automatic, provider-driven investment-income
// pipeline: ingest normalized issuer/fund income events (dividends, ETF and fund
// distributions, bond coupons, return of capital, stock dividends), match them
// to stable instruments, discover affected portfolios, calculate eligibility
// from HISTORICAL holdings, and credit cash (or reinvested shares) automatically
// through the portfolio aggregate coordinator.
//
// Users never enter ordinary dividends or distributions; they only see
// read-only timeline entries and may correct account-specific differences on an
// already-detected event. The portfolio domain depends only on this package's
// normalized model, never on any specific data provider.
package income

import (
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// Type is the normalized income-event type.
type Type string

const (
	TypeCashDividend     Type = "cash_dividend"
	TypeSpecialDividend  Type = "special_dividend"
	TypeETFDistribution  Type = "etf_distribution"
	TypeMutualFundDist   Type = "mutual_fund_distribution"
	TypeBondCoupon       Type = "bond_coupon"
	TypeFixedIncomeInt   Type = "fixed_income_interest"
	TypeCashInterest     Type = "cash_interest"
	TypeCapitalGainsDist Type = "capital_gains_distribution"
	TypeReturnOfCapital  Type = "return_of_capital"
	TypeStockDividend    Type = "stock_dividend"
	TypePaymentInLieu    Type = "payment_in_lieu"
	TypeStakingReward    Type = "staking_reward"
	TypeOtherProvider    Type = "other_provider_income"
)

// Classification groups types by their economic/accounting semantics so the
// pipeline applies the correct rule. Not every distribution is ordinary income.
type Classification string

const (
	// ClassOrdinary increases cash and ranked performance (cash dividend, ETF
	// distribution, coupon, interest, capital-gains distribution, staking).
	ClassOrdinary Classification = "ordinary"
	// ClassReturnOfCapital increases cash but reduces cost basis; not ordinary
	// income.
	ClassReturnOfCapital Classification = "return_of_capital"
	// ClassStockDividend is a quantity transformation with no cash and no ranked
	// jump.
	ClassStockDividend Classification = "stock_dividend"
)

// Classify maps a type to its accounting classification.
func Classify(t Type) Classification {
	switch t {
	case TypeReturnOfCapital:
		return ClassReturnOfCapital
	case TypeStockDividend:
		return ClassStockDividend
	default:
		return ClassOrdinary
	}
}

// Status is the lifecycle state of a normalized income event.
type Status string

const (
	StatusDetected             Status = "detected"
	StatusScheduled            Status = "scheduled"
	StatusEligibilityCalcuated Status = "eligibility_calculated"
	StatusAwaitingPayment      Status = "awaiting_payment"
	StatusReadyToApply         Status = "ready_to_apply"
	StatusApplied              Status = "applied"
	StatusCorrected            Status = "corrected"
	StatusCancelled            Status = "cancelled"
	StatusSuperseded           Status = "superseded"
	StatusUnresolved           Status = "unresolved"
	StatusFailedRetryable      Status = "failed_retryable"
	StatusFailedPermanent      Status = "failed_permanent"
	// StatusProcessedNoEntitlement is non-terminal: later identity resolution
	// or a provider correction can still make the event eligible.
	StatusProcessedNoEntitlement Status = "processed_no_entitlement"
)

// Quality is the confidence level of the event data.
type Quality string

const (
	QualityVerified       Quality = "verified"
	QualityHighConfidence Quality = "high_confidence"
	QualityIncomplete     Quality = "incomplete"
	QualityConflicting    Quality = "conflicting"
	QualityUnverified     Quality = "unverified"
	QualityCancelled      Quality = "cancelled"
)

func (q Quality) meetsAutoApplyPolicy() bool {
	return q == QualityVerified || q == QualityHighConfidence
}

// InstrumentReference identifies a security by whatever stable identifiers a
// provider supplies. Matching uses a deterministic priority; symbol is the
// minimum required.
type InstrumentReference struct {
	InstrumentID string `json:"instrument_id,omitempty"`
	ProviderID   string `json:"provider_id,omitempty"`
	FIGI         string `json:"figi,omitempty"`
	ISIN         string `json:"isin,omitempty"`
	CUSIP        string `json:"cusip,omitempty"`
	Exchange     string `json:"exchange,omitempty"`
	MIC          string `json:"mic,omitempty"`
	Currency     string `json:"currency,omitempty"`
	Symbol       string `json:"symbol"`
}

// IncomeEventRequest is the neutral provider fetch request.
type IncomeEventRequest struct {
	Instruments []InstrumentReference
	Since       time.Time
	Until       time.Time
}

// ProviderComponent is one economic slice of a mixed distribution (e.g. an ETF
// distribution split into ordinary income, capital gain and return of capital).
type ProviderComponent struct {
	Type          Type        `json:"type"`
	AmountPerUnit money.Price `json:"amount_per_unit"`
}

// ProviderIncomeEvent is a raw event as returned by a provider adapter, before
// normalization.
type ProviderIncomeEvent struct {
	ProviderEventID string
	Type            Type
	Instrument      InstrumentReference

	AmountPerUnit money.Price
	Currency      string
	// Components, when present, describe a mixed distribution. Their per-unit
	// amounts should sum to AmountPerUnit.
	Components []ProviderComponent

	DeclarationAt *time.Time
	ExDate        *time.Time
	RecordDate    *time.Time
	PaymentDate   time.Time
	Frequency     string

	TaxClassification string
	Cancelled         bool
	SourceURL         string
	RetrievedAt       time.Time
}

// IncomeEvent is the normalized, stored event. The portfolio pipeline consumes
// only this shape.
type IncomeEvent struct {
	ID              string
	Provider        string
	ProviderEventID string
	Type            Type

	Instrument InstrumentReference

	AmountPerUnit money.Price
	Currency      string
	Components    []ProviderComponent

	DeclarationAt *time.Time
	ExDate        *time.Time
	RecordDate    *time.Time
	PaymentDate   time.Time
	Frequency     string

	Status  Status
	Quality Quality

	TaxClassification string
	SourceURL         string
	RawFingerprint    string
	RetrievedAt       time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// entitlementDate returns the date used to compute eligible holdings: the
// ex-date when known, otherwise the record date, otherwise the payment date.
func (e IncomeEvent) entitlementDate() time.Time {
	if e.ExDate != nil {
		return *e.ExDate
	}
	if e.RecordDate != nil {
		return *e.RecordDate
	}
	return e.PaymentDate
}

// ApplicationStatus tracks a single (event, portfolio) application.
type ApplicationStatus string

const (
	ApplicationPending   ApplicationStatus = "pending"
	ApplicationApplying  ApplicationStatus = "applying"
	ApplicationApplied   ApplicationStatus = "applied"
	ApplicationFailed    ApplicationStatus = "failed"
	ApplicationSkipped   ApplicationStatus = "skipped"
	ApplicationCorrected ApplicationStatus = "corrected"
)

// Application is the durable, idempotent record that an event was (or is being)
// applied to one portfolio. Gross, withholding, fee and net are stored
// distinctly — never one ambiguous amount.
type Application struct {
	IncomeEventID string
	PortfolioID   string
	UserID        string
	Status        ApplicationStatus

	EligibleQuantity     money.Quantity
	GrossAmount          money.Amount
	WithholdingAmount    money.Amount
	FeeAmount            money.Amount
	NetAmount            money.Amount
	CashCurrency         string
	ReinvestmentQuantity money.Quantity
	Estimated            bool

	PortfolioVersionBefore   int64
	PortfolioVersionAfter    int64
	PerformanceVersionBefore int64
	PerformanceVersionAfter  int64

	AppliedAt   *time.Time
	ErrorCode   string
	RetryCount  int
	NextRetryAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
