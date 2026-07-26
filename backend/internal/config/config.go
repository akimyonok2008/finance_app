package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Default values used for local development when env vars are unset. The JWT
// secret MUST be overridden in production (see README).
const (
	defaultPort                   = "8080"
	defaultJWTSecret              = "dev-secret-change-me"
	defaultJWTExpiryHr            = 24
	defaultPriceProvider          = "mock"
	defaultStorageProvider        = "memory"
	defaultAppEnv                 = "development"
	defaultLeaderboardSecs        = 60
	defaultPriceCacheSecs         = 300
	defaultQuoteRefreshSecs       = 600
	defaultQuoteCacheSecs         = 600
	defaultTwelveRPM              = 6
	defaultTwelveDaily            = 500
	defaultTwelveTimeout          = 10
	defaultBaseCurrency           = "USD"
	defaultEnableBackground       = false
	defaultRankedSnapshotInterval = 4 * time.Hour
	defaultRankedRetentionDays    = 120
	defaultBenchmarkAwardMode     = "verified_only"
	defaultAlpacaDataBaseURL      = "https://data.alpaca.markets"
	defaultFMPBaseURL             = "https://financialmodelingprep.com/stable"
	defaultProviderTimeout        = 10 * time.Second
	defaultFMPDailyBudget         = 250
	defaultDataUsageMode          = "personal"
	defaultOpenFIGIBaseURL        = "https://api.openfigi.com"
)

// Config holds runtime configuration sourced from the environment.
type Config struct {
	AppEnv          string
	Port            string
	JWTSecret       string
	JWTExpiry       time.Duration
	PriceProvider   string
	StorageProvider string // "memory" or "postgres"
	DatabaseURL     string
	RedisURL        string // empty disables Redis
	BaseCurrency    string

	EnableBackgroundWorkers        bool
	LeaderboardRefreshInterval     time.Duration
	PriceCacheTTL                  time.Duration
	EnableRealMarketData           bool
	TwelveDataAPIKey               string
	TwelveDataBaseURL              string
	QuoteRefreshInterval           time.Duration
	QuoteCacheTTL                  time.Duration
	TwelveDataMaxRequestsPerMinute int
	TwelveDataDailyRequestBudget   int
	TwelveDataRequestTimeout       time.Duration
	EnableQuoteRefreshWorker       bool
	QuoteStaleAfter                time.Duration
	QuoteAllowStaleOnProviderError bool
	RankedSnapshotInterval         time.Duration
	RankedSnapshotRetention        time.Duration
	RankedBoundaryTolerance        time.Duration
	RankedEndFreshness             time.Duration
	RankedActiveCoverage           float64

	// BenchmarkAwardMode is the permanent-award policy for benchmark badges:
	// "disabled", "demo", or "verified_only" (default). It is independent of the
	// price provider so production status is never inferred from an API key.
	BenchmarkAwardMode string

	// Automatic corporate-action pipeline.
	CorporateActionsEnabled      bool
	CorporateActionPrimary       string
	CorporateActionFallback      string
	CorporateActionReference     string
	CorporateActionPollInterval  time.Duration
	CorporateActionLookback      time.Duration
	CorporateActionRetryInterval time.Duration

	// Automatic provider-driven income pipeline.
	IncomeTrackingEnabled     bool
	IncomePrimaryProvider     string
	IncomeFallbackProvider    string
	IncomePollInterval        time.Duration
	IncomeLookback            time.Duration
	IncomeApplicationInterval time.Duration
	IncomeRetryInterval       time.Duration
	IncomeReinvestByDefault   bool
	IncomeUseEstimatedGross   bool
	IncomeWithholdingDefault  float64

	// Alpaca market data (corporate actions). Credentials are required only when
	// a provider selection actually names "alpaca".
	AlpacaMarketDataEnabled bool
	AlpacaAPIKeyID          string
	AlpacaAPISecretKey      string
	AlpacaDataBaseURL       string
	AlpacaRequestTimeout    time.Duration

	// Financial Modeling Prep (dividends / splits).
	FMPEnabled            bool
	FMPAPIKey             string
	FMPBaseURL            string
	FMPRequestTimeout     time.Duration
	FMPDailyRequestBudget int

	// DataUsageMode documents the intended usage of the external provider
	// integrations ("personal" by default). Free-tier provider terms do not
	// cover public commercial redistribution; nothing gates on this value today
	// beyond a startup log line.
	DataUsageMode string

	// OpenFIGI instrument identity resolution. The API key is OPTIONAL:
	// OpenFIGI permits unauthenticated low-volume use. It is never logged.
	OpenFIGIEnabled        bool
	OpenFIGIAPIKey         string
	OpenFIGIBaseURL        string
	OpenFIGIRequestTimeout time.Duration

	GoogleAuthEnabled bool
	GoogleClientID    string
	AppleAuthEnabled  bool
	AppleClientID     string
	AppleTeamID       string
	AppleKeyID        string
	ApplePrivateKey   string
	AppleRedirectURI  string
}

// Load reads configuration from environment variables, falling back to
// development-friendly defaults when a variable is missing or invalid.
func Load() Config {
	loadDotEnvFiles(envFileCandidates()...)

	return Config{
		AppEnv:          getEnv("APP_ENV", defaultAppEnv),
		Port:            getEnv("PORT", defaultPort),
		JWTSecret:       getEnv("JWT_SECRET", defaultJWTSecret),
		JWTExpiry:       time.Duration(getEnvInt("JWT_EXPIRY_HOURS", defaultJWTExpiryHr)) * time.Hour,
		PriceProvider:   getEnv("PRICE_PROVIDER", defaultPriceProvider),
		StorageProvider: getEnv("STORAGE_PROVIDER", defaultStorageProvider),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable"),
		RedisURL:        getEnv("REDIS_URL", ""),
		BaseCurrency:    getEnv("BASE_CURRENCY", defaultBaseCurrency),

		EnableBackgroundWorkers:        getEnvBool("ENABLE_BACKGROUND_WORKERS", defaultEnableBackground),
		LeaderboardRefreshInterval:     time.Duration(getEnvInt("LEADERBOARD_REFRESH_INTERVAL_SECONDS", defaultLeaderboardSecs)) * time.Second,
		PriceCacheTTL:                  time.Duration(getEnvInt("PRICE_CACHE_TTL_SECONDS", defaultPriceCacheSecs)) * time.Second,
		EnableRealMarketData:           getEnvBool("ENABLE_REAL_MARKET_DATA", false),
		TwelveDataAPIKey:               getEnv("TWELVE_DATA_API_KEY", ""),
		TwelveDataBaseURL:              getEnv("TWELVE_DATA_BASE_URL", "https://api.twelvedata.com"),
		QuoteRefreshInterval:           time.Duration(getEnvInt("QUOTE_REFRESH_INTERVAL_SECONDS", defaultQuoteRefreshSecs)) * time.Second,
		QuoteCacheTTL:                  time.Duration(getEnvInt("QUOTE_CACHE_TTL_SECONDS", defaultQuoteCacheSecs)) * time.Second,
		TwelveDataMaxRequestsPerMinute: getEnvInt("TWELVE_DATA_MAX_REQUESTS_PER_MINUTE", defaultTwelveRPM),
		TwelveDataDailyRequestBudget:   getEnvInt("TWELVE_DATA_DAILY_REQUEST_BUDGET", defaultTwelveDaily),
		TwelveDataRequestTimeout:       time.Duration(getEnvInt("TWELVE_DATA_REQUEST_TIMEOUT_SECONDS", defaultTwelveTimeout)) * time.Second,
		EnableQuoteRefreshWorker:       getEnvBool("ENABLE_QUOTE_REFRESH_WORKER", false),
		QuoteStaleAfter:                time.Duration(getEnvInt("QUOTE_STALE_AFTER_SECONDS", 900)) * time.Second,
		QuoteAllowStaleOnProviderError: getEnvBool("QUOTE_ALLOW_STALE_ON_PROVIDER_ERROR", true),
		RankedSnapshotInterval:         getEnvDuration("RANKED_SNAPSHOT_INTERVAL", defaultRankedSnapshotInterval),
		RankedSnapshotRetention:        time.Duration(getEnvInt("RANKED_INTRADAY_RETENTION_DAYS", defaultRankedRetentionDays)) * 24 * time.Hour,
		RankedBoundaryTolerance:        getEnvDuration("RANKED_BOUNDARY_TOLERANCE", 36*time.Hour),
		RankedEndFreshness:             getEnvDuration("RANKED_END_FRESHNESS", 8*time.Hour),
		RankedActiveCoverage:           getEnvFloat("RANKED_ACTIVE_COVERAGE_THRESHOLD", 0.90),
		BenchmarkAwardMode:             getEnv("BENCHMARK_AWARD_MODE", defaultBenchmarkAwardMode),
		CorporateActionsEnabled:        getEnvBool("CORPORATE_ACTIONS_ENABLED", false),
		CorporateActionPrimary:         getEnv("CORPORATE_ACTION_PRIMARY_PROVIDER", "manual_dev"),
		CorporateActionFallback:        getEnv("CORPORATE_ACTION_FALLBACK_PROVIDER", ""),
		CorporateActionReference:       getEnv("CORPORATE_ACTION_REFERENCE_PROVIDER", "twelve_data"),
		CorporateActionPollInterval:    getEnvDuration("CORPORATE_ACTION_POLL_INTERVAL", 24*time.Hour),
		CorporateActionLookback:        getEnvDuration("CORPORATE_ACTION_LOOKBACK", 7*24*time.Hour),
		CorporateActionRetryInterval:   getEnvDuration("CORPORATE_ACTION_RETRY_INTERVAL", time.Hour),

		IncomeTrackingEnabled:     getEnvBool("INCOME_TRACKING_ENABLED", false),
		IncomePrimaryProvider:     getEnv("INCOME_PRIMARY_PROVIDER", "manual_dev"),
		IncomeFallbackProvider:    getEnv("INCOME_FALLBACK_PROVIDER", ""),
		IncomePollInterval:        getEnvDuration("INCOME_EVENT_POLL_INTERVAL", 24*time.Hour),
		IncomeLookback:            getEnvDuration("INCOME_EVENT_LOOKBACK", 14*24*time.Hour),
		IncomeApplicationInterval: getEnvDuration("INCOME_APPLICATION_INTERVAL", time.Hour),
		IncomeRetryInterval:       getEnvDuration("INCOME_RETRY_INTERVAL", time.Hour),
		IncomeReinvestByDefault:   getEnvBool("INCOME_REINVEST_BY_DEFAULT", false),
		IncomeUseEstimatedGross:   getEnvBool("INCOME_USE_ESTIMATED_GROSS", true),
		IncomeWithholdingDefault:  getEnvFloat("INCOME_WITHHOLDING_DEFAULT_RATE", 0),

		AlpacaMarketDataEnabled: getEnvBool("ALPACA_MARKET_DATA_ENABLED", false),
		AlpacaAPIKeyID:          getEnv("ALPACA_API_KEY_ID", ""),
		AlpacaAPISecretKey:      getEnv("ALPACA_API_SECRET_KEY", ""),
		AlpacaDataBaseURL:       getEnv("ALPACA_DATA_BASE_URL", defaultAlpacaDataBaseURL),
		AlpacaRequestTimeout:    getEnvDuration("ALPACA_REQUEST_TIMEOUT", defaultProviderTimeout),

		FMPEnabled:            getEnvBool("FMP_ENABLED", false),
		FMPAPIKey:             getEnv("FMP_API_KEY", ""),
		FMPBaseURL:            getEnv("FMP_BASE_URL", defaultFMPBaseURL),
		FMPRequestTimeout:     getEnvDuration("FMP_REQUEST_TIMEOUT", defaultProviderTimeout),
		FMPDailyRequestBudget: getEnvInt("FMP_DAILY_REQUEST_BUDGET", defaultFMPDailyBudget),

		DataUsageMode: getEnv("DATA_USAGE_MODE", defaultDataUsageMode),

		OpenFIGIEnabled:        getEnvBool("OPENFIGI_ENABLED", false),
		OpenFIGIAPIKey:         getEnv("OPENFIGI_API_KEY", ""),
		OpenFIGIBaseURL:        getEnv("OPENFIGI_BASE_URL", defaultOpenFIGIBaseURL),
		OpenFIGIRequestTimeout: getEnvDuration("OPENFIGI_REQUEST_TIMEOUT", defaultProviderTimeout),

		GoogleAuthEnabled: getEnvBool("GOOGLE_AUTH_ENABLED", false),
		GoogleClientID:    getEnv("GOOGLE_CLIENT_ID", ""),
		AppleAuthEnabled:  getEnvBool("APPLE_AUTH_ENABLED", false),
		AppleClientID:     getEnv("APPLE_CLIENT_ID", ""),
		AppleTeamID:       getEnv("APPLE_TEAM_ID", ""),
		AppleKeyID:        getEnv("APPLE_KEY_ID", ""),
		ApplePrivateKey:   getEnv("APPLE_PRIVATE_KEY", ""),
		AppleRedirectURI:  getEnv("APPLE_REDIRECT_URI", ""),
	}
}

func envFileCandidates() []string {
	paths := []string{}
	if custom := strings.TrimSpace(os.Getenv("FINANCE_APP_ENV_FILE")); custom != "" {
		paths = append(paths, custom)
	}
	return append(paths,
		".env",         // cd backend && go run ./cmd/api
		"../.env",      // cd backend with shared root config
		"backend/.env", // go run ./backend/cmd/api from repo root
	)
}

// UsingDefaultSecret reports whether the insecure development secret is in use,
// so the server can warn at startup.
func (c Config) UsingDefaultSecret() bool {
	return c.JWTSecret == defaultJWTSecret
}

func (c Config) Validate() error {
	if c.GoogleAuthEnabled && strings.TrimSpace(c.GoogleClientID) == "" {
		return fmt.Errorf("GOOGLE_AUTH_ENABLED=true requires GOOGLE_CLIENT_ID")
	}
	if c.AppleAuthEnabled && strings.TrimSpace(c.AppleClientID) == "" {
		return fmt.Errorf("APPLE_AUTH_ENABLED=true requires APPLE_CLIENT_ID")
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if value, ok := os.LookupEnv(key); ok {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func loadDotEnvFiles(paths ...string) {
	for _, path := range paths {
		loadDotEnvFile(path)
	}
}

func loadDotEnvFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseDotEnvLine(scanner.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}

func parseDotEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" || strings.ContainsAny(key, " \t") {
		return "", "", false
	}
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}
