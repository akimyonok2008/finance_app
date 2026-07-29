package leaderboard

import (
	"context"
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

// maxLeaderboardSize caps how many entries are served from the cache path.
const maxLeaderboardSize = 100

// RankingStore is the optional denormalized ranking projection (a Postgres
// table populated by RefreshCache from the same per-user valuation it already
// pays for). Reading it back lets board, per-user-rank, and windowed-standing
// queries do O(page size) work with rank computed by a Postgres window
// function, instead of enumerating every user and sorting in application
// memory. It is a pure optimization layer: any error, staleness, or absence
// falls back to the existing live computation, exactly like LeaderboardCache.
type RankingStore interface {
	Upsert(ctx context.Context, tf Timeframe, userID string, idx money.IndexValue, retPct money.Ratio, trackingStartedAt time.Time) error
	Delete(ctx context.Context, tf Timeframe, userID string) error
	// TopPage returns up to limit rows for tf, ranked by a window function and
	// joined to display metadata in a single round trip.
	TopPage(ctx context.Context, tf Timeframe, limit int) ([]rankedRow, error)
	// RankOf returns userID's exact rank/index/return for tf. found=false means
	// userID has no row for tf — this can mean genuinely unranked OR simply
	// "not yet reached by a refresh cycle", so callers must treat it as
	// "unknown" and fall back rather than as an authoritative negative.
	RankOf(ctx context.Context, tf Timeframe, userID string) (rank int, idx money.IndexValue, retPct money.Ratio, found bool, err error)
	// ValueAtRank returns the return percentage of the row at an exact 1-based
	// rank, for milestone-gap calculations.
	ValueAtRank(ctx context.Context, tf Timeframe, rank int) (retPct money.Ratio, found bool, err error)
	Count(ctx context.Context, tf Timeframe) (int, error)
	// OldestComputedAt reports the least-recently-refreshed row's timestamp for
	// tf. found=false means tf has no rows yet (never refreshed).
	OldestComputedAt(ctx context.Context, tf Timeframe) (at time.Time, found bool, err error)
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

// defaultMaxRankingAge bounds how stale the ranking projection may be and
// still be trusted; older than this, callers fall back to live computation.
const defaultMaxRankingAge = 30 * time.Minute

// Service builds the anonymous leaderboard. With a cache attached it serves the
// precomputed Redis ranking; otherwise (or when the cache is empty or failing)
// it falls back to live calculation, so the cache is never a single point of
// failure.
// defaultMaxSnapshotAge bounds how much older than the requested window start
// a base snapshot may be and still count for that window. It mirrors
// performancehistory's own BoundaryTolerance default (36h) so both places
// agree on what "close enough to the boundary" means.
const defaultMaxSnapshotAge = 36 * time.Hour

type Service struct {
	users          UserProvider
	ranked         RankedPerformanceProvider
	cache          LeaderboardCache      // optional
	snapshots      SnapshotStore         // optional; enables trailing-window timeframes
	profiles       ProfilePublicProvider // optional; enriches rows with handle/tag/weights
	maxSnapshotAge time.Duration
	now            func() time.Time

	// ranking is the optional denormalized ranking projection (see
	// RankingStore). maxRankingAge bounds how stale it may be and still be
	// preferred over live computation.
	ranking       RankingStore
	maxRankingAge time.Duration

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
	refreshMu        sync.Mutex
	refreshCursor    int
}

// NewService wires a leaderboard Service around the ranked-performance provider.
func NewService(users UserProvider, ranked RankedPerformanceProvider) *Service {
	return &Service{
		users: users, ranked: ranked, maxSnapshotAge: defaultMaxSnapshotAge,
		maxRankingAge: defaultMaxRankingAge,
		now:           func() time.Time { return time.Now().UTC() },
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
}

// SetCache attaches an optional ranking cache (Redis in production).
func (s *Service) SetCache(cache LeaderboardCache) {
	s.cache = cache
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

// SetMaxRankingAge overrides how stale the ranking projection may be and still
// be preferred over live computation. d <= 0 is ignored.
func (s *Service) SetMaxRankingAge(d time.Duration) {
	if d > 0 {
		s.maxRankingAge = d
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
	// Fastest path: the denormalized ranking projection, when fresh — a single
	// query for exactly the rows shown, metadata joined only for those rows.
	if entries, ok := s.buildFromRanking(ctx, TimeframeAll); ok {
		return entries, nil
	}
	// Fast path: serve from the cache when it has data. Any cache problem falls
	// through to live calculation — the cache is an optimization, not the truth.
	if s.cache != nil {
		if entries, ok := s.buildFromCache(ctx); ok {
			return entries, nil
		}
	}
	res, err := s.BuildResult(ctx)
	return res.Entries, err
}

// BuildTimeframe ranks users over the given window. ALL uses the cache fast
// path (identical to Build); trailing windows prefer the ranking projection
// when fresh, else are computed live from index snapshots. A user with no
// snapshot old enough is excluded from that window.
func (s *Service) BuildTimeframe(ctx context.Context, tf Timeframe) ([]LeaderboardEntry, error) {
	if tf == TimeframeAll {
		return s.Build(ctx)
	}
	if entries, ok := s.buildFromRanking(ctx, tf); ok {
		return entries, nil
	}
	rows, _, err := s.rankRows(ctx, tf)
	if err != nil {
		return nil, err
	}
	return entriesOf(rows), nil
}

// rankingFresh reports whether the ranking projection for tf is populated and
// recent enough to be preferred over live computation.
func (s *Service) rankingFresh(ctx context.Context, tf Timeframe) bool {
	if s.ranking == nil {
		return false
	}
	at, found, err := s.ranking.OldestComputedAt(ctx, tf)
	if err != nil || !found {
		return false
	}
	return s.now().Sub(at) <= s.maxRankingAge
}

// buildFromRanking assembles the board for tf directly from the ranking
// projection. ok=false means "use the next fallback path" (no ranking store,
// stale, or a query error).
func (s *Service) buildFromRanking(ctx context.Context, tf Timeframe) ([]LeaderboardEntry, bool) {
	if !s.rankingFresh(ctx, tf) {
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

// buildFromCache assembles the board from cached scores plus user metadata.
// ok=false means "use the live path" (cache empty, unavailable, or metadata
// lookup failed).
func (s *Service) buildFromCache(ctx context.Context) ([]LeaderboardEntry, bool) {
	scores, err := s.cache.GetGlobalTop(ctx, maxLeaderboardSize)
	if err != nil {
		log.Printf("leaderboard: cache unavailable, falling back to live calculation: %v", err)
		return nil, false
	}
	if len(scores) == 0 {
		return nil, false
	}
	users, err := s.users.ListRankableUsers(ctx)
	if err != nil {
		return nil, false
	}
	byID := make(map[string]auth.RankableUser, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	rows := make([]rankedRow, 0, len(scores))
	for _, sc := range scores {
		u, ok := byID[sc.UserID]
		if !ok {
			if err := s.cache.RemoveGlobalScore(ctx, sc.UserID); err != nil {
				slog.Warn("leaderboard_cache_update_failed",
					"operation", "remove_missing_user", "user_id", sc.UserID, "error", err)
			} else {
				slog.Info("leaderboard_cache_stale_user_removed", "user_id", sc.UserID)
			}
			continue
		}
		// Redis is only a membership accelerator. Re-read the exact persistent
		// ranked projection and apply the same comparator as the live path; a
		// float64 Redis score must never decide application rank or tie order.
		rp, err := s.ranked.CurrentRankedPerformance(ctx, sc.UserID)
		if err != nil {
			return nil, false
		}
		if rp.Paused {
			_ = s.cache.RemoveGlobalScore(ctx, sc.UserID)
			continue
		}
		e := rankedEntry(0, u.DisplayName, u.AvatarKey, rp.RankedReturnPercentage, rp.RankedIndex)
		s.enrich(ctx, sc.UserID, &e)
		rows = append(rows, rankedRow{
			userID: sc.UserID, entry: e,
			returnPct: rp.RankedReturnPercentage, rankedIndex: rp.RankedIndex,
		})
	}
	sortRankedRows(rows)
	return entriesOf(rows), len(rows) > 0
}

// RefreshCache recomputes every user's live performance and upserts the all-time
// score. Canonical history is owned by the independent ranked snapshot worker.
// With neither a cache nor snapshot reader attached this compatibility refresh
// is a no-op.
//
// nextBatch selects the slice of users this call should revalue, and reports
// whether that slice is every known user (true whenever refreshBatchSize is
// unset/non-positive, or happens to cover the whole list). Selection is a
// stable, sorted-by-ID sliding window that advances across calls, so a
// caller invoking RefreshCache on a fixed interval eventually covers every
// user over ceil(len(users)/refreshBatchSize) calls, with each individual
// call doing bounded work.
func (s *Service) nextBatch(users []auth.RankableUser) ([]auth.RankableUser, bool) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	if s.refreshBatchSize <= 0 || s.refreshBatchSize >= len(users) {
		return users, true
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
	batch := make([]auth.RankableUser, 0, size)
	for i := 0; i < size; i++ {
		batch = append(batch, sorted[(start+i)%n])
	}
	s.refreshCursor = (start + size) % n
	return batch, false
}

// refreshRankingRows upserts userID's ranking-projection row for every
// timeframe it's currently eligible for, and deletes the row for any
// timeframe it's no longer eligible for (e.g. a snapshot gap that just grew
// past maxSnapshotAge). rp is the already-fetched (non-paused) ranked
// performance from this refresh tick — no extra valuation call is made.
func (s *Service) refreshRankingRows(ctx context.Context, userID string, rp RankedPerformance) {
	if s.ranking == nil {
		return
	}
	for _, tf := range allTimeframes {
		retPct, idx, ok := s.timeframeReturn(ctx, userID, rp, tf)
		if !ok {
			if err := s.ranking.Delete(ctx, tf, userID); err != nil {
				slog.Warn("leaderboard_ranking_update_failed",
					"operation", "delete_ineligible", "timeframe", tf, "user_id", userID, "error", err)
			}
			continue
		}
		if err := s.ranking.Upsert(ctx, tf, userID, idx, retPct, rp.TrackingStartedAt); err != nil {
			slog.Warn("leaderboard_ranking_update_failed",
				"operation", "upsert", "timeframe", tf, "user_id", userID, "error", err)
		}
	}
}

// deleteRankingRows removes userID's ranking-projection rows for the given
// timeframes (used when a user becomes paused — excluded from ranking
// entirely, same as the Redis cache eviction above).
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

func (s *Service) RefreshCache(ctx context.Context) (int, error) {
	if s.cache == nil && s.snapshots == nil && s.ranking == nil {
		return 0, nil
	}
	users, err := s.users.ListRankableUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrListUsers, err)
	}
	// known/deletion detection covers every user on every call — it's a
	// membership check, not a valuation, so it stays cheap regardless of
	// scale. Only the expensive per-user CurrentRankedPerformance valuation
	// below is restricted to this call's batch.
	known := make(map[string]bool, len(users))
	for _, u := range users {
		known[u.ID] = true
	}
	batch, allProcessed := s.nextBatch(users)
	slog.Info("leaderboard_cache_reconciliation_started", "users", len(users), "batch", len(batch))
	skipped := 0
	activeUpserted, pausedRemoved, deletedRemoved, cacheFailures := 0, 0, 0, 0
	ranked := make(map[string]bool, len(batch))
	valuationFailed := make(map[string]bool)
	var firstCacheErr error
	for _, u := range batch {
		rp, err := s.ranked.CurrentRankedPerformance(ctx, u.ID)
		if err != nil {
			skipped++
			valuationFailed[u.ID] = true
			continue
		}
		if rp.Paused {
			// Explicitly EVICT rather than merely skip: a user who emptied their
			// portfolio would otherwise stay visible on the board via their last
			// cached score.
			if s.cache != nil {
				if err := s.cache.RemoveGlobalScore(ctx, u.ID); err != nil {
					cacheFailures++
					if firstCacheErr == nil {
						firstCacheErr = err
					}
					slog.Warn("leaderboard_cache_update_failed",
						"operation", "remove_paused", "user_id", u.ID, "error", err)
				} else {
					pausedRemoved++
				}
			}
			s.deleteRankingRows(ctx, u.ID, allTimeframes)
			continue
		}
		ranked[u.ID] = true
		if s.cache != nil {
			// Redis sorted-set scores require float64; this is the cache boundary.
			if err := s.cache.UpsertGlobalScore(ctx, u.ID, rp.RankedReturnPercentage); err != nil {
				cacheFailures++
				if firstCacheErr == nil {
					firstCacheErr = err
				}
				slog.Warn("leaderboard_cache_update_failed",
					"operation", "upsert_active", "user_id", u.ID, "error", err)
			} else {
				activeUpserted++
			}
		}
		s.refreshRankingRows(ctx, u.ID, rp)
	}
	// Prune members that are no longer rankable at all (deleted users, or users
	// whose ranked state disappeared), so the cache converges on the live set
	// instead of accumulating ghosts.
	if s.cache != nil {
		if cached, err := s.cache.GetGlobalTop(ctx, 0); err == nil {
			for _, sc := range cached {
				// A known user who simply wasn't in this tick's batch has
				// unknown status this round — never guess-evict them; only a
				// full unbounded pass (allProcessed) may prune a known user,
				// and only when it actually revalued them and found them
				// unrankable. A genuinely deleted user (!known) is always
				// safe to prune regardless of batching.
				if ranked[sc.UserID] || valuationFailed[sc.UserID] ||
					(known[sc.UserID] && !allProcessed) {
					continue
				}
				if err := s.cache.RemoveGlobalScore(ctx, sc.UserID); err != nil {
					cacheFailures++
					if firstCacheErr == nil {
						firstCacheErr = err
					}
					slog.Warn("leaderboard_cache_update_failed",
						"operation", "remove_stale", "user_id", sc.UserID, "error", err)
				} else if !known[sc.UserID] {
					deletedRemoved++
					slog.Info("leaderboard_cache_stale_user_removed", "user_id", sc.UserID)
				}
			}
		} else {
			cacheFailures++
			if firstCacheErr == nil {
				firstCacheErr = err
			}
		}
	}
	slog.Info("leaderboard_cache_reconciliation_completed",
		"active_upserted", activeUpserted,
		"paused_removed", pausedRemoved,
		"deleted_removed", deletedRemoved,
		"valuation_skipped", skipped,
		"cache_failures", cacheFailures,
	)
	if firstCacheErr != nil {
		slog.Error("leaderboard_cache_reconciliation_failed", "error", firstCacheErr)
	}
	return skipped, firstCacheErr
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
// fresh. ok=false means "use the live path" (no ranking store, stale, or a
// query error) — same fallback contract as buildFromRanking.
func (s *Service) allRankingRows(ctx context.Context, tf Timeframe) ([]rankedRow, bool) {
	if !s.rankingFresh(ctx, tf) {
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
	// take precedence over a stale cached membership.
	current, currentErr := s.ranked.CurrentRankedPerformance(ctx, userID)
	if currentErr == nil && current.Paused {
		if s.cache != nil {
			if err := s.cache.RemoveGlobalScore(ctx, userID); err != nil {
				slog.Warn("leaderboard_cache_update_failed",
					"operation", "remove_paused_rank_lookup", "user_id", userID, "error", err)
			}
		}
		return 0, nil
	}
	// Prefer the denormalized ranking projection when fresh: a single indexed
	// query instead of scanning every user. found=false is treated as
	// "unknown" (could be a brand-new user not yet reached by a refresh
	// cycle), not as an authoritative "unranked" — fall through instead.
	if currentErr == nil && s.rankingFresh(ctx, TimeframeAll) {
		if rank, _, _, found, err := s.ranking.RankOf(ctx, TimeframeAll, userID); err == nil && found {
			return rank, nil
		}
	}
	// Only trust a cached rank after the persistent ranked projection confirms
	// that this user still exists in an active, rankable lifecycle state.
	if s.cache != nil && currentErr == nil {
		if rank, err := s.cache.GetGlobalRank(ctx, userID); err == nil && rank > 0 {
			return rank, nil
		}
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
	if !s.rankingFresh(ctx, tf) {
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
