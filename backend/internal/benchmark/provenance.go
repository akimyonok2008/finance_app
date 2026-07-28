package benchmark

import (
	"errors"
	"time"
)

// PriceType records what kind of price series a leg was built from. This is the
// difference between a raw price-return series and a total-return series, and it
// is decisive for whether a benchmark may back a verified permanent award.
type PriceType string

const (
	// PriceTypeRawClose is an unadjusted closing price. Splits and dividends
	// distort raw-close returns, so it is NOT admissible for verified awards.
	PriceTypeRawClose PriceType = "raw_close"
	// PriceTypeAdjustedClose is a split/dividend-adjusted close (total-return
	// equivalent for a reinvesting holder).
	PriceTypeAdjustedClose PriceType = "adjusted_close"
	// PriceTypeTotalReturn is an explicit total-return index (price + reinvested
	// distributions).
	PriceTypeTotalReturn PriceType = "total_return"
)

// IsTotalReturnEquivalent reports whether a price type captures reinvested
// distributions, i.e. whether returns computed from it are total returns.
func (p PriceType) IsTotalReturnEquivalent() bool {
	return p == PriceTypeAdjustedClose || p == PriceTypeTotalReturn
}

// DataQuality is the trust level of a price series or an aggregate benchmark.
// Only DataQualityVerified may back a verified permanent award.
type DataQuality string

const (
	DataQualityVerified   DataQuality = "verified"   // real, adjusted/total-return, fresh, complete
	DataQualityAcceptable DataQuality = "acceptable" // real but raw-close / not total-return capable
	DataQualitySynthetic  DataQuality = "synthetic"  // mock / generated data
	DataQualityStale      DataQuality = "stale"      // real but past the freshness bound
	DataQualityIncomplete DataQuality = "incomplete" // insufficient coverage / points
	DataQualityInvalid    DataQuality = "invalid"    // failed structural validation
)

// qualityRank orders qualities from worst (0) to best. Aggregation keeps the
// weakest leg, so a single synthetic or raw leg cannot be laundered into a
// verified benchmark.
func qualityRank(q DataQuality) int {
	switch q {
	case DataQualityInvalid:
		return 0
	case DataQualityIncomplete:
		return 1
	case DataQualitySynthetic:
		return 2
	case DataQualityStale:
		return 3
	case DataQualityAcceptable:
		return 4
	case DataQualityVerified:
		return 5
	default:
		return 0
	}
}

// worseQuality returns whichever of two qualities is lower-trust.
func worseQuality(a, b DataQuality) DataQuality {
	if qualityRank(b) < qualityRank(a) {
		return b
	}
	return a
}

// AwardVerification is the trust classification stamped onto an awarded badge.
// Only AwardVerificationVerified counts as a permanent production achievement.
type AwardVerification string

const (
	AwardVerificationVerified   AwardVerification = "verified"
	AwardVerificationDemo       AwardVerification = "demo"
	AwardVerificationLegacy     AwardVerification = "legacy"
	AwardVerificationUnverified AwardVerification = "unverified"
)

// RebalancingPolicy selects the virtual-portfolio trading rule. Every declared
// value is implemented explicitly; unknown values fail closed.
type RebalancingPolicy string

const (
	RebalanceDailyTargetWeight RebalancingPolicy = "daily_target_weight"
	RebalanceBuyAndHold        RebalancingPolicy = "buy_and_hold"
	RebalancePeriodicMonthly   RebalancingPolicy = "periodic_monthly"
	RebalanceFilingSnapshot    RebalancingPolicy = "filing_snapshot_hold"
)

// BenchmarkDataMetadata is the provenance of a single symbol's price series.
// Metadata travels with the data so award policy is never decided by naming
// alone.
type BenchmarkDataMetadata struct {
	Provider          string      `json:"provider"`
	ProviderMode      string      `json:"provider_mode"` // e.g. "real", "mock"
	PriceType         PriceType   `json:"price_type"`
	IncludesDividends bool        `json:"includes_dividends"`
	IncludesSplits    bool        `json:"includes_splits"`
	IsAdjusted        bool        `json:"is_adjusted"`
	IsTotalReturn     bool        `json:"is_total_return"`
	IsSynthetic       bool        `json:"is_synthetic"`
	IsStale           bool        `json:"is_stale"`
	Quality           DataQuality `json:"quality"`
	// CorpActionsKnown is true only when the provider has told us, explicitly,
	// whether the series does or does not incorporate corporate actions.
	// Unknown handling fails closed for verified awards.
	CorpActionsKnown  bool      `json:"corp_actions_known"`
	RetrievedAt       time.Time `json:"retrieved_at"`
	SourceAsOf        time.Time `json:"source_as_of"`
	ProviderRequestID string    `json:"provider_request_id,omitempty"`
	ProviderDataset   string    `json:"provider_dataset,omitempty"`
	CurrencyTreatment string    `json:"currency_treatment,omitempty"`
	Currency          string    `json:"currency,omitempty"`
}

// BenchmarkPriceSeries couples a symbol's points with their provenance. The
// engine consumes these instead of bare []PricePoint so quality can never be
// silently lost.
type BenchmarkPriceSeries struct {
	Symbol   string                `json:"symbol"`
	Points   []PricePoint          `json:"points"`
	Metadata BenchmarkDataMetadata `json:"metadata"`
}

// SeriesRequirement expresses the data quality the caller needs. The engine
// requests total-return-capable data for award evaluation, and the provider is
// obliged to fail rather than silently downgrade.
type SeriesRequirement struct {
	RequireAdjusted    bool
	RequireTotalReturn bool
	AllowStale         bool
	AllowSynthetic     bool
}

// RequirementForAwards is the strict requirement used when evaluating whether a
// verified permanent award may be issued.
func RequirementForAwards() SeriesRequirement {
	return SeriesRequirement{RequireAdjusted: true, RequireTotalReturn: false, AllowStale: false, AllowSynthetic: false}
}

// RequirementForPreview is the lenient requirement used for progress previews,
// where synthetic/raw data is acceptable because no permanent award results.
func RequirementForPreview() SeriesRequirement {
	return SeriesRequirement{AllowStale: true, AllowSynthetic: true}
}

// RecipeVersionMetadata is the immutable identity of the recipe version used for
// an evaluation, enough to reproduce and to audit the source.
type RecipeVersionMetadata struct {
	RecipeID          string            `json:"recipe_id"`
	VersionID         string            `json:"version_id"`
	SourceType        string            `json:"source_type"`
	SourceURL         string            `json:"source_url,omitempty"`
	SourceAccession   string            `json:"source_accession,omitempty"`
	ReportPeriodEnd   *time.Time        `json:"report_period_end,omitempty"`
	PubliclyKnownAt   *time.Time        `json:"publicly_known_at,omitempty"`
	RebalancingPolicy RebalancingPolicy `json:"rebalancing_policy"`
	TotalReturnModel  bool              `json:"total_return_model"`
	InvestorProxy     bool              `json:"investor_proxy"`
	MappingCoverage   *float64          `json:"mapping_coverage_percentage,omitempty"`
}

// BenchmarkEvaluationMetadata summarizes the provenance of a complete,
// multi-leg benchmark evaluation. Quality is the weakest leg.
type BenchmarkEvaluationMetadata struct {
	Providers            []string                 `json:"providers"`
	Symbols              []string                 `json:"symbols"`
	PriceTypes           []PriceType              `json:"price_types"`
	Quality              DataQuality              `json:"quality"`
	IsSynthetic          bool                     `json:"is_synthetic"`
	UsedStaleData        bool                     `json:"used_stale_data"`
	IncludesDividends    bool                     `json:"includes_dividends"`
	IncludesSplits       bool                     `json:"includes_splits"`
	AllSeriesAdjusted    bool                     `json:"all_series_adjusted"`
	AllSeriesTotalReturn bool                     `json:"all_series_total_return"`
	CorpActionsKnown     bool                     `json:"corp_actions_known"`
	CurrencyTreatment    string                   `json:"currency_treatment,omitempty"`
	BaseCurrency         string                   `json:"base_currency,omitempty"`
	MethodologyVersion   string                   `json:"methodology_version,omitempty"`
	ActivatedVersions    []ActivatedRecipeVersion `json:"activated_recipe_versions,omitempty"`
	RebalanceDates       []string                 `json:"rebalance_dates,omitempty"`
	FXProvider           string                   `json:"fx_provider,omitempty"`
	FXPoints             []FXEvidencePoint        `json:"fx_points,omitempty"`
	EvaluatedAt          time.Time                `json:"evaluated_at"`
}

type ActivatedRecipeVersion struct {
	RecipeID        string            `json:"recipe_id"`
	VersionID       string            `json:"version_id"`
	ActivationDate  string            `json:"activation_date"`
	Policy          RebalancingPolicy `json:"policy"`
	Components      []AssetAllocation `json:"components"`
	PubliclyKnownAt string            `json:"publicly_known_at,omitempty"`
}

type FXEvidencePoint struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Date     string  `json:"date"`
	Rate     float64 `json:"rate"`
	Provider string  `json:"provider"`
}

// BenchmarkReturnResult is the full outcome of a benchmark evaluation: the
// number plus everything needed to trust, audit and reproduce it.
type BenchmarkReturnResult struct {
	ReturnPercentage float64                     `json:"return_percentage"`
	EffectiveStart   time.Time                   `json:"effective_start"`
	EffectiveEnd     time.Time                   `json:"effective_end"`
	RecipeVersion    RecipeVersionMetadata       `json:"recipe_version"`
	DataMetadata     BenchmarkEvaluationMetadata `json:"data_metadata"`
	Fingerprint      string                      `json:"fingerprint"`
	StartNAV         float64                     `json:"start_nav"`
	EndNAV           float64                     `json:"end_nav"`
	Points           []IndexPoint                `json:"points"`
}

// Typed benchmark-data-quality errors. The achievement layer distinguishes
// these so progress can explain the exact integrity failure instead of a
// generic "unavailable".
var (
	ErrAdjustedDataUnavailable      = errors.New("benchmark: adjusted/total-return data unavailable")
	ErrTotalReturnUnavailable       = errors.New("benchmark: total-return data unavailable")
	ErrSyntheticDataNotAllowed      = errors.New("benchmark: synthetic data not allowed for this evaluation")
	ErrStaleBenchmarkData           = errors.New("benchmark: benchmark data is stale")
	ErrIncompleteSeries             = errors.New("benchmark: incomplete price series")
	ErrInvalidBenchmarkSeries       = errors.New("benchmark: invalid price series")
	ErrRecipeVersionUnavailable     = errors.New("benchmark: no recipe version valid for the evaluated period")
	ErrRecipeMappingCoverage        = errors.New("benchmark: recipe mapping coverage below threshold")
	ErrCircularRecipe               = errors.New("benchmark: circular recipe reference")
	ErrUnsupportedRebalancingPolicy = errors.New("benchmark: unsupported rebalancing policy")
	ErrHistoricalFXUnavailable      = errors.New("benchmark: historical FX unavailable")
	ErrCurrencyTreatmentUnavailable = errors.New("benchmark: currency treatment unavailable")
	ErrBenchmarkDateAlignment       = errors.New("benchmark: benchmark and portfolio dates could not be aligned")
)
