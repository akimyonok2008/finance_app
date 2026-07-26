// Package providerfactory turns provider selection configuration into concrete
// provider adapters at startup. Selecting a real provider whose credentials are
// missing is a startup error — never a silent fallback to the development
// provider, which would make production look healthy while serving no data.
package providerfactory

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/config"
	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/income"
)

// Provider selection values accepted by configuration.
const (
	ProviderManualDev = "manual_dev"
	ProviderAlpaca    = "alpaca"
	ProviderFMP       = "fmp"
)

// Factory builds providers from a loaded configuration. The HTTP client is
// injected so tests can point every adapter at an httptest server.
type Factory struct {
	cfg  config.Config
	http *http.Client
}

// New builds a Factory. A nil client makes each adapter create its own client
// with the configured per-provider timeout.
func New(cfg config.Config, httpClient *http.Client) *Factory {
	return &Factory{cfg: cfg, http: httpClient}
}

// IncomeProvider returns the income provider named by INCOME_PRIMARY_PROVIDER.
func (f *Factory) IncomeProvider() (income.IncomeEventProvider, error) {
	return f.incomeProviderNamed(f.cfg.IncomePrimaryProvider, "INCOME_PRIMARY_PROVIDER")
}

// IncomeFallbackProvider returns the provider named by INCOME_FALLBACK_PROVIDER,
// or (nil, nil) when unset.
//
// NOTE: construction only. Chaining a fallback behind the primary provider (the
// retry/degradation policy) is deliberately NOT implemented in this pass; the
// service still consumes a single provider. See the report / future work.
func (f *Factory) IncomeFallbackProvider() (income.IncomeEventProvider, error) {
	if strings.TrimSpace(f.cfg.IncomeFallbackProvider) == "" {
		return nil, nil
	}
	return f.incomeProviderNamed(f.cfg.IncomeFallbackProvider, "INCOME_FALLBACK_PROVIDER")
}

func (f *Factory) incomeProviderNamed(name, envKey string) (income.IncomeEventProvider, error) {
	switch normalize(name) {
	case ProviderManualDev, "":
		return income.NewManualDevelopmentProvider(), nil
	case ProviderAlpaca:
		// Returned explicitly rather than inline so a nil concrete pointer never
		// becomes a non-nil interface value on the error path.
		p, err := income.NewAlpacaProvider(income.AlpacaConfig{
			BaseURL:    f.cfg.AlpacaDataBaseURL,
			KeyID:      f.cfg.AlpacaAPIKeyID,
			SecretKey:  f.cfg.AlpacaAPISecretKey,
			Timeout:    f.cfg.AlpacaRequestTimeout,
			HTTPClient: f.http,
		})
		if err != nil {
			return nil, err
		}
		return p, nil
	case ProviderFMP:
		p, err := income.NewFMPProvider(income.FMPConfig{
			BaseURL:            f.cfg.FMPBaseURL,
			APIKey:             f.cfg.FMPAPIKey,
			Timeout:            f.cfg.FMPRequestTimeout,
			DailyRequestBudget: f.cfg.FMPDailyRequestBudget,
			HTTPClient:         f.http,
		})
		if err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unsupported income provider: %s (%s)", name, envKey)
	}
}

// CorporateActionProvider returns the provider named by
// CORPORATE_ACTION_PRIMARY_PROVIDER.
func (f *Factory) CorporateActionProvider() (corpactions.CorporateActionProvider, error) {
	return f.corpProviderNamed(f.cfg.CorporateActionPrimary, "CORPORATE_ACTION_PRIMARY_PROVIDER")
}

// CorporateActionFallbackProvider returns the provider named by
// CORPORATE_ACTION_FALLBACK_PROVIDER, or (nil, nil) when unset. Construction
// only — see IncomeFallbackProvider for the chaining caveat.
func (f *Factory) CorporateActionFallbackProvider() (corpactions.CorporateActionProvider, error) {
	if strings.TrimSpace(f.cfg.CorporateActionFallback) == "" {
		return nil, nil
	}
	return f.corpProviderNamed(f.cfg.CorporateActionFallback, "CORPORATE_ACTION_FALLBACK_PROVIDER")
}

func (f *Factory) corpProviderNamed(name, envKey string) (corpactions.CorporateActionProvider, error) {
	switch normalize(name) {
	case ProviderManualDev, "":
		return corpactions.NewManualDevelopmentProvider(), nil
	case ProviderAlpaca:
		p, err := corpactions.NewAlpacaProvider(corpactions.AlpacaConfig{
			BaseURL:    f.cfg.AlpacaDataBaseURL,
			KeyID:      f.cfg.AlpacaAPIKeyID,
			SecretKey:  f.cfg.AlpacaAPISecretKey,
			Timeout:    f.cfg.AlpacaRequestTimeout,
			HTTPClient: f.http,
		})
		if err != nil {
			return nil, err
		}
		return p, nil
	case ProviderFMP:
		p, err := corpactions.NewFMPProvider(corpactions.FMPConfig{
			BaseURL:            f.cfg.FMPBaseURL,
			APIKey:             f.cfg.FMPAPIKey,
			Timeout:            f.cfg.FMPRequestTimeout,
			DailyRequestBudget: f.cfg.FMPDailyRequestBudget,
			HTTPClient:         f.http,
		})
		if err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unsupported corporate action provider: %s (%s)", name, envKey)
	}
}

func normalize(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
