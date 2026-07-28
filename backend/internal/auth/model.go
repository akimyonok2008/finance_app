package auth

import "time"

// User is the internal representation of an account. PasswordHash carries the
// bcrypt hash and is deliberately unexported from JSON via the absence of a tag
// on a never-marshalled struct — handlers always convert to PublicUser before
// writing a response.
type User struct {
	ID              string
	Email           string
	DisplayName     string
	AvatarKey       string
	PasswordHash    string
	HasPassword     bool
	EmailVerifiedAt *time.Time
	AuthVersion     int

	// Role gates moderation endpoints. Defaults to RoleUser for every normal
	// account; never trust a frontend-supplied role, only this DB-backed value.
	Role string
	// SuspendedUntil/SuspensionReason are set by a temporary_suspension
	// moderation action. A suspended (but not banned) user may still
	// authenticate to see their own status, but restricted actions are refused.
	SuspendedUntil   *time.Time
	SuspensionReason string
	// BannedAt/BanReason are set by a permanent_ban moderation action. A banned
	// user cannot authenticate at all.
	BannedAt  *time.Time
	BanReason string
}

const (
	RoleUser      = "user"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"
)

// IsBanned reports whether the account is permanently banned.
func (u *User) IsBanned() bool { return u.BannedAt != nil }

// IsSuspended reports whether the account is currently under a temporary
// suspension (i.e. SuspendedUntil is set and still in the future).
func (u *User) IsSuspended(now time.Time) bool {
	return u.SuspendedUntil != nil && now.Before(*u.SuspendedUntil)
}

// IsModerator reports whether the account has moderator or admin authority.
func (u *User) IsModerator() bool { return u.Role == RoleModerator || u.Role == RoleAdmin }

// IsAdmin reports whether the account has admin authority.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }

// PublicUser is the safe, client-facing projection of a User. It is the only
// user shape ever serialized in API responses, guaranteeing the password hash
// can never leak.
type PublicUser struct {
	ID               string     `json:"id"`
	Email            string     `json:"email"`
	DisplayName      string     `json:"display_name"`
	EmailVerified    bool       `json:"email_verified"`
	HasPassword      bool       `json:"has_password"`
	Role             string     `json:"role"`
	SuspendedUntil   *time.Time `json:"suspended_until,omitempty"`
	SuspensionReason string     `json:"suspension_reason,omitempty"`
}

// Public returns the response-safe projection of the user.
func (u *User) Public() PublicUser {
	return PublicUser{
		ID:               u.ID,
		Email:            u.Email,
		DisplayName:      u.DisplayName,
		EmailVerified:    u.EmailVerifiedAt != nil,
		HasPassword:      u.HasPassword,
		Role:             u.Role,
		SuspendedUntil:   u.SuspendedUntil,
		SuspensionReason: u.SuspensionReason,
	}
}

// RegisterInput holds the validated-on-entry fields for registration.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	AvatarKey   string
}

type AuthProvider string

const (
	ProviderGoogle AuthProvider = "google"
	ProviderApple  AuthProvider = "apple"
)

type AuthIdentity struct {
	ID              string
	UserID          string
	Provider        AuthProvider
	ProviderSubject string
	Email           string
	EmailVerified   bool
}

type ProviderClaims struct {
	Provider      AuthProvider
	Subject       string
	Email         string
	EmailVerified bool
	DisplayName   string
	AvatarKey     string
}

type LifecycleToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}
