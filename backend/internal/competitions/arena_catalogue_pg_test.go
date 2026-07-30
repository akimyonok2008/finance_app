package competitions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostgresDefinitionMetadata_ResolvesSeededDefinitions(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresCompetitionRepository(pool)
	ctx := context.Background()

	// Seeded by migration 0049: a slug-stable id, so this assertion survives
	// as long as that migration exists.
	desc, category, icon, err := repo.DefinitionMetadata(ctx, "c0000000-0000-4000-8000-000000000002")
	require.NoError(t, err)
	assert.Equal(t, "crypto", category)
	assert.NotEmpty(t, desc)
	assert.NotEmpty(t, icon)

	desc, category, icon, err = repo.DefinitionMetadata(ctx, "00000000-0000-4000-8000-000000000000")
	require.NoError(t, err)
	assert.Empty(t, desc)
	assert.Empty(t, category)
	assert.Empty(t, icon)
}
