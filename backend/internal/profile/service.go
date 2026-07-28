package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/dna"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type UserProvider interface {
	GetUserByID(ctx context.Context, userID string) (*auth.User, error)
}

type SummaryProvider interface {
	GetSummary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
}

// PublicWeightsProvider supplies just enough valuation data to compute a
// public allocation breakdown (open positions + cash), without the full
// ledger-scan economics SummaryProvider carries. It exists so enriching a
// leaderboard row with public weights doesn't pay for income/fee/realized-P&L
// reconstruction it never uses.
type PublicWeightsProvider interface {
	GetPublicWeights(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
}

type AchievementProvider interface {
	ListAchievementsForUser(ctx context.Context, userID string) ([]achievements.AchievementResponse, error)
}

type SprintRankProvider interface {
	CurrentCompetitionID(ctx context.Context) string
	GetUserRank(ctx context.Context, competitionID, userID string) (int, error)
}

type GlobalRankProvider interface {
	GetUserRank(ctx context.Context, userID string) (int, error)
}

type TimeframeRanking struct {
	UserID                 string
	Rank                   int
	RankedReturnPercentage float64
	RankedIndex            float64
}

type TimeframeRankProvider interface {
	UserRankings(ctx context.Context, timeframe string) ([]TimeframeRanking, error)
}

type PerformanceHistoryProvider interface {
	RankedHistory(ctx context.Context, userID string, start, end time.Time) ([]PublicPerformancePoint, error)
}

// RankedPerformance is the persistent ranked standing surfaced on public
// profiles. It is privacy-safe: index and return percentage only.
type RankedPerformance struct {
	RankedIndex            float64
	RankedReturnPercentage float64
	Paused                 bool
}

// RankedPerformanceProvider supplies a user's persistent ranked performance so
// public profiles show the trusted ranked index rather than a mutable
// current-basket figure. Optional; when unset the summary index is used.
type RankedPerformanceProvider interface {
	CurrentRankedPerformance(ctx context.Context, userID string) (RankedPerformance, error)
}

// BlockedPairSource exposes the caller's block-set so public profile view
// and Explore can filter blocked pairs at the service level — never fetched
// unfiltered and hidden only in a handler or the browser.
type BlockedPairSource interface {
	BlockedPairUserIDs(ctx context.Context, userID string) (map[string]bool, error)
}

type Service struct {
	repo             Repository
	users            UserProvider
	summaries        SummaryProvider
	exploreSummaries SummaryProvider // optional; defaults to summaries
	weights          PublicWeightsProvider
	achievements     AchievementProvider
	sprintRanks      SprintRankProvider
	globalRanks      GlobalRankProvider
	timeframeRanks   TimeframeRankProvider
	history          PerformanceHistoryProvider
	ranked           RankedPerformanceProvider
	blocked          BlockedPairSource
	dna              *dna.Service
	now              func() time.Time
}

// SetBlockedFilter attaches the block-set source used to keep blocked pairs
// out of public profile views and Explore results.
func (s *Service) SetBlockedFilter(b BlockedPairSource) { s.blocked = b }

// blockedSet returns the caller's block-set. It fails closed: a safety-store
// error is returned to the caller rather than silently treated as "no
// blocks", since that would leak blocked users into public profile views and
// Explore.
func (s *Service) blockedSet(ctx context.Context, userID string) (map[string]bool, error) {
	if s.blocked == nil || userID == "" {
		return nil, nil
	}
	set, err := s.blocked.BlockedPairUserIDs(ctx, userID)
	if err != nil {
		return nil, ErrSafetyUnavailable
	}
	return set, nil
}

// SetRankedPerformanceProvider attaches the ranked-performance source used for
// public profile performance.
func (s *Service) SetRankedPerformanceProvider(p RankedPerformanceProvider) { s.ranked = p }

// SetPublicWeightsProvider attaches the cheap weights-only valuation source
// used to enrich leaderboard rows. Without it, PublicInfoForUser falls back
// to the full SummaryProvider (correct, just more expensive).
func (s *Service) SetPublicWeightsProvider(p PublicWeightsProvider) { s.weights = p }

// SetExploreSummaryProvider attaches a distinct SummaryProvider used only by
// Explore's buildSimilar (the caller's own composition/DNA comparison). It
// exists so a caching decorator can sit in front of THAT one call site
// without also caching the owner's own profile preview (GetMe) or a public
// profile page view (GetPublic) — both of which callers reasonably expect to
// reflect their own just-made changes immediately. Unset means "use the same
// summaries provider everywhere", the previous behavior.
func (s *Service) SetExploreSummaryProvider(p SummaryProvider) { s.exploreSummaries = p }

// exploreSummaryProvider returns the dedicated Explore provider if set,
// falling back to the shared one.
func (s *Service) exploreSummaryProvider() SummaryProvider {
	if s.exploreSummaries != nil {
		return s.exploreSummaries
	}
	return s.summaries
}

func NewService(repo Repository, users UserProvider, summaries SummaryProvider) *Service {
	return &Service{
		repo:      repo,
		users:     users,
		summaries: summaries,
		dna:       dna.NewService(),
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// LeaderboardInfo is the public profile data the leaderboard joins onto its
// rows. Weights is populated only when the profile is public AND show weights.
type LeaderboardInfo struct {
	Handle      string
	StrategyTag string
	IsPublic    bool
	ShowWeights bool
	Weights     []PublicWeight
}

// PublicInfoForUser returns a user's public profile data for leaderboard
// enrichment. hasProfile=false means the user has no profile row yet.
func (s *Service) PublicInfoForUser(ctx context.Context, userID string) (LeaderboardInfo, bool, error) {
	p, err := s.repo.GetByUserID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return LeaderboardInfo{}, false, nil
	}
	if err != nil {
		return LeaderboardInfo{}, false, err
	}
	info := LeaderboardInfo{
		Handle:      p.Handle,
		StrategyTag: p.StrategyTag,
		IsPublic:    p.IsPublic,
		ShowWeights: p.ShowPublicWeights,
	}
	if p.IsPublic && p.ShowPublicWeights {
		if s.weights != nil {
			if summary, err := s.weights.GetPublicWeights(ctx, userID); err == nil && summary != nil {
				info.Weights, _, _, _ = buildComposition(summary)
			}
		} else if summary, err := s.summaries.GetSummary(ctx, userID); err == nil && summary != nil {
			info.Weights, _, _, _ = buildComposition(summary)
		}
	}
	return info, true, nil
}

func (s *Service) SetAchievementProvider(provider AchievementProvider) {
	s.achievements = provider
}

func (s *Service) SetSprintRankProvider(provider SprintRankProvider) {
	s.sprintRanks = provider
}

func (s *Service) SetGlobalRankProvider(provider GlobalRankProvider) {
	s.globalRanks = provider
}

func (s *Service) SetTimeframeRankProvider(provider TimeframeRankProvider) {
	s.timeframeRanks = provider
}

func (s *Service) SetPerformanceHistoryProvider(provider PerformanceHistoryProvider) {
	s.history = provider
}

func (s *Service) GetMe(ctx context.Context, userID string) (OwnerProfile, error) {
	p, err := s.getOrCreate(ctx, userID)
	if err != nil {
		return OwnerProfile{}, err
	}
	return s.ownerProjection(ctx, p), nil
}

func (s *Service) UpdateMe(ctx context.Context, userID string, input UpdateInput) (OwnerProfile, error) {
	p, err := s.getOrCreate(ctx, userID)
	if err != nil {
		return OwnerProfile{}, err
	}

	input = NormalizeInput(input)
	if input.Handle != nil {
		p.Handle = *input.Handle
	}
	if input.DisplayName != nil {
		p.DisplayName = *input.DisplayName
	}
	if input.AvatarKey != nil {
		p.AvatarKey = *input.AvatarKey
	}
	if input.Bio != nil {
		p.Bio = *input.Bio
	}
	if input.StrategyTag != nil {
		p.StrategyTag = *input.StrategyTag
	}
	if input.IsPublic != nil {
		p.IsPublic = *input.IsPublic
	}
	if input.ShowPublicWeights != nil {
		p.ShowPublicWeights = *input.ShowPublicWeights
	}
	p.UpdatedAt = s.now()

	if err := ValidateProfile(p); err != nil {
		return OwnerProfile{}, err
	}
	if err := s.repo.Update(ctx, p); err != nil {
		return OwnerProfile{}, err
	}
	return s.ownerProjection(ctx, p), nil
}

// OnAccountDeleted implements auth.AccountDeletionHook: it unpublishes the
// user's profile so it stops resolving via Explore or a direct handle lookup
// (GetPublic/Explore read straight from the profile repository and never
// check whether the underlying account still exists). A user who never
// created a profile is a no-op, not an error.
func (s *Service) OnAccountDeleted(ctx context.Context, userID string) error {
	p, err := s.repo.GetByUserID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !p.IsPublic && !p.ShowPublicWeights {
		return nil
	}
	p.IsPublic = false
	p.ShowPublicWeights = false
	p.UpdatedAt = s.now()
	return s.repo.Update(ctx, p)
}

// GetPublic returns handle's public profile as seen by callerID (empty when
// unauthenticated). A block in either direction between caller and target
// returns the same ErrNotFound as a nonexistent handle — a privacy-safe
// "unavailable" result that never confirms a block exists.
func (s *Service) GetPublic(ctx context.Context, callerID, handle string) (PublicProfile, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	p, err := s.repo.GetByHandle(ctx, handle)
	if errors.Is(err, ErrNotFound) || !p.IsPublic {
		return PublicProfile{}, ErrNotFound
	}
	if err != nil {
		return PublicProfile{}, err
	}
	if callerID != "" && callerID != p.UserID {
		blocked, err := s.blockedSet(ctx, callerID)
		if err != nil {
			return PublicProfile{}, err
		}
		if blocked[p.UserID] {
			return PublicProfile{}, ErrNotFound
		}
	}
	return s.publicProjection(ctx, p), nil
}

// PublicStrategyByHandle returns the safe public strategy weights used by copy
// and compare flows. Private profiles, hidden weights, missing/empty baselines,
// and summary failures all collapse to ErrNotFound so callers never leak which
// condition applied.
func (s *Service) PublicStrategyByHandle(ctx context.Context, handle string) (PublicStrategy, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	p, err := s.repo.GetByHandle(ctx, handle)
	if errors.Is(err, ErrNotFound) || !p.IsPublic || !p.ShowPublicWeights {
		return PublicStrategy{}, ErrNotFound
	}
	if err != nil {
		return PublicStrategy{}, err
	}
	summary, err := s.summaries.GetSummary(ctx, p.UserID)
	if err != nil || summary == nil || len(summary.Positions) == 0 {
		return PublicStrategy{}, ErrNotFound
	}
	weights, _, _, concentration := buildComposition(summary)
	if len(weights) == 0 {
		return PublicStrategy{}, ErrNotFound
	}
	out := PublicStrategy{
		UserID:        p.UserID,
		Handle:        p.Handle,
		DisplayName:   p.DisplayName,
		AvatarKey:     p.AvatarKey,
		StrategyTag:   p.StrategyTag,
		Concentration: concentration,
		Weights:       make([]StrategyWeight, 0, len(weights)),
	}
	for _, w := range weights {
		out.Weights = append(out.Weights, StrategyWeight{
			Symbol:           w.Symbol,
			AssetType:        w.AssetType,
			WeightPercentage: w.Weight,
		})
	}
	return out, nil
}

func (s *Service) getOrCreate(ctx context.Context, userID string) (Profile, error) {
	p, err := s.repo.GetByUserID(ctx, userID)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Profile{}, err
	}

	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return Profile{}, fmt.Errorf("load account: %w", err)
	}
	if user == nil {
		return Profile{}, errors.New("load account: user not found")
	}
	now := s.now()
	p = Profile{
		UserID:            user.ID,
		Handle:            HandleCandidate(user.DisplayName, user.ID),
		DisplayName:       truncate(strings.TrimSpace(user.DisplayName), 40),
		AvatarKey:         truncate(strings.TrimSpace(user.AvatarKey), 40),
		StrategyTag:       DefaultStrategyTag,
		IsPublic:          false,
		ShowPublicWeights: false,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if utf8.RuneCountInString(p.DisplayName) < 2 {
		p.DisplayName = "Investor"
	}
	if err := ValidateProfile(p); err != nil {
		return Profile{}, err
	}
	for attempt := 0; attempt < 100; attempt++ {
		if attempt > 0 {
			p.Handle = HandleWithSuffix(HandleCandidate(user.DisplayName, user.ID), attempt)
		}
		if err := s.repo.Create(ctx, p); err == nil {
			return p, nil
		} else if !errors.Is(err, ErrHandleExists) {
			return Profile{}, err
		}
		if existing, err := s.repo.GetByUserID(ctx, userID); err == nil {
			return existing, nil
		}
	}
	return Profile{}, errors.New("could not allocate unique profile handle")
}

func (s *Service) ownerProjection(ctx context.Context, p Profile) OwnerProfile {
	return OwnerProfile{
		Handle:            p.Handle,
		DisplayName:       p.DisplayName,
		AvatarKey:         p.AvatarKey,
		Bio:               p.Bio,
		StrategyTag:       p.StrategyTag,
		IsPublic:          p.IsPublic,
		ShowPublicWeights: p.ShowPublicWeights,
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
		PublicPreview:     s.publicProjection(ctx, p),
	}
}
