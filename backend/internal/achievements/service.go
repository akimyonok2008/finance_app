package achievements

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
)

// coverageFraction is how much of a badge's nominal period the portfolio's
// available history must span before the badge can be evaluated.
const coverageFraction = 0.9

// seriesWindow returns the earliest and latest dates in an index series.
func seriesWindow(series []benchmark.IndexPoint) (time.Time, time.Time, bool) {
	if len(series) < 2 {
		return time.Time{}, time.Time{}, false
	}
	sorted := append([]benchmark.IndexPoint(nil), series...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date < sorted[j].Date })
	first, err1 := time.Parse("2006-01-02", sorted[0].Date)
	last, err2 := time.Parse("2006-01-02", sorted[len(sorted)-1].Date)
	if err1 != nil || err2 != nil || !last.After(first) {
		return time.Time{}, time.Time{}, false
	}
	return first, last, true
}

// coversPeriod reports whether [first, last] spans at least coverageFraction of
// the badge's nominal period.
func coversPeriod(first, last time.Time, period benchmark.PeriodCode) bool {
	need := periodDays(period) * coverageFraction
	got := last.Sub(first).Hours() / 24
	return need > 0 && got >= need
}

func periodDays(period benchmark.PeriodCode) float64 {
	switch period {
	case benchmark.Period30D:
		return 30
	case benchmark.Period90D:
		return 90
	case benchmark.Period6M:
		return 182
	case benchmark.Period1Y:
		return 365
	default:
		return 0
	}
}

// PortfolioPerformanceProvider supplies a user's portfolio index (TWR) series
// over a window. Implemented by an adapter over the portfolio archive engine —
// the achievements module never recomputes returns from trades.
type PortfolioPerformanceProvider interface {
	GetPortfolioIndexSeries(ctx context.Context, userID string, start, end time.Time) ([]benchmark.IndexPoint, error)
}

// Service evaluates and awards benchmark badges. It is best-effort: evaluation
// failures never block the caller's main request.
type Service struct {
	repo        AchievementRepository
	performance PortfolioPerformanceProvider
	engine      *benchmark.BenchmarkConstructionService
	rules       *benchmark.RulesEngine
	badges      []benchmark.Badge
	now         func() time.Time
}

// NewService wires an achievements Service over the benchmark engine.
func NewService(
	repo AchievementRepository,
	performance PortfolioPerformanceProvider,
	engine *benchmark.BenchmarkConstructionService,
	rules *benchmark.RulesEngine,
) *Service {
	return &Service{
		repo:        repo,
		performance: performance,
		engine:      engine,
		rules:       rules,
		badges:      benchmark.Badges,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

// ListAchievementsForUser returns every badge in the catalogue with the user's
// unlock state and evidence.
func (s *Service) ListAchievementsForUser(ctx context.Context, userID string) ([]AchievementResponse, error) {
	return s.listAchievementsForUser(ctx, userID, false)
}

func (s *Service) listAchievementsForUser(ctx context.Context, userID string, includeProgress bool) ([]AchievementResponse, error) {
	awarded, err := s.repo.ListAwarded(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]AchievementResponse, 0, len(s.badges))
	for _, badge := range s.badges {
		resp := AchievementResponse{
			Key:         badge.ID,
			Name:        badge.Name,
			Description: badge.Description,
			IconKey:     iconKeyForDifficulty(badge.Difficulty),
			Difficulty:  string(badge.Difficulty),
			Period:      string(badge.Period),
			InspiredBy:  badge.InspiredBy,
		}
		if a, ok := awarded[badge.ID]; ok {
			at := a.UnlockedAt
			evidence := a.Evidence
			resp.Unlocked = true
			resp.UnlockedAt = &at
			resp.Evidence = &evidence
		} else if includeProgress {
			resp.Progress = s.progressForBadge(ctx, userID, badge)
		}
		out = append(out, resp)
	}
	return out, nil
}

// EvaluateAll re-checks every benchmark badge for the user and returns the
// updated list.
func (s *Service) EvaluateAll(ctx context.Context, userID string) ([]AchievementResponse, error) {
	_, _ = s.checkAndAwardBadges(ctx, userID)
	return s.listAchievementsForUser(ctx, userID, true)
}

// progressForBadge measures live progress without weakening the award rules.
// Sixty percent of the displayed progress represents having enough history to
// evaluate the full window; forty percent represents the current return/edge
// criteria. Unlocking still requires the existing 90% history coverage gate and
// the exact rule evaluator.
func (s *Service) progressForBadge(ctx context.Context, userID string, badge benchmark.Badge) *AchievementProgress {
	progress := &AchievementProgress{
		State:  "building_history",
		Reason: "Benchmark tracking is active. Collecting daily portfolio history.",
	}
	now := s.now()
	start, err := benchmark.SubtractPeriod(now, badge.Period)
	if err != nil {
		return progress
	}
	series, err := s.performance.GetPortfolioIndexSeries(ctx, userID, start, now)
	if err != nil || len(series) < 2 {
		return progress
	}
	first, last, ok := seriesWindow(series)
	if !ok {
		return progress
	}

	requiredDays := periodDays(badge.Period)
	if requiredDays <= 0 {
		progress.Reason = "Benchmark tracking is active, but this badge period is invalid."
		return progress
	}
	coverage := clamp01(last.Sub(first).Hours() / 24 / requiredDays)
	historyReadiness := clamp01(coverage / coverageFraction)
	progress.HistoryCoveragePercentage = roundProgress(coverage * 100)
	progress.ProgressPercentage = roundProgress(historyReadiness * 60)
	progress.StartDate = first.UTC().Format("2006-01-02")
	progress.EndDate = last.UTC().Format("2006-01-02")

	portfolioReturn, err := benchmark.CalculateIndexReturnPct(series)
	if err != nil {
		progress.Reason = fmt.Sprintf("Benchmark tracking is active. %.0f%% of the required history is available.", progress.HistoryCoveragePercentage)
		return progress
	}
	benchmarkReturn, err := s.engine.CalculateReturnPct(ctx, badge.RecipeID, first, last)
	if err != nil {
		progress.State = "benchmark_unavailable"
		progress.Reason = "Portfolio history is active; benchmark prices are temporarily unavailable."
		return progress
	}

	edge := portfolioReturn - benchmarkReturn
	portfolioReturn = roundProgress(portfolioReturn)
	benchmarkReturn = roundProgress(benchmarkReturn)
	edge = roundProgress(edge)
	progress.PortfolioReturnPercentage = &portfolioReturn
	progress.BenchmarkReturnPercentage = &benchmarkReturn
	progress.CurrentEdgePoints = &edge
	progress.State = "tracking"

	edgeProgress := 0.0
	if badge.Rule.RequiredEdgePoints > 0 {
		edgeProgress = clamp01(edge / badge.Rule.RequiredEdgePoints)
	} else if edge > 0 {
		edgeProgress = 1
	}
	positiveProgress := 1.0
	if badge.Rule.RequiresPositiveReturn && portfolioReturn <= 0 {
		positiveProgress = 0
	}
	criteriaProgress := math.Min(edgeProgress, positiveProgress)
	progress.ProgressPercentage = roundProgress(math.Min(99, (0.60*historyReadiness+0.40*criteriaProgress)*100))

	if coverage < coverageFraction {
		progress.Reason = fmt.Sprintf(
			"Tracking live. %.0f%% of the %s history window is available.",
			progress.HistoryCoveragePercentage,
			badge.Period,
		)
	} else if badge.Rule.RequiresPositiveReturn && portfolioReturn <= 0 {
		progress.Reason = "Full history is available; a positive portfolio return is still required."
	} else {
		progress.Reason = fmt.Sprintf(
			"Tracking live edge: %+.2f pts versus %+.2f pts required.",
			edge,
			badge.Rule.RequiredEdgePoints,
		)
	}
	return progress
}

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func roundProgress(value float64) float64 {
	return math.Round(value*100) / 100
}

// EvaluatePortfolioAchievements re-checks benchmark badges after a portfolio
// change. Best-effort: errors are swallowed so the caller is never blocked.
func (s *Service) EvaluatePortfolioAchievements(ctx context.Context, userID string) error {
	_, _ = s.checkAndAwardBadges(ctx, userID)
	return nil
}

// EvaluateSprintJoinAchievements re-checks benchmark badges when a user joins a
// sprint. Retained for the competitions integration; benchmark badges are
// portfolio-relative, so this simply triggers a re-check.
func (s *Service) EvaluateSprintJoinAchievements(ctx context.Context, userID string) error {
	_, _ = s.checkAndAwardBadges(ctx, userID)
	return nil
}

// EvaluateSprintRankAchievements re-checks benchmark badges after a sprint rank
// update. The competitionID is accepted for interface compatibility.
func (s *Service) EvaluateSprintRankAchievements(ctx context.Context, userID, _ string) error {
	_, _ = s.checkAndAwardBadges(ctx, userID)
	return nil
}

// checkAndAwardBadges evaluates every not-yet-awarded badge for the user and
// persists newly unlocked ones. It is the single source of unlock logic.
func (s *Service) checkAndAwardBadges(ctx context.Context, userID string) ([]AwardedAchievement, error) {
	awarded, err := s.repo.ListAwarded(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	var newly []AwardedAchievement

	for _, badge := range s.badges {
		if _, done := awarded[badge.ID]; done {
			continue
		}

		start, err := benchmark.SubtractPeriod(now, badge.Period)
		if err != nil {
			continue
		}

		series, err := s.performance.GetPortfolioIndexSeries(ctx, userID, start, now)
		if err != nil || len(series) < 2 {
			continue // not enough portfolio history to judge this period
		}

		// Measure the portfolio and the benchmark over the SAME window — the
		// actual span the portfolio has data for — and require that span to cover
		// most of the badge period, so a young portfolio can't earn a long-period
		// badge on a few days of data.
		firstDate, lastDate, ok := seriesWindow(series)
		if !ok || !coversPeriod(firstDate, lastDate, badge.Period) {
			continue
		}

		portfolioReturnPct, err := benchmark.CalculateIndexReturnPct(series)
		if err != nil {
			continue
		}

		benchmarkReturnPct, err := s.engine.CalculateReturnPct(ctx, badge.RecipeID, firstDate, lastDate)
		if err != nil {
			continue // benchmark data unavailable — never award on missing data
		}

		result, err := s.rules.Evaluate(benchmark.EvaluationContext{
			Badge:              badge,
			StartDate:          firstDate.UTC().Format("2006-01-02"),
			EndDate:            lastDate.UTC().Format("2006-01-02"),
			PortfolioReturnPct: portfolioReturnPct,
			BenchmarkReturnPct: benchmarkReturnPct,
		})
		if err != nil || !result.Unlocked || result.Evidence == nil {
			continue
		}

		record := AwardedAchievement{
			UserID:     userID,
			BadgeKey:   badge.ID,
			UnlockedAt: now,
			Evidence:   *result.Evidence,
		}
		if err := s.repo.Award(ctx, record); err != nil {
			continue
		}
		newly = append(newly, record)
	}

	return newly, nil
}
