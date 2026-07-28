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
	// defaultRefreshBatchSize bounds how many users a single leaderboard-cache
	// refresh or daily-snapshot pass processes per tick, so either job's cost
	// stays bounded regardless of how many users the platform has (see
	// leaderboard.Service.SetRefreshBatchSize and portfolioSnapshotAdapter in
	// cmd/api/main.go). 0 would mean unbounded; the default here is always a
	// real cap rather than opt-in, since both jobs run on a short ticker.
	defaultRefreshBatchSize = 500
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
	DemoModeEnabled bool

	// CORSAllowedOrigins is the explicit cross-origin allow-list
	// (CORS_ALLOWED_ORIGINS, comma-separated). Empty means "no explicit
	// allow-list configured" — the server then falls back to "*" outside
	// production, or to no CORS headers at all in production.
	CORSAllowedOrigins []string

	EnableBackgroundWorkers    bool
	OutboxReadinessMaxPending  int
	OutboxReadinessMaxAge      time.Duration
	LeaderboardRefreshInterval time.Duration
	// RefreshBatchSize bounds per-tick work for the leaderboard-cache refresh
	// and daily-snapshot jobs (see defaultRefreshBatchSize above). Shared
	// between the two since both are "full valuation over all users" jobs
	// with the identical scaling problem.
	RefreshBatchSize               int
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

	PasswordRegistrationEnabled bool
	PublicAppURL                string
	EmailSender                 string
	SMTPHost                    string
	SMTPPort                    string
	SMTPUsername                string
	SMTPPassword                string
	SMTPFrom                    string
	EmailVerificationTTL        time.Duration
	PasswordResetTTL            time.Duration
	ReauthenticationTTL         time.Duration

	// AdminBootstrapEmail, when set, is promoted to role=admin the first time
	// that account logs in or registers (see auth.Service). Empty disables
	// bootstrap entirely — role assignment then requires a direct database
	// action, never a public API.
	AdminBootstrapEmail string
}

// Load reads configuration from environment variables, falling back to
// development-friendly defaults when a variable is missing or invalid.
func Load() Config {
	loadDotEnvFiles(envFileCandidates()...)

	return Config{
		AppEnv:             getEnv("APP_ENV", defaultAppEnv),
		CORSAllowedOrigins: getEnvList("CORS_ALLOWED_ORIGINS"),
		Port:               getEnv("PORT", defaultPort),
		JWTSecret:          getEnv("JWT_SECRET", defaultJWTSecret),
		JWTExpiry:          time.Duration(getEnvInt("JWT_EXPIRY_HOURS", defaultJWTExpiryHr)) * time.Hour,
		PriceProvider:      getEnv("PRICE_PROVIDER", defaultPriceProvider),
		StorageProvider:    getEnv("STORAGE_PROVIDER", defaultStorageProvider),
		DatabaseURL:        getEnv("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable"),
		RedisURL:           getEnv("REDIS_URL", ""),
		BaseCurrency:       getEnv("BASE_CURRENCY", defaultBaseCurrency),
		DemoModeEnabled:    getEnvBool("DEMO_MODE_ENABLED", false),

		EnableBackgroundWorkers:        getEnvBool("ENABLE_BACKGROUND_WORKERS", defaultEnableBackground),
		OutboxReadinessMaxPending:      getEnvInt("OUTBOX_READINESS_MAX_PENDING", 1000),
		OutboxReadinessMaxAge:          getEnvDuration("OUTBOX_READINESS_MAX_AGE", 15*time.Minute),
		LeaderboardRefreshInterval:     time.Duration(getEnvInt("LEADERBOARD_REFRESH_INTERVAL_SECONDS", defaultLeaderboardSecs)) * time.Second,
		RefreshBatchSize:               getEnvInt("REFRESH_BATCH_SIZE", defaultRefreshBatchSize),
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
		IncomeLookback:            getEnvDuration("INCOME_EVENT_LOOKBACK", 120*24*time.Hour),
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

		PasswordRegistrationEnabled: getEnvBool("PASSWORD_REGISTRATION_ENABLED", true),
		PublicAppURL:                getEnv("PUBLIC_APP_URL", "http://localhost:5173"),
		EmailSender:                 getEnv("EMAIL_SENDER", "development"),
		SMTPHost:                    getEnv("SMTP_HOST", ""),
		SMTPPort:                    getEnv("SMTP_PORT", "587"),
		SMTPUsername:                getEnv("SMTP_USERNAME", ""),
		SMTPPassword:                getEnv("SMTP_PASSWORD", ""),
		SMTPFrom:                    getEnv("SMTP_FROM", ""),
		EmailVerificationTTL:        getEnvDuration("EMAIL_VERIFICATION_TTL", 24*time.Hour),
		PasswordResetTTL:            getEnvDuration("PASSWORD_RESET_TTL", time.Hour),
		ReauthenticationTTL:         getEnvDuration("REAUTHENTICATION_TTL", 5*time.Minute),

		AdminBootstrapEmail: getEnv("ADMIN_BOOTSTRAP_EMAIL", ""),
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
	// A missing JWT_SECRET only warns in development (see UsingDefaultSecret's
	// caller), which makes a credential-less production deployment look
	// healthy while every token is forgeable from the public source. Refuse to
	// start instead, matching how the benchmark award policy already treats
	// APP_ENV=production as the one environment that must not silently
	// degrade to an insecure default.
	if strings.EqualFold(c.AppEnv, "production") {
		if c.UsingDefaultSecret() || len(strings.TrimSpace(c.JWTSecret)) < 32 ||
			isKnownWeakSecret(c.JWTSecret) {
			return fmt.Errorf("APP_ENV=production requires a strong, unique JWT_SECRET of at least 32 characters")
		}
		if !strings.EqualFold(strings.TrimSpace(c.StorageProvider), "postgres") {
			return fmt.Errorf("APP_ENV=production requires STORAGE_PROVIDER=postgres")
		}
		if strings.TrimSpace(c.DatabaseURL) == "" {
			return fmt.Errorf("APP_ENV=production requires DATABASE_URL")
		}
		if err := validateProductionCORS(c.CORSAllowedOrigins); err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(c.PriceProvider), "mock") || strings.TrimSpace(c.PriceProvider) == "" {
			if !c.DemoModeEnabled || !strings.EqualFold(c.BenchmarkAwardMode, "demo") {
				return fmt.Errorf("production mock prices require DEMO_MODE_ENABLED=true and BENCHMARK_AWARD_MODE=demo")
			}
		}
		if strings.EqualFold(strings.TrimSpace(c.PriceProvider), "yahoo") {
			return fmt.Errorf("APP_ENV=production cannot use the prototype PRICE_PROVIDER=yahoo")
		}
		if strings.EqualFold(strings.TrimSpace(c.PriceProvider), "twelvedata") &&
			(!c.EnableRealMarketData || strings.TrimSpace(c.TwelveDataAPIKey) == "") {
			return fmt.Errorf("production PRICE_PROVIDER=twelvedata requires ENABLE_REAL_MARKET_DATA=true and TWELVE_DATA_API_KEY")
		}
		if strings.EqualFold(c.BenchmarkAwardMode, "verified_only") &&
			!strings.EqualFold(strings.TrimSpace(c.BaseCurrency), "USD") {
			return fmt.Errorf("verified production benchmarks require BASE_CURRENCY=USD until a historical FX provider is configured")
		}
		if !c.EnableBackgroundWorkers {
			return fmt.Errorf("APP_ENV=production requires ENABLE_BACKGROUND_WORKERS=true")
		}
		if !c.EnableQuoteRefreshWorker {
			return fmt.Errorf("APP_ENV=production requires ENABLE_QUOTE_REFRESH_WORKER=true")
		}
		if c.PasswordRegistrationEnabled && !strings.EqualFold(c.EmailSender, "smtp") {
			return fmt.Errorf("production password registration requires EMAIL_SENDER=smtp")
		}
	}
	if strings.EqualFold(c.EmailSender, "smtp") &&
		(strings.TrimSpace(c.SMTPHost) == "" || strings.TrimSpace(c.SMTPFrom) == "") {
		return fmt.Errorf("EMAIL_SENDER=smtp requires SMTP_HOST and SMTP_FROM")
	}
	if strings.TrimSpace(c.EmailSender) != "" &&
		!strings.EqualFold(c.EmailSender, "smtp") &&
		!strings.EqualFold(c.EmailSender, "development") {
		return fmt.Errorf("EMAIL_SENDER must be development or smtp")
	}
	return nil
}

func isKnownWeakSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	switch value {
	case "", "secret", "password", "changeme", "change-me", "jwt-secret",
		"your-secret-here", "replace-me", "replace-with-secure-secret":
		return true
	default:
		for _, marker := range []string{"dev-secret", "change-me", "changeme", "example", "placeholder"} {
			if strings.Contains(value, marker) {
				return true
			}
		}
		return len(strings.Trim(value, string(value[0]))) == 0
	}
}

func validateProductionCORS(origins []string) error {
	if len(origins) == 0 {
		return fmt.Errorf("APP_ENV=production requires CORS_ALLOWED_ORIGINS")
	}
	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" || trimmed == "*" {
			return fmt.Errorf("production CORS_ALLOWED_ORIGINS must contain explicit origins and cannot use *")
		}
		if !strings.HasPrefix(trimmed, "https://") {
			return fmt.Errorf("production CORS origin %q must use https", trimmed)
		}
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// getEnvList splits a comma-separated env var into trimmed, non-empty
// entries. An unset or blank var yields nil (not an empty non-nil slice), so
// callers can treat "nil" as "no allow-list configured".
func getEnvList(key string) []string {
	raw, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
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
