package safety

import (
	"context"
	"errors"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/profile"
)

// ProfileRepository is the narrow profile lookup safety needs to resolve a
// handle to a user id, mirroring social.ProfileRepository.
type ProfileRepository interface {
	GetByUserID(ctx context.Context, userID string) (profile.Profile, error)
	GetByHandle(ctx context.Context, handle string) (profile.Profile, error)
}

// FollowRemover lets Service clear any existing follow relationship in both
// directions when a block is created, so blocking always wins over an
// existing follow/friendship.
type FollowRemover interface {
	RemoveFollowBothDirections(ctx context.Context, userA, userB string) error
}

// UserStatusProvider supplies suspension/ban state for interaction checks,
// implemented by a thin adapter over auth.Service.
type UserStatusProvider interface {
	UserStatus(ctx context.Context, userID string) (suspended bool, banned bool, err error)
}

// Service implements both blocking (BlockService) and the canonical
// interaction-policy gate (InteractionPolicyService) described in the plan.
// A single service backs both roles because they share the same repository
// and there is no benefit to splitting them into separate types here.
type Service struct {
	repo     Repository
	profiles ProfileRepository
	follows  FollowRemover
	status   UserStatusProvider
}

func NewService(repo Repository, profiles ProfileRepository) *Service {
	return &Service{repo: repo, profiles: profiles}
}

func (s *Service) SetFollowRemover(f FollowRemover)           { s.follows = f }
func (s *Service) SetUserStatusProvider(p UserStatusProvider) { s.status = p }

// Block blocks the target handle on behalf of userID. Idempotent. Removes any
// existing follow relationship between the two users in both directions.
func (s *Service) Block(ctx context.Context, userID, handle string) error {
	target, err := s.profileByHandle(ctx, handle)
	if err != nil {
		return err
	}
	if target.UserID == userID {
		return ErrSelfBlock
	}
	if err := s.repo.Block(ctx, userID, target.UserID); err != nil {
		return err
	}
	if s.follows != nil {
		_ = s.follows.RemoveFollowBothDirections(ctx, userID, target.UserID)
	}
	return nil
}

// Unblock removes a block. Idempotent. It does not restore any prior follow
// relationship.
func (s *Service) Unblock(ctx context.Context, userID, handle string) error {
	target, err := s.profileByHandle(ctx, handle)
	if err != nil {
		return err
	}
	return s.repo.Unblock(ctx, userID, target.UserID)
}

// BlockedUsers lists the users the caller has blocked.
func (s *Service) BlockedUsers(ctx context.Context, userID string) (BlockedUsersResponse, error) {
	blocks, err := s.repo.ListBlocked(ctx, userID)
	if err != nil {
		return BlockedUsersResponse{}, err
	}
	out := make([]BlockedUserItem, 0, len(blocks))
	for _, b := range blocks {
		p, err := s.profiles.GetByUserID(ctx, b.BlockedUserID)
		if err != nil {
			continue
		}
		out = append(out, BlockedUserItem{
			Handle:      p.Handle,
			DisplayName: p.DisplayName,
			AvatarKey:   p.AvatarKey,
			BlockedAt:   b.CreatedAt,
		})
	}
	return BlockedUsersResponse{BlockedUsers: out}, nil
}

// CanUsersInteract is the canonical gate used by follow, DM, profile view,
// search/Explore, invitations, and social notifications. It never leaks
// which side blocked whom, or that a block exists at all versus a suspension
// — callers get a single opaque bool + typed error.
func (s *Service) CanUsersInteract(ctx context.Context, userA, userB string) (bool, error) {
	if s.status != nil {
		for _, id := range []string{userA, userB} {
			suspended, banned, err := s.status.UserStatus(ctx, id)
			if err != nil {
				return false, err
			}
			if banned {
				return false, ErrUserBanned
			}
			if suspended {
				return false, ErrUserSuspended
			}
		}
	}
	blocked, err := s.repo.IsBlockedEitherDirection(ctx, userA, userB)
	if err != nil {
		return false, err
	}
	if blocked {
		return false, ErrBlocked
	}
	return true, nil
}

// BlockedPairUserIDs exposes the repository's block-set lookup for other
// services (Explore, search, follower/following lists) to filter results at
// the query level rather than post-filtering in a handler.
func (s *Service) BlockedPairUserIDs(ctx context.Context, userID string) (map[string]bool, error) {
	return s.repo.BlockedPairUserIDs(ctx, userID)
}

func (s *Service) profileByHandle(ctx context.Context, handle string) (profile.Profile, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if err := profile.ValidateHandle(handle); err != nil {
		return profile.Profile{}, ErrInvalidHandle
	}
	p, err := s.profiles.GetByHandle(ctx, handle)
	if errors.Is(err, profile.ErrNotFound) {
		return profile.Profile{}, ErrNotFound
	}
	return p, err
}
