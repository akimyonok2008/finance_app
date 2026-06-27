package profile

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var exploreSymbolPattern = regexp.MustCompile(`^[A-Z0-9.\-]{1,15}$`)

// scoredCard pairs a public card with its underlying profile so sorts that need
// raw fields (e.g. updated_at for "recent") can reach them without leaking them.
type scoredCard struct {
	profile Profile
	card    PublicProfile
}

// Explore builds the discovery payload: featured cards, a paginated list of top
// performers, and globally trending public holdings.
//
// Trending holdings are computed from the full public dataset (a global
// discovery feature), independent of the q/symbol filters and pagination, while
// featured/top_performers reflect the filtered + sorted set.
//
// Prototype note: this iterates the full public-profile set and calls the
// summary/rank providers per profile (N+1). That is acceptable at prototype
// scale; see TODOs on the repository for the materialized/cached path.
func (s *Service) Explore(ctx context.Context, callerID string, filter ExploreFilter) (ExploreResponse, error) {
	profiles, err := s.repo.ListPublicProfiles(ctx)
	if err != nil {
		return ExploreResponse{}, err
	}

	all := make([]scoredCard, 0, len(profiles))
	allCards := make([]PublicProfile, 0, len(profiles))
	for _, p := range profiles {
		if !p.IsPublic {
			continue
		}
		card := s.publicProjection(ctx, p)
		all = append(all, scoredCard{profile: p, card: card})
		allCards = append(allCards, card)
	}

	trending := buildTrendingHoldings(allCards)
	// Similar strategies are computed against the caller's own composition,
	// independent of the q/symbol filter (a global discovery section).
	similar := s.buildSimilar(ctx, callerID, all)

	filtered := make([]scoredCard, 0, len(all))
	for _, sc := range all {
		if !matchesQuery(sc.card, filter.Query) {
			continue
		}
		if filter.Symbol != "" && !hasPublicSymbol(sc.card, filter.Symbol) {
			continue
		}
		filtered = append(filtered, sc)
	}

	sortCards(filtered, filter.Sort)

	total := len(filtered)

	featured := make([]PublicProfile, 0, featuredCount)
	for i := 0; i < len(filtered) && i < featuredCount; i++ {
		featured = append(featured, filtered[i].card)
	}

	start := filter.Offset
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + filter.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := filtered[start:end]
	topPerformers := make([]PublicProfile, 0, len(page))
	for _, sc := range page {
		topPerformers = append(topPerformers, sc.card)
	}

	return ExploreResponse{
		Featured:         featured,
		Similar:          similar,
		TopPerformers:    topPerformers,
		TrendingHoldings: trending,
		Pagination: ExplorePagination{
			Limit:   filter.Limit,
			Offset:  filter.Offset,
			Total:   total,
			HasMore: end < total,
		},
	}, nil
}

// buildSimilar returns up to maxSimilar public profiles whose composition most
// resembles the caller's. Similarity is the summed weight overlap over shared
// symbols (a "portfolio overlap %"); profiles sharing the caller's strategy_tag
// are used as a secondary signal and as a fallback when the caller has no
// composition yet. The caller's own card is always excluded.
func (s *Service) buildSimilar(ctx context.Context, callerID string, all []scoredCard) []PublicProfile {
	if callerID == "" {
		return []PublicProfile{}
	}

	callerWeights := map[string]float64{}
	if summary, err := s.summaries.GetSummary(ctx, callerID); err == nil && summary != nil {
		weights, _, _, _ := buildComposition(summary)
		for _, w := range weights {
			callerWeights[w.Symbol] = w.Weight
		}
	}
	callerTag := ""
	if p, err := s.repo.GetByUserID(ctx, callerID); err == nil {
		callerTag = p.StrategyTag
	}

	type scored struct {
		card    PublicProfile
		overlap float64
		sameTag bool
	}
	candidates := make([]scored, 0, len(all))
	for _, sc := range all {
		if sc.profile.UserID == callerID {
			continue // never suggest the caller to themselves
		}
		var overlap float64
		for _, w := range sc.card.PublicWeights {
			if cw, ok := callerWeights[w.Symbol]; ok {
				overlap += minFloat(cw, w.Weight)
			}
		}
		sameTag := callerTag != "" && sc.profile.StrategyTag == callerTag
		if overlap == 0 && !sameTag {
			continue // unrelated — not "similar"
		}
		candidates = append(candidates, scored{card: sc.card, overlap: round2(overlap), sameTag: sameTag})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].overlap != candidates[j].overlap {
			return candidates[i].overlap > candidates[j].overlap
		}
		if candidates[i].sameTag != candidates[j].sameTag {
			return candidates[i].sameTag // tag matches rank ahead on ties
		}
		if candidates[i].card.ReturnPercentage != candidates[j].card.ReturnPercentage {
			return candidates[i].card.ReturnPercentage > candidates[j].card.ReturnPercentage
		}
		return candidates[i].card.Handle < candidates[j].card.Handle
	})

	out := make([]PublicProfile, 0, maxSimilar)
	for i := 0; i < len(candidates) && i < maxSimilar; i++ {
		out = append(out, candidates[i].card)
	}
	return out
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// matchesQuery is a case-insensitive substring match over handle, display name,
// and public symbols. Hidden-weight profiles expose no public symbols, so symbol
// matches naturally never reach private holdings. Empty query matches all.
func matchesQuery(card PublicProfile, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	if strings.Contains(strings.ToLower(card.Handle), query) {
		return true
	}
	if strings.Contains(strings.ToLower(card.DisplayName), query) {
		return true
	}
	for _, w := range card.PublicWeights {
		if strings.Contains(strings.ToLower(w.Symbol), query) {
			return true
		}
	}
	return false
}

func hasPublicSymbol(card PublicProfile, symbol string) bool {
	for _, w := range card.PublicWeights {
		if w.Symbol == symbol {
			return true
		}
	}
	return false
}

func sortCards(cards []scoredCard, mode string) {
	switch mode {
	case SortReturn:
		sort.SliceStable(cards, func(i, j int) bool {
			a, b := cards[i].card, cards[j].card
			if a.ReturnPercentage != b.ReturnPercentage {
				return a.ReturnPercentage > b.ReturnPercentage
			}
			return a.Handle < b.Handle
		})
	case SortRank:
		sort.SliceStable(cards, func(i, j int) bool {
			if r := compareRank(cards[i].card, cards[j].card); r != 0 {
				return r < 0
			}
			return cards[i].card.Handle < cards[j].card.Handle
		})
	case SortRecent:
		sort.SliceStable(cards, func(i, j int) bool {
			a, b := cards[i].profile, cards[j].profile
			if !a.UpdatedAt.Equal(b.UpdatedAt) {
				return a.UpdatedAt.After(b.UpdatedAt)
			}
			return cards[i].card.Handle < cards[j].card.Handle
		})
	default: // SortTop
		sort.SliceStable(cards, func(i, j int) bool {
			a, b := cards[i].card, cards[j].card
			// Prefer global rank ascending when present (ranked beats unranked).
			if r := compareRank(a, b); r != 0 {
				return r < 0
			}
			if a.GlobalRank == nil && b.GlobalRank == nil {
				if a.PortfolioIndex != b.PortfolioIndex {
					return a.PortfolioIndex > b.PortfolioIndex
				}
				if a.ReturnPercentage != b.ReturnPercentage {
					return a.ReturnPercentage > b.ReturnPercentage
				}
			}
			return a.Handle < b.Handle
		})
	}
}

// compareRank orders by global rank ascending with missing ranks last. Returns
// <0 if a precedes b, >0 if b precedes a, 0 if equivalent on rank.
func compareRank(a, b PublicProfile) int {
	switch {
	case a.GlobalRank != nil && b.GlobalRank != nil:
		return *a.GlobalRank - *b.GlobalRank
	case a.GlobalRank != nil:
		return -1
	case b.GlobalRank != nil:
		return 1
	default:
		return 0
	}
}

// buildTrendingHoldings aggregates public symbols across public profiles.
// Profiles with show_public_weights=false expose no public weights, so they are
// naturally excluded from these counts.
func buildTrendingHoldings(cards []PublicProfile) []TrendingHolding {
	type agg struct {
		count       int
		weightSum   float64
		assetCounts map[string]int
		top10       int
	}
	stats := map[string]*agg{}
	order := make([]string, 0)

	for _, c := range cards {
		for _, w := range c.PublicWeights {
			a, ok := stats[w.Symbol]
			if !ok {
				a = &agg{assetCounts: map[string]int{}}
				stats[w.Symbol] = a
				order = append(order, w.Symbol)
			}
			a.count++
			a.weightSum += w.Weight
			if w.AssetType != "" {
				a.assetCounts[w.AssetType]++
			}
		}
	}

	// top10_count: symbols held by the current top-10 public profiles by global
	// rank. Profiles without a rank simply do not contribute.
	ranked := make([]PublicProfile, 0, len(cards))
	for _, c := range cards {
		if c.GlobalRank != nil {
			ranked = append(ranked, c)
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return *ranked[i].GlobalRank < *ranked[j].GlobalRank })
	if len(ranked) > trendingTopN {
		ranked = ranked[:trendingTopN]
	}
	for _, c := range ranked {
		seen := map[string]bool{}
		for _, w := range c.PublicWeights {
			if seen[w.Symbol] {
				continue
			}
			seen[w.Symbol] = true
			if a, ok := stats[w.Symbol]; ok {
				a.top10++
			}
		}
	}

	out := make([]TrendingHolding, 0, len(order))
	for _, sym := range order {
		a := stats[sym]
		out = append(out, TrendingHolding{
			Symbol:        sym,
			ProfileCount:  a.count,
			AverageWeight: round2(a.weightSum / float64(a.count)),
			Top10Count:    a.top10,
			AssetType:     mostCommonAsset(a.assetCounts),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ProfileCount != out[j].ProfileCount {
			return out[i].ProfileCount > out[j].ProfileCount
		}
		if out[i].AverageWeight != out[j].AverageWeight {
			return out[i].AverageWeight > out[j].AverageWeight
		}
		return out[i].Symbol < out[j].Symbol
	})
	if len(out) > maxTrendingHoldings {
		out = out[:maxTrendingHoldings]
	}
	return out
}

func mostCommonAsset(counts map[string]int) string {
	best := ""
	bestCount := 0
	for asset, n := range counts {
		if n > bestCount || (n == bestCount && asset < best) {
			best = asset
			bestCount = n
		}
	}
	return best
}

// ParseExploreFilter validates and normalizes raw query parameters. Malformed
// values return an ErrInvalid-wrapped error so the handler can map them to 400.
// limit is clamped to maxExploreLimit rather than rejected.
func ParseExploreFilter(get func(string) string) (ExploreFilter, error) {
	f := ExploreFilter{Sort: SortTop, Limit: defaultExploreLimit}

	f.Query = strings.TrimSpace(get("q"))

	if raw := strings.TrimSpace(get("symbol")); raw != "" {
		symbol := strings.ToUpper(raw)
		if !exploreSymbolPattern.MatchString(symbol) {
			return f, invalid("invalid symbol")
		}
		f.Symbol = symbol
	}

	if raw := strings.TrimSpace(get("sort")); raw != "" {
		switch raw {
		case SortTop, SortReturn, SortRank, SortRecent:
			f.Sort = raw
		default:
			return f, invalid("invalid sort")
		}
	}

	if raw := strings.TrimSpace(get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return f, invalid("invalid limit")
		}
		if n > maxExploreLimit {
			n = maxExploreLimit
		}
		f.Limit = n
	}

	if raw := strings.TrimSpace(get("offset")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return f, invalid("invalid offset")
		}
		f.Offset = n
	}

	return f, nil
}
