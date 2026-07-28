package social

import (
	"context"
	"time"
)

const MaxMessageLength = 1000

var defaultMessageAbusePolicy = MessageAbusePolicy{
	UserLimit: 30, UserWindow: time.Minute,
	ConversationLimit: 40, ConversationWindow: time.Minute,
	RepeatedLimit: 3, RepeatedWindow: 10 * time.Minute,
	BurstLimit: 8, BurstWindow: 10 * time.Second,
}

type MessageAbusePolicy struct {
	UserLimit          int
	UserWindow         time.Duration
	ConversationLimit  int
	ConversationWindow time.Duration
	RepeatedLimit      int
	RepeatedWindow     time.Duration
	BurstLimit         int
	BurstWindow        time.Duration
}

type SafeProfile struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarKey   string `json:"avatar_key"`
	StrategyTag string `json:"strategy_tag"`
}

type Follow struct {
	FollowerUserID  string
	FollowingUserID string
	CreatedAt       time.Time
}

type FollowState struct {
	Handle      string `json:"handle"`
	IsFollowing bool   `json:"is_following"`
	FollowsMe   bool   `json:"follows_me"`
	IsFriend    bool   `json:"is_friend"`
}

type FriendItem struct {
	Handle       string    `json:"handle"`
	DisplayName  string    `json:"display_name"`
	AvatarKey    string    `json:"avatar_key"`
	StrategyTag  string    `json:"strategy_tag"`
	FriendsSince time.Time `json:"friends_since"`
}

type FriendsResponse struct {
	Friends []FriendItem `json:"friends"`
}

type UserListResponse struct {
	Users []FriendItem `json:"users"`
}

type Conversation struct {
	ID            string
	ParticipantA  string
	ParticipantB  string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	LastMessageAt *time.Time
}

type Message struct {
	ID             string
	ConversationID string
	SenderUserID   string
	Body           string
	CreatedAt      time.Time
	RemovedAt      *time.Time
	RemovedBy      *string
}

type MessageNotification struct {
	ID          string
	RecipientID string
	Type        string
	DedupeKey   string
	Payload     map[string]string
	CreatedAt   time.Time
}

type MessageWrite struct {
	Message      Message
	Notification *MessageNotification
	// VolatileNotify is used only by the in-memory repository. PostgreSQL
	// persists Notification in the same transaction as Message.
	VolatileNotify func(context.Context) error
}

// IsRemoved reports whether a moderator tombstoned this message.
func (m Message) IsRemoved() bool { return m.RemovedAt != nil }

const removedTombstoneText = "This message was removed."

type LastMessagePreview struct {
	BodyPreview string    `json:"body_preview"`
	SentAt      time.Time `json:"sent_at"`
	SentByMe    bool      `json:"sent_by_me"`
}

type ConversationSummary struct {
	ID          string              `json:"id"`
	OtherUser   SafeProfile         `json:"other_user"`
	LastMessage *LastMessagePreview `json:"last_message,omitempty"`
	UpdatedAt   time.Time           `json:"updated_at"`
	UnreadCount int                 `json:"unread_count"`
}

type ConversationResponse struct {
	Conversation ConversationSummary `json:"conversation"`
}

type ConversationsResponse struct {
	Conversations []ConversationSummary `json:"conversations"`
}

type MessageDTO struct {
	ID             string      `json:"id"`
	ConversationID string      `json:"conversation_id"`
	Sender         SafeProfile `json:"sender"`
	Body           string      `json:"body"`
	Removed        bool        `json:"removed"`
	SentByMe       bool        `json:"sent_by_me"`
	CreatedAt      time.Time   `json:"created_at"`
}

type MarkReadInput struct {
	LastReadMessageID string `json:"last_read_message_id"`
}

type UnreadCountResponse struct {
	Count int `json:"count"`
}

type MessageResponse struct {
	Message MessageDTO `json:"message"`
}

type MessagesResponse struct {
	Messages []MessageDTO `json:"messages"`
}

type CreateConversationInput struct {
	Handle string `json:"handle"`
}

type SendMessageInput struct {
	Body string `json:"body"`
}
