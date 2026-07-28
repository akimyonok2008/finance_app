package safety

import (
	"context"
	"testing"

	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProfileRepo struct {
	byID map[string]profile.Profile
}

func (r *fakeProfileRepo) GetByUserID(_ context.Context, userID string) (profile.Profile, error) {
	p, ok := r.byID[userID]
	if !ok {
		return profile.Profile{}, profile.ErrNotFound
	}
	return p, nil
}

func (r *fakeProfileRepo) GetByHandle(_ context.Context, handle string) (profile.Profile, error) {
	for _, p := range r.byID {
		if p.Handle == handle {
			return p, nil
		}
	}
	return profile.Profile{}, profile.ErrNotFound
}

type fakeFollowRemover struct {
	removed [][2]string
}

func (f *fakeFollowRemover) RemoveFollowBothDirections(_ context.Context, a, b string) error {
	f.removed = append(f.removed, [2]string{a, b})
	return nil
}

type fakeStatus struct {
	suspended map[string]bool
	banned    map[string]bool
}

func (f *fakeStatus) UserStatus(_ context.Context, userID string) (bool, bool, error) {
	return f.suspended[userID], f.banned[userID], nil
}

func newTestService() (*Service, *fakeFollowRemover) {
	repo := NewInMemoryRepository()
	profiles := &fakeProfileRepo{byID: map[string]profile.Profile{
		"user-a": {UserID: "user-a", Handle: "alpha", DisplayName: "Alpha"},
		"user-b": {UserID: "user-b", Handle: "beta", DisplayName: "Beta"},
	}}
	svc := NewService(repo, profiles)
	remover := &fakeFollowRemover{}
	svc.SetFollowRemover(remover)
	return svc, remover
}

func TestService_BlockLifecycleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc, remover := newTestService()

	require.NoError(t, svc.Block(ctx, "user-a", "beta"))
	require.NoError(t, svc.Block(ctx, "user-a", "beta")) // idempotent
	require.Len(t, remover.removed, 2)                   // called once per Block call

	blocked, err := svc.BlockedUsers(ctx, "user-a")
	require.NoError(t, err)
	require.Len(t, blocked.BlockedUsers, 1)
	assert.Equal(t, "beta", blocked.BlockedUsers[0].Handle)

	require.NoError(t, svc.Unblock(ctx, "user-a", "beta"))
	require.NoError(t, svc.Unblock(ctx, "user-a", "beta")) // idempotent

	blocked, err = svc.BlockedUsers(ctx, "user-a")
	require.NoError(t, err)
	assert.Empty(t, blocked.BlockedUsers)
}

func TestService_SelfBlockRejected(t *testing.T) {
	svc, _ := newTestService()
	err := svc.Block(context.Background(), "user-a", "alpha")
	assert.ErrorIs(t, err, ErrSelfBlock)
}

func TestService_CanUsersInteractBothDirections(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()

	ok, err := svc.CanUsersInteract(ctx, "user-a", "user-b")
	require.NoError(t, err)
	assert.True(t, ok)

	require.NoError(t, svc.Block(ctx, "user-a", "beta"))

	ok, err = svc.CanUsersInteract(ctx, "user-a", "user-b")
	assert.ErrorIs(t, err, ErrBlocked)
	assert.False(t, ok)

	// Direction doesn't matter: a block by A on B also blocks B->A.
	ok, err = svc.CanUsersInteract(ctx, "user-b", "user-a")
	assert.ErrorIs(t, err, ErrBlocked)
	assert.False(t, ok)
}

func TestService_CanUsersInteractRespectsSuspensionAndBan(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	svc.SetUserStatusProvider(&fakeStatus{
		suspended: map[string]bool{"user-a": true},
		banned:    map[string]bool{"user-b": true},
	})

	ok, err := svc.CanUsersInteract(ctx, "user-a", "user-b")
	require.Error(t, err)
	assert.False(t, ok)
}

func TestService_BlockedPairUserIDsUnionsBothDirections(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService()
	require.NoError(t, svc.Block(ctx, "user-a", "beta"))

	set, err := svc.BlockedPairUserIDs(ctx, "user-b")
	require.NoError(t, err)
	assert.True(t, set["user-a"])
}
