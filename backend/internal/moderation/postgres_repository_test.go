package moderation

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/db"
)

// testPool connects to the integration-test database, applying migrations
// (including 0028_moderation_retention.sql). Tests are skipped when
// DATABASE_URL_TEST is unset so the suite stays green without local
// infrastructure — set it (e.g. to the docker-compose postgres service) to
// exercise these against a real Postgres instance.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres integration test")
	}
	pool, err := db.ConnectPostgres(context.Background(), url)
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(context.Background(), pool))
	t.Cleanup(pool.Close)
	return pool
}

func newPGUser(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	repo := auth.NewPostgresUserRepository(pool)
	u := &auth.User{
		ID: uuid.NewString(), Email: uuid.NewString() + "@example.com",
		DisplayName: name, AvatarKey: "fox", PasswordHash: "bcrypt-hash",
	}
	require.NoError(t, repo.Create(u))
	return u.ID
}

// newPGMessage inserts a real dm_conversations + dm_messages row so a
// report's message_id FK (message_id -> dm_messages(id) ON DELETE SET NULL)
// has something valid to reference.
func newPGMessage(t *testing.T, pool *pgxpool.Pool, senderID, otherID, body string) string {
	t.Helper()
	ctx := context.Background()
	convID := uuid.NewString()
	_, err := pool.Exec(ctx, `
		INSERT INTO dm_conversations (id, participant_a_user_id, participant_b_user_id, pair_key)
		VALUES ($1, $2, $3, $4)
	`, convID, senderID, otherID, senderID+":"+otherID)
	require.NoError(t, err)
	msgID := uuid.NewString()
	_, err = pool.Exec(ctx, `
		INSERT INTO dm_messages (id, conversation_id, sender_user_id, body)
		VALUES ($1, $2, $3, $4)
	`, msgID, convID, senderID, body)
	require.NoError(t, err)
	return msgID
}

func TestPostgresRepository_MessageReportAndEvidenceCommitTogether(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()

	reporter := newPGUser(t, pool, "Reporter")
	target := newPGUser(t, pool, "Target")
	msgID := newPGMessage(t, pool, reporter, target, "abusive text")

	rep := Report{
		ID: uuid.NewString(), ReporterUserID: reporter, ReportedUserID: target,
		MessageID: &msgID, Category: CategoryHarassment, Status: StatusOpen, CreatedAt: time.Now().UTC(),
	}
	ev := Evidence{
		ID: uuid.NewString(), ReportID: rep.ID, MessageID: &msgID,
		ParticipantIDs: []string{reporter, target}, ReportCreatedAt: rep.CreatedAt,
	}

	saved, err := repo.CreateMessageReport(ctx, rep, ev)
	require.NoError(t, err)

	got, err := repo.GetReport(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, reporter, got.ReporterUserID)
	assert.Equal(t, target, got.ReportedUserID)

	_, ok, err := repo.GetEvidence(ctx, saved.ID)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestPostgresRepository_DuplicateOpenReportRejectedByUniqueIndex(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()

	reporter := newPGUser(t, pool, "Reporter")
	target := newPGUser(t, pool, "Target")

	rep := Report{
		ID: uuid.NewString(), ReporterUserID: reporter, ReportedUserID: target,
		Category: CategoryHarassment, Status: StatusOpen, CreatedAt: time.Now().UTC(),
	}
	_, err := repo.CreateUserReport(ctx, rep)
	require.NoError(t, err)

	dup := rep
	dup.ID = uuid.NewString()
	_, err = repo.CreateUserReport(ctx, dup)
	assert.ErrorIs(t, err, ErrDuplicateReport)
}

func TestPostgresRepository_ResolveReportAtomicAndIdempotent(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()

	reporter := newPGUser(t, pool, "Reporter")
	target := newPGUser(t, pool, "Target")
	moderator := newPGUser(t, pool, "Mod")

	rep := Report{
		ID: uuid.NewString(), ReporterUserID: reporter, ReportedUserID: target,
		Category: CategoryHarassment, Status: StatusOpen, CreatedAt: time.Now().UTC(),
	}
	saved, err := repo.CreateUserReport(ctx, rep)
	require.NoError(t, err)

	build := func(ctx context.Context, current Report) (ResolveWrite, error) {
		now := time.Now().UTC()
		reviewer := moderator
		updated := current
		updated.Status = StatusResolvedActionTaken
		updated.Decision = "action_taken"
		updated.ReviewedAt = &now
		updated.ReviewerID = &reviewer
		until := now.AddDate(0, 0, 3)
		return ResolveWrite{
			Report: updated,
			Action: ModerationAction{
				ID: uuid.NewString(), ModeratorID: moderator, TargetUserID: current.ReportedUserID,
				ReportID: &current.ID, ActionType: ActionTemporarySuspension, Reason: "test", CreatedAt: now, ExpiresAt: &until,
			},
			ActionType: ActionTemporarySuspension, SuspendUntil: &until,
		}, nil
	}

	final, applied, err := repo.ResolveReport(ctx, saved.ID, build)
	require.NoError(t, err)
	assert.True(t, applied)
	assert.Equal(t, StatusResolvedActionTaken, final.Status)

	// Retry: build must not be invoked again, and the existing resolution is
	// returned unchanged (idempotent — no double action, no expiration drift).
	buildCalled := false
	again, applied2, err := repo.ResolveReport(ctx, saved.ID, func(ctx context.Context, current Report) (ResolveWrite, error) {
		buildCalled = true
		return ResolveWrite{}, nil
	})
	require.NoError(t, err)
	assert.False(t, applied2)
	assert.False(t, buildCalled, "build must not be invoked for an already-resolved report")
	assert.Equal(t, final.Status, again.Status)

	var actionCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM moderation_actions WHERE report_id = $1`, saved.ID).Scan(&actionCount))
	assert.Equal(t, 1, actionCount, "exactly one audit action must exist after the retry")
}

func TestPostgresRepository_SetNullOnAccountHardDeleteRetainsReport(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()

	reporter := newPGUser(t, pool, "Reporter")
	target := newPGUser(t, pool, "Target")

	rep := Report{
		ID: uuid.NewString(), ReporterUserID: reporter, ReportedUserID: target,
		Category: CategoryHarassment, Status: StatusOpen, CreatedAt: time.Now().UTC(),
	}
	saved, err := repo.CreateUserReport(ctx, rep)
	require.NoError(t, err)

	// Simulate a hard delete of the reported (target) account directly —
	// migration 0028 changed user_reports.reported_user_id from
	// ON DELETE CASCADE to ON DELETE SET NULL, so the report row (and its
	// evidence, and any moderation_actions) must survive.
	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, target)
	require.NoError(t, err)

	got, err := repo.GetReport(ctx, saved.ID)
	require.NoError(t, err, "report must still exist after the reported account is hard-deleted")
	assert.True(t, got.ReportedDeleted)
	assert.Equal(t, "", got.ReportedUserID)
	assert.Equal(t, reporter, got.ReporterUserID)

	var subjectID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT reported_subject_id FROM user_reports WHERE id = $1`, saved.ID).Scan(&subjectID))
	assert.NotEmpty(t, subjectID, "the pseudonymous subject id must be retained even though the live FK is now null")
}
