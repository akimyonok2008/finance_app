package moderation

import "errors"

var (
	ErrSelfReport           = errors.New("cannot report yourself")
	ErrInvalidCategory      = errors.New("invalid category")
	ErrDuplicateReport      = errors.New("report_duplicate")
	ErrMessageNotAccessible = errors.New("message_not_accessible")
	ErrNotFound             = errors.New("not found")
	ErrModerationForbidden  = errors.New("moderation_forbidden")
	ErrInvalidAction        = errors.New("invalid action")
	ErrAlreadyResolved      = errors.New("report already resolved")
)
