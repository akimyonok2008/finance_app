package auth

// User is the internal representation of an account. PasswordHash carries the
// bcrypt hash and is deliberately unexported from JSON via the absence of a tag
// on a never-marshalled struct — handlers always convert to PublicUser before
// writing a response.
type User struct {
	ID           string
	Email        string
	DisplayName  string
	AvatarKey    string
	PasswordHash string
}

// PublicUser is the safe, client-facing projection of a User. It is the only
// user shape ever serialized in API responses, guaranteeing the password hash
// can never leak.
type PublicUser struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
}

// Public returns the response-safe projection of the user.
func (u *User) Public() PublicUser {
	return PublicUser{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
	}
}

// RegisterInput holds the validated-on-entry fields for registration.
type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	AvatarKey   string
}
