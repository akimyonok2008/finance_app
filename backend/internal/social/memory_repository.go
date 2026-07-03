package social

import (
	"context"
	"sort"
	"sync"
	"time"
)

type InMemoryRepository struct {
	mu                 sync.RWMutex
	follows            map[string]Follow
	conversations      map[string]Conversation
	conversationByPair map[string]string
	messages           map[string][]Message
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		follows:            map[string]Follow{},
		conversations:      map[string]Conversation{},
		conversationByPair: map[string]string{},
		messages:           map[string][]Message{},
	}
}

func (r *InMemoryRepository) Follow(_ context.Context, followerUserID, followingUserID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := followKey(followerUserID, followingUserID)
	if _, ok := r.follows[key]; ok {
		return nil
	}
	r.follows[key] = Follow{FollowerUserID: followerUserID, FollowingUserID: followingUserID, CreatedAt: time.Now().UTC()}
	return nil
}

func (r *InMemoryRepository) Unfollow(_ context.Context, followerUserID, followingUserID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.follows, followKey(followerUserID, followingUserID))
	return nil
}

func (r *InMemoryRepository) IsFollowing(_ context.Context, followerUserID, followingUserID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.follows[followKey(followerUserID, followingUserID)]
	return ok, nil
}

func (r *InMemoryRepository) ListFollowing(_ context.Context, userID string) ([]Follow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Follow, 0)
	for _, f := range r.follows {
		if f.FollowerUserID == userID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *InMemoryRepository) ListFollowers(_ context.Context, userID string) ([]Follow, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Follow, 0)
	for _, f := range r.follows {
		if f.FollowingUserID == userID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (r *InMemoryRepository) ListMutualFriends(_ context.Context, userID string) ([]Friendship, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Friendship, 0)
	for _, f := range r.follows {
		if f.FollowerUserID != userID {
			continue
		}
		back, ok := r.follows[followKey(f.FollowingUserID, userID)]
		if !ok {
			continue
		}
		since := f.CreatedAt
		if back.CreatedAt.After(since) {
			since = back.CreatedAt
		}
		out = append(out, Friendship{UserID: f.FollowingUserID, FriendsSince: since})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FriendsSince.After(out[j].FriendsSince) })
	return out, nil
}

func (r *InMemoryRepository) GetConversationBetween(_ context.Context, userA, userB string) (Conversation, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.conversationByPair[pairKey(userA, userB)]
	if !ok {
		return Conversation{}, false, nil
	}
	return r.conversations[id], true, nil
}

func (r *InMemoryRepository) CreateConversation(_ context.Context, id, userA, userB string) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := pairKey(userA, userB)
	if existingID, ok := r.conversationByPair[key]; ok {
		return r.conversations[existingID], nil
	}
	now := time.Now().UTC()
	c := Conversation{ID: id, ParticipantA: userA, ParticipantB: userB, CreatedAt: now, UpdatedAt: now}
	r.conversations[id] = c
	r.conversationByPair[key] = id
	return c, nil
}

func (r *InMemoryRepository) GetConversation(_ context.Context, conversationID string) (Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conversations[conversationID]
	if !ok {
		return Conversation{}, ErrConversationNotFound
	}
	return c, nil
}

func (r *InMemoryRepository) ListConversations(_ context.Context, userID string) ([]Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Conversation, 0)
	for _, c := range r.conversations {
		if c.ParticipantA == userID || c.ParticipantB == userID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (r *InMemoryRepository) IsParticipant(_ context.Context, conversationID, userID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conversations[conversationID]
	if !ok {
		return false, ErrConversationNotFound
	}
	return c.ParticipantA == userID || c.ParticipantB == userID, nil
}

func (r *InMemoryRepository) OtherParticipant(_ context.Context, conversationID, userID string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conversations[conversationID]
	if !ok {
		return "", ErrConversationNotFound
	}
	if c.ParticipantA == userID {
		return c.ParticipantB, nil
	}
	if c.ParticipantB == userID {
		return c.ParticipantA, nil
	}
	return "", ErrForbidden
}

func (r *InMemoryRepository) AddMessage(_ context.Context, msg Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.conversations[msg.ConversationID]
	if !ok {
		return ErrConversationNotFound
	}
	r.messages[msg.ConversationID] = append(r.messages[msg.ConversationID], msg)
	c.UpdatedAt = msg.CreatedAt
	t := msg.CreatedAt
	c.LastMessageAt = &t
	r.conversations[msg.ConversationID] = c
	return nil
}

func (r *InMemoryRepository) ListMessages(_ context.Context, conversationID string, limit int) ([]Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.conversations[conversationID]; !ok {
		return nil, ErrConversationNotFound
	}
	items := append([]Message(nil), r.messages[conversationID]...)
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items, nil
}

func (r *InMemoryRepository) LastMessage(_ context.Context, conversationID string) (Message, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := r.messages[conversationID]
	if len(items) == 0 {
		return Message{}, false, nil
	}
	return items[len(items)-1], true, nil
}

func followKey(a, b string) string {
	return a + "->" + b
}

func pairKey(a, b string) string {
	if a < b {
		return a + ":" + b
	}
	return b + ":" + a
}
