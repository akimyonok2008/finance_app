package social

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePolicy lets tests toggle whether two users may interact, standing in
// for safety.Service.CanUsersInteract.
type fakePolicy struct {
	blocked map[string]bool
}

func (p *fakePolicy) CanUsersInteract(_ context.Context, a, b string) (bool, error) {
	key := a + "|" + b
	rev := b + "|" + a
	if p.blocked[key] || p.blocked[rev] {
		return false, nil
	}
	return true, nil
}

func friendPair(t *testing.T, svc *Service, a, b string) {
	t.Helper()
	require.NoError(t, svc.repo.Follow(context.Background(), a, b))
	require.NoError(t, svc.repo.Follow(context.Background(), b, a))
}

func TestService_InteractionPolicyBlocksFollowAndMessage(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	policy := &fakePolicy{blocked: map[string]bool{"user-a|user-b": true}}
	svc.SetInteractionPolicy(policy)

	_, err := svc.Follow(ctx, "user-a", "beta")
	assert.ErrorIs(t, err, ErrInteractionBlocked)

	friendPair(t, svc, "user-a", "user-c")
	_, err = svc.CreateConversation(ctx, "user-a", "beta")
	assert.ErrorIs(t, err, ErrInteractionBlocked)

	// Unblocked pair still works normally.
	convo, err := svc.CreateConversation(ctx, "user-a", "charlie")
	require.NoError(t, err)
	_, err = svc.SendMessage(ctx, "user-a", convo.Conversation.ID, "hello")
	require.NoError(t, err)
}

func TestService_HideMessageOnlyAffectsHiderView(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	friendPair(t, svc, "user-a", "user-b")
	convo, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)
	msg, err := svc.SendMessage(ctx, "user-a", convo.Conversation.ID, "hello there")
	require.NoError(t, err)

	require.NoError(t, svc.HideMessage(ctx, "user-a", msg.Message.ID))
	// Idempotent.
	require.NoError(t, svc.HideMessage(ctx, "user-a", msg.Message.ID))

	fromHider, err := svc.Messages(ctx, "user-a", convo.Conversation.ID, 10)
	require.NoError(t, err)
	assert.Empty(t, fromHider.Messages)

	fromOther, err := svc.Messages(ctx, "user-b", convo.Conversation.ID, 10)
	require.NoError(t, err)
	require.Len(t, fromOther.Messages, 1)
	assert.Equal(t, "hello there", fromOther.Messages[0].Body)
}

func TestService_HideMessageRequiresParticipant(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	friendPair(t, svc, "user-a", "user-b")
	convo, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)
	msg, err := svc.SendMessage(ctx, "user-a", convo.Conversation.ID, "hi")
	require.NoError(t, err)

	err = svc.HideMessage(ctx, "user-c", msg.Message.ID)
	assert.ErrorIs(t, err, ErrForbidden)
}

func TestService_ModeratorRemovalCreatesTombstone(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	friendPair(t, svc, "user-a", "user-b")
	convo, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)
	msg, err := svc.SendMessage(ctx, "user-a", convo.Conversation.ID, "abusive text")
	require.NoError(t, err)

	require.NoError(t, svc.RemoveMessage(ctx, "moderator-1", msg.Message.ID))

	out, err := svc.Messages(ctx, "user-b", convo.Conversation.ID, 10)
	require.NoError(t, err)
	require.Len(t, out.Messages, 1)
	assert.True(t, out.Messages[0].Removed)
	assert.Equal(t, "This message was removed.", out.Messages[0].Body)
	assert.NotContains(t, out.Messages[0].Body, "abusive text")
}

func TestService_UnreadCounts(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time {
		now = now.Add(time.Second)
		return now
	}
	friendPair(t, svc, "user-a", "user-b")
	convo, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)

	msg1, err := svc.SendMessage(ctx, "user-a", convo.Conversation.ID, "first")
	require.NoError(t, err)
	_, err = svc.SendMessage(ctx, "user-a", convo.Conversation.ID, "second")
	require.NoError(t, err)

	// Sender's own messages never count as unread for the sender.
	senderUnread, err := svc.UnreadCount(ctx, "user-a")
	require.NoError(t, err)
	assert.Equal(t, 0, senderUnread.Count)

	recipientUnread, err := svc.UnreadCount(ctx, "user-b")
	require.NoError(t, err)
	assert.Equal(t, 2, recipientUnread.Count)

	require.NoError(t, svc.MarkRead(ctx, "user-b", convo.Conversation.ID, msg1.Message.ID))
	afterPartialRead, err := svc.UnreadCount(ctx, "user-b")
	require.NoError(t, err)
	assert.Equal(t, 1, afterPartialRead.Count)
}

func TestService_MessageForReportRequiresParticipant(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	friendPair(t, svc, "user-a", "user-b")
	convo, err := svc.CreateConversation(ctx, "user-a", "beta")
	require.NoError(t, err)
	msg, err := svc.SendMessage(ctx, "user-a", convo.Conversation.ID, "hi")
	require.NoError(t, err)

	evidence, err := svc.MessageForReport(ctx, "user-b", msg.Message.ID)
	require.NoError(t, err)
	assert.Equal(t, "user-a", evidence.SenderID)
	assert.Equal(t, "hi", evidence.Text)

	_, err = svc.MessageForReport(ctx, "user-c", msg.Message.ID)
	assert.Error(t, err)
}

func TestService_BlockedFilterExcludesFollowersAndFriends(t *testing.T) {
	ctx := context.Background()
	svc := newTestSocialService()
	friendPair(t, svc, "user-a", "user-b")
	svc.SetBlockedFilter(&fakeBlockedSource{blocked: map[string]map[string]bool{"user-a": {"user-b": true}}})

	friends, err := svc.Friends(ctx, "user-a")
	require.NoError(t, err)
	assert.Empty(t, friends.Friends)
}

type fakeBlockedSource struct {
	blocked map[string]map[string]bool
}

func (f *fakeBlockedSource) BlockedPairUserIDs(_ context.Context, userID string) (map[string]bool, error) {
	return f.blocked[userID], nil
}
