package competitions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// RankingGeneration is one attempt at building a competition's ranking
// projection (table competition_ranking_generations). Unlike the global
// leaderboard's single active/building pointer row, each attempt gets its own
// row here so expected/processed/successful/failed counts and any failure
// reason stay attached to the specific generation they describe.
type RankingGeneration struct {
	CompetitionID     string
	Generation        int64
	Status            string // building | active | superseded | failed
	ExpectedEntries   int
	ProcessedEntries  int
	SuccessfulEntries int
	FailedEntries     int
	CursorEntryID     string
	WriteFailure      bool
	StartedAt         time.Time
	CompletedAt       *time.Time
	ActivatedAt       *time.Time
	FailureReason     string
}

// Ranking generation statuses.
const (
	GenerationBuilding   = "building"
	GenerationActive     = "active"
	GenerationSuperseded = "superseded"
	GenerationFailed     = "failed"
)

// CompetitionRankingRow is one entry's materialized ranking-projection row.
type CompetitionRankingRow struct {
	EntryID   string
	UserID    string
	Rank      int // 0 until PromoteGeneration materializes it
	Index     money.IndexValue
	ReturnPct money.Ratio
	ValuedAt  time.Time
}

// defaultRankingBatchSize bounds how many entries one RefreshCompetitionRankings
// pass claims and values per competition per call — the same fixed-cost-per-tick
// principle as the global leaderboard's refresh batching.
const defaultRankingBatchSize = 500

// RankingRepository is the engine-side persistence boundary for competition
// ranking generations — separate tables and separate promotion machinery from
// the global leaderboard (see leaderboard.RankingStore for the pattern this
// mirrors). Implemented by PostgresCompetitionRepository and
// InMemoryCompetitionRepository.
type RankingRepository interface {
	// EnsureBuildingGeneration returns the competition's current building
	// generation, creating generation 1 (building, empty cursor) if none
	// exists yet. Idempotent — safe to call every tick.
	EnsureBuildingGeneration(ctx context.Context, competitionID string) (RankingGeneration, error)
	// ClaimActiveEntries returns up to limit EntryActive entries for the
	// competition ordered by id after the cursor — the bounded batch one
	// worker tick values. A page shorter than limit marks the end of the
	// population for this lap (mirrors leaderboard's nextPagedBatch).
	ClaimActiveEntries(ctx context.Context, competitionID, afterEntryID string, limit int) ([]CompetitionEntry, error)
	// UpsertRanking writes one entry's valuation into the building generation.
	UpsertRanking(ctx context.Context, competitionID string, generation int64, row CompetitionRankingRow) error
	// AdvanceGeneration records this batch's progress: cursor, processed
	// counts, and whether any ranking-row write failed. writeFailed latches —
	// once true for a generation it stays true until the next generation.
	AdvanceGeneration(ctx context.Context, competitionID string, generation int64, cursorEntryID string, processed, successful, failed int, writeFailed bool) error
	// PromoteGeneration materializes sequential ranks (return% desc, display
	// name asc, user_id asc — see the package doc comment on tie-breaking),
	// atomically promotes the generation to active, marks the previously
	// active generation superseded and prunes its rows, and creates the next
	// building generation. Guarded: fails with ErrGenerationConflict if the
	// generation is not still building (e.g. a write failure was recorded
	// concurrently, or another replica already promoted it).
	PromoteGeneration(ctx context.Context, competitionID string, generation int64, now time.Time) error
	// ActiveGeneration returns the competition's currently active generation
	// number and when it was promoted. found=false means no generation has
	// ever completed a full pass — callers must return a controlled
	// "unavailable" response, never fall back to a live O(N) scan.
	ActiveGeneration(ctx context.Context, competitionID string) (generation int64, activatedAt time.Time, found bool, err error)
	// LeaderboardPage reads a cursor-paginated page (rank > afterRank) from
	// the active generation, joined with display metadata.
	LeaderboardPage(ctx context.Context, competitionID string, afterRank, limit int) ([]CompetitionLeaderboardEntry, error)
	// UserRankRow returns userID's exact row from the active generation.
	// found=false means the user has no row (not yet reached by a refresh, or
	// genuinely unranked) — callers treat this as "not ranked yet", not an error.
	UserRankRow(ctx context.Context, competitionID, userID string) (row CompetitionRankingRow, found bool, err error)
}

// SetRankingBatchSize overrides how many entries RefreshCompetitionRankings
// claims per competition per call. n <= 0 restores the default.
func (s *Service) SetRankingBatchSize(n int) {
	if n <= 0 {
		n = defaultRankingBatchSize
	}
	s.rankingBatchSize = n
}

// RefreshCompetitionRankings values one bounded batch of active entries per
// currently-active engine edition and writes them into that competition's
// building generation. When a batch reaches the end of the entry population
// (a short page), the lap is complete and the generation is promoted — unless
// any ranking-row write failed during the lap, in which case promotion is
// withheld and the same building generation is retried from scratch on the
// next lap (see PromoteGeneration's doc and leaderboard.Service.RefreshCache,
// which this mirrors). A single entry that fails VALUATION (unpriceable
// position, stale FX) is simply excluded from this generation — like the
// global leaderboard's "skipped" users — and does not block promotion.
func (s *Service) RefreshCompetitionRankings(ctx context.Context) error {
	repo, ok := s.repo.(RankingRepository)
	entryRepo, ok2 := s.repo.(EngineEntryRepository)
	if !ok || !ok2 {
		return nil
	}
	editions, err := entryRepo.ListEditionsByLifecycle(ctx, LifecycleActive)
	if err != nil {
		return fmt.Errorf("competitions: list active editions for ranking refresh: %w", err)
	}
	batchSize := s.rankingBatchSize
	if batchSize <= 0 {
		batchSize = defaultRankingBatchSize
	}
	now := s.clock.Now().UTC()
	for _, comp := range editions {
		if err := s.refreshEditionRanking(ctx, repo, comp.ID, batchSize, now); err != nil {
			slog.Error("competition_ranking_refresh_failed", "competition_id", comp.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) refreshEditionRanking(ctx context.Context, repo RankingRepository, competitionID string, batchSize int, now time.Time) error {
	gen, err := repo.EnsureBuildingGeneration(ctx, competitionID)
	if err != nil {
		return fmt.Errorf("ensure building generation: %w", err)
	}
	entries, err := repo.ClaimActiveEntries(ctx, competitionID, gen.CursorEntryID, batchSize)
	if err != nil {
		return fmt.Errorf("claim active entries: %w", err)
	}

	memo := newObservationMemo(s)
	processed, successful, failed := 0, 0, 0
	writeFailed := false
	var lastEntryID string
	for _, entry := range entries {
		processed++
		lastEntryID = entry.ID
		idx, retPct, err := s.valueCompetitionEntry(ctx, entry, memo)
		if err != nil {
			failed++
			slog.Warn("competition_ranking_entry_skipped",
				"competition_id", competitionID, "entry_id", entry.ID, "error", err)
			continue
		}
		row := CompetitionRankingRow{EntryID: entry.ID, UserID: entry.UserID, Index: idx, ReturnPct: retPct, ValuedAt: now}
		if err := repo.UpsertRanking(ctx, competitionID, gen.Generation, row); err != nil {
			writeFailed = true
			slog.Error("competition_ranking_write_failed",
				"competition_id", competitionID, "entry_id", entry.ID, "error", err)
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
	if err := repo.AdvanceGeneration(ctx, competitionID, gen.Generation, cursor, processed, successful, failed, writeFailed); err != nil {
		return fmt.Errorf("advance generation: %w", err)
	}

	if !lapComplete {
		return nil
	}
	// An empty lap on an edition that was never ranked (zero entries ever)
	// is not a completed pass over a real population — nothing to promote.
	if gen.ProcessedEntries+processed == 0 {
		return nil
	}
	if err := repo.PromoteGeneration(ctx, competitionID, gen.Generation, now); err != nil {
		slog.Warn("competition_ranking_promotion_skipped", "competition_id", competitionID, "generation", gen.Generation, "error", err)
		return nil
	}
	slog.Info("competition_ranking_generation_promoted", "competition_id", competitionID, "generation", gen.Generation)
	return nil
}

// valueCompetitionEntry reprices an entry's frozen, scoring-included
// composition at current prices (memo shares observations across a batch,
// exactly like baselineEntry) and returns its competition index (normalized
// to the 100 baseline) and return percentage. This is the single valuation
// core shared by ranking refresh and finalization — finalization simply
// supplies an end-time-scoped memo.
func (s *Service) valueCompetitionEntry(ctx context.Context, entry CompetitionEntry, memo *observationMemo) (money.IndexValue, money.Ratio, error) {
	if entry.EligibleStartingValueBase.Sign() <= 0 {
		return money.ZeroIndexValue(), money.ZeroRatio(), fmt.Errorf("entry %s has no eligible starting value", entry.ID)
	}
	total := money.ZeroAmount()
	for _, snap := range entry.Snapshots {
		if !snap.IncludedInScore {
			continue
		}
		price, currency, err := memo.priceOf(ctx, snap.Symbol)
		if err != nil {
			return money.ZeroIndexValue(), money.ZeroRatio(), fmt.Errorf("price %s: %w", snap.Symbol, err)
		}
		local := snap.Quantity.MulPrice(price)
		base, err := memo.toBase(ctx, local, currency)
		if err != nil {
			return money.ZeroIndexValue(), money.ZeroRatio(), fmt.Errorf("fx %s: %w", currency, err)
		}
		total = total.Add(base)
	}
	for _, cash := range entry.CashSnapshots {
		if !cash.IncludedInScore {
			continue
		}
		base, err := memo.toBase(ctx, cash.Amount, cash.Currency)
		if err != nil {
			return money.ZeroIndexValue(), money.ZeroRatio(), fmt.Errorf("fx cash %s: %w", cash.Currency, err)
		}
		total = total.Add(base)
	}
	factor, err := total.DivExact(entry.EligibleStartingValueBase, money.ScaleIndex+money.ScaleWeight)
	if err != nil {
		return money.ZeroIndexValue(), money.ZeroRatio(), fmt.Errorf("index factor: %w", err)
	}
	idx := money.QuantizeIndex(money.MustIndexValue("100").MulRatio(factor))
	retPct := money.QuantizeRatio(factor.Sub(money.MustRatio("1")).Mul(money.MustRatio("100")), 2)
	return idx, retPct, nil
}

// EditionLeaderboard reads a cursor-paginated page from the active ranking
// generation. Unavailable=true (never an error, never a live scan) means no
// generation has been promoted yet — see RankingRepository.ActiveGeneration.
func (s *Service) EditionLeaderboard(ctx context.Context, competitionID string, afterRank, limit int) (*CompetitionLeaderboardPage, error) {
	comp, err := s.loadCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	repo, ok := s.repo.(RankingRepository)
	if !ok || comp.IsLegacy() {
		return nil, ErrRankingUnavailable
	}
	if limit <= 0 || limit > maxCompetitionLeaderboardPage {
		limit = maxCompetitionLeaderboardPage
	}
	_, activatedAt, found, err := repo.ActiveGeneration(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	if !found {
		return &CompetitionLeaderboardPage{Unavailable: true}, nil
	}
	rows, err := repo.LeaderboardPage(ctx, competitionID, afterRank, limit)
	if err != nil {
		return nil, err
	}
	page := &CompetitionLeaderboardPage{Entries: rows, ValuedAt: &activatedAt}
	if len(rows) == limit {
		next := rows[len(rows)-1].Rank
		page.NextCursor = &next
	}
	return page, nil
}

// maxCompetitionLeaderboardPage caps how many rows one leaderboard read
// serves, mirroring the global leaderboard's page-size discipline.
const maxCompetitionLeaderboardPage = 100

// EngineMyStatus is EditionMyStatus's entry-point for handlers: it resolves
// the caller's entry (if any) for an engine edition, then reports rank/return
// from the ranking projection. A user with no entry is reported as not
// joined, matching the legacy MyStatus contract.
func (s *Service) EngineMyStatus(ctx context.Context, comp *Competition, userID string) (*MyCompetitionStatusResponse, error) {
	entry, err := s.repo.GetEntry(ctx, comp.ID, userID)
	if err != nil {
		if errors.Is(err, ErrEntryNotFound) {
			return &MyCompetitionStatusResponse{CompetitionID: comp.ID, Joined: false, SprintIndex: money.MustIndexValue("100")}, nil
		}
		return nil, err
	}
	return s.EditionMyStatus(ctx, comp, entry)
}

// EditionMyStatus returns the caller's status for an engine edition from the
// ranking projection — never a live per-request scan. found rank 0 means the
// user's entry has not yet been reached by a ranking pass (or the projection
// has never been promoted); this is reported as "not yet ranked", not an error.
func (s *Service) EditionMyStatus(ctx context.Context, comp *Competition, entry *CompetitionEntry) (*MyCompetitionStatusResponse, error) {
	resp := &MyCompetitionStatusResponse{
		CompetitionID: comp.ID, Joined: true, EntryStatus: entry.EntryStatus,
		SprintIndex: money.MustIndexValue("100"),
	}
	repo, ok := s.repo.(RankingRepository)
	if !ok || entry.EntryStatus != EntryActive {
		return resp, nil
	}
	row, found, err := repo.UserRankRow(ctx, comp.ID, entry.UserID)
	if err != nil {
		return nil, err
	}
	if !found {
		return resp, nil
	}
	resp.CurrentRank = row.Rank
	resp.SprintReturnPercentage = row.ReturnPct
	resp.SprintIndex = row.Index
	valuedAt := row.ValuedAt
	resp.ValuedAt = &valuedAt
	return resp, nil
}

// --- in-memory implementation -------------------------------------------------

type memoryRankingState struct {
	mu          sync.Mutex
	generations map[int64]*RankingGeneration
	rows        map[int64]map[string]CompetitionRankingRow // generation -> entryID -> row
	nextGen     int64
	active      int64 // 0 = none promoted yet
}

var _ RankingRepository = (*InMemoryCompetitionRepository)(nil)

func (r *InMemoryCompetitionRepository) rankingState(competitionID string) *memoryRankingState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rankingStates == nil {
		r.rankingStates = map[string]*memoryRankingState{}
	}
	st, ok := r.rankingStates[competitionID]
	if !ok {
		st = &memoryRankingState{generations: map[int64]*RankingGeneration{}, rows: map[int64]map[string]CompetitionRankingRow{}}
		r.rankingStates[competitionID] = st
	}
	return st
}

func (r *InMemoryCompetitionRepository) EnsureBuildingGeneration(_ context.Context, competitionID string) (RankingGeneration, error) {
	st := r.rankingState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.nextGen == 0 {
		st.nextGen = 1
		st.generations[1] = &RankingGeneration{CompetitionID: competitionID, Generation: 1, Status: GenerationBuilding, StartedAt: time.Now().UTC()}
		st.rows[1] = map[string]CompetitionRankingRow{}
	}
	// The current building generation is always the highest one in status
	// building; PromoteGeneration creates the next one atomically.
	var building *RankingGeneration
	for _, g := range st.generations {
		if g.Status == GenerationBuilding {
			building = g
		}
	}
	if building == nil {
		st.nextGen++
		building = &RankingGeneration{CompetitionID: competitionID, Generation: st.nextGen, Status: GenerationBuilding, StartedAt: time.Now().UTC()}
		st.generations[building.Generation] = building
		st.rows[building.Generation] = map[string]CompetitionRankingRow{}
	}
	return *building, nil
}

func (r *InMemoryCompetitionRepository) ClaimActiveEntries(_ context.Context, competitionID, afterEntryID string, limit int) ([]CompetitionEntry, error) {
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

func (r *InMemoryCompetitionRepository) UpsertRanking(_ context.Context, competitionID string, generation int64, row CompetitionRankingRow) error {
	st := r.rankingState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	g, ok := st.generations[generation]
	if !ok || g.Status != GenerationBuilding {
		return ErrGenerationConflict
	}
	st.rows[generation][row.EntryID] = row
	return nil
}

func (r *InMemoryCompetitionRepository) AdvanceGeneration(_ context.Context, competitionID string, generation int64, cursorEntryID string, processed, successful, failed int, writeFailed bool) error {
	st := r.rankingState(competitionID)
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

func (r *InMemoryCompetitionRepository) PromoteGeneration(_ context.Context, competitionID string, generation int64, now time.Time) error {
	st := r.rankingState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	g, ok := st.generations[generation]
	if !ok || g.Status != GenerationBuilding || g.WriteFailure {
		return ErrGenerationConflict
	}

	// Materialize sequential ranks: return% desc, display name asc, user_id
	// asc. This chain is a strict total order (user_id is unique), so ranks
	// are always sequential — no ties ever share a rank. See the doc on
	// RankingRepository.PromoteGeneration.
	rows := make([]CompetitionRankingRow, 0, len(st.rows[generation]))
	for _, row := range st.rows[generation] {
		rows = append(rows, row)
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		if u, err := r.userDisplayName(row.UserID); err == nil {
			names[row.UserID] = u
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
	for i := range rows {
		rows[i].Rank = i + 1
		st.rows[generation][rows[i].EntryID] = rows[i]
	}

	completed := now
	g.Status = GenerationActive
	g.CompletedAt = &completed
	g.ActivatedAt = &completed

	if st.active != 0 {
		if prev, ok := st.generations[st.active]; ok {
			prev.Status = GenerationSuperseded
		}
		delete(st.rows, st.active)
	}
	st.active = generation
	return nil
}

// userDisplayName is a tiny in-memory helper: the memory repository has no
// users table, so it looks the display name up via a package-level test hook
// when set, else falls back to the user id (still a valid, if unreadable,
// deterministic sort key).
func (r *InMemoryCompetitionRepository) userDisplayName(userID string) (string, error) {
	if r.displayNames == nil {
		return userID, nil
	}
	if name, ok := r.displayNames[userID]; ok {
		return name, nil
	}
	return userID, nil
}

// SetDisplayNames lets tests seed the display-name lookup PromoteGeneration
// uses for its tie-break (Postgres joins the users table for this instead).
func (r *InMemoryCompetitionRepository) SetDisplayNames(names map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.displayNames = names
}

func (r *InMemoryCompetitionRepository) ActiveGeneration(_ context.Context, competitionID string) (int64, time.Time, bool, error) {
	st := r.rankingState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.active == 0 {
		return 0, time.Time{}, false, nil
	}
	g := st.generations[st.active]
	return g.Generation, *g.ActivatedAt, true, nil
}

func (r *InMemoryCompetitionRepository) LeaderboardPage(_ context.Context, competitionID string, afterRank, limit int) ([]CompetitionLeaderboardEntry, error) {
	st := r.rankingState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.active == 0 {
		return nil, nil
	}
	rows := make([]CompetitionRankingRow, 0, len(st.rows[st.active]))
	for _, row := range st.rows[st.active] {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Rank < rows[j].Rank })
	out := make([]CompetitionLeaderboardEntry, 0, limit)
	for _, row := range rows {
		if row.Rank <= afterRank {
			continue
		}
		name, _ := r.userDisplayName(row.UserID)
		out = append(out, CompetitionLeaderboardEntry{
			Rank: row.Rank, DisplayName: name,
			ReturnPercentage: row.ReturnPct, CompetitionIndex: row.Index,
		})
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (r *InMemoryCompetitionRepository) UserRankRow(_ context.Context, competitionID, userID string) (CompetitionRankingRow, bool, error) {
	st := r.rankingState(competitionID)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.active == 0 {
		return CompetitionRankingRow{}, false, nil
	}
	for _, row := range st.rows[st.active] {
		if row.UserID == userID {
			return row, true, nil
		}
	}
	return CompetitionRankingRow{}, false, nil
}
