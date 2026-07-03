package coach

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/profile"
)

// --- collaborator interfaces -------------------------------------------------
// Declared here and satisfied directly by the existing concrete services, so
// the coach stays decoupled and there are no import cycles (auth/portfolio/
// competitions/achievements never import coach).

// UserLister enumerates users for top-performer ranking. Satisfied by *auth.Service.
type UserLister interface {
	ListUsers(ctx context.Context) ([]auth.User, error)
}

// SummaryProvider computes a user's portfolio summary. Satisfied by *portfolio.Service.
type SummaryProvider interface {
	Summary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
}

// AchievementLister supplies a user's badges. Optional; satisfied by *achievements.Service.
type AchievementLister interface {
	ListAchievementsForUser(ctx context.Context, userID string) ([]achievements.AchievementResponse, error)
}

type PublicStrategyProvider interface {
	PublicStrategyByHandle(ctx context.Context, handle string) (profile.PublicStrategy, error)
}

// Service orchestrates deterministic fact-building and provider analysis.
type Service struct {
	users     UserLister
	summaries SummaryProvider
	provider  Provider

	achievements AchievementLister // optional
	profiles     PublicStrategyProvider
	clock        clock.Clock
}

// NewService wires the coach service. Achievements are optional and may be nil.
func NewService(users UserLister, summaries SummaryProvider, provider Provider) *Service {
	return &Service{
		users:     users,
		summaries: summaries,
		provider:  provider,
		clock:     clock.RealClock{},
	}
}

// SetAchievementLister attaches optional achievement context.
func (s *Service) SetAchievementLister(a AchievementLister) { s.achievements = a }

// SetProfileProvider attaches public profile weights for compare-profile.
func (s *Service) SetProfileProvider(p PublicStrategyProvider) { s.profiles = p }

// SetClock overrides the clock (tests use a FixedClock for deterministic timestamps).
func (s *Service) SetClock(c clock.Clock) { s.clock = c }

// SupportedMode reports whether mode is a recognized analysis mode.
func SupportedMode(mode string) bool { return supportedModes[mode] }

// Analyze runs the requested analysis for userID. It validates the mode, loads
// and sanitizes context, calls the provider (mock by default), and assembles a
// structured, advice-free response. The provider is never called for an empty
// portfolio or an unsupported mode.
func (s *Service) Analyze(ctx context.Context, userID, mode string) (*CoachResponse, error) {
	if !SupportedMode(mode) {
		return nil, ErrUnsupportedMode
	}

	summary, err := s.summaries.Summary(ctx, userID)
	if err != nil {
		return nil, err
	}
	if summary == nil || len(summary.Positions) == 0 {
		return nil, ErrEmptyPortfolio
	}

	userFacts := buildUserFacts(summary)

	input := CoachProviderInput{
		Mode:               mode,
		User:               userFacts,
		SafetyInstructions: safetyInstructions(),
	}

	// Top-10 context is only built for the comparison modes.
	needsTop10 := mode == ModeCompareTop10
	if needsTop10 {
		input.Top10 = s.buildTop10Facts(ctx, userID, userFacts)
		if !input.Top10.Available {
			input.DataLimitations = append(input.DataLimitations,
				"Top-10 benchmark unavailable: more leaderboard participants with positions are needed.")
		} else if input.Top10.Limited {
			input.DataLimitations = append(input.DataLimitations,
				"Top-10 benchmark is limited: fewer than 10 other portfolios are available, so comparisons are directional only.")
		}
	}

	if mode == ModeTechnicalSetup {
		input.DataLimitations = append(input.DataLimitations,
			"Prototype price data only: no historical series, so moving averages, RSI, MACD, and support/resistance cannot be computed. Notes are based on current return, concentration, and contribution.")
	}
	if mode == ModeFundamentalContext {
		input.DataLimitations = append(input.DataLimitations,
			"No fundamental data source is connected: company financials (revenue, earnings, valuation) are not available. Context is based on asset mix and concentration only.")
	}

	if s.achievements != nil {
		input.Achievements = s.collectAchievementFacts(ctx, userID)
	}

	out, err := s.provider.GeneratePortfolioCoachAnalysis(ctx, input)
	if err != nil {
		return nil, err
	}

	return s.assemble(mode, input, out), nil
}

func (s *Service) CompareProfile(ctx context.Context, userID, handle string) (*CompareProfileResponse, error) {
	if s.profiles == nil {
		return nil, profile.ErrNotFound
	}
	target, err := s.profiles.PublicStrategyByHandle(ctx, strings.TrimSpace(handle))
	if errors.Is(err, profile.ErrNotFound) {
		return nil, profile.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	summary, err := s.summaries.Summary(ctx, userID)
	if err != nil {
		return nil, err
	}
	if summary == nil || len(summary.Positions) == 0 || summary.CurrentValue <= 0 {
		return nil, ErrEmptyPortfolio
	}

	myWeights := make(map[string]WeightDifference)
	var myTop3 float64
	mySorted := make([]WeightDifference, 0, len(summary.Positions))
	for _, p := range summary.Positions {
		weight := round2(p.CurrentValueBase / summary.CurrentValue * 100)
		row := WeightDifference{
			Symbol:             p.Symbol,
			MyWeightPercentage: weight,
			AssetType:          p.AssetType,
		}
		myWeights[p.Symbol] = row
		mySorted = append(mySorted, row)
	}
	sort.SliceStable(mySorted, func(i, j int) bool {
		if mySorted[i].MyWeightPercentage != mySorted[j].MyWeightPercentage {
			return mySorted[i].MyWeightPercentage > mySorted[j].MyWeightPercentage
		}
		return mySorted[i].Symbol < mySorted[j].Symbol
	})
	for i := 0; i < len(mySorted) && i < 3; i++ {
		myTop3 += mySorted[i].MyWeightPercentage
	}

	targetWeights := make(map[string]profile.StrategyWeight)
	targetTop3 := 0.0
	targetSorted := append([]profile.StrategyWeight(nil), target.Weights...)
	sort.SliceStable(targetSorted, func(i, j int) bool {
		if targetSorted[i].WeightPercentage != targetSorted[j].WeightPercentage {
			return targetSorted[i].WeightPercentage > targetSorted[j].WeightPercentage
		}
		return targetSorted[i].Symbol < targetSorted[j].Symbol
	})
	for i, w := range targetSorted {
		targetWeights[w.Symbol] = w
		if i < 3 {
			targetTop3 += w.WeightPercentage
		}
	}

	seen := map[string]bool{}
	diffs := make([]WeightDifference, 0, len(myWeights)+len(targetWeights))
	shared := make([]string, 0)
	overlap := 0.0
	for symbol, mine := range myWeights {
		targetWeight := targetWeights[symbol]
		if targetWeight.Symbol != "" {
			shared = append(shared, symbol)
			overlap += math.Min(mine.MyWeightPercentage, targetWeight.WeightPercentage)
		}
		mine.TargetWeightPercentage = targetWeight.WeightPercentage
		mine.DifferencePercentage = round2(mine.MyWeightPercentage - targetWeight.WeightPercentage)
		if mine.AssetType == "" {
			mine.AssetType = targetWeight.AssetType
		}
		diffs = append(diffs, mine)
		seen[symbol] = true
	}
	for symbol, targetWeight := range targetWeights {
		if seen[symbol] {
			continue
		}
		diffs = append(diffs, WeightDifference{
			Symbol:                 symbol,
			MyWeightPercentage:     0,
			TargetWeightPercentage: targetWeight.WeightPercentage,
			DifferencePercentage:   round2(-targetWeight.WeightPercentage),
			AssetType:              targetWeight.AssetType,
		})
	}
	sort.Strings(shared)
	sort.SliceStable(diffs, func(i, j int) bool {
		ai := math.Abs(diffs[i].DifferencePercentage)
		aj := math.Abs(diffs[j].DifferencePercentage)
		if ai != aj {
			return ai > aj
		}
		return diffs[i].Symbol < diffs[j].Symbol
	})
	if len(diffs) > 10 {
		diffs = diffs[:10]
	}

	concentration := ConcentrationComparison{
		MyPositionCount:            len(summary.Positions),
		TargetPositionCount:        len(target.Weights),
		MyTop3WeightPercentage:     round2(myTop3),
		TargetTop3WeightPercentage: round2(targetTop3),
	}
	learning := compareLearningPoints(concentration, overlap)
	resp := &CompareProfileResponse{
		TargetProfile: CompareTargetProfile{
			Handle:      target.Handle,
			DisplayName: target.DisplayName,
			AvatarKey:   target.AvatarKey,
			StrategyTag: target.StrategyTag,
		},
		Summary:                 fmt.Sprintf("Your strategy overlaps %.1f%% with %s.", round2(overlap), target.DisplayName),
		OverlapScore:            round2(overlap),
		SharedSymbols:           shared,
		WeightDifferences:       diffs,
		ConcentrationComparison: concentration,
		LearningPoints:          learning,
		Disclaimer:              "This is educational comparison, not investment advice.",
	}
	return resp, nil
}

// assemble merges provider narrative with backend-authoritative numbers and the
// mandatory disclaimer. It also guarantees the disclaimer text is present.
func (s *Service) assemble(mode string, input CoachProviderInput, out CoachProviderOutput) *CoachResponse {
	riskLevel := out.RiskLevel
	if riskLevel == "" {
		riskLevel = input.User.RiskLevel
	}

	resp := &CoachResponse{
		Mode:                mode,
		Title:               out.Title,
		Summary:             out.Summary,
		RiskLevel:           riskLevel,
		Observations:        out.Observations,
		TechnicalNotes:      out.TechnicalNotes,
		FundamentalNotes:    out.FundamentalNotes,
		LearningPoints:      out.LearningPoints,
		QuestionsToConsider: out.QuestionsToConsider,
		Disclaimer:          Disclaimer,
		GeneratedAt:         s.clock.Now().UTC(),
	}

	// Attach the deterministic comparison block for the comparison modes.
	if mode == ModeCompareTop10 {
		resp.Top10Comparison = toComparison(input.Top10)
	} else {
		resp.Top10Comparison = CoachTop10Comparison{Available: false}
	}

	// Never return nil slices — emit empty arrays for stable JSON.
	resp.Observations = nonNilObservations(resp.Observations)
	resp.TechnicalNotes = nonNil(resp.TechnicalNotes)
	resp.FundamentalNotes = nonNil(resp.FundamentalNotes)
	resp.LearningPoints = nonNil(resp.LearningPoints)
	resp.QuestionsToConsider = nonNil(resp.QuestionsToConsider)
	if resp.Top10Comparison.Notes == nil {
		resp.Top10Comparison.Notes = []string{}
	}

	return resp
}

// toComparison projects the internal top-10 facts to the public comparison DTO.
func toComparison(f PublicTop10Facts) CoachTop10Comparison {
	notes := []string{}
	if !f.Available {
		notes = append(notes, "More leaderboard participants are needed before a benchmark can be shown.")
		return CoachTop10Comparison{Available: false, SampleSize: f.SampleSize, Notes: notes}
	}
	if f.Limited {
		notes = append(notes, "Benchmark is limited to fewer than 10 portfolios; treat comparisons as directional.")
	}
	return CoachTop10Comparison{
		Available:                          true,
		SampleSize:                         f.SampleSize,
		Limited:                            f.Limited,
		ReturnGapPercentagePoints:          f.ReturnGapPercentagePoints,
		SharedSymbolsCount:                 f.SharedSymbolsCount,
		UserLargestWeightPercentage:        f.UserLargestWeightPercentage,
		Top10MedianLargestWeightPercentage: f.MedianLargestWeightPercentage,
		Notes:                              notes,
	}
}

func (s *Service) collectAchievementFacts(ctx context.Context, userID string) []AchievementFact {
	list, err := s.achievements.ListAchievementsForUser(ctx, userID)
	if err != nil {
		return nil
	}
	facts := make([]AchievementFact, 0, len(list))
	for _, a := range list {
		facts = append(facts, AchievementFact{Name: a.Name, Unlocked: a.Unlocked})
	}
	return facts
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilObservations(s []CoachObservation) []CoachObservation {
	if s == nil {
		return []CoachObservation{}
	}
	return s
}

func compareLearningPoints(c ConcentrationComparison, overlap float64) []LearningPoint {
	points := []LearningPoint{}
	switch {
	case c.TargetTop3WeightPercentage > c.MyTop3WeightPercentage+10:
		points = append(points, LearningPoint{
			Title: "Concentration difference",
			Body:  "The target strategy is more concentrated, so ranked performance may be more sensitive to a few holdings.",
		})
	case c.MyTop3WeightPercentage > c.TargetTop3WeightPercentage+10:
		points = append(points, LearningPoint{
			Title: "Concentration difference",
			Body:  "Your strategy is more concentrated than the target, so fewer holdings may drive more of your ranked movement.",
		})
	default:
		points = append(points, LearningPoint{
			Title: "Similar concentration",
			Body:  "Both strategies have a similar top-three concentration profile.",
		})
	}
	if overlap == 0 {
		points = append(points, LearningPoint{
			Title: "No shared symbols",
			Body:  "The strategies use different symbols, so overlap is zero even if risk levels look similar.",
		})
	}
	return points
}
