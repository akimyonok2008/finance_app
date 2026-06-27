package profile

import (
	"context"
	"math"
	"sort"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

func (s *Service) publicProjection(ctx context.Context, p Profile) PublicProfile {
	out := PublicProfile{
		Handle:            p.Handle,
		DisplayName:       p.DisplayName,
		AvatarKey:         p.AvatarKey,
		Bio:               p.Bio,
		StrategyTag:       p.StrategyTag,
		JoinedAt:          p.CreatedAt,
		Badges:            []PublicBadge{},
		PublicWeights:     []PublicWeight{},
		AssetTypeExposure: []Exposure{},
		CurrencyExposure:  []Exposure{},
	}

	// The portfolio summary is baseline-locked: every position's cost basis is
	// the market price at add time, so index/return here are ranked, fair
	// since-baseline performance — never historical buy-price gains.
	if summary, err := s.summaries.GetSummary(ctx, p.UserID); err == nil && summary != nil {
		out.PortfolioIndex = round2(summary.PortfolioIndex)
		out.ReturnPercentage = round2(summary.GainLossPercentage)
		out.PublicWeights, out.AssetTypeExposure, out.CurrencyExposure, out.Concentration = buildComposition(summary)
		if !p.ShowPublicWeights {
			out.PublicWeights = []PublicWeight{}
		}
	}

	if s.achievements != nil {
		if list, err := s.achievements.ListAchievementsForUser(ctx, p.UserID); err == nil {
			for _, badge := range list {
				if badge.Unlocked {
					out.Badges = append(out.Badges, PublicBadge{
						Key: badge.Key, Name: badge.Name, Icon: badge.IconKey, UnlockedAt: badge.UnlockedAt,
					})
				}
			}
		}
	}
	if s.sprintRanks != nil {
		competitionID := s.sprintRanks.CurrentCompetitionID(ctx)
		if competitionID != "" {
			if rank, err := s.sprintRanks.GetUserRank(ctx, competitionID, p.UserID); err == nil && rank > 0 {
				out.SprintRank = &rank
			}
		}
	}
	if s.globalRanks != nil {
		if rank, err := s.globalRanks.GetUserRank(ctx, p.UserID); err == nil && rank > 0 {
			out.GlobalRank = &rank
		}
	}
	return out
}

func buildComposition(summary *portfolio.PortfolioSummary) ([]PublicWeight, []Exposure, []Exposure, Concentration) {
	weights := []PublicWeight{}
	if summary == nil || summary.CurrentValue <= 0 {
		return weights, []Exposure{}, []Exposure{}, Concentration{}
	}

	assetTypes := map[string]float64{}
	currencies := map[string]float64{}
	for _, position := range summary.Positions {
		weight := round2(position.CurrentValueBase / summary.CurrentValue * 100)
		weights = append(weights, PublicWeight{
			Symbol: position.Symbol, AssetType: position.AssetType, Weight: weight,
		})
		assetTypes[position.AssetType] += position.CurrentValueBase
		currencies[position.CurrentPriceCurrency] += position.CurrentValueBase
	}
	sort.Slice(weights, func(i, j int) bool {
		if weights[i].Weight == weights[j].Weight {
			return weights[i].Symbol < weights[j].Symbol
		}
		return weights[i].Weight > weights[j].Weight
	})

	concentration := Concentration{}
	if len(weights) > 0 {
		concentration.LargestPosition = weights[0].Weight
	}
	for i := 0; i < len(weights) && i < 3; i++ {
		concentration.TopThree += weights[i].Weight
	}
	concentration.TopThree = round2(concentration.TopThree)

	return weights, exposureList(assetTypes, summary.CurrentValue), exposureList(currencies, summary.CurrentValue), concentration
}

func exposureList(values map[string]float64, total float64) []Exposure {
	out := make([]Exposure, 0, len(values))
	for name, value := range values {
		out = append(out, Exposure{Name: name, Weight: round2(value / total * 100)})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weight == out[j].Weight {
			return out[i].Name < out[j].Name
		}
		return out[i].Weight > out[j].Weight
	})
	return out
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
