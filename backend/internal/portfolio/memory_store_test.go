package portfolio

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInMemory_OutboxDeadLettersAfterMaxAttempts mirrors
// TestPG_OutboxDeadLettersAfterMaxAttempts against the InMemoryRepository, so
// the dead-letter contract is verified without a database. A permanently
// failing event must stop being retried once attempt_count reaches
// outboxMaxAttempts, and must drop out of both future claims and the
// readiness backlog.
func TestInMemory_OutboxDeadLettersAfterMaxAttempts(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	userID := uuid.NewString()
	pf, err := repo.EnsureDefaultPortfolio(ctx, userID)
	require.NoError(t, err)

	eventID := uuid.NewString()
	require.NoError(t, repo.WithLockedPortfolio(ctx, userID, func(ctx context.Context, tx AggregateTx) error {
		return tx.AppendOutbox(ctx, OutboxEvent{
			ID: eventID, EventType: EventPortfolioMutated,
			AggregateType: "portfolio", AggregateID: pf.ID, AggregateVersion: 1,
			UserID: userID, RankedIndex: testIndex("100"), RankingStatus: "active",
			TrackingStartedAt: time.Now().UTC().Add(-time.Hour),
			ValuationAsOf:     time.Now().UTC(),
			DataQualityStatus: "complete",
			CreatedAt:         time.Now().UTC(),
		})
	}))

	for i := 0; i < outboxMaxAttempts; i++ {
		delete(repo.nextAttempt, eventID) // bypass backoff between attempts

		claimed, err := repo.ClaimOutboxEvents(ctx, 10)
		require.NoError(t, err)
		require.Len(t, claimed, 1, "attempt %d: event should still be claimable", i+1)

		require.NoError(t, repo.MarkOutboxFailed(ctx, eventID, "price provider error"))
	}

	delete(repo.nextAttempt, eventID)
	claimed, err := repo.ClaimOutboxEvents(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, claimed, "dead-lettered event must not be claimed again")

	pending, _, err := repo.OutboxBacklog(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending, "dead-lettered event must not count toward backlog")
}
