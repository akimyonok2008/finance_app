package profile

import (
	"context"
	"time"
)

// exploreProjectionCandidateLimit bounds the recommendation work performed by
// an Explore request. The projection is already globally ranked, so this pool
// stays useful without making request cost proportional to the public-user
// population.
const exploreProjectionCandidateLimit = 200

var exploreTimeframes = []string{
	TimeframeAll, Timeframe1W, Timeframe1M, Timeframe3M, Timeframe6M, Timeframe1Y,
}

// ExploreProjectionRow is one privacy-safe, prebuilt public card for a ranking
// timeframe. UserID is stored only for server-side filtering and is never
// serialized in an API response.
type ExploreProjectionRow struct {
	UserID    string
	Timeframe string
	UpdatedAt time.Time
	Card      PublicProfile
}

type exploreTrendingProjectionRow struct {
	Timeframe    string
	Symbol       string
	ProfileCount int
	WeightSum    float64
	Top10Count   int
	AssetType    string
}

// ExploreProjectionSnapshot is the bounded data needed to assemble one
// Explore response. Page is already filtered, sorted and paginated by the
// database. Candidates is a small global pool for featured/similar selection.
type ExploreProjectionSnapshot struct {
	Page       []ExploreProjectionRow
	Candidates []ExploreProjectionRow
	Trending   []TrendingHolding
	Total      int
}

// exploreProjectionRepository is an optional production capability. The
// in-memory repository intentionally does not implement it and continues to
// use the exact live path used by unit tests and local development.
type exploreProjectionRepository interface {
	ReplaceExploreProjection(ctx context.Context, rows []ExploreProjectionRow) error
	LoadExploreProjection(
		ctx context.Context,
		filter ExploreFilter,
		blockedUserIDs []string,
		candidateLimit int,
	) (snapshot ExploreProjectionSnapshot, found bool, err error)
}

// RefreshExploreProjection rebuilds every public card off the request path and
// atomically publishes a complete generation. If any ranking timeframe cannot
// be read, the previous generation remains active.
func (s *Service) RefreshExploreProjection(ctx context.Context) (int, error) {
	store, ok := s.repo.(exploreProjectionRepository)
	if !ok || s.timeframeRanks == nil {
		return 0, nil
	}

	profiles, err := s.repo.ListPublicProfiles(ctx)
	if err != nil {
		return 0, err
	}
	rankings := make(map[string]map[string]TimeframeRanking, len(exploreTimeframes))
	for _, timeframe := range exploreTimeframes {
		list, err := s.timeframeRanks.UserRankings(ctx, timeframe)
		if err != nil {
			return 0, err
		}
		byUser := make(map[string]TimeframeRanking, len(list))
		for _, rank := range list {
			byUser[rank.UserID] = rank
		}
		rankings[timeframe] = byUser
	}

	summaries := s.exploreSummaryProvider()
	rows := make([]ExploreProjectionRow, 0, len(profiles)*len(exploreTimeframes))
	for _, p := range profiles {
		allRank, ranked := rankings[TimeframeAll][p.UserID]
		if !ranked {
			continue
		}
		base := s.buildPublicProfile(
			ctx, p, summaries, allRank.RankedIndex, allRank.RankedReturnPercentage,
		)
		if len(base.PublicWeights) == 0 {
			continue
		}
		for _, timeframe := range exploreTimeframes {
			rank, eligible := rankings[timeframe][p.UserID]
			if !eligible {
				continue
			}
			card := base
			card.PortfolioIndex = rank.RankedIndex
			card.ReturnPercentage = rank.RankedReturnPercentage
			r := rank.Rank
			card.GlobalRank = &r
			rows = append(rows, ExploreProjectionRow{
				UserID: p.UserID, Timeframe: timeframe, UpdatedAt: p.UpdatedAt, Card: card,
			})
		}
	}
	if err := store.ReplaceExploreProjection(ctx, rows); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func blockedIDs(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for id, blocked := range set {
		if blocked {
			out = append(out, id)
		}
	}
	return out
}

func buildExploreTrendingProjection(rows []ExploreProjectionRow) []exploreTrendingProjectionRow {
	type aggregate struct {
		count       int
		weightSum   float64
		top10       int
		assetCounts map[string]int
	}
	byTimeframe := make(map[string]map[string]*aggregate, len(exploreTimeframes))
	for _, row := range rows {
		bySymbol := byTimeframe[row.Timeframe]
		if bySymbol == nil {
			bySymbol = map[string]*aggregate{}
			byTimeframe[row.Timeframe] = bySymbol
		}
		top10 := row.Card.GlobalRank != nil && *row.Card.GlobalRank <= trendingTopN
		for _, weight := range row.Card.PublicWeights {
			stat := bySymbol[weight.Symbol]
			if stat == nil {
				stat = &aggregate{assetCounts: map[string]int{}}
				bySymbol[weight.Symbol] = stat
			}
			stat.count++
			stat.weightSum += weight.Weight
			if top10 {
				stat.top10++
			}
			if weight.AssetType != "" {
				stat.assetCounts[weight.AssetType]++
			}
		}
	}
	out := make([]exploreTrendingProjectionRow, 0)
	for timeframe, bySymbol := range byTimeframe {
		for symbol, stat := range bySymbol {
			out = append(out, exploreTrendingProjectionRow{
				Timeframe: timeframe, Symbol: symbol,
				ProfileCount: stat.count, WeightSum: stat.weightSum,
				Top10Count: stat.top10, AssetType: mostCommonAsset(stat.assetCounts),
			})
		}
	}
	return out
}

// exploreFromProjection assembles the response from a bounded, database-side
// snapshot. found=false is a cold-start signal and falls back to the live path.
func (s *Service) exploreFromProjection(
	ctx context.Context,
	callerID string,
	filter ExploreFilter,
	blocked map[string]bool,
) (ExploreResponse, bool, error) {
	store, ok := s.repo.(exploreProjectionRepository)
	if !ok {
		return ExploreResponse{}, false, nil
	}
	snapshot, found, err := store.LoadExploreProjection(
		ctx, filter, blockedIDs(blocked), exploreProjectionCandidateLimit,
	)
	if err != nil || !found {
		return ExploreResponse{}, found, err
	}

	all := make([]scoredCard, 0, len(snapshot.Candidates))
	for _, row := range snapshot.Candidates {
		all = append(all, scoredCard{
			profile: Profile{UserID: row.UserID, UpdatedAt: row.UpdatedAt},
			card:    row.Card,
		})
	}
	similar, err := s.buildSimilar(ctx, callerID, all)
	if err != nil {
		return ExploreResponse{}, true, err
	}
	similarHandles := make(map[string]bool, len(similar))
	for _, card := range similar {
		similarHandles[card.Handle] = true
	}
	featuredCandidates := make([]exploreCandidate, 0, len(all))
	for _, sc := range all {
		featuredCandidates = append(featuredCandidates, candidateFromCard(sc))
	}
	featured := selectFeaturedProfiles(callerID, featuredCandidates, similarHandles, featuredCount, false)

	page := make([]PublicProfile, 0, len(snapshot.Page))
	for _, row := range snapshot.Page {
		page = append(page, row.Card)
	}
	end := filter.Offset + len(page)
	return ExploreResponse{
		Timeframe:        filter.Timeframe,
		Featured:         featured,
		Similar:          similar,
		TopPerformers:    page,
		TrendingHoldings: snapshot.Trending,
		Pagination: ExplorePagination{
			Limit: filter.Limit, Offset: filter.Offset, Total: snapshot.Total,
			HasMore: end < snapshot.Total,
		},
	}, true, nil
}
