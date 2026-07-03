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
	defaultPort             = "8080"
	defaultJWTSecret        = "dev-secret-change-me"
	defaultJWTExpiryHr      = 24
	defaultPriceProvider    = "mock"
	defaultStorageProvider  = "memory"
	defaultAppEnv           = "development"
	defaultLeaderboardSecs  = 60
	defaultPriceCacheSecs   = 300
	defaultQuoteRefreshSecs = 600
	defaultQuoteCacheSecs   = 600
	defaultTwelveRPM        = 6
	defaultTwelveDaily      = 500
	defaultTwelveTimeout    = 10
	defaultBaseCurrency     = "USD"
	defaultEnableBackground = false
	defaultAIProvider       = "mock"
	defaultAITimeoutSecs    = 20
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

	// AI Portfolio Coach. Defaults are safe and key-free: the mock provider
	// runs locally with no external API. A real provider is only used when
	// AIEnableRealProvider is true AND AIProvider names a non-mock provider.
	AIProvider           string // "mock" (default); future: "openai", "gemini"
	AIModel              string
	AIAPIKey             string
	AIBaseURL            string
	AITimeout            time.Duration
	AIEnableRealProvider bool

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
	loadDotEnvFiles(".env", "backend/.env")

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

		AIProvider:           getEnv("AI_PROVIDER", defaultAIProvider),
		AIModel:              getEnv("AI_MODEL", ""),
		AIAPIKey:             getEnv("AI_API_KEY", ""),
		AIBaseURL:            getEnv("AI_BASE_URL", ""),
		AITimeout:            time.Duration(getEnvInt("AI_TIMEOUT_SECONDS", defaultAITimeoutSecs)) * time.Second,
		AIEnableRealProvider: getEnvBool("AI_ENABLE_REAL_PROVIDER", false),

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
