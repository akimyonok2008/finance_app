package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const minPasswordLength = 8

// Service holds the authentication business logic: validation, hashing,
// uniqueness checks, and token issuance. It depends only on the repository
// interface and the token manager.
type Service struct {
	repo   UserRepository
	tokens *TokenManager
}

// NewService wires a Service with its repository and token manager.
func NewService(repo UserRepository, tokens *TokenManager) *Service {
	return &Service{repo: repo, tokens: tokens}
}

// Register validates the input, hashes the password, persists the user, and
// returns the created user together with a freshly minted JWT.
func (s *Service) Register(in RegisterInput) (*User, string, error) {
	email := normalizeEmail(in.Email)
	if email == "" {
		return nil, "", ErrEmailRequired
	}
	if in.Password == "" {
		return nil, "", ErrPasswordRequired
	}
	if len(in.Password) < minPasswordLength {
		return nil, "", ErrPasswordTooShort
	}
	if in.DisplayName == "" {
		return nil, "", ErrDisplayNameRequired
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}

	avatarKey := strings.TrimSpace(in.AvatarKey)
	if avatarKey == "" {
		avatarKey = "default"
	}

	user := &User{
		ID:           uuid.NewString(),
		Email:        email,
		DisplayName:  in.DisplayName,
		AvatarKey:    avatarKey,
		PasswordHash: string(hash),
	}
	if err := s.repo.Create(user); err != nil {
		return nil, "", err // ErrEmailExists bubbles up unchanged
	}

	token, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// Login verifies credentials and returns the user plus a new JWT. Both an
// unknown email and a wrong password yield ErrInvalidCredentials so the API
// does not reveal which accounts exist.
func (s *Service) Login(email, password string) (*User, string, error) {
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// UserByID fetches a user by id, used by the /me handler after auth.
func (s *Service) UserByID(id string) (*User, error) {
	return s.repo.FindByID(id)
}

// ListUsers returns all users. It is consumed by the leaderboard module via the
// UserProvider interface. Callers receive full User values (including the
// password hash) and are responsible for projecting to a safe response shape —
// the hash must never be serialized to clients.
func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	return s.repo.ListUsers(ctx)
}
