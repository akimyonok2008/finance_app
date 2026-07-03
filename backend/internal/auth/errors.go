package auth

import "errors"

// Domain errors. Handlers map these to HTTP status codes; they never leak
// implementation detail to clients.
var (
	// ErrEmailRequired is returned when registration is missing an email.
	ErrEmailRequired = errors.New("email is required")
	// ErrPasswordRequired is returned when registration is missing a password.
	ErrPasswordRequired = errors.New("password is required")
	// ErrPasswordTooShort is returned when a password is under the minimum length.
	ErrPasswordTooShort = errors.New("password must be at least 8 characters")
	// ErrDisplayNameRequired is returned when registration is missing a display name.
	ErrDisplayNameRequired = errors.New("display name is required")
	// ErrEmailExists is returned when registering an already-used email.
	ErrEmailExists = errors.New("email already exists")
	// ErrInvalidCredentials is returned for any failed login. It is intentionally
	// vague so callers cannot distinguish "unknown email" from "wrong password".
	ErrInvalidCredentials = errors.New("invalid email or password")
	// ErrUserNotFound is returned by the repository when a lookup misses.
	ErrUserNotFound            = errors.New("user not found")
	ErrIdentityNotFound        = errors.New("auth identity not found")
	ErrProviderDisabled        = errors.New("this sign-in method is not enabled")
	ErrProviderNotConfigured   = errors.New("this sign-in method is not configured")
	ErrInvalidProviderToken    = errors.New("provider sign-in failed")
	ErrProviderEmailUnverified = errors.New("provider email is not verified")
)
