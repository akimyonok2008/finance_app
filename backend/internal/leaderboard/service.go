package leaderboard

import (
	"context"
	"fmt"
	"log"
	"sort"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// UserProvider enumerates the users to rank. Implemented by *auth.Service.
type UserProvider interface {
	ListUsers(ctx context.Context) ([]auth.User, error)
}

// PortfolioSummaryProvider computes a single user's portfolio summary.
// Implemented by *portfolio.Service (its Summary method).
type PortfolioSummaryProvider interface {
	Summary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
}

// maxLeaderboardSize caps how many entries are served from the cache path.
const maxLeaderboardSize = 100

// Service builds the anonymous leaderboard. With a cache attached it serves the
// precomputed Redis ranking; otherwise (or when the cache is empty or failing)
// it falls back to live calculation, so the cache is never a single point of
// failure.
type Service struct {
	users      UserProvider
	portfolios PortfolioSummaryProvider
	cache      LeaderboardCache // optional
}

// NewService wires a leaderboard Service.
func NewService(users UserProvider, portfolios PortfolioSummaryProvider) *Service {
	return &Service{users: users, portfolios: portfolios}
}

// SetCache attaches an optional ranking cache (Redis in production).
func (s *Service) SetCache(cache LeaderboardCache) {
	s.cache = cache
}

// Build computes the ranked, privacy-safe leaderboard:
//
//	list users -> summarize each -> keep only percentage + index ->
//	sort by gain_loss_percentage desc (ties: display_name asc) -> assign ranks.
//
// Failure policy (prototype): if one user's summary fails, that user is skipped
// and the leaderboard is still built from the rest. Later we may surface
// partial-error metadata (which users were skipped, and why) for internal
// monitoring — for now a failed user is silently omitted.
func (s *Service) Build(ctx context.Context) ([]LeaderboardEntry, error) {
	// Fast path: serve from the cache when it has data. Any cache problem falls
	// through to live calculation — the cache is an optimization, not the truth.
	if s.cache != nil {
		if entries, ok := s.buildFromCache(ctx); ok {
			return entries, nil
		}
	}
	res, err := s.BuildResult(ctx)
	return res.Entries, err
}

// buildFromCache assembles the board from cached scores plus user metadata.
// ok=false means "use the live path" (cache empty, unavailable, or metadata
// lookup failed).
func (s *Service) buildFromCache(ctx context.Context) ([]LeaderboardEntry, bool) {
	scores, err := s.cache.GetGlobalTop(ctx, maxLeaderboardSize)
	if err != nil {
		log.Printf("leaderboard: cache unavailable, falling back to live calculation: %v", err)
		return nil, false
	}
	if len(scores) == 0 {
		return nil, false
	}
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return nil, false
	}
	byID := make(map[string]auth.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	entries := make([]LeaderboardEntry, 0, len(scores))
	for _, sc := range scores {
		u, ok := byID[sc.UserID]
		if !ok {
			continue // user deleted since the cache was refreshed
		}
		entries = append(entries, LeaderboardEntry{
			Rank:               len(entries) + 1,
			DisplayName:        u.DisplayName,
			AvatarKey:          u.AvatarKey,
			GainLossPercentage: sc.Score,
			// portfolio_index = 100 + gain% holds exactly for our formulas.
			PortfolioIndex: 100 + sc.Score,
		})
	}
	return entries, len(entries) > 0
}

// RefreshCache recomputes every user's live performance and upserts the scores
// into the cache. Called by the background worker. Returns the skipped count.
func (s *Service) RefreshCache(ctx context.Context) (int, error) {
	if s.cache == nil {
		return 0, nil
	}
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrListUsers, err)
	}
	skipped := 0
	for _, u := range users {
		summary, err := s.portfolios.Summary(ctx, u.ID)
		if err != nil || summary == nil {
			skipped++
			continue
		}
		if err := s.cache.UpsertGlobalScore(ctx, u.ID, summary.GainLossPercentage); err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}

// Result carries the ranked entries plus internal metadata about how many users
// were skipped. The public handler returns only Entries; SkippedCount is for
// internal monitoring (and, later, an admin endpoint).
type Result struct {
	Entries      []LeaderboardEntry
	SkippedCount int
}

// BuildResult is Build with skip metadata. With symbol validation in place,
// SkippedCount should normally be 0; a non-zero value indicates a real systemic
// problem worth investigating.
func (s *Service) BuildResult(ctx context.Context) (Result, error) {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrListUsers, err)
	}

	entries := make([]LeaderboardEntry, 0, len(users))
	skipped := 0
	for _, u := range users {
		summary, err := s.portfolios.Summary(ctx, u.ID)
		if err != nil || summary == nil {
			skipped++
			log.Printf("leaderboard: skipping user %s due to summary error: %v", u.ID, err)
			continue // skip this user; do not fail the whole board
		}
		entries = append(entries, LeaderboardEntry{
			DisplayName:        u.DisplayName,
			AvatarKey:          u.AvatarKey,
			GainLossPercentage: summary.GainLossPercentage,
			PortfolioIndex:     summary.PortfolioIndex,
		})
	}

	// Sort by performance desc, breaking ties by display name asc for a
	// deterministic order.
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].GainLossPercentage != entries[j].GainLossPercentage {
			return entries[i].GainLossPercentage > entries[j].GainLossPercentage
		}
		return entries[i].DisplayName < entries[j].DisplayName
	})

	// Simple sequential ranks (1, 2, 3, ...). Olympic ranking is out of scope.
	for i := range entries {
		entries[i].Rank = i + 1
	}

	return Result{Entries: entries, SkippedCount: skipped}, nil
}
