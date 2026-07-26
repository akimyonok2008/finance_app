package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/benchmark"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/config"
	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/db"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/income"
	"github.com/ardakimyonok/finance_app/internal/jobs"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/marketdata"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/performancehistory"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
	"github.com/ardakimyonok/finance_app/internal/profile"
	"github.com/ardakimyonok/finance_app/internal/providerfactory"
	"github.com/ardakimyonok/finance_app/internal/server"
	"github.com/ardakimyonok/finance_app/internal/social"
	"github.com/ardakimyonok/finance_app/internal/strategy"
)

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
		out = append(out, benchmark.PricePoint{Date: bar.Date, AdjustedClose: bar.Close})
	}
	return out, nil
}

// portfolioSnapshotAdapter preserves the private daily cost-basis/composition
// archive. Ranked leaderboards, profiles, and achievements do not consume it.
type portfolioSnapshotAdapter struct {
	portfolio *portfolio.Service
	users     *auth.Service
}

func (a portfolioSnapshotAdapter) SnapshotAllDaily(ctx context.Context) (int, error) {
	users, err := a.users.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	recorded := 0
	for _, u := range users {
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
	users        *auth.Service
	history      *performancehistory.Service
	achievements *achievements.Service
}

func (a rankedSnapshotJobAdapter) SnapshotAll(ctx context.Context) performancehistory.BatchResult {
	result := performancehistory.BatchResult{}
	users, err := a.users.ListUsers(ctx)
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

type rankedCacheStateAdapter struct {
	users       *auth.Service
	performance *performance.Service
}

func (a rankedCacheStateAdapter) CurrentRankedCacheState(ctx context.Context, userID string) (jobs.RankedCacheState, bool, error) {
	if _, err := a.users.UserByID(userID); err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			return jobs.RankedCacheState{}, false, nil
		}
		return jobs.RankedCacheState{}, false, err
	}
	ranked, err := a.performance.CurrentRankedPerformance(ctx, userID)
	if err != nil {
		return jobs.RankedCacheState{}, false, err
	}
	return jobs.RankedCacheState{
		Active: ranked.Status == performance.StatusActive,
		Score:  ranked.RankedReturnPercentage,
	}, true, nil
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
		RankedIndex:            rp.RankedIndex,
		RankedReturnPercentage: rp.RankedReturnPercentage,
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
			PortfolioIndex:   point.RankedIndex,
			ReturnPercentage: point.RankedIndex - 100,
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
			RankedReturnPercentage: row.RankedReturnPercentage,
			RankedIndex:            row.RankedIndex,
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
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	ctx := context.Background()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	if cfg.UsingDefaultSecret() {
		slog.Warn("using the default development JWT secret; set JWT_SECRET before production")
	}

	// --- Redis (optional) ---
	var redisClient *redis.Client
	if cfg.RedisURL != "" {
		opts, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			slog.Error("invalid REDIS_URL", "error", err)
			os.Exit(1)
		}
		redisClient = redis.NewClient(opts)
		if err := redisClient.Ping(ctx).Err(); err != nil {
			slog.Error("redis connection failed", "error", err)
			os.Exit(1)
		}
		slog.Info("redis connected")
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
			users:        auth.NewPostgresUserRepository(pool),
			portfolio:    portfolio.NewPostgresRepository(pool),
			performance:  performance.NewPostgresStateReader(pool),
			history:      performancehistory.NewPostgresRepository(pool),
			competitions: competitions.NewPostgresCompetitionRepository(pool),
			achievements: achRepo,
			profiles:     profile.NewPostgresRepository(pool),
			marketdata:   marketdata.NewPostgresRepository(pool),
			social:       social.NewPostgresRepository(pool),
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
	if redisClient != nil {
		readinessChecks = append(readinessChecks, server.ReadinessCheck{
			Name:  "redis",
			Check: func(ctx context.Context) error { return redisClient.Ping(ctx).Err() },
		})
	}

	// --- services ---
	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTExpiry)
	authSvc := auth.NewService(repos.users, tokens)
	authSvc.ConfigureProviderAuth(auth.ProviderAuthConfig{
		GoogleEnabled:  cfg.GoogleAuthEnabled,
		AppleEnabled:   cfg.AppleAuthEnabled,
		GoogleVerifier: auth.NewGoogleVerifier(cfg.GoogleClientID),
		AppleVerifier:  auth.NewAppleVerifier(cfg.AppleClientID),
	})
	fxProvider := fx.NewMockFXProvider()
	portfolioSvc := portfolio.NewService(repos.portfolio, priceProvider, fxProvider)
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
	leaderboardSvc := leaderboard.NewService(authSvc, rankedPerformanceAdapter{performanceSvc})
	leaderboardSvc.SetSnapshotStore(historySvc)
	competitionsSvc := competitions.NewService(
		repos.competitions, userProvider{authSvc}, positionProvider{portfolioSvc},
		priceProvider, fxProvider, clock.RealClock{},
	)
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
	profileSvc.SetAchievementProvider(achievementsSvc)
	profileSvc.SetSprintRankProvider(competitionsSvc)
	profileSvc.SetGlobalRankProvider(leaderboardSvc)
	profileSvc.SetTimeframeRankProvider(profileTimeframeRankAdapter{leaderboardSvc})
	profileSvc.SetPerformanceHistoryProvider(profileHistoryAdapter{historySvc})
	profileSvc.SetRankedPerformanceProvider(profileRankedAdapter{performanceSvc})
	// Enrich leaderboard rows with public profile data (handle/tag/weights).
	leaderboardSvc.SetProfileProvider(leaderboardProfileAdapter{profileSvc})

	strategySvc := strategy.NewService(profileSvc, portfolioSvc)
	socialSvc := social.NewService(repos.social, repos.profiles)

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

	// --- leaderboard caches (Redis only) ---
	var rankedCache *leaderboard.RedisLeaderboardCache
	if redisClient != nil {
		rankedCache = leaderboard.NewRedisLeaderboardCache(redisClient)
		leaderboardSvc.SetCache(rankedCache)
		competitionsSvc.SetCache(rankedCache)
	}

	// --- transactional outbox processor ---
	// Derived work (leaderboard cache sync, daily archive snapshots) runs AFTER
	// the mutation commits, driven by durable events written inside the same
	// transaction. A projection failure retries; it can never roll back or fail a
	// portfolio mutation, and Redis being down cannot corrupt core state.
	if store, ok := repos.portfolio.(jobs.EventSource); ok && cfg.EnableBackgroundWorkers {
		outboxProcessor := jobs.NewOutboxProcessor(store, cfg.LeaderboardRefreshInterval)
		if rankedCache != nil {
			outboxProcessor.SetCache(rankedCache, rankedCacheStateAdapter{
				users: authSvc, performance: performanceSvc,
			})
		}
		outboxProcessor.SetSnapshotRecorder(portfolioSvc)
		outboxProcessor.SetRankedSnapshotRecorder(rankedMutationSnapshotAdapter{history: historySvc})
		outboxProcessor.Start(ctx)
		slog.Info("portfolio outbox processor started")
	}

	// --- background workers ---
	if cfg.EnableBackgroundWorkers {
		worker := jobs.NewWorker(leaderboardSvc, competitionsSvc, cfg.LeaderboardRefreshInterval)
		// Private daily portfolio archives remain available for owner analytics;
		// ranked achievements use the independent canonical snapshot worker below.
		worker.SetPortfolioSnapshotter(portfolioSnapshotAdapter{portfolio: portfolioSvc, users: authSvc})
		worker.Start(ctx)
		rankedWorker := jobs.NewRankedSnapshotWorker(rankedSnapshotJobAdapter{
			users: authSvc, history: historySvc, achievements: achievementsSvc,
		}, cfg.RankedSnapshotInterval)
		rankedWorker.Start(ctx)
	} else {
		slog.Info("background workers disabled (ENABLE_BACKGROUND_WORKERS=false)")
	}
	if cfg.EnableQuoteRefreshWorker {
		quoteWorker := marketdata.NewQuoteRefreshWorker(marketDataSvc, repos.portfolio, cfg.QuoteRefreshInterval)
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
		corpSvc := corpactions.NewService(corpProvider, corpStore, corpActionGateway{svc: portfolioSvc})
		corpSvc.SetLookback(cfg.CorporateActionLookback)
		corpActionView = corpActionViewAdapter{svc: corpSvc}
		if cfg.EnableBackgroundWorkers {
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
		incomeSvc := income.NewService(incomeProvider, incomeStore, incomeGateway{svc: portfolioSvc})
		incomeSvc.SetLookback(cfg.IncomeLookback)
		incomeSvc.SetRetryInterval(cfg.IncomeRetryInterval)
		incomeSvc.SetPreferences(income.Preferences{
			ReinvestByDefault: cfg.IncomeReinvestByDefault,
			UseEstimatedGross: cfg.IncomeUseEstimatedGross,
			Withholding:       income.WithholdingProfile{DefaultRate: cfg.IncomeWithholdingDefault},
		})
		incomeView = incomeViewAdapter{svc: incomeSvc}
		if cfg.EnableBackgroundWorkers {
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
		Auth:                authSvc,
		Tokens:              tokens,
		Portfolio:           portfolioSvc,
		Leaderboard:         leaderboardSvc,
		Competitions:        competitionsSvc,
		Achievements:        achievementsSvc,
		Profile:             profileSvc,
		Strategy:            strategySvc,
		MarketData:          marketDataSvc,
		Social:              socialSvc,
		CorporateActionView: corpActionView,
		IncomeEventView:     incomeView,
		ReadinessChecks:     readinessChecks,
		Info: map[string]string{
			"storage_provider": cfg.StorageProvider,
			"price_provider":   priceProviderName,
			"real_market_data": strconv.FormatBool(cfg.EnableRealMarketData && priceProviderName == "twelvedata"),
		},
	})

	slog.Info("finance_app API starting",
		"app_env", cfg.AppEnv,
		"port", cfg.Port,
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
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
