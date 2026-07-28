package safety

import "context"

// Repository is the persistence boundary for user-to-user blocking.
type Repository interface {
	// Block creates a block from blockerUserID -> blockedUserID. Idempotent:
	// blocking twice is not an error.
	Block(ctx context.Context, blockerUserID, blockedUserID string) error
	// Unblock removes the block, if any. Idempotent.
	Unblock(ctx context.Context, blockerUserID, blockedUserID string) error
	// IsBlocked reports whether blockerUserID has blocked blockedUserID
	// (single direction).
	IsBlocked(ctx context.Context, blockerUserID, blockedUserID string) (bool, error)
	// IsBlockedEitherDirection reports whether either user has blocked the
	// other. A block in either direction is sufficient to disable interaction.
	IsBlockedEitherDirection(ctx context.Context, userA, userB string) (bool, error)
	// ListBlocked returns the users blockerUserID has blocked, most recent first.
	ListBlocked(ctx context.Context, blockerUserID string) ([]Block, error)
	// BlockedPairUserIDs returns the set of user IDs that have any block
	// relationship (either direction) with userID. Used to filter search,
	// Explore, and other discovery surfaces at the query/service level.
	BlockedPairUserIDs(ctx context.Context, userID string) (map[string]bool, error)
}
