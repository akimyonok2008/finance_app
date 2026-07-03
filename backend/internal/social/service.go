package social

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/profile"
)

type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID string) (profile.Profile, error)
	GetByHandle(ctx context.Context, handle string) (profile.Profile, error)
}

type Service struct {
	repo     Repository
	profiles ProfileRepository
	now      func() time.Time
}

func NewService(repo Repository, profiles ProfileRepository) *Service {
	return &Service{
		repo:     repo,
		profiles: profiles,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) Follow(ctx context.Context, userID, handle string) (FollowState, error) {
	target, err := s.publicProfileByHandle(ctx, handle)
	if err != nil {
		return FollowState{}, err
	}
	if target.UserID == userID {
		return FollowState{}, ErrSelfFollow
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

func (s *Service) Following(ctx context.Context, userID string) (UserListResponse, error) {
	items, err := s.repo.ListFollowing(ctx, userID)
	if err != nil {
		return UserListResponse{}, err
	}
	out := make([]FriendItem, 0, len(items))
	for _, item := range items {
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
	out := make([]FriendItem, 0, len(items))
	for _, item := range items {
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
	out := make([]FriendItem, 0, len(items))
	for _, item := range items {
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
	out := make([]ConversationSummary, 0, len(items))
	for _, c := range items {
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
	items, err := s.repo.ListMessages(ctx, conversationID, limit)
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
	if err := s.repo.AddMessage(ctx, msg); err != nil {
		return MessageResponse{}, err
	}
	dto, err := s.messageDTO(ctx, userID, msg)
	if err != nil {
		return MessageResponse{}, err
	}
	return MessageResponse{Message: dto}, nil
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
	dto := ConversationSummary{
		ID:        c.ID,
		OtherUser: safeProfile(p),
		UpdatedAt: c.UpdatedAt,
	}
	if msg, ok, err := s.repo.LastMessage(ctx, c.ID); err != nil {
		return ConversationSummary{}, err
	} else if ok {
		dto.LastMessage = &LastMessagePreview{
			BodyPreview: truncatePreview(msg.Body),
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
	return MessageDTO{
		ID:             msg.ID,
		ConversationID: msg.ConversationID,
		Sender:         safeProfile(p),
		Body:           msg.Body,
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
