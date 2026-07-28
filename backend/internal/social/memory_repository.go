package social

import (
	"context"
	"sort"
	"sync"
	"time"
)

type participantState struct {
	lastReadMessageID string
	lastReadAt        time.Time
}

type InMemoryRepository struct {
	mu                 sync.RWMutex
	follows            map[string]Follow
	conversations      map[string]Conversation
	conversationByPair map[string]string
	messages           map[string][]Message
	messageByID        map[string]string                      // messageID -> conversationID
	hiddenFor          map[string]map[string]bool             // userID -> messageID -> hidden
	readState          map[string]map[string]participantState // conversationID -> userID -> state
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		follows:            map[string]Follow{},
		conversations:      map[string]Conversation{},
		conversationByPair: map[string]string{},
		messages:           map[string][]Message{},
		messageByID:        map[string]string{},
		hiddenFor:          map[string]map[string]bool{},
		readState:          map[string]map[string]participantState{},
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
	r.messageByID[msg.ID] = msg.ConversationID
	c.UpdatedAt = msg.CreatedAt
	t := msg.CreatedAt
	c.LastMessageAt = &t
	r.conversations[msg.ConversationID] = c
	return nil
}

func (r *InMemoryRepository) ListMessages(_ context.Context, conversationID, viewerID string, limit int) ([]Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.conversations[conversationID]; !ok {
		return nil, ErrConversationNotFound
	}
	hidden := r.hiddenFor[viewerID]
	filtered := make([]Message, 0, len(r.messages[conversationID]))
	for _, m := range r.messages[conversationID] {
		if hidden != nil && hidden[m.ID] {
			continue
		}
		filtered = append(filtered, m)
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}
	return filtered, nil
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

func (r *InMemoryRepository) MessageByID(_ context.Context, messageID string) (Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	convID, ok := r.messageByID[messageID]
	if !ok {
		return Message{}, ErrMessageNotFound
	}
	for _, m := range r.messages[convID] {
		if m.ID == messageID {
			return m, nil
		}
	}
	return Message{}, ErrMessageNotFound
}

func (r *InMemoryRepository) HideMessageForUser(_ context.Context, messageID, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.messageByID[messageID]; !ok {
		return ErrMessageNotFound
	}
	if r.hiddenFor[userID] == nil {
		r.hiddenFor[userID] = map[string]bool{}
	}
	r.hiddenFor[userID][messageID] = true
	return nil
}

func (r *InMemoryRepository) RemoveMessage(_ context.Context, messageID, moderatorID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	convID, ok := r.messageByID[messageID]
	if !ok {
		return ErrMessageNotFound
	}
	items := r.messages[convID]
	for i, m := range items {
		if m.ID == messageID {
			now := time.Now().UTC()
			m.RemovedAt = &now
			mod := moderatorID
			m.RemovedBy = &mod
			items[i] = m
			return nil
		}
	}
	return ErrMessageNotFound
}

func (r *InMemoryRepository) MarkRead(_ context.Context, conversationID, userID, lastReadMessageID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.conversations[conversationID]; !ok {
		return ErrConversationNotFound
	}
	var readAt time.Time
	found := false
	for _, m := range r.messages[conversationID] {
		if m.ID == lastReadMessageID {
			readAt = m.CreatedAt
			found = true
			break
		}
	}
	if !found {
		return ErrMessageNotFound
	}
	if r.readState[conversationID] == nil {
		r.readState[conversationID] = map[string]participantState{}
	}
	r.readState[conversationID][userID] = participantState{lastReadMessageID: lastReadMessageID, lastReadAt: readAt}
	return nil
}

func (r *InMemoryRepository) ConversationUnreadCount(_ context.Context, conversationID, userID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	state, hasState := r.readState[conversationID][userID]
	hidden := r.hiddenFor[userID]
	count := 0
	for _, m := range r.messages[conversationID] {
		if m.SenderUserID == userID {
			continue
		}
		if m.IsRemoved() {
			continue
		}
		if hidden != nil && hidden[m.ID] {
			continue
		}
		if hasState && !m.CreatedAt.After(state.lastReadAt) {
			continue
		}
		count++
	}
	return count, nil
}

func (r *InMemoryRepository) UnreadCount(ctx context.Context, userID string) (int, error) {
	r.mu.RLock()
	convIDs := make([]string, 0)
	for id, c := range r.conversations {
		if c.ParticipantA == userID || c.ParticipantB == userID {
			convIDs = append(convIDs, id)
		}
	}
	r.mu.RUnlock()
	total := 0
	for _, id := range convIDs {
		n, err := r.ConversationUnreadCount(ctx, id, userID)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
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

func (r *InMemoryRepository) OnAccountDeleted(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for key, follow := range r.follows {
		if follow.FollowerUserID == userID || follow.FollowingUserID == userID {
			delete(r.follows, key)
		}
	}
	for id, conversation := range r.conversations {
		if conversation.ParticipantA == userID || conversation.ParticipantB == userID {
			delete(r.conversationByPair, pairKey(conversation.ParticipantA, conversation.ParticipantB))
			delete(r.messages, id)
			delete(r.conversations, id)
		}
	}
	return nil
}
