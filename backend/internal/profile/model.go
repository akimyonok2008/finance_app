package profile

import (
	"time"

	"github.com/ardakimyonok/finance_app/internal/dna"
)

const DefaultStrategyTag = "balanced_global"

type Profile struct {
	UserID            string
	Handle            string
	DisplayName       string
	AvatarKey         string
	Bio               string
	StrategyTag       string
	IsPublic          bool
	ShowPublicWeights bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UpdateInput struct {
	Handle            *string `json:"handle"`
	DisplayName       *string `json:"display_name"`
	AvatarKey         *string `json:"avatar_key"`
	Bio               *string `json:"bio"`
	StrategyTag       *string `json:"strategy_tag"`
	IsPublic          *bool   `json:"is_public"`
	ShowPublicWeights *bool   `json:"show_public_weights"`
}

type PublicBadge struct {
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Icon       string     `json:"icon"`
	UnlockedAt *time.Time `json:"unlocked_at,omitempty"`
}

type PublicWeight struct {
	Symbol    string  `json:"symbol"`
	AssetType string  `json:"asset_type"`
	Weight    float64 `json:"weight"`
}

type PublicPerformancePoint struct {
	CapturedAt       string  `json:"captured_at"`
	PortfolioIndex   float64 `json:"portfolio_index"`
	ReturnPercentage float64 `json:"return_percentage"`
}

type PublicClosedPosition struct {
	Symbol           string  `json:"symbol"`
	AssetType        string  `json:"asset_type"`
	ClosedAt         string  `json:"closed_at"`
	ReturnPercentage float64 `json:"return_percentage"`
}

// StrategyWeight is the privacy-safe, reusable representation of a public
// strategy allocation. It intentionally carries only symbol, asset type, and
// percentage weight.
type StrategyWeight struct {
	Symbol           string  `json:"symbol"`
	AssetType        string  `json:"asset_type"`
	WeightPercentage float64 `json:"weight_percentage"`
}

type Exposure struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

type Concentration struct {
	LargestPosition float64 `json:"largest_position"`
	TopThree        float64 `json:"top_three"`
}

type ProfilePerformanceDrivers struct {
	Summary                  string   `json:"summary"`
	PositiveDrivers          []string `json:"positive_drivers"`
	NegativeDrivers          []string `json:"negative_drivers"`
	OpenContributionPoints   float64  `json:"open_contribution_points"`
	ClosedContributionPoints float64  `json:"closed_contribution_points"`
}

type ProfileBenchmark struct {
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name"`
	Index      float64 `json:"index"`
	EdgePoints float64 `json:"edge_points"`
}

type ProfileBenchmarkContext struct {
	InvestorIndex float64            `json:"investor_index"`
	Benchmarks    []ProfileBenchmark `json:"benchmarks"`
	Note          string             `json:"note,omitempty"`
}

type ProfileContributor struct {
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name,omitempty"`
	ContributionPoints float64 `json:"contribution_points"`
}

type ProfileOpenClosedPerformance struct {
	OpenReturnPercentage     float64 `json:"open_return_percentage"`
	ClosedReturnPercentage   float64 `json:"closed_return_percentage"`
	OpenContributionPoints   float64 `json:"open_contribution_points"`
	ClosedContributionPoints float64 `json:"closed_contribution_points"`
	HasClosedPositions       bool    `json:"has_closed_positions"`
	CompositionVisible       bool    `json:"composition_visible"`
	// IncludesSelfReportedPrices is true when OpenReturnPercentage or
	// ClosedReturnPercentage above is built from at least one user-entered
	// execution price rather than a provider estimate. Unlike the top-level
	// PortfolioIndex/ReturnPercentage (the ranked index, always priced from
	// tracked market quotes and immune to this), these two figures are
	// unverifiable whenever this is true — a viewer should not read them as
	// equally trustworthy.
	IncludesSelfReportedPrices bool `json:"includes_self_reported_prices"`
}

type ProfileInsights struct {
	InvestmentStyle       string                       `json:"investment_style"`
	StyleSummary          string                       `json:"style_summary"`
	FocusAreas            []string                     `json:"focus_areas"`
	DNA                   dna.PortfolioDNAScores       `json:"dna"`
	Explanations          dna.DNAExplanations          `json:"dna_explanations"`
	PerformanceDrivers    ProfilePerformanceDrivers    `json:"performance_drivers"`
	BenchmarkContext      ProfileBenchmarkContext      `json:"benchmark_context"`
	Contributors          []ProfileContributor         `json:"contributors"`
	Detractors            []ProfileContributor         `json:"detractors"`
	OpenClosedPerformance ProfileOpenClosedPerformance `json:"open_closed_performance"`
}

type PublicProfile struct {
	Handle                string                   `json:"handle"`
	DisplayName           string                   `json:"display_name"`
	AvatarKey             string                   `json:"avatar_key"`
	Bio                   string                   `json:"bio"`
	StrategyTag           string                   `json:"strategy_tag"`
	JoinedAt              time.Time                `json:"joined_at"`
	PortfolioIndex        float64                  `json:"portfolio_index"`
	ReturnPercentage      float64                  `json:"return_percentage"`
	GlobalRank            *int                     `json:"global_rank,omitempty"`
	SprintRank            *int                     `json:"sprint_rank,omitempty"`
	Badges                []PublicBadge            `json:"badges"`
	PerformanceHistory    []PublicPerformancePoint `json:"performance_history"`
	PublicClosedPositions []PublicClosedPosition   `json:"public_closed_positions"`
	PublicWeights         []PublicWeight           `json:"public_weights"`
	AssetTypeExposure     []Exposure               `json:"asset_type_exposure"`
	CurrencyExposure      []Exposure               `json:"currency_exposure"`
	Concentration         Concentration            `json:"concentration"`
	Insights              ProfileInsights          `json:"insights"`
}

// PublicStrategy is the internal safe contract used by copy/compare features.
// UserID is for server-side self-copy checks only and must never be serialized
// on public responses.
type PublicStrategy struct {
	UserID        string
	Handle        string
	DisplayName   string
	AvatarKey     string
	StrategyTag   string
	Weights       []StrategyWeight
	Concentration Concentration
}

type OwnerProfile struct {
	Handle            string        `json:"handle"`
	DisplayName       string        `json:"display_name"`
	AvatarKey         string        `json:"avatar_key"`
	Bio               string        `json:"bio"`
	StrategyTag       string        `json:"strategy_tag"`
	IsPublic          bool          `json:"is_public"`
	ShowPublicWeights bool          `json:"show_public_weights"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	PublicPreview     PublicProfile `json:"public_preview"`
}
