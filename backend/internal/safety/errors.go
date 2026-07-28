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
)
