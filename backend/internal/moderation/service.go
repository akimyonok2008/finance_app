package moderation

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MessageEvidence is the evidence-relevant projection of a DM message.
type MessageEvidence struct {
	MessageID      string
	ConversationID string
	SenderID       string
	ParticipantIDs []string
	Text           string
	CreatedAt      time.Time
}

// MessageAccessor lets Service pull evidence for a message report while
// enforcing that the reporter is actually a participant in its conversation
// (never an arbitrary message id belonging to someone else's conversation).
type MessageAccessor interface {
	MessageForReport(ctx context.Context, requesterID, messageID string) (MessageEvidence, error)
}

// MessageRemover applies a moderator content_removal action: the message
// becomes a tombstone in normal conversation history, its text no longer
// reachable through ordinary messaging endpoints.
type MessageRemover interface {
	RemoveMessage(ctx context.Context, moderatorID, messageID string) error
}

// UserView is the narrow account projection moderation needs.
type UserView struct {
	ID      string
	Role    string
	IsAdmin bool
}

// UserAdmin is the narrow account-mutation boundary moderation needs,
// implemented by an adapter over auth.Service so this package never imports
// auth directly (avoiding a dependency cycle risk and keeping the
// interface explicit about exactly what moderation can do to an account).
type UserAdmin interface {
	UserByID(ctx context.Context, id string) (UserView, error)
	Suspend(ctx context.Context, userID string, until *time.Time, reason string) error
	Ban(ctx context.Context, userID string, reason string) error
}

type Service struct {
	repo     Repository
	messages MessageAccessor
	remover  MessageRemover
	users    UserAdmin
	now      func() time.Time
}

func NewService(repo Repository, users UserAdmin) *Service {
	return &Service{repo: repo, users: users, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) SetMessageAccessor(m MessageAccessor) { s.messages = m }
func (s *Service) SetMessageRemover(m MessageRemover)   { s.remover = m }

// ReportUser records a report against a user account.
func (s *Service) ReportUser(ctx context.Context, reporterID, reportedUserID, category, explanation string) (ReportResponse, error) {
	if reporterID == reportedUserID {
		return ReportResponse{}, ErrSelfReport
	}
	if !ValidCategories[category] {
		return ReportResponse{}, ErrInvalidCategory
	}
	rep := Report{
		ID: uuid.NewString(), ReporterUserID: reporterID, ReportedUserID: reportedUserID,
		Category: category, Explanation: strings.TrimSpace(explanation),
		Status: StatusOpen, CreatedAt: s.now(),
	}
	// CreateUserReport re-checks the open-duplicate constraint inside its own
	// atomic write, closing the check-then-insert race a separate
	// HasOpenDuplicate call followed by CreateReport would leave open.
	rep, err := s.repo.CreateUserReport(ctx, rep)
	if err != nil {
		return ReportResponse{}, err
	}
	return ReportResponse{ReportID: rep.ID, Status: rep.Status}, nil
}

// ReportMessage records a report against a single DM message, preserving an
// immutable evidence snapshot at the moment of report creation.
func (s *Service) ReportMessage(ctx context.Context, reporterID, messageID, category, explanation string) (ReportResponse, error) {
	if !ValidCategories[category] {
		return ReportResponse{}, ErrInvalidCategory
	}
	if s.messages == nil {
		return ReportResponse{}, ErrMessageNotAccessible
	}
	evidence, err := s.messages.MessageForReport(ctx, reporterID, messageID)
	if err != nil {
		return ReportResponse{}, ErrMessageNotAccessible
	}
	if evidence.SenderID == reporterID {
		return ReportResponse{}, ErrSelfReport
	}
	msgID := messageID
	rep := Report{
		ID: uuid.NewString(), ReporterUserID: reporterID, ReportedUserID: evidence.SenderID,
		MessageID: &msgID, Category: category, Explanation: strings.TrimSpace(explanation),
		Status: StatusOpen, CreatedAt: s.now(),
	}
	convID := evidence.ConversationID
	senderID := evidence.SenderID
	text := evidence.Text
	createdAt := evidence.CreatedAt
	ev := Evidence{
		ID: uuid.NewString(), ReportID: rep.ID, MessageID: &msgID, ConversationID: &convID,
		SenderID: &senderID, ParticipantIDs: evidence.ParticipantIDs, MessageText: &text,
		MessageCreatedAt: &createdAt, ReportCreatedAt: rep.CreatedAt,
	}
	// CreateMessageReport writes the report and its evidence snapshot in one
	// atomic operation: if the evidence write fails, the report insert is
	// rolled back too, so no evidence-less report is ever left behind.
	rep, err = s.repo.CreateMessageReport(ctx, rep, ev)
	if err != nil {
		return ReportResponse{}, err
	}
	return ReportResponse{ReportID: rep.ID, Status: rep.Status}, nil
}

// ListReports is moderator/admin-only; enforcement happens via the
// RequireModerator middleware wired in the router.
func (s *Service) ListReports(ctx context.Context, status string) (ReportListResponse, error) {
	reports, err := s.repo.ListReports(ctx, status)
	if err != nil {
		return ReportListResponse{}, err
	}
	out := make([]ReportSummary, 0, len(reports))
	for _, rep := range reports {
		out = append(out, summaryFromReport(rep))
	}
	return ReportListResponse{Reports: out}, nil
}

func (s *Service) GetReportDetail(ctx context.Context, id string) (ReportDetail, error) {
	rep, err := s.repo.GetReport(ctx, id)
	if err != nil {
		return ReportDetail{}, ErrNotFound
	}
	detail := ReportDetail{
		ReportSummary:  summaryFromReport(rep),
		Explanation:    rep.Explanation,
		ReviewedAt:     rep.ReviewedAt,
		ReviewerID:     rep.ReviewerID,
		Decision:       rep.Decision,
		ModeratorNotes: rep.ModeratorNotes,
	}
	// Account deletion can cascade the live message and SET report.message_id
	// to NULL. Evidence is keyed by report_id and must remain reviewable even
	// in that state, so its lookup cannot be gated on the live message FK.
	if evidence, ok, err := s.repo.GetEvidence(ctx, rep.ID); err == nil && ok {
		dto := &EvidenceDTO{}
		if evidence.MessageText != nil {
			dto.MessageText = *evidence.MessageText
		}
		dto.MessageCreatedAt = evidence.MessageCreatedAt
		if evidence.ConversationID != nil {
			dto.ConversationID = *evidence.ConversationID
		} else {
			dto.ConversationID = evidence.ConversationSubjectID
		}
		if evidence.SenderID != nil {
			dto.SenderID = *evidence.SenderID
		} else {
			dto.SenderID = evidence.SenderSubjectID
		}
		detail.Evidence = dto
	}
	return detail, nil
}

// ResolveReport records the moderator's decision, applies the corresponding
// account/content action, writes an immutable audit record, and notifies the
// target user (except for no_action). Resolving an already-resolved report
// is idempotent: it returns the existing resolution unchanged rather than
// erroring or double-applying the action.
func (s *Service) ResolveReport(ctx context.Context, moderatorID string, actorRole UserView, reportID string, in ResolveInput) (ReportDetail, error) {
	// An initial (unlocked) read so we can fail fast on a missing report and
	// on invalid input before ever taking the row lock in repo.ResolveReport.
	// The authoritative status/permission checks are re-evaluated against
	// the locked row inside the builder below, so a race between this read
	// and the lock cannot cause a double-apply.
	rep, err := s.repo.GetReport(ctx, reportID)
	if err != nil {
		return ReportDetail{}, ErrNotFound
	}
	if rep.Status == StatusResolvedActionTaken || rep.Status == StatusResolvedNoAction {
		return s.GetReportDetail(ctx, reportID)
	}

	var status, decision string
	switch in.Decision {
	case "action_taken":
		if !ValidActionTypes[in.ActionType] || in.ActionType == ActionNoAction {
			return ReportDetail{}, ErrInvalidAction
		}
		status, decision = StatusResolvedActionTaken, "action_taken"
	case "no_action":
		status, decision = StatusResolvedNoAction, "no_action"
		in.ActionType = ActionNoAction
	default:
		return ReportDetail{}, ErrInvalidAction
	}

	if !rep.ReportedDeleted {
		target, err := s.users.UserByID(ctx, rep.ReportedUserID)
		if err != nil {
			return ReportDetail{}, err
		}
		// A moderator cannot act against an admin unless the acting account is
		// also an admin.
		if target.IsAdmin && !actorRole.IsAdmin {
			return ReportDetail{}, ErrModerationForbidden
		}
	} else if actionRequiresLiveTarget(in.ActionType) {
		return ReportDetail{}, ErrNotFound
	}

	build := func(ctx context.Context, current Report) (ResolveWrite, error) {
		// Re-check against the locked row: the target may have been deleted
		// between the initial read above and repository row-lock acquisition.
		if current.ReportedDeleted && actionRequiresLiveTarget(in.ActionType) {
			return ResolveWrite{}, ErrNotFound
		}
		now := s.now()
		updated := current
		updated.Status = status
		updated.Decision = decision
		updated.ModeratorNotes = strings.TrimSpace(in.ModeratorNotes)
		updated.ReviewedAt = &now
		reviewer := moderatorID
		updated.ReviewerID = &reviewer

		var expiresAt, suspendUntil *time.Time
		var notification *Notification
		switch in.ActionType {
		case ActionWarning:
			notification = &Notification{
				ID: uuid.NewString(), RecipientID: current.ReportedUserID, Type: NotifModerationWarning,
				Payload: map[string]string{"reason": in.Reason}, DedupeKey: "moderation:" + current.ID, CreatedAt: now,
			}
		case ActionTemporarySuspension:
			days := in.SuspensionDays
			if days <= 0 {
				days = 3
			}
			until := now.AddDate(0, 0, days)
			expiresAt = &until
			suspendUntil = &until
			notification = &Notification{
				ID: uuid.NewString(), RecipientID: current.ReportedUserID, Type: NotifAccountSuspended,
				Payload:   map[string]string{"reason": in.Reason, "until": until.Format(time.RFC3339)},
				DedupeKey: "moderation:" + current.ID, CreatedAt: now,
			}
		case ActionPermanentBan, ActionContentRemoval, ActionNoAction:
			// No notification for these today.
		}

		action := ModerationAction{
			ID: uuid.NewString(), ModeratorID: moderatorID, TargetUserID: current.ReportedUserID,
			ReportID: &current.ID, ActionType: in.ActionType, Reason: strings.TrimSpace(in.Reason),
			CreatedAt: now, ExpiresAt: expiresAt,
		}

		actionType := in.ActionType
		messageID := current.MessageID
		volatileApply := func(ctx context.Context) error {
			switch actionType {
			case ActionTemporarySuspension:
				return s.users.Suspend(ctx, current.ReportedUserID, suspendUntil, in.Reason)
			case ActionPermanentBan:
				return s.users.Ban(ctx, current.ReportedUserID, in.Reason)
			case ActionContentRemoval:
				if messageID != nil && s.remover != nil {
					return s.remover.RemoveMessage(ctx, moderatorID, *messageID)
				}
			}
			return nil
		}

		return ResolveWrite{
			Report: updated, Action: action, ActionType: actionType, SuspendUntil: suspendUntil,
			Notification: notification, VolatileApply: volatileApply,
		}, nil
	}

	if _, _, err := s.repo.ResolveReport(ctx, reportID, build); err != nil {
		return ReportDetail{}, err
	}
	return s.GetReportDetail(ctx, reportID)
}

func actionRequiresLiveTarget(actionType string) bool {
	switch actionType {
	case ActionWarning, ActionTemporarySuspension, ActionPermanentBan:
		return true
	default:
		return false
	}
}

func (s *Service) notify(ctx context.Context, recipientID, notifType, dedupeKey string, payload map[string]string) {
	_ = s.repo.CreateNotification(ctx, Notification{
		ID: uuid.NewString(), RecipientID: recipientID, Type: notifType,
		Payload: payload, DedupeKey: dedupeKey, CreatedAt: s.now(),
	})
}

// Notify is the exported form used by other packages (e.g. social, for
// new_message notifications) via the moderation.NotificationCreator
// interface they declare.
func (s *Service) Notify(ctx context.Context, recipientID, notifType, dedupeKey string, payload map[string]string) error {
	return s.repo.CreateNotification(ctx, Notification{
		ID: uuid.NewString(), RecipientID: recipientID, Type: notifType,
		Payload: payload, DedupeKey: dedupeKey, CreatedAt: s.now(),
	})
}

func (s *Service) Notifications(ctx context.Context, recipientID string) (NotificationsResponse, error) {
	items, err := s.repo.ListNotifications(ctx, recipientID, 50)
	if err != nil {
		return NotificationsResponse{}, err
	}
	out := make([]NotificationDTO, 0, len(items))
	for _, n := range items {
		out = append(out, NotificationDTO{
			ID: n.ID, Type: n.Type, Payload: n.Payload, Read: n.ReadAt != nil, CreatedAt: n.CreatedAt,
		})
	}
	return NotificationsResponse{Notifications: out}, nil
}

func (s *Service) UnreadNotificationCount(ctx context.Context, recipientID string) (UnreadCountResponse, error) {
	count, err := s.repo.UnreadNotificationCount(ctx, recipientID)
	if err != nil {
		return UnreadCountResponse{}, err
	}
	return UnreadCountResponse{Count: count}, nil
}

func (s *Service) MarkNotificationRead(ctx context.Context, recipientID, notificationID string) error {
	return s.repo.MarkNotificationRead(ctx, recipientID, notificationID)
}

func (s *Service) MarkAllNotificationsRead(ctx context.Context, recipientID string) error {
	return s.repo.MarkAllNotificationsRead(ctx, recipientID)
}

// deletedAccountLabel is rendered in moderator-facing API responses in place
// of a user id whose live account FK has been nulled out (see
// migration 0028_moderation_retention.sql). The report/action row itself,
// and its evidence, are retained regardless.
const deletedAccountLabel = "Deleted account"

func displayUserID(id string, deleted bool) string {
	if deleted {
		return deletedAccountLabel
	}
	return id
}

func summaryFromReport(rep Report) ReportSummary {
	return ReportSummary{
		ID:             rep.ID,
		ReporterUserID: displayUserID(rep.ReporterUserID, rep.ReporterDeleted),
		ReportedUserID: displayUserID(rep.ReportedUserID, rep.ReportedDeleted),
		MessageID:      rep.MessageID, Category: rep.Category, Status: rep.Status, CreatedAt: rep.CreatedAt,
	}
}
