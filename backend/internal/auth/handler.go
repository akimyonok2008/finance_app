package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// authResponse is the shared success shape for register and login.
type authResponse struct {
	User  PublicUser `json:"user"`
	Token string     `json:"token"`
}

// errorResponse is the consistent error envelope: {"error": "message"}.
type errorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// Handler adapts HTTP requests to the auth Service.
type Handler struct {
	svc *Service
}

// NewHandler constructs a Handler backed by the given service.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	AvatarKey   string `json:"avatar_key"` // optional; defaults to "default"
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type deleteAccountRequest struct {
	ReauthenticationToken string `json:"reauthentication_token"`
}

type emailRequest struct {
	Email string `json:"email"`
}

type tokenRequest struct {
	Token string `json:"token"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type reauthenticateRequest struct {
	Password   string       `json:"password,omitempty"`
	Provider   AuthProvider `json:"provider,omitempty"`
	Credential string       `json:"credential,omitempty"`
}

type setPasswordRequest struct {
	ReauthenticationToken string `json:"reauthentication_token"`
	NewPassword           string `json:"new_password"`
}

type googleRequest struct {
	Credential string `json:"credential"`
	GCSRFToken string `json:"g_csrf_token,omitempty"`
}

type appleRequest struct {
	IdentityToken     string `json:"identity_token"`
	AuthorizationCode string `json:"authorization_code"`
	User              struct {
		Name struct {
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		} `json:"name"`
		Email string `json:"email"`
	} `json:"user"`
}

// Register handles POST /auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, _, err := h.svc.RegisterContext(r.Context(), RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		AvatarKey:   req.AvatarKey,
	})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user": user.Public(), "verification_required": true,
	})
}

// Login handles POST /auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, token, err := h.svc.Login(req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrEmailVerificationRequired) {
			writeErrorCode(w, http.StatusForbidden, ErrEmailVerificationRequired.Error(), "email_verification_required")
			return
		}
		writeError(w, statusForError(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, authResponse{User: user.Public(), Token: token})
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req tokenRequest
	if decodeJSON(r, &req) != nil || strings.TrimSpace(req.Token) == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	user, token, err := h.svc.VerifyEmail(r.Context(), req.Token)
	if err != nil {
		writeErrorCode(w, statusForError(err), err.Error(), "invalid_verification_token")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{User: user.Public(), Token: token})
}

func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h.svc.ResendVerification(r.Context(), req.Email)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "If verification is available for that address, an email has been sent.",
	})
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req emailRequest
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h.svc.ForgotPassword(r.Context(), req.Email)
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "If an account can be recovered, a reset email has been sent.",
	})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetPasswordRequest
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Google(w http.ResponseWriter, r *http.Request) {
	var req googleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Credential) == "" {
		writeError(w, http.StatusBadRequest, "credential is required")
		return
	}
	user, token, err := h.svc.LoginWithGoogle(r.Context(), req.Credential)
	if err != nil {
		writeProviderError(w, err, "Google sign-in failed.")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{User: user.Public(), Token: token})
}

func (h *Handler) Apple(w http.ResponseWriter, r *http.Request) {
	var req appleRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	displayName := strings.TrimSpace(req.User.Name.FirstName + " " + req.User.Name.LastName)
	user, token, err := h.svc.LoginWithApple(r.Context(), req.IdentityToken, ProviderClaims{
		Email: req.User.Email, DisplayName: displayName,
	})
	if err != nil {
		writeProviderError(w, err, "Apple sign-in failed.")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{User: user.Public(), Token: token})
}

// Me handles GET /me. It relies on RequireAuth having placed the user id in the
// request context, and returns the current user's public projection — letting
// the SPA validate a stored token and rehydrate the user on boot.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
		return
	}

	user, err := h.svc.UserByID(userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
		return
	}

	writeJSON(w, http.StatusOK, user.Public())
}

// ChangePassword handles POST /auth/change-password. It requires the current
// password even though the caller already holds a valid JWT: a destructive
// or sensitive account action re-confirms the credential rather than trusting
// session possession alone (a stolen token doesn't necessarily mean the
// attacker knows the password).
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
		return
	}
	var req changePasswordRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	token, err := h.svc.ChangePasswordAndRevoke(userID, req.CurrentPassword, req.NewPassword)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) SetPassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
		return
	}
	var req setPasswordRequest
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	token, err := h.svc.SetFirstPassword(r.Context(), userID, req.ReauthenticationToken, req.NewPassword)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (h *Handler) Reauthenticate(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
		return
	}
	var req reauthenticateRequest
	if decodeJSON(r, &req) != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var (
		token string
		err   error
	)
	switch {
	case req.Password != "":
		token, err = h.svc.ReauthenticatePassword(r.Context(), userID, req.Password)
	case req.Provider != "" && req.Credential != "":
		token, err = h.svc.ReauthenticateProvider(r.Context(), userID, req.Provider, req.Credential)
	default:
		err = ErrReauthenticationRequired
	}
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"reauthentication_token": token})
}

func (h *Handler) RevokeSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
		return
	}
	if err := h.svc.RevokeSessions(userID); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DeleteAccount handles POST /auth/delete-account. It requires a recent,
// single-use reauthentication token and permanently erases the account.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, ErrInvalidToken.Error())
		return
	}
	var req deleteAccountRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.svc.DeleteAccount(r.Context(), userID, req.ReauthenticationToken); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// statusForError maps domain errors to HTTP status codes.
func statusForError(err error) int {
	switch {
	case errors.Is(err, ErrEmailExists):
		return http.StatusConflict
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUserNotFound):
		return http.StatusUnauthorized
	case errors.Is(err, ErrEmailVerificationRequired):
		return http.StatusForbidden
	case errors.Is(err, ErrInvalidLifecycleToken), errors.Is(err, ErrReauthenticationRequired):
		return http.StatusUnauthorized
	case errors.Is(err, ErrPasswordAlreadySet):
		return http.StatusConflict
	case errors.Is(err, ErrEmailRequired),
		errors.Is(err, ErrPasswordRequired),
		errors.Is(err, ErrPasswordTooShort),
		errors.Is(err, ErrDisplayNameRequired):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeProviderError(w http.ResponseWriter, err error, fallback string) {
	switch {
	case errors.Is(err, ErrProviderDisabled), errors.Is(err, ErrProviderNotConfigured):
		writeError(w, http.StatusServiceUnavailable, "This sign-in method is not configured.")
	case errors.Is(err, ErrProviderEmailUnverified), errors.Is(err, ErrInvalidProviderToken):
		writeError(w, http.StatusUnauthorized, fallback)
	default:
		writeError(w, http.StatusInternalServerError, fallback)
	}
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeErrorCode(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, errorResponse{Error: message, Code: code})
}
