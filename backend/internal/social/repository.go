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
	ListMessages(ctx context.Context, conversationID string, limit int) ([]Message, error)
	LastMessage(ctx context.Context, conversationID string) (Message, bool, error)
}

type Friendship struct {
	UserID       string
	FriendsSince time.Time
}
