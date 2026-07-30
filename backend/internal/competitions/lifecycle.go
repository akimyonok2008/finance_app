package competitions

import "fmt"

// Lifecycle states for competition editions. Unlike the legacy sprint's
// derive-at-read status, these are STORED and only change through validated
// transitions (see ValidateLifecycleTransition and the repository's guarded
// TransitionLifecycle), so an edition's history is explicit and auditable.
//
// LifecycleLegacy is the sentinel carried by pre-engine weekly-sprint rows
// (and the rows the legacy EnsureCurrentSprint still creates until cutover):
// those rows keep deriving upcoming/active/completed at read time and are
// invisible to the new engine's transition machinery.
const (
	LifecycleLegacy             = "legacy"
	LifecycleDraft              = "draft"
	LifecyclePublished          = "published"
	LifecycleRegistrationOpen   = "registration_open"
	LifecycleRegistrationClosed = "registration_closed"
	LifecycleActive             = "active"
	LifecycleFinalizing         = "finalizing"
	LifecycleCompleted          = "completed"
	LifecycleCancelled          = "cancelled"
)

// Entry lifecycle states (competition_entries.entry_status).
const (
	EntryLegacyActive   = "legacy_active"   // pre-engine sprint entry
	EntryAdmitted       = "admitted"        // joined in the window, awaiting common baseline
	EntryActive         = "active"          // baselined, being ranked
	EntryWithdrawn      = "withdrawn"       // left before join_closes_at
	EntryBaselineFailed = "baseline_failed" // no baseline within the operational window
	EntryDisqualified   = "disqualified"
	EntryFinalized      = "finalized"
)

// Baseline states (competition_entries.baseline_status).
const (
	BaselinePending   = "pending"
	BaselineCompleted = "completed"
	BaselineFailed    = "failed"
)

// lifecycleTransitions is the complete set of allowed edition transitions.
// Anything not listed is invalid — in particular, completed and cancelled are
// terminal (a completed competition can never return to active, and its
// results can never be rebuilt), and no state may re-enter draft, so rules
// snapshots can never be silently re-edited after publication.
var lifecycleTransitions = map[string][]string{
	LifecycleDraft:              {LifecyclePublished, LifecycleCancelled},
	LifecyclePublished:          {LifecycleRegistrationOpen, LifecycleCancelled},
	LifecycleRegistrationOpen:   {LifecycleRegistrationClosed, LifecycleCancelled},
	LifecycleRegistrationClosed: {LifecycleActive, LifecycleCancelled},
	LifecycleActive:             {LifecycleFinalizing, LifecycleCancelled},
	LifecycleFinalizing:         {LifecycleCompleted, LifecycleCancelled},
	LifecycleCompleted:          {},
	LifecycleCancelled:          {},
}

// ValidateLifecycleTransition reports whether from → to is an allowed edition
// transition. The legacy sentinel participates in no transitions: legacy rows
// are managed only by the legacy derive-at-read path.
func ValidateLifecycleTransition(from, to string) error {
	allowed, known := lifecycleTransitions[from]
	if !known {
		return fmt.Errorf("%w: unknown lifecycle status %q", ErrInvalidLifecycleTransition, from)
	}
	for _, next := range allowed {
		if next == to {
			return nil
		}
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidLifecycleTransition, from, to)
}
