package competitions

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// PositionBaseline is the official start-time observation for one included
// snapshot position, written by CompleteBaseline.
type PositionBaseline struct {
	SnapshotID    string
	Symbol        string
	Quantity      money.Quantity
	Price         money.Price
	PriceCurrency string
	ValueBase     money.Amount
	Weight        money.Ratio
	ObservedAt    time.Time
}

// EngineEntryRepository is the engine-side persistence surface for entries
// and scheduled editions. Implemented by both PostgresCompetitionRepository
// and InMemoryCompetitionRepository (the latter for unit tests and memory
// mode).
type EngineEntryRepository interface {
	// CreateEngineEntry atomically persists the entry with its position and
	// cash snapshots, evidence, and lifecycle fields. ErrEntryExists when the
	// (competition, user) entry already exists — the caller treats that as an
	// idempotent replay.
	CreateEngineEntry(ctx context.Context, entry CompetitionEntry) error
	// UpdateEntryStatus applies a guarded entry-state change (used for
	// withdrawal). ErrEntryConflict when the entry was not in `from`.
	UpdateEntryStatus(ctx context.Context, entryID, from, to string, now time.Time) error
	// ListEditionsByLifecycle returns engine editions currently in the given
	// lifecycle state (never legacy rows).
	ListEditionsByLifecycle(ctx context.Context, lifecycle string) ([]Competition, error)
	// ListBaselineDueEditions returns engine editions whose starts_at has
	// passed and whose lifecycle is registration_closed or active — the set
	// the baseline worker sweeps.
	ListBaselineDueEditions(ctx context.Context, now time.Time) ([]Competition, error)
	// ListPendingBaselineEntries returns up to limit admitted entries still
	// awaiting their baseline for one competition.
	ListPendingBaselineEntries(ctx context.Context, competitionID string, limit int) ([]CompetitionEntry, error)
	// HasPendingBaselineEntries reports whether any admitted entry for this
	// competition has not yet reached a terminal baseline state (active,
	// baseline_failed, withdrawn, or disqualified). Baseline runs in bounded
	// batches (see defaultBaselineBatch), so a competition can have entries
	// still awaiting their first baseline pass well after ranking refresh
	// starts running for it — this is the coverage check that keeps a ranking
	// generation from being promoted off a partial population (see
	// refreshEditionRanking).
	HasPendingBaselineEntries(ctx context.Context, competitionID string) (bool, error)
	// CompleteBaseline atomically writes the official baseline: the entry's
	// eligible starting value, per-position official prices/values/weights,
	// baseline completion, and entry activation. Guarded on the entry still
	// being admitted+pending (idempotent across worker replicas).
	CompleteBaseline(ctx context.Context, entryID string, eligibleStartingValue money.Amount, baselines []PositionBaseline, now time.Time) error
	// FailBaseline marks an entry explicitly failed with auditable evidence.
	FailBaseline(ctx context.Context, entryID, reason string) error
}

// --- in-memory implementation -------------------------------------------------

// CreateEdition stores an engine edition (unit tests / memory mode).
func (r *InMemoryCompetitionRepository) CreateEdition(_ context.Context, e Competition) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.competitions[e.ID]; exists {
		return ErrEditionExists
	}
	r.compOrder = append(r.compOrder, e.ID)
	r.competitions[e.ID] = e
	return nil
}

// TransitionLifecycle mirrors the Postgres guarded transition.
func (r *InMemoryCompetitionRepository) TransitionLifecycle(_ context.Context, competitionID, from, to string, now time.Time) error {
	if err := ValidateLifecycleTransition(from, to); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.competitions[competitionID]
	if !ok {
		return ErrCompetitionNotFound
	}
	if c.LifecycleStatus != from {
		return ErrLifecycleConflict
	}
	c.LifecycleStatus = to
	stamp := now.UTC()
	switch to {
	case LifecyclePublished:
		c.PublishedAt = &stamp
	case LifecycleCompleted:
		c.FinalizedAt = &stamp
	case LifecycleCancelled:
		c.CancelledAt = &stamp
	}
	r.competitions[competitionID] = c
	return nil
}

var (
	_ EditionRepository     = (*InMemoryCompetitionRepository)(nil)
	_ EngineEntryRepository = (*InMemoryCompetitionRepository)(nil)
)

func (r *InMemoryCompetitionRepository) CreateEngineEntry(_ context.Context, entry CompetitionEntry) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries[entry.CompetitionID] == nil {
		r.entries[entry.CompetitionID] = make(map[string]CompetitionEntry)
	}
	if _, exists := r.entries[entry.CompetitionID][entry.UserID]; exists {
		return ErrEntryExists
	}
	r.entries[entry.CompetitionID][entry.UserID] = entry
	return nil
}

func (r *InMemoryCompetitionRepository) UpdateEntryStatus(_ context.Context, entryID, from, to string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for compID, byUser := range r.entries {
		for userID, e := range byUser {
			if e.ID != entryID {
				continue
			}
			if e.EntryStatus != from {
				return ErrEntryConflict
			}
			e.EntryStatus = to
			if to == EntryWithdrawn {
				t := now
				e.WithdrawnAt = &t
			}
			r.entries[compID][userID] = e
			return nil
		}
	}
	return ErrEntryNotFound
}

func (r *InMemoryCompetitionRepository) ListEditionsByLifecycle(_ context.Context, lifecycle string) ([]Competition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Competition
	for _, id := range r.compOrder {
		c := r.competitions[id]
		if !c.IsLegacy() && c.LifecycleStatus == lifecycle {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *InMemoryCompetitionRepository) ListBaselineDueEditions(_ context.Context, now time.Time) ([]Competition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []Competition
	for _, id := range r.compOrder {
		c := r.competitions[id]
		if c.IsLegacy() || c.StartsAt.After(now) {
			continue
		}
		if c.LifecycleStatus == LifecycleRegistrationClosed || c.LifecycleStatus == LifecycleActive {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *InMemoryCompetitionRepository) ListPendingBaselineEntries(_ context.Context, competitionID string, limit int) ([]CompetitionEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []CompetitionEntry
	for _, e := range r.entries[competitionID] {
		if e.EntryStatus == EntryAdmitted && e.BaselineStatus == BaselinePending {
			out = append(out, e)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}

func (r *InMemoryCompetitionRepository) HasPendingBaselineEntries(_ context.Context, competitionID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries[competitionID] {
		if e.EntryStatus == EntryAdmitted {
			return true, nil
		}
	}
	return false, nil
}

func (r *InMemoryCompetitionRepository) CompleteBaseline(_ context.Context, entryID string, eligibleStartingValue money.Amount, baselines []PositionBaseline, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	byID := map[string]PositionBaseline{}
	for _, b := range baselines {
		byID[b.SnapshotID] = b
	}
	for compID, byUser := range r.entries {
		for userID, e := range byUser {
			if e.ID != entryID {
				continue
			}
			if e.EntryStatus != EntryAdmitted || e.BaselineStatus != BaselinePending {
				return ErrEntryConflict
			}
			e.EntryStatus = EntryActive
			e.BaselineStatus = BaselineCompleted
			t := now
			e.BaselineCompletedAt = &t
			e.EligibleStartingValueBase = eligibleStartingValue
			e.StartingValue = eligibleStartingValue
			for i := range e.Snapshots {
				if b, ok := byID[e.Snapshots[i].ID]; ok {
					e.Snapshots[i].Symbol = b.Symbol
					e.Snapshots[i].Quantity = b.Quantity
					e.Snapshots[i].StartingPrice = b.Price
					e.Snapshots[i].StartingPriceCurrency = b.PriceCurrency
					e.Snapshots[i].StartingValueBase = b.ValueBase
					e.Snapshots[i].StartingWeight = b.Weight
					observed := b.ObservedAt
					e.Snapshots[i].BaselinePriceObservedAt = &observed
				}
			}
			r.entries[compID][userID] = e
			return nil
		}
	}
	return ErrEntryNotFound
}

func (r *InMemoryCompetitionRepository) FailBaseline(_ context.Context, entryID, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for compID, byUser := range r.entries {
		for userID, e := range byUser {
			if e.ID != entryID {
				continue
			}
			if e.EntryStatus != EntryAdmitted || e.BaselineStatus != BaselinePending {
				return ErrEntryConflict
			}
			e.EntryStatus = EntryBaselineFailed
			e.BaselineStatus = BaselineFailed
			e.DisqualificationReason = reason
			r.entries[compID][userID] = e
			return nil
		}
	}
	return ErrEntryNotFound
}
