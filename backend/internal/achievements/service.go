package achievements

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
)

// RankedPerformanceHistoryProvider is the canonical history contract shared
// with timeframe leaderboards. It never exposes portfolio values or cost basis.
type RankedPerformanceHistoryProvider interface {
	Window(ctx context.Context, userID string, start, end time.Time) (performancehistory.Window, error)
	ProtectEvidence(ctx context.Context, ids ...string) error
	EligibilityThreshold() float64
	SnapshotFrequency() string
}

type Service struct {
	repo                 AchievementRepository
	history              RankedPerformanceHistoryProvider
	engine               *benchmark.BenchmarkConstructionService
	rules                *benchmark.RulesEngine
	badges               []benchmark.Badge
	now                  func() time.Time
	benchmarkDataSource  string
	allowPermanentAwards bool
	// policy decides whether a benchmark result may become a permanent award and
	// with what verification level. environment gates production awards.
	policy      benchmark.AwardEligibilityPolicy
	environment benchmark.EnvironmentMode
}

func NewService(
	repo AchievementRepository,
	history RankedPerformanceHistoryProvider,
	engine *benchmark.BenchmarkConstructionService,
	rules *benchmark.RulesEngine,
) *Service {
	return &Service{
		repo: repo, history: history, engine: engine, rules: rules,
		badges: benchmark.Badges, now: func() time.Time { return time.Now().UTC() },
		benchmarkDataSource:  "configured_historical_provider",
		allowPermanentAwards: true,
		// Default safely: only verified real data may back a permanent award.
		policy:      benchmark.NewAwardEligibilityPolicy(benchmark.AwardModeVerifiedOnly),
		environment: benchmark.EnvironmentDevelopment,
	}
}

func (s *Service) SetClock(now func() time.Time) { s.now = now }
func (s *Service) SetBenchmarkDataSource(source string) {
	if source != "" {
		s.benchmarkDataSource = source
		// Synthetic prices are useful for local progress previews, but they are
		// not admissible evidence for a permanent performance award.
		s.allowPermanentAwards = source != "mock"
	}
}

// SetAwardPolicy configures the permanent-award policy and environment. This is
// the single authority over verified/demo/disabled behavior.
func (s *Service) SetAwardPolicy(mode benchmark.AwardMode, env benchmark.EnvironmentMode) {
	s.policy = benchmark.NewAwardEligibilityPolicy(mode)
	if env != "" {
		s.environment = env
	}
}

func (s *Service) ListAchievementsForUser(ctx context.Context, userID string) ([]AchievementResponse, error) {
	return s.list(ctx, userID, false)
}

func (s *Service) EvaluateAll(ctx context.Context, userID string) ([]AchievementResponse, error) {
	if err := s.EvaluateLocked(ctx, userID); err != nil {
		return nil, err
	}
	return s.list(ctx, userID, true)
}

// EvaluateLocked evaluates only badges that have not already been awarded.
// Workers call this after a trusted snapshot commit.
func (s *Service) EvaluateLocked(ctx context.Context, userID string) error {
	_, err := s.checkAndAwardBadges(ctx, userID)
	return err
}

type periodResult struct {
	window performancehistory.Window
	err    error
}

func (s *Service) periodWindows(ctx context.Context, userID string, now time.Time) map[benchmark.PeriodCode]periodResult {
	out := map[benchmark.PeriodCode]periodResult{}
	for _, period := range []benchmark.PeriodCode{
		benchmark.Period30D, benchmark.Period90D, benchmark.Period6M, benchmark.Period1Y,
	} {
		start, err := benchmark.SubtractPeriod(now, period)
		if err != nil {
			out[period] = periodResult{err: err}
			continue
		}
		window, err := s.history.Window(ctx, userID, start, now)
		out[period] = periodResult{window: window, err: err}
	}
	return out
}

func (s *Service) list(ctx context.Context, userID string, includeProgress bool) ([]AchievementResponse, error) {
	awarded, err := s.repo.ListAwarded(ctx, userID)
	if err != nil {
		return nil, err
	}
	var windows map[benchmark.PeriodCode]periodResult
	if includeProgress {
		windows = s.periodWindows(ctx, userID, s.now())
	}
	benchmarkCache := map[string]benchmarkResult{}
	out := make([]AchievementResponse, 0, len(s.badges))
	for _, badge := range s.badges {
		resp := AchievementResponse{
			Key: badge.ID, Name: badge.Name, Description: badge.Description,
			IconKey:    iconKeyForDifficulty(badge.Difficulty),
			Difficulty: string(badge.Difficulty), Period: string(badge.Period),
			InspiredBy: badge.InspiredBy,
		}
		if award, ok := awarded[badge.ID]; ok {
			at, evidence := award.UnlockedAt, award.Evidence
			if evidence.EvaluationModel == "" {
				evidence.EvaluationModel = "archive_model_v0"
				evidence.EvidenceVersion = 0
			}
			resp.Unlocked, resp.UnlockedAt, resp.Evidence = true, &at, &evidence
			resp.LegacyEvidence = evidence.EvaluationModel != "ranked_snapshot_v1"
		} else if includeProgress {
			result := windows[badge.Period]
			resp.Progress = s.progress(ctx, badge, result, benchmarkCache)
		}
		out = append(out, resp)
	}
	return out, nil
}

type benchmarkResult struct {
	value float64
	err   error
}

func (s *Service) benchmarkReturn(
	ctx context.Context, badge benchmark.Badge, window performancehistory.Window,
	cache map[string]benchmarkResult,
) (float64, error) {
	start, end := window.StartSnapshot.CapturedAt, window.EndSnapshot.CapturedAt
	key := badge.RecipeID + "|" + start.Format(time.RFC3339Nano) + "|" + end.Format(time.RFC3339Nano)
	if cached, ok := cache[key]; ok {
		return cached.value, cached.err
	}
	value, err := s.engine.CalculateReturnPct(ctx, badge.RecipeID, start, end)
	cache[key] = benchmarkResult{value: value, err: err}
	return value, err
}

func rankedReturn(window performancehistory.Window) (float64, error) {
	start, end := window.StartSnapshot.RankedIndex, window.EndSnapshot.RankedIndex
	if start.Sign() <= 0 || end.Sign() <= 0 {
		return 0, performancehistory.ErrInvalidSnapshot
	}
	return performancehistory.TimeframeReturnPercent(start, end)
}

func (s *Service) progress(
	ctx context.Context, badge benchmark.Badge, period periodResult,
	cache map[string]benchmarkResult,
) *AchievementProgress {
	progress := &AchievementProgress{
		State:              "building_history",
		Reason:             "Building trusted ranked-performance history.",
		RequiredEdgePoints: badge.Rule.RequiredEdgePoints,
	}
	if period.err != nil {
		return progress
	}
	window := period.window
	progress.HistoryCoveragePercentage = round(window.HistoryCoverage * 100)
	progress.ActiveCoveragePercentage = round(window.ActiveCoverage * 100)
	progress.TrustedDataCoveragePercentage = round(window.TrustedCoverage * 100)
	progress.EffectiveStartAt = window.StartSnapshot.CapturedAt.Format(time.RFC3339)
	progress.EffectiveEndAt = window.EndSnapshot.CapturedAt.Format(time.RFC3339)
	progress.LatestSnapshotAt = progress.EffectiveEndAt
	progress.StartDate = window.StartSnapshot.CapturedAt.Format("2006-01-02")
	progress.EndDate = window.EndSnapshot.CapturedAt.Format("2006-01-02")
	threshold := s.history.EligibilityThreshold()
	historyReadiness := clamp(window.HistoryCoverage / threshold)
	activeReadiness := clamp(window.ActiveCoverage / threshold)
	trustedReadiness := clamp(window.TrustedCoverage / threshold)
	coverageReadiness := math.Min(historyReadiness, math.Min(activeReadiness, trustedReadiness))
	progress.ProgressPercentage = round(coverageReadiness * 60)

	if window.HistoryCoverage < threshold {
		progress.Reason = fmt.Sprintf("Building ranked history: %.0f%% available; %.0f%% is required.",
			progress.HistoryCoveragePercentage, threshold*100)
		return progress
	}
	if window.ActiveCoverage < threshold {
		progress.State = "insufficient_active_coverage"
		progress.Reason = fmt.Sprintf("Portfolio was active for %.0f%% of this period; %.0f%% is required.",
			progress.ActiveCoveragePercentage, threshold*100)
		return progress
	}
	if window.TrustedCoverage < threshold {
		progress.State = "insufficient_trusted_data"
		progress.Reason = fmt.Sprintf("Trusted snapshot coverage is %.0f%%; %.0f%% is required.",
			progress.TrustedDataCoveragePercentage, threshold*100)
		return progress
	}
	portfolioReturn, err := rankedReturn(window)
	if err != nil {
		return progress
	}
	// Preview-grade benchmark return: always computed so the user sees where they
	// stand even when the data is not award-grade.
	benchmarkReturn, err := s.benchmarkReturn(ctx, badge, window, cache)
	if err != nil {
		progress.State = classifyBenchmarkError(err)
		progress.Reason = benchmarkReasonFor(progress.State)
		return progress
	}
	edge := portfolioReturn - benchmarkReturn
	roundedPortfolio, roundedBenchmark, roundedEdge := round(portfolioReturn), round(benchmarkReturn), round(edge)
	progress.PortfolioReturnPercentage = &roundedPortfolio
	progress.BenchmarkReturnPercentage = &roundedBenchmark
	progress.CurrentEdgePoints = &roundedEdge

	// Distinguish "beats the benchmark" from "may earn a verified award". Probe
	// award-grade data to classify the exact integrity status.
	if s.policy.Mode == benchmark.AwardModeDisabled {
		progress.State = "benchmark_unverified"
		progress.Reason = "Benchmark awards are disabled in this environment; progress is preview-only."
	} else if _, awardErr := s.engine.CalculateReturn(ctx, badge.RecipeID,
		window.StartSnapshot.CapturedAt, window.EndSnapshot.CapturedAt,
		benchmark.RequirementForAwards()); awardErr != nil {
		progress.State = classifyBenchmarkError(awardErr)
		progress.Reason = benchmarkReasonForEdge(progress.State, roundedEdge)
	} else if s.policy.Mode == benchmark.AwardModeDemo {
		progress.State = "benchmark_unverified"
		progress.Reason = "This badge is running in demo mode and cannot create a verified permanent award."
	} else {
		progress.State = "eligible_but_rule_not_met"
		progress.Reason = fmt.Sprintf("Trusted ranked edge: %+.2f pts versus %+.2f pts required.",
			roundedEdge, badge.Rule.RequiredEdgePoints)
	}

	edgeReadiness := 1.0
	if badge.Rule.RequiredEdgePoints > 0 {
		edgeReadiness = clamp(edge / badge.Rule.RequiredEdgePoints)
	} else if edge <= 0 {
		edgeReadiness = 0
	}
	if badge.Rule.RequiresPositiveReturn && portfolioReturn <= 0 {
		edgeReadiness = 0
	}
	progress.ProgressPercentage = round(math.Min(99, 60+40*edgeReadiness))
	return progress
}

// classifyBenchmarkError maps a typed benchmark-data error to a specific
// progress state, so the UI never shows a generic "unavailable" for every
// integrity failure.
func classifyBenchmarkError(err error) string {
	switch {
	case errors.Is(err, benchmark.ErrAdjustedDataUnavailable), errors.Is(err, benchmark.ErrTotalReturnUnavailable):
		return "benchmark_unadjusted"
	case errors.Is(err, benchmark.ErrStaleBenchmarkData):
		return "benchmark_stale"
	case errors.Is(err, benchmark.ErrSyntheticDataNotAllowed):
		return "benchmark_unverified"
	case errors.Is(err, benchmark.ErrRecipeVersionUnavailable):
		return "recipe_version_unavailable"
	default:
		return "benchmark_unavailable"
	}
}

func benchmarkReasonFor(state string) string {
	switch state {
	case "benchmark_unadjusted":
		return "Verified total-return benchmark data is unavailable for this period."
	case "benchmark_stale":
		return "Benchmark data for this period is stale."
	case "benchmark_unverified":
		return "Benchmark data is preview-only and cannot create a verified permanent award."
	case "recipe_version_unavailable":
		return "The benchmark recipe for this historical period is unavailable."
	default:
		return "Benchmark data is unavailable for the ranked-performance interval."
	}
}

func benchmarkReasonForEdge(state string, edge float64) string {
	if state == "benchmark_unadjusted" && edge > 0 {
		return "Your portfolio currently beats the benchmark, but verified total-return data is unavailable."
	}
	return benchmarkReasonFor(state)
}

func (s *Service) checkAndAwardBadges(ctx context.Context, userID string) ([]AwardedAchievement, error) {
	// Disabled mode shows catalogue and progress but never writes an award.
	if s.policy.Mode == benchmark.AwardModeDisabled {
		return nil, nil
	}
	awarded, err := s.repo.ListAwarded(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := s.now()
	windows := s.periodWindows(ctx, userID, now)
	newly := []AwardedAchievement{}
	for _, badge := range s.badges {
		if _, exists := awarded[badge.ID]; exists {
			continue
		}
		period := windows[badge.Period]
		if period.err != nil || !period.window.Eligible(s.history.EligibilityThreshold()) {
			continue
		}
		window := period.window
		portfolioReturn, err := rankedReturn(window)
		if err != nil {
			continue
		}
		// Award-grade benchmark evaluation. Under verified_only the strict
		// requirement makes raw, synthetic or stale data fail closed with a typed
		// error before any award. Under demo mode synthetic data is permitted into
		// the evaluation so the policy can classify it as a demo award; the policy
		// — not the requirement — remains the single authority on verification.
		req := benchmark.RequirementForAwards()
		if s.policy.Mode == benchmark.AwardModeDemo {
			req = benchmark.RequirementForPreview()
		}
		benchResult, err := s.engine.CalculateReturn(
			ctx, badge.RecipeID,
			window.StartSnapshot.CapturedAt, window.EndSnapshot.CapturedAt,
			req,
		)
		if err != nil {
			continue
		}
		benchReturnPct, err := strconv.ParseFloat(benchResult.ReturnPercentage.String(), 64)
		if err != nil {
			continue
		}
		result, err := s.rules.Evaluate(benchmark.EvaluationContext{
			Badge:              badge,
			StartDate:          benchResult.EffectiveStart.Format("2006-01-02"),
			EndDate:            benchResult.EffectiveEnd.Format("2006-01-02"),
			PortfolioReturnPct: portfolioReturn,
			BenchmarkReturnPct: benchReturnPct,
		})
		if err != nil || !result.Unlocked || result.Evidence == nil {
			continue
		}
		// The rule passing is not sufficient: the award policy decides whether the
		// data is trusted enough to persist, and with what verification level.
		decision := s.policy.CanPersistPermanentAward(benchResult, s.environment)
		if !decision.Eligible {
			// Structured, privacy-safe: no monetary values or holdings.
			slog.Info("benchmark award blocked by data policy",
				"badge", badge.ID, "recipe_version", benchResult.RecipeVersion.VersionID,
				"quality", string(benchResult.DataMetadata.Quality), "reasons", decision.Reasons)
			continue
		}
		evidence := *result.Evidence
		evidence.EvaluationModel = "ranked_snapshot_v1"
		evidence.EvidenceVersion = 2
		evidence.TrackingEpoch = window.StartSnapshot.TrackingStartedAt.Format(time.RFC3339Nano)
		evidence.StartRankedIndex = window.StartSnapshot.RankedIndex.Float64()
		evidence.EndRankedIndex = window.EndSnapshot.RankedIndex.Float64()
		evidence.StartSnapshotAt = window.StartSnapshot.CapturedAt.Format(time.RFC3339Nano)
		evidence.EndSnapshotAt = window.EndSnapshot.CapturedAt.Format(time.RFC3339Nano)
		evidence.ActiveCoveragePct = round(window.ActiveCoverage * 100)
		evidence.TrustedCoveragePct = round(window.TrustedCoverage * 100)
		evidence.BenchmarkDataSource = s.benchmarkDataSource
		evidence.SnapshotFrequency = s.history.SnapshotFrequency()
		// Benchmark data-integrity provenance (evidence v2).
		evidence.Verification = decision.Verification
		evidence.BenchmarkRecipeVersion = benchResult.RecipeVersion.VersionID
		evidence.RebalancingPolicy = benchResult.RecipeVersion.RebalancingPolicy
		evidence.BenchmarkInputHash = benchResult.Fingerprint
		dataEvidence := benchmark.EvidenceFromResult(benchResult)
		evidence.DataSourceSummary = &dataEvidence
		if err := s.history.ProtectEvidence(ctx, window.StartSnapshot.ID, window.EndSnapshot.ID); err != nil {
			continue
		}
		record := AwardedAchievement{
			UserID: userID, BadgeKey: badge.ID, UnlockedAt: now, Evidence: evidence,
			StartSnapshotID: window.StartSnapshot.ID, EndSnapshotID: window.EndSnapshot.ID,
		}
		if err := s.repo.Award(ctx, record); err != nil {
			continue
		}
		slog.Info("benchmark award issued",
			"badge", badge.ID, "verification", string(decision.Verification),
			"recipe_version", benchResult.RecipeVersion.VersionID,
			"quality", string(benchResult.DataMetadata.Quality),
			"fingerprint", benchResult.Fingerprint)
		newly = append(newly, record)
	}
	return newly, nil
}

// Legacy trigger ports remain no-ops. Evaluation is driven by committed trusted
// snapshots or the explicit POST /achievements/evaluate endpoint.
func (s *Service) EvaluatePortfolioAchievements(context.Context, string) error          { return nil }
func (s *Service) EvaluateSprintJoinAchievements(context.Context, string) error         { return nil }
func (s *Service) EvaluateSprintRankAchievements(context.Context, string, string) error { return nil }

func clamp(value float64) float64 { return math.Max(0, math.Min(1, value)) }
func round(value float64) float64 { return math.Round(value*100) / 100 }

func IsHistoryNotReady(err error) bool {
	return errors.Is(err, performancehistory.ErrWindowNotReady)
}
