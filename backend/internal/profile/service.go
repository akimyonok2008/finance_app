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
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type UserProvider interface {
	GetUserByID(ctx context.Context, userID string) (*auth.User, error)
}

type SummaryProvider interface {
	GetSummary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
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

type Service struct {
	repo         Repository
	users        UserProvider
	summaries    SummaryProvider
	achievements AchievementProvider
	sprintRanks  SprintRankProvider
	globalRanks  GlobalRankProvider
	now          func() time.Time
}

func NewService(repo Repository, users UserProvider, summaries SummaryProvider) *Service {
	return &Service{
		repo:      repo,
		users:     users,
		summaries: summaries,
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
		if summary, err := s.summaries.GetSummary(ctx, userID); err == nil && summary != nil {
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

func (s *Service) GetPublic(ctx context.Context, handle string) (PublicProfile, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	p, err := s.repo.GetByHandle(ctx, handle)
	if errors.Is(err, ErrNotFound) || !p.IsPublic {
		return PublicProfile{}, ErrNotFound
	}
	if err != nil {
		return PublicProfile{}, err
	}
	return s.publicProjection(ctx, p), nil
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
