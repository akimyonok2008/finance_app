package achievements

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// PositionProvider lists a user's positions. Implemented by an adapter over
// *portfolio.Service.
type PositionProvider interface {
	ListPositions(ctx context.Context, userID string) ([]portfolio.Position, error)
}

// PortfolioSummaryProvider computes a user's portfolio summary.
type PortfolioSummaryProvider interface {
	GetSummary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
}

// CompetitionRankProvider returns a user's rank in a competition (0 if not
// ranked). Implemented by an adapter over *competitions.Service — this module
// never imports competitions, avoiding an import cycle.
type CompetitionRankProvider interface {
	GetUserRank(ctx context.Context, competitionID, userID string) (int, error)
}

// CurrentCompetitionProvider returns the id of the currently active sprint, so
// EvaluateAll can re-check sprint achievements. Optional.
type CurrentCompetitionProvider interface {
	CurrentCompetitionID(ctx context.Context) string
}

// Service evaluates and reports achievements. It depends only on interfaces and
// is safe to call best-effort: evaluation failures never block the caller's
// main request.
type Service struct {
	repo      AchievementRepository
	positions PositionProvider
	summaries PortfolioSummaryProvider
	ranks     CompetitionRankProvider
	current   CurrentCompetitionProvider // optional
}

// NewService wires an achievements Service.
func NewService(repo AchievementRepository, positions PositionProvider, summaries PortfolioSummaryProvider, ranks CompetitionRankProvider) *Service {
	return &Service{repo: repo, positions: positions, summaries: summaries, ranks: ranks}
}

// SetCurrentCompetitionProvider attaches an optional provider so EvaluateAll can
// re-check sprint achievements against the active sprint.
func (s *Service) SetCurrentCompetitionProvider(c CurrentCompetitionProvider) {
	s.current = c
}

// EvaluateAll re-evaluates every achievement for the user and returns the
// updated list. Portfolio badges are always checked; sprint badges are checked
// against the current sprint when a CurrentCompetitionProvider is configured.
func (s *Service) EvaluateAll(ctx context.Context, userID string) ([]AchievementResponse, error) {
	_ = s.EvaluatePortfolioAchievements(ctx, userID)
	if s.current != nil {
		if cid := s.current.CurrentCompetitionID(ctx); cid != "" {
			if rank, err := s.ranks.GetUserRank(ctx, cid, userID); err == nil && rank >= 1 {
				s.unlock(ctx, userID, KeyFirstSprint) // ranked ⇒ joined
				if rank <= 10 {
					s.unlock(ctx, userID, KeyTop10Sprint)
				}
			}
		}
	}
	return s.ListAchievementsForUser(ctx, userID)
}

// ListAchievementsForUser returns every badge with the user's unlock state.
func (s *Service) ListAchievementsForUser(ctx context.Context, userID string) ([]AchievementResponse, error) {
	defs, err := s.repo.ListAchievements(ctx)
	if err != nil {
		return nil, err
	}
	userAch, err := s.repo.ListUserAchievements(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Build an achievementID -> unlockedAt lookup.
	unlockedAt := make(map[string]time.Time, len(userAch))
	for _, ua := range userAch {
		unlockedAt[ua.AchievementID] = ua.UnlockedAt
	}

	out := make([]AchievementResponse, 0, len(defs))
	for _, a := range defs {
		resp := AchievementResponse{
			Key:         a.Key,
			Name:        a.Name,
			Description: a.Description,
			IconKey:     a.IconKey,
		}
		if at, ok := unlockedAt[a.ID]; ok {
			at := at // copy for a stable pointer
			resp.Unlocked = true
			resp.UnlockedAt = &at
		}
		out = append(out, resp)
	}
	return out, nil
}

// EvaluatePortfolioAchievements unlocks first_portfolio (>=1 position),
// green_portfolio (gain% > 0), and index_110 (index >= 110) as applicable.
func (s *Service) EvaluatePortfolioAchievements(ctx context.Context, userID string) error {
	if positions, err := s.positions.ListPositions(ctx, userID); err == nil && len(positions) > 0 {
		s.unlock(ctx, userID, KeyFirstPortfolio)
	}
	if summary, err := s.summaries.GetSummary(ctx, userID); err == nil && summary != nil {
		if summary.GainLossPercentage > 0 {
			s.unlock(ctx, userID, KeyGreenPortfolio)
		}
		if summary.PortfolioIndex >= 110 {
			s.unlock(ctx, userID, KeyIndex110)
		}
	}
	return nil
}

// EvaluateSprintJoinAchievements unlocks first_sprint.
func (s *Service) EvaluateSprintJoinAchievements(ctx context.Context, userID string) error {
	s.unlock(ctx, userID, KeyFirstSprint)
	return nil
}

// EvaluateSprintRankAchievements unlocks top_10_sprint when the user's rank is
// between 1 and 10 inclusive (rank 0 means not ranked).
func (s *Service) EvaluateSprintRankAchievements(ctx context.Context, userID, competitionID string) error {
	rank, err := s.ranks.GetUserRank(ctx, competitionID, userID)
	if err != nil {
		return nil // best-effort
	}
	if rank >= 1 && rank <= 10 {
		s.unlock(ctx, userID, KeyTop10Sprint)
	}
	return nil
}

// unlock resolves a key to its achievement and unlocks it (idempotent).
func (s *Service) unlock(ctx context.Context, userID, key string) {
	ach, err := s.repo.GetAchievementByKey(ctx, key)
	if err != nil || ach == nil {
		return
	}
	_ = s.repo.UnlockAchievement(ctx, userID, ach.ID)
}
