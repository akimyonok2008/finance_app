package leaderboard

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// UserProvider enumerates the users to rank. Implemented by *auth.Service.
type UserProvider interface {
	ListUsers(ctx context.Context) ([]auth.User, error)
}

// PortfolioSummaryProvider computes a single user's portfolio summary.
// Implemented by *portfolio.Service (its Summary method).
type PortfolioSummaryProvider interface {
	Summary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error)
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
type Service struct {
	users      UserProvider
	portfolios PortfolioSummaryProvider
	cache      LeaderboardCache      // optional
	snapshots  SnapshotStore         // optional; enables trailing-window timeframes
	profiles   ProfilePublicProvider // optional; enriches rows with handle/tag/weights
	now        func() time.Time
}

// NewService wires a leaderboard Service.
func NewService(users UserProvider, portfolios PortfolioSummaryProvider) *Service {
	return &Service{users: users, portfolios: portfolios, now: func() time.Time { return time.Now().UTC() }}
}

// SetCache attaches an optional ranking cache (Redis in production).
func (s *Service) SetCache(cache LeaderboardCache) {
	s.cache = cache
}

// SetSnapshotStore attaches the index-snapshot store that powers trailing
// timeframes. Without it, every timeframe falls back to since-baseline (ALL).
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
// snapshots — a user with no snapshot old enough falls back to since-baseline.
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
			continue // user deleted since the cache was refreshed
		}
		// portfolio_index = 100 + gain% holds exactly for our formulas.
		e := rankedEntry(len(entries)+1, u.DisplayName, u.AvatarKey, sc.Score, 100+sc.Score)
		s.enrich(ctx, sc.UserID, &e)
		entries = append(entries, e)
	}
	return entries, len(entries) > 0
}

// RefreshCache recomputes every user's live performance, records an index
// snapshot (for trailing timeframes), and upserts the all-time score into the
// cache. Called by the background worker each tick. It runs whenever either a
// cache OR a snapshot store is attached; with neither it is a no-op.
func (s *Service) RefreshCache(ctx context.Context) (int, error) {
	if s.cache == nil && s.snapshots == nil {
		return 0, nil
	}
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrListUsers, err)
	}
	now := s.now()
	skipped := 0
	for _, u := range users {
		summary, err := s.portfolios.Summary(ctx, u.ID)
		if err != nil || summary == nil {
			skipped++
			continue
		}
		if s.snapshots != nil {
			if err := s.snapshots.Record(ctx, u.ID, summary.PortfolioIndex, now); err != nil {
				log.Printf("leaderboard: snapshot record failed for %s: %v", u.ID, err)
			}
		}
		if s.cache != nil {
			if err := s.cache.UpsertGlobalScore(ctx, u.ID, summary.GainLossPercentage); err != nil {
				return skipped, err
			}
		}
	}
	return skipped, nil
}

// Result carries the ranked entries plus internal metadata about how many users
// were skipped. The public handler returns only Entries; SkippedCount is for
// internal monitoring (and, later, an admin endpoint).
type Result struct {
	Entries      []LeaderboardEntry
	SkippedCount int
}

// GetUserRank returns the exact global rank for userID, or 0 when the user's
// portfolio cannot be ranked. Internal ids are used only for matching and are
// never added to the public leaderboard response.
func (s *Service) GetUserRank(ctx context.Context, userID string) (int, error) {
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
		returnPct   float64
	}
	rows := make([]rankedUser, 0, len(users))
	for _, user := range users {
		summary, err := s.portfolios.Summary(ctx, user.ID)
		if err == nil && summary != nil {
			rows = append(rows, rankedUser{id: user.ID, displayName: user.DisplayName, returnPct: summary.GainLossPercentage})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].returnPct != rows[j].returnPct {
			return rows[i].returnPct > rows[j].returnPct
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
	TotalParticipants      int
	RankedReturnPercentage float64
	RankedIndex            float64
	Ranked                 bool
}

// UserStanding computes the caller's rank within the timeframe board, plus the
// total number of ranked participants. Always uses the live path so the result
// is consistent for any timeframe.
func (s *Service) UserStanding(ctx context.Context, userID string, tf Timeframe) (Standing, error) {
	rows, _, err := s.rankRows(ctx, tf)
	if err != nil {
		return Standing{}, err
	}
	st := Standing{Timeframe: tf, TotalParticipants: len(rows)}
	for _, r := range rows {
		if r.userID == userID {
			st.Rank = r.entry.Rank
			st.RankedReturnPercentage = r.entry.RankedReturnPercentage
			st.RankedIndex = r.entry.RankedIndex
			st.Ranked = true
			break
		}
	}
	return st, nil
}

// rankedRow pairs an internal user id with its public entry. The id is used for
// matching/snapshots only and never serialized.
type rankedRow struct {
	userID string
	entry  LeaderboardEntry
}

// rankRows is the single live-ranking core: summarize each user, compute the
// timeframe return (since-baseline for ALL, else current-index vs the snapshot
// at now-window), enrich with public profile data, then sort and assign ranks.
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
		summary, err := s.portfolios.Summary(ctx, u.ID)
		if err != nil || summary == nil {
			skipped++
			log.Printf("leaderboard: skipping user %s due to summary error: %v", u.ID, err)
			continue
		}
		// Default = since-baseline (ALL): index already encodes it.
		retPct := summary.PortfolioIndex - 100
		idx := summary.PortfolioIndex
		if windowed && s.snapshots != nil {
			if base, found, err := s.snapshots.IndexAtOrBefore(ctx, u.ID, cutoff); err == nil && found && base > 0 {
				retPct = (summary.PortfolioIndex/base - 1) * 100
				idx = 100 * summary.PortfolioIndex / base
			}
		}
		e := rankedEntry(0, u.DisplayName, u.AvatarKey, round2(retPct), round2(idx))
		s.enrich(ctx, u.ID, &e)
		rows = append(rows, rankedRow{userID: u.ID, entry: e})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].entry, rows[j].entry
		if a.RankedReturnPercentage != b.RankedReturnPercentage {
			return a.RankedReturnPercentage > b.RankedReturnPercentage
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

func round2(v float64) float64 { return math.Round(v*100) / 100 }
