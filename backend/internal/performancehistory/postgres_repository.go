package performancehistory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

type PostgresRepository struct{ pool *pgxpool.Pool }

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Insert(ctx context.Context, s Snapshot) (bool, error) {
	if !s.Valid() {
		return false, ErrInvalidSnapshot
	}
	var id string
	var err error
	args := []any{s.ID, s.PortfolioID, s.UserID, s.TrackingStartedAt, s.RankedIndex,
		string(s.RankingStatus), s.CapturedAt, string(s.Kind), s.BucketStart,
		nullDate(s.SnapshotDate), s.ValuationAsOf, string(s.DataQualityStatus), s.CreatedAt,
		evaluationStatusFor(s)}
	base := `INSERT INTO ranked_performance_snapshots (
		id, portfolio_id, user_id, tracking_started_at, ranked_index,
		ranking_status, captured_at, snapshot_kind, bucket_start, snapshot_date,
		valuation_as_of, data_quality_status, created_at, evaluation_status
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) `
	switch s.Kind {
	case KindDaily:
		err = r.pool.QueryRow(ctx, base+`
			ON CONFLICT (portfolio_id, tracking_started_at, snapshot_date)
			WHERE snapshot_kind = 'daily'
			DO UPDATE SET ranked_index=EXCLUDED.ranked_index,
				ranking_status=EXCLUDED.ranking_status,
				captured_at=EXCLUDED.captured_at,
				valuation_as_of=EXCLUDED.valuation_as_of,
				data_quality_status=EXCLUDED.data_quality_status,
				evaluation_status=EXCLUDED.evaluation_status
			WHERE ranked_performance_snapshots.data_quality_status <> 'complete'
			  AND EXCLUDED.data_quality_status = 'complete'
			RETURNING id`, args...).Scan(&id)
	case KindIntraday:
		err = r.pool.QueryRow(ctx, base+`
			ON CONFLICT (portfolio_id, tracking_started_at, bucket_start)
			WHERE snapshot_kind = 'intraday'
			DO UPDATE SET ranked_index=EXCLUDED.ranked_index,
				ranking_status=EXCLUDED.ranking_status,
				captured_at=EXCLUDED.captured_at,
				valuation_as_of=EXCLUDED.valuation_as_of,
				data_quality_status=EXCLUDED.data_quality_status,
				evaluation_status=EXCLUDED.evaluation_status
			WHERE ranked_performance_snapshots.data_quality_status <> 'complete'
			  AND EXCLUDED.data_quality_status = 'complete'
			RETURNING id`, args...).Scan(&id)
	default:
		err = r.pool.QueryRow(ctx, base+`
			ON CONFLICT (portfolio_id, tracking_started_at, snapshot_kind, captured_at)
			WHERE snapshot_kind = 'transition'
			DO NOTHING RETURNING id`, args...).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("ranked snapshots: insert: %w", err)
	}
	return true, nil
}

func evaluationStatusFor(s Snapshot) string {
	if s.DataQualityStatus == QualityComplete && s.Kind != KindDaily {
		return "pending"
	}
	return "done"
}

func nullDate(date string) any {
	if date == "" {
		return nil
	}
	return date
}

func scanSnapshot(row pgx.Row) (Snapshot, error) {
	var s Snapshot
	var status, kind, quality string
	var snapshotDate *time.Time
	err := row.Scan(&s.ID, &s.PortfolioID, &s.UserID, &s.TrackingStartedAt,
		&s.RankedIndex, &status, &s.CapturedAt, &kind, &s.BucketStart,
		&snapshotDate, &s.ValuationAsOf, &quality, &s.EvidenceProtected, &s.CreatedAt)
	if snapshotDate != nil {
		s.SnapshotDate = snapshotDate.UTC().Format("2006-01-02")
	}
	s.RankingStatus = performanceStatus(status)
	s.Kind = SnapshotKind(kind)
	s.DataQualityStatus = QualityStatus(quality)
	return s, err
}

func performanceStatus(value string) performance.Status {
	return performance.Status(value)
}

const snapshotColumns = `id, portfolio_id, user_id, tracking_started_at,
	ranked_index, ranking_status, captured_at, snapshot_kind, bucket_start,
	snapshot_date, valuation_as_of, data_quality_status, evidence_protected, created_at`

func (r *PostgresRepository) List(ctx context.Context, userID string, from, to time.Time) ([]Snapshot, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+snapshotColumns+`
		FROM ranked_performance_snapshots
		WHERE user_id=$1 AND captured_at >= $2 AND captured_at <= $3
		ORDER BY captured_at, snapshot_kind`, userID, from.UTC(), to.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snapshot{}
	for rows.Next() {
		s, err := scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) IndexAtOrBefore(ctx context.Context, userID string, cutoff, epoch time.Time) (float64, bool, error) {
	var index float64
	err := r.pool.QueryRow(ctx, `SELECT ranked_index
		FROM ranked_performance_snapshots
		WHERE user_id=$1 AND tracking_started_at=$2 AND captured_at <= $3
		  AND data_quality_status='complete'
		ORDER BY captured_at DESC LIMIT 1`, userID, epoch.UTC(), cutoff.UTC()).Scan(&index)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return index, err == nil, err
}

func (r *PostgresRepository) Latest(ctx context.Context, userID, portfolioID string, epoch time.Time) (Snapshot, bool, error) {
	s, err := scanSnapshot(r.pool.QueryRow(ctx, `SELECT `+snapshotColumns+`
		FROM ranked_performance_snapshots
		WHERE user_id=$1 AND portfolio_id=$2 AND tracking_started_at=$3
		ORDER BY captured_at DESC LIMIT 1`, userID, portfolioID, epoch.UTC()))
	if errors.Is(err, pgx.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	return s, err == nil, err
}

func (r *PostgresRepository) StatusAtOrBefore(ctx context.Context, portfolioID string, epoch, at time.Time) (performance.Status, bool, error) {
	var status string
	err := r.pool.QueryRow(ctx, `SELECT ranking_status
		FROM ranked_performance_snapshots
		WHERE portfolio_id=$1 AND tracking_started_at=$2 AND captured_at <= $3
		ORDER BY captured_at DESC LIMIT 1`, portfolioID, epoch.UTC(), at.UTC()).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return performance.Status(status), err == nil, err
}

func (r *PostgresRepository) Protect(ctx context.Context, ids ...string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `UPDATE ranked_performance_snapshots
		SET evidence_protected=TRUE WHERE id = ANY($1::uuid[])`, ids)
	return err
}

func (r *PostgresRepository) Compact(ctx context.Context, before time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM ranked_performance_snapshots intraday
		WHERE intraday.snapshot_kind='intraday'
		  AND intraday.captured_at < $1
		  AND NOT intraday.evidence_protected
		  AND EXISTS (
			SELECT 1 FROM ranked_performance_snapshots daily
			WHERE daily.portfolio_id=intraday.portfolio_id
			  AND daily.tracking_started_at=intraday.tracking_started_at
			  AND daily.snapshot_kind='daily'
			  AND daily.snapshot_date=(intraday.captured_at AT TIME ZONE 'UTC')::date
			  AND daily.data_quality_status='complete'
		  )`, before.UTC())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *PostgresRepository) ClaimEvaluations(ctx context.Context, limit int, retryBefore time.Time) ([]EvaluationClaim, error) {
	rows, err := r.pool.Query(ctx, `WITH claimed AS (
		SELECT id FROM ranked_performance_snapshots
		WHERE evaluation_status='pending'
		   OR (evaluation_status='processing' AND evaluation_claimed_at < $2)
		ORDER BY captured_at
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	)
	UPDATE ranked_performance_snapshots s
	SET evaluation_status='processing', evaluation_claimed_at=now()
	FROM claimed WHERE s.id=claimed.id
	RETURNING s.id, s.user_id`, limit, retryBefore.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EvaluationClaim{}
	for rows.Next() {
		var claim EvaluationClaim
		if err := rows.Scan(&claim.SnapshotID, &claim.UserID); err != nil {
			return nil, err
		}
		out = append(out, claim)
	}
	return out, rows.Err()
}

func (r *PostgresRepository) CompleteEvaluation(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE ranked_performance_snapshots
		SET evaluation_status='done', evaluation_claimed_at=NULL,
		    evaluation_last_error=NULL WHERE id=$1`, id)
	return err
}

func (r *PostgresRepository) FailEvaluation(ctx context.Context, id, cause string) error {
	_, err := r.pool.Exec(ctx, `UPDATE ranked_performance_snapshots
		SET evaluation_status='pending', evaluation_claimed_at=NULL,
		    evaluation_attempts=evaluation_attempts+1,
		    evaluation_last_error=$2 WHERE id=$1`, id, cause)
	return err
}
