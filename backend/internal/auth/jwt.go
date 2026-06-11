package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned when a token is missing, malformed, expired, or
// signed with the wrong key.
var ErrInvalidToken = errors.New("invalid or expired token")

// Claims is the JWT payload. It embeds RegisteredClaims to get standard fields
// such as expiry (exp) and issued-at (iat).
type Claims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// TokenManager generates and verifies signed JWTs using a shared secret.
type TokenManager struct {
	secret []byte
	expiry time.Duration
}

// NewTokenManager creates a manager that signs tokens with secret and sets an
// expiry of the given duration from issue time.
func NewTokenManager(secret string, expiry time.Duration) *TokenManager {
	return &TokenManager{secret: []byte(secret), expiry: expiry}
}

// Generate issues a signed token carrying the user id, email, and expiry.
func (tm *TokenManager) Generate(userID, email string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tm.expiry)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(tm.secret)
}

// Parse verifies the signature and expiry of a token and returns its claims.
// Any failure (bad signature, malformed, expired) collapses to ErrInvalidToken.
func (tm *TokenManager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return tm.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}
