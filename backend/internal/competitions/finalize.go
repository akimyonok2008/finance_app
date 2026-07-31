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
// defaultBaselineBatch's fail-closed-then-explicit-resolution shape.
const defaultFinalizationWindow = 2 * time.Hour

// defaultFinalizationBatchSize bounds how many entries one finalizeEdition
// pass claims and values per competition per call — the same fixed-cost-per-
// tick principle as RefreshCompetitionRankings' batching (see ranking.go).
// A large public competition's finalization then spans however many worker
// ticks it needs, never one unbounded pass that loads every participant into
// memory at once.
const defaultFinalizationBatchSize = 500

// SetFinalizationWindow overrides the operational window (d <= 0 ignored).
func (s *Service) SetFinalizationWindow(d time.Duration) {
	if d > 0 {
		s.finalizationWindow = d
	}
}

// SetFinalizationBatchSize overrides how many entries finalizeEdition claims
// per competition per call. n <= 0 restores the default.
func (s *Service) SetFinalizationBatchSize(n int) {
	if n <= 0 {
		n = defaultFinalizationBatchSize
	}
	s.finalizationBatchSize = n
}

// FinalizationAchievementTrigger is the optional idempotent trophy hook fired
// once per finalized participant. Implemented by an adapter over the
// achievements package; a nil trigger simply skips it. Finalization itself no
// longer calls this directly — see competition_outbox /
// CompetitionAchievementProjector, which drives it durably from the
// competition.finalized event PromoteFinalizationGeneration commits.
type FinalizationAchievementTrigger interface {
	EvaluateCompetitionFinalizationAchievements(ctx context.Context, userID, competitionID string) error
}

// FinalizationGeneration is one attempt at finalizing a competition (table
// competition_finalization_generations, migration 0054) — the SAME
// building/promote generation shape RankingGeneration uses (see ranking.go's
// doc comment), adapted for a one-shot, terminal pass instead of a
// continuously-refreshed live board: there is no "active"/"superseded"
// state, because an edition is only ever finalized once. failed_entries
// counts only entries still awaiting a retry WITHIN the finalization window;
// an entry already disqualified past the window is resolved and does not
// block promotion.
type FinalizationGeneration struct {
	CompetitionID     string
	Generation        int64
	Status            string // building | completed | failed
	ExpectedEntries   int
	ProcessedEntries  int
	SuccessfulEntries int
	FailedEntries     int
	CursorEntryID     string
	WriteFailure      bool
	StartedAt         time.Time
	CompletedAt       *time.Time
	FailureReason     string
}

// GenerationCompleted is FinalizationGeneration's terminal-success status.
// GenerationBuilding and GenerationFailed (ranking.go) are reused as-is:
// the string values are identical, and a "building" or "failed" generation
// means the same thing whether it is a ranking refresh or a finalization
// pass.
const GenerationCompleted = "completed"

// CompetitionFinalizationRow is one entry's DRAFT end-time valuation, written
// into a finalization generation's staging rows (competition_finalization_rows)
// before promotion. Mirrors CompetitionRankingRow.
type CompetitionFinalizationRow struct {
	EntryID   string
	UserID    string
	Index     money.IndexValue
	ReturnPct money.Ratio
	ValuedAt  time.Time
}

// CompetitionResultRow is one immutable final-result row, written exactly
// once by PromoteFinalizationGeneration and never recomputed. Rank is
// materialized here (sequential, same tie-break chain as ranking
// generations — see PromoteGeneration's doc) rather than derived at read
// time.
type CompetitionResultRow struct {
	EntryID      string
	UserID       string
	Rank         int
	Index        money.IndexValue
	ReturnPct    money.Ratio
	EvidenceJSON json.RawMessage
}

// FinalizationRepository is the engine-side persistence boundary for
// completing an edition. Mirrors RankingRepository's building/promote shape
// (competition_finalization_generations / competition_finalization_rows,
// migration 0054) so a large competition's finalization is bounded and
// resumable across worker ticks instead of one unbounded pass; the terminal
// write target is the immutable competition_results table (migration 0048).
type FinalizationRepository interface {
	// EnsureBuildingFinalizationGeneration returns the competition's current
	// building finalization generation, creating generation 1 (building,
	// empty cursor) if none exists yet. Idempotent — safe to call every tick.
	EnsureBuildingFinalizationGeneration(ctx context.Context, competitionID string) (FinalizationGeneration, error)
	// ClaimEntriesForFinalization returns up to limit EntryActive entries for
	// the competition ordered by id after the cursor — the bounded batch one
	// worker tick values (mirrors RankingRepository.ClaimActiveEntries).
	// Withdrawn, never-baselined, and already-disqualified entries are never
	// scored into final results.
	ClaimEntriesForFinalization(ctx context.Context, competitionID, afterEntryID string, limit int) ([]CompetitionEntry, error)
	// UpsertFinalizationRow writes one entry's draft end-time valuation into
	// the building generation.
	UpsertFinalizationRow(ctx context.Context, competitionID string, generation int64, row CompetitionFinalizationRow) error
	// AdvanceFinalizationGeneration records this batch's progress: cursor,
	// processed counts, and whether any row write failed. writeFailed
	// latches — once true for a generation it stays true until the next
	// generation.
	AdvanceFinalizationGeneration(ctx context.Context, competitionID string, generation int64, cursorEntryID string, processed, successful, failed int, writeFailed bool) error
	// FailFinalizationGeneration terminally rejects an incomplete building
	// generation and removes its draft rows; the next
	// EnsureBuildingFinalizationGeneration call creates a fresh retry.
	FailFinalizationGeneration(ctx context.Context, competitionID string, generation int64, reason string, now time.Time) error
	// DisqualifyEntry guardedly moves an EntryActive entry to disqualified
	// with an auditable reason — the explicit resolution finalization applies
	// to entries that never obtained a valid end-time valuation within the
	// operational window.
	DisqualifyEntry(ctx context.Context, entryID, reason string, now time.Time) error
	// PromoteFinalizationGeneration atomically materializes sequential final
	// ranks (return% desc, display name asc, user_id asc) for the
	// generation's draft rows, copies them into the immutable
	// competition_results table, transitions scored entries to finalized,
	// transitions the edition finalizing -> completed, commits the
	// competition.finalized outbox event, and prunes the draft rows.
	// Guarded: fails with ErrGenerationConflict if the generation is not
	// still cleanly building (a write failure or an outstanding retryable
	// failure was recorded). Idempotent: if the competition is already
	// completed, this is a no-op success (safe to call again after a partial
	// failure or a worker restart), independent of which generation number
	// is passed.
	PromoteFinalizationGeneration(ctx context.Context, competitionID string, generation int64, now time.Time) error
	// TryLockCompetitionFinalization claims exclusive ownership of this
	// competition's finalization pass for the caller — mirrors
	// RankingRepository.TryLockCompetitionRanking, for the same reason: the
	// claim/advance/promote calls above have no per-row claim of their own
	// and would double-process a batch or double-count generation totals if
	// the scheduled worker and an admin retry ran concurrently.
	TryLockCompetitionFinalization(ctx context.Context, competitionID string) (unlock func(context.Context), ok bool, err error)
	// ListResults reads the immutable final results, ordered by rank. Returns
	// an empty slice (not an error) for a completed competition with zero
	// scored participants.
	ListResults(ctx context.Context, competitionID string) ([]CompetitionResultEntry, error)
}

// FinalizeCompetitions advances every edition in LifecycleFinalizing by one
// bounded finalization batch:
//
//  1. Value up to defaultFinalizationBatchSize EntryActive entries from the
//     competition's persisted end observation set (competition_observation_sets),
//     captured once per symbol/pair and never re-queried afterward, so a
//     slow or delayed finalizer's retries never reprice participants against
//     a later market moment than the pass that valued everyone else.
//  2. An entry whose valuation fails is retried on a later call while still
//     inside the finalization window of ends_at; past that window it is
//     explicitly disqualified (auditable reason) and finalization proceeds
//     without it — never a silent omission.
//  3. Once a full lap over the active population completes with zero
//     outstanding retryable failures, sequential final ranks are
//     materialized once, atomically, alongside the lifecycle transition —
//     results are then immutable: nothing after this point ever reprices
//     them with current market data. A large competition simply spans
//     however many calls its population needs; nothing here loads every
//     participant into memory at once.
//  4. A competition.finalized event is committed with the results. An
//     idempotent projector evaluates achievements with durable retries.
func (s *Service) FinalizeCompetitions(ctx context.Context) (int, error) {
	entryRepo, ok := s.repo.(EngineEntryRepository)
	finalRepo, ok2 := s.repo.(FinalizationRepository)
	obsRepo, ok3 := s.repo.(ObservationRepository)
	if !ok || !ok2 || !ok3 {
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
	batchSize := s.finalizationBatchSize
	if batchSize <= 0 {
		batchSize = defaultFinalizationBatchSize
	}
	now := s.clock.Now().UTC()

	finalized := 0
	for _, comp := range editions {
		ok, err := s.finalizeEdition(ctx, finalRepo, obsRepo, comp, batchSize, window, now)
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

func (s *Service) retryCompetitionFinalization(ctx context.Context, comp Competition) error {
	repo, ok := s.repo.(FinalizationRepository)
	obsRepo, ok2 := s.repo.(ObservationRepository)
	if !ok || !ok2 {
		return fmt.Errorf("competition finalization repository is unavailable")
	}
	if comp.LifecycleStatus != LifecycleFinalizing {
		return fmt.Errorf("finalization retry requires finalizing lifecycle")
	}
	window := s.finalizationWindow
	if window <= 0 {
		window = defaultFinalizationWindow
	}
	batchSize := s.finalizationBatchSize
	if batchSize <= 0 {
		batchSize = defaultFinalizationBatchSize
	}
	_, err := s.finalizeEdition(ctx, repo, obsRepo, comp, batchSize, window, s.clock.Now().UTC())
	return err
}

// finalizeEdition advances one competition's finalization by exactly one
// bounded batch, mirroring refreshEditionRanking's shape: claim a batch,
// value it against the persisted end observation set, advance the
// generation's coverage counters, and — only once a full lap completes with
// nothing left outstanding — promote atomically. ok=true only on the call
// that actually completes the edition.
func (s *Service) finalizeEdition(ctx context.Context, repo FinalizationRepository, obsRepo ObservationRepository, comp Competition, batchSize int, window time.Duration, now time.Time) (bool, error) {
	unlock, ok, err := repo.TryLockCompetitionFinalization(ctx, comp.ID)
	if err != nil {
		return false, fmt.Errorf("lock competition finalization: %w", err)
	}
	if !ok {
		slog.Info("competition_finalization_skipped_busy", "competition_id", comp.ID)
		return false, nil
	}
	defer unlock(ctx)

	_, scoring, err := effectiveRules(&comp)
	if err != nil {
		return false, fmt.Errorf("load scoring policy: %w", err)
	}

	gen, err := repo.EnsureBuildingFinalizationGeneration(ctx, comp.ID)
	if err != nil {
		return false, fmt.Errorf("ensure building finalization generation: %w", err)
	}
	entries, err := repo.ClaimEntriesForFinalization(ctx, comp.ID, gen.CursorEntryID, batchSize)
	if err != nil {
		return false, fmt.Errorf("claim entries for finalization: %w", err)
	}
	adjustedEntries := make([]CompetitionEntry, 0, len(entries))
	for _, entry := range entries {
		adjusted, adjustErr := s.adjustedEntryBasket(ctx, comp, entry, comp.EndsAt)
		if adjustErr != nil {
			return false, adjustErr
		}
		adjustedEntries = append(adjustedEntries, adjusted)
	}
	entries = adjustedEntries

	// One shared, persisted end-time observation set — captured incrementally
	// across every batch and every retry, so a slow or delayed finalizer
	// never reprices participants against a later market moment than the
	// pass that valued everyone else (see observations.go).
	memo, err := s.captureObservations(ctx, obsRepo, comp.ID, BoundaryEnd, comp.EndsAt, entries, now)
	if err != nil {
		return false, fmt.Errorf("capture end observations: %w", err)
	}

	deadline := comp.EndsAt.Add(window)
	processed, successful, failed, disqualified := 0, 0, 0, 0
	writeFailed := false
	var lastEntryID string
	for _, entry := range entries {
		processed++
		lastEntryID = entry.ID
		idx, retPct, valErr := s.valueCompetitionEntry(ctx, entry, memo, scoring)
		if valErr != nil {
			if now.After(deadline) {
				reason := fmt.Sprintf("no valid end-time valuation within the %s operational window after ends_at: %v", window, valErr)
				if dqErr := repo.DisqualifyEntry(ctx, entry.ID, reason, now); dqErr != nil {
					return false, fmt.Errorf("disqualify entry %s: %w", entry.ID, dqErr)
				}
				disqualified++
				slog.Warn("competition_finalization_entry_disqualified",
					"competition_id", comp.ID, "entry_id", entry.ID, "error", valErr)
			} else {
				failed++
				slog.Warn("competition_finalization_entry_retry",
					"competition_id", comp.ID, "entry_id", entry.ID, "error", valErr)
			}
			continue
		}
		row := CompetitionFinalizationRow{EntryID: entry.ID, UserID: entry.UserID, Index: idx, ReturnPct: retPct, ValuedAt: now}
		if err := repo.UpsertFinalizationRow(ctx, comp.ID, gen.Generation, row); err != nil {
			writeFailed = true
			slog.Error("competition_finalization_write_failed",
				"competition_id", comp.ID, "entry_id", entry.ID, "error", err)
			continue
		}
		successful++
	}

	lapComplete := len(entries) < batchSize
	cursor := gen.CursorEntryID
	if lastEntryID != "" {
		cursor = lastEntryID
	}
	if lapComplete {
		cursor = "" // start the next lap from the beginning
	}
	if err := repo.AdvanceFinalizationGeneration(ctx, comp.ID, gen.Generation, cursor, processed, successful, failed, writeFailed); err != nil {
		return false, fmt.Errorf("advance finalization generation: %w", err)
	}
	if disqualified > 0 {
		slog.Warn("competition_finalization_disqualified_entries", "competition_id", comp.ID, "count", disqualified)
	}

	if !lapComplete {
		return false, nil
	}
	totalFailed := gen.FailedEntries + failed
	if gen.WriteFailure || writeFailed || totalFailed > 0 {
		reason := fmt.Sprintf("incomplete coverage: processed=%d successful=%d failed=%d write_failure=%t",
			gen.ProcessedEntries+processed, gen.SuccessfulEntries+successful, totalFailed, gen.WriteFailure || writeFailed)
		if err := repo.FailFinalizationGeneration(ctx, comp.ID, gen.Generation, reason, now); err != nil {
			return false, fmt.Errorf("fail incomplete finalization generation: %w", err)
		}
		slog.Warn("competition_finalization_generation_failed",
			"competition_id", comp.ID, "generation", gen.Generation, "failed_entries", totalFailed)
		return false, nil
	}

	if err := repo.PromoteFinalizationGeneration(ctx, comp.ID, gen.Generation, now); err != nil {
		return false, fmt.Errorf("promote finalization generation: %w", err)
	}
	slog.Info("competition_finalized", "competition_id", comp.ID, "generation", gen.Generation)
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

// memoryFinalizationState mirrors memoryRankingState (see ranking.go), minus
// the active/superseded bookkeeping: a finalization generation is either
// still building, failed (retried under a fresh generation number), or
// completed (terminal — the edition is never finalized again).
type memoryFinalizationState struct {
	mu          sync.Mutex
	generations map[int64]*FinalizationGeneration
	rows        map[int64]map[string]CompetitionFinalizationRow
	nextGen     int64
	locked      bool
}

func (r *InMemoryCompetitionRepository) finalizationState(competitionID string) *memoryFinalizationState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finalizationStates == nil {
		r.finalizationStates = map[string]*memoryFinalizationState{}
	}
	st, ok := r.finalizationStates[competitionID]
	if !ok {
		st = &memoryFinalizationState{generations: map[int64]*FinalizationGeneration{}, rows: map[int64]map[string]CompetitionFinalizationRow{}}
		r.finalizationStates[competitionID] = st
	}
	return st
}

func (r *InMemoryCompetitionRepository) TryLockCompetitionFinalization(_ context.Context, competitionID string) (func(context.Context), bool, error) {
	st := r.finalizationState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.locked {
		return nil, false, nil
	}
	st.locked = true
	return func(context.Context) {
		st.mu.Lock()
		st.locked = false
		st.mu.Unlock()
	}, true, nil
}

func (r *InMemoryCompetitionRepository) EnsureBuildingFinalizationGeneration(_ context.Context, competitionID string) (FinalizationGeneration, error) {
	st := r.finalizationState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	var building *FinalizationGeneration
	for _, g := range st.generations {
		if g.Status == GenerationBuilding {
			building = g
		}
	}
	if building == nil {
		st.nextGen++
		building = &FinalizationGeneration{CompetitionID: competitionID, Generation: st.nextGen, Status: GenerationBuilding, StartedAt: time.Now().UTC()}
		st.generations[building.Generation] = building
		st.rows[building.Generation] = map[string]CompetitionFinalizationRow{}
	}
	return *building, nil
}

func (r *InMemoryCompetitionRepository) ClaimEntriesForFinalization(_ context.Context, competitionID, afterEntryID string, limit int) ([]CompetitionEntry, error) {
	r.mu.RLock()
	byUser := r.entries[competitionID]
	entries := make([]CompetitionEntry, 0, len(byUser))
	for _, e := range byUser {
		if e.EntryStatus == EntryActive {
			entries = append(entries, e)
		}
	}
	r.mu.RUnlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	out := make([]CompetitionEntry, 0, limit)
	for _, e := range entries {
		if afterEntryID != "" && e.ID <= afterEntryID {
			continue
		}
		out = append(out, e)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (r *InMemoryCompetitionRepository) UpsertFinalizationRow(_ context.Context, competitionID string, generation int64, row CompetitionFinalizationRow) error {
	st := r.finalizationState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	g, ok := st.generations[generation]
	if !ok || g.Status != GenerationBuilding {
		return ErrGenerationConflict
	}
	st.rows[generation][row.EntryID] = row
	return nil
}

func (r *InMemoryCompetitionRepository) AdvanceFinalizationGeneration(_ context.Context, competitionID string, generation int64, cursorEntryID string, processed, successful, failed int, writeFailed bool) error {
	st := r.finalizationState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	g, ok := st.generations[generation]
	if !ok {
		return ErrGenerationConflict
	}
	g.CursorEntryID = cursorEntryID
	g.ProcessedEntries += processed
	g.SuccessfulEntries += successful
	g.FailedEntries += failed
	if writeFailed {
		g.WriteFailure = true
	}
	return nil
}

func (r *InMemoryCompetitionRepository) FailFinalizationGeneration(_ context.Context, competitionID string, generation int64, reason string, now time.Time) error {
	st := r.finalizationState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	g, ok := st.generations[generation]
	if !ok || g.Status != GenerationBuilding {
		return ErrGenerationConflict
	}
	completed := now.UTC()
	g.Status = GenerationFailed
	g.CompletedAt = &completed
	g.FailureReason = reason
	delete(st.rows, generation)
	return nil
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

// PromoteFinalizationGeneration materializes sequential final ranks (return%
// desc, display name asc, user_id asc — the same tie-break chain as
// PromoteGeneration), copies the generation's draft rows into the immutable
// results store, transitions the edition finalizing -> completed, and prunes
// the draft rows. If the competition is already completed, this is an
// idempotent no-op success regardless of the generation argument — mirrors
// the Postgres implementation's lifecycle-first guard.
func (r *InMemoryCompetitionRepository) PromoteFinalizationGeneration(_ context.Context, competitionID string, generation int64, now time.Time) error {
	r.mu.Lock()
	comp, ok := r.competitions[competitionID]
	if !ok {
		r.mu.Unlock()
		return ErrCompetitionNotFound
	}
	if comp.LifecycleStatus == LifecycleCompleted {
		r.mu.Unlock()
		return nil // idempotent replay: already finalized
	}
	if comp.LifecycleStatus != LifecycleFinalizing {
		r.mu.Unlock()
		return ErrLifecycleConflict
	}
	r.mu.Unlock()

	st := r.finalizationState(competitionID)
	st.mu.Lock()
	g, ok := st.generations[generation]
	if !ok || g.Status != GenerationBuilding || g.WriteFailure || g.FailedEntries != 0 {
		st.mu.Unlock()
		return ErrGenerationConflict
	}
	rows := make([]CompetitionFinalizationRow, 0, len(st.rows[generation]))
	for _, row := range st.rows[generation] {
		rows = append(rows, row)
	}
	st.mu.Unlock()

	names := make(map[string]string, len(rows))
	for _, row := range rows {
		if name, err := r.userDisplayName(row.UserID); err == nil {
			names[row.UserID] = name
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if c := rows[i].ReturnPct.Cmp(rows[j].ReturnPct); c != 0 {
			return c > 0
		}
		if names[rows[i].UserID] != names[rows[j].UserID] {
			return names[rows[i].UserID] < names[rows[j].UserID]
		}
		return rows[i].UserID < rows[j].UserID
	})

	evidence, _ := json.Marshal(map[string]any{
		"valuation_policy":  "fixed_basket_price_return_v1",
		"corporate_actions": "splits_and_symbol_changes_normalized",
		"finalized_at":      now,
	})
	results := make([]CompetitionResultRow, 0, len(rows))
	for i, row := range rows {
		results = append(results, CompetitionResultRow{
			EntryID: row.EntryID, UserID: row.UserID, Rank: i + 1,
			Index: row.Index, ReturnPct: row.ReturnPct, EvidenceJSON: evidence,
		})
	}

	store := r.results()
	store.mu.Lock()
	store.rows[competitionID] = results
	store.mu.Unlock()

	r.mu.Lock()
	comp = r.competitions[competitionID]
	comp.LifecycleStatus = LifecycleCompleted
	stamp := now.UTC()
	comp.FinalizedAt = &stamp
	r.competitions[competitionID] = comp
	for _, res := range results {
		for userID, e := range r.entries[competitionID] {
			// Scored entries become terminally 'finalized' — this is what
			// makes the result immutable end to end: a finalized entry can
			// no longer be disqualified, withdrawn, or otherwise mutated by
			// any other guarded transition (all require entry_status = 'active').
			if e.ID == res.EntryID && e.EntryStatus == EntryActive {
				e.EntryStatus = EntryFinalized
				r.entries[competitionID][userID] = e
			}
		}
	}
	r.mu.Unlock()

	st.mu.Lock()
	g.Status = GenerationCompleted
	completed := now.UTC()
	g.CompletedAt = &completed
	delete(st.rows, generation)
	st.mu.Unlock()

	return nil
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
