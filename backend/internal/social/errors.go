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
	ErrMessageRateLimited   = errors.New("message sending limit reached; try again shortly")
	ErrConversationLimited  = errors.New("conversation message limit reached; try again shortly")
	ErrRepeatedMessage      = errors.New("repeated identical messages are not allowed")
	ErrSpamBurst            = errors.New("message burst detected; slow down")
	ErrConversationNotFound = errors.New("conversation not found")
	ErrForbidden            = errors.New("forbidden")
	// ErrInteractionBlocked is returned whenever the canonical interaction
	// policy refuses an action (blocked pair, suspended, or banned account).
	// It is deliberately generic so callers cannot distinguish the reason.
	ErrInteractionBlocked = errors.New("interaction_blocked")
	ErrMessageNotFound    = errors.New("message not found")
	// ErrSafetyUnavailable is returned when the safety/block store could not
	// be queried. Callers must fail closed (server error) rather than
	// returning unfiltered or empty results, since either could leak a
	// blocked relationship.
	ErrSafetyUnavailable = errors.New("safety_unavailable")
)
