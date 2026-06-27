package profile

// Explore powers the "Explore Strategies" discovery page. Everything here is
// built on top of the existing public projection (PublicProfile), so the same
// privacy guarantees apply: only public symbols, weights, ranks, badges and
// strategy metadata are ever exposed — never quantities, values, cost basis,
// absolute gain/loss, emails, or internal IDs.

// Allowed sort modes for GET /profiles/explore.
const (
	SortTop    = "top"
	SortReturn = "return"
	SortRank   = "rank"
	SortRecent = "recent"
)

const (
	defaultExploreLimit = 20
	maxExploreLimit     = 50
	featuredCount       = 3
	maxSimilar          = 5
	maxTrendingHoldings = 12
	trendingTopN        = 10
)

// ExploreFilter is the validated, normalized query for the Explore endpoint.
type ExploreFilter struct {
	Query  string // free-text search over handle / display name / public symbol
	Symbol string // exact public-symbol filter, already uppercased
	Sort   string // one of SortTop/SortReturn/SortRank/SortRecent
	Limit  int    // page size for top_performers (1..maxExploreLimit)
	Offset int    // page offset for top_performers
}

// TrendingHolding summarizes how often a symbol appears across public profiles.
// It deliberately carries no per-profile or monetary data.
type TrendingHolding struct {
	Symbol        string  `json:"symbol"`
	ProfileCount  int     `json:"profile_count"`
	AverageWeight float64 `json:"average_weight_percentage"`
	Top10Count    int     `json:"top10_count"`
	AssetType     string  `json:"asset_type"`
}

// ExplorePagination describes the top_performers window.
type ExplorePagination struct {
	Limit   int  `json:"limit"`
	Offset  int  `json:"offset"`
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

// ExploreResponse is the full payload for the Explore page. Featured, Similar
// and TopPerformers reuse the public-safe PublicProfile card so the card
// contract stays identical to GET /profiles/{handle}.
type ExploreResponse struct {
	Featured         []PublicProfile   `json:"featured"`
	Similar          []PublicProfile   `json:"similar"`
	TopPerformers    []PublicProfile   `json:"top_performers"`
	TrendingHoldings []TrendingHolding `json:"trending_holdings"`
	Pagination       ExplorePagination `json:"pagination"`
}
