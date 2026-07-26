package income

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

// FMPConfig configures the Financial Modeling Prep income adapter.
type FMPConfig struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
	// DailyRequestBudget caps outbound requests per UTC day; <= 0 is unlimited.
	DailyRequestBudget int
	// HTTPClient is injected so tests can target an httptest server.
	HTTPClient *http.Client
}

// FMPProvider is a real IncomeEventProvider backed by FMP's dividends endpoint.
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

type fmpDividend struct {
	Symbol          string  `json:"symbol"`
	Date            string  `json:"date"` // ex-dividend date
	RecordDate      string  `json:"recordDate"`
	PaymentDate     string  `json:"paymentDate"`
	DeclarationDate string  `json:"declarationDate"`
	Dividend        float64 `json:"dividend"`
	Frequency       string  `json:"frequency"`
}

// FetchIncomeEvents queries FMP once per requested symbol. A budget exhaustion
// stops further requests and is surfaced as ErrBudgetExhausted.
func (p *FMPProvider) FetchIncomeEvents(ctx context.Context, request IncomeEventRequest) ([]ProviderIncomeEvent, error) {
	now := time.Now().UTC()
	out := make([]ProviderIncomeEvent, 0)
	for _, inst := range request.Instruments {
		if inst.Symbol == "" {
			continue
		}
		// Reserve budget BEFORE issuing any request.
		if err := p.budget.Consume(); err != nil {
			return out, err
		}
		q := url.Values{}
		q.Set("symbol", inst.Symbol)
		q.Set("apikey", p.apiKey)
		endpoint := p.baseURL + "/dividends?" + q.Encode()

		body, err := p.client.GetJSON(ctx, endpoint, nil)
		if err != nil {
			// The API key is never included in the error text.
			return out, fmt.Errorf("fmp dividends fetch failed for %s: %w", inst.Symbol, err)
		}
		var rows []fmpDividend
		if err := json.Unmarshal(body, &rows); err != nil {
			return out, fmt.Errorf("fmp dividends decode failed for %s: %w", inst.Symbol, err)
		}
		for _, row := range rows {
			symbol := row.Symbol
			if symbol == "" {
				symbol = inst.Symbol
			}
			payment := providerDate(row.PaymentDate, row.Date)
			if payment == nil {
				continue
			}
			if !request.Since.IsZero() && payment.Before(request.Since) {
				continue
			}
			if !request.Until.IsZero() && payment.After(request.Until) {
				continue
			}
			out = append(out, ProviderIncomeEvent{
				ProviderEventID: fmt.Sprintf("%s:%s:%s", TypeCashDividend, symbol, payment.Format("2006-01-02")),
				Type:            TypeCashDividend,
				Instrument:      InstrumentReference{Symbol: symbol},
				AmountPerUnit:   row.Dividend,
				Currency:        "USD",
				DeclarationAt:   providerDate(row.DeclarationDate),
				ExDate:          providerDate(row.Date),
				RecordDate:      providerDate(row.RecordDate),
				PaymentDate:     *payment,
				Frequency:       row.Frequency,
				RetrievedAt:     now,
			})
		}
	}
	return out, nil
}
