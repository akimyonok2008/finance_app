package safety

import "time"

type Block struct {
	BlockerUserID string
	BlockedUserID string
	CreatedAt     time.Time
}

type BlockedUserItem struct {
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	AvatarKey   string    `json:"avatar_key"`
	BlockedAt   time.Time `json:"blocked_at"`
}

type BlockedUsersResponse struct {
	BlockedUsers []BlockedUserItem `json:"blocked_users"`
}

type BlockStateResponse struct {
	Handle    string `json:"handle"`
	IsBlocked bool   `json:"is_blocked"`
}
