// Package coach implements the AI Portfolio Coach: a structured, analysis-only
// portfolio intelligence feature. It computes deterministic facts about the
// authenticated user's portfolio and the public top-performer context, then
// hands those facts to a pluggable Provider (mock by default) which returns
// narrative analysis. The coach NEVER returns buy/sell advice and NEVER exposes
// another user's private financial values or identity.
package coach

import "time"

// Supported analysis modes for POST /portfolio/coach.
const (
	ModeAnalyzePortfolio   = "analyze_portfolio"
	ModeCompareTop10       = "compare_top10"
	ModeTechnicalSetup     = "technical_setup"
	ModeFundamentalContext = "fundamental_context"
)

// supportedModes is the allow-list used for request validation.
var supportedModes = map[string]bool{
	ModeAnalyzePortfolio:   true,
	ModeCompareTop10:       true,
	ModeTechnicalSetup:     true,
	ModeFundamentalContext: true,
}

// Disclaimer is appended to every coach response, unconditionally.
const Disclaimer = "Educational portfolio analysis only. Not financial advice."

// minBenchmarkPortfolios is the smallest number of OTHER portfolios required
// before top-10 comparison aggregates are considered meaningful. Below this we
// return available=false; between this and 10 we return a labelled, limited
// benchmark so local development with few users still works.
const minBenchmarkPortfolios = 1

// topPerformerLimit caps how many ranked portfolios form the public benchmark.
const topPerformerLimit = 10

// --- API DTOs ----------------------------------------------------------------

// CoachRequest is the request body for POST /portfolio/coach.
type CoachRequest struct {
	Mode string `json:"mode"`
}

// CoachObservation is a single labelled finding.
type CoachObservation struct {
	Label  string `json:"label"`
	Status string `json:"status"` // positive | neutral | watch | risk
	Text   string `json:"text"`
}

// CoachTop10Comparison is the deterministic comparison block. Numbers here are
// computed by the backend (never by the AI) so they cannot be hallucinated.
// When Available is false the numeric fields are zero and Notes explains why.
type CoachTop10Comparison struct {
	Available                          bool     `json:"available"`
	SampleSize                         int      `json:"sample_size"`
	Limited                            bool     `json:"limited"`
	ReturnGapPercentagePoints          float64  `json:"return_gap_percentage_points"`
	SharedSymbolsCount                 int      `json:"shared_symbols_count"`
	UserLargestWeightPercentage        float64  `json:"user_largest_weight_percentage"`
	Top10MedianLargestWeightPercentage float64  `json:"top10_median_largest_weight_percentage"`
	Notes                              []string `json:"notes"`
}

// CoachResponse is the structured response returned to the authenticated user.
type CoachResponse struct {
	Mode                string               `json:"mode"`
	Title               string               `json:"title"`
	Summary             string               `json:"summary"`
	RiskLevel           string               `json:"risk_level"` // low|moderate|elevated|high|unknown
	Observations        []CoachObservation   `json:"observations"`
	TechnicalNotes      []string             `json:"technical_notes"`
	FundamentalNotes    []string             `json:"fundamental_notes"`
	Top10Comparison     CoachTop10Comparison `json:"top10_comparison"`
	LearningPoints      []string             `json:"learning_points"`
	QuestionsToConsider []string             `json:"questions_to_consider"`
	Disclaimer          string               `json:"disclaimer"`
	GeneratedAt         time.Time            `json:"generated_at"`
}

// --- deterministic facts (provider input) ------------------------------------
//
// These structs are serialized into the provider prompt. They deliberately
// carry ONLY percentages, weights, and public identity — never quantities,
// values, cost basis, dollar gains, ids, or emails — so the whole provider
// input is free of private/forbidden fields.

// HoldingFact is one position reduced to safe, analysis-ready figures.
type HoldingFact struct {
	Symbol                       string  `json:"symbol"`
	AssetType                    string  `json:"asset_type"`
	WeightPercentage             float64 `json:"weight_percentage"`
	GainLossPercentage           float64 `json:"gain_loss_percentage"`
	ContributionPercentagePoints float64 `json:"contribution_percentage_points"`
}

// UserPortfolioFacts is the authenticated user's own analysis context. It uses
// only relative figures (weights, percentages) — no raw private values.
type UserPortfolioFacts struct {
	PortfolioIndex          float64            `json:"portfolio_index"`
	GainLossPercentage      float64            `json:"gain_loss_percentage"`
	PositionCount           int                `json:"position_count"`
	LargestSymbol           string             `json:"largest_symbol"`
	LargestWeightPercentage float64            `json:"largest_weight_percentage"`
	BaseCurrency            string             `json:"base_currency"`
	Holdings                []HoldingFact      `json:"holdings"`
	AssetTypeExposure       map[string]float64 `json:"asset_type_exposure"`
	CurrencyExposure        map[string]float64 `json:"currency_exposure"`
	RiskLevel               string             `json:"risk_level"`
}

// PublicHolding is a top performer's holding reduced to the only fields ever
// allowed in public competitive context: symbol, weight, asset type.
type PublicHolding struct {
	Symbol           string  `json:"symbol"`
	WeightPercentage float64 `json:"weight_percentage"`
	AssetType        string  `json:"asset_type"`
}

// PublicPortfolio is a top performer's public projection. It NEVER contains
// quantities, values, cost basis, dollar gains, user_id, portfolio_id, or email.
type PublicPortfolio struct {
	Rank             int             `json:"rank"`
	DisplayName      string          `json:"display_name"`
	AvatarKey        string          `json:"avatar_key"`
	ReturnPercentage float64         `json:"return_percentage"`
	PortfolioIndex   float64         `json:"portfolio_index"`
	Holdings         []PublicHolding `json:"holdings"`
}

// PublicTop10Facts is the sanitized public benchmark plus deterministic
// comparison aggregates against the requesting user.
type PublicTop10Facts struct {
	Available                     bool              `json:"available"`
	Limited                       bool              `json:"limited"`
	SampleSize                    int               `json:"sample_size"`
	Portfolios                    []PublicPortfolio `json:"portfolios"`
	MedianLargestWeightPercentage float64           `json:"median_largest_weight_percentage"`
	MedianPositionCount           float64           `json:"median_position_count"`
	SharedSymbolsCount            int               `json:"shared_symbols_count"`
	ReturnGapPercentagePoints     float64           `json:"return_gap_percentage_points"`
	UserLargestWeightPercentage   float64           `json:"user_largest_weight_percentage"`
}

// AchievementFact is a badge reduced to a name and unlock state.
type AchievementFact struct {
	Name     string `json:"name"`
	Unlocked bool   `json:"unlocked"`
}

// CoachProviderInput is the full, sanitized context handed to a Provider.
type CoachProviderInput struct {
	Mode               string             `json:"mode"`
	User               UserPortfolioFacts `json:"user"`
	Top10              PublicTop10Facts   `json:"top10"`
	Achievements       []AchievementFact  `json:"achievements"`
	DataLimitations    []string           `json:"data_limitations"`
	SafetyInstructions []string           `json:"safety_instructions"`
}

// CoachProviderOutput is the narrative a Provider returns. The backend supplies
// the authoritative numeric comparison separately, so providers cannot invent
// figures.
type CoachProviderOutput struct {
	Title               string             `json:"title"`
	Summary             string             `json:"summary"`
	RiskLevel           string             `json:"risk_level"`
	Observations        []CoachObservation `json:"observations"`
	TechnicalNotes      []string           `json:"technical_notes"`
	FundamentalNotes    []string           `json:"fundamental_notes"`
	LearningPoints      []string           `json:"learning_points"`
	QuestionsToConsider []string           `json:"questions_to_consider"`
}
