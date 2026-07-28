package moderation

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- report + evidence atomicity -------------------------------------------

func TestInMemoryRepo_MessageReportEvidenceFailureLeavesNoReport(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	repo.SetFailHook(func(stage string) error {
		if stage == "evidence_insert" {
			return assert.AnError
		}
		return nil
	})

	rep := Report{ID: "r1", ReporterUserID: "u1", ReportedUserID: "u2", Status: StatusOpen}
	_, err := repo.CreateMessageReport(ctx, rep, Evidence{ID: "e1", ReportID: "r1"})
	require.Error(t, err)

	_, err = repo.GetReport(ctx, "r1")
	assert.ErrorIs(t, err, ErrNotFound, "evidence failure must roll back the report insert too")
	_, ok, _ := repo.GetEvidence(ctx, "r1")
	assert.False(t, ok)
}

func TestInMemoryRepo_MessageReportEvidenceSuccessCommitsBoth(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	rep := Report{ID: "r1", ReporterUserID: "u1", ReportedUserID: "u2", Status: StatusOpen}
	_, err := repo.CreateMessageReport(ctx, rep, Evidence{ID: "e1", ReportID: "r1"})
	require.NoError(t, err)

	_, err = repo.GetReport(ctx, "r1")
	require.NoError(t, err)
	_, ok, _ := repo.GetEvidence(ctx, "r1")
	assert.True(t, ok)
}

// --- duplicate reports under concurrency -----------------------------------

func TestInMemoryRepo_ConcurrentDuplicateUserReportsRejected(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()

	const n = 20
	var wg sync.WaitGroup
	var successes int32
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rep := Report{ID: uuidLike(i), ReporterUserID: "reporter", ReportedUserID: "target", Status: StatusOpen}
			if _, err := repo.CreateUserReport(ctx, rep); err == nil {
				atomic.AddInt32(&successes, 1)
			}
		}(i)
	}
	wg.Wait()
	assert.EqualValues(t, 1, successes, "exactly one concurrent duplicate report should succeed")
}

func uuidLike(i int) string {
	return "report-" + string(rune('a'+i))
}

// --- resolution atomicity per action type ----------------------------------

func TestService_ResolutionAtomicity_AllActionTypes(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		actionType string
		decision   string
	}{
		{"warning", ActionWarning, "action_taken"},
		{"suspension", ActionTemporarySuspension, "action_taken"},
		{"ban", ActionPermanentBan, "action_taken"},
		{"content_removal", ActionContentRemoval, "action_taken"},
		{"no_action", ActionNoAction, "no_action"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, messages, _ := newTestService()
			messages.byID["msg-x"] = MessageEvidence{
				MessageID: "msg-x", ConversationID: "conv-1", SenderID: "target-1",
				ParticipantIDs: []string{"reporter-1", "target-1"}, Text: "bad", CreatedAt: clockNow(),
			}
			rep, err := svc.ReportMessage(ctx, "reporter-1", "msg-x", CategoryHarassment, "")
			require.NoError(t, err)

			actor := UserView{ID: "mod-1", Role: "moderator"}
			in := ResolveInput{Decision: tc.decision, ActionType: tc.actionType, Reason: "r"}
			detail, err := svc.ResolveReport(ctx, "mod-1", actor, rep.ReportID, in)
			require.NoError(t, err)

			memRepo := svc.repo.(*InMemoryRepository)
			// Report resolved.
			assert.Contains(t, []string{StatusResolvedActionTaken, StatusResolvedNoAction}, detail.Status)
			// Audit action recorded exactly once.
			found := 0
			for _, a := range memRepo.actions {
				if a.ReportID != nil && *a.ReportID == rep.ReportID {
					found++
				}
			}
			assert.Equal(t, 1, found, "exactly one audit action for %s", tc.name)
		})
	}
}

// --- idempotent retry: same resolution, no re-apply ------------------------

func TestService_ResolveRetryReturnsSameResolutionNoReapply(t *testing.T) {
	ctx := context.Background()
	svc, users, _, _ := newTestService()

	rep, err := svc.ReportUser(ctx, "reporter-1", "target-1", CategoryHarassment, "")
	require.NoError(t, err)

	actor := UserView{ID: "mod-1", Role: "moderator"}
	first, err := svc.ResolveReport(ctx, "mod-1", actor, rep.ReportID, ResolveInput{
		Decision: "action_taken", ActionType: ActionTemporarySuspension, Reason: "x", SuspensionDays: 3,
	})
	require.NoError(t, err)
	firstUntil := *users.suspended["target-1"]

	// Simulate a client retry (e.g. after an ambiguous network failure).
	second, err := svc.ResolveReport(ctx, "mod-1", actor, rep.ReportID, ResolveInput{
		Decision: "action_taken", ActionType: ActionTemporarySuspension, Reason: "x", SuspensionDays: 3,
	})
	require.NoError(t, err)

	assert.Equal(t, first.Status, second.Status)
	assert.Equal(t, first.Decision, second.Decision)
	secondUntil := *users.suspended["target-1"]
	assert.True(t, firstUntil.Equal(secondUntil), "suspension expiration must not be extended on retry")

	memRepo := svc.repo.(*InMemoryRepository)
	count := 0
	for _, a := range memRepo.actions {
		if a.ReportID != nil && *a.ReportID == rep.ReportID {
			count++
		}
	}
	assert.Equal(t, 1, count, "retry must not record a second audit action")
}

// --- concurrent resolutions produce exactly one action ---------------------

func TestService_ConcurrentResolutionsProduceExactlyOneAction(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newTestService()

	rep, err := svc.ReportUser(ctx, "reporter-1", "target-1", CategoryHarassment, "")
	require.NoError(t, err)

	const n = 25
	var wg sync.WaitGroup
	actor := UserView{ID: "mod-1", Role: "moderator"}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.ResolveReport(ctx, "mod-1", actor, rep.ReportID, ResolveInput{
				Decision: "action_taken", ActionType: ActionTemporarySuspension, Reason: "x", SuspensionDays: 3,
			})
		}()
	}
	wg.Wait()

	memRepo := svc.repo.(*InMemoryRepository)
	count := 0
	for _, a := range memRepo.actions {
		if a.ReportID != nil && *a.ReportID == rep.ReportID {
			count++
		}
	}
	assert.Equal(t, 1, count, "concurrent resolutions must produce exactly one action")
}

// --- injected failure at each write stage leaves no partial state ----------

func TestInMemoryRepo_ResolveReportFailureStagesLeaveNoPartialState(t *testing.T) {
	stages := []string{"action_insert", "external_apply", "notification_insert", "report_update"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			ctx := context.Background()
			svc, users, _, _ := newTestService()
			rep, err := svc.ReportUser(ctx, "reporter-1", "target-1", CategoryHarassment, "")
			require.NoError(t, err)

			memRepo := svc.repo.(*InMemoryRepository)
			memRepo.SetFailHook(func(s string) error {
				if s == stage {
					return assert.AnError
				}
				return nil
			})

			actor := UserView{ID: "mod-1", Role: "moderator"}
			_, err = svc.ResolveReport(ctx, "mod-1", actor, rep.ReportID, ResolveInput{
				Decision: "action_taken", ActionType: ActionTemporarySuspension, Reason: "x", SuspensionDays: 3,
			})
			require.Error(t, err)
			memRepo.SetFailHook(nil)

			// Report must still be open; no audit action; no suspension left applied as "resolved".
			current, err := memRepo.GetReport(ctx, rep.ReportID)
			require.NoError(t, err)
			assert.Equal(t, StatusOpen, current.Status, "report must remain open after a failure at stage %s", stage)

			count := 0
			for _, a := range memRepo.actions {
				if a.ReportID != nil && *a.ReportID == rep.ReportID {
					count++
				}
			}
			assert.Equal(t, 0, count, "no audit action should be recorded after a failure at stage %s", stage)
			_, suspended := users.suspended["target-1"]
			assert.False(t, suspended, "no suspension should survive a failure at stage %s", stage)
		})
	}
}
