package achievements

import "time"

// Achievement keys.
const (
	KeyFirstPortfolio = "first_portfolio"
	KeyFirstSprint    = "first_sprint"
	KeyGreenPortfolio = "green_portfolio"
	KeyIndex110       = "index_110"
	KeyTop10Sprint    = "top_10_sprint"
)

// Achievement is a badge definition.
type Achievement struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IconKey     string    `json:"icon_key"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserAchievement records that a user unlocked an achievement.
type UserAchievement struct {
	UserID        string    `json:"user_id"`
	AchievementID string    `json:"achievement_id"`
	UnlockedAt    time.Time `json:"unlocked_at"`
}

// AchievementResponse is the public, privacy-safe projection. It carries no
// internal ids — only the user-facing fields plus unlock state.
type AchievementResponse struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	IconKey     string     `json:"icon_key"`
	Unlocked    bool       `json:"unlocked"`
	UnlockedAt  *time.Time `json:"unlocked_at,omitempty"`
}

// seedDefinitions returns the initial badge catalogue in a stable order.
func seedDefinitions(now time.Time) []Achievement {
	def := func(key, name, desc, icon string) Achievement {
		return Achievement{ID: key, Key: key, Name: name, Description: desc, IconKey: icon, CreatedAt: now}
	}
	return []Achievement{
		def(KeyFirstPortfolio, "First Portfolio", "Added your first portfolio position.", "portfolio"),
		def(KeyFirstSprint, "First Sprint", "Joined your first weekly sprint.", "sprint"),
		def(KeyGreenPortfolio, "Green Portfolio", "Your portfolio performance is positive.", "green"),
		def(KeyIndex110, "Index 110", "Reached a portfolio index of 110 or higher.", "index"),
		def(KeyTop10Sprint, "Top 10 Sprint", "Ranked in the top 10 of a sprint leaderboard.", "trophy"),
	}
}
