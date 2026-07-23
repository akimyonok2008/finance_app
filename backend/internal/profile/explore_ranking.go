package profile

import (
	"math"
	"sort"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/dna"
)

const minimumSimilarityScore = 0.18

type exploreCandidate struct {
	userID   string
	card     PublicProfile
	dna      []float64
	hasDNA   bool
	hasIndex bool
}

type similarCandidate struct {
	candidate exploreCandidate
	score     float64
}

type featuredCandidate struct {
	candidate exploreCandidate
	score     float64
}

func candidateFromCard(sc scoredCard) exploreCandidate {
	vector := dnaScoreVector(sc.card.Insights.DNA)
	return exploreCandidate{
		userID:   sc.profile.UserID,
		card:     sc.card,
		dna:      vector,
		hasDNA:   vectorMagnitude(vector) > 0,
		hasIndex: finitePositive(sc.card.PortfolioIndex),
	}
}

func selectSimilarProfiles(current exploreCandidate, candidates []exploreCandidate, limit int) []PublicProfile {
	if limit <= 0 || len(current.card.PublicWeights) < 2 {
		return []PublicProfile{}
	}

	scored := make([]similarCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.userID == current.userID || len(candidate.card.PublicWeights) < 2 {
			continue
		}
		if candidate.hasIndex && !finitePositive(candidate.card.PortfolioIndex) {
			continue
		}
		score := similarityScore(current, candidate)
		if score >= minimumSimilarityScore {
			scored = append(scored, similarCandidate{candidate: candidate, score: score})
		}
	}

	selected := make([]similarCandidate, 0, minInt(limit, len(scored)))
	for len(selected) < limit && len(scored) > 0 {
		best := 0
		bestMMR := math.Inf(-1)
		for i, candidate := range scored {
			redundancy := 0.0
			for _, prior := range selected {
				redundancy = math.Max(redundancy, weightedJaccard(
					weightsBySymbol(candidate.candidate.card.PublicWeights),
					weightsBySymbol(prior.candidate.card.PublicWeights),
				))
			}
			mmr := 0.85*candidate.score - 0.15*redundancy
			if mmr > bestMMR || (almostEqual(mmr, bestMMR) && similarLess(candidate, scored[best])) {
				best, bestMMR = i, mmr
			}
		}
		selected = append(selected, scored[best])
		scored = append(scored[:best], scored[best+1:]...)
	}

	out := make([]PublicProfile, 0, len(selected))
	for _, selectedCandidate := range selected {
		out = append(out, selectedCandidate.candidate.card)
	}
	return out
}

func similarityScore(current, candidate exploreCandidate) float64 {
	symbol := weightedJaccard(weightsBySymbol(current.card.PublicWeights), weightsBySymbol(candidate.card.PublicWeights))
	asset := cosineSimilarity(weightsByAssetType(current.card.PublicWeights), weightsByAssetType(candidate.card.PublicWeights))
	tag := strategyTagSimilarity(current.card.StrategyTag, candidate.card.StrategyTag)
	concentration := concentrationSimilarity(current.card.Concentration.TopThree, candidate.card.Concentration.TopThree)
	if current.hasDNA && candidate.hasDNA {
		return 0.45*symbol + 0.20*cosineSlice(current.dna, candidate.dna) + 0.15*asset + 0.10*tag + 0.10*concentration
	}
	return 0.55*symbol + 0.20*asset + 0.15*tag + 0.10*concentration
}

func selectFeaturedProfiles(currentUserID string, candidates []exploreCandidate, similarHandles map[string]bool, limit int, timeframeFallback bool) []PublicProfile {
	if limit <= 0 {
		return []PublicProfile{}
	}
	scored := make([]featuredCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		card := candidate.card
		if candidate.userID == currentUserID || len(card.PublicWeights) < 2 || card.GlobalRank == nil || *card.GlobalRank <= 0 || !finitePositive(card.PortfolioIndex) {
			continue
		}
		scored = append(scored, featuredCandidate{candidate: candidate, score: featuredScore(card, timeframeFallback)})
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if !almostEqual(scored[i].score, scored[j].score) {
			return scored[i].score > scored[j].score
		}
		return scored[i].candidate.card.Handle < scored[j].candidate.card.Handle
	})

	selected := make([]featuredCandidate, 0, minInt(limit, len(scored)))
	used := make(map[string]bool, len(scored))
	tagCounts := map[string]int{}
	topSymbolCounts := map[string]int{}
	// Relax in the product-defined order: strategy tag, top holding, then
	// overlap with Similar to You. Each pass only adds previously skipped cards.
	for relaxation := 0; relaxation < 4 && len(selected) < limit; relaxation++ {
		for _, candidate := range scored {
			card := candidate.candidate.card
			if used[card.Handle] {
				continue
			}
			topSymbol := largestHoldingSymbol(card.PublicWeights)
			if relaxation < 1 && tagCounts[card.StrategyTag] >= 2 {
				continue
			}
			if relaxation < 2 && topSymbol != "" && topSymbolCounts[topSymbol] >= 2 {
				continue
			}
			if relaxation < 3 && similarHandles[card.Handle] {
				continue
			}
			selected = append(selected, candidate)
			used[card.Handle] = true
			tagCounts[card.StrategyTag]++
			if topSymbol != "" {
				topSymbolCounts[topSymbol]++
			}
			if len(selected) == limit {
				break
			}
		}
	}

	out := make([]PublicProfile, 0, len(selected))
	for _, selectedCandidate := range selected {
		out = append(out, selectedCandidate.candidate.card)
	}
	return out
}

func featuredScore(card PublicProfile, timeframeFallback bool) float64 {
	returnScore := clamp((card.ReturnPercentage+10)/40, 0, 1)
	rankScore := 0.0
	if card.GlobalRank != nil && *card.GlobalRank > 0 {
		rankScore = clamp(5/math.Sqrt(float64(*card.GlobalRank)), 0, 1)
	}
	badgeScore := clamp(float64(len(card.Badges))/5, 0, 1)
	completeness := 0.0
	if strings.TrimSpace(card.Bio) != "" {
		completeness += 0.35
	}
	if strings.TrimSpace(card.StrategyTag) != "" {
		completeness += 0.25
	}
	if len(card.PublicWeights) > 0 {
		completeness += 0.25
	}
	if card.Concentration.TopThree > 0 || card.Concentration.LargestPosition > 0 {
		completeness += 0.15
	}
	diversification := 1.0
	if card.Concentration.TopThree > 90 {
		diversification -= 0.4
	}
	if len(card.PublicWeights) < 3 {
		diversification -= 0.25
	}
	if card.Concentration.LargestPosition > 70 {
		diversification -= 0.25
	}
	timeframe := 1.0
	if timeframeFallback {
		timeframe = 0.75
	}
	return 0.35*returnScore + 0.20*rankScore + 0.15*badgeScore + 0.15*clamp(completeness, 0, 1) + 0.10*clamp(diversification, 0, 1) + 0.05*timeframe
}

func weightsBySymbol(weights []PublicWeight) map[string]float64 {
	out := map[string]float64{}
	for _, weight := range weights {
		if weight.Weight > 0 {
			out[strings.ToUpper(weight.Symbol)] += weight.Weight
		}
	}
	return normalizeVector(out)
}

func weightsByAssetType(weights []PublicWeight) map[string]float64 {
	out := map[string]float64{}
	for _, weight := range weights {
		if weight.Weight > 0 {
			assetType := strings.ToLower(strings.TrimSpace(weight.AssetType))
			if assetType == "" {
				assetType = "other"
			}
			out[assetType] += weight.Weight
		}
	}
	return normalizeVector(out)
}

func normalizeVector(values map[string]float64) map[string]float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	if total <= 0 {
		return values
	}
	for key := range values {
		values[key] /= total
	}
	return values
}

func weightedJaccard(a, b map[string]float64) float64 {
	keys := map[string]bool{}
	for key := range a {
		keys[key] = true
	}
	for key := range b {
		keys[key] = true
	}
	numerator, denominator := 0.0, 0.0
	for key := range keys {
		numerator += math.Min(a[key], b[key])
		denominator += math.Max(a[key], b[key])
	}
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func cosineSimilarity(a, b map[string]float64) float64 {
	dot, normA, normB := 0.0, 0.0, 0.0
	for key, value := range a {
		dot += value * b[key]
		normA += value * value
	}
	for _, value := range b {
		normB += value * value
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func cosineSlice(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	dot, normA, normB := 0.0, 0.0, 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func dnaScoreVector(scores dna.PortfolioDNAScores) []float64 {
	return []float64{float64(scores.Growth), float64(scores.Income), float64(scores.Commodities), float64(scores.Defensive), float64(scores.International), float64(scores.Concentration), float64(scores.Volatility)}
}

func vectorMagnitude(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value * value
	}
	return math.Sqrt(total)
}

func concentrationSimilarity(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return 1 - math.Min(math.Abs(a-b)/100, 1)
}

func strategyTagSimilarity(a, b string) float64 {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	if strategyFamily(a) == strategyFamily(b) {
		return 0.5
	}
	return 0
}

func strategyFamily(tag string) string {
	prefix := strings.SplitN(tag, "_", 2)[0]
	switch prefix {
	case "macro", "thematic":
		return "macro"
	case "commodity", "commodities":
		return "commodity"
	case "growth", "income", "balanced", "defensive":
		return prefix
	default:
		return prefix
	}
}

func largestHoldingSymbol(weights []PublicWeight) string {
	best := PublicWeight{}
	for _, weight := range weights {
		if weight.Weight > best.Weight || (almostEqual(weight.Weight, best.Weight) && weight.Symbol < best.Symbol) {
			best = weight
		}
	}
	return best.Symbol
}

func similarLess(a, b similarCandidate) bool {
	if !almostEqual(a.score, b.score) {
		return a.score > b.score
	}
	return a.candidate.card.Handle < b.candidate.card.Handle
}

func finitePositive(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
func clamp(value, low, high float64) float64 { return math.Max(low, math.Min(high, value)) }
func almostEqual(a, b float64) bool          { return math.Abs(a-b) < 1e-12 }
