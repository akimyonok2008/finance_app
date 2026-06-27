package coach

import (
	"context"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
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

// Service orchestrates deterministic fact-building and provider analysis.
type Service struct {
	users     UserLister
	summaries SummaryProvider
	provider  Provider

	achievements AchievementLister // optional
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
