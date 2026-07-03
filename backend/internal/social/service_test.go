package social

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestService_FollowLifecycleAndFriends(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()

	state, err := svc.Follow(ctx, "user-a", "beta")
	require.NoError(t, err)
	assert.True(t, state.IsFollowing)
	assert.False(t, state.FollowsMe)
	assert.False(t, state.IsFriend)

	state, err = svc.Follow(ctx, "user-a", "beta")
	require.NoError(t, err)
	assert.True(t, state.IsFollowing)

	_, err = svc.Follow(ctx, "user-a", "alpha")
	assert.ErrorIs(t, err, ErrSelfFollow)

	_, err = svc.Follow(ctx, "user-b", "alpha")
	require.NoError(t, err)

	state, err = svc.FollowState(ctx, "user-a", "beta")
	require.NoError(t, err)
	assert.True(t, state.IsFriend)

	friends, err := svc.Friends(ctx, "user-a")
	require.NoError(t, err)
	require.Len(t, friends.Friends, 1)
	assert.Equal(t, "beta", friends.Friends[0].Handle)

	state, err = svc.Unfollow(ctx, "user-a", "beta")
	require.NoError(t, err)
	assert.False(t, state.IsFollowing)
	assert.True(t, state.FollowsMe)
	assert.False(t, state.IsFriend)
}

func TestService_FollowLists(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	require.NoError(t, svc.repo.Follow(ctx, "user-a", "user-b"))
	require.NoError(t, svc.repo.Follow(ctx, "user-c", "user-a"))

	following, err := svc.Following(ctx, "user-a")
	require.NoError(t, err)
	require.Len(t, following.Users, 1)
	assert.Equal(t, "beta", following.Users[0].Handle)

	followers, err := svc.Followers(ctx, "user-a")
	require.NoError(t, err)
	require.Len(t, followers.Users, 1)
	assert.Equal(t, "charlie", followers.Users[0].Handle)
}

func TestService_DMRequiresMutualFollow(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	require.NoError(t, svc.repo.Follow(ctx, "user-a", "user-b"))

	_, err := svc.CreateConversation(ctx, "user-a", "beta")
	assert.ErrorIs(t, err, ErrNotFriends)

	require.NoError(t, svc.repo.Follow(ctx, "user-b", "user-a"))
	resp, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Conversation.ID)

	again, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)
	assert.Equal(t, resp.Conversation.ID, again.Conversation.ID)

	_, err = svc.CreateConversation(ctx, "user-a", "alpha")
	assert.ErrorIs(t, err, ErrSelfDM)
}

func TestService_MessagesParticipantAndFriendshipRules(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	require.NoError(t, svc.repo.Follow(ctx, "user-a", "user-b"))
	require.NoError(t, svc.repo.Follow(ctx, "user-b", "user-a"))
	conv, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)

	_, err = svc.Messages(ctx, "user-c", conv.Conversation.ID, 50)
	assert.ErrorIs(t, err, ErrForbidden)

	_, err = svc.SendMessage(ctx, "user-a", conv.Conversation.ID, "   ")
	assert.ErrorIs(t, err, ErrInvalidMessage)

	_, err = svc.SendMessage(ctx, "user-a", conv.Conversation.ID, strings.Repeat("x", MaxMessageLength+1))
	assert.ErrorIs(t, err, ErrMessageTooLong)

	msg, err := svc.SendMessage(ctx, "user-a", conv.Conversation.ID, " Nice portfolio structure. ")
	require.NoError(t, err)
	assert.Equal(t, "Nice portfolio structure.", msg.Message.Body)
	assert.True(t, msg.Message.SentByMe)

	messages, err := svc.Messages(ctx, "user-b", conv.Conversation.ID, 50)
	require.NoError(t, err)
	require.Len(t, messages.Messages, 1)
	assert.False(t, messages.Messages[0].SentByMe)

	require.NoError(t, svc.repo.Unfollow(ctx, "user-a", "user-b"))
	_, err = svc.SendMessage(ctx, "user-b", conv.Conversation.ID, "Still there?")
	assert.ErrorIs(t, err, ErrNotFriends)
}

func TestService_DTOPrivacy(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	require.NoError(t, svc.repo.Follow(ctx, "user-a", "user-b"))
	require.NoError(t, svc.repo.Follow(ctx, "user-b", "user-a"))
	conv, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)
	_, err = svc.SendMessage(ctx, "user-a", conv.Conversation.ID, "hello")
	require.NoError(t, err)
	out, err := svc.Conversations(ctx, "user-b")
	require.NoError(t, err)

	raw, err := json.Marshal(out)
	require.NoError(t, err)
	body := string(raw)
	for _, forbidden := range []string{
		"email", "user_id", "portfolio_id", "position_id", "portfolio_value",
		"cost_basis", "quantity", "average_buy_price", "gain_loss", "brokerage",
	} {
		assert.NotContains(t, body, forbidden)
	}
}

func newTestSocialService() *Service {
	repo := NewInMemoryRepository()
	profiles := &fakeProfileRepo{byID: map[string]profile.Profile{}}
	for _, p := range []profile.Profile{
		{UserID: "user-a", Handle: "alpha", DisplayName: "Alpha", AvatarKey: "owl", StrategyTag: "growth", IsPublic: true},
		{UserID: "user-b", Handle: "beta", DisplayName: "Beta", AvatarKey: "fox", StrategyTag: "balanced_global", IsPublic: true},
		{UserID: "user-c", Handle: "charlie", DisplayName: "Charlie", AvatarKey: "bear", StrategyTag: "value", IsPublic: true},
		{UserID: "user-private", Handle: "private", DisplayName: "Private", AvatarKey: "", StrategyTag: "value", IsPublic: false},
	} {
		profiles.byID[p.UserID] = p
	}
	svc := NewService(repo, profiles)
	return svc
}

type fakeProfileRepo struct {
	byID map[string]profile.Profile
}

func (r *fakeProfileRepo) GetByUserID(_ context.Context, userID string) (profile.Profile, error) {
	p, ok := r.byID[userID]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return p, nil
}

func (r *fakeProfileRepo) GetByHandle(_ context.Context, handle string) (profile.Profile, error) {
	for _, p := range r.byID {
		if p.Handle == handle {
			return p, nil
		}
	}
	return profile.Profile{}, profile.ErrNotFound
}

var _ ProfileRepository = (*fakeProfileRepo)(nil)
