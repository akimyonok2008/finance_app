package competitions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

func TestArenaCatalogue_ListsEngineWithJoinStatus_HidesUnjoinedLegacySprint(t *testing.T) {
	h := newHarness(nil, nil)
	ctx := context.Background()
	edition := rankingEdition(t, h, "arena-1")
	require.NoError(t, h.repo.CreateEngineEntry(ctx, activeEntry("e1", edition.ID, "u1", "AAA", "100")))

	page, err := h.svc.ArenaCatalogue(ctx, "u1", ArenaCatalogueQuery{})
	require.NoError(t, err)
	cards := page.Cards

	var sprint, arena *ArenaCompetitionSummary
	for i := range cards {
		switch cards[i].ID {
		case compID():
			sprint = &cards[i]
		case edition.ID:
			arena = &cards[i]
		}
	}
	assert.Nil(t, sprint, "a legacy sprint the caller never joined must not surface in discovery")

	require.NotNil(t, arena)
	assert.True(t, arena.Joined)
	assert.False(t, arena.IsLegacy)
	assert.Equal(t, EntryActive, arena.EntryStatus)
	assert.Equal(t, LifecycleActive, arena.Status, "engine editions report their stored lifecycle, not a derived legacy bucket")
	assert.Equal(t, 1, arena.ParticipantCount)

	otherUser, err := h.svc.ArenaCatalogueItem(ctx, edition.ID, "u2")
	require.NoError(t, err)
	assert.False(t, otherUser.Joined)
	assert.Equal(t, 1, otherUser.ParticipantCount, "participant count is global, not per-caller")
}

func TestArenaCatalogue_KeepsLegacySprintForExistingParticipant(t *testing.T) {
	h := newHarness(
		map[string]*auth.User{"u1": {ID: "u1"}},
		map[string][]portfolio.Position{"u1": {pos("AAPL", 10, 180, "USD")}},
	)
	ctx := context.Background()
	_, err := h.svc.JoinCompetition(ctx, compID(), "u1")
	require.NoError(t, err)

	page, err := h.svc.ArenaCatalogue(ctx, "u1", ArenaCatalogueQuery{})
	require.NoError(t, err)
	cards := page.Cards

	var sprint *ArenaCompetitionSummary
	for i := range cards {
		if cards[i].ID == compID() {
			sprint = &cards[i]
		}
	}
	require.NotNil(t, sprint, "a legacy sprint the caller already joined stays visible for migration compatibility")
	assert.True(t, sprint.Joined)
	assert.True(t, sprint.IsLegacy)

	// A different, unjoined caller must not see it.
	otherPage, err := h.svc.ArenaCatalogue(ctx, "u2", ArenaCatalogueQuery{})
	require.NoError(t, err)
	for _, c := range otherPage.Cards {
		assert.NotEqual(t, compID(), c.ID, "an unjoined caller must not see someone else's legacy sprint")
	}
}

func TestArenaCatalogue_UnknownCompetitionNotFound(t *testing.T) {
	h := newHarness(nil, nil)
	_, err := h.svc.ArenaCatalogueItem(context.Background(), "does-not-exist", "u1")
	assert.ErrorIs(t, err, ErrCompetitionNotFound)
}
