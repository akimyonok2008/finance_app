// Package performance is the single trusted source of persistent, chain-linked
// ranked performance. It is deliberately independent of the portfolio domain:
// the portfolio service owns mutable positions and private monetary summaries,
// while this package owns the accumulated ranked index that portfolio mutations
// may never rewrite.
//
// The central invariant: portfolio mutations may alter future exposure, but they
// must never rewrite past ranked performance. Only market-price and FX movements
// while a portfolio is active may change the ranked index. Adding capital,
// increasing/decreasing quantities, deleting positions, replacing the portfolio
// through strategy copying, or emptying and later resuming the portfolio all
// generate exactly zero ranked return at the moment they happen.
package performance

import "time"

// Status is the lifecycle of a portfolio's ranked tracking.
type Status string

const (
	// StatusActive means the portfolio currently holds positions and its ranked
	// index moves with market/FX.
	StatusActive Status = "active"
	// StatusPaused means the portfolio is empty. The last accumulated ranked
	// index is preserved and the user is excluded from active leaderboard ranking
	// until they add a position again.
	StatusPaused Status = "paused"
)

// State is the persistent ranked-performance record for one portfolio. It is
// private: its absolute values (checkpoint_index, segment_start_value_base) are
// never exposed through any public API.
//
//	current_ranked_index = checkpoint_index * current_value_base / segment_start_value_base   (active)
//	                     = checkpoint_index                                                    (paused)
type State struct {
	PortfolioID string
	UserID      string
	// CheckpointIndex is the ranked index captured at the start of the current
	// segment. It always equals the current ranked index immediately after a
	// mutation, so a mutation contributes zero return.
	CheckpointIndex float64
	// SegmentStartValueBase is the base-currency portfolio value at the start of
	// the current segment. Nil when paused (empty portfolio), positive when active.
	SegmentStartValueBase *float64
	Status                Status
	// TrackingStartedAt is the ranking-epoch timestamp: when this portfolio first
	// entered the (new) ranked-tracking model. Legacy snapshots recorded before
	// this instant are ignored by timeframe leaderboards.
	TrackingStartedAt time.Time
	// SegmentStartedAt is when the current segment began (the last mutation, or
	// activation). Nil when never activated.
	SegmentStartedAt *time.Time
	UpdatedAt        time.Time
	// Version is an optimistic-concurrency guard: an update must present the
	// version it read, and the store rejects the write if another writer has
	// advanced it in the meantime.
	Version int64
}

// RankedPerformance is the privacy-safe, response-ready view of a portfolio's
// ranked standing. It carries NO absolute monetary values, quantities, cost
// basis, or internal identifiers — only the index, the return percentage, the
// status, and the ranking epoch.
type RankedPerformance struct {
	RankedIndex            float64   `json:"ranked_index"`
	RankedReturnPercentage float64   `json:"ranked_return_percentage"`
	Status                 Status    `json:"ranking_status"`
	TrackingStartedAt      time.Time `json:"tracking_started_at"`
}

// CheckpointInput describes a single atomic portfolio mutation from the ranked
// engine's point of view. The caller (portfolio service) values the portfolio
// before and after the mutation using ONE consistent price/FX snapshot, so the
// mutation itself contributes no return.
type CheckpointInput struct {
	PortfolioID string
	UserID      string
	// ValueBeforeBase is the base-currency market value of the active positions
	// BEFORE the mutation, at current prices. Ignored when the portfolio was empty
	// (paused) before the mutation.
	ValueBeforeBase float64
	HasActiveBefore bool
	// ValueAfterBase is the base-currency market value of the active positions
	// AFTER the mutation, at the same current prices. Ignored when the portfolio
	// is empty after the mutation.
	ValueAfterBase float64
	HasActiveAfter bool
	At             time.Time
}
