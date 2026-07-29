package profile

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

type projectionTestRepository struct {
	*InMemoryRepository
	snapshot  ExploreProjectionSnapshot
	found     bool
	loadCalls int
	listCalls int
	replaced  []ExploreProjectionRow
}

func (r *projectionTestRepository) ListPublicProfiles(ctx context.Context) ([]Profile, error) {
	r.listCalls++
	return r.InMemoryRepository.ListPublicProfiles(ctx)
}

func (r *projectionTestRepository) ReplaceExploreProjection(_ context.Context, rows []ExploreProjectionRow) error {
	r.replaced = append([]ExploreProjectionRow(nil), rows...)
	return nil
}

func (r *projectionTestRepository) LoadExploreProjection(
	_ context.Context,
	_ ExploreFilter,
	_ []string,
	_ int,
) (ExploreProjectionSnapshot, bool, error) {
	r.loadCalls++
	return r.snapshot, r.found, nil
}

func projectedCard(handle string, rank int) PublicProfile {
	return PublicProfile{
		Handle: handle, DisplayName: handle, PortfolioIndex: 110, ReturnPercentage: 10,
		GlobalRank: &rank,
		PublicWeights: []PublicWeight{
			{Symbol: "AAPL", AssetType: "stock", Weight: 60},
			{Symbol: "MSFT", AssetType: "stock", Weight: 40},
		},
	}
}

func TestExplore_ProjectionPathLoadsOnlyBoundedSnapshot(t *testing.T) {
	repo := &projectionTestRepository{
		InMemoryRepository: NewInMemoryRepository(),
		found:              true,
		snapshot: ExploreProjectionSnapshot{
			Page: []ExploreProjectionRow{
				{UserID: "page-user", Card: projectedCard("page_user", 250)},
			},
			Candidates: []ExploreProjectionRow{
				{UserID: "candidate", Card: projectedCard("candidate", 1)},
			},
			Trending: []TrendingHolding{{Symbol: "AAPL", ProfileCount: 5000}},
			Total:    5000,
		},
	}
	svc := NewService(repo, testUsers{}, testSummaries{})

	out, err := svc.Explore(context.Background(), "", ExploreFilter{
		Sort: SortTop, Timeframe: TimeframeAll, Limit: 1, Offset: 249,
	})
	require.NoError(t, err)
	require.Len(t, out.TopPerformers, 1)
	assert.Equal(t, "page_user", out.TopPerformers[0].Handle)
	assert.Equal(t, 5000, out.Pagination.Total)
	assert.True(t, out.Pagination.HasMore)
	assert.Equal(t, 1, repo.loadCalls)
	assert.Zero(t, repo.listCalls, "request path must not enumerate public profiles")
}

func TestRefreshExploreProjectionBuildsAllTimeframesOffRequestPath(t *testing.T) {
	ctx := context.Background()
	repo := &projectionTestRepository{InMemoryRepository: NewInMemoryRepository()}
	now := time.Now().UTC()
	require.NoError(t, repo.Create(ctx, Profile{
		UserID: "u1", Handle: "projected_user", DisplayName: "Projected User",
		StrategyTag: DefaultStrategyTag, IsPublic: true, ShowPublicWeights: true,
		CreatedAt: now, UpdatedAt: now,
	}))
	summaries := testSummaries{"u1": {
		UserID: "u1", CurrentValue: money.AmountFromFloat64(100),
		Positions: []portfolio.PositionSummary{
			{PositionID: "p1", Symbol: "AAPL", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(60)},
			{PositionID: "p2", Symbol: "MSFT", AssetType: "stock", CurrentValueBase: money.AmountFromFloat64(40)},
		},
	}}
	ranks := fakeTimeframeRanks{}
	for _, timeframe := range exploreTimeframes {
		ranks[timeframe] = []TimeframeRanking{{
			UserID: "u1", Rank: 1, RankedIndex: 110, RankedReturnPercentage: 10,
		}}
	}
	svc := NewService(repo, testUsers{"u1": {ID: "u1"}}, summaries)
	svc.SetTimeframeRankProvider(ranks)

	count, err := svc.RefreshExploreProjection(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(exploreTimeframes), count)
	require.Len(t, repo.replaced, len(exploreTimeframes))
	for _, row := range repo.replaced {
		assert.Equal(t, "u1", row.UserID)
		assert.Equal(t, 1, *row.Card.GlobalRank)
		assert.Len(t, row.Card.PublicWeights, 2)
	}
}
