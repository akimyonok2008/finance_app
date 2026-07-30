package competitions

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
)

func uniqueDefinition() Definition {
	id := uuid.NewString()
	return Definition{
		ID:          id,
		Slug:        "test-def-" + id,
		Name:        "Test Definition",
		Description: "integration test template",
		Category:    "test",
		IconKey:     "flask",
		IsEnabled:   true,
	}
}

func versionFor(defID string, version int64) DefinitionVersion {
	return DefinitionVersion{
		DefinitionID:         defID,
		Version:              version,
		EligibilityRulesJSON: json.RawMessage(`{"schema_version":1,"all":[{"code":"c","label":"l","metric":"portfolio_weight","filter":{"asset_types":["crypto"]},"operator":"gte","value":"0.30"}]}`),
		ScoringRulesJSON:     json.RawMessage(`{"schema_version":1,"scope":"full_portfolio","include_cash":true}`),
		CreatedBy:            "test",
	}
}

func TestPostgresDefinitionRepository_CreateGetAndVersionSequence(t *testing.T) {
	repo := NewPostgresDefinitionRepository(testPool(t))
	ctx := context.Background()
	def := uniqueDefinition()

	require.NoError(t, repo.CreateDefinition(ctx, def))
	assert.ErrorIs(t, repo.CreateDefinition(ctx, def), ErrDefinitionExists)

	got, err := repo.GetDefinitionBySlug(ctx, def.Slug)
	require.NoError(t, err)
	assert.Equal(t, def.ID, got.ID)
	assert.Equal(t, int64(0), got.CurrentVersion, "a fresh definition has no versions")

	require.NoError(t, repo.CreateDefinitionVersion(ctx, versionFor(def.ID, 1)))
	require.NoError(t, repo.CreateDefinitionVersion(ctx, versionFor(def.ID, 2)))

	got, err = repo.GetDefinition(ctx, def.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2), got.CurrentVersion, "current_version must track the latest appended version")

	// Append-only: rewriting an existing version or skipping ahead both fail.
	assert.ErrorIs(t, repo.CreateDefinitionVersion(ctx, versionFor(def.ID, 1)), ErrDefinitionVersionExists)
	assert.ErrorIs(t, repo.CreateDefinitionVersion(ctx, versionFor(def.ID, 2)), ErrDefinitionVersionExists)
	assert.Error(t, repo.CreateDefinitionVersion(ctx, versionFor(def.ID, 5)), "gaps must be rejected")

	v1, err := repo.GetDefinitionVersion(ctx, def.ID, 1)
	require.NoError(t, err)
	assert.JSONEq(t, string(versionFor(def.ID, 1).EligibilityRulesJSON), string(v1.EligibilityRulesJSON),
		"stored rule payload must round-trip exactly")

	_, err = repo.GetDefinitionVersion(ctx, def.ID, 99)
	assert.ErrorIs(t, err, ErrDefinitionVersionNotFound)
	_, err = repo.GetDefinition(ctx, uuid.NewString())
	assert.ErrorIs(t, err, ErrDefinitionNotFound)
}

func TestPostgresDefinitionRepository_LegacySprintDefinitionSeeded(t *testing.T) {
	repo := NewPostgresDefinitionRepository(testPool(t))
	ctx := context.Background()

	def, err := repo.GetDefinition(ctx, LegacyWeeklySprintDefinitionID)
	require.NoError(t, err, "migration 0045 must seed the legacy Weekly Open Sprint definition")
	assert.Equal(t, "weekly-open-sprint-legacy", def.Slug)
	require.GreaterOrEqual(t, def.CurrentVersion, int64(1))

	v1, err := repo.GetDefinitionVersion(ctx, LegacyWeeklySprintDefinitionID, 1)
	require.NoError(t, err)
	var scoring map[string]any
	require.NoError(t, json.Unmarshal(v1.ScoringRulesJSON, &scoring))
	assert.Equal(t, "full_portfolio", scoring["scope"])
	assert.Equal(t, false, scoring["include_cash"], "legacy sprints never captured cash")

	// The migrated legacy payloads must be valid documents under the typed
	// engine — history is interpretable, not just stored.
	_, parsedScoring, err := rules.ValidateDefinitionVersionPayloads(v1.EligibilityRulesJSON, v1.ScoringRulesJSON)
	require.NoError(t, err)
	assert.True(t, parsedScoring.LegacyJoinTimeBaseline)
}

func TestPostgresDefinitionRepository_RejectsInvalidRuleDocuments(t *testing.T) {
	repo := NewPostgresDefinitionRepository(testPool(t))
	ctx := context.Background()
	def := uniqueDefinition()
	require.NoError(t, repo.CreateDefinition(ctx, def))

	bad := versionFor(def.ID, 1)
	bad.EligibilityRulesJSON = json.RawMessage(`{"schema_version":1,"all":[{"code":"c","label":"l","metric":"vibes","operator":"gte","value":"1"}]}`)
	err := repo.CreateDefinitionVersion(ctx, bad)
	require.ErrorIs(t, err, ErrInvalidRuleDocument)

	got, err := repo.GetDefinition(ctx, def.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got.CurrentVersion, "nothing may be persisted for an invalid document")
}

func TestPostgresEditionRepository_CreateAndLifecycle(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	defs := NewPostgresDefinitionRepository(pool)
	ctx := context.Background()

	def := uniqueDefinition()
	require.NoError(t, defs.CreateDefinition(ctx, def))
	require.NoError(t, defs.CreateDefinitionVersion(ctx, versionFor(def.ID, 1)))
	v1, err := defs.GetDefinitionVersion(ctx, def.ID, 1)
	require.NoError(t, err)
	snapshot, err := BuildRulesSnapshot(*v1)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	opens, closes := now.Add(1*time.Hour), now.Add(24*time.Hour)
	edition := Competition{
		ID: "test_edition_" + uuid.NewString(), Name: "Test Edition", Type: "engine",
		StartsAt: now.Add(24 * time.Hour), EndsAt: now.Add(8 * 24 * time.Hour), CreatedAt: now,
		DefinitionID: def.ID, DefinitionVersion: 1,
		JoinOpensAt: &opens, JoinClosesAt: &closes,
		LifecycleStatus: LifecycleDraft, RulesSnapshotJSON: snapshot, ScoringScope: "full_portfolio",
	}
	require.NoError(t, repo.CreateEdition(ctx, edition))
	assert.ErrorIs(t, repo.CreateEdition(ctx, edition), ErrEditionExists)

	got, err := repo.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	assert.False(t, got.IsLegacy())
	assert.Equal(t, def.ID, got.DefinitionID)
	assert.Equal(t, int64(1), got.DefinitionVersion)
	require.NotNil(t, got.JoinOpensAt)
	assert.True(t, got.JoinOpensAt.Equal(opens))
	var storedSnapshot RulesSnapshot
	require.NoError(t, json.Unmarshal(got.RulesSnapshotJSON, &storedSnapshot))
	assert.JSONEq(t, string(v1.EligibilityRulesJSON), string(storedSnapshot.Eligibility),
		"the edition must carry the exact rule payload of its definition version")

	// Guarded transitions: happy path stamps timestamps...
	require.NoError(t, repo.TransitionLifecycle(ctx, edition.ID, LifecycleDraft, LifecyclePublished, now))
	got, err = repo.GetCompetition(ctx, edition.ID)
	require.NoError(t, err)
	assert.Equal(t, LifecyclePublished, got.LifecycleStatus)
	require.NotNil(t, got.PublishedAt)

	// ...an invalid move is rejected by the state machine...
	err = repo.TransitionLifecycle(ctx, edition.ID, LifecyclePublished, LifecycleActive, now)
	assert.ErrorIs(t, err, ErrInvalidLifecycleTransition)

	// ...and a stale `from` (someone else moved it first) conflicts instead
	// of silently overwriting.
	err = repo.TransitionLifecycle(ctx, edition.ID, LifecycleDraft, LifecyclePublished, now)
	assert.ErrorIs(t, err, ErrLifecycleConflict)

	err = repo.TransitionLifecycle(ctx, "missing_"+uuid.NewString(), LifecycleDraft, LifecyclePublished, now)
	assert.ErrorIs(t, err, ErrCompetitionNotFound)
}

// TestPostgresEditionRepository_LegacyRowsRemainLegacy proves the additive
// columns leave the legacy sprint path untouched: rows created by the old
// CreateCompetition land as 'legacy' with empty edition fields, and the
// engine's transition machinery refuses to move them.
func TestPostgresEditionRepository_LegacyRowsRemainLegacy(t *testing.T) {
	repo := NewPostgresCompetitionRepository(testPool(t))
	ctx := context.Background()

	sprint := uniqueSprint()
	require.NoError(t, repo.CreateCompetition(ctx, sprint))
	got, err := repo.GetCompetition(ctx, sprint.ID)
	require.NoError(t, err)
	assert.True(t, got.IsLegacy())
	assert.Empty(t, got.DefinitionID)
	assert.Empty(t, got.RulesSnapshotJSON)
	assert.Equal(t, sprint.Status, got.Status, "legacy status column is untouched")

	err = repo.TransitionLifecycle(ctx, sprint.ID, LifecycleLegacy, LifecyclePublished, time.Now().UTC())
	assert.ErrorIs(t, err, ErrInvalidLifecycleTransition)
}
