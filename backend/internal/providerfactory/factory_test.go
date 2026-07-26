package providerfactory

import (
	"strings"
	"testing"

	"github.com/ardakimyonok/finance_app/internal/config"
	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/income"
)

func baseConfig() config.Config {
	return config.Config{
		AlpacaDataBaseURL: "https://data.alpaca.markets",
		FMPBaseURL:        "https://financialmodelingprep.com/stable",
	}
}

func TestIncomeProviderAlpacaWithCredentials(t *testing.T) {
	cfg := baseConfig()
	cfg.IncomePrimaryProvider = "alpaca"
	cfg.AlpacaAPIKeyID = "key"
	cfg.AlpacaAPISecretKey = "secret"

	p, err := New(cfg, nil).IncomeProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*income.AlpacaProvider); !ok {
		t.Fatalf("expected *income.AlpacaProvider, got %T", p)
	}
	if p.Name() != "alpaca" {
		t.Fatalf("expected provider name alpaca, got %q", p.Name())
	}
}

func TestIncomeProviderAlpacaMissingCredentialsFailsStartup(t *testing.T) {
	cfg := baseConfig()
	cfg.IncomePrimaryProvider = "alpaca"

	p, err := New(cfg, nil).IncomeProvider()
	if err == nil {
		t.Fatalf("expected error, got provider %T", p)
	}
	if p != nil {
		t.Fatalf("expected nil provider on error, got %T", p)
	}
	if !strings.Contains(err.Error(), "ALPACA_API_KEY_ID") {
		t.Fatalf("error should name the missing credentials: %v", err)
	}
}

func TestIncomeProviderFMPWithKey(t *testing.T) {
	cfg := baseConfig()
	cfg.IncomePrimaryProvider = "fmp"
	cfg.FMPAPIKey = "abc"

	p, err := New(cfg, nil).IncomeProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*income.FMPProvider); !ok {
		t.Fatalf("expected *income.FMPProvider, got %T", p)
	}
}

func TestIncomeProviderFMPMissingKeyFailsStartup(t *testing.T) {
	cfg := baseConfig()
	cfg.IncomePrimaryProvider = "fmp"

	if _, err := New(cfg, nil).IncomeProvider(); err == nil {
		t.Fatal("expected error when FMP_API_KEY is unset")
	}
}

func TestIncomeProviderManualDev(t *testing.T) {
	cfg := baseConfig()
	cfg.IncomePrimaryProvider = "manual_dev"

	p, err := New(cfg, nil).IncomeProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*income.ManualDevelopmentProvider); !ok {
		t.Fatalf("expected *income.ManualDevelopmentProvider, got %T", p)
	}
}

func TestIncomeProviderUnknownValue(t *testing.T) {
	cfg := baseConfig()
	cfg.IncomePrimaryProvider = "bloomberg"

	_, err := New(cfg, nil).IncomeProvider()
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported income provider: bloomberg") {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestCorporateActionProviderSelection(t *testing.T) {
	t.Run("alpaca with credentials", func(t *testing.T) {
		cfg := baseConfig()
		cfg.CorporateActionPrimary = "alpaca"
		cfg.AlpacaAPIKeyID = "key"
		cfg.AlpacaAPISecretKey = "secret"
		p, err := New(cfg, nil).CorporateActionProvider()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*corpactions.AlpacaProvider); !ok {
			t.Fatalf("expected *corpactions.AlpacaProvider, got %T", p)
		}
	})

	t.Run("alpaca without credentials", func(t *testing.T) {
		cfg := baseConfig()
		cfg.CorporateActionPrimary = "alpaca"
		if _, err := New(cfg, nil).CorporateActionProvider(); err == nil {
			t.Fatal("expected startup error, never a silent manual_dev fallback")
		}
	})

	t.Run("fmp with key", func(t *testing.T) {
		cfg := baseConfig()
		cfg.CorporateActionPrimary = "fmp"
		cfg.FMPAPIKey = "abc"
		p, err := New(cfg, nil).CorporateActionProvider()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*corpactions.FMPProvider); !ok {
			t.Fatalf("expected *corpactions.FMPProvider, got %T", p)
		}
	})

	t.Run("fmp without key", func(t *testing.T) {
		cfg := baseConfig()
		cfg.CorporateActionPrimary = "fmp"
		if _, err := New(cfg, nil).CorporateActionProvider(); err == nil {
			t.Fatal("expected error when FMP_API_KEY is unset")
		}
	})

	t.Run("manual dev", func(t *testing.T) {
		cfg := baseConfig()
		cfg.CorporateActionPrimary = "manual_dev"
		p, err := New(cfg, nil).CorporateActionProvider()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := p.(*corpactions.ManualDevelopmentProvider); !ok {
			t.Fatalf("expected manual dev provider, got %T", p)
		}
	})

	t.Run("unknown value", func(t *testing.T) {
		cfg := baseConfig()
		cfg.CorporateActionPrimary = "nasdaq"
		_, err := New(cfg, nil).CorporateActionProvider()
		if err == nil || !strings.Contains(err.Error(), "unsupported corporate action provider: nasdaq") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestFallbackProvidersUnsetReturnNil(t *testing.T) {
	f := New(baseConfig(), nil)
	p, err := f.IncomeFallbackProvider()
	if err != nil || p != nil {
		t.Fatalf("expected (nil, nil), got (%T, %v)", p, err)
	}
	c, err := f.CorporateActionFallbackProvider()
	if err != nil || c != nil {
		t.Fatalf("expected (nil, nil), got (%T, %v)", c, err)
	}
}

func TestFallbackProviderConstructedWhenSet(t *testing.T) {
	cfg := baseConfig()
	cfg.IncomeFallbackProvider = "manual_dev"
	p, err := New(cfg, nil).IncomeFallbackProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := p.(*income.ManualDevelopmentProvider); !ok {
		t.Fatalf("expected manual dev fallback, got %T", p)
	}
}
