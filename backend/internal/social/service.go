package social

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/pairlock"
	"github.com/ardakimyonok/finance_app/internal/profile"
)

type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID string) (profile.Profile, error)
	GetByHandle(ctx context.Context, handle string) (profile.Profile, error)
}

// InteractionPolicy is the canonical cross-cutting gate (blocking +
// suspension/ban) checked before follow and DM actions. It is the single
// source of truth shared with profile view/search/Explore — see
// safety.Service.CanUsersInteract.
type InteractionPolicy interface {
	CanUsersInteract(ctx context.Context, userA, userB string) (bool, error)
}

// NotificationCreator lets Service raise a lightweight in-app "new_message"
// notification without importing the moderation package directly.
type NotificationCreator interface {
	Notify(ctx context.Context, recipientID, notifType, dedupeKey string, payload map[string]string) error
}

type Service struct {
	repo     Repository
	profiles ProfileRepository
	policy   InteractionPolicy
	blocked  BlockedPairSource
	notifier NotificationCreator
	now      func() time.Time
}

func NewService(repo Repository, profiles ProfileRepository) *Service {
	return &Service{
		repo:     repo,
		profiles: profiles,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// BlockedPairSource exposes the caller's block-set so following/followers/
// friends lists can be filtered at the service level (never fetched
// unfiltered and hidden only in a handler or the browser).
type BlockedPairSource interface {
	BlockedPairUserIDs(ctx context.Context, userID string) (map[string]bool, error)
}

// SetInteractionPolicy attaches the canonical block/suspension/ban gate.
func (s *Service) SetInteractionPolicy(p InteractionPolicy) { s.policy = p }

// SetBlockedFilter attaches the block-set source used to filter
// following/followers/friends lists.
func (s *Service) SetBlockedFilter(b BlockedPairSource) { s.blocked = b }

// SetNotificationCreator attaches the notification sink used for new_message
// events.
func (s *Service) SetNotificationCreator(n NotificationCreator) { s.notifier = n }

func (s *Service) checkInteraction(ctx context.Context, a, b string) error {
	if s.policy == nil {
		return nil
	}
	ok, err := s.policy.CanUsersInteract(ctx, a, b)
	if err != nil {
		return err
	}
	if !ok {
		return ErrInteractionBlocked
	}
	return nil
}

func (s *Service) Follow(ctx context.Context, userID, handle string) (FollowState, error) {
	target, err := s.publicProfileByHandle(ctx, handle)
	if err != nil {
		return FollowState{}, err
	}
	if target.UserID == userID {
		return FollowState{}, ErrSelfFollow
	}
	// Serialize against a concurrent Block for the same pair (see
	// pairlock and safety.Service.Block) so a follow can never be created
	// after a block has been checked-and-passed but before it commits.
	unlock := pairlock.Lock(userID, target.UserID)
	defer unlock()
	if err := s.checkInteraction(ctx, userID, target.UserID); err != nil {
		return FollowState{}, err
	}
	if err := s.repo.Follow(ctx, userID, target.UserID); err != nil {
		return FollowState{}, err
	}
	return s.followStateForTarget(ctx, userID, target)
}

func (s *Service) Unfollow(ctx context.Context, userID, handle string) (FollowState, error) {
	target, err := s.profileByHandle(ctx, handle)
	if err != nil {
		return FollowState{}, err
	}
	if target.UserID == userID {
		return FollowState{}, ErrSelfFollow
	}
	if err := s.repo.Unfollow(ctx, userID, target.UserID); err != nil {
		return FollowState{}, err
	}
	return s.followStateForTarget(ctx, userID, target)
}

func (s *Service) FollowState(ctx context.Context, userID, handle string) (FollowState, error) {
	target, err := s.profileByHandle(ctx, handle)
	if err != nil {
		return FollowState{}, err
	}
	if target.UserID == userID {
		return FollowState{Handle: target.Handle}, nil
	}
	return s.followStateForTarget(ctx, userID, target)
}

// blockedSet returns the caller's block-set. It fails closed: if the
// underlying safety store cannot be queried, callers must treat this as a
// hard error rather than silently proceeding with an unfiltered (or empty)
// set, since either could leak a blocked user into a list/Explore surface.
func (s *Service) blockedSet(ctx context.Context, userID string) (map[string]bool, error) {
	if s.blocked == nil {
		return nil, nil
	}
	set, err := s.blocked.BlockedPairUserIDs(ctx, userID)
	if err != nil {
		return nil, ErrSafetyUnavailable
	}
	return set, nil
}

func (s *Service) Following(ctx context.Context, userID string) (UserListResponse, error) {
	items, err := s.repo.ListFollowing(ctx, userID)
	if err != nil {
		return UserListResponse{}, err
	}
	blocked, err := s.blockedSet(ctx, userID)
	if err != nil {
		return UserListResponse{}, err
	}
	out := make([]FriendItem, 0, len(items))
	for _, item := range items {
		if blocked[item.FollowingUserID] {
			continue
		}
		if p, err := s.profiles.GetByUserID(ctx, item.FollowingUserID); err == nil {
			out = append(out, friendItem(p, item.CreatedAt))
		}
	}
	return UserListResponse{Users: out}, nil
}

func (s *Service) Followers(ctx context.Context, userID string) (UserListResponse, error) {
	items, err := s.repo.ListFollowers(ctx, userID)
	if err != nil {
		return UserListResponse{}, err
	}
	blocked, err := s.blockedSet(ctx, userID)
	if err != nil {
		return UserListResponse{}, err
	}
	out := make([]FriendItem, 0, len(items))
	for _, item := range items {
		if blocked[item.FollowerUserID] {
			continue
		}
		if p, err := s.profiles.GetByUserID(ctx, item.FollowerUserID); err == nil {
			out = append(out, friendItem(p, item.CreatedAt))
		}
	}
	return UserListResponse{Users: out}, nil
}

func (s *Service) Friends(ctx context.Context, userID string) (FriendsResponse, error) {
	items, err := s.repo.ListMutualFriends(ctx, userID)
	if err != nil {
		return FriendsResponse{}, err
	}
	blocked, err := s.blockedSet(ctx, userID)
	if err != nil {
		return FriendsResponse{}, err
	}
	out := make([]FriendItem, 0, len(items))
	for _, item := range items {
		if blocked[item.UserID] {
			continue
		}
		if p, err := s.profiles.GetByUserID(ctx, item.UserID); err == nil {
			out = append(out, friendItem(p, item.FriendsSince))
		}
	}
	return FriendsResponse{Friends: out}, nil
}

func (s *Service) Conversations(ctx context.Context, userID string) (ConversationsResponse, error) {
	items, err := s.repo.ListConversations(ctx, userID)
	if err != nil {
		return ConversationsResponse{}, err
	}
	blocked, err := s.blockedSet(ctx, userID)
	if err != nil {
		return ConversationsResponse{}, err
	}
	out := make([]ConversationSummary, 0, len(items))
	for _, c := range items {
		other := c.ParticipantA
		if other == userID {
			other = c.ParticipantB
		}
		if blocked[other] {
			continue
		}
		dto, err := s.conversationSummary(ctx, userID, c)
		if err != nil {
			continue
		}
		out = append(out, dto)
	}
	return ConversationsResponse{Conversations: out}, nil
}

func (s *Service) CreateConversation(ctx context.Context, userID, handle string) (ConversationResponse, error) {
	target, err := s.profileByHandle(ctx, handle)
	if err != nil {
		return ConversationResponse{}, err
	}
	if target.UserID == userID {
		return ConversationResponse{}, ErrSelfDM
	}
	if err := s.checkInteraction(ctx, userID, target.UserID); err != nil {
		return ConversationResponse{}, err
	}
	if ok, err := s.areFriends(ctx, userID, target.UserID); err != nil {
		return ConversationResponse{}, err
	} else if !ok {
		return ConversationResponse{}, ErrNotFriends
	}
	c, ok, err := s.repo.GetConversationBetween(ctx, userID, target.UserID)
	if err != nil {
		return ConversationResponse{}, err
	}
	if !ok {
		c, err = s.repo.CreateConversation(ctx, uuid.NewString(), userID, target.UserID)
		if err != nil {
			return ConversationResponse{}, err
		}
	}
	dto, err := s.conversationSummary(ctx, userID, c)
	if err != nil {
		return ConversationResponse{}, err
	}
	return ConversationResponse{Conversation: dto}, nil
}

func (s *Service) Messages(ctx context.Context, userID, conversationID string, limit int) (MessagesResponse, error) {
	ok, err := s.repo.IsParticipant(ctx, conversationID, userID)
	if err != nil {
		return MessagesResponse{}, err
	}
	if !ok {
		return MessagesResponse{}, ErrForbidden
	}
	items, err := s.repo.ListMessages(ctx, conversationID, userID, limit)
	if err != nil {
		return MessagesResponse{}, err
	}
	out := make([]MessageDTO, 0, len(items))
	for _, item := range items {
		dto, err := s.messageDTO(ctx, userID, item)
		if err != nil {
			continue
		}
		out = append(out, dto)
	}
	return MessagesResponse{Messages: out}, nil
}

func (s *Service) SendMessage(ctx context.Context, userID, conversationID, body string) (MessageResponse, error) {
	c, err := s.repo.GetConversation(ctx, conversationID)
	if err != nil {
		return MessageResponse{}, err
	}
	if c.ParticipantA != userID && c.ParticipantB != userID {
		return MessageResponse{}, ErrForbidden
	}
	other := c.ParticipantA
	if other == userID {
		other = c.ParticipantB
	}
	if err := s.checkInteraction(ctx, userID, other); err != nil {
		return MessageResponse{}, err
	}
	if ok, err := s.areFriends(ctx, userID, other); err != nil {
		return MessageResponse{}, err
	} else if !ok {
		return MessageResponse{}, ErrNotFriends
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return MessageResponse{}, ErrInvalidMessage
	}
	if utf8.RuneCountInString(body) > MaxMessageLength {
		return MessageResponse{}, ErrMessageTooLong
	}
	msg := Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		SenderUserID:   userID,
		Body:           body,
		CreatedAt:      s.now(),
	}
	write := MessageWrite{Message: msg}
	if s.notifier != nil {
		notification := &MessageNotification{
			ID: uuid.NewString(), RecipientID: other, Type: "new_message",
			DedupeKey: "message:" + msg.ID,
			Payload:   map[string]string{"conversation_id": conversationID},
			CreatedAt: msg.CreatedAt,
		}
		write.Notification = notification
		write.VolatileNotify = func(ctx context.Context) error {
			return s.notifier.Notify(
				ctx, notification.RecipientID, notification.Type,
				notification.DedupeKey, notification.Payload,
			)
		}
	}
	if err := s.repo.CreateMessage(ctx, write, defaultMessageAbusePolicy); err != nil {
		return MessageResponse{}, err
	}
	dto, err := s.messageDTO(ctx, userID, msg)
	if err != nil {
		return MessageResponse{}, err
	}
	return MessageResponse{Message: dto}, nil
}

// HideMessage removes a message from userID's own conversation view without
// affecting the other participant. Idempotent; only a conversation
// participant may hide a message.
func (s *Service) HideMessage(ctx context.Context, userID, messageID string) error {
	msg, err := s.repo.MessageByID(ctx, messageID)
	if err != nil {
		return err
	}
	ok, err := s.repo.IsParticipant(ctx, msg.ConversationID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return s.repo.HideMessageForUser(ctx, messageID, userID)
}

// RemoveMessage implements moderation.MessageRemover: it applies a moderator
// content-removal tombstone. The service layer never checks the moderator's
// role here — that authorization happens in the moderation handler/service.
func (s *Service) RemoveMessage(ctx context.Context, moderatorID, messageID string) error {
	return s.repo.RemoveMessage(ctx, messageID, moderatorID)
}

// MessageForReport implements moderation.MessageAccessor: it returns the
// evidence-relevant fields for messageID only if requesterID is a
// participant in its conversation, preventing reporting an arbitrary message
// id belonging to someone else's conversation.
func (s *Service) MessageForReport(ctx context.Context, requesterID, messageID string) (MessageReportEvidence, error) {
	msg, err := s.repo.MessageByID(ctx, messageID)
	if err != nil {
		return MessageReportEvidence{}, err
	}
	ok, err := s.repo.IsParticipant(ctx, msg.ConversationID, requesterID)
	if err != nil {
		return MessageReportEvidence{}, err
	}
	if !ok {
		return MessageReportEvidence{}, ErrForbidden
	}
	c, err := s.repo.GetConversation(ctx, msg.ConversationID)
	if err != nil {
		return MessageReportEvidence{}, err
	}
	return MessageReportEvidence{
		MessageID: msg.ID, ConversationID: msg.ConversationID, SenderID: msg.SenderUserID,
		ParticipantIDs: []string{c.ParticipantA, c.ParticipantB}, Text: msg.Body, CreatedAt: msg.CreatedAt,
	}, nil
}

// MarkRead advances userID's read position in conversationID up to (and
// including) lastReadMessageID.
func (s *Service) MarkRead(ctx context.Context, userID, conversationID, lastReadMessageID string) error {
	ok, err := s.repo.IsParticipant(ctx, conversationID, userID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return s.repo.MarkRead(ctx, conversationID, userID, lastReadMessageID)
}

// UnreadCount returns the total unread DM count across every conversation
// userID participates in.
func (s *Service) UnreadCount(ctx context.Context, userID string) (UnreadCountResponse, error) {
	n, err := s.repo.UnreadCount(ctx, userID)
	if err != nil {
		return UnreadCountResponse{}, err
	}
	return UnreadCountResponse{Count: n}, nil
}

// MessageReportEvidence is social's projection of a message for moderation
// reporting; it satisfies moderation.MessageEvidence's shape via an adapter
// in cmd/api/main.go (kept here to avoid social importing moderation).
type MessageReportEvidence struct {
	MessageID      string
	ConversationID string
	SenderID       string
	ParticipantIDs []string
	Text           string
	CreatedAt      time.Time
}

// RemoveFollowBothDirections implements safety.FollowRemover: blocking must
// remove or deactivate any existing follow/friendship between the two users.
func (s *Service) RemoveFollowBothDirections(ctx context.Context, userA, userB string) error {
	if err := s.repo.Unfollow(ctx, userA, userB); err != nil {
		return err
	}
	return s.repo.Unfollow(ctx, userB, userA)
}

func (s *Service) followStateForTarget(ctx context.Context, userID string, target profile.Profile) (FollowState, error) {
	isFollowing, err := s.repo.IsFollowing(ctx, userID, target.UserID)
	if err != nil {
		return FollowState{}, err
	}
	followsMe, err := s.repo.IsFollowing(ctx, target.UserID, userID)
	if err != nil {
		return FollowState{}, err
	}
	return FollowState{
		Handle:      target.Handle,
		IsFollowing: isFollowing,
		FollowsMe:   followsMe,
		IsFriend:    isFollowing && followsMe,
	}, nil
}

func (s *Service) areFriends(ctx context.Context, a, b string) (bool, error) {
	ab, err := s.repo.IsFollowing(ctx, a, b)
	if err != nil {
		return false, err
	}
	ba, err := s.repo.IsFollowing(ctx, b, a)
	if err != nil {
		return false, err
	}
	return ab && ba, nil
}

func (s *Service) conversationSummary(ctx context.Context, userID string, c Conversation) (ConversationSummary, error) {
	otherID := c.ParticipantA
	if otherID == userID {
		otherID = c.ParticipantB
	}
	p, err := s.profiles.GetByUserID(ctx, otherID)
	if err != nil {
		return ConversationSummary{}, err
	}
	unread, err := s.repo.ConversationUnreadCount(ctx, c.ID, userID)
	if err != nil {
		return ConversationSummary{}, err
	}
	dto := ConversationSummary{
		ID:          c.ID,
		OtherUser:   safeProfile(p),
		UpdatedAt:   c.UpdatedAt,
		UnreadCount: unread,
	}
	if msg, ok, err := s.repo.LastMessage(ctx, c.ID); err != nil {
		return ConversationSummary{}, err
	} else if ok {
		preview := truncatePreview(msg.Body)
		if msg.IsRemoved() {
			preview = removedTombstoneText
		}
		dto.LastMessage = &LastMessagePreview{
			BodyPreview: preview,
			SentAt:      msg.CreatedAt,
			SentByMe:    msg.SenderUserID == userID,
		}
	}
	return dto, nil
}

func (s *Service) messageDTO(ctx context.Context, userID string, msg Message) (MessageDTO, error) {
	p, err := s.profiles.GetByUserID(ctx, msg.SenderUserID)
	if err != nil {
		return MessageDTO{}, err
	}
	body := msg.Body
	if msg.IsRemoved() {
		body = removedTombstoneText
	}
	return MessageDTO{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		Sender:         safeProfile(p),
		Body:           body,
		Removed:        msg.IsRemoved(),
		SentByMe:       msg.SenderUserID == userID,
		CreatedAt:      msg.CreatedAt,
	}, nil
}

func (s *Service) publicProfileByHandle(ctx context.Context, handle string) (profile.Profile, error) {
	p, err := s.profileByHandle(ctx, handle)
	if err != nil {
		return profile.Profile{}, err
	}
	if !p.IsPublic {
		return profile.Profile{}, ErrNotFound
	}
	return p, nil
}

func (s *Service) profileByHandle(ctx context.Context, handle string) (profile.Profile, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if err := profile.ValidateHandle(handle); err != nil {
		return profile.Profile{}, ErrInvalidHandle
	}
	p, err := s.profiles.GetByHandle(ctx, handle)
	if errors.Is(err, profile.ErrNotFound) {
		return profile.Profile{}, ErrNotFound
	}
	return p, err
}

func friendItem(p profile.Profile, since time.Time) FriendItem {
	return FriendItem{
		Handle:       p.Handle,
		DisplayName:  p.DisplayName,
		AvatarKey:    p.AvatarKey,
		StrategyTag:  p.StrategyTag,
		FriendsSince: since,
	}
}

func safeProfile(p profile.Profile) SafeProfile {
	return SafeProfile{
		Handle:      p.Handle,
		DisplayName: p.DisplayName,
		AvatarKey:   p.AvatarKey,
		StrategyTag: p.StrategyTag,
	}
}

func truncatePreview(body string) string {
	body = strings.TrimSpace(body)
	runes := []rune(body)
	if len(runes) <= 80 {
		return body
	}
	return string(runes[:80])
}
