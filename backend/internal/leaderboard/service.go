package leaderboard

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"math"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
)

// UserProvider enumerates the users to rank. Implemented by *auth.Service.
type UserProvider interface {
	ListUsers(ctx context.Context) ([]auth.User, error)
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
}

// NewService wires a leaderboard Service around the ranked-performance provider.
func NewService(users UserProvider, ranked RankedPerformanceProvider) *Service {
	return &Service{
		users: users, ranked: ranked, maxSnapshotAge: defaultMaxSnapshotAge,
		now: func() time.Time { return time.Now().UTC() },
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
// path (identical to Build); trailing windows are computed live from index
// snapshots. A user with no snapshot old enough is excluded from that window.
func (s *Service) BuildTimeframe(ctx context.Context, tf Timeframe) ([]LeaderboardEntry, error) {
	if tf == TimeframeAll {
		return s.Build(ctx)
	}
	rows, _, err := s.rankRows(ctx, tf)
	if err != nil {
		return nil, err
	}
	return entriesOf(rows), nil
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
func rankedEntry(rank int, displayName, avatarKey string, returnPct, index float64) LeaderboardEntry {
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
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return nil, false
	}
	byID := make(map[string]auth.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}

	entries := make([]LeaderboardEntry, 0, len(scores))
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
		// portfolio_index = 100 + gain% holds exactly for our formulas.
		e := rankedEntry(
			len(entries)+1, u.DisplayName, u.AvatarKey,
			round2(sc.Score), round2(100+sc.Score),
		)
		s.enrich(ctx, sc.UserID, &e)
		entries = append(entries, e)
	}
	return entries, len(entries) > 0
}

// RefreshCache recomputes every user's live performance and upserts the all-time
// score. Canonical history is owned by the independent ranked snapshot worker.
// With neither a cache nor snapshot reader attached this compatibility refresh
// is a no-op.
func (s *Service) RefreshCache(ctx context.Context) (int, error) {
	if s.cache == nil && s.snapshots == nil {
		return 0, nil
	}
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrListUsers, err)
	}
	slog.Info("leaderboard_cache_reconciliation_started", "users", len(users))
	skipped := 0
	activeUpserted, pausedRemoved, deletedRemoved, cacheFailures := 0, 0, 0, 0
	ranked := make(map[string]bool, len(users))
	known := make(map[string]bool, len(users))
	valuationFailed := make(map[string]bool)
	var firstCacheErr error
	for _, u := range users {
		known[u.ID] = true
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
	}
	// Prune members that are no longer rankable at all (deleted users, or users
	// whose ranked state disappeared), so the cache converges on the live set
	// instead of accumulating ghosts.
	if s.cache != nil {
		if cached, err := s.cache.GetGlobalTop(ctx, 0); err == nil {
			for _, sc := range cached {
				if ranked[sc.UserID] || valuationFailed[sc.UserID] {
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
	RankedReturnPercentage float64
	RankedIndex            float64
}

func (s *Service) UserRankings(ctx context.Context, tf Timeframe) ([]UserRanking, error) {
	rows, _, err := s.rankRows(ctx, tf)
	if err != nil {
		return nil, err
	}
	out := make([]UserRanking, 0, len(rows))
	for _, row := range rows {
		out = append(out, UserRanking{
			UserID:                 row.userID,
			Rank:                   row.entry.Rank,
			RankedReturnPercentage: row.entry.RankedReturnPercentage,
			RankedIndex:            row.entry.RankedIndex,
		})
	}
	return out, nil
}

// GetUserRank returns the exact global rank for userID, or 0 when the user's
// portfolio cannot be ranked. Internal ids are used only for matching and are
// never added to the public leaderboard response.
func (s *Service) GetUserRank(ctx context.Context, userID string) (int, error) {
	// The requested user's current lifecycle state is cheap to validate and must
	// take precedence over a stale cached membership.
	if current, err := s.ranked.CurrentRankedPerformance(ctx, userID); err == nil && current.Paused {
		if s.cache != nil {
			if err := s.cache.RemoveGlobalScore(ctx, userID); err != nil {
				slog.Warn("leaderboard_cache_update_failed",
					"operation", "remove_paused_rank_lookup", "user_id", userID, "error", err)
			}
		}
		return 0, nil
	}
	if s.cache != nil {
		if scores, err := s.cache.GetGlobalTop(ctx, maxLeaderboardSize); err == nil && len(scores) > 0 {
			for i, score := range scores {
				if score.UserID == userID {
					return i + 1, nil
				}
			}
			return 0, nil
		}
	}

	users, err := s.users.ListUsers(ctx)
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
	RankedReturnPercentage float64
	RankedIndex            float64
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
	ReturnGapPercentage float64
}

// UserStanding computes the caller's rank within the timeframe board, plus the
// total number of ranked participants. Always uses the live path so the result
// is consistent for any timeframe.
func (s *Service) UserStanding(ctx context.Context, userID string, tf Timeframe) (Standing, error) {
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
			st.RankedIndex = round2(rp.RankedIndex.Float64())
			st.RankedReturnPercentage = round2(rp.RankedReturnPercentage.Float64())
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

// rankRows is the single live-ranking core: summarize each user, compute the
// timeframe return (since-baseline for ALL, else current-index vs the snapshot
// at now-window), enrich with public profile data, then sort and assign ranks.
// Windowed rows require old-enough snapshots; otherwise the user is excluded
// from that timeframe rather than being ranked on all-time performance.
func (s *Service) rankRows(ctx context.Context, tf Timeframe) ([]rankedRow, int, error) {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrListUsers, err)
	}
	window, windowed := tf.window()
	cutoff := s.now().Add(-window)

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
		// Default = all-time (since epoch): the ranked index already encodes it.
		retPct := rp.RankedReturnPercentage
		idx := rp.RankedIndex
		if windowed {
			if s.snapshots == nil {
				continue
			}
			// Only post-epoch snapshots are eligible; legacy pre-epoch history is
			// ignored so a windowed return can never use a manipulable old index.
			base, capturedAt, found, err := s.snapshots.IndexAtOrBefore(ctx, u.ID, cutoff, rp.TrackingStartedAt)
			if err != nil {
				skipped++
				log.Printf("leaderboard: skipping user %s due to snapshot error: %v", u.ID, err)
				continue
			}
			if !found || base.Sign() <= 0 {
				continue
			}
			// A base snapshot far older than the window's own start means a gap
			// in recorded history (a missed snapshot run, a paused-then-resumed
			// account) — using it would silently stretch e.g. "1W" into a much
			// longer real span, so the user is excluded from this timeframe
			// instead of mis-measured.
			if cutoff.Sub(capturedAt) > s.maxSnapshotAge {
				continue
			}
			var calcErr error
			retPct, calcErr = performancehistory.TimeframeReturnRatio(base, rp.RankedIndex)
			if calcErr != nil {
				continue
			}
			factor, divErr := rp.RankedIndex.DivExact(base, money.ScaleIndex+money.ScaleWeight)
			if divErr != nil {
				continue
			}
			idx = money.MustIndexValue("100").MulRatio(factor)
		}
		e := rankedEntry(0, u.DisplayName, u.AvatarKey, round2(retPct.Float64()), round2(idx.Float64()))
		s.enrich(ctx, u.ID, &e)
		rows = append(rows, rankedRow{userID: u.ID, entry: e, returnPct: retPct, rankedIndex: idx})
	}

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
	return rows, skipped, nil
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

func nextMilestone(rows []rankedRow, rank int, returnPct float64) *Milestone {
	if rank <= 1 {
		return nil
	}
	targetRank, label := 1, "#1"
	switch {
	case rank > 100:
		targetRank, label = 100, "Top 100"
	case rank > 25:
		targetRank, label = 25, "Top 25"
	case rank > 10:
		targetRank, label = 10, "Top 10"
	}
	gap := 0.0
	if targetRank > 0 && targetRank <= len(rows) {
		gap = round2(rows[targetRank-1].entry.RankedReturnPercentage - returnPct)
		if gap < 0 {
			gap = 0
		}
	}
	return &Milestone{
		Label:               label,
		TargetRank:          targetRank,
		RankGap:             rank - targetRank,
		ReturnGapPercentage: gap,
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
