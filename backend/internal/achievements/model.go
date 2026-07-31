package achievements

import (
	"time"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
)

// AwardedAchievement records that a user unlocked a benchmark badge, with the
// privacy-safe evidence that justified it.
type AwardedAchievement struct {
	UserID          string                        `json:"user_id"`
	BadgeKey        string                        `json:"badge_key"`
	UnlockedAt      time.Time                     `json:"unlocked_at"`
	Evidence        benchmark.AchievementEvidence `json:"evidence"`
	StartSnapshotID string                        `json:"-"`
	EndSnapshotID   string                        `json:"-"`
}

// AchievementResponse is the public, privacy-safe projection of a badge and the
// user's unlock state. It carries no internal ids, monetary values, or holdings
// — only user-facing catalogue metadata plus (when unlocked) percentage-based
// evidence.
type AchievementResponse struct {
	Key            string                         `json:"key"`
	Name           string                         `json:"name"`
	Description    string                         `json:"description"`
	IconKey        string                         `json:"icon_key"`
	Difficulty     string                         `json:"difficulty"`
	Period         string                         `json:"period"`
	InspiredBy     string                         `json:"inspired_by,omitempty"`
	Unlocked       bool                           `json:"unlocked"`
	UnlockedAt     *time.Time                     `json:"unlocked_at,omitempty"`
	Evidence       *benchmark.AchievementEvidence `json:"evidence,omitempty"`
	Progress       *AchievementProgress           `json:"progress,omitempty"`
	LegacyEvidence bool                           `json:"legacy_evidence,omitempty"`
}

// AchievementProgress is the live, privacy-safe state for a locked benchmark
// badge. It exposes percentage returns and time coverage only—never holdings,
// values, quantities, account ids, or monetary gain/loss.
type AchievementProgress struct {
	State                         string   `json:"state"`
	ProgressPercentage            float64  `json:"progress_percentage"`
	HistoryCoveragePercentage     float64  `json:"history_coverage_percentage"`
	ActiveCoveragePercentage      float64  `json:"active_coverage_percentage"`
	TrustedDataCoveragePercentage float64  `json:"trusted_data_coverage_percentage"`
	PortfolioReturnPercentage     *float64 `json:"portfolio_return_percentage,omitempty"`
	BenchmarkReturnPercentage     *float64 `json:"benchmark_return_percentage,omitempty"`
	CurrentEdgePoints             *float64 `json:"current_edge_points,omitempty"`
	StartDate                     string   `json:"start_date,omitempty"`
	EndDate                       string   `json:"end_date,omitempty"`
	EffectiveStartAt              string   `json:"effective_start_at,omitempty"`
	EffectiveEndAt                string   `json:"effective_end_at,omitempty"`
	LatestSnapshotAt              string   `json:"latest_snapshot_at,omitempty"`
	RequiredEdgePoints            float64  `json:"required_edge_points"`
	Reason                        string   `json:"reason"`
}

// AchievementReturnsResponse is the privacy-safe comparison table for one
// leaderboard-style timeframe. It contains percentages and dates only.
type AchievementReturnsResponse struct {
	Timeframe string                 `json:"timeframe"`
	From      string                 `json:"from,omitempty"`
	To        string                 `json:"to"`
	Rows      []AchievementReturnRow `json:"rows"`
}

// AchievementReturnRow compares the authenticated user's canonical ranked
// return with one badge benchmark over aligned market-close boundaries.
type AchievementReturnRow struct {
	Key                       string   `json:"key"`
	Name                      string   `json:"name"`
	Difficulty                string   `json:"difficulty"`
	NativePeriod              string   `json:"native_period"`
	Available                 bool     `json:"available"`
	PortfolioReturnPercentage *float64 `json:"portfolio_return_percentage,omitempty"`
	BenchmarkReturnPercentage *float64 `json:"benchmark_return_percentage,omitempty"`
	EdgePoints                *float64 `json:"edge_points,omitempty"`
	AlignedFrom               string   `json:"aligned_from,omitempty"`
	AlignedTo                 string   `json:"aligned_to,omitempty"`
	Reason                    string   `json:"reason,omitempty"`
}

// iconKeyForDifficulty maps a difficulty tier to a stable icon key the frontend
// can style. Kept deterministic so responses never churn.
func iconKeyForDifficulty(d benchmark.Difficulty) string {
	switch d {
	case benchmark.DifficultyEasy:
		return "medal"
	case benchmark.DifficultyMedium:
		return "award"
	case benchmark.DifficultyHard:
		return "trophy"
	case benchmark.DifficultyElite:
		return "crown"
	default:
		return "trophy"
	}
}
