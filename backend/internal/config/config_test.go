package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	cfg := Config{AppEnv: "production", JWTSecret: defaultJWTSecret}

	assert.ErrorContains(t, cfg.Validate(), "JWT_SECRET")
}

func TestValidateAllowsDefaultSecretOutsideProduction(t *testing.T) {
	for _, env := range []string{"development", "test", "demo", ""} {
		cfg := Config{AppEnv: env, JWTSecret: defaultJWTSecret}
		assert.NoError(t, cfg.Validate(), "env %q should only warn, not fail", env)
	}
}

func TestValidateAllowsProductionWithRealSecret(t *testing.T) {
	cfg := Config{AppEnv: "production", JWTSecret: "a-real-generated-secret"}

	assert.NoError(t, cfg.Validate())
}

func TestValidateProductionCheckIsCaseInsensitive(t *testing.T) {
	cfg := Config{AppEnv: "Production", JWTSecret: defaultJWTSecret}

	assert.ErrorContains(t, cfg.Validate(), "JWT_SECRET")
}
