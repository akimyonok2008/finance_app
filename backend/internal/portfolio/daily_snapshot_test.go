package portfolio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

func TestRecordDailySnapshot_SkipsWithoutPositions(t *testing.T) {
	svc, _ := newTestService()
	wrote, err := svc.RecordDailySnapshot(ctx(), "user-1")
	require.NoError(t, err)
	assert.False(t, wrote, "no positions ⇒ no snapshot")
}

func TestRecordDailySnapshot_WritesOnceThenDedupes(t *testing.T) {
	repo := NewInMemoryRepository()
	svc := NewService(repo, prices.NewMockPriceProvider(), fx.NewMockFXProvider())

	_, err := svc.AddPosition(ctx(), "user-1", validInput())
	require.NoError(t, err)

	// Start from a clean slate so the assertions are deterministic regardless of
	// whether other operations recorded a snapshot today.
	repo.archives = nil

	wrote, err := svc.RecordDailySnapshot(ctx(), "user-1")
	require.NoError(t, err)
	assert.True(t, wrote, "no snapshot today ⇒ one is written")

	// A second call the same UTC day is a no-op.
	wrote, err = svc.RecordDailySnapshot(ctx(), "user-1")
	require.NoError(t, err)
	assert.False(t, wrote, "idempotent per UTC day")
}
