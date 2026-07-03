package strategy

import (
	"context"
	"errors"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/profile"
)

type ProfileProvider interface {
	PublicStrategyByHandle(ctx context.Context, handle string) (profile.PublicStrategy, error)
}

type PortfolioReplacer interface {
	ReplaceWithStrategyWeights(ctx context.Context, userID string, weights []portfolio.StrategyWeightInput) error
}

type Service struct {
	profiles  ProfileProvider
	portfolio PortfolioReplacer
}

func NewService(profiles ProfileProvider, portfolio PortfolioReplacer) *Service {
	return &Service{profiles: profiles, portfolio: portfolio}
}

func (s *Service) CopyPreview(ctx context.Context, callerID, handle string) (CopyPreviewResponse, error) {
	source, err := s.loadSource(ctx, handle)
	if err != nil {
		return CopyPreviewResponse{}, err
	}
	if source.UserID == callerID {
		return CopyPreviewResponse{}, ErrSelfCopy
	}
	return CopyPreviewResponse{
		SourceProfile: sourceProfile(source),
		Weights:       weightsOf(source.Weights),
		Disclaimer:    CopyDisclaimer,
	}, nil
}

func (s *Service) CopyFromProfile(ctx context.Context, callerID string, req CopyFromProfileRequest) (CopyFromProfileResponse, error) {
	source, err := s.loadSource(ctx, req.Handle)
	if err != nil {
		return CopyFromProfileResponse{}, err
	}
	if source.UserID == callerID {
		return CopyFromProfileResponse{}, ErrSelfCopy
	}
	weights := req.Weights
	if len(weights) == 0 {
		weights = weightsOf(source.Weights)
	}
	if err := s.portfolio.ReplaceWithStrategyWeights(ctx, callerID, toPortfolioWeights(weights)); err != nil {
		return CopyFromProfileResponse{}, err
	}
	return CopyFromProfileResponse{
		SourceProfile: sourceProfile(source),
		Weights:       weights,
		Disclaimer:    CopyDisclaimer,
	}, nil
}

func (s *Service) loadSource(ctx context.Context, handle string) (profile.PublicStrategy, error) {
	source, err := s.profiles.PublicStrategyByHandle(ctx, strings.TrimSpace(handle))
	if errors.Is(err, profile.ErrNotFound) {
		return profile.PublicStrategy{}, ErrNotFound
	}
	if err != nil {
		return profile.PublicStrategy{}, err
	}
	return source, nil
}

func sourceProfile(source profile.PublicStrategy) SourceProfile {
	return SourceProfile{
		Handle:      source.Handle,
		DisplayName: source.DisplayName,
		AvatarKey:   source.AvatarKey,
		StrategyTag: source.StrategyTag,
	}
}

func weightsOf(weights []profile.StrategyWeight) []Weight {
	out := make([]Weight, 0, len(weights))
	for _, w := range weights {
		out = append(out, Weight{
			Symbol:           w.Symbol,
			AssetType:        w.AssetType,
			WeightPercentage: w.WeightPercentage,
		})
	}
	return out
}
