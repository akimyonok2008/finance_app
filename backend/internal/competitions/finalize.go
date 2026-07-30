package competitions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// defaultFinalizationWindow bounds how long past ends_at the finalizer keeps
// retrying entries whose end-time valuation fails before disqualifying them
// (with auditable evidence) and finalizing without them. Mirrors
// defaultBaselineWindow's fail-closed-then-explicit-resolution shape.
const defaultFinalizationWindow = 2 * time.Hour

// SetFinalizationWindow overrides the operational window (d <= 0 ignored).
func (s *Service) SetFinalizationWindow(d time.Duration) {
	if d > 0 {
		s.finalizationWindow = d
	}
}

// FinalizationAchievementTrigger is the optional idempotent trophy hook fired
// once per finalized participant. Implemented by an adapter over the
// achievements package; a nil trigger simply skips it.
type FinalizationAchievementTrigger interface {
	EvaluateCompetitionFinalizationAchievements(ctx context.Context, userID, competitionID string) error
}

// CompetitionResultRow is one immutable final-result row, computed once at
// finalization and never recomputed. Rank is materialized here (sequential,
// same tie-break chain as ranking generations — see PromoteGeneration's doc)
// rather than derived at read time.
type CompetitionResultRow struct {
	EntryID      string
	UserID       string
	Rank         int
	Index        money.IndexValue
	ReturnPct    money.Ratio
	EvidenceJSON json.RawMessage
}

// FinalizationRepository is the engine-side persistence boundary for
// completing an edition (table competition_results, migration 0048).
type FinalizationRepository interface {
	// ListActiveEntriesForFinalization returns every EntryActive entry (with
	// snapshots) for a competition — the population the finalizer values.
	// Withdrawn, never-baselined, and already-disqualified entries are never
	// scored into final results.
	ListActiveEntriesForFinalization(ctx context.Context, competitionID string) ([]CompetitionEntry, error)
	// DisqualifyEntry guardedly moves an EntryActive entry to disqualified
	// with an auditable reason — the explicit resolution the finalizer applies
	// to entries that never obtained a valid end-time valuation within the
	// operational window.
	DisqualifyEntry(ctx context.Context, entryID, reason string, now time.Time) error
	// FinalizeResults atomically writes the immutable result rows and
	// transitions the edition finalizing -> completed in one transaction.
	// Idempotent: if the edition is already completed with results already
	// present, this is a no-op success (safe to call again after a partial
	// failure or a worker restart) rather than an error.
	FinalizeResults(ctx context.Context, competitionID string, results []CompetitionResultRow, now time.Time) error
	// ListResults reads the immutable final results, ordered by rank. Returns
	// an empty slice (not an error) for a completed competition with zero
	// scored participants.
	ListResults(ctx context.Context, competitionID string) ([]CompetitionResultEntry, error)
}

// FinalizeCompetitions transitions every edition in LifecycleFinalizing to
// LifecycleCompleted:
//
//  1. Value every EntryActive entry at end-time observations, shared across
//     the whole pass (one observationMemo) so every participant is priced
//     from identical observations — the "authoritative end-time price and FX
//     observations" policy: the first pass whose valuations all succeed (or
//     whose failures age out of the operational window) becomes final.
//  2. An entry that fails valuation is retried on the next call while still
//     inside defaultFinalizationWindow of ends_at; past that window it is
//     explicitly disqualified (auditable reason) and finalization proceeds
//     without it — never a silent omission.
//  3. Sequential final ranks are materialized once, atomically, alongside the
//     lifecycle transition — results are then immutable: nothing after this
//     point ever reprices them with current market data.
//  4. The idempotent achievement hook fires once per finalized participant,
//     best-effort (a failure there never blocks or unwinds finalization).
func (s *Service) FinalizeCompetitions(ctx context.Context) (int, error) {
	entryRepo, ok := s.repo.(EngineEntryRepository)
	finalRepo, ok2 := s.repo.(FinalizationRepository)
	if !ok || !ok2 {
		return 0, nil
	}
	editions, err := entryRepo.ListEditionsByLifecycle(ctx, LifecycleFinalizing)
	if err != nil {
		return 0, fmt.Errorf("competitions: list finalizing editions: %w", err)
	}
	window := s.finalizationWindow
	if window <= 0 {
		window = defaultFinalizationWindow
	}
	now := s.clock.Now().UTC()

	finalized := 0
	for _, comp := range editions {
		ok, err := s.finalizeEdition(ctx, finalRepo, comp, window, now)
		if err != nil {
			slog.Error("competition_finalization_failed", "competition_id", comp.ID, "error", err)
			continue
		}
		if ok {
			finalized++
		}
	}
	return finalized, nil
}

func (s *Service) finalizeEdition(ctx context.Context, repo FinalizationRepository, comp Competition, window time.Duration, now time.Time) (bool, error) {
	entries, err := repo.ListActiveEntriesForFinalization(ctx, comp.ID)
	if err != nil {
		return false, fmt.Errorf("list active entries: %w", err)
	}

	memo := newObservationMemo(s)
	type valuedEntry struct {
		entry    CompetitionEntry
		idx      money.IndexValue
		retPct   money.Ratio
		userName string
	}
	var valued []valuedEntry
	var failed []CompetitionEntry
	for _, entry := range entries {
		idx, retPct, err := s.valueCompetitionEntry(ctx, entry, memo)
		if err != nil {
			failed = append(failed, entry)
			continue
		}
		name := entry.UserID
		if u, err := s.users.GetUserByID(ctx, entry.UserID); err == nil && u != nil {
			name = u.DisplayName
		}
		valued = append(valued, valuedEntry{entry: entry, idx: idx, retPct: retPct, userName: name})
	}

	if len(failed) > 0 {
		if now.Before(comp.EndsAt.Add(window)) {
			slog.Warn("competition_finalization_retry",
				"competition_id", comp.ID, "failed_entries", len(failed))
			return false, nil
		}
		for _, e := range failed {
			reason := fmt.Sprintf("no valid end-time valuation within the %s operational window after ends_at", window)
			if err := repo.DisqualifyEntry(ctx, e.ID, reason, now); err != nil {
				return false, fmt.Errorf("disqualify entry %s: %w", e.ID, err)
			}
		}
		slog.Warn("competition_finalization_disqualified_entries",
			"competition_id", comp.ID, "count", len(failed))
	}

	sort.SliceStable(valued, func(i, j int) bool {
		if c := valued[i].retPct.Cmp(valued[j].retPct); c != 0 {
			return c > 0
		}
		if valued[i].userName != valued[j].userName {
			return valued[i].userName < valued[j].userName
		}
		return valued[i].entry.UserID < valued[j].entry.UserID
	})

	evidence, _ := json.Marshal(map[string]any{
		"valuation_policy":   "shared_observation_pass_at_finalization",
		"disqualified_count": len(failed),
		"finalized_at":       now,
	})
	results := make([]CompetitionResultRow, 0, len(valued))
	for i, v := range valued {
		results = append(results, CompetitionResultRow{
			EntryID: v.entry.ID, UserID: v.entry.UserID, Rank: i + 1,
			Index: v.idx, ReturnPct: v.retPct, EvidenceJSON: evidence,
		})
	}

	if err := repo.FinalizeResults(ctx, comp.ID, results, now); err != nil {
		return false, fmt.Errorf("finalize results: %w", err)
	}

	if s.achievements != nil {
		for _, v := range valued {
			if err := s.achievements.EvaluateCompetitionFinalizationAchievements(ctx, v.entry.UserID, comp.ID); err != nil {
				slog.Warn("competition_finalization_achievement_failed",
					"competition_id", comp.ID, "user_id", v.entry.UserID, "error", err)
			}
		}
	}
	slog.Info("competition_finalized", "competition_id", comp.ID, "results", len(results), "disqualified", len(failed))
	return true, nil
}

// Results returns the immutable final leaderboard for a completed edition.
func (s *Service) Results(ctx context.Context, competitionID string) ([]CompetitionResultEntry, error) {
	comp, err := s.loadCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	if comp.IsLegacy() || comp.LifecycleStatus != LifecycleCompleted {
		return nil, ErrResultsNotAvailable
	}
	repo, ok := s.repo.(FinalizationRepository)
	if !ok {
		return nil, ErrResultsNotAvailable
	}
	return repo.ListResults(ctx, competitionID)
}

// --- in-memory implementation -------------------------------------------------

var _ FinalizationRepository = (*InMemoryCompetitionRepository)(nil)

func (r *InMemoryCompetitionRepository) ListActiveEntriesForFinalization(_ context.Context, competitionID string) ([]CompetitionEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []CompetitionEntry
	for _, e := range r.entries[competitionID] {
		if e.EntryStatus == EntryActive {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *InMemoryCompetitionRepository) DisqualifyEntry(_ context.Context, entryID, reason string, _ time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for compID, byUser := range r.entries {
		for userID, e := range byUser {
			if e.ID != entryID {
				continue
			}
			if e.EntryStatus != EntryActive {
				return ErrEntryConflict
			}
			e.EntryStatus = EntryDisqualified
			e.DisqualificationReason = reason
			r.entries[compID][userID] = e
			return nil
		}
	}
	return ErrEntryNotFound
}

type memoryResults struct {
	mu   sync.Mutex
	rows map[string][]CompetitionResultRow
}

func (r *InMemoryCompetitionRepository) results() *memoryResults {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.resultStore == nil {
		r.resultStore = &memoryResults{rows: map[string][]CompetitionResultRow{}}
	}
	return r.resultStore
}

func (r *InMemoryCompetitionRepository) FinalizeResults(_ context.Context, competitionID string, results []CompetitionResultRow, now time.Time) error {
	store := r.results()
	store.mu.Lock()
	defer store.mu.Unlock()

	r.mu.Lock()
	comp, ok := r.competitions[competitionID]
	if !ok {
		r.mu.Unlock()
		return ErrCompetitionNotFound
	}
	if comp.LifecycleStatus == LifecycleCompleted {
		r.mu.Unlock()
		if _, exists := store.rows[competitionID]; exists {
			return nil // idempotent replay
		}
		return ErrLifecycleConflict
	}
	if comp.LifecycleStatus != LifecycleFinalizing {
		r.mu.Unlock()
		return ErrLifecycleConflict
	}
	comp.LifecycleStatus = LifecycleCompleted
	stamp := now.UTC()
	comp.FinalizedAt = &stamp
	r.competitions[competitionID] = comp
	r.mu.Unlock()

	if _, exists := store.rows[competitionID]; !exists {
		store.rows[competitionID] = results
		// Scored entries become terminally 'finalized' — see the Postgres
		// implementation's identical guard for why this is what makes the
		// result immutable end to end (a finalized entry can no longer be
		// disqualified or otherwise mutated).
		r.mu.Lock()
		for _, res := range results {
			for userID, e := range r.entries[competitionID] {
				if e.ID == res.EntryID && e.EntryStatus == EntryActive {
					e.EntryStatus = EntryFinalized
					r.entries[competitionID][userID] = e
				}
			}
		}
		r.mu.Unlock()
	}
	return nil
}

func (r *InMemoryCompetitionRepository) ListResults(_ context.Context, competitionID string) ([]CompetitionResultEntry, error) {
	store := r.results()
	store.mu.Lock()
	defer store.mu.Unlock()
	rows := store.rows[competitionID]
	out := make([]CompetitionResultEntry, 0, len(rows))
	for _, row := range rows {
		name, _ := r.userDisplayName(row.UserID)
		out = append(out, CompetitionResultEntry{
			Rank: row.Rank, DisplayName: name,
			ReturnPercentage: row.ReturnPct, CompetitionIndex: row.Index,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rank < out[j].Rank })
	return out, nil
}
