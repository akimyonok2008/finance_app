package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimilarSelectionPrefersSymbolOverlap(t *testing.T) {
	current := rankingCandidate("me", "me", "balanced_global", rankedWeights("QQQ", "GLD"), 10, 110, 1)
	closeMatch := rankingCandidate("u1", "close", "growth_quality", rankedWeights("QQQ", "SPY"), 12, 112, 2)
	assetOnly := rankingCandidate("u2", "asset", "growth_quality", rankedWeights("VTI", "VXUS"), 12, 112, 3)

	got := selectSimilarProfiles(current, []exploreCandidate{assetOnly, closeMatch}, 5)
	require.NotEmpty(t, got)
	assert.Equal(t, "close", got[0].Handle)
}

func TestSimilarSelectionUsesAssetTypeWhenSymbolsDiffer(t *testing.T) {
	current := rankingCandidate("me", "me", "balanced_global", []PublicWeight{{Symbol: "AAA", AssetType: "stock", Weight: 50}, {Symbol: "BBB", AssetType: "stock", Weight: 50}}, 0, 100, 1)
	stockMatch := rankingCandidate("u1", "stock_match", "income_core", []PublicWeight{{Symbol: "CCC", AssetType: "stock", Weight: 50}, {Symbol: "DDD", AssetType: "stock", Weight: 50}}, 0, 100, 2)
	cryptoMismatch := rankingCandidate("u2", "crypto", "macro_tactical", []PublicWeight{{Symbol: "EEE", AssetType: "crypto", Weight: 50}, {Symbol: "FFF", AssetType: "crypto", Weight: 50}}, 0, 100, 3)

	got := selectSimilarProfiles(current, []exploreCandidate{cryptoMismatch, stockMatch}, 5)
	require.Len(t, got, 1)
	assert.Equal(t, "stock_match", got[0].Handle)
}

func TestSimilarSelectionExcludesCurrentUser(t *testing.T) {
	current := rankingCandidate("me", "me", "balanced_global", rankedWeights("QQQ", "GLD"), 10, 110, 1)
	other := rankingCandidate("u1", "other", "balanced_global", rankedWeights("QQQ", "GLD"), 10, 110, 2)

	got := selectSimilarProfiles(current, []exploreCandidate{current, other}, 5)
	assert.Equal(t, []string{"other"}, handlesOf(got))
}

func TestSimilarSelectionReturnsFewerThanLimitBelowThreshold(t *testing.T) {
	current := rankingCandidate("me", "me", "balanced_global", []PublicWeight{{Symbol: "AAA", AssetType: "stock", Weight: 50}, {Symbol: "BBB", AssetType: "stock", Weight: 50}}, 0, 100, 1)
	quality := rankingCandidate("u1", "quality", "income_core", []PublicWeight{{Symbol: "CCC", AssetType: "stock", Weight: 50}, {Symbol: "DDD", AssetType: "stock", Weight: 50}}, 0, 100, 2)
	weak := rankingCandidate("u2", "weak", "macro_tactical", []PublicWeight{{Symbol: "EEE", AssetType: "crypto", Weight: 50}, {Symbol: "FFF", AssetType: "crypto", Weight: 50}}, 0, 100, 3)

	got := selectSimilarProfiles(current, []exploreCandidate{quality, weak}, 5)
	assert.Equal(t, []string{"quality"}, handlesOf(got))
}

func TestFeaturedBalancesPerformanceWithProfileQuality(t *testing.T) {
	extreme := rankingCandidate("u1", "extreme", "growth", []PublicWeight{{Symbol: "AAA", AssetType: "stock", Weight: 80}, {Symbol: "BBB", AssetType: "stock", Weight: 20}}, 30, 130, 1)
	extreme.card.Concentration = Concentration{LargestPosition: 80, TopThree: 100}
	complete := rankingCandidate("u2", "complete", "balanced", rankedWeights("CCC", "DDD", "EEE"), 10, 110, 4)
	complete.card.Bio = "A documented, repeatable strategy."
	complete.card.Badges = []PublicBadge{{Key: "a"}, {Key: "b"}, {Key: "c"}, {Key: "d"}, {Key: "e"}}

	got := selectFeaturedProfiles("me", []exploreCandidate{extreme, complete}, nil, 5, false)
	require.Len(t, got, 2)
	assert.Equal(t, "complete", got[0].Handle)
}

func TestFeaturedScoreRewardsCompleteness(t *testing.T) {
	plain := rankingCandidate("u1", "plain", "", rankedWeights("AAA", "BBB", "CCC"), 10, 110, 10).card
	complete := plain
	complete.Handle = "complete"
	complete.Bio = "Clear strategy notes"
	complete.StrategyTag = "balanced_global"

	assert.Greater(t, featuredScore(complete, false), featuredScore(plain, false))
}

func TestFeaturedScorePenalizesExtremeConcentration(t *testing.T) {
	diverse := rankingCandidate("u1", "diverse", "balanced", rankedWeights("AAA", "BBB", "CCC"), 10, 110, 5).card
	extreme := diverse
	extreme.PublicWeights = []PublicWeight{{Symbol: "AAA", AssetType: "stock", Weight: 80}, {Symbol: "BBB", AssetType: "stock", Weight: 20}}
	extreme.Concentration = Concentration{LargestPosition: 80, TopThree: 100}

	assert.Greater(t, featuredScore(diverse, false), featuredScore(extreme, false))
}

func TestFeaturedSelectionAppliesStrategyTagDiversity(t *testing.T) {
	candidates := []exploreCandidate{
		rankingCandidate("u1", "growth_1", "growth", rankedWeights("A1", "B1", "C1"), 25, 125, 1),
		rankingCandidate("u2", "growth_2", "growth", rankedWeights("A2", "B2", "C2"), 24, 124, 2),
		rankingCandidate("u3", "growth_3", "growth", rankedWeights("A3", "B3", "C3"), 23, 123, 3),
		rankingCandidate("u4", "income", "income", rankedWeights("A4", "B4", "C4"), 18, 118, 4),
		rankingCandidate("u5", "macro", "macro", rankedWeights("A5", "B5", "C5"), 17, 117, 5),
		rankingCandidate("u6", "defensive", "defensive", rankedWeights("A6", "B6", "C6"), 16, 116, 6),
	}

	got := selectFeaturedProfiles("me", candidates, nil, 5, false)
	growthCount := 0
	for _, card := range got {
		if card.StrategyTag == "growth" {
			growthCount++
		}
	}
	assert.LessOrEqual(t, growthCount, 2)
}

func TestFeaturedAvoidsSimilarDuplicatesWhenPossible(t *testing.T) {
	candidates := make([]exploreCandidate, 0, 6)
	for i, handle := range []string{"shared", "one", "two", "three", "four", "five"} {
		candidates = append(candidates, rankingCandidate(handle, handle, "tag_"+handle, rankedWeights(handle+"A", handle+"B", handle+"C"), float64(20-i), float64(120-i), i+1))
	}

	got := selectFeaturedProfiles("me", candidates, map[string]bool{"shared": true}, 5, false)
	assert.Len(t, got, 5)
	assert.NotContains(t, handlesOf(got), "shared")
}

func rankingCandidate(userID, handle, tag string, weights []PublicWeight, rankedReturn, index float64, rank int) exploreCandidate {
	card := PublicProfile{
		Handle: handle, DisplayName: handle, StrategyTag: tag, PublicWeights: weights,
		ReturnPercentage: rankedReturn, PortfolioIndex: index, GlobalRank: &rank,
		Concentration: Concentration{LargestPosition: weights[0].Weight, TopThree: 100},
		Badges:        []PublicBadge{},
	}
	return exploreCandidate{userID: userID, card: card, hasIndex: true}
}

func rankedWeights(symbols ...string) []PublicWeight {
	weights := make([]PublicWeight, 0, len(symbols))
	for i, symbol := range symbols {
		weight := 100 / float64(len(symbols))
		if i == len(symbols)-1 {
			weight = 100 - float64(len(symbols)-1)*weight
		}
		weights = append(weights, PublicWeight{Symbol: symbol, AssetType: "stock", Weight: weight})
	}
	return weights
}
