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

	var expectedUntil time.Time
	build := func(ctx context.Context, current Report) (ResolveWrite, error) {
		now := time.Now().UTC()
		reviewer := moderator
		updated := current
		updated.Status = StatusResolvedActionTaken
		updated.Decision = "action_taken"
		updated.ReviewedAt = &now
		updated.ReviewerID = &reviewer
		until := now.AddDate(0, 0, 3)
		expectedUntil = until
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

	var suspendedUntil *time.Time
	var suspensionReason *string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT suspended_until, suspension_reason FROM users WHERE id = $1
	`, target).Scan(&suspendedUntil, &suspensionReason))
	require.NotNil(t, suspendedUntil)
	require.NotNil(t, suspensionReason)
	assert.WithinDuration(t, expectedUntil, *suspendedUntil, time.Second)
	assert.Equal(t, "test", *suspensionReason)

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

func TestPostgresRepository_CommitFailureRollsBackDurableResolutionEffects(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// A deferred constraint trigger raises from tx.Commit, after ResolveReport
	// has already executed the account/content UPDATE. This reproduces the
	// exact failure window that used to leave an external mutation committed.
	_, err := pool.Exec(ctx, `
		CREATE OR REPLACE FUNCTION moderation_test_fail_resolution_commit()
		RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN
			IF NEW.reason = 'force deferred commit failure' THEN
				RAISE EXCEPTION 'forced deferred commit failure';
			END IF;
			RETURN NEW;
		END
		$$;
		DROP TRIGGER IF EXISTS moderation_test_fail_resolution_commit ON moderation_actions;
		CREATE CONSTRAINT TRIGGER moderation_test_fail_resolution_commit
			AFTER INSERT ON moderation_actions
			DEFERRABLE INITIALLY DEFERRED
			FOR EACH ROW EXECUTE FUNCTION moderation_test_fail_resolution_commit();
	`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DROP TRIGGER IF EXISTS moderation_test_fail_resolution_commit ON moderation_actions;
			DROP FUNCTION IF EXISTS moderation_test_fail_resolution_commit();
		`)
	})

	for _, actionType := range []string{
		ActionTemporarySuspension,
		ActionPermanentBan,
		ActionContentRemoval,
	} {
		t.Run(actionType, func(t *testing.T) {
			repo := NewPostgresRepository(pool)
			reporter := newPGUser(t, pool, "Reporter")
			target := newPGUser(t, pool, "Target")
			moderator := newPGUser(t, pool, "Moderator")
			now := time.Now().UTC()

			rep := Report{
				ID: uuid.NewString(), ReporterUserID: reporter, ReportedUserID: target,
				Category: CategoryHarassment, Status: StatusOpen, CreatedAt: now,
			}
			var messageID string
			if actionType == ActionContentRemoval {
				messageID = newPGMessage(t, pool, target, reporter, "reported message")
				rep.MessageID = &messageID
				_, err = repo.CreateMessageReport(ctx, rep, Evidence{
					ID: uuid.NewString(), ReportID: rep.ID, MessageID: &messageID,
					ParticipantIDs: []string{reporter, target}, ReportCreatedAt: now,
				})
			} else {
				_, err = repo.CreateUserReport(ctx, rep)
			}
			require.NoError(t, err)

			_, applied, err := repo.ResolveReport(ctx, rep.ID, func(_ context.Context, current Report) (ResolveWrite, error) {
				reviewer := moderator
				updated := current
				updated.Status = StatusResolvedActionTaken
				updated.Decision = "action_taken"
				updated.ReviewedAt = &now
				updated.ReviewerID = &reviewer
				var until *time.Time
				if actionType == ActionTemporarySuspension {
					value := now.Add(72 * time.Hour)
					until = &value
				}
				return ResolveWrite{
					Report: updated,
					Action: ModerationAction{
						ID: uuid.NewString(), ModeratorID: moderator, TargetUserID: target,
						ReportID: &current.ID, ActionType: actionType,
						Reason: "force deferred commit failure", CreatedAt: now, ExpiresAt: until,
					},
					ActionType: actionType, SuspendUntil: until,
				}, nil
			})
			require.Error(t, err)
			assert.False(t, applied)

			current, getErr := repo.GetReport(ctx, rep.ID)
			require.NoError(t, getErr)
			assert.Equal(t, StatusOpen, current.Status)

			var actionCount int
			require.NoError(t, pool.QueryRow(ctx,
				`SELECT count(*) FROM moderation_actions WHERE report_id = $1`, rep.ID,
			).Scan(&actionCount))
			assert.Zero(t, actionCount)

			switch actionType {
			case ActionTemporarySuspension:
				var affected bool
				require.NoError(t, pool.QueryRow(ctx,
					`SELECT suspended_until IS NOT NULL FROM users WHERE id = $1`, target,
				).Scan(&affected))
				assert.False(t, affected, "commit failure must roll back suspension")
			case ActionPermanentBan:
				var affected bool
				require.NoError(t, pool.QueryRow(ctx,
					`SELECT banned_at IS NOT NULL FROM users WHERE id = $1`, target,
				).Scan(&affected))
				assert.False(t, affected, "commit failure must roll back ban")
			case ActionContentRemoval:
				var affected bool
				require.NoError(t, pool.QueryRow(ctx,
					`SELECT removed_at IS NOT NULL FROM dm_messages WHERE id = $1`, messageID,
				).Scan(&affected))
				assert.False(t, affected, "commit failure must roll back message removal")
			}
		})
	}
}

func TestPostgresRepository_SetNullOnAccountHardDeleteRetainsReport(t *testing.T) {
	pool := testPool(t)
	repo := NewPostgresRepository(pool)
	ctx := context.Background()

	reporter := newPGUser(t, pool, "Reporter")
	target := newPGUser(t, pool, "Target")
	moderator := newPGUser(t, pool, "Moderator")
	messageID := newPGMessage(t, pool, target, reporter, "retained evidence")
	var conversationID string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT conversation_id FROM dm_messages WHERE id = $1`, messageID,
	).Scan(&conversationID))

	rep := Report{
		ID: uuid.NewString(), ReporterUserID: reporter, ReportedUserID: target,
		MessageID: &messageID, Category: CategoryHarassment,
		Status: StatusOpen, CreatedAt: time.Now().UTC(),
	}
	senderID := target
	saved, err := repo.CreateMessageReport(ctx, rep, Evidence{
		ID: uuid.NewString(), ReportID: rep.ID, MessageID: &messageID,
		ConversationID: &conversationID, SenderID: &senderID,
		ParticipantIDs:  []string{reporter, target},
		ReportCreatedAt: rep.CreatedAt,
	})
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

	var (
		rawSender             *string
		rawConversation       *string
		rawParticipants       []string
		senderSubject         string
		participantSubjectIDs []string
		conversationSubject   string
	)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT sender_id, conversation_id, participant_ids, sender_subject_id,
		       participant_subject_ids, conversation_subject_id
		FROM report_evidence WHERE report_id = $1
	`, saved.ID).Scan(
		&rawSender, &rawConversation, &rawParticipants, &senderSubject,
		&participantSubjectIDs, &conversationSubject,
	))
	assert.Nil(t, rawSender)
	assert.Nil(t, rawConversation)
	assert.Empty(t, rawParticipants)
	assert.Equal(t, subjectID, senderSubject)
	assert.Contains(t, participantSubjectIDs, subjectID)
	assert.NotEmpty(t, conversationSubject)

	var mappingCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM moderation_subject_ids WHERE user_id = $1`, target,
	).Scan(&mappingCount))
	assert.Zero(t, mappingCount, "account erasure must destroy the raw user-to-subject mapping")

	// A final content-removal decision remains writable even though account
	// deletion already cascaded the live message away. The audit row copies
	// the report's subject id and leaves target_user_id NULL.
	_, applied, err := repo.ResolveReport(ctx, saved.ID, func(_ context.Context, current Report) (ResolveWrite, error) {
		now := time.Now().UTC()
		reviewer := moderator
		updated := current
		updated.Status = StatusResolvedActionTaken
		updated.Decision = "action_taken"
		updated.ReviewedAt = &now
		updated.ReviewerID = &reviewer
		return ResolveWrite{
			Report: updated,
			Action: ModerationAction{
				ID: uuid.NewString(), ModeratorID: moderator, TargetUserID: current.ReportedUserID,
				ReportID: &current.ID, ActionType: ActionContentRemoval, CreatedAt: now,
			},
			ActionType: ActionContentRemoval,
		}, nil
	})
	require.NoError(t, err)
	assert.True(t, applied)

	var actionTarget *string
	var actionTargetSubject string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT target_user_id, target_subject_id
		FROM moderation_actions WHERE report_id = $1
	`, saved.ID).Scan(&actionTarget, &actionTargetSubject))
	assert.Nil(t, actionTarget)
	assert.Equal(t, subjectID, actionTargetSubject)
}
