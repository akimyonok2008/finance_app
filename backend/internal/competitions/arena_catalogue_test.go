package competitions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArenaCatalogue_ListsLegacyAndEngineWithJoinStatus(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "arena-1")
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))

	cards, err := h.svc.ArenaCatalogue(ctx, "u1")
	require.NoError(t, err)

	var sprint, arena *ArenaCompetitionSummary
	for i := range cards {
		switch cards[i].ID {
		case compID():
			sprint = &cards[i]
		case edition.ID:
			arena = &cards[i]
		}
	}
	require.NotNil(t, sprint, "the legacy weekly sprint is still in the catalogue")
	assert.False(t, sprint.Joined)

	require.NotNil(t, arena)
	assert.True(t, arena.Joined)
	assert.Equal(t, EntryActive, arena.EntryStatus)
	assert.Equal(t, LifecycleActive, arena.Status, "engine editions report their stored lifecycle, not a derived legacy bucket")
	assert.Equal(t, 1, arena.ParticipantCount)

	otherUser, err := h.svc.ArenaCatalogueItem(ctx, edition.ID, "u2")
	require.NoError(t, err)
	assert.False(t, otherUser.Joined)
	assert.Equal(t, 1, otherUser.ParticipantCount, "participant count is global, not per-caller")
}

func TestArenaCatalogue_UnknownCompetitionNotFound(t *testing.T) {
	h := newHarness(nil, nil)
	_, err := h.svc.ArenaCatalogueItem(context.Background(), "does-not-exist", "u1")
	assert.ErrorIs(t, err, ErrCompetitionNotFound)
}
