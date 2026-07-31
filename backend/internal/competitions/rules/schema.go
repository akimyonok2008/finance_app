// Package rules is the typed, versioned rule engine behind Arena
// competitions. Eligibility ("who may enter?") and scoring ("which part of
// the portfolio is measured?") are two INDEPENDENT documents: the same
// eligibility rule set can pair with full-portfolio or matching-assets
// scoring purely by configuration.
//
// This is deliberately NOT a scripting engine. It supports a closed set of
// metrics, operators and filter dimensions, validated strictly (unknown JSON
// fields are rejected) so a definition version that parses today parses
// identically forever. Every numeric threshold is an exact decimal string
// ("0.30") parsed through the money domain — never a float. There are no
// competition-specific branches anywhere: "crypto", "ETF" or "XIST"
// competitions differ only in the data inside these documents.
package rules

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// SchemaVersion is the rule-document schema this engine interprets. Stored
// documents carry their version so future schema changes can coexist with
// historical snapshots.
const SchemaVersion = 1

// Metrics (closed set).
const (
	MetricPortfolioWeight       = "portfolio_weight"        // weight of filter-matching positions in total portfolio value
	MetricPositionCount         = "position_count"          // number of (optionally filter-matching) positions
	MetricAssetClassCount       = "asset_class_count"       // distinct asset types held
	MetricLargestPositionWeight = "largest_position_weight" // concentration of the single largest position
	MetricPortfolioAgeDays      = "portfolio_age_days"      // days since ranked tracking started
	MetricCashWeight            = "cash_weight"             // cash share of total portfolio value
)

// Operators (closed set).
const (
	OpGTE     = "gte"
	OpGT      = "gt"
	OpLTE     = "lte"
	OpLT      = "lt"
	OpEQ      = "eq"
	OpBetween = "between" // inclusive [value, value_high]
)

// Scoring scopes (closed set).
const (
	ScopeFullPortfolio  = "full_portfolio"
	ScopeMatchingAssets = "matching_assets"
	ScopeCustomUniverse = "custom_universe"
)

// Filter selects positions by STABLE instrument metadata — never by symbol
// patterns. A position matches when it satisfies every populated dimension
// (dimensions AND together; values within one dimension OR together).
// Universe names a versioned instrument-ID universe resolved by the caller.
type Filter struct {
	AssetTypes       []string `json:"asset_types,omitempty"`
	Sectors          []string `json:"sectors,omitempty"`
	VenueMICs        []string `json:"venue_mics,omitempty"`
	IssuerCountries  []string `json:"issuer_countries,omitempty"`
	ListingCountries []string `json:"listing_countries,omitempty"`
	Currencies       []string `json:"currencies,omitempty"`
	InstrumentIDs    []string `json:"instrument_ids,omitempty"`
	Universe         string   `json:"universe,omitempty"`
}

// IsZero reports whether no dimension is populated.
func (f *Filter) IsZero() bool {
	return f == nil || (len(f.AssetTypes) == 0 && len(f.Sectors) == 0 &&
		len(f.VenueMICs) == 0 && len(f.IssuerCountries) == 0 &&
		len(f.ListingCountries) == 0 && len(f.Currencies) == 0 &&
		len(f.InstrumentIDs) == 0 && f.Universe == "")
}

// Rule is one eligibility check. Value (and ValueHigh for between) are exact
// decimal strings; weight metrics use fractions ("0.30" = 30%).
type Rule struct {
	Code      string  `json:"code"`
	Label     string  `json:"label"`
	Metric    string  `json:"metric"`
	Filter    *Filter `json:"filter,omitempty"`
	Operator  string  `json:"operator"`
	Value     string  `json:"value"`
	ValueHigh string  `json:"value_high,omitempty"`
}

// Eligibility is the validated eligibility document: every rule in All must
// pass, and — when the Any group is present — at least one rule in Any must
// pass. Nested groups are intentionally unsupported in schema version 1;
// composition stays trivially auditable.
type Eligibility struct {
	SchemaVersion int    `json:"schema_version"`
	All           []Rule `json:"all,omitempty"`
	Any           []Rule `json:"any,omitempty"`
}

// Scoring is the validated scoring document. Scope decides which frozen
// positions are valued; IncludeCash decides whether frozen cash joins the
// scored composition. Version 1 is fixed-basket price return; deposits and
// withdrawals are structurally neutralized because scoring reprices the
// frozen join-time composition rather than following live flows.
type Scoring struct {
	SchemaVersion int     `json:"schema_version"`
	Scope         string  `json:"scope"`
	Filter        *Filter `json:"filter,omitempty"`
	IncludeCash   bool    `json:"include_cash"`

	// These fields are retained for schema compatibility. Version 1 implements
	// fixed-basket PRICE return, not total return: dividend and fee inclusion
	// are rejected when true. IncludeFX defaults to true; false freezes each
	// component's baseline FX contribution and measures local-price movement.
	IncludeDividends *bool `json:"include_dividends,omitempty"`
	IncludeFees      *bool `json:"include_fees,omitempty"`
	IncludeFX        *bool `json:"include_fx,omitempty"`

	// LegacyJoinTimeBaseline marks the pre-engine weekly-sprint semantics
	// (baseline at join time instead of the common start). Only the migrated
	// legacy definition carries it; the engine never sets it on new
	// definitions.
	LegacyJoinTimeBaseline bool `json:"legacy_join_time_baseline,omitempty"`
}

var validMetrics = map[string]bool{
	MetricPortfolioWeight: true, MetricPositionCount: true, MetricAssetClassCount: true,
	MetricLargestPositionWeight: true, MetricPortfolioAgeDays: true, MetricCashWeight: true,
}

var validOperators = map[string]bool{
	OpGTE: true, OpGT: true, OpLTE: true, OpLT: true, OpEQ: true, OpBetween: true,
}

// metricsAcceptingFilter lists which metrics a filter may (or must) accompany.
var metricsAcceptingFilter = map[string]bool{
	MetricPortfolioWeight: true, // filter (or universe) is REQUIRED: unfiltered weight is always 1
	MetricPositionCount:   true, // optional: count matching positions
}

func strictUnmarshal(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	// Trailing content after the document is malformed input, not config.
	if dec.More() {
		return fmt.Errorf("unexpected trailing content")
	}
	return nil
}

// ParseEligibility strictly decodes and validates an eligibility document.
func ParseEligibility(raw []byte) (Eligibility, error) {
	var e Eligibility
	if err := strictUnmarshal(raw, &e); err != nil {
		return Eligibility{}, fmt.Errorf("eligibility: %w", err)
	}
	if e.SchemaVersion != SchemaVersion {
		return Eligibility{}, fmt.Errorf("eligibility: unsupported schema_version %d", e.SchemaVersion)
	}
	if len(e.All)+len(e.Any) == 0 {
		return Eligibility{}, fmt.Errorf("eligibility: at least one rule is required")
	}
	seen := map[string]bool{}
	for _, group := range [][]Rule{e.All, e.Any} {
		for i := range group {
			if err := validateRule(group[i]); err != nil {
				return Eligibility{}, fmt.Errorf("eligibility rule %q: %w", group[i].Code, err)
			}
			if seen[group[i].Code] {
				return Eligibility{}, fmt.Errorf("eligibility: duplicate rule code %q", group[i].Code)
			}
			seen[group[i].Code] = true
		}
	}
	return e, nil
}

func validateRule(r Rule) error {
	if r.Code == "" || r.Label == "" {
		return fmt.Errorf("code and label are required")
	}
	if !validMetrics[r.Metric] {
		return fmt.Errorf("unknown metric %q", r.Metric)
	}
	if !validOperators[r.Operator] {
		return fmt.Errorf("unknown operator %q", r.Operator)
	}
	value, err := money.ParseRatio(r.Value)
	if err != nil {
		return fmt.Errorf("value %q is not an exact decimal: %w", r.Value, err)
	}
	if value.Sign() < 0 {
		return fmt.Errorf("value must not be negative")
	}
	if r.Operator == OpBetween {
		high, err := money.ParseRatio(r.ValueHigh)
		if err != nil {
			return fmt.Errorf("value_high %q is not an exact decimal: %w", r.ValueHigh, err)
		}
		if high.Cmp(value) <= 0 {
			return fmt.Errorf("value_high must be greater than value")
		}
	} else if r.ValueHigh != "" {
		return fmt.Errorf("value_high is only valid with the between operator")
	}
	if err := validateFilter(r.Filter); err != nil {
		return err
	}
	if !r.Filter.IsZero() && !metricsAcceptingFilter[r.Metric] {
		return fmt.Errorf("metric %q does not accept a filter", r.Metric)
	}
	if r.Metric == MetricPortfolioWeight && r.Filter.IsZero() {
		return fmt.Errorf("portfolio_weight requires a filter (unfiltered weight is always 1)")
	}
	return nil
}

func validateFilter(f *Filter) error {
	if f == nil {
		return nil
	}
	for _, dim := range [][]string{
		f.AssetTypes, f.Sectors, f.VenueMICs, f.IssuerCountries,
		f.ListingCountries, f.Currencies, f.InstrumentIDs,
	} {
		for _, v := range dim {
			if v == "" {
				return fmt.Errorf("filter contains an empty value")
			}
		}
	}
	return nil
}

// ParseScoring strictly decodes and validates a scoring document.
func ParseScoring(raw []byte) (Scoring, error) {
	var s Scoring
	if err := strictUnmarshal(raw, &s); err != nil {
		return Scoring{}, fmt.Errorf("scoring: %w", err)
	}
	if s.SchemaVersion != SchemaVersion {
		return Scoring{}, fmt.Errorf("scoring: unsupported schema_version %d", s.SchemaVersion)
	}
	if err := validateFilter(s.Filter); err != nil {
		return Scoring{}, fmt.Errorf("scoring: %w", err)
	}
	if s.IncludeDividends != nil && *s.IncludeDividends {
		return Scoring{}, fmt.Errorf("scoring: include_dividends=true is unsupported by fixed-basket price return")
	}
	if s.IncludeFees != nil && *s.IncludeFees {
		return Scoring{}, fmt.Errorf("scoring: include_fees=true is unsupported by fixed-basket price return")
	}
	switch s.Scope {
	case ScopeFullPortfolio:
		if !s.Filter.IsZero() {
			return Scoring{}, fmt.Errorf("scoring: full_portfolio must not carry a filter")
		}
	case ScopeMatchingAssets:
		if s.Filter.IsZero() {
			return Scoring{}, fmt.Errorf("scoring: matching_assets requires a filter")
		}
	case ScopeCustomUniverse:
		if s.Filter == nil || s.Filter.Universe == "" {
			return Scoring{}, fmt.Errorf("scoring: custom_universe requires filter.universe")
		}
	default:
		return Scoring{}, fmt.Errorf("scoring: unknown scope %q", s.Scope)
	}
	return s, nil
}

// ValidateDefinitionVersionPayloads is the admission check for creating a
// definition version: both documents must parse and validate. It returns the
// parsed forms so callers can inspect them without re-parsing.
func ValidateDefinitionVersionPayloads(eligibilityJSON, scoringJSON []byte) (Eligibility, Scoring, error) {
	e, err := ParseEligibility(eligibilityJSON)
	if err != nil {
		return Eligibility{}, Scoring{}, err
	}
	s, err := ParseScoring(scoringJSON)
	if err != nil {
		return Eligibility{}, Scoring{}, err
	}
	return e, s, nil
}

// Capabilities describes which filter dimensions this deployment can
// actually evaluate. Schema validation only checks that a document is
// syntactically well-formed; a filter dimension can be syntactically valid
// and still be backed by no real classification data or resolver, in which
// case it can never match a position. ValidateCapabilities is the second,
// deployment-aware admission gate that catches that case.
type Capabilities struct {
	// SectorSupported reports whether classification enrichment populates
	// PositionFacts.Sector for live portfolios.
	SectorSupported bool
	// IssuerCountrySupported reports whether classification enrichment
	// populates PositionFacts.IssuerCountry for live portfolios.
	IssuerCountrySupported bool
	// UniverseResolverWired reports whether a UniverseResolver is attached,
	// so filter.universe references can actually be resolved.
	UniverseResolverWired bool
}

// ValidateCapabilities rejects eligibility or scoring filters that reference
// a dimension this deployment cannot evaluate. A rule document must not be
// considered valid merely because its JSON is syntactically valid: an
// administrator publishing a filter on a dimension with no backing data
// would create a competition whose rules can never match anyone.
func ValidateCapabilities(e Eligibility, s Scoring, caps Capabilities) error {
	for _, group := range [][]Rule{e.All, e.Any} {
		for _, r := range group {
			if err := validateFilterCapabilities(r.Filter, caps); err != nil {
				return fmt.Errorf("eligibility rule %q: %w", r.Code, err)
			}
		}
	}
	if err := validateFilterCapabilities(s.Filter, caps); err != nil {
		return fmt.Errorf("scoring: %w", err)
	}
	return nil
}

func validateFilterCapabilities(f *Filter, caps Capabilities) error {
	if f == nil {
		return nil
	}
	if len(f.Sectors) > 0 && !caps.SectorSupported {
		return fmt.Errorf("sector filter is not supported by this deployment (no sector classification data)")
	}
	if len(f.IssuerCountries) > 0 && !caps.IssuerCountrySupported {
		return fmt.Errorf("issuer_country filter is not supported by this deployment (no issuer classification data)")
	}
	if f.Universe != "" && !caps.UniverseResolverWired {
		return fmt.Errorf("custom universe %q is not supported: no universe resolver is configured", f.Universe)
	}
	return nil
}
