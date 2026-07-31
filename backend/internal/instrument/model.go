// Package instrument holds the stable instrument-identity layer: an immutable
// internal identity (instrument_master) plus time-scoped aliases
// (instrument_aliases) that survive ticker changes, ticker reuse and
// cross-exchange symbol collisions.
//
// Integration status: this package is STANDALONE. Nothing in the portfolio
// buy path, income pipeline or corporate-action pipeline consumes it yet; those
// still identify instruments by ticker string. Wiring is a later slice.
package instrument

import (
	"errors"
	"time"
)

// AliasType enumerates the identifier namespaces an instrument can be known by.
// It mirrors the instrument_alias_type_valid CHECK constraint (migration 0020).
type AliasType string

const (
	AliasTicker         AliasType = "ticker"
	AliasFIGI           AliasType = "figi"
	AliasCompositeFIGI  AliasType = "composite_figi"
	AliasShareClassFIGI AliasType = "share_class_figi"
	AliasISIN           AliasType = "isin"
	AliasCUSIP          AliasType = "cusip"
	AliasCIK            AliasType = "cik"
	AliasProviderSymbol AliasType = "provider_symbol"
)

// Valid reports whether the alias type is one the schema accepts.
func (a AliasType) Valid() bool {
	switch a {
	case AliasTicker, AliasFIGI, AliasCompositeFIGI, AliasShareClassFIGI,
		AliasISIN, AliasCUSIP, AliasCIK, AliasProviderSymbol:
		return true
	}
	return false
}

// IdentityQuality records how much confidence the identity carries.
type IdentityQuality string

const (
	// QualityVerified is an identity confirmed against an authoritative source
	// beyond a single mapping call (reserved; nothing sets it in this slice).
	QualityVerified IdentityQuality = "verified"
	// QualityResolved is a single unambiguous provider match.
	QualityResolved IdentityQuality = "resolved"
	// QualityAmbiguous means the provider returned more than one candidate. No
	// instrument is created: picking arbitrarily would mint a wrong identity.
	QualityAmbiguous IdentityQuality = "ambiguous"
	// QualityUnresolved means no provider candidate was found.
	QualityUnresolved IdentityQuality = "unresolved"
)

// Instrument statuses.
const (
	StatusActive   = "active"
	StatusDelisted = "delisted"
)

// Instrument is the immutable identity register row. ID is the internal
// identity every other table should eventually point at; the provider
// identifiers (FIGI, ISIN, ...) may be filled in or corrected later without
// invalidating those references.
type Instrument struct {
	ID               string
	FIGI             string
	CompositeFIGI    string
	ShareClassFIGI   string
	ISIN             string
	CUSIP            string
	CIK              string
	CurrentSymbol    string
	Name             string
	SecurityType     string
	AssetType        string
	ExchangeCode     string
	MIC              string
	VenueID          string
	IssuerID         string
	Currency         string
	Country          string
	Sector           Sector
	Status           string
	ListedAt         *time.Time
	DelistedAt       *time.Time
	IdentityQuality  IdentityQuality
	IdentityProvider string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// InstrumentAlias is one identifier an instrument is (or was) known by, scoped
// to a validity window. ValidTo == nil means the alias is currently active.
type InstrumentAlias struct {
	ID              string
	InstrumentID    string
	AliasType       AliasType
	AliasValue      string
	ExchangeCode    string
	MIC             string
	VenueID         string
	ValidFrom       time.Time
	ValidTo         *time.Time
	Provider        string
	ProviderEventID string
	CreatedAt       time.Time
}

// Venue is a first-class exchange/listing entity, identified by MIC. It is
// the authoritative counterpart to the exchange_code/mic snapshot columns
// carried on Instrument and InstrumentAlias for cheap reads.
type Venue struct {
	ID           string
	MIC          string
	ExchangeCode string
	Name         string
	Country      string
	Currency     string
	CreatedAt    time.Time
}

// Issuer is a first-class company/entity that may have multiple listed
// instruments (share classes, dual listings). Linked from Instrument via
// IssuerID; resolved opportunistically from CIK when available.
type Issuer struct {
	ID        string
	Name      string
	CIK       string
	LEI       string
	Country   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Active reports whether the alias is currently in force.
func (a InstrumentAlias) Active() bool { return a.ValidTo == nil }

// ActiveAt reports whether the alias was in force at the given instant.
func (a InstrumentAlias) ActiveAt(t time.Time) bool {
	if t.Before(a.ValidFrom) {
		return false
	}
	return a.ValidTo == nil || t.Before(*a.ValidTo)
}

// Sentinel errors.
var (
	ErrInstrumentNotFound = errors.New("instrument: not found")
	ErrAliasNotFound      = errors.New("instrument: alias not found")
	// ErrAliasConflict is returned when an active alias already exists for the
	// same (type, value, exchange, mic) tuple.
	ErrAliasConflict  = errors.New("instrument: active alias already exists")
	ErrInvalidAlias   = errors.New("instrument: invalid alias")
	ErrAliasNotActive = errors.New("instrument: alias is already closed")
)
