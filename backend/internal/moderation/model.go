package moderation

import "time"

// Report categories.
const (
	CategoryHarassment       = "harassment"
	CategorySpam             = "spam"
	CategoryImpersonation    = "impersonation"
	CategoryFraudOrScam      = "fraud_or_scam"
	CategoryHateOrAbuse      = "hate_or_abuse"
	CategorySexualContent    = "sexual_content"
	CategoryThreats          = "threats"
	CategoryPrivacyViolation = "privacy_violation"
	CategoryOther            = "other"
)

var ValidCategories = map[string]bool{
	CategoryHarassment: true, CategorySpam: true, CategoryImpersonation: true,
	CategoryFraudOrScam: true, CategoryHateOrAbuse: true, CategorySexualContent: true,
	CategoryThreats: true, CategoryPrivacyViolation: true, CategoryOther: true,
}

// Report statuses.
const (
	StatusOpen                = "open"
	StatusUnderReview         = "under_review"
	StatusResolvedActionTaken = "resolved_action_taken"
	StatusResolvedNoAction    = "resolved_no_action"
)

// Report is the persisted report record. The reporter_user_id/reported_user_id
// FK columns are nullable at the DB layer: a future hard account deletion
// sets the FK to NULL (see migration 0028_moderation_retention.sql) rather
// than cascading the report away. Report surfaces that as ReporterUserID/
// ReportedUserID == "" (a UUID is never empty, so this is an unambiguous
// sentinel) rather than as a pointer, to avoid touching every call site that
// already treats these as plain strings; ReporterDeleted/ReportedDeleted
// spell the same fact out explicitly for callers that want it.
// reporterSubjectID/reportedSubjectID are opaque, non-reversible per-user
// ids assigned by that same migration's moderation_subject_ids table; they
// let a moderator recognize "the same reporter/target across multiple
// reports" even after the live user_id has been nulled out. They are
// deliberately unexported so no JSON encoding of Report (there is none
// today, but this guards against one being added later) can ever leak them.
type Report struct {
	ID                string
	ReporterUserID    string
	ReportedUserID    string
	ReporterDeleted   bool
	ReportedDeleted   bool
	reporterSubjectID string
	reportedSubjectID string
	MessageID         *string
	Category          string
	Explanation       string
	Status            string
	CreatedAt         time.Time
	ReviewedAt        *time.Time
	ReviewerID        *string
	Decision          string
	ModeratorNotes    string
}

// Evidence is an immutable snapshot captured at report-creation time. It
// remains available to moderators regardless of later hides/removals/account
// deletion, and is never exposed through ordinary messaging endpoints.
type Evidence struct {
	ID               string
	ReportID         string
	MessageID        *string
	ConversationID   *string
	SenderID         *string
	ParticipantIDs   []string
	MessageText      *string
	MessageCreatedAt *time.Time
	ReportCreatedAt  time.Time
}

// Moderation action types.
const (
	ActionNoAction            = "no_action"
	ActionWarning             = "warning"
	ActionTemporarySuspension = "temporary_suspension"
	ActionPermanentBan        = "permanent_ban"
	ActionContentRemoval      = "content_removal"
)

var ValidActionTypes = map[string]bool{
	ActionNoAction: true, ActionWarning: true, ActionTemporarySuspension: true,
	ActionPermanentBan: true, ActionContentRemoval: true,
}

// ModerationAction is an append-only audit record. Never overwritten.
// ModeratorID/TargetUserID follow the same "" == deleted-account sentinel as
// Report (see its comment); moderatorSubjectID/targetSubjectID are the
// opaque per-user ids from moderation_subject_ids, unexported so they can
// never leak through a JSON encoding of this type.
type ModerationAction struct {
	ID                 string
	ModeratorID        string
	TargetUserID       string
	ModeratorDeleted   bool
	TargetDeleted      bool
	moderatorSubjectID string
	targetSubjectID    string
	ReportID           *string
	ActionType         string
	Reason             string
	CreatedAt          time.Time
	ExpiresAt          *time.Time
}

// --- request/response DTOs -------------------------------------------------

type ReportInput struct {
	Category    string `json:"category"`
	Explanation string `json:"explanation"`
}

type ReportResponse struct {
	ReportID string `json:"report_id"`
	Status   string `json:"status"`
}

// ReportSummary is the moderator-facing list-row shape. It intentionally
// omits moderator_notes/decision detail (see ReportDetail) but does include
// the reporter id — moderators need it, ordinary users never see this shape.
type ReportSummary struct {
	ID             string    `json:"id"`
	ReporterUserID string    `json:"reporter_user_id"`
	ReportedUserID string    `json:"reported_user_id"`
	MessageID      *string   `json:"message_id,omitempty"`
	Category       string    `json:"category"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type ReportListResponse struct {
	Reports []ReportSummary `json:"reports"`
}

type EvidenceDTO struct {
	MessageText      string     `json:"message_text,omitempty"`
	MessageCreatedAt *time.Time `json:"message_created_at,omitempty"`
	ConversationID   string     `json:"conversation_id,omitempty"`
	SenderID         string     `json:"sender_id,omitempty"`
}

type ReportDetail struct {
	ReportSummary
	Explanation    string       `json:"explanation"`
	ReviewedAt     *time.Time   `json:"reviewed_at,omitempty"`
	ReviewerID     *string      `json:"reviewer_id,omitempty"`
	Decision       string       `json:"decision,omitempty"`
	ModeratorNotes string       `json:"moderator_notes,omitempty"`
	Evidence       *EvidenceDTO `json:"evidence,omitempty"`
}

// ResolveInput is the moderator resolution request body. Decision must be
// "action_taken" or "no_action"; ActionType is required when Decision is
// "action_taken". SuspensionDays is required (and only used) for
// action_type=temporary_suspension.
type ResolveInput struct {
	Decision       string `json:"decision"`
	ActionType     string `json:"action_type,omitempty"`
	Reason         string `json:"reason,omitempty"`
	ModeratorNotes string `json:"moderator_notes,omitempty"`
	SuspensionDays int    `json:"suspension_days,omitempty"`
}

// --- notifications ----------------------------------------------------------

const (
	NotifFriendRequest         = "friend_request"
	NotifFriendRequestAccepted = "friend_request_accepted"
	NotifMessageRequest        = "message_request"
	NotifNewMessage            = "new_message"
	NotifModerationWarning     = "moderation_warning"
	NotifAccountSuspended      = "account_suspended"
)

type Notification struct {
	ID          string
	RecipientID string
	Type        string
	Payload     map[string]string
	DedupeKey   string
	ReadAt      *time.Time
	CreatedAt   time.Time
}

type NotificationDTO struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Payload   map[string]string `json:"payload"`
	Read      bool              `json:"read"`
	CreatedAt time.Time         `json:"created_at"`
}

type NotificationsResponse struct {
	Notifications []NotificationDTO `json:"notifications"`
}

type UnreadCountResponse struct {
	Count int `json:"count"`
}
