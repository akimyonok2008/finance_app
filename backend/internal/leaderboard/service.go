package leaderboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
)

// UserProvider enumerates the users to rank. Implemented by *auth.Service via
// ListRankableUsers, which returns only the narrow projection the leaderboard
// needs — never the password hash or other sensitive account fields.
type UserProvider interface {
	ListRankableUsers(ctx context.Context) ([]auth.RankableUser, error)
}

// PagedUserProvider is an optional capability of UserProvider: keyset-
// paginated enumeration ordered by user ID. When the attached provider
// implements it (and batching is enabled), RefreshCache selects each tick's
// batch with a single bounded page query — O(batch size) per tick — instead
// of loading every rankable user into memory on every run. afterID = "" starts
// from the beginning; a returned page shorter than limit marks the end of the
// population (the end of a refresh lap).
type PagedUserProvider interface {
	ListRankableUsersPage(ctx context.Context, afterID string, limit int) ([]auth.RankableUser, error)
}

// RankedPerformance is the leaderboard's view of a user's persistent ranked
// standing — the single trusted source of global performance. It carries NO
// absolute monetary values. Paused users (empty portfolios) preserve their index
// but are excluded from active ranking.
type RankedPerformance struct {
	RankedIndex            money.IndexValue
	RankedReturnPercentage money.Ratio
	Paused                 bool
	TrackingStartedAt      time.Time
}

// RankedPerformanceProvider supplies a user's persistent ranked performance.
// Implemented by an adapter over *performance.Service. This replaces the old
// PortfolioSummaryProvider: the leaderboard no longer derives trusted ranking
// from mutable position baselines.
type RankedPerformanceProvider interface {
	CurrentRankedPerformance(ctx context.Context, userID string) (RankedPerformance, error)
}

// ProfilePublicInfo is the public-facing profile data joined onto a leaderboard
// row. HasProfile=false means the user never created a profile.
type ProfilePublicInfo struct {
	Handle      string
	StrategyTag string
	IsPublic    bool
	ShowWeights bool
	Weights     []PublicWeight
}

// ProfilePublicProvider supplies public profile data for enrichment.
// Implemented by an adapter over *profile.Service, wired in main.
type ProfilePublicProvider interface {
	PublicInfo(ctx context.Context, userID string) (info ProfilePublicInfo, hasProfile bool, err error)
}

// maxLeaderboardSize caps how many entries a leaderboard read serves.
const maxLeaderboardSize = 100

// RankingStore is the optional denormalized ranking projection (a Postgres
// table populated by RefreshCache from the same per-user valuation it already
// pays for). Reading it back lets board, per-user-rank, and windowed-standing
// queries do O(page size) work, because rank is materialized once per
// generation by CompleteCycle rather than recomputed by a window function on
// every read — see PostgresRankingStore's doc. A query error or a
// never-promoted projection falls back to the live computation; mere
// staleness does not (see useRanking) — a stale O(page) read always beats an
// O(population) live pass.
type RankingStore interface {
	// Upsert and Delete always act on the projection's current "building"
	// generation (tracked in leaderboard_ranking_state) — never on the
	// generation reads are currently served from. Rows only become visible to
	// TopPage/RankOf/ValueAtRank/Count once CompleteCycle promotes this
	// generation to active.
	Upsert(ctx context.Context, tf Timeframe, userID string, idx money.IndexValue, retPct money.Ratio, trackingStartedAt time.Time) error
	Delete(ctx context.Context, tf Timeframe, userID string) error
	// TopPage returns up to limit rows for tf from the active generation,
	// ranked by a window function and joined to display metadata in a single
	// round trip.
	TopPage(ctx context.Context, tf Timeframe, limit int) ([]rankedRow, error)
	// RankOf returns userID's exact rank/index/return for tf from the active
	// generation. found=false means userID has no row for tf — this can mean
	// genuinely unranked OR simply "not yet reached by a refresh cycle", so
	// callers must treat it as "unknown" and fall back rather than as an
	// authoritative negative.
	RankOf(ctx context.Context, tf Timeframe, userID string) (rank int, idx money.IndexValue, retPct money.Ratio, found bool, err error)
	// ValueAtRank returns the return percentage of the row at an exact 1-based
	// rank in the active generation, for milestone-gap calculations.
	ValueAtRank(ctx context.Context, tf Timeframe, rank int) (retPct money.Ratio, found bool, err error)
	Count(ctx context.Context, tf Timeframe) (int, error)
	// ActiveGenerationAge reports when the currently-active generation was
	// promoted (see CompleteCycle). found=false means no generation has ever
	// completed a full, verified pass over every eligible user — callers must
	// fall back to live computation rather than trust a partially-built or
	// never-built projection.
	ActiveGenerationAge(ctx context.Context) (activatedAt time.Time, found bool, err error)
	// CompleteCycle atomically promotes the building generation (the one
	// Upsert/Delete have been writing into) to active — making its rows the
	// ones TopPage/RankOf/etc. read — then starts the next building
	// generation and prunes the rows of the generation that was just
	// replaced. Callers must only call this once a full pass over every
	// eligible user has actually been attempted (see Service.nextBatch's
	// cycle tracking); calling it early would promote an incomplete
	// generation and reintroduce the "board silently omits users" bug this
	// mechanism exists to prevent.
	CompleteCycle(ctx context.Context) error
}

// CycleDurationReporter is an optional capability of RankingStore: how long
// the most recently promoted generation took to build. rankingFresh uses it
// to scale the projection trust window with the measured cycle time, so a
// large population (whose full refresh lap necessarily exceeds any fixed
// window) keeps serving O(page size) projection reads instead of collapsing
// onto the O(N) live path for the tail of every cycle.
type CycleDurationReporter interface {
	LastCycleDuration(ctx context.Context) (time.Duration, bool, error)
}

// ProfilePublicBatchProvider is an optional capability of ProfilePublicProvider:
// implementing it lets enrichPage join profile data for an entire page in one
// query instead of one round trip per row.
type ProfilePublicBatchProvider interface {
	PublicInfoBatch(ctx context.Context, userIDs []string) (map[string]ProfilePublicInfo, error)
}

// allTimeframes is every timeframe RefreshCache maintains a ranking-projection
// row for.
var allTimeframes = []Timeframe{TimeframeAll, Timeframe1W, Timeframe1M, Timeframe3M, Timeframe6M, Timeframe1Y}

// defaultMaxRankingAge is the FLOOR of the projection trust window: a
// generation younger than this is always trusted. When the ranking store
// reports the measured duration of the last completed build cycle (see
// CycleDurationReporter), the window stretches to cycleTrustFactor times that
// duration, so trust scales with the population instead of expiring on a
// fixed clock while the next generation is still (necessarily) building.
const defaultMaxRankingAge = 30 * time.Minute

// cycleTrustFactor multiplies the last measured cycle duration to form the
// adaptive trust window. 2x means: the projection stays trusted through one
// full rebuild plus an equally long grace lap, so a single slow or failed lap
// never flips reads onto the O(N) live path.
const cycleTrustFactor = 2

// defaultMaxRankingHardAge caps the adaptive trust window. A generation older
// than this is distrusted no matter what the last cycle duration was — at
// that point the refresh pipeline is not merely slow, it is dead, and falling
// back to live computation (plus alerting) is the correct behavior.
const defaultMaxRankingHardAge = 24 * time.Hour

// defaultRefreshParallelism is how many users a RefreshCache batch revalues
// concurrently. Parallelism directly divides full-cycle wall time, which is
// what keeps generation build time inside the trust window as the population
// grows.
const defaultRefreshParallelism = 1

// defaultMaxLiveComputeUsers bounds the cold-start-only live ranking path: a
// population up to this size may still be ranked live per request before the
// first projection generation completes; beyond it, reads degrade softly
// instead of amplifying load (see liveComputeAllowed).
const defaultMaxLiveComputeUsers = 2000

// defaultMaxSnapshotAge bounds how much older than the requested window start
// a base snapshot may be and still count for that window. It mirrors
// performancehistory's own BoundaryTolerance default (36h) so both places
// agree on what "close enough to the boundary" means.
const defaultMaxSnapshotAge = 36 * time.Hour

type Service struct {
	users          UserProvider
	ranked         RankedPerformanceProvider
	snapshots      SnapshotStore         // optional; enables trailing-window timeframes
	profiles       ProfilePublicProvider // optional; enriches rows with handle/tag/weights
	maxSnapshotAge time.Duration
	now            func() time.Time

	// maxLiveComputeUsers bounds the O(N) live ranking path: when no
	// projection generation has ever been promoted (cold start) and the
	// population exceeds this, reads degrade instead of valuing every user
	// per request. <= 0 disables the bound. See liveComputeAllowed.
	maxLiveComputeUsers int

	// ranking is the optional denormalized ranking projection (see
	// RankingStore). maxRankingAge is the trust-window floor and
	// maxRankingHardAge its absolute cap — see rankingFresh for how the
	// window adapts to the measured cycle duration between them.
	ranking           RankingStore
	maxRankingAge     time.Duration
	maxRankingHardAge time.Duration

	// refreshBatchSize bounds how many users RefreshCache revalues (one
	// CurrentRankedPerformance call each, which does a full portfolio
	// valuation) in a single call. Zero means unbounded — every call
	// revalues every user, the original behavior, which existing tests and
	// any deployment that hasn't opted in via SetRefreshBatchSize still get.
	// When set, RefreshCache advances refreshCursor through the (stably
	// sorted) user list on every call, so a periodic caller doing one full
	// scan every len(users)/refreshBatchSize calls still eventually
	// refreshes everyone — cost per call is bounded by refreshBatchSize
	// regardless of how many users the platform has, so a short ticker
	// interval no longer means unbounded work per tick.
	refreshBatchSize int
	// refreshParallelism bounds how many of a batch's users are revalued
	// concurrently (see SetRefreshParallelism). <=1 means sequential.
	refreshParallelism int
	refreshMu          sync.Mutex
	refreshCursor      int
	// refreshCursorID is the keyset cursor for the paged selection path (see
	// nextPagedBatch): the last user ID handed out, "" meaning "start of the
	// population". Only used when the attached UserProvider implements
	// PagedUserProvider and batching is enabled.
	refreshCursorID string
	// lastRefreshPromoted records whether the most recent RefreshCache call
	// promoted a new generation — read by LastRefreshPromotedGeneration so
	// the background worker can gate population-wide downstream rebuilds
	// (Explore) on actual new data instead of running them every tick.
	lastRefreshPromoted bool
	// cycleProgress counts users advanced through since the sliding window
	// last completed a full lap. It reaches len(users) exactly when every
	// eligible user has been attempted at least once since the previous
	// generation was promoted — see nextBatch's cycleComplete return value
	// and RefreshCache's CompleteCycle call.
	cycleProgress int
	// cycleRankingFailed latches true when any refreshRankingRows call
	// within the current lap fails to write a user's row. It's checked
	// (and cleared) alongside cycleComplete so a lap that dropped rows
	// never gets promoted — the building generation is left in place and
	// retried on the next lap instead. Guarded by refreshMu.
	cycleRankingFailed bool
}

// NewService wires a leaderboard Service around the ranked-performance provider.
func NewService(users UserProvider, ranked RankedPerformanceProvider) *Service {
	return &Service{
		users: users, ranked: ranked, maxSnapshotAge: defaultMaxSnapshotAge,
		maxRankingAge:       defaultMaxRankingAge,
		maxRankingHardAge:   defaultMaxRankingHardAge,
		refreshParallelism:  defaultRefreshParallelism,
		maxLiveComputeUsers: defaultMaxLiveComputeUsers,
		now:                 func() time.Time { return time.Now().UTC() },
	}
}

// SetMaxSnapshotAge overrides how much older than a window's start a base
// snapshot may be and still count for that window. A gap larger than this
// (a missed snapshot run, a paused-then-resumed account) makes the user
// ineligible for that timeframe rather than silently stretching e.g. "1W"
// into a much longer real span.
func (s *Service) SetMaxSnapshotAge(d time.Duration) {
	if d > 0 {
		s.maxSnapshotAge = d
	}
}

// SetRefreshBatchSize bounds how many users a single RefreshCache call
// revalues; see the Service.refreshBatchSize field comment. n <= 0 restores
// the unbounded default (revalue everyone on every call).
func (s *Service) SetRefreshBatchSize(n int) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.refreshBatchSize = n
	s.refreshCursor = 0
	s.refreshCursorID = ""
	s.cycleProgress = 0
}

// SetRefreshParallelism bounds how many users of a RefreshCache batch are
// revalued concurrently. n <= 1 keeps the sequential default. Higher values
// divide full-cycle wall time, keeping generation build time inside the
// projection trust window as the population grows.
func (s *Service) SetRefreshParallelism(n int) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	if n < 1 {
		n = 1
	}
	s.refreshParallelism = n
}

// LastRefreshPromotedGeneration reports whether the most recent RefreshCache
// call promoted a new ranking generation. The background worker uses it to
// run population-wide downstream rebuilds (the Explore projection) only when
// their input actually changed — once per completed cycle — instead of on
// every tick.
func (s *Service) LastRefreshPromotedGeneration() bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	return s.lastRefreshPromoted
}

// SetMaxLiveComputeUsers overrides the cold-start live-path population bound
// (see liveComputeAllowed). n <= 0 disables the bound entirely.
func (s *Service) SetMaxLiveComputeUsers(n int) {
	s.maxLiveComputeUsers = n
}

// SetSnapshotStore attaches the index-snapshot store that powers trailing
// timeframes. Without it, windowed leaderboards have no eligible rows.
func (s *Service) SetSnapshotStore(store SnapshotStore) {
	s.snapshots = store
}

// SetProfileProvider attaches the public-profile join used to enrich rows with
// handle, strategy tag, and public weights.
func (s *Service) SetProfileProvider(p ProfilePublicProvider) {
	s.profiles = p
}

// SetRankingStore attaches the optional denormalized ranking projection (see
// RankingStore). Without it, every read falls back to the pre-existing live
// computation — this call is additive, not required.
func (s *Service) SetRankingStore(r RankingStore) {
	s.ranking = r
}

// SetMaxRankingAge overrides the trust-window floor: a generation younger
// than this is always trusted. d <= 0 is ignored.
func (s *Service) SetMaxRankingAge(d time.Duration) {
	if d > 0 {
		s.maxRankingAge = d
	}
}

// SetMaxRankingHardAge overrides the absolute cap on the adaptive trust
// window (see rankingFresh). d <= 0 is ignored.
func (s *Service) SetMaxRankingHardAge(d time.Duration) {
	if d > 0 {
		s.maxRankingHardAge = d
	}
}

// Build computes the ranked, privacy-safe leaderboard:
//
//	list users -> summarize each -> keep only percentage + index ->
//	sort by gain_loss_percentage desc (ties: display_name asc) -> assign ranks.
//
// Failure policy (prototype): if one user's summary fails, that user is skipped
// and the leaderboard is still built from the rest. Later we may surface
// partial-error metadata (which users were skipped, and why) for internal
// monitoring — for now a failed user is silently omitted.
func (s *Service) Build(ctx context.Context) ([]LeaderboardEntry, error) {
	// Primary path: the denormalized ranking projection — a single query for
	// exactly the rows shown, metadata joined only for those rows. Served
	// even when stale (see useRanking): a stale board is a strictly better
	// failure mode than an O(N) live computation per request.
	if entries, ok := s.buildFromRanking(ctx, TimeframeAll); ok {
		return entries, nil
	}
	res, err := s.BuildResult(ctx)
	if errors.Is(err, ErrRankingUnavailable) {
		// Cold start at scale: the first cycle hasn't completed and live
		// computation is disallowed. An empty board (which clients already
		// handle) is the degradation, never an error page.
		slog.Warn("leaderboard_live_compute_declined", "path", "build")
		return []LeaderboardEntry{}, nil
	}
	return res.Entries, err
}

// BuildTimeframe ranks users over the given window. ALL is identical to
// Build; trailing windows prefer the ranking projection (even stale — see
// useRanking), and are computed live from index snapshots only on cold start.
// A user with no snapshot old enough is excluded from that window.
func (s *Service) BuildTimeframe(ctx context.Context, tf Timeframe) ([]LeaderboardEntry, error) {
	if tf == TimeframeAll {
		return s.Build(ctx)
	}
	if entries, ok := s.buildFromRanking(ctx, tf); ok {
		return entries, nil
	}
	rows, _, err := s.rankRows(ctx, tf)
	if errors.Is(err, ErrRankingUnavailable) {
		slog.Warn("leaderboard_live_compute_declined", "path", "build_timeframe", "timeframe", tf)
		return []LeaderboardEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	return entriesOf(rows), nil
}

// rankingServable reports whether the projection can serve reads at all
// (servable: an activated generation exists and the state read succeeded)
// and whether that generation is within its freshness window (fresh). age is
// meaningful only when servable.
//
// The freshness window ADAPTS to the measured build-cycle duration: it is
// max(maxRankingAge, cycleTrustFactor x last cycle duration), capped at
// maxRankingHardAge. A fixed window plus a population-proportional cycle time
// would otherwise guarantee that, past some user count, every cycle ends with
// a stretch where the projection reads as stale — and at scale the response
// to staleness must never be O(N) live computation (see useRanking).
//
// Every timeframe is built together within the same refresh cycle (see
// RefreshCache/nextBatch), so one activation age is shared by all of them.
func (s *Service) rankingServable(ctx context.Context) (servable, fresh bool, age time.Duration) {
	if s.ranking == nil {
		return false, false, 0
	}
	at, found, err := s.ranking.ActiveGenerationAge(ctx)
	if err != nil || !found {
		return false, false, 0
	}
	age = s.now().Sub(at)
	window := s.maxRankingAge
	if reporter, ok := s.ranking.(CycleDurationReporter); ok {
		if last, known, err := reporter.LastCycleDuration(ctx); err == nil && known {
			if adaptive := time.Duration(cycleTrustFactor) * last; adaptive > window {
				window = adaptive
			}
		}
	}
	if s.maxRankingHardAge > 0 && window > s.maxRankingHardAge {
		window = s.maxRankingHardAge
	}
	return true, age <= window, age
}

// useRanking is the single serve-or-fall-back decision for projection reads:
// serve whenever a promoted generation exists, warn-logging (an alerting
// signal — see docs/operations/alerting.md) when it is past its freshness
// window. Staleness deliberately does NOT flip reads to the live path: a
// stale projection stays O(page size) per request, while the live path is
// O(population) — falling back under load is the feedback loop that melts
// the database exactly when the refresh pipeline most needs headroom. The
// live path remains only for a true cold start (no generation ever
// promoted) or a ranking-state read error.
func (s *Service) useRanking(ctx context.Context) bool {
	servable, fresh, age := s.rankingServable(ctx)
	if servable && !fresh {
		slog.Warn("leaderboard_projection_stale_served", "age", age.String())
	}
	return servable
}

// buildFromRanking assembles the board for tf directly from the ranking
// projection. ok=false means "use the next fallback path" (no ranking store,
// never promoted, or a query error — NOT mere staleness, see useRanking).
func (s *Service) buildFromRanking(ctx context.Context, tf Timeframe) ([]LeaderboardEntry, bool) {
	if !s.useRanking(ctx) {
		return nil, false
	}
	rows, err := s.ranking.TopPage(ctx, tf, maxLeaderboardSize)
	if err != nil {
		return nil, false
	}
	s.enrichPage(ctx, rows)
	return entriesOf(rows), true
}

// enrichPage joins public profile data onto every row in a page, batching the
// lookup into a single query when the attached ProfilePublicProvider supports
// it (see ProfilePublicBatchProvider), else falling back to one lookup per row.
func (s *Service) enrichPage(ctx context.Context, rows []rankedRow) {
	if s.profiles == nil || len(rows) == 0 {
		return
	}
	if batch, ok := s.profiles.(ProfilePublicBatchProvider); ok {
		ids := make([]string, len(rows))
		for i, r := range rows {
			ids[i] = r.userID
		}
		if infos, err := batch.PublicInfoBatch(ctx, ids); err == nil {
			for i := range rows {
				info, found := infos[rows[i].userID]
				if !found || !info.IsPublic {
					continue
				}
				rows[i].entry.Handle = info.Handle
				rows[i].entry.StrategyTag = info.StrategyTag
				if info.ShowWeights {
					rows[i].entry.PublicWeights = info.Weights
				}
			}
			return
		}
	}
	for i := range rows {
		s.enrich(ctx, rows[i].userID, &rows[i].entry)
	}
}

// enrich joins public profile data onto a row. Private profiles (or users with
// no profile) stay anonymous: no handle, tag, or weights. Weights additionally
// require show_public_weights. Best-effort: a lookup error leaves the row bare.
func (s *Service) enrich(ctx context.Context, userID string, e *LeaderboardEntry) {
	if s.profiles == nil {
		return
	}
	info, ok, err := s.profiles.PublicInfo(ctx, userID)
	if err != nil || !ok || !info.IsPublic {
		return
	}
	e.Handle = info.Handle
	e.StrategyTag = info.StrategyTag
	if info.ShowWeights {
		e.PublicWeights = info.Weights
	}
}

// rankedEntry builds a row, populating both the ranked_* fields and their
// backward-compatible gain_loss/portfolio aliases with the same values.
func rankedEntry(rank int, displayName, avatarKey string, returnPct money.Ratio, index money.IndexValue) LeaderboardEntry {
	return LeaderboardEntry{
		Rank:                   rank,
		DisplayName:            displayName,
		AvatarKey:              avatarKey,
		RankedReturnPercentage: returnPct,
		RankedIndex:            index,
		GainLossPercentage:     returnPct,
		PortfolioIndex:         index,
	}
}

// liveComputeAllowed reports whether the O(N) live ranking path may run. The
// live path values every rankable user per request; on a cold start at scale
// (no projection generation promoted yet), letting requests do that would
// take down the database — and with it the refresh pipeline whose completion
// is the only way out. Populations above maxLiveComputeUsers therefore
// degrade softly until the first cycle promotes. The probe uses one bounded
// keyset page when available; providers without pagination (in-memory mode,
// tests) are dev-scale by construction and always allowed.
func (s *Service) liveComputeAllowed(ctx context.Context) bool {
	if s.maxLiveComputeUsers <= 0 {
		return true
	}
	paged, ok := s.users.(PagedUserProvider)
	if !ok {
		return true
	}
	page, err := paged.ListRankableUsersPage(ctx, "", s.maxLiveComputeUsers+1)
	if err != nil {
		// Let the live path surface the listing error itself rather than
		// masking it behind a degraded-but-successful response.
		return true
	}
	return len(page) <= s.maxLiveComputeUsers
}

// RefreshCache (the name predates the ranking projection) revalues one batch
// of users per call and writes their rows into the projection's building
// generation. Canonical history is owned by the independent ranked snapshot
// worker. Without a ranking store attached it is a no-op.
//
// nextBatch selects the slice of users this call should revalue (the unpaged
// path; see nextPagedBatch for the keyset path). allProcessed reports whether
// THIS SINGLE call touched every known user (true whenever refreshBatchSize
// is unset/non-positive, or happens to cover the whole list). cycleComplete
// instead reports whether the CUMULATIVE sliding window has now covered every
// user at least once since the last completed lap — see the cycleProgress
// field doc — which is what gates promoting a new ranking-projection
// generation in RefreshCache. Selection is a stable, sorted-by-ID sliding
// window that advances across calls, so a caller invoking RefreshCache on a
// fixed interval eventually covers every user over
// ceil(len(users)/refreshBatchSize) calls, with each individual call doing
// bounded work.
func (s *Service) nextBatch(users []auth.RankableUser) (batch []auth.RankableUser, allProcessed bool, cycleComplete bool) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	if s.refreshBatchSize <= 0 || s.refreshBatchSize >= len(users) {
		return users, true, true
	}
	sorted := make([]auth.RankableUser, len(users))
	copy(sorted, users)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	n := len(sorted)
	if s.refreshCursor >= n {
		s.refreshCursor = 0
	}
	start := s.refreshCursor
	size := s.refreshBatchSize
	if size > n {
		size = n
	}
	batch = make([]auth.RankableUser, 0, size)
	for i := 0; i < size; i++ {
		batch = append(batch, sorted[(start+i)%n])
	}
	s.refreshCursor = (start + size) % n
	s.cycleProgress += size
	if s.cycleProgress >= n {
		cycleComplete = true
		s.cycleProgress = 0
	}
	return batch, false, cycleComplete
}

// markCycleRankingFailure latches a ranking-row write failure for the
// current lap; consumeCycleRankingFailure reads and clears it. Both share
// refreshMu with nextBatch since they guard the same per-lap state.
func (s *Service) markCycleRankingFailure() {
	s.refreshMu.Lock()
	s.cycleRankingFailed = true
	s.refreshMu.Unlock()
}

func (s *Service) consumeCycleRankingFailure() bool {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	failed := s.cycleRankingFailed
	s.cycleRankingFailed = false
	return failed
}

// refreshRankingRows upserts userID's ranking-projection row for every
// timeframe it's currently eligible for, and deletes the row for any
// timeframe it's no longer eligible for (e.g. a snapshot gap that just grew
// past maxSnapshotAge). rp is the already-fetched (non-paused) ranked
// performance from this refresh tick — no extra valuation call is made.
//
// The returned error aggregates every Upsert/Delete failure (via
// errors.Join) so the caller can withhold CompleteCycle promotion for a lap
// that wrote incomplete rows, rather than promoting a building generation
// that silently dropped rows for this user.
func (s *Service) refreshRankingRows(ctx context.Context, userID string, rp RankedPerformance) error {
	if s.ranking == nil {
		return nil
	}
	var errs []error
	for _, tf := range allTimeframes {
		retPct, idx, ok := s.timeframeReturn(ctx, userID, rp, tf)
		if !ok {
			if err := s.ranking.Delete(ctx, tf, userID); err != nil {
				slog.Warn("leaderboard_ranking_update_failed",
					"operation", "delete_ineligible", "timeframe", tf, "user_id", userID, "error", err)
				errs = append(errs, err)
			}
			continue
		}
		if err := s.ranking.Upsert(ctx, tf, userID, idx, retPct, rp.TrackingStartedAt); err != nil {
			slog.Warn("leaderboard_ranking_update_failed",
				"operation", "upsert", "timeframe", tf, "user_id", userID, "error", err)
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteRankingRows removes userID's ranking-projection rows for the given
// timeframes (used when a user becomes paused — excluded from ranking
// entirely).
func (s *Service) deleteRankingRows(ctx context.Context, userID string, timeframes []Timeframe) {
	if s.ranking == nil {
		return
	}
	for _, tf := range timeframes {
		if err := s.ranking.Delete(ctx, tf, userID); err != nil {
			slog.Warn("leaderboard_ranking_update_failed",
				"operation", "delete_paused", "timeframe", tf, "user_id", userID, "error", err)
		}
	}
}

// nextPagedBatch selects this call's batch with a single keyset page query —
// O(batch size) per tick, never loading the whole population — advancing the
// in-memory cursor across calls. cycleComplete=true means the page reached
// the end of the population: every user has now been attempted since the
// cursor last wrapped, which is the same promotion precondition nextBatch's
// cycleProgress tracks for the unpaged path.
func (s *Service) nextPagedBatch(ctx context.Context, paged PagedUserProvider, size int) (batch []auth.RankableUser, cycleComplete bool, err error) {
	s.refreshMu.Lock()
	cursor := s.refreshCursorID
	s.refreshMu.Unlock()

	batch, err = paged.ListRankableUsersPage(ctx, cursor, size)
	if err != nil {
		return nil, false, err
	}
	next := cursor
	if len(batch) > 0 {
		next = batch[len(batch)-1].ID
	}
	if len(batch) < size {
		// End of the population. An empty table with a fresh cursor is not a
		// completed lap — promoting then would publish an empty generation.
		cycleComplete = cursor != "" || len(batch) > 0
		next = ""
	}
	s.refreshMu.Lock()
	s.refreshCursorID = next
	s.refreshMu.Unlock()
	return batch, cycleComplete, nil
}

// refreshTally aggregates the per-user outcomes of one RefreshCache batch.
// mu guards every field because processBatch may run refreshUser calls
// concurrently.
type refreshTally struct {
	mu            sync.Mutex
	skipped       int
	upserted      int
	pausedRemoved int
	firstErr      error
}

func (t *refreshTally) noteErr(err error) {
	t.mu.Lock()
	if t.firstErr == nil {
		t.firstErr = err
	}
	t.mu.Unlock()
}

// refreshUser revalues a single user and reconciles their ranking-projection
// rows — the per-user unit of work RefreshCache used to inline, factored out
// so processBatch can run it under bounded concurrency.
func (s *Service) refreshUser(ctx context.Context, u auth.RankableUser, t *refreshTally) {
	rp, err := s.ranked.CurrentRankedPerformance(ctx, u.ID)
	if err != nil {
		t.mu.Lock()
		t.skipped++
		t.mu.Unlock()
		return
	}
	if rp.Paused {
		// Explicitly DELETE rather than merely skip: a user who emptied their
		// portfolio must drop out of the next promoted generation instead of
		// lingering with their last projected row.
		s.deleteRankingRows(ctx, u.ID, allTimeframes)
		t.mu.Lock()
		t.pausedRemoved++
		t.mu.Unlock()
		return
	}
	if err := s.refreshRankingRows(ctx, u.ID, rp); err != nil {
		s.markCycleRankingFailure()
		t.noteErr(err)
		return
	}
	t.mu.Lock()
	t.upserted++
	t.mu.Unlock()
}

// processBatch runs refreshUser over the batch, fanning out across up to
// refreshParallelism goroutines. Parallelism divides the wall-clock time of a
// full refresh lap, which is what keeps generation build time inside the
// projection trust window as the population grows.
func (s *Service) processBatch(ctx context.Context, batch []auth.RankableUser) *refreshTally {
	t := &refreshTally{}
	s.refreshMu.Lock()
	par := s.refreshParallelism
	s.refreshMu.Unlock()
	if par <= 1 || len(batch) <= 1 {
		for _, u := range batch {
			s.refreshUser(ctx, u, t)
		}
		return t
	}
	sem := make(chan struct{}, par)
	var wg sync.WaitGroup
	for _, u := range batch {
		wg.Add(1)
		sem <- struct{}{}
		go func(u auth.RankableUser) {
			defer wg.Done()
			defer func() { <-sem }()
			s.refreshUser(ctx, u, t)
		}(u)
	}
	wg.Wait()
	return t
}

func (s *Service) RefreshCache(ctx context.Context) (int, error) {
	if s.ranking == nil {
		return 0, nil
	}
	s.refreshMu.Lock()
	s.lastRefreshPromoted = false
	batchSize := s.refreshBatchSize
	s.refreshMu.Unlock()

	var (
		batch         []auth.RankableUser
		cycleComplete bool
	)
	if paged, ok := s.users.(PagedUserProvider); ok && batchSize > 0 {
		// Paged selection: one bounded page query per tick — the full
		// population is never listed. Deleted users need no reconciliation
		// here: their projection rows drop out via ON DELETE CASCADE and
		// the reads' join on non-deleted users.
		var err error
		batch, cycleComplete, err = s.nextPagedBatch(ctx, paged, batchSize)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", ErrListUsers, err)
		}
		slog.Info("leaderboard_ranking_refresh_started", "batch", len(batch), "paged", true)
	} else {
		users, err := s.users.ListRankableUsers(ctx)
		if err != nil {
			return 0, fmt.Errorf("%w: %v", ErrListUsers, err)
		}
		batch, _, cycleComplete = s.nextBatch(users)
		slog.Info("leaderboard_ranking_refresh_started", "users", len(users), "batch", len(batch))
	}

	t := s.processBatch(ctx, batch)

	// A full lap over every eligible user just finished: the building
	// generation has now been attempted for everyone, so it's safe to
	// promote it to active. A user whose valuation kept failing simply has
	// no row in the new generation rather than blocking promotion — see
	// RankingStore.CompleteCycle's doc for why this must only happen here,
	// after cycleComplete, and never mid-lap.
	//
	// But a user whose valuation succeeded and whose ranking row write
	// failed is different: promoting now would publish a generation
	// missing that user's row even though nothing marked them ineligible.
	// consumeCycleRankingFailure reports (and clears) whether any such
	// write failed anywhere in this lap — possibly across earlier batch
	// calls, since a lap can span several RefreshCache invocations — and
	// promotion is withheld for the whole lap when it has. The building
	// generation is left in place (nextBatch already reset cycleProgress)
	// and gets retried from scratch on the next lap.
	if s.ranking != nil && cycleComplete {
		if rankingFailedThisLap := s.consumeCycleRankingFailure(); rankingFailedThisLap {
			slog.Warn("leaderboard_ranking_cycle_complete_skipped",
				"reason", "ranking_row_write_failed_this_lap")
		} else if err := s.ranking.CompleteCycle(ctx); err != nil {
			slog.Warn("leaderboard_ranking_cycle_complete_failed", "error", err)
			t.noteErr(err)
		} else {
			s.refreshMu.Lock()
			s.lastRefreshPromoted = true
			s.refreshMu.Unlock()
		}
	}
	slog.Info("leaderboard_ranking_refresh_completed",
		"upserted", t.upserted,
		"paused_removed", t.pausedRemoved,
		"valuation_skipped", t.skipped,
		"promoted", cycleComplete && s.LastRefreshPromotedGeneration(),
	)
	if t.firstErr != nil {
		slog.Error("leaderboard_ranking_refresh_failed", "error", t.firstErr)
	}
	return t.skipped, t.firstErr
}

// Result carries the ranked entries plus internal metadata about how many users
// were skipped. The public handler returns only Entries; SkippedCount is for
// internal monitoring (and, later, an admin endpoint).
type Result struct {
	Entries      []LeaderboardEntry
	SkippedCount int
}

type UserRanking struct {
	UserID                 string
	Rank                   int
	RankedReturnPercentage money.Ratio
	RankedIndex            money.IndexValue
}

// UserRankings returns every ranked user's row for tf — the full per-user
// join Explore uses to enrich its discovery feed with timeframe ranks. It
// prefers the ranking projection when fresh (one bounded query instead of one
// valuation per user); the live path remains the fallback for identical
// results when the projection is absent, stale, or errors.
func (s *Service) UserRankings(ctx context.Context, tf Timeframe) ([]UserRanking, error) {
	if rows, ok := s.allRankingRows(ctx, tf); ok {
		return toUserRankings(rows), nil
	}
	rows, _, err := s.rankRows(ctx, tf)
	if err != nil {
		return nil, err
	}
	return toUserRankings(rows), nil
}

func toUserRankings(rows []rankedRow) []UserRanking {
	out := make([]UserRanking, 0, len(rows))
	for _, row := range rows {
		out = append(out, UserRanking{
			UserID:                 row.userID,
			Rank:                   row.entry.Rank,
			RankedReturnPercentage: row.entry.RankedReturnPercentage,
			RankedIndex:            row.entry.RankedIndex,
		})
	}
	return out
}

// allRankingRows returns every row for tf from the ranking projection when
// servable (even stale — see useRanking). ok=false means "use the live path"
// (no ranking store, never promoted, or a query error) — same fallback
// contract as buildFromRanking.
func (s *Service) allRankingRows(ctx context.Context, tf Timeframe) ([]rankedRow, bool) {
	if !s.useRanking(ctx) {
		return nil, false
	}
	n, err := s.ranking.Count(ctx, tf)
	if err != nil || n == 0 {
		return nil, false
	}
	rows, err := s.ranking.TopPage(ctx, tf, n)
	if err != nil {
		return nil, false
	}
	return rows, true
}

// GetUserRank returns the exact global rank for userID, or 0 when the user's
// portfolio cannot be ranked. Internal ids are used only for matching and are
// never added to the public leaderboard response.
func (s *Service) GetUserRank(ctx context.Context, userID string) (int, error) {
	// The requested user's current lifecycle state is cheap to validate and must
	// take precedence over any projected membership.
	current, currentErr := s.ranked.CurrentRankedPerformance(ctx, userID)
	if currentErr == nil && current.Paused {
		return 0, nil
	}
	// Prefer the denormalized ranking projection (even stale — see
	// useRanking): a single indexed query instead of scanning every user.
	// found=false is treated as "unknown" (could be a brand-new user not yet
	// reached by a refresh cycle), not as an authoritative "unranked" — fall
	// through instead.
	if currentErr == nil && s.useRanking(ctx) {
		if rank, _, _, found, err := s.ranking.RankOf(ctx, TimeframeAll, userID); err == nil && found {
			return rank, nil
		}
	}
	// The live path values every rankable user; at scale that is only
	// tolerable for the rare not-yet-projected user, and never on a large
	// cold start. Degrade to "unranked" — the same answer a fresh projection
	// gives a user who hasn't been cycled in yet.
	if !s.liveComputeAllowed(ctx) {
		slog.Warn("leaderboard_live_compute_declined", "path", "user_rank")
		return 0, nil
	}
	users, err := s.users.ListRankableUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrListUsers, err)
	}
	type rankedUser struct {
		id          string
		displayName string
		returnPct   money.Ratio
	}
	rows := make([]rankedUser, 0, len(users))
	for _, user := range users {
		rp, err := s.ranked.CurrentRankedPerformance(ctx, user.ID)
		if err == nil && !rp.Paused {
			rows = append(rows, rankedUser{id: user.ID, displayName: user.DisplayName, returnPct: rp.RankedReturnPercentage})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if cmp := rows[i].returnPct.Cmp(rows[j].returnPct); cmp != 0 {
			return cmp > 0
		}
		if rows[i].displayName != rows[j].displayName {
			return rows[i].displayName < rows[j].displayName
		}
		return rows[i].id < rows[j].id
	})
	for i, row := range rows {
		if row.id == userID {
			return i + 1, nil
		}
	}
	return 0, nil
}

// BuildResult is the all-time board with skip metadata. With symbol validation
// in place, SkippedCount should normally be 0; a non-zero value indicates a real
// systemic problem worth investigating.
func (s *Service) BuildResult(ctx context.Context) (Result, error) {
	rows, skipped, err := s.rankRows(ctx, TimeframeAll)
	if err != nil {
		return Result{}, err
	}
	return Result{Entries: entriesOf(rows), SkippedCount: skipped}, nil
}

// Standing is one user's position on a timeframe board. Ranked=false means the
// user has no rankable portfolio (and so does not appear on the board).
type Standing struct {
	Timeframe              Timeframe
	Rank                   int
	PreviousRank           *int
	RankDelta              *int
	BestRank               *int
	ParticipantCount       int
	Percentile             float64
	RankedReturnPercentage money.Ratio
	RankedIndex            money.IndexValue
	Ranked                 bool
	Paused                 bool
	Reason                 string
	NextMilestone          *Milestone
}

// Milestone is the next reachable rank target for the caller. All values are
// rank/percentage-only and safe for the private standing response.
type Milestone struct {
	Label               string
	TargetRank          int
	RankGap             int
	ReturnGapPercentage money.Ratio
}

// UserStanding computes the caller's rank within the timeframe board, plus the
// total number of ranked participants. Prefers the denormalized ranking
// projection when fresh (a couple of indexed queries instead of building the
// whole board); falls back to the live path for consistent behavior on any
// timeframe otherwise.
func (s *Service) UserStanding(ctx context.Context, userID string, tf Timeframe) (Standing, error) {
	if st, ok := s.userStandingFromRanking(ctx, userID, tf); ok {
		return st, nil
	}
	rows, _, err := s.rankRows(ctx, tf)
	if errors.Is(err, ErrRankingUnavailable) {
		slog.Warn("leaderboard_live_compute_declined", "path", "user_standing", "timeframe", tf)
		return Standing{
			Timeframe: tf,
			Reason:    "Global rankings are being prepared. Check back soon.",
		}, nil
	}
	if err != nil {
		return Standing{}, err
	}
	st := Standing{Timeframe: tf, ParticipantCount: len(rows)}
	for _, r := range rows {
		if r.userID == userID {
			st.Rank = r.entry.Rank
			st.RankedReturnPercentage = r.entry.RankedReturnPercentage
			st.RankedIndex = r.entry.RankedIndex
			st.Ranked = true
			best := r.entry.Rank
			st.BestRank = &best
			st.Percentile = percentile(r.entry.Rank, len(rows))
			st.NextMilestone = nextMilestone(rows, r.entry.Rank, r.entry.RankedReturnPercentage)
			break
		}
	}
	if !st.Ranked {
		// Distinguish a paused portfolio (had ranked history, now empty — index
		// preserved and excluded from ranking) from a user who has never ranked.
		if rp, err := s.ranked.CurrentRankedPerformance(ctx, userID); err == nil && rp.Paused {
			st.Paused = true
			st.RankedIndex = rp.RankedIndex
			st.RankedReturnPercentage = rp.RankedReturnPercentage
			st.Reason = "Ranked tracking is paused. Your accumulated index is preserved — add a position to resume from it."
		} else if _, windowed := tf.window(); windowed {
			st.Reason = "Not enough ranked history for this timeframe yet."
		} else {
			st.Reason = "Create a strategy baseline to enter the leaderboard."
		}
	}
	return st, nil
}

// rankedRow pairs an internal user id with its public entry. The id is used for
// matching/snapshots only and never serialized.
type rankedRow struct {
	userID      string
	entry       LeaderboardEntry
	returnPct   money.Ratio
	rankedIndex money.IndexValue
}

// timeframeReturn computes the return percentage and index for tf given an
// already-fetched (non-paused) rp. For ALL it's just rp's own since-baseline
// values. For a windowed timeframe it requires an eligible snapshot at or
// before now-window: ok=false means userID is excluded from tf rather than
// mis-ranked (no snapshot, a pre-epoch snapshot, or a gap wider than
// maxSnapshotAge — see SnapshotStore.IndexAtOrBefore's doc).
func (s *Service) timeframeReturn(ctx context.Context, userID string, rp RankedPerformance, tf Timeframe) (retPct money.Ratio, idx money.IndexValue, ok bool) {
	window, windowed := tf.window()
	if !windowed {
		return rp.RankedReturnPercentage, rp.RankedIndex, true
	}
	if s.snapshots == nil {
		return money.ZeroRatio(), money.ZeroIndexValue(), false
	}
	cutoff := s.now().Add(-window)
	// Only post-epoch snapshots are eligible; legacy pre-epoch history is
	// ignored so a windowed return can never use a manipulable old index.
	base, capturedAt, found, err := s.snapshots.IndexAtOrBefore(ctx, userID, cutoff, rp.TrackingStartedAt)
	if err != nil {
		log.Printf("leaderboard: skipping user %s due to snapshot error: %v", userID, err)
		return money.ZeroRatio(), money.ZeroIndexValue(), false
	}
	if !found || base.Sign() <= 0 {
		return money.ZeroRatio(), money.ZeroIndexValue(), false
	}
	// A base snapshot far older than the window's own start means a gap in
	// recorded history (a missed snapshot run, a paused-then-resumed account) —
	// using it would silently stretch e.g. "1W" into a much longer real span,
	// so the user is excluded from this timeframe instead of mis-measured.
	if cutoff.Sub(capturedAt) > s.maxSnapshotAge {
		return money.ZeroRatio(), money.ZeroIndexValue(), false
	}
	retPct, err = performancehistory.TimeframeReturnRatio(base, rp.RankedIndex)
	if err != nil {
		return money.ZeroRatio(), money.ZeroIndexValue(), false
	}
	factor, err := rp.RankedIndex.DivExact(base, money.ScaleIndex+money.ScaleWeight)
	if err != nil {
		return money.ZeroRatio(), money.ZeroIndexValue(), false
	}
	idx = money.MustIndexValue("100").MulRatio(factor)
	return retPct, idx, true
}

// rankRows is the single live-ranking core: summarize each user, compute the
// timeframe return (since-baseline for ALL, else current-index vs the snapshot
// at now-window), enrich with public profile data, then sort and assign ranks.
// Windowed rows require old-enough snapshots; otherwise the user is excluded
// from that timeframe rather than being ranked on all-time performance.
func (s *Service) rankRows(ctx context.Context, tf Timeframe) ([]rankedRow, int, error) {
	if !s.liveComputeAllowed(ctx) {
		return nil, 0, ErrRankingUnavailable
	}
	users, err := s.users.ListRankableUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrListUsers, err)
	}

	rows := make([]rankedRow, 0, len(users))
	skipped := 0
	for _, u := range users {
		rp, err := s.ranked.CurrentRankedPerformance(ctx, u.ID)
		if err != nil {
			skipped++
			log.Printf("leaderboard: skipping user %s due to ranked-performance error: %v", u.ID, err)
			continue
		}
		if rp.Paused {
			continue // empty portfolio: preserved but excluded from active ranking
		}
		retPct, idx, ok := s.timeframeReturn(ctx, u.ID, rp, tf)
		if !ok {
			continue
		}
		e := rankedEntry(0, u.DisplayName, u.AvatarKey, retPct, idx)
		s.enrich(ctx, u.ID, &e)
		rows = append(rows, rankedRow{userID: u.ID, entry: e, returnPct: retPct, rankedIndex: idx})
	}

	sortRankedRows(rows)
	return rows, skipped, nil
}

// sortRankedRows is the one canonical global ranking rule. Both cached and
// live reads call it after loading exact persistent ranked values.
func sortRankedRows(rows []rankedRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].entry, rows[j].entry
		if cmp := rows[i].returnPct.Cmp(rows[j].returnPct); cmp != 0 {
			return cmp > 0
		}
		if a.DisplayName != b.DisplayName {
			return a.DisplayName < b.DisplayName
		}
		return rows[i].userID < rows[j].userID
	})
	for i := range rows {
		rows[i].entry.Rank = i + 1
	}
}

func entriesOf(rows []rankedRow) []LeaderboardEntry {
	entries := make([]LeaderboardEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, r.entry)
	}
	return entries
}

func percentile(rank, participants int) float64 {
	if rank <= 0 || participants <= 0 {
		return 0
	}
	return round2(float64(participants-rank+1) / float64(participants) * 100)
}

// milestoneTarget picks the next reachable rank tier above rank.
func milestoneTarget(rank int) (targetRank int, label string) {
	switch {
	case rank > 100:
		return 100, "Top 100"
	case rank > 25:
		return 25, "Top 25"
	case rank > 10:
		return 10, "Top 10"
	default:
		return 1, "#1"
	}
}

func nextMilestone(rows []rankedRow, rank int, returnPct money.Ratio) *Milestone {
	if rank <= 1 {
		return nil
	}
	targetRank, label := milestoneTarget(rank)
	gap := money.ZeroRatio()
	if targetRank > 0 && targetRank <= len(rows) {
		gap = rows[targetRank-1].entry.RankedReturnPercentage.Sub(returnPct)
		if gap.Sign() < 0 {
			gap = money.ZeroRatio()
		}
	}
	return &Milestone{
		Label:               label,
		TargetRank:          targetRank,
		RankGap:             rank - targetRank,
		ReturnGapPercentage: gap,
	}
}

// nextMilestoneFromRanking is nextMilestone's counterpart for the ranking
// projection: instead of indexing into an in-memory rows slice, it fetches the
// exact value at the target rank with a single query.
func (s *Service) nextMilestoneFromRanking(ctx context.Context, tf Timeframe, rank int, returnPct money.Ratio, total int) *Milestone {
	if rank <= 1 {
		return nil
	}
	targetRank, label := milestoneTarget(rank)
	gap := money.ZeroRatio()
	if targetRank > 0 && targetRank <= total {
		if targetPct, found, err := s.ranking.ValueAtRank(ctx, tf, targetRank); err == nil && found {
			gap = targetPct.Sub(returnPct)
			if gap.Sign() < 0 {
				gap = money.ZeroRatio()
			}
		}
	}
	return &Milestone{
		Label:               label,
		TargetRank:          targetRank,
		RankGap:             rank - targetRank,
		ReturnGapPercentage: gap,
	}
}

// userStandingFromRanking is UserStanding's ranking-projection-preferred path.
// ok=false means "fall back to the live path" — no ranking store, stale data,
// a query error, or userID simply has no row yet (ambiguous between
// genuinely-unranked and not-yet-refreshed, so it's always safe to defer to
// the live path rather than guess).
func (s *Service) userStandingFromRanking(ctx context.Context, userID string, tf Timeframe) (Standing, bool) {
	if !s.useRanking(ctx) {
		return Standing{}, false
	}
	total, err := s.ranking.Count(ctx, tf)
	if err != nil {
		return Standing{}, false
	}
	rank, idx, retPct, found, err := s.ranking.RankOf(ctx, tf, userID)
	if err != nil || !found {
		return Standing{}, false
	}
	best := rank
	st := Standing{
		Timeframe: tf, ParticipantCount: total,
		Rank: rank, RankedReturnPercentage: retPct, RankedIndex: idx,
		Ranked: true, BestRank: &best,
	}
	st.Percentile = percentile(rank, total)
	st.NextMilestone = s.nextMilestoneFromRanking(ctx, tf, rank, retPct, total)
	return st, true
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
