package social

import "errors"

var (
	ErrNotFound             = errors.New("not found")
	ErrInvalidHandle        = errors.New("invalid handle")
	ErrSelfFollow           = errors.New("cannot follow yourself")
	ErrSelfDM               = errors.New("cannot message yourself")
	ErrNotFriends           = errors.New("you can only message mutual friends")
	ErrInvalidMessage       = errors.New("message body is required")
	ErrMessageTooLong       = errors.New("message body must be 1000 characters or fewer")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrForbidden            = errors.New("forbidden")
)
