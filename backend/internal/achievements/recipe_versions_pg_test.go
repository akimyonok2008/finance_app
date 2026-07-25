package achievements

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/benchmark"
)

// TestPostgresRecipeVersions_SeededAndConstrained verifies the durable recipe
// version table: the authoritative Berkshire versions are seeded with source
// metadata, the (recipe_id, version_id) uniqueness holds, and the sub-threshold
// coverage constraint fails closed.
func TestPostgresRecipeVersions_SeededAndConstrained(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM benchmark_recipe_versions WHERE recipe_id = 'BUFFETT_13F'`).Scan(&count))
	assert.GreaterOrEqual(t, count, 2, "both Berkshire versions must be seeded")

	// Source metadata is recorded for the latest filing.
	var accession, url string
	var coverage float64
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT source_accession, source_url, mapping_coverage_pct
		 FROM benchmark_recipe_versions WHERE version_id = 'BUFFETT_13F_2026Q1'`).
		Scan(&accession, &url, &coverage))
	assert.Equal(t, "0001193125-26-226661", accession)
	assert.Contains(t, url, "sec.gov")
	assert.GreaterOrEqual(t, coverage, benchmark.MinMappingCoverage)

	// Uniqueness: re-inserting the same (recipe_id, version_id) is rejected.
	_, err := pool.Exec(ctx,
		`INSERT INTO benchmark_recipe_versions
		 (recipe_id, version_id, publicly_known_at, effective_from, components_json, source_type)
		 VALUES ('BUFFETT_13F', 'BUFFETT_13F_2026Q1', now(), now(), '[]'::jsonb, 'static_model')`)
	assert.Error(t, err, "duplicate version id must be rejected")

	// Coverage constraint: a 13F version below threshold fails closed.
	_, err = pool.Exec(ctx,
		`INSERT INTO benchmark_recipe_versions
		 (recipe_id, version_id, report_period_end, publicly_known_at, effective_from,
		  components_json, source_type, source_url, source_accession, mapping_coverage_pct)
		 VALUES ('BUFFETT_13F', 'BUFFETT_13F_low', DATE '2026-03-31', now(), now(),
		  '[]'::jsonb, 'sec_13f_hr', 'https://sec.gov/x', '0001-26-9', 0.4)`)
	assert.Error(t, err, "sub-threshold coverage must violate the check constraint")
}

// TestPostgresEvidenceImmutable verifies the DB trigger blocks in-place rewrite
// of awarded evidence.
func TestPostgresEvidenceImmutable(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	repo, err := NewPostgresAchievementRepository(ctx, pool)
	require.NoError(t, err)

	userID := seedPGUser(t, pool)
	require.NoError(t, repo.Award(ctx, AwardedAchievement{
		UserID: userID, BadgeKey: "cash_plus_30d", UnlockedAt: time.Now().UTC(),
		Evidence: benchmark.AchievementEvidence{
			Period: benchmark.Period30D, PortfolioReturnPct: 5, BenchmarkReturnPct: 2,
			EdgePoints: 3, BenchmarkRecipeID: "CASH", Verification: benchmark.AwardVerificationVerified,
		},
	}))

	_, err = pool.Exec(ctx,
		`UPDATE user_benchmark_achievements SET evidence = '{"tampered":true}'::jsonb
		 WHERE user_id = $1 AND badge_key = 'cash_plus_30d'`, userID)
	assert.Error(t, err, "evidence must be immutable once written")
}
