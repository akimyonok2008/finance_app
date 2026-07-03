package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/redis/go-redis/v9"

	"github.com/ardakimyonok/finance_app/internal/achievements"
	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/clock"
	"github.com/ardakimyonok/finance_app/internal/coach"
	"github.com/ardakimyonok/finance_app/internal/competitions"
	"github.com/ardakimyonok/finance_app/internal/config"
	"github.com/ardakimyonok/finance_app/internal/db"
	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/jobs"
	"github.com/ardakimyonok/finance_app/internal/leaderboard"
	"github.com/ardakimyonok/finance_app/internal/marketdata"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
	"github.com/ardakimyonok/finance_app/internal/prices"
	"github.com/ardakimyonok/finance_app/internal/profile"
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

func (p positionProvider) ListPositions(_ context.Context, userID string) ([]portfolio.Position, error) {
	ptrs, err := p.s.ListPositions(userID)
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
	competitions competitions.CompetitionRepository
	achievements achievements.AchievementRepository
	profiles     profile.Repository
	snapshots    leaderboard.SnapshotStore
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
		repos           repositories
		readinessChecks []server.ReadinessCheck
	)
	switch cfg.StorageProvider {
	case "memory":
		repos = repositories{
			users:        auth.NewInMemoryUserRepository(),
			portfolio:    portfolio.NewInMemoryRepository(),
			competitions: competitions.NewInMemoryCompetitionRepository(),
			achievements: achievements.NewInMemoryAchievementRepository(),
			profiles:     profile.NewInMemoryRepository(),
			snapshots:    leaderboard.NewInMemorySnapshotStore(),
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
			competitions: competitions.NewPostgresCompetitionRepository(pool),
			achievements: achRepo,
			profiles:     profile.NewPostgresRepository(pool),
			snapshots:    leaderboard.NewPostgresSnapshotStore(pool),
			marketdata:   marketdata.NewPostgresRepository(pool),
			social:       social.NewPostgresRepository(pool),
		}
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
	leaderboardSvc := leaderboard.NewService(authSvc, portfolioSvc)
	leaderboardSvc.SetSnapshotStore(repos.snapshots)
	competitionsSvc := competitions.NewService(
		repos.competitions, userProvider{authSvc}, positionProvider{portfolioSvc},
		priceProvider, fxProvider, clock.RealClock{},
	)
	achievementsSvc := achievements.NewService(
		repos.achievements, positionProvider{portfolioSvc}, summaryProvider{portfolioSvc},
		rankProvider{competitionsSvc},
	)
	achievementsSvc.SetCurrentCompetitionProvider(competitionsSvc)
	profileSvc := profile.NewService(repos.profiles, userProvider{authSvc}, summaryProvider{portfolioSvc})
	profileSvc.SetAchievementProvider(achievementsSvc)
	profileSvc.SetSprintRankProvider(competitionsSvc)
	profileSvc.SetGlobalRankProvider(leaderboardSvc)
	profileSvc.SetTimeframeRankProvider(profileTimeframeRankAdapter{leaderboardSvc})
	// Enrich leaderboard rows with public profile data (handle/tag/weights).
	leaderboardSvc.SetProfileProvider(leaderboardProfileAdapter{profileSvc})

	// --- AI Portfolio Coach ---
	// Mock is the default, key-free provider. A real provider is only used when
	// explicitly enabled AND implemented; until then we warn and fall back.
	var coachProvider coach.Provider = coach.NewMockProvider()
	if cfg.AIEnableRealProvider && cfg.AIProvider != "mock" && cfg.AIProvider != "" {
		slog.Warn("AI_ENABLE_REAL_PROVIDER set but no real provider is implemented yet; using mock",
			"requested_provider", cfg.AIProvider)
	}
	coachSvc := coach.NewService(authSvc, portfolioSvc, coachProvider)
	coachSvc.SetAchievementLister(achievementsSvc)
	coachSvc.SetProfileProvider(profileSvc)
	strategySvc := strategy.NewService(profileSvc, portfolioSvc)
	socialSvc := social.NewService(repos.social, repos.profiles)

	// --- leaderboard caches (Redis only) ---
	if redisClient != nil {
		cache := leaderboard.NewRedisLeaderboardCache(redisClient)
		leaderboardSvc.SetCache(cache)
		competitionsSvc.SetCache(cache)
	}

	// --- background workers ---
	if cfg.EnableBackgroundWorkers {
		worker := jobs.NewWorker(leaderboardSvc, competitionsSvc, cfg.LeaderboardRefreshInterval)
		worker.Start(ctx)
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

	handler := server.New(server.Deps{
		Auth:            authSvc,
		Tokens:          tokens,
		Portfolio:       portfolioSvc,
		Leaderboard:     leaderboardSvc,
		Competitions:    competitionsSvc,
		Achievements:    achievementsSvc,
		Coach:           coachSvc,
		Profile:         profileSvc,
		Strategy:        strategySvc,
		MarketData:      marketDataSvc,
		Social:          socialSvc,
		ReadinessChecks: readinessChecks,
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
