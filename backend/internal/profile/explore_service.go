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
// PostgreSQL production uses an atomically published public-card projection:
// filtering, sorting, pagination and global trending aggregation stay in SQL,
// and Go decodes only the requested page plus a bounded recommendation pool.
// The live implementation below remains the cold-start and memory-repository
// fallback so a missing projection never makes Explore unavailable.
func (s *Service) Explore(ctx context.Context, callerID string, filter ExploreFilter) (ExploreResponse, error) {
	blocked, err := s.blockedSet(ctx, callerID)
	if err != nil {
		return ExploreResponse{}, err
	}
	if out, found, err := s.exploreFromProjection(ctx, callerID, filter, blocked); err != nil {
		return ExploreResponse{}, err
	} else if found {
		return out, nil
	}

	// Cold start / memory-mode compatibility path. Production PostgreSQL
	// requests leave this path as soon as the background worker publishes the
	// first complete projection generation.
	profiles, err := s.repo.ListPublicProfiles(ctx)
	if err != nil {
		return ExploreResponse{}, err
	}
	ranks, fallback := s.exploreRankings(ctx, filter.Timeframe)

	summaries := s.exploreSummaryProvider()
	all := make([]scoredCard, 0, len(profiles))
	allCards := make([]PublicProfile, 0, len(profiles))
	for _, p := range profiles {
		if !p.IsPublic || !p.ShowPublicWeights {
			continue
		}
		if blocked[p.UserID] {
			continue
		}
		rank, hasRank, index, returnPct, ok := s.resolveExploreRanking(ctx, p.UserID, ranks, fallback)
		if !ok {
			// Unranked, excluded from the requested timeframe, or a live
			// valuation failed for just this one profile — skip it rather
			// than aborting the whole discovery response for every caller.
			continue
		}
		card := s.buildPublicProfile(ctx, p, summaries, index, returnPct)
		if len(card.PublicWeights) == 0 {
			continue
		}
		if hasRank {
			r := rank.Rank
			card.GlobalRank = &r
		}
		all = append(all, scoredCard{profile: p, card: card})
		allCards = append(allCards, card)
	}

	trending := buildTrendingHoldings(allCards)
	// Similar strategies are computed against the caller's own composition,
	// independent of the q/symbol filter (a global discovery section).
	similar, err := s.buildSimilar(ctx, callerID, all)
	if err != nil {
		return ExploreResponse{}, err
	}

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

	similarHandles := make(map[string]bool, len(similar))
	for _, card := range similar {
		similarHandles[card.Handle] = true
	}
	// Featured is a stable discovery rail. Search and symbol filters only affect
	// top_performers (the frontend's dedicated search results), so typing a
	// query never rewrites the recommendations beside it.
	featuredCandidates := make([]exploreCandidate, 0, len(all))
	for _, sc := range all {
		featuredCandidates = append(featuredCandidates, candidateFromCard(sc))
	}
	featured := selectFeaturedProfiles(callerID, featuredCandidates, similarHandles, featuredCount, fallback)

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
		Timeframe:         filter.Timeframe,
		TimeframeFallback: fallback,
		Featured:          featured,
		Similar:           similar,
		TopPerformers:     topPerformers,
		TrendingHoldings:  trending,
		Pagination: ExplorePagination{
			Limit:   filter.Limit,
			Offset:  filter.Offset,
			Total:   total,
			HasMore: end < total,
		},
	}, nil
}

func (s *Service) exploreRankings(ctx context.Context, timeframe string) (map[string]TimeframeRanking, bool) {
	if timeframe == "" {
		timeframe = TimeframeAll
	}
	if s.timeframeRanks == nil {
		return map[string]TimeframeRanking{}, true
	}
	rows, err := s.timeframeRanks.UserRankings(ctx, timeframe)
	if err != nil {
		return map[string]TimeframeRanking{}, true
	}
	out := make(map[string]TimeframeRanking, len(rows))
	for _, row := range rows {
		out[row.UserID] = row
	}
	return out, false
}

// resolveExploreRanking resolves userID's ranked index/return percentage for a
// single Explore card, preferring the bulk-fetched ranks map (populated once
// per request from the ranking projection via s.timeframeRanks.UserRankings)
// over a live per-profile valuation. ok=false means the profile must be
// excluded from Explore entirely (no ranking for the requested timeframe, or
// — only reached when no ranking provider is configured at all — a live
// valuation failure).
func (s *Service) resolveExploreRanking(ctx context.Context, userID string, ranks map[string]TimeframeRanking, fallback bool) (rank TimeframeRanking, hasRank bool, index, returnPct float64, ok bool) {
	if rank, hasRank = ranks[userID]; hasRank {
		return rank, true, rank.RankedIndex, rank.RankedReturnPercentage, true
	}
	if s.timeframeRanks != nil && !fallback {
		return TimeframeRanking{}, false, 0, 0, false
	}
	if s.ranked == nil {
		return TimeframeRanking{}, false, 0, 0, false
	}
	rp, err := s.ranked.CurrentRankedPerformance(ctx, userID)
	if err != nil {
		return TimeframeRanking{}, false, 0, 0, false
	}
	return TimeframeRanking{}, false, round2(rp.RankedIndex), round2(rp.RankedReturnPercentage), true
}

// buildSimilar returns deterministic, quality-thresholded matches using symbol,
// asset-type, DNA, strategy-tag and concentration similarity. MMR selection
// adds light symbol diversity after the candidates are scored.
func (s *Service) buildSimilar(ctx context.Context, callerID string, all []scoredCard) ([]PublicProfile, error) {
	if callerID == "" {
		return []PublicProfile{}, nil
	}

	var current exploreCandidate
	current.userID = callerID
	if summary, err := s.exploreSummaryProvider().GetSummary(ctx, callerID); err == nil && summary != nil {
		weights, _, _, concentration := buildComposition(summary)
		current.card.PublicWeights = weights
		current.card.Concentration = concentration
		if s.ranked == nil {
			return nil, ErrRankedDataUnavailable
		}
		rp, err := s.ranked.CurrentRankedPerformance(ctx, callerID)
		if err != nil {
			return nil, ErrRankedDataUnavailable
		}
		current.card.PortfolioIndex = rp.RankedIndex
		current.hasIndex = finitePositive(rp.RankedIndex)
		if result, dnaErr := s.dna.Calculate(ctx, dnaInputsFromSummary(summary)); dnaErr == nil && result.HasData {
			current.dna = dnaScoreVector(result.Scores)
			current.hasDNA = true
		}
	}
	if p, err := s.repo.GetByUserID(ctx, callerID); err == nil {
		current.card.StrategyTag = p.StrategyTag
	}
	candidates := make([]exploreCandidate, 0, len(all))
	for _, sc := range all {
		candidates = append(candidates, candidateFromCard(sc))
	}
	return selectSimilarProfiles(current, candidates, maxSimilar), nil
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
	return buildTrendingHoldingsLimit(cards, maxTrendingHoldings)
}

func buildTrendingHoldingsLimit(cards []PublicProfile, limit int) []TrendingHolding {
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
	if limit > 0 && len(out) > limit {
		out = out[:limit]
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
	f := ExploreFilter{Sort: SortTop, Timeframe: TimeframeAll, Limit: defaultExploreLimit}

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

	if raw := strings.TrimSpace(get("timeframe")); raw != "" {
		switch strings.ToUpper(raw) {
		case Timeframe1W, Timeframe1M, Timeframe3M, Timeframe6M, Timeframe1Y, TimeframeAll:
			f.Timeframe = strings.ToUpper(raw)
		default:
			return f, invalid("invalid timeframe")
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
