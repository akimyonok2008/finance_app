package moderation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeUsers struct {
	users     map[string]UserView
	suspended map[string]*time.Time
	banned    map[string]bool
}

func newFakeUsers() *fakeUsers {
	return &fakeUsers{
		users:     map[string]UserView{},
		suspended: map[string]*time.Time{},
		banned:    map[string]bool{},
	}
}

func (f *fakeUsers) UserByID(_ context.Context, id string) (UserView, error) {
	u, ok := f.users[id]
	if !ok {
		return UserView{}, assert.AnError
	}
	return u, nil
}

func (f *fakeUsers) Suspend(_ context.Context, userID string, until *time.Time, _ string) error {
	f.suspended[userID] = until
	return nil
}

func (f *fakeUsers) Ban(_ context.Context, userID string, _ string) error {
	f.banned[userID] = true
	return nil
}

type fakeMessages struct {
	byID map[string]MessageEvidence
	// accessDenied lists requesterIDs that are NOT participants for any message.
	accessDenied map[string]bool
}

func (f *fakeMessages) MessageForReport(_ context.Context, requesterID, messageID string) (MessageEvidence, error) {
	if f.accessDenied[requesterID] {
		return MessageEvidence{}, assert.AnError
	}
	e, ok := f.byID[messageID]
	if !ok {
		return MessageEvidence{}, assert.AnError
	}
	return e, nil
}

type fakeRemover struct {
	removed []string
}

func (f *fakeRemover) RemoveMessage(_ context.Context, _, messageID string) error {
	f.removed = append(f.removed, messageID)
	return nil
}

func newTestService() (*Service, *fakeUsers, *fakeMessages, *fakeRemover) {
	repo := NewInMemoryRepository()
	users := newFakeUsers()
	users.users["reporter-1"] = UserView{ID: "reporter-1", Role: "user"}
	users.users["target-1"] = UserView{ID: "target-1", Role: "user"}
	users.users["admin-target"] = UserView{ID: "admin-target", Role: "admin", IsAdmin: true}
	svc := NewService(repo, users)
	messages := &fakeMessages{byID: map[string]MessageEvidence{}, accessDenied: map[string]bool{}}
	remover := &fakeRemover{}
	svc.SetMessageAccessor(messages)
	svc.SetMessageRemover(remover)
	return svc, users, messages, remover
}

func TestService_ReportUser(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newTestService()

	out, err := svc.ReportUser(ctx, "reporter-1", "target-1", CategoryHarassment, "being awful")
	require.NoError(t, err)
	assert.Equal(t, StatusOpen, out.Status)

	_, err = svc.ReportUser(ctx, "reporter-1", "reporter-1", CategorySpam, "")
	assert.ErrorIs(t, err, ErrSelfReport)

	_, err = svc.ReportUser(ctx, "reporter-1", "target-1", "not-a-category", "")
	assert.ErrorIs(t, err, ErrInvalidCategory)
}

func TestService_ReportUserDuplicateRejected(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newTestService()

	_, err := svc.ReportUser(ctx, "reporter-1", "target-1", CategoryHarassment, "")
	require.NoError(t, err)

	_, err = svc.ReportUser(ctx, "reporter-1", "target-1", CategorySpam, "")
	assert.ErrorIs(t, err, ErrDuplicateReport)
}

func TestService_ReportMessageRequiresAccessAndPreservesEvidence(t *testing.T) {
	ctx := context.Background()
	svc, _, messages, _ := newTestService()
	messages.byID["msg-1"] = MessageEvidence{
		MessageID: "msg-1", ConversationID: "conv-1", SenderID: "target-1",
		ParticipantIDs: []string{"reporter-1", "target-1"}, Text: "abusive content", CreatedAt: time.Now(),
	}

	out, err := svc.ReportMessage(ctx, "reporter-1", "msg-1", CategoryThreats, "scared")
	require.NoError(t, err)

	detail, err := svc.GetReportDetail(ctx, out.ReportID)
	require.NoError(t, err)
	require.NotNil(t, detail.Evidence)
	assert.Equal(t, "abusive content", detail.Evidence.MessageText)

	// Inaccessible message (not a participant) cannot be reported.
	messages.accessDenied["outsider"] = true
	_, err = svc.ReportMessage(ctx, "outsider", "msg-1", CategoryThreats, "")
	assert.ErrorIs(t, err, ErrMessageNotAccessible)
}

func TestService_ResolveReportAppliesSuspensionAndAudit(t *testing.T) {
	ctx := context.Background()
	svc, users, _, _ := newTestService()

	rep, err := svc.ReportUser(ctx, "reporter-1", "target-1", CategoryHarassment, "")
	require.NoError(t, err)

	actor := UserView{ID: "mod-1", Role: "moderator"}
	detail, err := svc.ResolveReport(ctx, "mod-1", actor, rep.ReportID, ResolveInput{
		Decision: "action_taken", ActionType: ActionTemporarySuspension, Reason: "harassment", SuspensionDays: 3,
	})
	require.NoError(t, err)
	assert.Equal(t, StatusResolvedActionTaken, detail.Status)
	assert.NotNil(t, users.suspended["target-1"])

	// Resolving again is idempotent: it returns the existing resolution
	// rather than re-applying the suspension or erroring.
	again, err := svc.ResolveReport(ctx, "mod-1", actor, rep.ReportID, ResolveInput{
		Decision: "action_taken", ActionType: ActionPermanentBan,
	})
	require.NoError(t, err)
	assert.Equal(t, detail.Status, again.Status)
	assert.False(t, users.banned["target-1"]) // ban was NOT applied on the idempotent retry
}

func TestService_ModeratorCannotActAgainstAdminUnlessAdmin(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newTestService()

	rep, err := svc.ReportUser(ctx, "reporter-1", "admin-target", CategoryHarassment, "")
	require.NoError(t, err)

	modActor := UserView{ID: "mod-1", Role: "moderator"}
	_, err = svc.ResolveReport(ctx, "mod-1", modActor, rep.ReportID, ResolveInput{
		Decision: "action_taken", ActionType: ActionWarning,
	})
	assert.ErrorIs(t, err, ErrModerationForbidden)

	adminActor := UserView{ID: "admin-2", Role: "admin", IsAdmin: true}
	_, err = svc.ResolveReport(ctx, "admin-2", adminActor, rep.ReportID, ResolveInput{
		Decision: "action_taken", ActionType: ActionWarning,
	})
	require.NoError(t, err)
}

func TestService_DeletedTargetCanStillBeResolvedWithoutLiveAccountAction(t *testing.T) {
	ctx := context.Background()
	repo := NewInMemoryRepository()
	users := newFakeUsers()
	svc := NewService(repo, users)

	_, err := repo.CreateUserReport(ctx, Report{
		ID: "deleted-target-report", ReporterUserID: "reporter-1",
		ReportedDeleted: true, Category: CategoryHarassment,
		Status: StatusOpen, CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	detail, err := svc.ResolveReport(ctx, "mod-1", UserView{ID: "mod-1", Role: "moderator"},
		"deleted-target-report", ResolveInput{Decision: "no_action"})
	require.NoError(t, err)
	assert.Equal(t, StatusResolvedNoAction, detail.Status)
	assert.Equal(t, deletedAccountLabel, detail.ReportedUserID)

	evidenceText := "retained evidence"
	_, err = repo.CreateMessageReport(ctx, Report{
		ID: "deleted-target-content-removal", ReporterUserID: "reporter-1",
		ReportedDeleted: true, Category: CategoryHarassment,
		Status: StatusOpen, CreatedAt: time.Now().UTC(),
	}, Evidence{
		ID: "deleted-target-evidence", ReportID: "deleted-target-content-removal",
		MessageText: &evidenceText, ReportCreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	detail, err = svc.ResolveReport(ctx, "mod-1", UserView{ID: "mod-1", Role: "moderator"},
		"deleted-target-content-removal", ResolveInput{
			Decision: "action_taken", ActionType: ActionContentRemoval,
		})
	require.NoError(t, err)
	assert.Equal(t, StatusResolvedActionTaken, detail.Status)
	require.NotNil(t, detail.Evidence)
	assert.Equal(t, evidenceText, detail.Evidence.MessageText)

	_, err = repo.CreateUserReport(ctx, Report{
		ID: "deleted-target-live-action", ReporterUserID: "reporter-1",
		ReportedDeleted: true, Category: CategoryHarassment,
		Status: StatusOpen, CreatedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	_, err = svc.ResolveReport(ctx, "mod-1", UserView{ID: "mod-1", Role: "moderator"},
		"deleted-target-live-action", ResolveInput{
			Decision: "action_taken", ActionType: ActionPermanentBan,
		})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestService_NotificationsReadState(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := newTestService()

	require.NoError(t, svc.Notify(ctx, "user-x", NotifModerationWarning, "dedupe-1", map[string]string{"reason": "test"}))
	require.NoError(t, svc.Notify(ctx, "user-x", NotifModerationWarning, "dedupe-1", map[string]string{"reason": "retried"}))

	count, err := svc.UnreadNotificationCount(ctx, "user-x")
	require.NoError(t, err)
	assert.Equal(t, 1, count.Count) // dedupe prevented a duplicate

	list, err := svc.Notifications(ctx, "user-x")
	require.NoError(t, err)
	require.Len(t, list.Notifications, 1)

	require.NoError(t, svc.MarkAllNotificationsRead(ctx, "user-x"))
	count, err = svc.UnreadNotificationCount(ctx, "user-x")
	require.NoError(t, err)
	assert.Equal(t, 0, count.Count)
}
