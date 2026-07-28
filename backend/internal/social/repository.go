package social

import (
	"context"
	"time"
)

type Repository interface {
	Follow(ctx context.Context, followerUserID, followingUserID string) error
	Unfollow(ctx context.Context, followerUserID, followingUserID string) error
	IsFollowing(ctx context.Context, followerUserID, followingUserID string) (bool, error)
	ListFollowing(ctx context.Context, userID string) ([]Follow, error)
	ListFollowers(ctx context.Context, userID string) ([]Follow, error)
	ListMutualFriends(ctx context.Context, userID string) ([]Friendship, error)

	GetConversationBetween(ctx context.Context, userA, userB string) (Conversation, bool, error)
	CreateConversation(ctx context.Context, id, userA, userB string) (Conversation, error)
	GetConversation(ctx context.Context, conversationID string) (Conversation, error)
	ListConversations(ctx context.Context, userID string) ([]Conversation, error)
	IsParticipant(ctx context.Context, conversationID, userID string) (bool, error)
	OtherParticipant(ctx context.Context, conversationID, userID string) (string, error)
	AddMessage(ctx context.Context, msg Message) error
	// CreateMessage atomically enforces sender/conversation abuse policy,
	// persists the message, and creates its notification.
	CreateMessage(ctx context.Context, write MessageWrite, policy MessageAbusePolicy) error
	// ListMessages returns visible messages for viewerID: messages that
	// viewerID has hidden-for-me are excluded; moderator-removed messages are
	// still returned but with a tombstone body (see Service.messageDTO).
	ListMessages(ctx context.Context, conversationID, viewerID string, limit int) ([]Message, error)
	LastMessage(ctx context.Context, conversationID string) (Message, bool, error)
	MessageByID(ctx context.Context, messageID string) (Message, error)

	// HideMessageForUser hides a message from just userID's own view.
	// Idempotent.
	HideMessageForUser(ctx context.Context, messageID, userID string) error
	// RemoveMessage applies a moderator content-removal tombstone. The raw
	// text is not deleted (evidence retention), only hidden from normal reads.
	RemoveMessage(ctx context.Context, messageID, moderatorID string) error

	// MarkRead advances userID's read position in a conversation.
	MarkRead(ctx context.Context, conversationID, userID, lastReadMessageID string) error
	// ConversationUnreadCount counts messages after userID's read position,
	// authored by someone else, excluding hidden-for-user and removed
	// messages. A conversation the user has blocked/declined never
	// contributes to this count.
	ConversationUnreadCount(ctx context.Context, conversationID, userID string) (int, error)
	// UnreadCount sums ConversationUnreadCount across every active
	// conversation userID participates in.
	UnreadCount(ctx context.Context, userID string) (int, error)
}

type Friendship struct {
	UserID       string
	FriendsSince time.Time
}
