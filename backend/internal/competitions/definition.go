package competitions

import (
	"encoding/json"
	"time"
)

// LegacyWeeklySprintDefinitionID is the fixed id of the "Weekly Open Sprint"
// definition seeded by migration 0045. Pre-engine sprint rows were stamped
// onto its version 1 so their historical semantics stay documented and
// queryable; new Weekly Open Sprint editions attach to the same definition.
const LegacyWeeklySprintDefinitionID = "c0000000-0000-4000-8000-000000000001"

// Definition is a reusable competition template ("Crypto Challenge",
// "ETF Battle"). It never carries rules directly — rules live in immutable
// DefinitionVersions, and each edition snapshots the exact version it runs
// under. CurrentVersion is the latest created version (0 = none yet; such a
// definition cannot back an edition).
type Definition struct {
	ID                     string          `json:"id"`
	Slug                   string          `json:"slug"`
	Name                   string          `json:"name"`
	Description            string          `json:"description"`
	Category               string          `json:"category"`
	IconKey                string          `json:"icon_key"`
	PresentationConfigJSON json.RawMessage `json:"presentation_config"`
	IsEnabled              bool            `json:"is_enabled"`
	CurrentVersion         int64           `json:"current_version"`
	CreatedAt              time.Time       `json:"created_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
}

// DefinitionVersion is one immutable rule set for a definition. The JSON
// payloads are validated documents (see the rules package, Phase 2); numeric
// thresholds inside them are decimal strings, never floats. Versions are
// append-only: the repository exposes no update, and (definition_id, version)
// is the primary key, so an existing version can never be overwritten.
type DefinitionVersion struct {
	DefinitionID         string          `json:"definition_id"`
	Version              int64           `json:"version"`
	EligibilityRulesJSON json.RawMessage `json:"eligibility_rules"`
	ScoringRulesJSON     json.RawMessage `json:"scoring_rules"`
	ScheduleDefaultsJSON json.RawMessage `json:"schedule_defaults"`
	DisplayRulesJSON     json.RawMessage `json:"display_rules"`
	CreatedAt            time.Time       `json:"created_at"`
	CreatedBy            string          `json:"created_by"`
}

// RulesSnapshot is the immutable rule payload stamped onto an edition at
// creation time, combining the eligibility and scoring documents of the exact
// definition version the edition runs under. It is what the engine interprets
// for that edition forever after — later definition versions never apply.
type RulesSnapshot struct {
	SchemaVersion int             `json:"schema_version"`
	Eligibility   json.RawMessage `json:"eligibility"`
	Scoring       json.RawMessage `json:"scoring"`
}

// BuildRulesSnapshot assembles the edition rules snapshot from a definition
// version.
func BuildRulesSnapshot(v DefinitionVersion) (json.RawMessage, error) {
	return json.Marshal(RulesSnapshot{
		SchemaVersion: 1,
		Eligibility:   v.EligibilityRulesJSON,
		Scoring:       v.ScoringRulesJSON,
	})
}
