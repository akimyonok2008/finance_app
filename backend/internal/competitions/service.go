package competitions

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// UserProvider resolves a user's public profile. Implemented by *auth.Service.
type UserProvider interface {
	GetUserByID(ctx context.Context, userID string) (*auth.User, error)
}

// PositionProvider lists a user's current positions. Implemented by an adapter
// over *portfolio.Service. Used only at join time to capture the snapshot.
type PositionProvider interface {
	ListPositions(ctx context.Context, userID string) ([]portfolio.Position, error)
}

// Service holds weekly-sprint business logic. Sprint performance is computed
// from the join-time snapshot (not the live portfolio) and normalized to the
// base currency, so it is fair and comparable.
type Service struct {
	repo      CompetitionRepository
	users     UserProvider
	positions PositionProvider
	prices    prices.PriceProvider
	fx        fx.FXProvider
	clock     clock.Clock
	cache     leaderboard.LeaderboardCache // optional sprint ranking cache

	// Engine dependencies (see eligibility.go). snapshots is the narrow
	// read-only portfolio boundary; universes resolves named instrument
	// universes. Both optional: without snapshots, eligibility previews are
	// unavailable rather than degraded.
	snapshots SnapshotProvider
	universes UniverseResolver
	// baselineWindow bounds how long after starts_at baseline retries keep
	// running before an entry is explicitly failed (see RunCompetitionBaselines).
	baselineWindow time.Duration
	// rankingBatchSize bounds how many entries RefreshCompetitionRankings
	// claims per competition per call (see ranking.go). <= 0 uses the default.
	rankingBatchSize int
	// finalizationWindow bounds how long past ends_at the finalizer retries
	// entries with failed end-time valuation before disqualifying them and
	// finalizing without them (see finalize.go).
	finalizationWindow time.Duration
	// finalizationBatchSize bounds how many entries one finalizeEdition pass
	// claims per competition per call (see finalize.go). <= 0 uses the default.
	finalizationBatchSize int
	// achievements is the optional idempotent trophy hook fired after each
	// finalized result (see finalize.go).
	achievements FinalizationAchievementTrigger
	// basketAdjustments applies trusted splits and symbol changes to the frozen
	// scoring basket without following live portfolio mutations.
	basketAdjustments BasketAdjustmentProvider
}

// SetAchievementTrigger attaches the optional idempotent achievement hook
// fired for each user after competition finalization.
func (s *Service) SetAchievementTrigger(t FinalizationAchievementTrigger) {
	s.achievements = t
}

// NewService wires a competitions Service.
func NewService(repo CompetitionRepository, users UserProvider, positions PositionProvider, priceProvider prices.PriceProvider, fxp fx.FXProvider, clk clock.Clock) *Service {
	return &Service{repo: repo, users: users, positions: positions, prices: priceProvider, fx: fxp, clock: clk}
}

// CurrentCompetitionID returns the id of the sprint active right now.
func (s *Service) CurrentCompetitionID(_ context.Context) string {
	return WeeklySprintID(s.clock.Now())
}

// SetCache attaches an optional sprint ranking cache (Redis in production).
func (s *Service) SetCache(cache leaderboard.LeaderboardCache) {
	s.cache = cache
}

// EnsureCurrentSprint makes sure the sprint for the current ISO week exists.
// Called lazily on reads and periodically by the background worker.
func (s *Service) EnsureCurrentSprint(ctx context.Context) error {
	return s.ensureCurrentSprint(ctx)
}

// ListActiveCompetitionIDs returns the ids of competitions active right now,
// used by the background worker to know which sprint boards to refresh.
func (s *Service) ListActiveCompetitionIDs(ctx context.Context) ([]string, error) {
	comps, err := s.ListCompetitions(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, c := range comps {
		if c.Status == StatusActive {
			ids = append(ids, c.ID)
		}
	}
	return ids, nil
}

// RefreshCache recomputes every entry's sprint return from its snapshot and
// upserts the scores into the cache. Returns the skipped count.
func (s *Service) RefreshCache(ctx context.Context, competitionID string) (int, error) {
	if s.cache == nil {
		return 0, nil
	}
	entries, err := s.repo.ListEntries(ctx, competitionID)
	if err != nil {
		return 0, err
	}
	skipped := 0
	for _, e := range entries {
		currentBase, ok := s.snapshotCurrentValueBase(ctx, e.Snapshots)
		if !ok || e.StartingValue.Cmp(money.ZeroAmount()) <= 0 {
			skipped++
			continue
		}
		factor, err := currentBase.Sub(e.StartingValue).DivExact(e.StartingValue, 36)
		if err != nil {
			skipped++
			continue
		}
		score := money.QuantizeRatio(factor.Mul(money.MustRatio("100")), 2)
		if err := s.cache.UpsertCompetitionScore(ctx, competitionID, e.UserID, score); err != nil {
			return skipped, err
		}
	}
	return skipped, nil
}

// ensureCurrentSprint makes sure the sprint for the current ISO week exists in
// the repository, creating it lazily. Returns nothing useful beyond the error.
func (s *Service) ensureCurrentSprint(ctx context.Context) error {
	cur := WeeklySprint(s.clock.Now())
	if _, err := s.repo.GetCompetition(ctx, cur.ID); err != nil {
		if errors.Is(err, ErrCompetitionNotFound) {
			return s.repo.CreateCompetition(ctx, cur)
		}
		return err
	}
	return nil
}

// GetCompetition fetches one competition with its status freshly derived
// from the clock. Exported for handlers that need to branch on
// Competition.IsLegacy() before choosing a read path (e.g. the leaderboard
// and status endpoints, which serve legacy sprints and engine editions
// differently).
func (s *Service) GetCompetition(ctx context.Context, competitionID string) (*Competition, error) {
	return s.loadCompetition(ctx, competitionID)
}

// loadCompetition fetches a competition with its status freshly derived from the
// clock (stored status may be stale).
func (s *Service) loadCompetition(ctx context.Context, competitionID string) (*Competition, error) {
	if err := s.ensureCurrentSprint(ctx); err != nil {
		return nil, err
	}
	c, err := s.repo.GetCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	c.Status = deriveStatus(c.StartsAt, c.EndsAt, s.clock.Now().UTC())
	return c, nil
}

// ListCompetitions returns all competitions with statuses derived from the clock.
func (s *Service) ListCompetitions(ctx context.Context) ([]Competition, error) {
	if err := s.ensureCurrentSprint(ctx); err != nil {
		return nil, err
	}
	comps, err := s.repo.ListCompetitions(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC()
	for i := range comps {
		comps[i].Status = deriveStatus(comps[i].StartsAt, comps[i].EndsAt, now)
	}
	return comps, nil
}

// JoinCompetition captures a snapshot of the user's positions (priced and
// converted to base currency) and records an entry. Idempotent.
func (s *Service) JoinCompetition(ctx context.Context, competitionID, userID string) (*JoinCompetitionResponse, error) {
	comp, err := s.loadCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	if comp.Status != StatusActive {
		return nil, ErrCompetitionNotActive
	}

	if existing, err := s.repo.GetEntry(ctx, competitionID, userID); err == nil {
		return &JoinCompetitionResponse{CompetitionID: competitionID, Joined: true, StartingIndex: existing.StartingIndex}, nil
	}

	positions, err := s.positions.ListPositions(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, ErrEmptyPortfolio
	}

	entryID := uuid.NewString()
	snapshots := make([]CompetitionEntrySnapshotPosition, 0, len(positions))
	startingValueBase := money.ZeroAmount()
	for _, pos := range positions {
		price, err := s.prices.GetLatestPrice(ctx, pos.Symbol)
		if err != nil {
			return nil, ErrJoinSnapshot
		}
		exactPrice := money.PriceFromFloat64(price.Price)
		valueLocal := pos.Quantity.MulPrice(exactPrice)
		// Market-price and FX providers expose float64 today. Convert only at
		// those provider boundaries; all competition state and math stay exact.
		converted, err := s.fx.Convert(ctx, valueLocal.Float64(), price.Currency, fx.BaseCurrency)
		if err != nil {
			return nil, ErrJoinSnapshot
		}
		valueBase := money.AmountFromFloat64(converted)
		startingValueBase = startingValueBase.Add(valueBase)
		snapshots = append(snapshots, CompetitionEntrySnapshotPosition{
			ID:                    uuid.NewString(),
			CompetitionEntryID:    entryID,
			Symbol:                pos.Symbol,
			AssetType:             pos.AssetType,
			Quantity:              pos.Quantity,
			Currency:              pos.Currency,
			StartingPrice:         exactPrice,
			StartingPriceCurrency: price.Currency,
			StartingValueBase:     valueBase,
		})
	}
	if startingValueBase.Cmp(money.ZeroAmount()) <= 0 {
		return nil, ErrEmptyPortfolio
	}

	entry := CompetitionEntry{
		ID:            entryID,
		CompetitionID: competitionID,
		UserID:        userID,
		StartingValue: startingValueBase,
		StartingIndex: money.MustIndexValue("100"),
		JoinedAt:      time.Now().UTC(),
		Snapshots:     snapshots,
	}
	if err := s.repo.CreateEntry(ctx, entry); err != nil {
		return nil, err
	}
	return &JoinCompetitionResponse{CompetitionID: competitionID, Joined: true, StartingIndex: money.MustIndexValue("100")}, nil
}

// MyStatus returns the requesting user's own sprint status.
func (s *Service) MyStatus(ctx context.Context, competitionID, userID string) (*MyCompetitionStatusResponse, error) {
	if _, err := s.loadCompetition(ctx, competitionID); err != nil {
		return nil, err
	}

	if _, err := s.repo.GetEntry(ctx, competitionID, userID); err != nil {
		if errors.Is(err, ErrEntryNotFound) {
			return &MyCompetitionStatusResponse{CompetitionID: competitionID, Joined: false, SprintIndex: money.MustIndexValue("100")}, nil
		}
		return nil, err
	}

	ranked, err := s.rankedEntries(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	resp := &MyCompetitionStatusResponse{CompetitionID: competitionID, Joined: true, SprintIndex: money.MustIndexValue("100")}
	for _, r := range ranked {
		if r.userID == userID {
			resp.CurrentRank = r.rank
			resp.SprintReturnPercentage = r.returnPct
			resp.SprintIndex = r.index
			break
		}
	}
	return resp, nil
}

// Leaderboard returns the privacy-safe ranked sprint entries. A populated cache
// is served first; an empty or failing cache falls back to live snapshot math.
func (s *Service) Leaderboard(ctx context.Context, competitionID string) ([]SprintLeaderboardEntry, error) {
	if _, err := s.loadCompetition(ctx, competitionID); err != nil {
		return nil, err
	}
	if s.cache != nil {
		if board, ok := s.leaderboardFromCache(ctx, competitionID); ok {
			return board, nil
		}
	}
	ranked, err := s.rankedEntries(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	out := make([]SprintLeaderboardEntry, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, SprintLeaderboardEntry{
			Rank:                   r.rank,
			DisplayName:            r.displayName,
			AvatarKey:              r.avatarKey,
			SprintReturnPercentage: r.returnPct,
			SprintIndex:            r.index,
		})
	}
	return out, nil
}

// leaderboardFromCache assembles the sprint board from cached scores joined
// with user metadata. ok=false means "take the live path".
func (s *Service) leaderboardFromCache(ctx context.Context, competitionID string) ([]SprintLeaderboardEntry, bool) {
	scores, err := s.cache.GetCompetitionTop(ctx, competitionID, 100)
	if err != nil || len(scores) == 0 {
		return nil, false
	}
	out := make([]SprintLeaderboardEntry, 0, len(scores))
	for _, sc := range scores {
		user, err := s.users.GetUserByID(ctx, sc.UserID)
		if err != nil || user == nil {
			continue
		}
		out = append(out, SprintLeaderboardEntry{
			Rank:                   len(out) + 1,
			DisplayName:            user.DisplayName,
			AvatarKey:              user.AvatarKey,
			SprintReturnPercentage: money.RatioFromFloat64(sc.Score),
			// sprint_index = 100 + return% holds exactly for our formulas.
			SprintIndex: money.MustIndexValue("100").AddRatio(money.RatioFromFloat64(sc.Score)),
		})
	}
	return out, len(out) > 0
}

// GetUserRank returns the user's rank in a competition, or 0 if not ranked.
func (s *Service) GetUserRank(ctx context.Context, competitionID, userID string) (int, error) {
	ranked, err := s.rankedEntries(ctx, competitionID)
	if err != nil {
		return 0, err
	}
	for _, r := range ranked {
		if r.userID == userID {
			return r.rank, nil
		}
	}
	return 0, nil
}

type rankedRow struct {
	userID      string
	displayName string
	avatarKey   string
	returnPct   money.Ratio
	index       money.IndexValue
	rank        int
}

// rankedEntries computes sorted, ranked rows from each entry's SNAPSHOT (never
// the live portfolio). Users whose snapshot can't be repriced are skipped.
func (s *Service) rankedEntries(ctx context.Context, competitionID string) ([]rankedRow, error) {
	entries, err := s.repo.ListEntries(ctx, competitionID)
	if err != nil {
		return nil, err
	}

	rows := make([]rankedRow, 0, len(entries))
	for _, e := range entries {
		user, err := s.users.GetUserByID(ctx, e.UserID)
		if err != nil || user == nil {
			continue
		}
		currentBase, ok := s.snapshotCurrentValueBase(ctx, e.Snapshots)
		if !ok || e.StartingValue.Cmp(money.ZeroAmount()) <= 0 {
			continue
		}
		factor, err := currentBase.DivExact(e.StartingValue, 36)
		if err != nil {
			continue
		}
		returnPct := money.QuantizeRatio(factor.Sub(money.MustRatio("1")).Mul(money.MustRatio("100")), 2)
		index := money.QuantizeIndex(money.MustIndexValue("100").MulRatio(factor))
		rows = append(rows, rankedRow{
			userID:      e.UserID,
			displayName: user.DisplayName,
			avatarKey:   user.AvatarKey,
			returnPct:   returnPct,
			index:       index,
		})
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if comparison := rows[i].returnPct.Cmp(rows[j].returnPct); comparison != 0 {
			return comparison > 0
		}
		if rows[i].displayName != rows[j].displayName {
			return rows[i].displayName < rows[j].displayName
		}
		return rows[i].userID < rows[j].userID
	})
	for i := range rows {
		rows[i].rank = i + 1
	}
	return rows, nil
}

// snapshotCurrentValueBase reprices the snapshot positions at current prices and
// sums their base-currency value. ok=false if any position can't be priced or
// converted (the user is then skipped from the board).
func (s *Service) snapshotCurrentValueBase(ctx context.Context, snapshots []CompetitionEntrySnapshotPosition) (money.Amount, bool) {
	total := money.ZeroAmount()
	for _, snap := range snapshots {
		price, err := s.prices.GetLatestPrice(ctx, snap.Symbol)
		if err != nil {
			return money.ZeroAmount(), false
		}
		valueLocal := snap.Quantity.MulPrice(money.PriceFromFloat64(price.Price))
		converted, err := s.fx.Convert(ctx, valueLocal.Float64(), price.Currency, fx.BaseCurrency)
		if err != nil {
			return money.ZeroAmount(), false
		}
		total = total.Add(money.AmountFromFloat64(converted))
	}
	return total, true
}
