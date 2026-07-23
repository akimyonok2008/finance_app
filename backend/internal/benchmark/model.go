// Package benchmark implements a deterministic engine for "beat-the-benchmark"
// achievement badges. Given a set of investor-inspired benchmark recipes and
// unlock rules, it constructs a daily-rebalanced benchmark index return over a
// period and compares it to a portfolio's return.
//
// The engine is a faithful Go port of the reference TypeScript design. It is
// modular: models, catalogue, market-data ports, benchmark construction, and a
// rules engine. Benchmark construction depends only on a HistoricalPriceProvider
// port, so the data source (a licensed feed, a cache, or the bundled mock) is
// swappable without touching the math.
package benchmark

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
	Period             PeriodCode `json:"period"`
	StartDate          string     `json:"start_date"`
	EndDate            string     `json:"end_date"`
	PortfolioReturnPct float64    `json:"portfolio_return_pct"`
	BenchmarkReturnPct float64    `json:"benchmark_return_pct"`
	EdgePoints         float64    `json:"edge_points"`
	BenchmarkRecipeID  string     `json:"benchmark_recipe_id"`
}

// PricePoint is a single adjusted-close observation. Adjusted closes are
// required so splits and dividends do not distort benchmark returns.
type PricePoint struct {
	Date          string  `json:"date"` // YYYY-MM-DD
	AdjustedClose float64 `json:"adjusted_close"`
}

// IndexPoint is a single point on a normalized index series (e.g. a portfolio
// TWR index).
type IndexPoint struct {
	Date  string  `json:"date"`
	Index float64 `json:"index"`
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
