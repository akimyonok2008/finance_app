package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const userIDKey contextKey = "auth.userID"

// RequireAuth returns middleware that validates the Bearer JWT in the
// Authorization header. On success it stores the user id in the request context
// and calls the next handler; otherwise it writes a 401 and stops.
func RequireAuth(tm *TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, ok := bearerToken(r)
			if !ok {
				writeError(w, http.StatusUnauthorized, "missing or malformed authorization header")
				return
			}

			claims, err := tm.Parse(token)
			if err != nil {
				writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserExistenceChecker reports whether a user id still exists. *Service
// satisfies it via UserByID.
type UserExistenceChecker interface {
	UserByID(id string) (*User, error)
}

// RequireAuthWithUser is RequireAuth plus a check that the token's user still
// exists in the repository. This guards against valid-but-stale JWTs after an
// in-memory restart: a syntactically valid token for a missing user yields 401.
func RequireAuthWithUser(tm *TokenManager, users UserExistenceChecker) func(http.Handler) http.Handler {
	base := RequireAuth(tm)
	return func(next http.Handler) http.Handler {
		// Wrap next with the existence check, then apply the base token middleware.
		checked := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, _ := UserIDFromContext(r.Context())
			if _, err := users.UserByID(id); err != nil {
				writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
				return
			}
			next.ServeHTTP(w, r)
		})
		return base(checked)
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header, returning ok=false if the header is absent or malformed.
func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

// UserIDFromContext retrieves the authenticated user id placed by RequireAuth.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok
}
