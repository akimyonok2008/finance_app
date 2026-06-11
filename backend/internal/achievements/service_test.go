package achievements

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// --- test doubles ------------------------------------------------------------

type fakePositions struct{ m map[string][]portfolio.Position }

func (f fakePositions) ListPositions(_ context.Context, userID string) ([]portfolio.Position, error) {
	return f.m[userID], nil
}

type fakeSummaries struct{ m map[string]*portfolio.PortfolioSummary }

func (f fakeSummaries) GetSummary(_ context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	return f.m[userID], nil
}

type fakeRanks struct{ m map[string]int }

func (f fakeRanks) GetUserRank(_ context.Context, _ /*competitionID*/, userID string) (int, error) {
	return f.m[userID], nil
}

func newService(pos fakePositions, sums fakeSummaries, ranks fakeRanks) (*Service, *InMemoryAchievementRepository) {
	repo := NewInMemoryAchievementRepository()
	return NewService(repo, pos, sums, ranks), repo
}

func find(t *testing.T, list []AchievementResponse, key string) AchievementResponse {
	t.Helper()
	for _, a := range list {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("achievement %q not found", key)
	return AchievementResponse{}
}

// --- tests -------------------------------------------------------------------

func TestList_ReturnsAllSeededAchievements(t *testing.T) {
	svc, _ := newService(fakePositions{}, fakeSummaries{}, fakeRanks{})

	list, err := svc.ListAchievementsForUser(context.Background(), "u1")
	require.NoError(t, err)
	assert.Len(t, list, 5)
}

func TestList_NewUserHasEverythingLocked(t *testing.T) {
	svc, _ := newService(fakePositions{}, fakeSummaries{}, fakeRanks{})

	list, err := svc.ListAchievementsForUser(context.Background(), "u1")
	require.NoError(t, err)
	for _, a := range list {
		assert.Falsef(t, a.Unlocked, "%s should start locked", a.Key)
		assert.Nil(t, a.UnlockedAt)
	}
}

func TestUnlock_Works(t *testing.T) {
	svc, repo := newService(fakePositions{m: map[string][]portfolio.Position{"u1": {{ID: "p1"}}}}, fakeSummaries{}, fakeRanks{})

	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	ach, err := repo.GetAchievementByKey(context.Background(), KeyFirstPortfolio)
	require.NoError(t, err)
	has, err := repo.HasAchievement(context.Background(), "u1", ach.ID)
	require.NoError(t, err)
	assert.True(t, has)
}

func TestUnlock_IsIdempotent(t *testing.T) {
	svc, repo := newService(fakePositions{m: map[string][]portfolio.Position{"u1": {{ID: "p1"}}}}, fakeSummaries{}, fakeRanks{})

	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	userAch, err := repo.ListUserAchievements(context.Background(), "u1")
	require.NoError(t, err)
	count := 0
	for _, ua := range userAch {
		ach, _ := repo.GetAchievementByKey(context.Background(), KeyFirstPortfolio)
		if ua.AchievementID == ach.ID {
			count++
		}
	}
	assert.Equal(t, 1, count, "unlocking twice must not create duplicates")
}

func TestFirstPortfolio_UnlocksWithAtLeastOnePosition(t *testing.T) {
	svc, _ := newService(fakePositions{m: map[string][]portfolio.Position{"u1": {{ID: "p1"}}}}, fakeSummaries{}, fakeRanks{})
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.True(t, find(t, list, KeyFirstPortfolio).Unlocked)
}

func TestFirstPortfolio_LockedWithZeroPositions(t *testing.T) {
	svc, _ := newService(fakePositions{m: map[string][]portfolio.Position{"u1": {}}}, fakeSummaries{}, fakeRanks{})
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.False(t, find(t, list, KeyFirstPortfolio).Unlocked)
}

func TestGreenPortfolio_UnlocksWhenGainPositive(t *testing.T) {
	sums := fakeSummaries{m: map[string]*portfolio.PortfolioSummary{"u1": {GainLossPercentage: 5.0, PortfolioIndex: 105}}}
	svc, _ := newService(fakePositions{}, sums, fakeRanks{})
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.True(t, find(t, list, KeyGreenPortfolio).Unlocked)
}

func TestGreenPortfolio_LockedWhenNotPositive(t *testing.T) {
	sums := fakeSummaries{m: map[string]*portfolio.PortfolioSummary{"u1": {GainLossPercentage: 0, PortfolioIndex: 100}}}
	svc, _ := newService(fakePositions{}, sums, fakeRanks{})
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.False(t, find(t, list, KeyGreenPortfolio).Unlocked)
}

func TestIndex110_UnlocksAtOrAbove110(t *testing.T) {
	sums := fakeSummaries{m: map[string]*portfolio.PortfolioSummary{"u1": {GainLossPercentage: 10, PortfolioIndex: 110}}}
	svc, _ := newService(fakePositions{}, sums, fakeRanks{})
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.True(t, find(t, list, KeyIndex110).Unlocked)
}

func TestIndex110_LockedBelow110(t *testing.T) {
	sums := fakeSummaries{m: map[string]*portfolio.PortfolioSummary{"u1": {GainLossPercentage: 9, PortfolioIndex: 109.99}}}
	svc, _ := newService(fakePositions{}, sums, fakeRanks{})
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.False(t, find(t, list, KeyIndex110).Unlocked)
}

func TestFirstSprint_UnlocksAfterJoin(t *testing.T) {
	svc, _ := newService(fakePositions{}, fakeSummaries{}, fakeRanks{})
	require.NoError(t, svc.EvaluateSprintJoinAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.True(t, find(t, list, KeyFirstSprint).Unlocked)
}

func TestTop10Sprint_UnlocksWhenRankWithinTen(t *testing.T) {
	svc, _ := newService(fakePositions{}, fakeSummaries{}, fakeRanks{m: map[string]int{"u1": 10}})
	require.NoError(t, svc.EvaluateSprintRankAchievements(context.Background(), "u1", "weekly_2026_24"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.True(t, find(t, list, KeyTop10Sprint).Unlocked)
}

func TestTop10Sprint_LockedWhenRankAboveTen(t *testing.T) {
	svc, _ := newService(fakePositions{}, fakeSummaries{}, fakeRanks{m: map[string]int{"u1": 11}})
	require.NoError(t, svc.EvaluateSprintRankAchievements(context.Background(), "u1", "weekly_2026_24"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.False(t, find(t, list, KeyTop10Sprint).Unlocked)
}

func TestTop10Sprint_LockedWhenRankZero(t *testing.T) {
	// rank 0 means "not ranked / not joined" and must not unlock.
	svc, _ := newService(fakePositions{}, fakeSummaries{}, fakeRanks{m: map[string]int{"u1": 0}})
	require.NoError(t, svc.EvaluateSprintRankAchievements(context.Background(), "u1", "weekly_2026_24"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	assert.False(t, find(t, list, KeyTop10Sprint).Unlocked)
}

type fakeCurrent struct{ id string }

func (f fakeCurrent) CurrentCompetitionID(_ context.Context) string { return f.id }

func TestEvaluateAll_UnlocksPortfolioAndReturnsList(t *testing.T) {
	sums := fakeSummaries{m: map[string]*portfolio.PortfolioSummary{"u1": {GainLossPercentage: 12, PortfolioIndex: 112}}}
	svc, _ := newService(fakePositions{m: map[string][]portfolio.Position{"u1": {{ID: "p1"}}}}, sums, fakeRanks{})

	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	byKey := map[string]bool{}
	for _, a := range list {
		byKey[a.Key] = a.Unlocked
	}
	assert.True(t, byKey[KeyFirstPortfolio])
	assert.True(t, byKey[KeyGreenPortfolio])
	assert.True(t, byKey[KeyIndex110])
	assert.False(t, byKey[KeyFirstSprint], "no sprint provider configured ⇒ sprint badges stay locked")
}

func TestEvaluateAll_UnlocksSprintWhenRankedTop10(t *testing.T) {
	svc, _ := newService(fakePositions{}, fakeSummaries{}, fakeRanks{m: map[string]int{"u1": 3}})
	svc.SetCurrentCompetitionProvider(fakeCurrent{id: "weekly_2026_24"})

	list, err := svc.EvaluateAll(context.Background(), "u1")
	require.NoError(t, err)
	byKey := map[string]bool{}
	for _, a := range list {
		byKey[a.Key] = a.Unlocked
	}
	assert.True(t, byKey[KeyFirstSprint])
	assert.True(t, byKey[KeyTop10Sprint])
}

func TestResponse_DoesNotExposeInternalIDs(t *testing.T) {
	svc, _ := newService(fakePositions{m: map[string][]portfolio.Position{"u1": {{ID: "p1"}}}}, fakeSummaries{}, fakeRanks{})
	require.NoError(t, svc.EvaluatePortfolioAchievements(context.Background(), "u1"))

	list, _ := svc.ListAchievementsForUser(context.Background(), "u1")
	raw, err := json.Marshal(list)
	require.NoError(t, err)
	body := string(raw)
	assert.NotContains(t, body, `"id"`)
	assert.NotContains(t, body, `"achievement_id"`)
	assert.NotContains(t, body, `"user_id"`)
}
