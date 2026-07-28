// Package benchmark implements deterministic virtual-portfolio benchmark
// indexes and the rules for benchmark achievement badges.
//
// The engine is a faithful Go port of the reference TypeScript design. It is
// modular: models, catalogue, market-data ports, benchmark construction, and a
// rules engine. Benchmark construction depends only on a HistoricalPriceProvider
// port, so the data source (a licensed feed, a cache, or the bundled mock) is
// swappable without touching the math.
package benchmark

import "time"

// Difficulty tiers order badges from onboarding-easy to elite.
type Difficulty string

const (
	DifficultyEasy   Difficulty = "easy"
	DifficultyMedium Difficulty = "medium"
	DifficultyHard   Difficulty = "hard"
	DifficultyElite  Difficulty = "elite"
)

// PeriodCode is the lookback window a badge is evaluated over.
type PeriodCode string

const (
	Period30D PeriodCode = "30D"
	Period90D PeriodCode = "90D"
	Period6M  PeriodCode = "6M"
	Period1Y  PeriodCode = "1Y"
)

// RuleKind selects which evaluator decides whether a badge unlocks.
type RuleKind string

const (
	RulePositiveAndBeat        RuleKind = "positive_and_beat"
	RuleBeatByPoints           RuleKind = "beat_by_points"
	RuleBeatByPointsAndPositiv RuleKind = "beat_by_points_and_positive"
)

// AssetAllocation is one leg of a benchmark recipe. Exactly one of Symbol or
// RecipeRef must be set: Symbol names a tradable ticker; RecipeRef nests another
// recipe (e.g. a commodity basket inside All-Weather).
type AssetAllocation struct {
	Symbol    string  `json:"symbol,omitempty"`
	RecipeRef string  `json:"recipe_ref,omitempty"`
	Weight    float64 `json:"weight"` // target weight as a decimal, 0.60 = 60%
}

// BenchmarkRecipe is a named target allocation. Dynamic recipes (e.g. a 13F
// basket) are resolved through a DynamicRecipeResolver before use.
type BenchmarkRecipe struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Components  []AssetAllocation `json:"components"`
	Dynamic     bool              `json:"dynamic,omitempty"`
}

// UnlockRule describes the outperformance criteria for a badge.
type UnlockRule struct {
	Kind                   RuleKind `json:"kind"`
	RequiredEdgePoints     float64  `json:"required_edge_points"`
	RequiresPositiveReturn bool     `json:"requires_positive_return"`
}

// Badge is an unlockable achievement tied to a benchmark recipe and a rule.
type Badge struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Difficulty  Difficulty `json:"difficulty"`
	Period      PeriodCode `json:"period"`
	Description string     `json:"description"`
	InspiredBy  string     `json:"inspired_by,omitempty"`
	RecipeID    string     `json:"recipe_id"`
	Rule        UnlockRule `json:"rule"`
}

// AchievementEvidence captures why a badge unlocked. It is privacy-safe: it
// carries only percentages, dates, and the benchmark id — never monetary
// values, holdings, or identifiers.
type AchievementEvidence struct {
	Period              PeriodCode `json:"period"`
	StartDate           string     `json:"start_date"`
	EndDate             string     `json:"end_date"`
	PortfolioReturnPct  float64    `json:"portfolio_return_pct"`
	BenchmarkReturnPct  float64    `json:"benchmark_return_pct"`
	EdgePoints          float64    `json:"edge_points"`
	BenchmarkRecipeID   string     `json:"benchmark_recipe_id"`
	EvaluationModel     string     `json:"evaluation_model,omitempty"`
	EvidenceVersion     int        `json:"evidence_version,omitempty"`
	TrackingEpoch       string     `json:"tracking_epoch,omitempty"`
	StartRankedIndex    float64    `json:"start_ranked_index,omitempty"`
	EndRankedIndex      float64    `json:"end_ranked_index,omitempty"`
	StartSnapshotAt     string     `json:"start_snapshot_at,omitempty"`
	EndSnapshotAt       string     `json:"end_snapshot_at,omitempty"`
	ActiveCoveragePct   float64    `json:"active_coverage_percentage,omitempty"`
	TrustedCoveragePct  float64    `json:"trusted_data_coverage_percentage,omitempty"`
	BenchmarkDataSource string     `json:"benchmark_data_source,omitempty"`
	SnapshotFrequency   string     `json:"snapshot_frequency,omitempty"`

	// Benchmark data-integrity provenance (evidence v2). These record exactly
	// which recipe version, price methodology, data quality and verification
	// level produced the award, so it is reproducible and audit-safe.
	BenchmarkRecipeVersion string                 `json:"benchmark_recipe_version,omitempty"`
	Verification           AwardVerification      `json:"verification,omitempty"`
	RebalancingPolicy      RebalancingPolicy      `json:"rebalancing_policy,omitempty"`
	BenchmarkInputHash     string                 `json:"benchmark_input_hash,omitempty"`
	DataSourceSummary      *BenchmarkDataEvidence `json:"benchmark_data,omitempty"`
	BenchmarkMethodology   string                 `json:"benchmark_methodology,omitempty"`
	AlignmentMethod        string                 `json:"alignment_method,omitempty"`
	BaseCurrency           string                 `json:"base_currency,omitempty"`
	CurrencyTreatment      string                 `json:"currency_treatment,omitempty"`
	RebalanceDates         []string               `json:"rebalance_dates,omitempty"`
}

// BenchmarkDataEvidence is the privacy-safe provenance of the benchmark leg of
// an award: which providers, price methodology, data quality, and — for dynamic
// 13F recipes — the filing source and mapping coverage. It contains no monetary
// values, holdings, or identifiers.
type BenchmarkDataEvidence struct {
	Providers             []string   `json:"providers,omitempty"`
	Symbols               []string   `json:"symbols,omitempty"`
	PriceType             string     `json:"price_type,omitempty"`
	Quality               string     `json:"quality,omitempty"`
	IsSynthetic           bool       `json:"is_synthetic"`
	UsedStaleData         bool       `json:"used_stale_data"`
	IncludesDividends     bool       `json:"includes_dividends"`
	IncludesSplits        bool       `json:"includes_splits"`
	TotalReturnMethod     string     `json:"total_return_method,omitempty"`
	RetrievedAt           time.Time  `json:"retrieved_at,omitempty"`
	RecipeSourceType      string     `json:"recipe_source_type,omitempty"`
	RecipeSourceAccession string     `json:"recipe_source_accession,omitempty"`
	RecipeSourceURL       string     `json:"recipe_source_url,omitempty"`
	RecipeReportPeriodEnd *time.Time `json:"recipe_report_period_end,omitempty"`
	RecipePubliclyKnownAt *time.Time `json:"recipe_publicly_known_at,omitempty"`
	MappingCoveragePct    *float64   `json:"mapping_coverage_percentage,omitempty"`
	MethodologyVersion    string     `json:"methodology_version,omitempty"`
	BaseCurrency          string     `json:"base_currency,omitempty"`
	CurrencyTreatment     string     `json:"currency_treatment,omitempty"`
	FXProvider            string     `json:"fx_provider,omitempty"`
}

// EvidenceFromResult projects a benchmark evaluation's provenance into the
// privacy-safe evidence summary.
func EvidenceFromResult(result BenchmarkReturnResult) BenchmarkDataEvidence {
	m := result.DataMetadata
	priceType := ""
	if len(m.PriceTypes) > 0 {
		priceType = string(m.PriceTypes[0])
	}
	trMethod := "none"
	if m.AllSeriesTotalReturn {
		trMethod = "total_return"
	} else if m.AllSeriesAdjusted {
		trMethod = "adjusted_close"
	}
	ev := BenchmarkDataEvidence{
		Providers:          m.Providers,
		Symbols:            m.Symbols,
		PriceType:          priceType,
		Quality:            string(m.Quality),
		IsSynthetic:        m.IsSynthetic,
		UsedStaleData:      m.UsedStaleData,
		IncludesDividends:  m.IncludesDividends,
		IncludesSplits:     m.IncludesSplits,
		TotalReturnMethod:  trMethod,
		RetrievedAt:        m.EvaluatedAt,
		RecipeSourceType:   result.RecipeVersion.SourceType,
		RecipeSourceURL:    result.RecipeVersion.SourceURL,
		MappingCoveragePct: result.RecipeVersion.MappingCoverage,
		MethodologyVersion: result.DataMetadata.MethodologyVersion,
		BaseCurrency:       result.DataMetadata.BaseCurrency,
		CurrencyTreatment:  result.DataMetadata.CurrencyTreatment,
		FXProvider:         result.DataMetadata.FXProvider,
	}
	if result.RecipeVersion.SourceAccession != "" {
		ev.RecipeSourceAccession = result.RecipeVersion.SourceAccession
	}
	ev.RecipeReportPeriodEnd = result.RecipeVersion.ReportPeriodEnd
	ev.RecipePubliclyKnownAt = result.RecipeVersion.PubliclyKnownAt
	return ev
}

// PricePoint is a single benchmark price observation. Raw provider closes are
// kept separate so they can never be silently blessed as adjusted data.
type PricePoint struct {
	Date          string  `json:"date"` // YYYY-MM-DD
	AdjustedClose float64 `json:"adjusted_close"`
	RawClose      float64 `json:"raw_close,omitempty"`
}

// IndexPoint is a generic normalized index point. Ranked achievements do not
// use this legacy archive-series type.
type IndexPoint struct {
	Date  string  `json:"date"`
	Index float64 `json:"index"`
}

const MethodologyVirtualPortfolioV2 = "benchmark_virtual_portfolio_v2"

type CurrencyTreatment string

const CurrencyTreatmentHistoricalSpot CurrencyTreatment = "historical_spot_unhedged"

type BenchmarkEvaluationRequest struct {
	RecipeID          string
	Start             time.Time
	End               time.Time
	BaseCurrency      string
	CurrencyTreatment CurrencyTreatment
	SeriesRequirement SeriesRequirement
}

// BenchmarkPortfolioState is the scale-independent virtual account carried
// through each common close. Units only change when the executed policy says
// to rebalance.
type BenchmarkPortfolioState struct {
	NAV               float64
	Cash              float64
	Holdings          map[string]float64
	RecipeID          string
	RecipeVersionID   string
	RebalancingPolicy RebalancingPolicy
	LastValuationDate time.Time
	LastRebalanceDate *time.Time
}

// EvaluationContext is the input a rule evaluator scores.
type EvaluationContext struct {
	Badge              Badge
	StartDate          string
	EndDate            string
	PortfolioReturnPct float64
	BenchmarkReturnPct float64
}

// EvaluationResult is the outcome of evaluating a rule. Evidence is set only
// when Unlocked is true.
type EvaluationResult struct {
	Unlocked bool
	Reason   string
	Evidence *AchievementEvidence
}
