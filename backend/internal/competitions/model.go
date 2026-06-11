package competitions

import (
	"fmt"
	"time"
)

// Competition types and statuses.
const (
	TypeWeeklySprint = "weekly_sprint"

	StatusUpcoming  = "upcoming"
	StatusActive    = "active"
	StatusCompleted = "completed"
)

// Competition is a time-bound competition. The prototype runs one active weekly
// sprint at a time, derived from the current ISO week.
type Competition struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	StartsAt  time.Time `json:"starts_at"`
	EndsAt    time.Time `json:"ends_at"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// CompetitionEntry records a user's participation. StartingValue (the private
// base-currency portfolio value captured at join) and Snapshots are NEVER
// exposed by any API. Sprint performance is computed from Snapshots so editing
// the live portfolio after joining cannot change sprint composition.
type CompetitionEntry struct {
	ID            string
	CompetitionID string
	UserID        string
	StartingValue float64 // sum of snapshot StartingValueBase (base currency)
	StartingIndex float64
	JoinedAt      time.Time
	Snapshots     []CompetitionEntrySnapshotPosition
}

// CompetitionEntrySnapshotPosition is a frozen copy of a position at join time.
// It is internal only and never serialized to clients.
type CompetitionEntrySnapshotPosition struct {
	ID                    string
	CompetitionEntryID    string
	Symbol                string
	AssetType             string
	Quantity              float64
	Currency              string
	StartingPrice         float64
	StartingPriceCurrency string
	StartingValueBase     float64
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
	CompetitionID string  `json:"competition_id"`
	Joined        bool    `json:"joined"`
	StartingIndex float64 `json:"starting_index"`
}

// MyCompetitionStatusResponse is the requesting user's own sprint status.
type MyCompetitionStatusResponse struct {
	CompetitionID          string  `json:"competition_id"`
	Joined                 bool    `json:"joined"`
	CurrentRank            int     `json:"current_rank"`
	SprintReturnPercentage float64 `json:"sprint_return_percentage"`
	SprintIndex            float64 `json:"sprint_index"`
}

// SprintLeaderboardEntry is the privacy-safe sprint ranking row.
type SprintLeaderboardEntry struct {
	Rank                   int     `json:"rank"`
	DisplayName            string  `json:"display_name"`
	AvatarKey              string  `json:"avatar_key"`
	SprintReturnPercentage float64 `json:"sprint_return_percentage"`
	SprintIndex            float64 `json:"sprint_index"`
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
