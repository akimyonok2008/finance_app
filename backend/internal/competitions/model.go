package competitions

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// Competition types and statuses.
const (
	TypeWeeklySprint = "weekly_sprint"

	StatusUpcoming  = "upcoming"
	StatusActive    = "active"
	StatusCompleted = "completed"
)

// Competition is one competition EDITION: a scheduled occurrence of a
// competition definition (see Definition/DefinitionVersion), or a legacy
// weekly-sprint row from before the engine existed.
//
// Legacy rows (LifecycleStatus == LifecycleLegacy) have empty edition fields
// and keep deriving Status at read time. Engine editions carry a stored,
// transition-validated lifecycle plus an immutable RulesSnapshotJSON copied
// from their definition version at creation — the single payload the engine
// interprets for this edition forever, regardless of later definition edits.
type Competition struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`

	// Edition fields (empty/nil on legacy rows). Never serialized directly —
	// public shapes are the explicit DTOs below.
	DefinitionID      string          `json:"-"`
	DefinitionVersion int64           `json:"-"`
	JoinOpensAt       *time.Time      `json:"-"`
	JoinClosesAt      *time.Time      `json:"-"`
	LifecycleStatus   string          `json:"-"`
	RulesSnapshotJSON json.RawMessage `json:"-"`
	ScoringScope      string          `json:"-"`
	PublishedAt       *time.Time      `json:"-"`
	FinalizedAt       *time.Time      `json:"-"`
	CancelledAt       *time.Time      `json:"-"`
}

// IsLegacy reports whether this row predates the competition engine (or was
// created by the legacy weekly-sprint path): status derived at read time, no
// stored lifecycle, join-time baseline.
func (c Competition) IsLegacy() bool {
	return c.LifecycleStatus == "" || c.LifecycleStatus == LifecycleLegacy
}

// CompetitionEntry records a user's participation. StartingValue (the private
// base-currency portfolio value captured at join) and Snapshots are NEVER
// exposed by any API. Sprint performance is computed from Snapshots so editing
// the live portfolio after joining cannot change sprint composition.
type CompetitionEntry struct {
	ID            string
	CompetitionID string
	UserID        string
	StartingValue money.Amount // sum of snapshot StartingValueBase (base currency)
	StartingIndex money.IndexValue
	JoinedAt      time.Time
	Snapshots     []CompetitionEntrySnapshotPosition

	// Engine lifecycle fields (zero on legacy entries, whose EntryStatus is
	// EntryLegacyActive). EligibilityEvidenceJSON is the structured rule
	// evidence captured when the user joined — the server-computed record the
	// entry was admitted under; it is never recomputed from later state.
	EntryStatus             string
	PortfolioVersion        int64
	SnapshotCapturedAt      *time.Time
	EligibilityEvidenceJSON json.RawMessage
	ScoringScope            string
	// EligibleStartingValueBase is the scored sleeve's official baseline
	// value, written by the common start-time baseline (provisionally the
	// join-time value until then). Internal only.
	EligibleStartingValueBase money.Amount
	BaselineStatus            string
	BaselineCompletedAt       *time.Time
	DisqualificationReason    string
	WithdrawnAt               *time.Time
	CashSnapshots             []CompetitionEntrySnapshotCash
}

// CompetitionEntrySnapshotCash is a frozen cash balance captured at join time
// for competitions whose scoring configuration includes cash. Internal only —
// never serialized to clients.
type CompetitionEntrySnapshotCash struct {
	ID                 string
	CompetitionEntryID string
	Currency           string
	Amount             money.Amount
	ValueBase          money.Amount
	IncludedInScore    bool
}

// CompetitionEntrySnapshotPosition is a frozen copy of a position at join time.
// It is internal only and never serialized to clients.
type CompetitionEntrySnapshotPosition struct {
	ID                    string
	CompetitionEntryID    string
	Symbol                string
	AssetType             string
	Quantity              money.Quantity
	Currency              string
	StartingPrice         money.Price
	StartingPriceCurrency string
	StartingValueBase     money.Amount

	// Engine identity/classification fields (empty on legacy snapshots).
	// InstrumentID references the stable identity layer WITHOUT a foreign
	// key: ClassificationSnapshotJSON is the frozen evidence this row was
	// admitted and scored under, immune to later metadata changes.
	InstrumentID               string
	VenueMIC                   string
	ClassificationSnapshotJSON json.RawMessage
	IncludedInScore            bool
	StartingWeight             money.Ratio
	BaselinePriceObservedAt    *time.Time
	BaselineFXObservedAt       *time.Time
}

// --- public DTOs (the only shapes ever serialized) ---------------------------

// CompetitionResponse is the public projection of a Competition (no CreatedAt).
type CompetitionResponse struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Status   string    `json:"status"`
}

// JoinCompetitionResponse confirms a join. It returns starting_index (always
// 100) but never the private starting value.
type JoinCompetitionResponse struct {
	CompetitionID string           `json:"competition_id"`
	Joined        bool             `json:"joined"`
	StartingIndex money.IndexValue `json:"starting_index"`
}

// MyCompetitionStatusResponse is the requesting user's own sprint status.
// EntryStatus and ValuedAt are populated only for engine editions (empty/nil
// on legacy sprint responses, and omitted from their JSON).
type MyCompetitionStatusResponse struct {
	CompetitionID          string           `json:"competition_id"`
	Joined                 bool             `json:"joined"`
	EntryStatus            string           `json:"entry_status,omitempty"`
	CurrentRank            int              `json:"current_rank"`
	SprintReturnPercentage money.Ratio      `json:"sprint_return_percentage"`
	SprintIndex            money.IndexValue `json:"sprint_index"`
	ValuedAt               *time.Time       `json:"valued_at,omitempty"`
}

// CompetitionLeaderboardEntry is one privacy-safe row of an engine edition's
// ranking projection: rank, display identity, and percentage/index only —
// never absolute portfolio values.
type CompetitionLeaderboardEntry struct {
	Rank             int              `json:"rank"`
	DisplayName      string           `json:"display_name"`
	AvatarKey        string           `json:"avatar_key"`
	ReturnPercentage money.Ratio      `json:"return_percentage"`
	CompetitionIndex money.IndexValue `json:"competition_index"`
}

// CompetitionLeaderboardPage is a cursor-paginated read from the active
// ranking generation. Unavailable=true means no generation has ever been
// promoted yet (a controlled "not ready" response, never a live O(N) scan —
// see Service.EditionLeaderboard).
type CompetitionLeaderboardPage struct {
	Entries     []CompetitionLeaderboardEntry `json:"entries"`
	NextCursor  *int                          `json:"next_cursor,omitempty"`
	ValuedAt    *time.Time                    `json:"valued_at,omitempty"`
	Unavailable bool                          `json:"unavailable,omitempty"`
}

// CompetitionResultEntry is one immutable final-result row: privacy-safe,
// identical shape to a leaderboard row, but sourced only from
// competition_results — never repriced with current market data.
type CompetitionResultEntry struct {
	Rank             int              `json:"rank"`
	DisplayName      string           `json:"display_name"`
	AvatarKey        string           `json:"avatar_key"`
	ReturnPercentage money.Ratio      `json:"return_percentage"`
	CompetitionIndex money.IndexValue `json:"competition_index"`
}

// SprintLeaderboardEntry is the privacy-safe sprint ranking row.
type SprintLeaderboardEntry struct {
	Rank                   int              `json:"rank"`
	DisplayName            string           `json:"display_name"`
	AvatarKey              string           `json:"avatar_key"`
	SprintReturnPercentage money.Ratio      `json:"sprint_return_percentage"`
	SprintIndex            money.IndexValue `json:"sprint_index"`
}

// weekStart returns Monday 00:00 UTC of the ISO week containing now.
func weekStart(now time.Time) time.Time {
	t := now.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	wd := int(day.Weekday()) // Sunday=0 .. Saturday=6
	if wd == 0 {
		wd = 7 // make Monday=1 .. Sunday=7
	}
	return day.AddDate(0, 0, -(wd - 1))
}

// WeeklySprintID returns the ISO-week-based id for the sprint containing now,
// e.g. "weekly_2026_24".
func WeeklySprintID(now time.Time) string {
	year, week := weekStart(now).ISOWeek()
	return fmt.Sprintf("weekly_%d_%02d", year, week)
}

// deriveStatus computes a competition's status relative to now.
func deriveStatus(startsAt, endsAt, now time.Time) string {
	switch {
	case now.Before(startsAt):
		return StatusUpcoming
	case now.Before(endsAt):
		return StatusActive
	default:
		return StatusCompleted
	}
}

// WeeklySprint returns the weekly sprint covering now: Monday 00:00 UTC to the
// next Monday 00:00 UTC, with status derived from now.
func WeeklySprint(now time.Time) Competition {
	start := weekStart(now)
	end := start.AddDate(0, 0, 7)
	return Competition{
		ID:        WeeklySprintID(now),
		Name:      "Weekly Sprint",
		Type:      TypeWeeklySprint,
		StartsAt:  start,
		EndsAt:    end,
		Status:    deriveStatus(start, end, now.UTC()),
		CreatedAt: now.UTC(),
	}
}

func toCompetitionResponse(c Competition) CompetitionResponse {
	return CompetitionResponse{
		ID: c.ID, Name: c.Name, Type: c.Type,
		StartsAt: c.StartsAt, EndsAt: c.EndsAt, Status: c.Status,
	}
}
