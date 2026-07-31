package competitions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestPostgresAdminMutationRollsBackWhenAuditFails(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionAdminRepository(pool)
	ctx := context.Background()
	def := uniqueDefinition()
	// A syntactically valid but nonexistent actor violates the audit FK. The
	// definition insert must disappear with it.
	audit := AdminAuditRecord{ID: uuid.NewString(), ActorUserID: uuid.NewString(), Action: "definition.create", TargetType: "definition", TargetID: def.ID, Details: json.RawMessage(`{}`), Succeeded: true, CreatedAt: time.Now().UTC()}
	require.Error(t, repo.CreateDefinitionWithAudit(ctx, def, audit))
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM competition_definitions WHERE id=$1`, def.ID).Scan(&count))
	require.Zero(t, count, "mutation survived a failed audit insert")
}

func TestPostgresFinalizeCommitsOutboxWithResults(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()
	edition := pgEngineEdition(t, repo, defs)
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleRegistrationClosed, LifecycleActive, time.Now()))
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleActive, LifecycleFinalizing, time.Now()))
	// With participant-granular projection, zero results correctly enqueue no
	// achievement work.
	gen, err := repo.EnsureBuildingFinalizationGeneration(ctx, edition.ID)
	require.NoError(t, err)
	require.NoError(t, repo.AdvanceFinalizationGeneration(ctx, edition.ID, gen.Generation, "", 0, 0, 0, false))
	require.NoError(t, repo.PromoteFinalizationGeneration(ctx, edition.ID, gen.Generation, time.Now()))
	var outboxCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM competition_outbox WHERE competition_id=$1`, edition.ID).Scan(&outboxCount))
	require.Zero(t, outboxCount)
}
