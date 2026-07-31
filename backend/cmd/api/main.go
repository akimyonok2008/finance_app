package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/benchmark"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/config"
	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/db"
	"github.com/ardakimyonok/finance_app/internal/dbtx"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/income"
	"github.com/ardakimyonok/finance_app/internal/instrument"
	"github.com/ardakimyonok/finance_app/internal/jobs"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/leaderlock"
	"github.com/ardakimyonok/finance_app/internal/marketdata"
	"github.com/ardakimyonok/finance_app/internal/moderation"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/ardakimyonok/finance_app/internal/providerfactory"
	"github.com/ardakimyonok/finance_app/internal/safety"
	"github.com/ardakimyonok/finance_app/internal/server"
	"github.com/ardakimyonok/finance_app/internal/social"
	"github.com/ardakimyonok/finance_app/internal/strategy"
	"github.com/ardakimyonok/finance_app/internal/ttlcache"
)

// mutationPriceCacheTTL bounds the in-process burst-absorption cache placed in
// front of the price provider consumed by mutation-time repricing (see the
// priceProvider wrapping below). It is deliberately short: this exists to
// absorb rapid successive mutations, not to relax real quote freshness.
const mutationPriceCacheTTL = 5 * time.Second

// --- adapters -----------------------------------------------------------------
// These bridge existing service method names/signatures to the small interfaces
// each feature module declares, keeping the modules decoupled from concretes.

type userProvider struct{ s *auth.Service }

func (u userProvider) GetUserByID(_ context.Context, id string) (*auth.User, error) {
	return u.s.UserByID(id)
}

type summaryProvider struct{ s *portfolio.Service }

func (p summaryProvider) GetSummary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	return p.s.Summary(ctx, userID)
}

// publicWeightsProvider skips Summary's full activity-ledger reconstruction:
// the leaderboard enriches every public-weights row on every request, and all
// it reads from the summary is the position/cash valuation buildComposition
// needs — never income, fees, or realized P&L.
type publicWeightsProvider struct{ s *portfolio.Service }

func (p publicWeightsProvider) GetPublicWeights(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	return p.s.PublicWeightsSummary(ctx, userID)
}

// leaderboardCacheTTL bounds how long a per-user weights/summary computation
// is reused across repeated leaderboard/standing or Explore requests. The
// underlying data is a read-time projection, never the source of truth, so a
// short, deliberate staleness window is an acceptable tradeoff for not
// recomputing it (position list + per-symbol price/FX lookups, or — for
// Explore — the full ledger scan) on every single request.
const leaderboardCacheTTL = 30 * time.Second

// cachingWeightsProvider memoizes GetPublicWeights per userID so a leaderboard
// build enriching many public-weights rows (or repeated requests within the
// TTL window) doesn't recompute the same user's weights over and over.
type cachingWeightsProvider struct {
	inner profile.PublicWeightsProvider
	cache *ttlcache.Cache[*portfolio.PortfolioSummary]
}

func newCachingWeightsProvider(inner profile.PublicWeightsProvider) cachingWeightsProvider {
	return cachingWeightsProvider{inner: inner, cache: ttlcache.New[*portfolio.PortfolioSummary](leaderboardCacheTTL)}
}

func (c cachingWeightsProvider) GetPublicWeights(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	if cached, ok := c.cache.Get(userID); ok {
		return cached, nil
	}
	summary, err := c.inner.GetPublicWeights(ctx, userID)
	if err != nil {
		return nil, err
	}
	c.cache.Set(userID, summary)
	return summary, nil
}

// cachingSummaryProvider is the same memoization applied to a full
// SummaryProvider — used only for Explore's caller-comparison lookup (see
// profile.Service.SetExploreSummaryProvider), never for the owner's own
// profile view or a public profile page, both of which must reflect the
// caller's own just-made changes immediately.
type cachingSummaryProvider struct {
	inner profile.SummaryProvider
	cache *ttlcache.Cache[*portfolio.PortfolioSummary]
}

func newCachingSummaryProvider(inner profile.SummaryProvider) cachingSummaryProvider {
	return cachingSummaryProvider{inner: inner, cache: ttlcache.New[*portfolio.PortfolioSummary](leaderboardCacheTTL)}
}

func (c cachingSummaryProvider) GetSummary(ctx context.Context, userID string) (*portfolio.PortfolioSummary, error) {
	if cached, ok := c.cache.Get(userID); ok {
		return cached, nil
	}
	summary, err := c.inner.GetSummary(ctx, userID)
	if err != nil {
		return nil, err
	}
	c.cache.Set(userID, summary)
	return summary, nil
}

type positionProvider struct{ s *portfolio.Service }

func (p positionProvider) ListPositions(ctx context.Context, userID string) ([]portfolio.Position, error) {
	ptrs, err := p.s.ListPositions(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]portfolio.Position, 0, len(ptrs))
	for _, x := range ptrs {
		out = append(out, *x)
	}
	return out, nil
}

type rankProvider struct{ s *competitions.Service }

func (r rankProvider) GetUserRank(ctx context.Context, competitionID, userID string) (int, error) {
	return r.s.GetUserRank(ctx, competitionID, userID)
}

// benchmarkHistoryAdapter maps the marketdata historical provider to the
// benchmark engine's HistoricalPriceProvider port.
type benchmarkHistoryAdapter struct {
	h *marketdata.TwelveDataHistoryProvider
}

func (a benchmarkHistoryAdapter) GetAdjustedCloseSeries(ctx context.Context, symbol string, start, end time.Time) ([]benchmark.PricePoint, error) {
	bars, err := a.h.DailySeries(ctx, symbol, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]benchmark.PricePoint, 0, len(bars))
	for _, bar := range bars {
		price, err := money.ParsePrice(strconv.FormatFloat(bar.Close, 'f', -1, 64))
		if err != nil {
			return nil, err
		}
		out = append(out, benchmark.PricePoint{Date: bar.Date, RawClose: price})
	}
	return out, nil
}

func (a benchmarkHistoryAdapter) GetSeries(ctx context.Context, symbol string, start, end time.Time, _ benchmark.SeriesRequirement) (benchmark.BenchmarkPriceSeries, error) {
	points, err := a.GetAdjustedCloseSeries(ctx, symbol, start, end)
	if err != nil {
		return benchmark.BenchmarkPriceSeries{}, err
	}
	now := time.Now().UTC()
	return benchmark.BenchmarkPriceSeries{
		Symbol: symbol,
		Points: points,
		Metadata: benchmark.BenchmarkDataMetadata{
			Provider:          "twelvedata",
			ProviderMode:      "real",
			PriceType:         benchmark.PriceTypeRawClose,
			Quality:           benchmark.DataQualityAcceptable,
			CorpActionsKnown:  false,
			RetrievedAt:       now,
			SourceAsOf:        now,
			ProviderDataset:   "time_series_daily_close",
			CurrencyTreatment: "native_quote_currency_unhedged",
			Currency:          "USD",
		},
	}, nil
}

// benchmarkComparisonAdapter exposes the benchmark construction engine to the
// ranked-history service through its narrow BenchmarkReturner port, so
// performancehistory never imports internal/benchmark.
//
// It uses the PREVIEW-grade requirement: the Performance tab is a display, not a
// permanent award, so synthetic/raw data is allowed — but the data quality and
// synthetic flag travel with the number so the UI can label it honestly.
type benchmarkComparisonAdapter struct {
	engine  *benchmark.BenchmarkConstructionService
	recipes map[string]benchmark.BenchmarkRecipe
}

func (a benchmarkComparisonAdapter) ReturnOver(ctx context.Context, recipeID string, start, end time.Time) (performancehistory.BenchmarkReturn, error) {
	result, err := a.engine.CalculateReturn(ctx, recipeID, start, end, benchmark.RequirementForPreview())
	if err != nil {
		reason := "Benchmark component data incomplete."
		switch {
		case errors.Is(err, benchmark.ErrAdjustedDataUnavailable), errors.Is(err, benchmark.ErrTotalReturnUnavailable):
			reason = "Total-return benchmark data unavailable."
		case errors.Is(err, benchmark.ErrHistoricalFXUnavailable):
			reason = "Historical FX unavailable."
		case errors.Is(err, benchmark.ErrRecipeVersionUnavailable):
			reason = "Recipe version unavailable."
		case errors.Is(err, benchmark.ErrCurrencyTreatmentUnavailable):
			reason = "Benchmark currency treatment unavailable."
		}
		return performancehistory.BenchmarkReturn{}, performancehistory.BenchmarkUnavailableError{Reason: reason}
	}
	returnPct, err := strconv.ParseFloat(result.ReturnPercentage.String(), 64)
	if err != nil {
		return performancehistory.BenchmarkReturn{}, err
	}
	return performancehistory.BenchmarkReturn{
		RecipeID:         recipeID,
		Name:             a.recipes[recipeID].Name,
		ReturnPercentage: returnPct,
		EffectiveStart:   result.EffectiveStart,
		EffectiveEnd:     result.EffectiveEnd,
		Quality:          string(result.DataMetadata.Quality),
		Synthetic:        result.DataMetadata.IsSynthetic,
		DataType:         benchmarkComparisonDataType(result.DataMetadata),
		TotalReturnComparable: result.DataMetadata.AllSeriesAdjusted ||
			result.DataMetadata.AllSeriesTotalReturn || result.DataMetadata.IsSynthetic,
		CurrencyTreatment: result.DataMetadata.CurrencyTreatment,
	}, nil
}

func benchmarkComparisonDataType(meta benchmark.BenchmarkEvaluationMetadata) string {
	if meta.IsSynthetic {
		return "synthetic"
	}
	if meta.AllSeriesTotalReturn {
		return "total_return"
	}
	if meta.AllSeriesAdjusted {
		return "adjusted_close"
	}
	if len(meta.PriceTypes) > 0 {
		return string(meta.PriceTypes[0])
	}
	return "unavailable"
}

// userBatchWalker hands each caller a bounded, cursor-advancing batch of
// rankable users via auth's keyset pagination — O(batch) listed per tick
// instead of the full population — wrapping to the start once a lap reaches
// the end of the table. A periodic caller therefore still covers every user
// over ceil(N/batch) ticks, with per-tick cost independent of N. batch <= 0
// falls back to one full listing per call (dev/memory scale). Safe for
// concurrent use; adapters embed a pointer so value copies share the cursor.
type userBatchWalker struct {
	mu     sync.Mutex
	cursor string
	users  *auth.Service
	batch  int
}

func (w *userBatchWalker) next(ctx context.Context) ([]auth.RankableUser, error) {
	if w.batch <= 0 {
		return w.users.ListRankableUsers(ctx)
	}
	w.mu.Lock()
	cursor := w.cursor
	w.mu.Unlock()
	page, err := w.users.ListRankableUsersPage(ctx, cursor, w.batch)
	if err != nil {
		return nil, err
	}
	next := ""
	if len(page) == w.batch {
		next = page[len(page)-1].ID
	}
	w.mu.Lock()
	w.cursor = next
	w.mu.Unlock()
	return page, nil
}

// portfolioSnapshotAdapter preserves the private daily cost-basis/composition
// archive. Ranked leaderboards, profiles, and achievements do not consume it.
//
// SnapshotAllDaily is called on every worker tick (default 60s), but a
// snapshot is only ever needed once per user per UTC day. The
// SnapshottedUserIDsToday pre-filter keeps every tick after a user's snapshot
// from re-paying the full RecordDailySnapshot valuation
// (portfolio.Service.Summary) just to have CreateArchiveSnapshot's unique
// index discard it. The walker bounds both the listing and the valuations of
// a single tick, so a burst right after UTC midnight (when everyone is newly
// pending at once) is spread across several ticks, and no tick ever loads the
// whole user table.
type portfolioSnapshotAdapter struct {
	portfolio *portfolio.Service
	walker    *userBatchWalker
}

func (a portfolioSnapshotAdapter) SnapshotAllDaily(ctx context.Context) (int, error) {
	batch, err := a.walker.next(ctx)
	if err != nil {
		return 0, err
	}
	done, err := a.portfolio.SnapshottedUserIDsToday(ctx)
	if err != nil {
		return 0, err
	}
	recorded := 0
	for _, u := range batch {
		if done[u.ID] {
			continue
		}
		wrote, err := a.portfolio.RecordDailySnapshot(ctx, u.ID)
		if err != nil {
			slog.Warn("daily snapshot failed for user", "user_id", u.ID, "error", err)
			continue
		}
		if wrote {
			recorded++
		}
	}
	return recorded, nil
}

type rankedMutationSnapshotAdapter struct{ history *performancehistory.Service }

func (a rankedMutationSnapshotAdapter) RecordMutationSnapshot(ctx context.Context, ev portfolio.OutboxEvent) error {
	_, err := a.history.RecordTransitionIfChanged(ctx, performance.TransitionSnapshot{
		PortfolioID: ev.AggregateID, UserID: ev.UserID,
		TrackingStartedAt: ev.TrackingStartedAt,
		RankedIndex:       ev.RankedIndex, Status: performance.Status(ev.RankingStatus),
		CapturedAt: ev.CreatedAt, ValuationAsOf: ev.ValuationAsOf,
		DataQualityStatus: ev.DataQualityStatus,
	})
	return err
}

type rankedSnapshotJobAdapter struct {
	walker       *userBatchWalker
	history      *performancehistory.Service
	achievements *achievements.Service
}

// SnapshotAll (the name predates batching) records the canonical ranked
// snapshot for one walker batch per call, so a single pass never values the
// whole population — full coverage accrues across ticks exactly like the
// leaderboard refresh.
func (a rankedSnapshotJobAdapter) SnapshotAll(ctx context.Context) performancehistory.BatchResult {
	result := performancehistory.BatchResult{}
	users, err := a.walker.next(ctx)
	if err != nil {
		result.Failures++
		return result
	}
	for _, user := range users {
		result.UsersProcessed++
		created, snapshotQuality, err := a.history.RecordCurrent(ctx, user.ID)
		if err != nil {
			result.SkippedValuations++
			continue
		}
		result.SnapshotsCreated += created
		if snapshotQuality == performancehistory.QualityStale {
			result.StaleSnapshots++
		}
	}
	return result
}

func (a rankedSnapshotJobAdapter) ProcessEvaluations(ctx context.Context) (processed, failed int) {
	claims, err := a.history.ClaimEvaluations(ctx, 100)
	if err != nil {
		return 0, 1
	}
	for _, claim := range claims {
		if err := a.achievements.EvaluateLocked(ctx, claim.UserID); err != nil {
			failed++
			_ = a.history.FailEvaluation(ctx, claim.SnapshotID, err)
			continue
		}
		if err := a.history.CompleteEvaluation(ctx, claim.SnapshotID); err != nil {
			failed++
			continue
		}
		processed++
	}
	return processed, failed
}

func (a rankedSnapshotJobAdapter) Compact(ctx context.Context) (int64, error) {
	return a.history.Compact(ctx)
}

// leaderboardProfileAdapter joins public profile data onto leaderboard rows,
// converting profile weights into the leaderboard package's shape.
type leaderboardProfileAdapter struct{ s *profile.Service }

func (a leaderboardProfileAdapter) PublicInfo(ctx context.Context, userID string) (leaderboard.ProfilePublicInfo, bool, error) {
	info, ok, err := a.s.PublicInfoForUser(ctx, userID)
	if err != nil || !ok {
		return leaderboard.ProfilePublicInfo{}, ok, err
	}
	weights := make([]leaderboard.PublicWeight, 0, len(info.Weights))
	for _, w := range info.Weights {
		weights = append(weights, leaderboard.PublicWeight{
			Symbol: w.Symbol, AssetType: w.AssetType, WeightPercentage: w.Weight,
		})
	}
	return leaderboard.ProfilePublicInfo{
		Handle: info.Handle, StrategyTag: info.StrategyTag,
		IsPublic: info.IsPublic, ShowWeights: info.ShowWeights, Weights: weights,
	}, true, nil
}

// PublicInfoBatch implements leaderboard.ProfilePublicBatchProvider, letting a
// leaderboard page enrich in one query instead of one per row.
func (a leaderboardProfileAdapter) PublicInfoBatch(ctx context.Context, userIDs []string) (map[string]leaderboard.ProfilePublicInfo, error) {
	infos, err := a.s.PublicInfoForUsers(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]leaderboard.ProfilePublicInfo, len(infos))
	for userID, info := range infos {
		weights := make([]leaderboard.PublicWeight, 0, len(info.Weights))
		for _, w := range info.Weights {
			weights = append(weights, leaderboard.PublicWeight{
				Symbol: w.Symbol, AssetType: w.AssetType, WeightPercentage: w.Weight,
			})
		}
		out[userID] = leaderboard.ProfilePublicInfo{
			Handle: info.Handle, StrategyTag: info.StrategyTag,
			IsPublic: info.IsPublic, ShowWeights: info.ShowWeights, Weights: weights,
		}
	}
	return out, nil
}

// rankedPerformanceAdapter bridges the ranked-performance service to the
// leaderboard's narrow provider port, translating the paused status into the
// boolean the leaderboard uses to exclude empty portfolios.
type rankedPerformanceAdapter struct{ s *performance.Service }

func (a rankedPerformanceAdapter) CurrentRankedPerformance(ctx context.Context, userID string) (leaderboard.RankedPerformance, error) {
	rp, err := a.s.CurrentRankedPerformance(ctx, userID)
	if err != nil {
		return leaderboard.RankedPerformance{}, err
	}
	return leaderboard.RankedPerformance{
		RankedIndex:            rp.RankedIndex,
		RankedReturnPercentage: rp.RankedReturnPercentage,
		Paused:                 rp.Status == performance.StatusPaused,
		TrackingStartedAt:      rp.TrackingStartedAt,
	}, nil
}

// profileRankedAdapter bridges the ranked-performance service to the profile
// module's port so public profiles surface the trusted ranked index.
type profileRankedAdapter struct{ s *performance.Service }

func (a profileRankedAdapter) CurrentRankedPerformance(ctx context.Context, userID string) (profile.RankedPerformance, error) {
	rp, err := a.s.CurrentRankedPerformance(ctx, userID)
	if err != nil {
		return profile.RankedPerformance{}, err
	}
	return profile.RankedPerformance{
		RankedIndex:            rp.RankedIndex.Float64(),
		RankedReturnPercentage: rp.RankedReturnPercentage.Float64(),
		Paused:                 rp.Status == performance.StatusPaused,
	}, nil
}

type profileHistoryAdapter struct{ s *performancehistory.Service }

func (a profileHistoryAdapter) RankedHistory(ctx context.Context, userID string, start, end time.Time) ([]profile.PublicPerformancePoint, error) {
	points, err := a.s.Series(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]profile.PublicPerformancePoint, 0, len(points))
	for _, point := range points {
		out = append(out, profile.PublicPerformancePoint{
			CapturedAt:       point.CapturedAt.Format(time.RFC3339),
			PortfolioIndex:   point.RankedIndex.Float64(),
			ReturnPercentage: point.RankedIndex.Sub(money.MustIndexValue("100")).Float64(),
		})
	}
	return out, nil
}

type profileTimeframeRankAdapter struct{ s *leaderboard.Service }

func (a profileTimeframeRankAdapter) UserRankings(ctx context.Context, timeframe string) ([]profile.TimeframeRanking, error) {
	rows, err := a.s.UserRankings(ctx, leaderboard.ParseTimeframe(timeframe))
	if err != nil {
		return nil, err
	}
	out := make([]profile.TimeframeRanking, 0, len(rows))
	for _, row := range rows {
		out = append(out, profile.TimeframeRanking{
			UserID:                 row.UserID,
			Rank:                   row.Rank,
			RankedReturnPercentage: row.RankedReturnPercentage.Float64(),
			RankedIndex:            row.RankedIndex.Float64(),
		})
	}
	return out, nil
}

// repositories groups whichever implementations the storage provider selected.
type repositories struct {
	users        auth.UserRepository
	portfolio    portfolio.Repository
	performance  performance.StateReader
	history      performancehistory.Repository
	competitions competitions.CompetitionRepository
	achievements achievements.AchievementRepository
	profiles     profile.Repository
	marketdata   marketdata.Repository
	social       social.Repository
	instruments  instrument.Repository
	safety       safety.Repository
	moderation   moderation.Repository
	// blockCoordinator is non-nil only for the postgres provider, where it
	// performs block-creation + follow-removal as a single transaction. The
	// memory provider falls back to safety.Service's non-coordinator path.
	blockCoordinator safety.BlockCoordinator
	// pgPool is non-nil only for the postgres provider. It backs the leader
	// election used to keep the non-multi-instance-safe periodic jobs
	// (leaderboard/sprint refresh, daily snapshots, quote refresh) running on
	// exactly one replica; see internal/leaderlock.
	pgPool *pgxpool.Pool
}

// authAdminAdapter bridges auth.Service to the moderation.UserAdmin and
// safety.UserStatusProvider ports, so neither package imports auth directly.
type authAdminAdapter struct{ s *auth.Service }

func (a authAdminAdapter) UserByID(_ context.Context, id string) (moderation.UserView, error) {
	u, err := a.s.UserByID(id)
	if err != nil {
		return moderation.UserView{}, err
	}
	return moderation.UserView{ID: u.ID, Role: u.Role, IsAdmin: u.IsAdmin()}, nil
}

func (a authAdminAdapter) Suspend(ctx context.Context, userID string, until *time.Time, reason string) error {
	return a.s.Suspend(ctx, userID, until, reason)
}

func (a authAdminAdapter) Ban(ctx context.Context, userID string, reason string) error {
	return a.s.Ban(ctx, userID, reason)
}

func (a authAdminAdapter) UserStatus(_ context.Context, userID string) (suspended bool, banned bool, err error) {
	u, err := a.s.UserByID(userID)
	if err != nil {
		return false, false, err
	}
	return u.IsSuspended(time.Now().UTC()), u.IsBanned(), nil
}

// moderationMessageAdapter bridges social.Service's report-evidence lookup to
// moderation.MessageAccessor.
type moderationMessageAdapter struct{ s *social.Service }

func (a moderationMessageAdapter) MessageForReport(ctx context.Context, requesterID, messageID string) (moderation.MessageEvidence, error) {
	e, err := a.s.MessageForReport(ctx, requesterID, messageID)
	if err != nil {
		return moderation.MessageEvidence{}, err
	}
	return moderation.MessageEvidence{
		MessageID: e.MessageID, ConversationID: e.ConversationID, SenderID: e.SenderID,
		ParticipantIDs: e.ParticipantIDs, Text: e.Text, CreatedAt: e.CreatedAt,
	}, nil
}

func shouldStartOutbox(storageProvider string, optionalWorkers bool) bool {
	return storageProvider == "postgres" || optionalWorkers
}

func shouldStartOptionalJobs(optionalWorkers bool) bool { return optionalWorkers }

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	// Cancelled on SIGINT/SIGTERM. Background workers below (outbox, ranked
	// snapshots, quote refresh, corporate actions, income) all select on this
	// same ctx, so a container stop or Ctrl-C now drains them cleanly instead
	// of killing them mid-write — the previous context.Background() never
	// cancelled, so shutdown was a hard kill regardless of signal.
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	if cfg.UsingDefaultSecret() {
		slog.Warn("using the default development JWT secret; set JWT_SECRET before production")
	}

	// --- Sentry (optional error reporting; never gates startup) ---
	// Unset SENTRY_DSN entirely disables reporting rather than silently
	// pointing at a shared/default project — there's no default DSN here.
	if cfg.SentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              cfg.SentryDSN,
			Environment:      cfg.AppEnv,
			AttachStacktrace: true,
		}); err != nil {
			slog.Error("sentry init failed; continuing without error reporting", "error", err)
		} else {
			defer sentry.Flush(2 * time.Second)
			slog.Info("sentry error reporting enabled")
		}
	}

	// --- Redis (optional cache/rate-limit backend; never gates startup) ---
	var redisClient *redis.Client
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("invalid REDIS_URL", "error", err)
			os.Exit(1)
		}
		client := redis.NewClient(opts)
		if err := client.Ping(ctx).Err(); err != nil {
			// Redis backs caches and the rate limiter, both of which have
			// documented in-memory fallbacks; a transient outage here must
			// not become a full application outage.
			slog.Error("redis connection failed; continuing with in-memory fallbacks", "error", err)
			_ = client.Close()
		} else {
			redisClient = client
			slog.Info("redis connected")
		}
	}

	var priceCache prices.PriceCache = prices.NewInMemoryPriceCache()
	if redisClient != nil {
		priceCache = prices.NewRedisPriceCache(redisClient)
	}

	// --- storage ---
	var (
		repos            repositories
		readinessChecks  []server.ReadinessCheck
		corpActionStorer corpactions.Store // durable when postgres; in-memory otherwise
		incomeStorer     income.Store      // durable when postgres; in-memory otherwise
	)
	switch cfg.StorageProvider {
	case "memory":
		memPortfolio := portfolio.NewInMemoryRepository()
		repos = repositories{
			users:        auth.NewInMemoryUserRepository(),
			portfolio:    memPortfolio,
			performance:  memPortfolio,
			history:      performancehistory.NewInMemoryRepository(),
			competitions: competitions.NewInMemoryCompetitionRepository(),
			achievements: achievements.NewInMemoryAchievementRepository(),
			profiles:     profile.NewInMemoryRepository(),
			marketdata:   marketdata.NewInMemoryRepository(),
			social:       social.NewInMemoryRepository(),
			instruments:  instrument.NewInMemoryRepository(),
			safety:       safety.NewInMemoryRepository(),
			moderation:   moderation.NewInMemoryRepository(),
		}
	case "postgres":
		pool, err := db.ConnectPostgres(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("postgres connection failed", "error", err)
			os.Exit(1)
		}
		if err := db.RunMigrations(ctx, pool); err != nil {
			slog.Error("postgres migrations failed", "error", err)
			os.Exit(1)
		}
		slog.Info("postgres connected, migrations applied")
		achRepo, err := achievements.NewPostgresAchievementRepository(ctx, pool)
		if err != nil {
			slog.Error("achievement catalogue seeding failed", "error", err)
			os.Exit(1)
		}
		repos = repositories{
			users:            auth.NewPostgresUserRepository(pool),
			portfolio:        portfolio.NewPostgresRepository(pool),
			performance:      performance.NewPostgresStateReader(pool),
			history:          performancehistory.NewPostgresRepository(pool),
			competitions:     competitions.NewPostgresCompetitionRepository(pool),
			achievements:     achRepo,
			profiles:         profile.NewPostgresRepository(pool),
			marketdata:       marketdata.NewPostgresRepository(pool),
			social:           social.NewPostgresRepository(pool),
			instruments:      instrument.NewPostgresRepository(pool),
			safety:           safety.NewPostgresRepository(pool),
			moderation:       moderation.NewPostgresRepository(pool),
			blockCoordinator: safety.NewPostgresBlockCoordinator(pool),
			pgPool:           pool,
		}
		corpActionStorer = corpactions.NewPostgresStore(pool)
		incomeStorer = income.NewPostgresStore(pool)
		readinessChecks = append(readinessChecks, server.ReadinessCheck{
			Name:  "postgres",
			Check: func(ctx context.Context) error { return pool.Ping(ctx) },
		})
	default:
		slog.Error("unknown STORAGE_PROVIDER (allowed: memory, postgres)", "value", cfg.StorageProvider)
		os.Exit(1)
	}

	// --- market-data provider (+ conservative quote cache service) ---
	var marketProvider marketdata.Provider
	var twelveData *marketdata.TwelveDataProvider // set when the real feed is active
	priceProviderName := cfg.PriceProvider
	switch cfg.PriceProvider {
	case "mock", "":
		marketProvider = marketdata.NewMockProvider(prices.NewMockPriceProvider())
		priceProviderName = "mock"
	case "twelvedata":
		if !cfg.EnableRealMarketData {
			slog.Error("PRICE_PROVIDER=twelvedata requires ENABLE_REAL_MARKET_DATA=true")
			os.Exit(1)
		}
		p, err := marketdata.NewTwelveDataProvider(marketdata.TwelveDataConfig{
			APIKey: cfg.TwelveDataAPIKey, BaseURL: cfg.TwelveDataBaseURL,
			Timeout: cfg.TwelveDataRequestTimeout, CacheTTL: cfg.QuoteCacheTTL,
			MaxPerMinute: cfg.TwelveDataMaxRequestsPerMinute,
			DailyBudget:  cfg.TwelveDataDailyRequestBudget,
		})
		if err != nil {
			slog.Error("twelve data configuration error", "error", err)
			os.Exit(1)
		}
		marketProvider = p
		twelveData = p
	case "yahoo":
		slog.Warn("the Yahoo (finance-go) provider is PROTOTYPE ONLY; use PRICE_PROVIDER=twelvedata for Prototype 3A real data")
		baseProvider, err := prices.NewProvider(cfg.PriceProvider)
		if err != nil {
			slog.Error("price provider configuration error", "error", err)
			os.Exit(1)
		}
		cached := prices.NewCachedPriceProvider(baseProvider, priceCache, cfg.PriceCacheTTL)
		marketProvider = marketdata.NewPriceProviderAdapter(cached, "yahoo", cfg.PriceCacheTTL)
	default:
		slog.Error("unknown PRICE_PROVIDER (allowed: mock, yahoo, twelvedata)", "value", cfg.PriceProvider)
		os.Exit(1)
	}
	marketDataSvc := marketdata.NewService(repos.marketdata, marketProvider, marketdata.ServiceConfig{
		RealMarketDataEnabled: cfg.EnableRealMarketData && priceProviderName == "twelvedata",
		QuoteCacheTTL:         cfg.QuoteCacheTTL,
		QuoteStaleAfter:       cfg.QuoteStaleAfter,
		AllowStaleOnError:     cfg.QuoteAllowStaleOnProviderError,
		SearchLimit:           10,
		MaxQuoteBatchSize:     25,
	})
	var priceProvider prices.PriceProvider = marketDataSvc
	// A short in-process cache absorbs bursts of mutations against the same
	// held symbols in quick succession: every mutation re-prices EVERY
	// currently-held symbol for the ranked-index checkpoint, so without this,
	// several buys/sells in a row each pay a repo/provider round trip per
	// held symbol even though the price hasn't (and can't have) changed in
	// that window. The TTL is short and purely a burst absorber — real quote
	// freshness is still governed by marketDataSvc's own QuoteCacheTTL
	// underneath, which this sits in front of, not instead of.
	priceProvider = prices.NewCachedPriceProvider(priceProvider, prices.NewInMemoryPriceCache(), mutationPriceCacheTTL)
	// Redis is a cache/rate-limit optimization with in-memory fallbacks, so its
	// availability does not gate readiness (see connection setup above).

	// --- services ---
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpiry)
	authSvc := auth.NewService(repos.users, tokens)
	var emailSender auth.EmailSender = auth.DevelopmentEmailSender{}
	if strings.EqualFold(cfg.EmailSender, "smtp") {
		sender, err := auth.NewSMTPEmailSender(auth.SMTPConfig{
			Host: cfg.SMTPHost, Port: cfg.SMTPPort, Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword, From: cfg.SMTPFrom,
		})
		if err != nil {
			slog.Error("email sender configuration failed", "error", err)
			os.Exit(1)
		}
		emailSender = sender
	}
	authSvc.ConfigureLifecycle(auth.LifecycleConfig{
		EmailSender: emailSender, PublicAppURL: cfg.PublicAppURL,
		VerificationTTL: cfg.EmailVerificationTTL,
		ResetTTL:        cfg.PasswordResetTTL,
		ReauthTTL:       cfg.ReauthenticationTTL,
	})
	authSvc.ConfigureProviderAuth(auth.ProviderAuthConfig{
		GoogleEnabled:  cfg.GoogleAuthEnabled,
		AppleEnabled:   cfg.AppleAuthEnabled,
		GoogleVerifier: auth.NewGoogleVerifier(cfg.GoogleClientID),
		AppleVerifier:  auth.NewAppleVerifier(cfg.AppleClientID),
	})
	// Reuses the JWT secret as an HMAC pepper for SignupIPHash — this is an
	// internal correlation signal (multi-account detection), not a security
	// boundary of its own, so a dedicated secret would be pure overhead.
	authSvc.ConfigureSignupIPHashing(cfg.JWTSecret)
	// Authentication mail is correctness-critical, not an optional analytics
	// job. Registration commits its verification email to a durable outbox, so
	// this processor runs even when ENABLE_BACKGROUND_WORKERS is false.
	authSvc.StartEmailOutboxProcessor(ctx, 5*time.Second)
	fxProvider := fx.NewMockFXProvider()
	portfolioSvc := portfolio.NewService(repos.portfolio, priceProvider, fxProvider)
	if pgPortfolioRepo, ok := repos.portfolio.(*portfolio.PostgresRepository); ok {
		// Create the default portfolio inside the SAME transaction as the new
		// user row (see PostgresUserRepository.CreateWithVerification), so a
		// portfolio-creation failure rolls the user back too instead of
		// leaving a registered account with no portfolio.
		authSvc.RegisterTxCreationHook(func(ctx context.Context, q dbtx.Querier, userID string) error {
			_, err := pgPortfolioRepo.EnsureDefaultPortfolioTx(ctx, q, userID)
			return err
		})
	} else {
		authSvc.RegisterCreationHook(portfolioSvc)
	}
	var identityProvider instrument.IdentityProvider
	if cfg.OpenFIGIEnabled {
		identityProvider = instrument.NewOpenFIGIProvider(instrument.OpenFIGIConfig{
			BaseURL: cfg.OpenFIGIBaseURL, APIKey: cfg.OpenFIGIAPIKey,
			Timeout: cfg.OpenFIGIRequestTimeout,
		})
	}
	identityResolver := instrument.NewResolver(repos.instruments, identityProvider)
	portfolioSvc.SetInstrumentResolver(identityResolver)
	portfolioSvc.SetPriceProviderName(priceProviderName)
	portfolioSvc.SetInstrumentResolutionRequired(cfg.InstrumentResolutionRequired)

	// One-shot CLI mode: `go run ./cmd/api backfill-instruments` resolves
	// legacy positions/activities against the local identity register (never
	// OpenFIGI) and queues anything ambiguous/unresolved for admin review
	// via GET /admin/identity-reconciliation, then exits — it never starts
	// the HTTP server or background workers.
	if len(os.Args) > 1 && os.Args[1] == "backfill-instruments" {
		summary, err := portfolioSvc.RunInstrumentBackfill(ctx, 0)
		if err != nil {
			slog.Error("instrument backfill failed", "error", err)
			os.Exit(1)
		}
		slog.Info("instrument backfill complete",
			"positions_scanned", summary.PositionsScanned, "positions_resolved", summary.PositionsResolved, "positions_queued", summary.PositionsQueued,
			"activities_scanned", summary.ActivitiesScanned, "activities_resolved", summary.ActivitiesResolved, "activities_queued", summary.ActivitiesQueued)
		return
	}
	// Ranked-performance engine: the single trusted source of global ranked
	// performance. It is READ-ONLY — ranked state is written exclusively inside
	// the portfolio aggregate transaction (portfolio.MutationCoordinator), so a
	// checkpoint can never commit apart from the position write that caused it.
	performanceSvc := performance.NewService(repos.performance)
	performanceSvc.SetValuator(portfolioSvc)
	portfolioSvc.SetRankedPerformanceProvider(performanceSvc)
	// Ranked-index history: the evidence series behind timeframe leaderboards and
	// benchmark badges. It reads ranked performance, never portfolio values.
	historySvc := performancehistory.NewService(repos.history, performanceSvc, performancehistory.Config{
		IntradayInterval:     cfg.RankedSnapshotInterval,
		BoundaryTolerance:    cfg.RankedBoundaryTolerance,
		EndFreshness:         cfg.RankedEndFreshness,
		EligibilityThreshold: cfg.RankedActiveCoverage,
		IntradayRetention:    cfg.RankedSnapshotRetention,
	})
	portfolioSvc.Coordinator().SetTrustedSnapshotBoundary(historySvc)
	leaderboardSvc := leaderboard.NewService(authSvc, rankedPerformanceAdapter{performanceSvc})
	leaderboardSvc.SetSnapshotStore(historySvc)
	// Same tolerance performancehistory itself uses for "close enough to a
	// requested boundary" (RANKED_BOUNDARY_TOLERANCE), so a windowed
	// leaderboard/standing and /performance/history agree on how large a gap
	// in recorded snapshots is still acceptable for a given window.
	leaderboardSvc.SetMaxSnapshotAge(cfg.RankedBoundaryTolerance)
	// Denormalized ranking projection (table leaderboard_rankings): lets board,
	// per-user-rank, and windowed-standing reads become a single indexed query
	// instead of enumerating every user. Populated by leaderboardSvc's own
	// RefreshCache from the same per-user valuation it already pays for. A nil
	// pool (memory-mode storage) leaves it unset, and every read path falls
	// back to live computation exactly as before.
	if repos.pgPool != nil {
		leaderboardSvc.SetRankingStore(leaderboard.NewPostgresRankingStore(repos.pgPool))
	}
	competitionsSvc := competitions.NewService(
		repos.competitions, userProvider{authSvc}, positionProvider{portfolioSvc},
		priceProvider, fxProvider, clock.RealClock{},
	)
	// The competition engine reads portfolio composition ONLY through this
	// narrow, read-only, version-consistent boundary (eligibility previews
	// and, later, join snapshots) — never through repositories directly.
	competitionsSvc.SetSnapshotProvider(portfolioSvc)
	var competitionAdminSvc *competitions.AdminService
	if repos.pgPool != nil {
		competitionAdminSvc = competitions.NewAdminService(
			competitionsSvc,
			competitions.NewPostgresDefinitionRepository(repos.pgPool),
			repos.competitions.(competitions.EditionRepository),
			competitions.NewPostgresCompetitionAdminRepository(repos.pgPool),
		)
	}
	// Benchmark badge engine. The portfolio side uses canonical ranked snapshots;
	// the benchmark side uses Twelve Data historical closes when enabled. The
	// deterministic offline provider supports previews in local development but
	// is not eligible to create permanent awards.
	var benchmarkHistory benchmark.HistoricalPriceProvider
	if twelveData != nil {
		benchmarkHistory = benchmarkHistoryAdapter{
			marketdata.NewTwelveDataHistoryProvider(twelveData, cfg.PriceCacheTTL),
		}
		slog.Info("benchmark evaluation using real Twelve Data historical prices")
	} else {
		benchmarkHistory = benchmark.NewMockHistoricalPriceProvider(benchmark.DefaultMockReturns())
		slog.Warn("benchmark evaluation using offline mock prices (set PRICE_PROVIDER=twelvedata + ENABLE_REAL_MARKET_DATA=true for real data)")
	}
	benchmarkEngine := benchmark.NewBenchmarkConstructionService(
		benchmarkHistory,
		benchmark.Recipes,
		benchmark.NewSnapshotRecipeResolver(benchmark.DefaultRecipeSnapshots()),
	)
	benchmarkEngine.SetBaseCurrency(cfg.BaseCurrency)
	// The Performance tab's benchmark comparison reuses the SAME construction
	// engine the achievements pipeline uses — there is no second benchmark path.
	historySvc.SetBenchmark(benchmarkComparisonAdapter{
		engine: benchmarkEngine, recipes: benchmark.Recipes,
	})
	rulesEngine := benchmark.NewRulesEngine(benchmark.DefaultEvaluators())
	achievementsSvc := achievements.NewService(
		repos.achievements,
		historySvc,
		benchmarkEngine,
		rulesEngine,
	)
	if twelveData != nil {
		achievementsSvc.SetBenchmarkDataSource("twelvedata_close")
	} else {
		achievementsSvc.SetBenchmarkDataSource("mock")
	}
	// Permanent-award policy. The environment and mode come from configuration,
	// never inferred from whether an API key is present. Verified awards require
	// real, adjusted/total-return data AND a valid recipe version; misconfigured
	// production is downgraded to disabled with a prominent warning.
	awardMode := benchmark.ParseAwardMode(cfg.BenchmarkAwardMode)
	environment := benchmark.EnvironmentMode(cfg.AppEnv)
	if awardMode == benchmark.AwardModeVerifiedOnly && twelveData == nil {
		slog.Warn("BENCHMARK_AWARD_MODE=verified_only but no real price provider is configured; verified awards are impossible with mock data — disabling permanent awards",
			"price_provider", cfg.PriceProvider, "app_env", cfg.AppEnv)
		if environment == benchmark.EnvironmentProduction {
			slog.Error("refusing to issue verified benchmark awards in production without a verified price provider")
		}
		awardMode = benchmark.AwardModeDisabled
	}
	achievementsSvc.SetAwardPolicy(awardMode, environment)
	slog.Info("benchmark award policy configured", "mode", string(awardMode), "environment", string(environment))
	profileSvc := profile.NewService(repos.profiles, userProvider{authSvc}, summaryProvider{portfolioSvc})
	profileSvc.SetPublicWeightsProvider(newCachingWeightsProvider(publicWeightsProvider{portfolioSvc}))
	profileSvc.SetExploreSummaryProvider(newCachingSummaryProvider(summaryProvider{portfolioSvc}))
	profileSvc.SetAchievementProvider(achievementsSvc)
	profileSvc.SetSprintRankProvider(competitionsSvc)
	profileSvc.SetGlobalRankProvider(leaderboardSvc)
	profileSvc.SetTimeframeRankProvider(profileTimeframeRankAdapter{leaderboardSvc})
	profileSvc.SetPerformanceHistoryProvider(profileHistoryAdapter{historySvc})
	profileSvc.SetRankedPerformanceProvider(profileRankedAdapter{performanceSvc})
	// Enrich leaderboard rows with public profile data (handle/tag/weights).
	leaderboardSvc.SetProfileProvider(leaderboardProfileAdapter{profileSvc})
	// Deleting an account also unpublishes its profile, so it stops appearing
	// on Explore or via a direct handle lookup (those read the profile
	// repository directly and never check whether the account still exists).
	authSvc.RegisterDeletionHook(profileSvc)
	for _, candidate := range []any{
		repos.portfolio, repos.history, repos.competitions,
		repos.achievements, repos.profiles, repos.social,
		repos.safety, repos.moderation,
	} {
		if eraser, ok := candidate.(auth.AccountDeletionHook); ok {
			authSvc.RegisterDeletionHook(eraser)
		}
	}

	strategySvc := strategy.NewService(profileSvc, portfolioSvc)
	socialSvc := social.NewService(repos.social, repos.profiles)

	// Social-safety layer: blocking + the canonical interaction-policy gate,
	// and reporting/moderation. safetySvc.CanUsersInteract is the single
	// source of truth wired into follow/DM (social), and blocked-pair
	// filtering is wired into social's following/followers/friends/
	// conversations lists and profile's public-view/Explore.
	safetySvc := safety.NewService(repos.safety, repos.profiles)
	safetySvc.SetFollowRemover(socialSvc)
	safetySvc.SetUserStatusProvider(authAdminAdapter{authSvc})
	if repos.blockCoordinator != nil {
		safetySvc.SetBlockCoordinator(repos.blockCoordinator)
	}

	moderationSvc := moderation.NewService(repos.moderation, authAdminAdapter{authSvc})
	moderationSvc.SetMessageAccessor(moderationMessageAdapter{socialSvc})
	moderationSvc.SetMessageRemover(socialSvc)

	socialSvc.SetInteractionPolicy(safetySvc)
	socialSvc.SetBlockedFilter(safetySvc)
	socialSvc.SetNotificationCreator(moderationSvc)
	profileSvc.SetBlockedFilter(safetySvc)

	if err := authSvc.EnsureBootstrapAdmin(ctx, cfg.AdminBootstrapEmail); err != nil {
		slog.Warn("admin bootstrap failed", "error", err)
	}

	// Ranking-epoch backfill: initialize persistent ranked state at index 100 for
	// every existing portfolio so their new epoch begins at deployment. Legacy
	// pre-epoch leaderboard snapshots are ignored by timeframe ranking. Idempotent
	// and best-effort — a user that already has ranked state is left untouched.
	if existingUsers, err := authSvc.ListUsers(ctx); err == nil {
		backfilled := 0
		for _, u := range existingUsers {
			if err := portfolioSvc.Coordinator().EnsureRankedEpoch(ctx, u.ID); err != nil {
				slog.Warn("ranked-epoch backfill failed for user", "user_id", u.ID, "error", err)
				continue
			}
			if _, err := historySvc.RecordCurrentTransition(ctx, u.ID); err != nil {
				slog.Warn("initial ranked snapshot failed for user", "user_id", u.ID, "error", err)
			}
			backfilled++
		}
		if backfilled > 0 {
			slog.Info("ranked-performance epoch initialized for existing portfolios", "count", backfilled)
		}
	} else {
		slog.Warn("ranked-epoch backfill skipped: list users failed", "error", err)
	}

	// --- sprint leaderboard cache (Redis only) ---
	// The global leaderboard no longer uses Redis at all: it is served solely
	// from the generation-based Postgres ranking projection (stale-tolerant by
	// design — see leaderboard.Service.useRanking). Redis remains for sprint
	// competition rankings, and its deletion hook still erases every
	// leaderboard:* key on account deletion.
	var rankedCache *leaderboard.RedisLeaderboardCache
	if redisClient != nil {
		rankedCache = leaderboard.NewRedisLeaderboardCache(redisClient)
		competitionsSvc.SetCache(rankedCache)
		authSvc.RegisterDeletionHook(rankedCache)
	}

	// --- transactional outbox processor ---
	// Derived work (lifecycle snapshots, daily archives) runs AFTER the
	// mutation commits, driven by durable events written inside the same
	// transaction. A projection failure retries; it can never roll back or fail a
	// portfolio mutation.
	store, eventSourceOK := repos.portfolio.(jobs.EventSource)
	mandatoryProjection := cfg.StorageProvider == "postgres"
	optionalJobs := shouldStartOptionalJobs(cfg.EnableBackgroundWorkers)
	if mandatoryProjection && !eventSourceOK {
		slog.Error("mandatory portfolio projector cannot be constructed: repository has no outbox event source")
		os.Exit(1)
	}
	if eventSourceOK && shouldStartOutbox(cfg.StorageProvider, optionalJobs) {
		outboxProcessor := jobs.NewOutboxProcessor(store, cfg.LeaderboardRefreshInterval)
		outboxProcessor.SetRankedSnapshotRecorder(rankedMutationSnapshotAdapter{history: historySvc})
		if optionalJobs {
			outboxProcessor.SetSnapshotRecorder(portfolioSvc)
		}
		outboxProcessor.Start(ctx)
		slog.Info("mandatory portfolio outbox projector started", "postgres_mandatory", mandatoryProjection)
		if mandatoryProjection {
			backlog, ok := repos.portfolio.(jobs.BacklogSource)
			if !ok {
				slog.Error("mandatory portfolio projector readiness cannot be constructed")
				os.Exit(1)
			}
			readinessChecks = append(readinessChecks, server.ReadinessCheck{
				Name: "projector",
				Check: func(ctx context.Context) error {
					return jobs.CheckProjectorReadiness(ctx, outboxProcessor.Running(), backlog,
						int64(cfg.OutboxReadinessMaxPending), cfg.OutboxReadinessMaxAge)
				},
				Details: func(ctx context.Context) map[string]string {
					pending, age, err := backlog.OutboxBacklog(ctx)
					if err != nil {
						return map[string]string{"projector_running": strconv.FormatBool(outboxProcessor.Running())}
					}
					return map[string]string{
						"projector_running":         strconv.FormatBool(outboxProcessor.Running()),
						"pending_outbox_count":      strconv.FormatInt(pending, 10),
						"oldest_pending_outbox_age": age.String(),
					}
				},
			})
			readinessChecks = append(readinessChecks, server.ReadinessCheck{
				Name: "outbox_backlog",
				Check: func(ctx context.Context) error {
					_, _, err := backlog.OutboxBacklog(ctx)
					return err
				},
			})
		}
	}

	// --- background workers ---
	// leaderboardSvc's own RefreshCache batching (SetRefreshBatchSize below)
	// bounds per-tick valuation cost; the leader elector on top of that
	// ensures only one replica pays that bounded cost at a time, rather than
	// every replica doing its own full unpaginated pass. It's non-nil only
	// for the postgres provider (repos.pgPool) — a nil *leaderlock.Elector is
	// treated as "always leader", so memory-mode/single-instance dev is
	// unaffected. The outbox and ranked-snapshot workers below are not gated:
	// they already claim work per-row via SELECT ... FOR UPDATE SKIP LOCKED /
	// evaluation claims and are safe across replicas without coordination.
	var leader *leaderlock.Elector
	if repos.pgPool != nil {
		leader = leaderlock.New(repos.pgPool)
		go leader.Run(ctx)
	}
	leaderboardSvc.SetRefreshBatchSize(cfg.RefreshBatchSize)
	leaderboardSvc.SetRefreshParallelism(cfg.RefreshParallelism)
	if optionalJobs {
		worker := jobs.NewWorker(leaderboardSvc, competitionsSvc, cfg.LeaderboardRefreshInterval)
		worker.SetLeaderElector(leader)
		worker.SetExploreProjectionRefresher(profileSvc)
		// Private daily portfolio archives remain available for owner analytics;
		// ranked achievements use the independent canonical snapshot worker below.
		worker.SetPortfolioSnapshotter(portfolioSnapshotAdapter{
			portfolio: portfolioSvc,
			walker:    &userBatchWalker{users: authSvc, batch: cfg.RefreshBatchSize},
		})
		worker.Start(ctx)
		rankedWorker := jobs.NewRankedSnapshotWorker(rankedSnapshotJobAdapter{
			walker:       &userBatchWalker{users: authSvc, batch: cfg.RefreshBatchSize},
			history:      historySvc,
			achievements: achievementsSvc,
		}, cfg.RankedSnapshotInterval)
		rankedWorker.Start(ctx)
	} else {
		slog.Info("background workers disabled (ENABLE_BACKGROUND_WORKERS=false)")
	}
	if cfg.EnableQuoteRefreshWorker {
		quoteWorker := marketdata.NewQuoteRefreshWorker(marketDataSvc, repos.portfolio, cfg.QuoteRefreshInterval)
		quoteWorker.SetLeaderElector(leader)
		quoteWorker.Start(ctx)
		slog.Info("quote refresh worker enabled", "interval", cfg.QuoteRefreshInterval.String())
	} else {
		slog.Info("quote refresh worker disabled (ENABLE_QUOTE_REFRESH_WORKER=false)")
	}

	// Automatic corporate-action pipeline. Users never enter corporate actions;
	// the worker ingests provider events and applies routine transformations
	// (splits, ticker changes) automatically through the aggregate coordinator.
	// The manual development provider needs no API keys, so the pipeline runs
	// offline; swap in a real provider adapter via configuration.
	// External provider integrations are intended for personal / local / internal
	// use; free-tier provider terms do not cover public commercial
	// redistribution. DATA_USAGE_MODE documents the intent only.
	slog.Info("provider data usage mode", "mode", cfg.DataUsageMode)
	providers := providerfactory.New(cfg, nil)
	var corpActionView portfolio.CorporateActionViewReader
	if cfg.CorporateActionsEnabled {
		corpProvider, err := providers.CorporateActionProvider()
		if err != nil {
			slog.Error("corporate-action provider configuration invalid", "error", err)
			os.Exit(1)
		}
		var corpStore corpactions.Store = corpActionStorer
		if corpStore == nil {
			corpStore = corpactions.NewInMemoryStore()
		}
		if eraser, ok := corpStore.(auth.AccountDeletionHook); ok {
			authSvc.RegisterDeletionHook(eraser)
		}
		corpSvc := corpactions.NewService(corpProvider, corpStore, corpActionGateway{svc: portfolioSvc})
		corpSvc.SetLookback(cfg.CorporateActionLookback)
		corpSvc.SetInstrumentResolver(identityResolver)
		corpActionView = corpActionViewAdapter{svc: corpSvc}
		if optionalJobs {
			jobs.NewCorporateActionWorker(corpSvc, cfg.CorporateActionPollInterval).Start(ctx)
			slog.Info("corporate-action pipeline enabled",
				"primary_provider", cfg.CorporateActionPrimary,
				"poll_interval", cfg.CorporateActionPollInterval.String())
		}
	} else {
		slog.Info("corporate-action pipeline disabled (CORPORATE_ACTIONS_ENABLED=false)")
	}

	// Automatic provider-driven income pipeline. Users never enter ordinary
	// dividends or distributions; the worker ingests provider income events and
	// credits cash (or reinvested shares) automatically through the aggregate
	// coordinator, using HISTORICAL holdings for entitlement. The manual
	// development provider needs no API keys, so the pipeline runs offline; swap in
	// a real provider adapter via configuration.
	var incomeView portfolio.IncomeEventViewReader
	if cfg.IncomeTrackingEnabled {
		incomeProvider, err := providers.IncomeProvider()
		if err != nil {
			slog.Error("income provider configuration invalid", "error", err)
			os.Exit(1)
		}
		var incomeStore income.Store = incomeStorer
		if incomeStore == nil {
			incomeStore = income.NewInMemoryStore()
		}
		if eraser, ok := incomeStore.(auth.AccountDeletionHook); ok {
			authSvc.RegisterDeletionHook(eraser)
		}
		incomeSvc := income.NewService(incomeProvider, incomeStore, incomeGateway{svc: portfolioSvc})
		incomeSvc.SetLookback(cfg.IncomeLookback)
		incomeSvc.SetRetryInterval(cfg.IncomeRetryInterval)
		incomeSvc.SetPreferences(income.Preferences{
			ReinvestByDefault: cfg.IncomeReinvestByDefault,
			UseEstimatedGross: cfg.IncomeUseEstimatedGross,
			Withholding:       income.WithholdingProfile{DefaultRate: money.RatioFromFloat64(cfg.IncomeWithholdingDefault)},
		})
		incomeView = incomeViewAdapter{svc: incomeSvc}
		if optionalJobs {
			jobs.NewIncomeWorker(incomeSvc, cfg.IncomePollInterval).Start(ctx)
			slog.Info("income pipeline enabled",
				"primary_provider", cfg.IncomePrimaryProvider,
				"poll_interval", cfg.IncomePollInterval.String(),
				"reinvest_by_default", cfg.IncomeReinvestByDefault)
		}
	} else {
		slog.Info("income pipeline disabled (INCOME_TRACKING_ENABLED=false)")
	}

	handler := server.New(server.Deps{
		Auth:                        authSvc,
		Tokens:                      tokens,
		Portfolio:                   portfolioSvc,
		Leaderboard:                 leaderboardSvc,
		Competitions:                competitionsSvc,
		CompetitionAdmin:            competitionAdminSvc,
		Achievements:                achievementsSvc,
		Profile:                     profileSvc,
		Strategy:                    strategySvc,
		MarketData:                  marketDataSvc,
		Social:                      socialSvc,
		Safety:                      safetySvc,
		Moderation:                  moderationSvc,
		PerformanceHistory:          historySvc,
		CorporateActionView:         corpActionView,
		IncomeEventView:             incomeView,
		ReadinessChecks:             readinessChecks,
		AppEnv:                      cfg.AppEnv,
		CORSAllowedOrigins:          cfg.CORSAllowedOrigins,
		TrustedProxyCIDRs:           cfg.TrustedProxyCIDRs,
		RateLimitRedis:              redisClient,
		DisablePasswordRegistration: !cfg.PasswordRegistrationEnabled,
		Info: map[string]string{
			"storage_provider": cfg.StorageProvider,
			"price_provider":   priceProviderName,
			"real_market_data": strconv.FormatBool(cfg.EnableRealMarketData && priceProviderName == "twelvedata"),
		},
	})
	outboxAdmin, _ := repos.portfolio.(server.OutboxAdmin)
	operationsHandler := server.NewOperations(server.Deps{
		ReadinessChecks: readinessChecks,
		Info: map[string]string{
			"storage_provider": cfg.StorageProvider,
			"price_provider":   priceProviderName,
			"real_market_data": strconv.FormatBool(cfg.EnableRealMarketData && priceProviderName == "twelvedata"),
		},
		OutboxAdmin: outboxAdmin,
	})

	slog.Info("finance_app API starting",
		"app_env", cfg.AppEnv,
		"port", cfg.Port,
		"operations_addr", cfg.OperationsAddr,
		"storage_provider", cfg.StorageProvider,
		"price_provider", priceProviderName,
		"real_market_data", cfg.EnableRealMarketData && priceProviderName == "twelvedata",
		"redis_enabled", redisClient != nil,
		"background_workers", cfg.EnableBackgroundWorkers,
		"google_auth_enabled", cfg.GoogleAuthEnabled,
		"apple_auth_enabled", cfg.AppleAuthEnabled,
		"quote_refresh_interval", cfg.QuoteRefreshInterval.String(),
		"quote_cache_ttl", cfg.QuoteCacheTTL.String(),
	)
	// Explicit timeouts close the Slowloris-style gap of a bare
	// http.ListenAndServe (an idle/slow client can otherwise hold a connection
	// open indefinitely, exhausting server file descriptors). WriteTimeout is
	// long enough for the slower aggregate/leaderboard endpoints without
	// letting any single request hang forever.
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	operationsSrv := &http.Server{
		Addr:              cfg.OperationsAddr,
		Handler:           operationsHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serveErr := make(chan error, 2)
	startServer := func(name string, target *http.Server) {
		if err := target.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- fmt.Errorf("%s server: %w", name, err)
			return
		}
		serveErr <- nil
	}
	go startServer("public", srv)
	go startServer("operations", operationsSrv)

	select {
	case err := <-serveErr:
		if err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received; draining in-flight requests")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		if err := operationsSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("operations graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		slog.Info("server shut down cleanly")
	}
}
