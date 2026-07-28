package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/ardakimyonok/finance_app/internal/social"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryAccountDeletionHooksEraseOwnedAndSharedSurfaceData(t *testing.T) {
	ctx := context.Background()
	const userID = "delete-me"
	const otherUserID = "keep-me"

	portfolioRepo := portfolio.NewInMemoryRepository()
	profileRepo := profile.NewInMemoryRepository()
	socialRepo := social.NewInMemoryRepository()
	achievementRepo := achievements.NewInMemoryAchievementRepository()
	competitionRepo := competitions.NewInMemoryCompetitionRepository()

	_, err := portfolioRepo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)
	require.NoError(t, profileRepo.Create(ctx, profile.Profile{
		UserID: userID, Handle: "delete-me", DisplayName: "Delete Me",
		IsPublic: true, ShowPublicWeights: true,
	}))
	require.NoError(t, socialRepo.Follow(ctx, userID, otherUserID))
	require.NoError(t, socialRepo.Follow(ctx, otherUserID, userID))
	_, err = socialRepo.CreateConversation(ctx, "conversation-1", userID, otherUserID)
	require.NoError(t, err)
	require.NoError(t, socialRepo.AddMessage(ctx, social.Message{
		ID: "message-1", ConversationID: "conversation-1", SenderUserID: userID,
		Body: "personal content", CreatedAt: time.Now().UTC(),
	}))
	require.NoError(t, achievementRepo.Award(ctx, achievements.AwardedAchievement{
		UserID: userID, BadgeKey: "test-badge",
	}))
	require.NoError(t, competitionRepo.CreateEntry(ctx, competitions.CompetitionEntry{
		ID: "entry-1", CompetitionID: "competition-1", UserID: userID,
	}))

	hooks := []auth.AccountDeletionHook{
		portfolioRepo, profileRepo, socialRepo, achievementRepo, competitionRepo,
	}
	for _, hook := range hooks {
		require.NoError(t, hook.OnAccountDeleted(ctx, userID))
	}

	_, err = portfolioRepo.GetPortfolioByUser(ctx, userID)
	assert.ErrorIs(t, err, portfolio.ErrPortfolioNotFound)
	_, err = profileRepo.GetByUserID(ctx, userID)
	assert.ErrorIs(t, err, profile.ErrNotFound)
	followsDeletedUser, err := socialRepo.IsFollowing(ctx, otherUserID, userID)
	require.NoError(t, err)
	assert.False(t, followsDeletedUser)
	deletedUserFollows, err := socialRepo.IsFollowing(ctx, userID, otherUserID)
	require.NoError(t, err)
	assert.False(t, deletedUserFollows)
	_, err = socialRepo.GetConversation(ctx, "conversation-1")
	assert.True(t, errors.Is(err, social.ErrConversationNotFound))
	awarded, err := achievementRepo.ListAwarded(ctx, userID)
	require.NoError(t, err)
	assert.Empty(t, awarded)
	_, err = competitionRepo.GetEntry(ctx, "competition-1", userID)
	assert.ErrorIs(t, err, competitions.ErrEntryNotFound)
}
