package auth

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRankableUsersPage_FallbackPaginatesByID(t *testing.T) {
	repo := NewInMemoryUserRepository()
	svc := NewService(repo, NewTokenManager("test-secret", time.Hour))
	for _, id := range []string{"u3", "u1", "u5", "u2", "u4"} {
		require.NoError(t, repo.Create(&User{
			ID: id, Email: id + "@example.com", DisplayName: "User " + id,
		}))
	}
	ctx := context.Background()

	page1, err := svc.ListRankableUsersPage(ctx, "", 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, "u1", page1[0].ID)
	assert.Equal(t, "u2", page1[1].ID)

	page2, err := svc.ListRankableUsersPage(ctx, page1[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, "u3", page2[0].ID)
	assert.Equal(t, "u4", page2[1].ID)

	// Final short page marks the end of the population.
	page3, err := svc.ListRankableUsersPage(ctx, page2[1].ID, 2)
	require.NoError(t, err)
	require.Len(t, page3, 1)
	assert.Equal(t, "u5", page3[0].ID)

	empty, err := svc.ListRankableUsersPage(ctx, page3[0].ID, 2)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
