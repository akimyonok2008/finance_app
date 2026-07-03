package social

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Follow(ctx context.Context, followerUserID, followingUserID string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO follows (follower_user_id, following_user_id)
		VALUES ($1, $2)
		ON CONFLICT (follower_user_id, following_user_id) DO NOTHING
	`, followerUserID, followingUserID)
	return mapPGError(err)
}

func (r *PostgresRepository) Unfollow(ctx context.Context, followerUserID, followingUserID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM follows WHERE follower_user_id = $1 AND following_user_id = $2
	`, followerUserID, followingUserID)
	return err
}

func (r *PostgresRepository) IsFollowing(ctx context.Context, followerUserID, followingUserID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM follows WHERE follower_user_id = $1 AND following_user_id = $2
		)
	`, followerUserID, followingUserID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) ListFollowing(ctx context.Context, userID string) ([]Follow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT follower_user_id, following_user_id, created_at
		FROM follows
		WHERE follower_user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFollows(rows)
}

func (r *PostgresRepository) ListFollowers(ctx context.Context, userID string) ([]Follow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT follower_user_id, following_user_id, created_at
		FROM follows
		WHERE following_user_id = $1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFollows(rows)
}

func (r *PostgresRepository) ListMutualFriends(ctx context.Context, userID string) ([]Friendship, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT f.following_user_id, GREATEST(f.created_at, back.created_at) AS friends_since
		FROM follows f
		JOIN follows back ON back.follower_user_id = f.following_user_id AND back.following_user_id = f.follower_user_id
		WHERE f.follower_user_id = $1
		ORDER BY friends_since DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Friendship, 0)
	for rows.Next() {
		var item Friendship
		if err := rows.Scan(&item.UserID, &item.FriendsSince); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) GetConversationBetween(ctx context.Context, userA, userB string) (Conversation, bool, error) {
	c, err := scanConversation(r.pool.QueryRow(ctx, `
		SELECT id, participant_a_user_id, participant_b_user_id, created_at, updated_at, last_message_at
		FROM dm_conversations
		WHERE pair_key = $1
	`, pairKey(userA, userB)))
	if errors.Is(err, ErrConversationNotFound) {
		return Conversation{}, false, nil
	}
	return c, err == nil, err
}

func (r *PostgresRepository) CreateConversation(ctx context.Context, id, userA, userB string) (Conversation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Conversation{}, err
	}
	defer tx.Rollback(ctx)

	c, err := scanConversation(tx.QueryRow(ctx, `
		INSERT INTO dm_conversations (id, participant_a_user_id, participant_b_user_id, pair_key)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (pair_key) DO UPDATE SET pair_key = EXCLUDED.pair_key
		RETURNING id, participant_a_user_id, participant_b_user_id, created_at, updated_at, last_message_at
	`, id, userA, userB, pairKey(userA, userB)))
	if err != nil {
		return Conversation{}, mapPGError(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO dm_conversation_participants (conversation_id, user_id)
		VALUES ($1, $2), ($1, $3)
		ON CONFLICT DO NOTHING
	`, c.ID, c.ParticipantA, c.ParticipantB); err != nil {
		return Conversation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Conversation{}, err
	}
	return c, nil
}

func (r *PostgresRepository) GetConversation(ctx context.Context, conversationID string) (Conversation, error) {
	return scanConversation(r.pool.QueryRow(ctx, `
		SELECT id, participant_a_user_id, participant_b_user_id, created_at, updated_at, last_message_at
		FROM dm_conversations
		WHERE id = $1
	`, conversationID))
}

func (r *PostgresRepository) ListConversations(ctx context.Context, userID string) ([]Conversation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, participant_a_user_id, participant_b_user_id, created_at, updated_at, last_message_at
		FROM dm_conversations
		WHERE participant_a_user_id = $1 OR participant_b_user_id = $1
		ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Conversation, 0)
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) IsParticipant(ctx context.Context, conversationID, userID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM dm_conversation_participants WHERE conversation_id = $1 AND user_id = $2
		)
	`, conversationID, userID).Scan(&exists)
	return exists, err
}

func (r *PostgresRepository) OtherParticipant(ctx context.Context, conversationID, userID string) (string, error) {
	var other string
	err := r.pool.QueryRow(ctx, `
		SELECT user_id
		FROM dm_conversation_participants
		WHERE conversation_id = $1 AND user_id <> $2
		LIMIT 1
	`, conversationID, userID).Scan(&other)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	return other, err
}

func (r *PostgresRepository) AddMessage(ctx context.Context, msg Message) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO dm_messages (id, conversation_id, sender_user_id, body, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, msg.ID, msg.ConversationID, msg.SenderUserID, msg.Body, msg.CreatedAt); err != nil {
		return mapPGError(err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE dm_conversations SET updated_at = $2, last_message_at = $2 WHERE id = $1
	`, msg.ConversationID, msg.CreatedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresRepository) ListMessages(ctx context.Context, conversationID string, limit int) ([]Message, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, conversation_id, sender_user_id, body, created_at
		FROM (
			SELECT id, conversation_id, sender_user_id, body, created_at
			FROM dm_messages
			WHERE conversation_id = $1 AND deleted_at IS NULL
			ORDER BY created_at DESC
			LIMIT $2
		) recent
		ORDER BY created_at ASC
	`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Message, 0)
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) LastMessage(ctx context.Context, conversationID string) (Message, bool, error) {
	msg, err := scanMessage(r.pool.QueryRow(ctx, `
		SELECT id, conversation_id, sender_user_id, body, created_at
		FROM dm_messages
		WHERE conversation_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`, conversationID))
	if errors.Is(err, ErrConversationNotFound) {
		return Message{}, false, nil
	}
	return msg, err == nil, err
}

func scanFollows(rows pgx.Rows) ([]Follow, error) {
	out := make([]Follow, 0)
	for rows.Next() {
		var f Follow
		if err := rows.Scan(&f.FollowerUserID, &f.FollowingUserID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConversation(row rowScanner) (Conversation, error) {
	var c Conversation
	err := row.Scan(&c.ID, &c.ParticipantA, &c.ParticipantB, &c.CreatedAt, &c.UpdatedAt, &c.LastMessageAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Conversation{}, ErrConversationNotFound
	}
	return c, err
}

func scanMessage(row rowScanner) (Message, error) {
	var msg Message
	err := row.Scan(&msg.ID, &msg.ConversationID, &msg.SenderUserID, &msg.Body, &msg.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrConversationNotFound
	}
	return msg, err
}

func mapPGError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" {
		return ErrInvalidMessage
	}
	return err
}
