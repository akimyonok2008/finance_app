package leaderboard

// LeaderboardEntry is the privacy-safe, public-facing ranking row. It is the
// ONLY shape ever serialized by this module. It deliberately omits every
// absolute financial figure and every identifier (user id, portfolio id,
// email, holdings) so the leaderboard reveals performance without revealing
// wealth, holdings, or identity.
type LeaderboardEntry struct {
	Rank               int     `json:"rank"`
	DisplayName        string  `json:"display_name"`
	AvatarKey          string  `json:"avatar_key"`
	GainLossPercentage float64 `json:"gain_loss_percentage"`
	PortfolioIndex     float64 `json:"portfolio_index"`
}
