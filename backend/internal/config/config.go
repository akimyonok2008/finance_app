package config

import (
	"os"
	"strconv"
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
	defaultBaseCurrency     = "USD"
	defaultEnableBackground = false
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

	EnableBackgroundWorkers    bool
	LeaderboardRefreshInterval time.Duration
	PriceCacheTTL              time.Duration
}

// Load reads configuration from environment variables, falling back to
// development-friendly defaults when a variable is missing or invalid.
func Load() Config {
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

		EnableBackgroundWorkers:    getEnvBool("ENABLE_BACKGROUND_WORKERS", defaultEnableBackground),
		LeaderboardRefreshInterval: time.Duration(getEnvInt("LEADERBOARD_REFRESH_INTERVAL_SECONDS", defaultLeaderboardSecs)) * time.Second,
		PriceCacheTTL:              time.Duration(getEnvInt("PRICE_CACHE_TTL_SECONDS", defaultPriceCacheSecs)) * time.Second,
	}
}

// UsingDefaultSecret reports whether the insecure development secret is in use,
// so the server can warn at startup.
func (c Config) UsingDefaultSecret() bool {
	return c.JWTSecret == defaultJWTSecret
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
