package safety

import "errors"

var (
	ErrSelfBlock     = errors.New("cannot block yourself")
	ErrBlocked       = errors.New("interaction_blocked")
	ErrNotFound      = errors.New("not found")
	ErrInvalidHandle = errors.New("invalid handle")
	ErrForbidden     = errors.New("forbidden")
	ErrUserSuspended = errors.New("user_suspended")
	ErrUserBanned    = errors.New("user_banned")
	// ErrBlockFailed is returned when a block could not be applied atomically
	// (e.g. follow removal failed and had to be rolled back). Callers should
	// treat this as a server error, not a client error.
	ErrBlockFailed = errors.New("block_failed")
)
