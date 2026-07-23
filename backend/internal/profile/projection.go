package profile

import (
	"context"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/dna"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

func (s *Service) publicProjection(ctx context.Context, p Profile) PublicProfile {
	out := PublicProfile{
		Handle:                p.Handle,
		DisplayName:           p.DisplayName,
		AvatarKey:             p.AvatarKey,
		Bio:                   p.Bio,
		StrategyTag:           p.StrategyTag,
		JoinedAt:              p.CreatedAt,
		Badges:                []PublicBadge{},
		PerformanceHistory:    []PublicPerformancePoint{},
		PublicClosedPositions: []PublicClosedPosition{},
		PublicWeights:         []PublicWeight{},
		AssetTypeExposure:     []Exposure{},
		CurrencyExposure:      []Exposure{},
		Insights:              s.buildProfileInsights(ctx, nil, p.ShowPublicWeights),
	}

	hasSummary := false

	// Public performance is the PERSISTENT ranked index (the single trusted source
	// of ranked performance), never the mutable current-basket figure. Public
	// composition (weights/exposure) still reflects the live portfolio where the
	// user opted in.
	if summary, err := s.summaries.GetSummary(ctx, p.UserID); err == nil && summary != nil {
		hasSummary = true
		out.PortfolioIndex = round2(summary.PortfolioIndex)
		out.ReturnPercentage = round2(summary.GainLossPercentage)
		if s.ranked != nil {
			if rp, err := s.ranked.CurrentRankedPerformance(ctx, p.UserID); err == nil {
				out.PortfolioIndex = round2(rp.RankedIndex)
				out.ReturnPercentage = round2(rp.RankedReturnPercentage)
			}
		}
		out.PublicWeights, out.AssetTypeExposure, out.CurrencyExposure, out.Concentration = buildComposition(summary)
		out.Insights = s.buildProfileInsights(ctx, summary, p.ShowPublicWeights)
		if !p.ShowPublicWeights {
			out.PublicWeights = []PublicWeight{}
		} else {
			out.PublicClosedPositions = buildClosedPositions(summary.ClosedPositions)
		}
	}

	if s.history != nil {
		if history, err := s.history.Archives(ctx, p.UserID, portfolio.ArchiveTimeframe1Y); err == nil && history != nil {
			out.PerformanceHistory = publicPerformanceHistory(history.Points)
		}
	}
	if len(out.PerformanceHistory) == 0 && hasSummary {
		out.PerformanceHistory = append(out.PerformanceHistory, PublicPerformancePoint{
			CapturedAt:       s.now().Format(time.RFC3339),
			PortfolioIndex:   out.PortfolioIndex,
			ReturnPercentage: out.ReturnPercentage,
		})
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

type contributionSignal struct {
	symbol string
	points float64
}

func (s *Service) buildProfileInsights(ctx context.Context, summary *portfolio.PortfolioSummary, showComposition bool) ProfileInsights {
	insights := ProfileInsights{
		InvestmentStyle: "Profile warming up",
		StyleSummary:    "Portfolio DNA will appear after this investor adds enough positions.",
		FocusAreas:      []string{},
		DNA:             dna.PortfolioDNAScores{},
		Explanations:    dna.DNAExplanations{},
		PerformanceDrivers: ProfilePerformanceDrivers{
			Summary:         "Performance drivers will appear after this investor has enough portfolio activity.",
			PositiveDrivers: []string{},
			NegativeDrivers: []string{},
		},
		BenchmarkContext: ProfileBenchmarkContext{
			InvestorIndex: 100,
			Benchmarks:    []ProfileBenchmark{},
			Note:          "Benchmark index history is not connected yet.",
		},
		Contributors: []ProfileContributor{},
		Detractors:   []ProfileContributor{},
		OpenClosedPerformance: ProfileOpenClosedPerformance{
			CompositionVisible: showComposition,
		},
	}
	if summary == nil {
		return insights
	}

	result, err := s.dna.Calculate(ctx, dnaInputsFromSummary(summary))
	if err == nil && result.HasData {
		insights.DNA = result.Scores
		insights.Explanations = result.Explanations
		insights.FocusAreas = result.FocusAreas
		insights.InvestmentStyle = result.InvestmentStyle
		insights.StyleSummary = result.StyleSummary
	}
	insights.BenchmarkContext.InvestorIndex = round2(summary.PortfolioIndex)

	openPoints := contributionPoints(summary.UnrealizedGainLossBase, summary.TotalCostBasis)
	closedPoints := contributionPoints(summary.RealizedGainLossBase, summary.TotalCostBasis)
	openReturn := percentage(summary.UnrealizedGainLossBase, summary.ActiveCostBasisBase)
	closedReturn := percentage(summary.RealizedGainLossBase, summary.ClosedCostBasisBase)
	insights.OpenClosedPerformance = ProfileOpenClosedPerformance{
		OpenReturnPercentage:     round2(openReturn),
		ClosedReturnPercentage:   round2(closedReturn),
		OpenContributionPoints:   round2(openPoints),
		ClosedContributionPoints: round2(closedPoints),
		HasClosedPositions:       len(summary.ClosedPositions) > 0,
		CompositionVisible:       showComposition,
	}
	insights.PerformanceDrivers = buildPerformanceDrivers(summary, insights.FocusAreas, showComposition)
	if showComposition {
		insights.Contributors, insights.Detractors = buildContributors(summary)
	}
	return insights
}

// dnaInputsFromSummary converts a portfolio summary into privacy-safe DNA
// inputs. Weights are derived from current base-currency position values (owner
// side); only symbol, asset type, currency, and percentage weight cross into the
// scorer — no monetary values, quantities, or identifiers.
func dnaInputsFromSummary(summary *portfolio.PortfolioSummary) []dna.PositionDNAInput {
	if summary == nil || summary.CurrentValue <= 0 {
		return nil
	}
	inputs := make([]dna.PositionDNAInput, 0, len(summary.Positions))
	for _, position := range summary.Positions {
		if position.CurrentValueBase <= 0 {
			continue
		}
		inputs = append(inputs, dna.PositionDNAInput{
			Symbol:    position.Symbol,
			AssetType: position.AssetType,
			Currency:  position.CurrentPriceCurrency,
			Weight:    position.CurrentValueBase / summary.CurrentValue,
		})
	}
	return inputs
}

func buildPerformanceDrivers(summary *portfolio.PortfolioSummary, focusAreas []string, showComposition bool) ProfilePerformanceDrivers {
	openPoints := contributionPoints(summary.UnrealizedGainLossBase, summary.TotalCostBasis)
	closedPoints := contributionPoints(summary.RealizedGainLossBase, summary.TotalCostBasis)
	source := "open positions"
	if math.Abs(closedPoints) > math.Abs(openPoints) {
		source = "closed positions"
	}
	direction := "flat"
	if summary.GainLossPercentage > 0 {
		direction = "positive"
	} else if summary.GainLossPercentage < 0 {
		direction = "negative"
	}
	focusText := "the current allocation"
	if len(focusAreas) > 0 {
		focusText = strings.Join(focusAreas[:minInt(len(focusAreas), 3)], ", ")
	}
	summaryText := "Performance is currently " + direction + ", with most index movement coming from " + source + "."
	if showComposition && len(focusAreas) > 0 {
		summaryText = "Performance is currently " + direction + ", led by exposure to " + focusText + " and with most index movement coming from " + source + "."
	}

	positive := []string{}
	negative := []string{}
	if openPoints > 0 {
		positive = append(positive, "Open positions added "+signedPoints(openPoints))
	} else if openPoints < 0 {
		negative = append(negative, "Open positions reduced returns by "+points(math.Abs(openPoints)))
	}
	if closedPoints > 0 {
		positive = append(positive, "Closed positions added "+signedPoints(closedPoints))
	} else if closedPoints < 0 {
		negative = append(negative, "Closed positions reduced returns by "+points(math.Abs(closedPoints)))
	}
	if len(positive) == 0 {
		positive = append(positive, "No positive contribution is visible yet")
	}
	if len(negative) == 0 {
		negative = append(negative, "No material detractor is visible yet")
	}

	return ProfilePerformanceDrivers{
		Summary:                  summaryText,
		PositiveDrivers:          positive,
		NegativeDrivers:          negative,
		OpenContributionPoints:   round2(openPoints),
		ClosedContributionPoints: round2(closedPoints),
	}
}

func buildContributors(summary *portfolio.PortfolioSummary) ([]ProfileContributor, []ProfileContributor) {
	signals := make([]contributionSignal, 0, len(summary.Positions)+len(summary.ClosedPositions))
	for _, position := range summary.Positions {
		signals = append(signals, contributionSignal{
			symbol: position.Symbol,
			points: contributionPoints(position.GainLossBase, summary.TotalCostBasis),
		})
	}
	for _, position := range summary.ClosedPositions {
		signals = append(signals, contributionSignal{
			symbol: position.Symbol,
			points: contributionPoints(position.RealizedGainLossBase, summary.TotalCostBasis),
		})
	}
	contributors := []ProfileContributor{}
	detractors := []ProfileContributor{}
	for _, signal := range signals {
		if signal.points > 0 {
			contributors = append(contributors, ProfileContributor{Symbol: signal.symbol, ContributionPoints: round2(signal.points)})
		}
		if signal.points < 0 {
			detractors = append(detractors, ProfileContributor{Symbol: signal.symbol, ContributionPoints: round2(signal.points)})
		}
	}
	sort.Slice(contributors, func(i, j int) bool {
		if contributors[i].ContributionPoints == contributors[j].ContributionPoints {
			return contributors[i].Symbol < contributors[j].Symbol
		}
		return contributors[i].ContributionPoints > contributors[j].ContributionPoints
	})
	sort.Slice(detractors, func(i, j int) bool {
		if detractors[i].ContributionPoints == detractors[j].ContributionPoints {
			return detractors[i].Symbol < detractors[j].Symbol
		}
		return detractors[i].ContributionPoints < detractors[j].ContributionPoints
	})
	return contributors[:minInt(len(contributors), 5)], detractors[:minInt(len(detractors), 5)]
}

func contributionPoints(gainLossBase, totalCostBasis float64) float64 {
	if totalCostBasis == 0 {
		return 0
	}
	return gainLossBase / totalCostBasis * 100
}

func percentage(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator * 100
}

func signedPoints(value float64) string {
	if value >= 0 {
		return "+" + points(value)
	}
	return "-" + points(math.Abs(value))
}

func points(value float64) string {
	return strings.TrimRight(strings.TrimRight(formatFloat(round2(value)), "0"), ".") + " pts"
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func publicPerformanceHistory(points []portfolio.PortfolioArchivePoint) []PublicPerformancePoint {
	out := make([]PublicPerformancePoint, 0, len(points))
	for _, point := range points {
		out = append(out, PublicPerformancePoint{
			CapturedAt:       point.CapturedAt,
			PortfolioIndex:   round2(point.PortfolioIndex),
			ReturnPercentage: round2(point.GainLossPercentage),
		})
	}
	return out
}

func buildClosedPositions(positions []portfolio.ClosedPositionSummary) []PublicClosedPosition {
	out := make([]PublicClosedPosition, 0, len(positions))
	for _, position := range positions {
		out = append(out, PublicClosedPosition{
			Symbol:           position.Symbol,
			AssetType:        position.AssetType,
			ClosedAt:         position.ClosedAt,
			ReturnPercentage: round2(position.RealizedGainLossPercentage),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ClosedAt == out[j].ClosedAt {
			return out[i].Symbol < out[j].Symbol
		}
		return out[i].ClosedAt > out[j].ClosedAt
	})
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
