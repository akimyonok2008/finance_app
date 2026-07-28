package safety

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/ardakimyonok/finance_app/internal/social"
)

// failingFollowRemover always fails RemoveFollowBothDirections, to exercise
// the rollback path in Service.Block.
type failingFollowRemover struct{}

func (failingFollowRemover) RemoveFollowBothDirections(_ context.Context, _, _ string) error {
	return errors.New("boom")
}

func TestService_Block_RollsBackWhenFollowRemovalFails(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	profiles := &fakeProfileRepo{byID: map[string]profile.Profile{
		"user-a": {UserID: "user-a", Handle: "alpha", DisplayName: "Alpha"},
		"user-b": {UserID: "user-b", Handle: "beta", DisplayName: "Beta"},
	}}
	svc := NewService(repo, profiles)
	svc.SetFollowRemover(failingFollowRemover{})

	err := svc.Block(ctx, "user-a", "beta")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrBlockFailed)

	// The block must not have been left in place: a follow-removal failure
	// must not leave a block coexisting with a stale follow relationship.
	blocked, err := repo.IsBlocked(ctx, "user-a", "user-b")
	require.NoError(t, err)
	assert.False(t, blocked, "block must be rolled back when follow removal fails")
}

func TestService_Block_RollbackDoesNotDisturbPreexistingBlock(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	profiles := &fakeProfileRepo{byID: map[string]profile.Profile{
		"user-a": {UserID: "user-a", Handle: "alpha", DisplayName: "Alpha"},
		"user-b": {UserID: "user-b", Handle: "beta", DisplayName: "Beta"},
	}}
	svc := NewService(repo, profiles)
	svc.SetFollowRemover(&fakeFollowRemover{})
	require.NoError(t, svc.Block(ctx, "user-a", "beta"))

	// Second Block call fails follow removal, but since the block already
	// existed prior to this call, it must not be erroneously unblocked.
	svc.SetFollowRemover(failingFollowRemover{})
	err := svc.Block(ctx, "user-a", "beta")
	require.Error(t, err)

	blocked, err := repo.IsBlocked(ctx, "user-a", "user-b")
	require.NoError(t, err)
	assert.True(t, blocked, "preexisting block must not be undone by a later failed call")
}

// TestBlockVsFollow_ConcurrentInvariant wires a real social.Service against a
// real safety.Service (both in-memory) and hammers concurrent Follow and
// Block calls for the same pair. Regardless of interleaving, the end state
// must never have both an active block and an active follow for that pair —
// pairlock is what prevents that race. Run with -race.
func TestBlockVsFollow_ConcurrentInvariant(t *testing.T) {
	ctx := context.Background()
	safetyRepo := NewInMemoryRepository()
	socialRepo := social.NewInMemoryRepository()
	profiles := &fakeProfileRepo{byID: map[string]profile.Profile{
		"user-a": {UserID: "user-a", Handle: "alpha", DisplayName: "Alpha"},
		"user-b": {UserID: "user-b", Handle: "beta", DisplayName: "Beta"},
	}}

	socialSvc := social.NewService(socialRepo, socialProfileAdapter{profiles})
	safetySvc := NewService(safetyRepo, profiles)
	safetySvc.SetFollowRemover(socialSvc)
	socialSvc.SetInteractionPolicy(safetySvc)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = socialSvc.Follow(ctx, "user-a", "beta")
		}()
		go func() {
			defer wg.Done()
			_ = safetySvc.Block(ctx, "user-a", "beta")
		}()
	}
	wg.Wait()

	blocked, err := safetyRepo.IsBlockedEitherDirection(ctx, "user-a", "user-b")
	require.NoError(t, err)
	following, err := socialRepo.IsFollowing(ctx, "user-a", "user-b")
	require.NoError(t, err)

	if blocked {
		assert.False(t, following, "invariant violated: block and active follow coexist for the same pair")
	}
}

// socialProfileAdapter adapts safety's ProfileRepository shape to social's
// identical ProfileRepository interface (both are structurally the same;
// this avoids a direct type dependency between the two test files).
type socialProfileAdapter struct {
	inner *fakeProfileRepo
}

func (a socialProfileAdapter) GetByUserID(ctx context.Context, userID string) (profile.Profile, error) {
	return a.inner.GetByUserID(ctx, userID)
}

func (a socialProfileAdapter) GetByHandle(ctx context.Context, handle string) (profile.Profile, error) {
	return a.inner.GetByHandle(ctx, handle)
}
