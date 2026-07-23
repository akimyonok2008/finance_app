package strategy

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/profile"
)

type ProfileProvider interface {
	PublicStrategyByHandle(ctx context.Context, handle string) (profile.PublicStrategy, error)
}

type PortfolioReplacer interface {
	ReplaceWithStrategyWeights(ctx context.Context, userID string, weights []portfolio.StrategyWeightInput) error
	Summary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
}

func (s *Service) CompareProfile(ctx context.Context, userID, handle string) (CompareProfileResponse, error) {
	target, err := s.loadSource(ctx, handle)
	if err != nil {
		return CompareProfileResponse{}, err
	}
	summary, err := s.portfolio.Summary(ctx, userID)
	if err != nil {
		return CompareProfileResponse{}, err
	}
	if summary == nil || len(summary.Positions) == 0 || summary.CurrentValue <= 0 {
		return CompareProfileResponse{}, ErrEmptyPortfolio
	}

	myWeights := make(map[string]WeightDifference)
	mySorted := make([]WeightDifference, 0, len(summary.Positions))
	for _, position := range summary.Positions {
		row := WeightDifference{
			Symbol:             position.Symbol,
			MyWeightPercentage: round2(position.CurrentValueBase / summary.CurrentValue * 100),
			AssetType:          position.AssetType,
		}
		myWeights[position.Symbol] = row
		mySorted = append(mySorted, row)
	}
	sort.SliceStable(mySorted, func(i, j int) bool {
		if mySorted[i].MyWeightPercentage != mySorted[j].MyWeightPercentage {
			return mySorted[i].MyWeightPercentage > mySorted[j].MyWeightPercentage
		}
		return mySorted[i].Symbol < mySorted[j].Symbol
	})
	myTop3 := 0.0
	for i := 0; i < len(mySorted) && i < 3; i++ {
		myTop3 += mySorted[i].MyWeightPercentage
	}

	targetWeights := make(map[string]profile.StrategyWeight)
	targetSorted := append([]profile.StrategyWeight(nil), target.Weights...)
	sort.SliceStable(targetSorted, func(i, j int) bool {
		if targetSorted[i].WeightPercentage != targetSorted[j].WeightPercentage {
			return targetSorted[i].WeightPercentage > targetSorted[j].WeightPercentage
		}
		return targetSorted[i].Symbol < targetSorted[j].Symbol
	})
	targetTop3 := 0.0
	for i, weight := range targetSorted {
		targetWeights[weight.Symbol] = weight
		if i < 3 {
			targetTop3 += weight.WeightPercentage
		}
	}

	seen := make(map[string]bool)
	differences := make([]WeightDifference, 0, len(myWeights)+len(targetWeights))
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
		differences = append(differences, mine)
		seen[symbol] = true
	}
	for symbol, targetWeight := range targetWeights {
		if seen[symbol] {
			continue
		}
		differences = append(differences, WeightDifference{
			Symbol:                 symbol,
			TargetWeightPercentage: targetWeight.WeightPercentage,
			DifferencePercentage:   round2(-targetWeight.WeightPercentage),
			AssetType:              targetWeight.AssetType,
		})
	}
	sort.Strings(shared)
	sort.SliceStable(differences, func(i, j int) bool {
		left, right := math.Abs(differences[i].DifferencePercentage), math.Abs(differences[j].DifferencePercentage)
		if left != right {
			return left > right
		}
		return differences[i].Symbol < differences[j].Symbol
	})
	if len(differences) > 10 {
		differences = differences[:10]
	}

	concentration := ConcentrationComparison{
		MyPositionCount:            len(summary.Positions),
		TargetPositionCount:        len(target.Weights),
		MyTop3WeightPercentage:     round2(myTop3),
		TargetTop3WeightPercentage: round2(targetTop3),
	}
	return CompareProfileResponse{
		TargetProfile:           sourceProfile(target),
		Summary:                 fmt.Sprintf("Your strategy overlaps %.1f%% with %s.", round2(overlap), target.DisplayName),
		OverlapScore:            round2(overlap),
		SharedSymbols:           shared,
		WeightDifferences:       differences,
		ConcentrationComparison: concentration,
		LearningPoints:          compareLearningPoints(concentration, overlap),
		Disclaimer:              "This is educational comparison, not investment advice.",
	}, nil
}

func compareLearningPoints(c ConcentrationComparison, overlap float64) []LearningPoint {
	points := make([]LearningPoint, 0, 2)
	switch {
	case c.TargetTop3WeightPercentage > c.MyTop3WeightPercentage+10:
		points = append(points, LearningPoint{Title: "Concentration difference", Body: "The target strategy is more concentrated, so ranked performance may be more sensitive to a few holdings."})
	case c.MyTop3WeightPercentage > c.TargetTop3WeightPercentage+10:
		points = append(points, LearningPoint{Title: "Concentration difference", Body: "Your strategy is more concentrated than the target, so fewer holdings may drive more of your ranked movement."})
	default:
		points = append(points, LearningPoint{Title: "Similar concentration", Body: "Both strategies have a similar top-three concentration profile."})
	}
	if overlap == 0 {
		points = append(points, LearningPoint{Title: "No shared symbols", Body: "The strategies use different symbols, so overlap is zero even if risk levels look similar."})
	}
	return points
}

func round2(value float64) float64 { return math.Round(value*100) / 100 }

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
