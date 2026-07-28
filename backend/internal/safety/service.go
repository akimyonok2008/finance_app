package safety

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/pairlock"
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

// BlockCoordinator owns the database-side changes of a block as a single
// transactional unit: creating the block and removing any follow/friendship
// relationship in both directions commit or roll back together. When set,
// Service.Block delegates to it instead of the non-atomic
// repo.Block + FollowRemover fallback path.
type BlockCoordinator interface {
	BlockAndRemoveRelationships(ctx context.Context, blockerUserID, blockedUserID string) error
}

// Service implements both blocking (BlockService) and the canonical
// interaction-policy gate (InteractionPolicyService) described in the plan.
// A single service backs both roles because they share the same repository
// and there is no benefit to splitting them into separate types here.
type Service struct {
	repo        Repository
	profiles    ProfileRepository
	follows     FollowRemover
	status      UserStatusProvider
	coordinator BlockCoordinator
}

func NewService(repo Repository, profiles ProfileRepository) *Service {
	return &Service{repo: repo, profiles: profiles}
}

func (s *Service) SetFollowRemover(f FollowRemover)           { s.follows = f }
func (s *Service) SetUserStatusProvider(p UserStatusProvider) { s.status = p }
func (s *Service) SetBlockCoordinator(c BlockCoordinator)     { s.coordinator = c }

// Block blocks the target handle on behalf of userID. Idempotent. Removes any
// existing follow relationship between the two users in both directions.
//
// This is all-or-nothing: if the coordinator is set (PostgreSQL), the block
// insert and follow removal commit together in one transaction. Otherwise
// (in-memory, or no coordinator wired), the same guarantee is approximated
// by rolling back the just-created block if follow removal fails, so a
// follow-removal error never leaves a block coexisting with a stale follow.
// Either way, blocking and following for the same pair are serialized via
// pairlock so a concurrent follow request cannot race a concurrent block.
func (s *Service) Block(ctx context.Context, userID, handle string) error {
	target, err := s.profileByHandle(ctx, handle)
	if err != nil {
		return err
	}
	if target.UserID == userID {
		return ErrSelfBlock
	}

	unlock := pairlock.Lock(userID, target.UserID)
	defer unlock()

	if s.coordinator != nil {
		return s.coordinator.BlockAndRemoveRelationships(ctx, userID, target.UserID)
	}

	alreadyBlocked, err := s.repo.IsBlocked(ctx, userID, target.UserID)
	if err != nil {
		return err
	}
	if err := s.repo.Block(ctx, userID, target.UserID); err != nil {
		return err
	}
	if s.follows != nil {
		if err := s.follows.RemoveFollowBothDirections(ctx, userID, target.UserID); err != nil {
			if !alreadyBlocked {
				// Best-effort rollback: never leave a block committed if we
				// couldn't clear the follow relationship, since that would
				// let a stale follow coexist with a block (and an eventual
				// unblock could look like it "restored" the follow).
				_ = s.repo.Unblock(ctx, userID, target.UserID)
			}
			return fmt.Errorf("%w: %v", ErrBlockFailed, err)
		}
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
