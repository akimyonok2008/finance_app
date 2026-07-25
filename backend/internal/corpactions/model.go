// Package corpactions implements the automatic, provider-driven corporate-action
// pipeline: ingest normalized issuer/exchange events, match them to stable
// instruments, discover affected portfolios, and apply routine transformations
// (splits, ticker changes) automatically through the portfolio aggregate
// coordinator. Users never enter corporate actions; they only see read-only,
// plain-language results.
//
// The portfolio domain depends only on this package's normalized model, never on
// any specific data provider.
package corpactions

import "time"

// Type is the normalized corporate-action type.
type Type string

const (
	TypeSplit        Type = "split"
	TypeReverseSplit Type = "reverse_split"
	TypeSymbolChange Type = "symbol_change"
	TypeStockMerger  Type = "stock_merger"
	TypeCashMerger   Type = "cash_merger"
	TypeMixedMerger  Type = "mixed_merger"
	TypeSpinOff      Type = "spin_off"
	TypeDelisting    Type = "delisting"
)

// Status is the lifecycle state of a normalized event.
type Status string

const (
	StatusDetected        Status = "detected"
	StatusValidated       Status = "validated"
	StatusScheduled       Status = "scheduled"
	StatusApplying        Status = "applying"
	StatusApplied         Status = "applied"
	StatusIgnored         Status = "ignored"
	StatusSuperseded      Status = "superseded"
	StatusCancelled       Status = "cancelled"
	StatusUnresolved      Status = "unresolved"
	StatusFailedRetryable Status = "failed_retryable"
	StatusFailedPermanent Status = "failed_permanent"
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

// meetsAutoApplyPolicy reports whether a quality level is trusted enough for
// automatic application.
func (q Quality) meetsAutoApplyPolicy() bool {
	return q == QualityVerified || q == QualityHighConfidence
}

// InstrumentReference identifies a security by whatever stable identifiers a
// provider supplies. Not every field is present; matching uses a deterministic
// priority (see matchInstrument).
type InstrumentReference struct {
	InstrumentID string `json:"instrument_id,omitempty"` // internal stable id
	ProviderID   string `json:"provider_id,omitempty"`
	FIGI         string `json:"figi,omitempty"`
	ISIN         string `json:"isin,omitempty"`
	CUSIP        string `json:"cusip,omitempty"`
	Exchange     string `json:"exchange,omitempty"`
	MIC          string `json:"mic,omitempty"`
	Currency     string `json:"currency,omitempty"`
	Symbol       string `json:"symbol"`
}

// CorporateActionRequest is the neutral provider fetch request.
type CorporateActionRequest struct {
	Instruments []InstrumentReference
	Since       time.Time
	Until       time.Time
}

// ProviderCorporateAction is a raw event as returned by a provider adapter,
// before normalization.
type ProviderCorporateAction struct {
	ProviderEventID  string
	Type             Type
	Source           InstrumentReference
	Target           *InstrumentReference
	AnnouncementAt   *time.Time
	EffectiveAt      time.Time
	RecordAt         *time.Time
	PayableAt        *time.Time
	RatioNumerator   *float64
	RatioDenominator *float64
	CashPerShare     *float64
	CashCurrency     *string
	ParentBasisPct   *float64
	ChildBasisPct    *float64
	Cancelled        bool
	Effective        bool // provider has confirmed the action is in effect
	SourceURL        string
	RetrievedAt      time.Time
}

// CorporateAction is the normalized, stored event. The portfolio service consumes
// only this shape.
type CorporateAction struct {
	ID              string
	Provider        string
	ProviderEventID string
	Type            Type
	Source          InstrumentReference
	Target          *InstrumentReference

	AnnouncementAt *time.Time
	EffectiveAt    time.Time
	RecordAt       *time.Time
	PayableAt      *time.Time

	RatioNumerator   *float64
	RatioDenominator *float64
	CashPerShare     *float64
	CashCurrency     *string
	ParentBasisPct   *float64
	ChildBasisPct    *float64

	Status  Status
	Quality Quality

	SourceURL      string
	RawFingerprint string
	RetrievedAt    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ApplicationStatus tracks a single (event, portfolio) application.
type ApplicationStatus string

const (
	ApplicationPending  ApplicationStatus = "pending"
	ApplicationApplying ApplicationStatus = "applying"
	ApplicationApplied  ApplicationStatus = "applied"
	ApplicationFailed   ApplicationStatus = "failed"
	ApplicationSkipped  ApplicationStatus = "skipped"
)

// Application is the durable, idempotent record that an event was (or is being)
// applied to one portfolio.
type Application struct {
	CorporateActionID        string
	PortfolioID              string
	UserID                   string
	Status                   ApplicationStatus
	PortfolioVersionBefore   int64
	PortfolioVersionAfter    int64
	PerformanceVersionBefore int64
	PerformanceVersionAfter  int64
	AppliedAt                *time.Time
	ErrorCode                string
	RetryCount               int
	NextRetryAt              *time.Time
	CreatedAt                time.Time
	UpdatedAt                time.Time
}
