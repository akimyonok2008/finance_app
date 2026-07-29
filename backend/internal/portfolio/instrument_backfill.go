package portfolio

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ardakimyonok/finance_app/internal/instrument"
	"github.com/ardakimyonok/finance_app/internal/telemetry"
)

// ErrNoInstrumentResolver is returned by RunInstrumentBackfill when no
// resolver was wired via SetInstrumentResolver.
var ErrNoInstrumentResolver = errors.New("portfolio: no instrument resolver configured")

// RunInstrumentBackfill performs one bounded backfill pass using the
// service's own repository and identity resolver. See BackfillJob.
func (s *Service) RunInstrumentBackfill(ctx context.Context, batchLimit int) (BackfillSummary, error) {
	if s.identity == nil {
		return BackfillSummary{}, ErrNoInstrumentResolver
	}
	job := NewBackfillJob(s.repo, s.identity)
	job.BatchLimit = batchLimit
	return job.Run(ctx)
}

// ListPendingIdentityReconciliation returns legacy rows queued for
// administrative review (see BackfillJob).
func (s *Service) ListPendingIdentityReconciliation(ctx context.Context, limit int) ([]ReconciliationItem, error) {
	return s.repo.ListPendingReconciliation(ctx, limit)
}

// ResolveIdentityReconciliation assigns an instrument identity an
// administrator chose to a queued row.
func (s *Service) ResolveIdentityReconciliation(ctx context.Context, id, instrumentID, resolvedBy string) error {
	return s.repo.ResolveReconciliation(ctx, id, instrumentID, resolvedBy)
}

// RejectIdentityReconciliation marks a queued row as reviewed with no
// resolution (e.g. the symbol was a data-entry error with no real
// instrument).
func (s *Service) RejectIdentityReconciliation(ctx context.Context, id, resolvedBy string) error {
	return s.repo.RejectReconciliation(ctx, id, resolvedBy)
}

// BackfillJob resolves legacy positions/activities (instrument_id IS NULL)
// against the LOCAL identity register only — it never calls out to OpenFIGI,
// so it is safe to run repeatedly and cheaply. A record with a single
// unambiguous ticker match in the local register at its transaction date is
// resolved directly; a record with zero or multiple candidates is queued for
// administrative review (see ReconciliationItem) rather than guessed at.
//
// This does not backfill FIGI/ISIN evidence — positions/activities never
// stored those to begin with (see model.go) — so "strong evidence" here
// means "ticker + transaction date resolves to exactly one instrument in the
// register already built by the buy path", which is the best evidence these
// tables can carry.
type BackfillJob struct {
	repo     Repository
	resolver *instrument.Resolver
	// BatchLimit caps how many rows of each kind are processed per Run call,
	// so a single invocation stays bounded on a large legacy dataset. Zero
	// uses the repository's own default.
	BatchLimit int
}

// NewBackfillJob wires a backfill job against repo (for the legacy rows) and
// resolver (for the local-register lookup).
func NewBackfillJob(repo Repository, resolver *instrument.Resolver) *BackfillJob {
	return &BackfillJob{repo: repo, resolver: resolver}
}

// BackfillSummary reports what one Run call did, split by table.
type BackfillSummary struct {
	PositionsResolved  int
	PositionsQueued    int
	PositionsScanned   int
	ActivitiesResolved int
	ActivitiesQueued   int
	ActivitiesScanned  int
}

// Run performs one bounded pass over legacy positions and activities.
// Calling it again continues where the previous pass left off (resolved
// rows no longer match the "instrument_id IS NULL" scan; queued rows are not
// re-queued, per EnqueueIdentityReconciliation's idempotency).
func (j *BackfillJob) Run(ctx context.Context) (BackfillSummary, error) {
	var summary BackfillSummary

	positions, err := j.repo.ListPositionsMissingInstrumentID(ctx, j.BatchLimit)
	if err != nil {
		return summary, fmt.Errorf("portfolio: backfill list positions: %w", err)
	}
	summary.PositionsScanned = len(positions)
	for _, p := range positions {
		resolved, err := j.resolveOne(ctx, "positions", p.ID, p.Symbol, p.CreatedAt)
		if err != nil {
			return summary, err
		}
		if resolved {
			summary.PositionsResolved++
		} else {
			summary.PositionsQueued++
		}
	}

	activities, err := j.repo.ListActivitiesMissingInstrumentID(ctx, j.BatchLimit)
	if err != nil {
		return summary, fmt.Errorf("portfolio: backfill list activities: %w", err)
	}
	summary.ActivitiesScanned = len(activities)
	for _, a := range activities {
		resolved, err := j.resolveOne(ctx, "portfolio_activities", a.ID, a.Symbol, a.OccurredAt)
		if err != nil {
			return summary, err
		}
		if resolved {
			summary.ActivitiesResolved++
		} else {
			summary.ActivitiesQueued++
		}
	}

	return summary, nil
}

// resolveOne resolves a single legacy row, returning resolved=true when it
// applied instrument_id directly, or false when it queued the row for
// review (in which case no error is returned — an unresolved/ambiguous
// legacy row is an expected outcome, not a failure).
func (j *BackfillJob) resolveOne(ctx context.Context, table, recordID, symbol string, asOf time.Time) (bool, error) {
	in, quality, err := j.resolver.ResolveTickerAsOf(ctx, symbol, asOf)
	if err != nil {
		return false, fmt.Errorf("portfolio: backfill resolve %s %s: %w", table, recordID, err)
	}
	switch quality {
	case instrument.QualityResolved:
		var setErr error
		if table == "positions" {
			setErr = j.repo.SetPositionInstrumentID(ctx, recordID, in.ID)
		} else {
			setErr = j.repo.SetActivityInstrumentID(ctx, recordID, in.ID)
		}
		if setErr != nil {
			return false, fmt.Errorf("portfolio: backfill apply %s %s: %w", table, recordID, setErr)
		}
		return true, nil
	case instrument.QualityAmbiguous:
		telemetry.IncInstrumentResolutionAmbiguous()
		return false, j.repo.EnqueueIdentityReconciliation(ctx, ReconciliationItem{
			TableName: table, RecordID: recordID, Symbol: symbol,
			Evidence: "ticker_ambiguous_multiple_local_instruments", Confidence: ReconciliationConfidenceLow,
		})
	default: // QualityUnresolved
		telemetry.IncInstrumentResolutionUnresolved()
		return false, j.repo.EnqueueIdentityReconciliation(ctx, ReconciliationItem{
			TableName: table, RecordID: recordID, Symbol: symbol,
			Evidence: "ticker_no_local_match", Confidence: ReconciliationConfidenceLow,
		})
	}
}
