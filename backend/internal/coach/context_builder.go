package coach

import (
	"context"
	"math"
	"sort"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// buildUserFacts reduces a portfolio summary to safe, analysis-ready figures:
// weights, percentages, exposures, and concentration. No raw values are kept.
func buildUserFacts(summary *portfolio.PortfolioSummary) UserPortfolioFacts {
	totalValue := summary.CurrentValue
	totalCost := summary.TotalCostBasis

	holdings := make([]HoldingFact, 0, len(summary.Positions))
	assetExposure := map[string]float64{}
	currencyExposure := map[string]float64{}
	var largestWeight float64
	var largestSymbol string

	for _, p := range summary.Positions {
		var weight float64
		if totalValue > 0 {
			weight = round2(p.CurrentValueBase / totalValue * 100)
		}
		var contribution float64
		if totalCost > 0 {
			contribution = round2((p.CurrentValueBase - p.CostBasisBase) / totalCost * 100)
		}
		holdings = append(holdings, HoldingFact{
			Symbol:                       p.Symbol,
			AssetType:                    p.AssetType,
			WeightPercentage:             weight,
			GainLossPercentage:           round2(p.GainLossPercentage),
			ContributionPercentagePoints: contribution,
		})
		assetExposure[p.AssetType] += weight
		currencyExposure[p.Currency] += weight
		if weight > largestWeight {
			largestWeight = weight
			largestSymbol = p.Symbol
		}
	}

	for k, v := range assetExposure {
		assetExposure[k] = round2(v)
	}
	for k, v := range currencyExposure {
		currencyExposure[k] = round2(v)
	}

	return UserPortfolioFacts{
		PortfolioIndex:          round2(summary.PortfolioIndex),
		GainLossPercentage:      round2(summary.GainLossPercentage),
		PositionCount:           len(summary.Positions),
		LargestSymbol:           largestSymbol,
		LargestWeightPercentage: largestWeight,
		BaseCurrency:            summary.BaseCurrency,
		Holdings:                holdings,
		AssetTypeExposure:       assetExposure,
		CurrencyExposure:        currencyExposure,
		RiskLevel:               riskFromConcentration(largestWeight, len(summary.Positions)),
	}
}

// riskFromConcentration derives a deterministic risk band from the largest
// holding weight and the number of positions. A single-name portfolio is high
// risk regardless of weight maths.
func riskFromConcentration(largestWeight float64, positionCount int) string {
	switch {
	case positionCount <= 1:
		return "high"
	case largestWeight >= 50:
		return "high"
	case largestWeight >= 35:
		return "elevated"
	case largestWeight >= 20:
		return "moderate"
	default:
		return "low"
	}
}

// publicHoldings projects another user's summary to public holdings (symbol,
// weight, asset type only) and reports their largest weight and position count.
func publicHoldings(summary *portfolio.PortfolioSummary) (holdings []PublicHolding, largestWeight float64, positionCount int) {
	total := summary.CurrentValue
	holdings = make([]PublicHolding, 0, len(summary.Positions))
	for _, p := range summary.Positions {
		var weight float64
		if total > 0 {
			weight = round2(p.CurrentValueBase / total * 100)
		}
		holdings = append(holdings, PublicHolding{
			Symbol:           p.Symbol,
			WeightPercentage: weight,
			AssetType:        p.AssetType,
		})
		if weight > largestWeight {
			largestWeight = weight
		}
	}
	return holdings, largestWeight, len(summary.Positions)
}

// rankedPortfolio is an internal ranking row. The summary is used only to derive
// public weights; it never leaves this package.
type rankedPortfolio struct {
	user           auth.User
	publicHoldings []PublicHolding
	largestWeight  float64
	positionCount  int
	returnPct      float64
}

// buildTop10Facts ranks every user with positions by return, projects the top
// group to public-only data, and computes deterministic comparison aggregates
// against the requesting user. The requesting user is excluded from the
// benchmark aggregates so "compared with the top performers" is meaningful.
func (s *Service) buildTop10Facts(ctx context.Context, requestingUserID string, userFacts UserPortfolioFacts) PublicTop10Facts {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return PublicTop10Facts{Available: false}
	}

	ranked := make([]rankedPortfolio, 0, len(users))
	for _, u := range users {
		summary, err := s.summaries.Summary(ctx, u.ID)
		if err != nil || summary == nil || len(summary.Positions) == 0 {
			continue
		}
		hs, largest, count := publicHoldings(summary)
		ranked = append(ranked, rankedPortfolio{
			user:           u,
			publicHoldings: hs,
			largestWeight:  largest,
			positionCount:  count,
			returnPct:      round2(summary.GainLossPercentage),
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].returnPct != ranked[j].returnPct {
			return ranked[i].returnPct > ranked[j].returnPct
		}
		return ranked[i].user.DisplayName < ranked[j].user.DisplayName
	})
	if len(ranked) > topPerformerLimit {
		ranked = ranked[:topPerformerLimit]
	}

	// Public projection of the whole top group (may include the requesting user).
	portfolios := make([]PublicPortfolio, 0, len(ranked))
	for i, rp := range ranked {
		portfolios = append(portfolios, PublicPortfolio{
			Rank:             i + 1,
			DisplayName:      rp.user.DisplayName,
			AvatarKey:        rp.user.AvatarKey,
			ReturnPercentage: rp.returnPct,
			PortfolioIndex:   round2(100 + rp.returnPct),
			Holdings:         rp.publicHoldings,
		})
	}

	// Benchmark aggregates exclude the requesting user's own entry.
	var others []rankedPortfolio
	for _, rp := range ranked {
		if rp.user.ID != requestingUserID {
			others = append(others, rp)
		}
	}

	facts := PublicTop10Facts{
		SampleSize:                  len(others),
		Portfolios:                  portfolios,
		UserLargestWeightPercentage: userFacts.LargestWeightPercentage,
	}

	if len(others) < minBenchmarkPortfolios {
		facts.Available = false
		return facts
	}
	facts.Available = true
	facts.Limited = len(others) < topPerformerLimit

	largestWeights := make([]float64, 0, len(others))
	positionCounts := make([]float64, 0, len(others))
	returns := make([]float64, 0, len(others))
	otherSymbols := map[string]bool{}
	for _, rp := range others {
		largestWeights = append(largestWeights, rp.largestWeight)
		positionCounts = append(positionCounts, float64(rp.positionCount))
		returns = append(returns, rp.returnPct)
		for _, h := range rp.publicHoldings {
			otherSymbols[h.Symbol] = true
		}
	}

	shared := 0
	for _, h := range userFacts.Holdings {
		if otherSymbols[h.Symbol] {
			shared++
		}
	}

	facts.MedianLargestWeightPercentage = round2(median(largestWeights))
	facts.MedianPositionCount = round2(median(positionCounts))
	facts.SharedSymbolsCount = shared
	facts.ReturnGapPercentagePoints = round2(userFacts.GainLossPercentage - median(returns))

	return facts
}

// median returns the median of values (0 for an empty slice).
func median(values []float64) float64 {
	n := len(values)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)
	mid := n / 2
	if n%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}
