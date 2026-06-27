package leaderboard

// LeaderboardEntry is the privacy-safe, public-facing ranking row. It is the
// ONLY shape ever serialized by this module. It deliberately omits every
// absolute financial figure and every identifier (user id, portfolio id,
// email, holdings) so the leaderboard reveals performance without revealing
// wealth, holdings, or identity.
type LeaderboardEntry struct {
	Rank        int    `json:"rank"`
	DisplayName string `json:"display_name"`
	AvatarKey   string `json:"avatar_key"`
	// RankedReturnPercentage / RankedIndex are the strategy-baseline ranking
	// (since-baseline percentage performance, index starting at 100).
	RankedReturnPercentage float64 `json:"ranked_return_percentage"`
	RankedIndex            float64 `json:"ranked_index"`
	// GainLossPercentage / PortfolioIndex are retained as backward-compatible
	// aliases for the Prototype 1 frontend. They carry the SAME percentage value
	// as the ranked fields — never an absolute money figure. New clients should
	// read the ranked_* fields.
	GainLossPercentage float64 `json:"gain_loss_percentage"`
	PortfolioIndex     float64 `json:"portfolio_index"`

	// The fields below are populated only for users whose profile is public, by
	// joining profile data. They let the board link to a profile and show the
	// strategy tag / composition. Private profiles stay fully anonymous: Handle
	// and PublicWeights are omitted. PublicWeights additionally requires the
	// profile's show_public_weights to be enabled.
	Handle        string         `json:"handle,omitempty"`
	StrategyTag   string         `json:"strategy_tag,omitempty"`
	PublicWeights []PublicWeight `json:"public_weights,omitempty"`
}

// PublicWeight is a privacy-safe composition entry: symbol, asset type, and
// percentage weight only — never quantities or money.
type PublicWeight struct {
	Symbol           string  `json:"symbol"`
	AssetType        string  `json:"asset_type"`
	WeightPercentage float64 `json:"weight_percentage"`
}
