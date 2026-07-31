package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validProductionConfig() Config {
	return Config{
		AppEnv:                       "production",
		JWTSecret:                    "0123456789abcdef0123456789abcdef",
		StorageProvider:              "postgres",
		DatabaseURL:                  "postgres://app:secret@db/finance",
		CORSAllowedOrigins:           []string{"https://app.example.com"},
		PriceProvider:                "twelvedata",
		EnableRealMarketData:         true,
		TwelveDataAPIKey:             "ci-provider-key",
		BaseCurrency:                 "USD",
		BenchmarkAwardMode:           "verified_only",
		EnableBackgroundWorkers:      true,
		EnableQuoteRefreshWorker:     true,
		PasswordRegistrationEnabled:  false,
		EmailSender:                  "development",
		InstrumentResolutionRequired: true,
	}
}

func TestParseDotEnvLine(t *testing.T) {
	tests := []struct {
		name      string
		line      string
		wantKey   string
		wantValue string
		wantOK    bool
	}{
		{name: "plain", line: "GOOGLE_AUTH_ENABLED=true", wantKey: "GOOGLE_AUTH_ENABLED", wantValue: "true", wantOK: true},
		{name: "quoted", line: `GOOGLE_CLIENT_ID="client.apps.googleusercontent.com"`, wantKey: "GOOGLE_CLIENT_ID", wantValue: "client.apps.googleusercontent.com", wantOK: true},
		{name: "export", line: "export STORAGE_PROVIDER=postgres", wantKey: "STORAGE_PROVIDER", wantValue: "postgres", wantOK: true},
		{name: "comment", line: "# GOOGLE_AUTH_ENABLED=true", wantOK: false},
		{name: "empty", line: "   ", wantOK: false},
		{name: "missing separator", line: "GOOGLE_AUTH_ENABLED", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotValue, gotOK := parseDotEnvLine(tt.line)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotKey != tt.wantKey || gotValue != tt.wantValue {
				t.Fatalf("parseDotEnvLine() = (%q, %q), want (%q, %q)", gotKey, gotValue, tt.wantKey, tt.wantValue)
			}
		})
	}
}

func TestValidateRequiresGoogleClientIDWhenEnabled(t *testing.T) {
	cfg := Config{GoogleAuthEnabled: true}

	assert.ErrorContains(t, cfg.Validate(), "GOOGLE_CLIENT_ID")
}

// TestValidateRefusesDefaultSecretInProduction: a missing JWT_SECRET must not
// merely warn once APP_ENV=production — that would let a credential-less
// deployment look healthy while every token is forgeable from the public
// default secret string.
func TestValidateRefusesDefaultSecretInProduction(t *testing.T) {
	cfg := validProductionConfig()
	cfg.JWTSecret = defaultJWTSecret

	assert.ErrorContains(t, cfg.Validate(), "JWT_SECRET")
}

func TestValidateAllowsDefaultSecretOutsideProduction(t *testing.T) {
	for _, env := range []string{"development", "test", "demo", ""} {
		cfg := Config{AppEnv: env, JWTSecret: defaultJWTSecret}
		assert.NoError(t, cfg.Validate(), "env %q should only warn, not fail", env)
	}
}

func TestValidateAllowsProductionWithRealSecret(t *testing.T) {
	cfg := validProductionConfig()

	assert.NoError(t, cfg.Validate())
}

func TestValidateProductionPasswordRegistrationRequiresSMTP(t *testing.T) {
	cfg := validProductionConfig()
	cfg.PasswordRegistrationEnabled = true
	require.ErrorContains(t, cfg.Validate(), "EMAIL_SENDER=smtp")
}

func TestValidateProductionPasswordRegistrationAllowsConfiguredSMTP(t *testing.T) {
	cfg := validProductionConfig()
	cfg.PasswordRegistrationEnabled = true
	cfg.EmailSender = "smtp"
	cfg.SMTPHost = "smtp.example.com"
	cfg.SMTPFrom = "security@example.com"
	require.NoError(t, cfg.Validate())
}

func TestValidateProductionCheckIsCaseInsensitive(t *testing.T) {
	cfg := validProductionConfig()
	cfg.AppEnv = "Production"
	cfg.JWTSecret = defaultJWTSecret

	assert.ErrorContains(t, cfg.Validate(), "JWT_SECRET")
}

func TestValidateProductionSafetyRequirements(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		message string
	}{
		{name: "short JWT secret", mutate: func(c *Config) { c.JWTSecret = "too-short" }, message: "JWT_SECRET"},
		{name: "memory storage", mutate: func(c *Config) { c.StorageProvider = "memory" }, message: "STORAGE_PROVIDER=postgres"},
		{name: "missing database", mutate: func(c *Config) { c.DatabaseURL = "" }, message: "DATABASE_URL"},
		{name: "missing CORS", mutate: func(c *Config) { c.CORSAllowedOrigins = nil }, message: "CORS_ALLOWED_ORIGINS"},
		{name: "wildcard CORS", mutate: func(c *Config) { c.CORSAllowedOrigins = []string{"*"} }, message: "cannot use *"},
		{name: "insecure CORS", mutate: func(c *Config) { c.CORSAllowedOrigins = []string{"http://example.com"} }, message: "must use https"},
		{name: "mock price outside demo", mutate: func(c *Config) { c.PriceProvider = "mock" }, message: "DEMO_MODE_ENABLED"},
		{name: "prototype Yahoo provider", mutate: func(c *Config) { c.PriceProvider = "yahoo" }, message: "prototype"},
		{name: "missing real market switch", mutate: func(c *Config) { c.EnableRealMarketData = false }, message: "ENABLE_REAL_MARKET_DATA"},
		{name: "missing Twelve Data key", mutate: func(c *Config) { c.TwelveDataAPIKey = "" }, message: "TWELVE_DATA_API_KEY"},
		{name: "static FX for verified multicurrency", mutate: func(c *Config) { c.BaseCurrency = "EUR" }, message: "historical FX provider"},
		{name: "background workers disabled", mutate: func(c *Config) { c.EnableBackgroundWorkers = false }, message: "ENABLE_BACKGROUND_WORKERS"},
		{name: "quote worker disabled", mutate: func(c *Config) { c.EnableQuoteRefreshWorker = false }, message: "ENABLE_QUOTE_REFRESH_WORKER"},
		{name: "instrument resolution not required", mutate: func(c *Config) { c.InstrumentResolutionRequired = false }, message: "INSTRUMENT_RESOLUTION_REQUIRED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validProductionConfig()
			tt.mutate(&cfg)
			require.ErrorContains(t, cfg.Validate(), tt.message)
		})
	}
}

func TestValidateProductionExplicitDemoAllowsMockPrices(t *testing.T) {
	cfg := validProductionConfig()
	cfg.PriceProvider = "mock"
	cfg.DemoModeEnabled = true
	cfg.BenchmarkAwardMode = "demo"

	require.NoError(t, cfg.Validate())
}

// TestLoad_InstrumentResolutionRequiredDefaultsTrueInProduction: an operator
// who deploys APP_ENV=production without ever hearing about
// INSTRUMENT_RESOLUTION_REQUIRED must not silently get the permissive
// (ticker-only-position-saving) behavior.
func TestLoad_InstrumentResolutionRequiredDefaultsTrueInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("FINANCE_APP_ENV_FILE", "/nonexistent-so-no-.env-leaks-in")
	cfg := Load()
	assert.True(t, cfg.InstrumentResolutionRequired)

	t.Setenv("APP_ENV", "development")
	cfg = Load()
	assert.False(t, cfg.InstrumentResolutionRequired)

	t.Setenv("INSTRUMENT_RESOLUTION_REQUIRED", "true")
	cfg = Load()
	assert.True(t, cfg.InstrumentResolutionRequired, "explicit override must still work outside production")
}

func TestValidateRejectsInvalidTrustedProxyCIDR(t *testing.T) {
	cfg := validProductionConfig()
	cfg.TrustedProxyCIDRs = []string{"10.0.0.0/8", "not-a-cidr"}

	err := cfg.Validate()
	require.ErrorContains(t, err, "TRUSTED_PROXY_CIDRS")
}

func TestValidateCompetitionWorkerRequiresBackgroundWorkers(t *testing.T) {
	cfg := Config{EnableCompetitionWorker: true}
	require.ErrorContains(t, cfg.Validate(), "ENABLE_BACKGROUND_WORKERS")
}
