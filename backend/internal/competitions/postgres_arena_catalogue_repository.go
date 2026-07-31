package competitions

import (
	"context"
	"fmt"
)

var _ ArenaCatalogueRepository = (*PostgresCompetitionRepository)(nil)

// ArenaCataloguePage implements ArenaCatalogueRepository: one query joins
// competition_definitions for display metadata and competition_entries
// (twice — an EXISTS for "joined", a scalar subselect for entry_status, a
// COUNT for participant_count) rather than the N+1 per-row queries the
// Go-side fallback does. Bucket and category are pushed into the WHERE
// clause; effective_status mirrors deriveStatus (model.go) for legacy rows
// and reads the stored lifecycle_status directly for engine editions, so a
// row's bucket here matches Service.summarize's Status field exactly.
func (r *PostgresCompetitionRepository) ArenaCataloguePage(ctx context.Context, filter ArenaCatalogueFilter) (ArenaCataloguePage, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultArenaCataloguePageSize
	}

	rows, err := r.pool.Query(ctx, `
		WITH effective AS (
			SELECT
				c.id, c.name, c.starts_at, c.ends_at, c.join_opens_at, c.join_closes_at,
				c.scoring_scope,
				(c.lifecycle_status = '`+LifecycleLegacy+`') AS is_legacy,
				CASE
					WHEN c.lifecycle_status <> '`+LifecycleLegacy+`' THEN c.lifecycle_status
					WHEN now() < c.starts_at THEN '`+StatusUpcoming+`'
					WHEN now() < c.ends_at THEN '`+StatusActive+`'
					ELSE '`+StatusCompleted+`'
				END AS effective_status,
				COALESCE(d.description, '') AS description,
				COALESCE(d.category, '') AS category,
				COALESCE(d.icon_key, '') AS icon_key,
				EXISTS (
					SELECT 1 FROM competition_entries ce
					WHERE ce.competition_id = c.id AND ce.user_id = $1
				) AS joined,
				(SELECT ce2.entry_status FROM competition_entries ce2
					WHERE ce2.competition_id = c.id AND ce2.user_id = $1) AS entry_status,
				(SELECT COUNT(*) FROM competition_entries ce3
					WHERE ce3.competition_id = c.id) AS participant_count
			FROM competitions c
			LEFT JOIN competition_definitions d ON d.id = c.definition_id
		)
		SELECT id, name, starts_at, ends_at, join_opens_at, join_closes_at, scoring_scope,
			is_legacy, effective_status, description, category, icon_key,
			joined, entry_status, participant_count
		FROM effective
		WHERE (NOT is_legacy OR joined)
			AND ($2 = '' OR category = $2)
			AND (
				$3 = ''
				OR ($3 = '`+ArenaBucketLive+`' AND effective_status = '`+StatusActive+`')
				OR ($3 = '`+ArenaBucketCompleted+`' AND effective_status = '`+StatusCompleted+`')
				OR ($3 = '`+ArenaBucketUpcoming+`' AND effective_status NOT IN ('`+StatusActive+`', '`+StatusCompleted+`', '`+LifecycleCancelled+`') AND NOT joined)
				OR ($3 = '`+ArenaBucketMine+`' AND joined AND effective_status <> '`+StatusCompleted+`')
			)
		ORDER BY starts_at
		LIMIT $4 OFFSET $5
	`, filter.UserID, filter.Category, filter.Bucket, limit+1, filter.Offset)
	if err != nil {
		return ArenaCataloguePage{}, fmt.Errorf("competition repository: arena catalogue page: %w", err)
	}
	defer rows.Close()

	cards := make([]ArenaCompetitionSummary, 0, limit)
	for rows.Next() {
		var c ArenaCompetitionSummary
		var entryStatus *string
		if err := rows.Scan(&c.ID, &c.Name, &c.StartsAt, &c.EndsAt, &c.JoinOpensAt, &c.JoinClosesAt,
			&c.ScoringScope, &c.IsLegacy, &c.Status, &c.Description, &c.Category, &c.IconKey,
			&c.Joined, &entryStatus, &c.ParticipantCount); err != nil {
			return ArenaCataloguePage{}, fmt.Errorf("competition repository: scan arena catalogue row: %w", err)
		}
		if c.IsLegacy {
			// Legacy cards never carry join-window fields (see summarize).
			c.JoinOpensAt, c.JoinClosesAt = nil, nil
		}
		if entryStatus != nil {
			c.EntryStatus = *entryStatus
		} else if c.Joined {
			c.EntryStatus = EntryLegacyActive
		}
		cards = append(cards, c)
	}
	if err := rows.Err(); err != nil {
		return ArenaCataloguePage{}, fmt.Errorf("competition repository: arena catalogue rows: %w", err)
	}

	hasMore := len(cards) > limit
	if hasMore {
		cards = cards[:limit]
	}
	return ArenaCataloguePage{Cards: cards, HasMore: hasMore}, nil
}
