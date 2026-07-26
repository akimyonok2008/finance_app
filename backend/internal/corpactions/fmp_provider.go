package corpactions

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/providerhttp"
)

const fmpProviderName = "fmp"

// ErrBudgetExhausted signals the FMP daily request allowance is spent. It is
// distinct from an empty result: callers must not treat it as "no data".
var ErrBudgetExhausted = providerhttp.ErrBudgetExhausted

// FMPConfig configures the Financial Modeling Prep corporate-action adapter.
type FMPConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	// DailyRequestBudget caps outbound requests per UTC day; <= 0 is unlimited.
	DailyRequestBudget int
	// HTTPClient is injected so tests can target an httptest server.
	HTTPClient *http.Client
}

// FMPProvider is a real CorporateActionProvider backed by FMP's splits endpoint.
// Only splits are ingested; FMP's merger/spin-off coverage is not wired in this
// pass.
type FMPProvider struct {
	baseURL string
	apiKey  string
	client  *providerhttp.Client
	budget  *providerhttp.DailyBudget
}

// NewFMPProvider builds the adapter, validating the API key is present.
func NewFMPProvider(cfg FMPConfig) (*FMPProvider, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("%w: FMP_API_KEY is required", ErrMissingCredentials)
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://financialmodelingprep.com/stable"
	}
	return &FMPProvider{
		baseURL: base,
		apiKey:  cfg.APIKey,
		client:  providerhttp.New(cfg.HTTPClient, cfg.Timeout),
		budget:  providerhttp.NewDailyBudget(cfg.DailyRequestBudget),
	}, nil
}

func (p *FMPProvider) Name() string { return fmpProviderName }

type fmpSplit struct {
	Symbol      string  `json:"symbol"`
	Date        string  `json:"date"`
	Numerator   float64 `json:"numerator"`
	Denominator float64 `json:"denominator"`
}

// FetchActions queries FMP once per requested symbol. Budget exhaustion stops
// further requests and is surfaced as ErrBudgetExhausted.
func (p *FMPProvider) FetchActions(ctx context.Context, request CorporateActionRequest) ([]ProviderCorporateAction, error) {
	now := time.Now().UTC()
	out := make([]ProviderCorporateAction, 0)
	for _, inst := range request.Instruments {
		if inst.Symbol == "" {
			continue
		}
		if err := p.budget.Consume(); err != nil {
			return out, err
		}
		q := url.Values{}
		q.Set("symbol", inst.Symbol)
		q.Set("apikey", p.apiKey)
		endpoint := p.baseURL + "/splits?" + q.Encode()

		body, err := p.client.GetJSON(ctx, endpoint, nil)
		if err != nil {
			return out, fmt.Errorf("fmp splits fetch failed for %s: %w", inst.Symbol, err)
		}
		var rows []fmpSplit
		if err := json.Unmarshal(body, &rows); err != nil {
			return out, fmt.Errorf("fmp splits decode failed for %s: %w", inst.Symbol, err)
		}
		for _, row := range rows {
			symbol := row.Symbol
			if symbol == "" {
				symbol = inst.Symbol
			}
			effective := alpacaDate(row.Date)
			if effective == nil || row.Numerator <= 0 || row.Denominator <= 0 {
				continue
			}
			if !request.Since.IsZero() && effective.Before(request.Since) {
				continue
			}
			if !request.Until.IsZero() && effective.After(request.Until) {
				continue
			}
			t := TypeSplit
			if row.Numerator < row.Denominator {
				t = TypeReverseSplit
			}
			num, den := row.Numerator, row.Denominator
			out = append(out, ProviderCorporateAction{
				ProviderEventID:  fmt.Sprintf("%s:%s:%s", t, symbol, effective.Format("2006-01-02")),
				Type:             t,
				Source:           InstrumentReference{Symbol: symbol},
				EffectiveAt:      *effective,
				RatioNumerator:   &num,
				RatioDenominator: &den,
				Effective:        !effective.After(now),
				RetrievedAt:      now,
			})
		}
	}
	return out, nil
}
