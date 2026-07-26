package income

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/providerhttp"
)

const alpacaProviderName = "alpaca"

// ErrMissingCredentials is returned when an adapter is constructed without the
// credentials its API requires, so provider selection fails loudly instead of
// silently degrading to the development provider.
var ErrMissingCredentials = errors.New("income: provider credentials missing")

// AlpacaConfig configures the Alpaca income adapter.
type AlpacaConfig struct {
	BaseURL   string
	KeyID     string
	SecretKey string
	Timeout   time.Duration
	// HTTPClient is injected so tests can target an httptest server.
	HTTPClient *http.Client
}

// AlpacaProvider is a real IncomeEventProvider backed by Alpaca's
// /v1/corporate-actions endpoint, from which it reads the distribution
// announcements the income domain models: cash dividends (ordinary or special)
// and stock dividends.
type AlpacaProvider struct {
	baseURL   string
	keyID     string
	secretKey string
	client    *providerhttp.Client
}

// NewAlpacaProvider builds the adapter, validating that credentials are present.
func NewAlpacaProvider(cfg AlpacaConfig) (*AlpacaProvider, error) {
	if strings.TrimSpace(cfg.KeyID) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, fmt.Errorf("%w: ALPACA_API_KEY_ID and ALPACA_API_SECRET_KEY are required", ErrMissingCredentials)
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://data.alpaca.markets"
	}
	return &AlpacaProvider{
		baseURL:   base,
		keyID:     cfg.KeyID,
		secretKey: cfg.SecretKey,
		client:    providerhttp.New(cfg.HTTPClient, cfg.Timeout),
	}, nil
}

func (p *AlpacaProvider) Name() string { return alpacaProviderName }

type alpacaIncomeResponse struct {
	CorporateActions struct {
		CashDividends  []alpacaCashDividend  `json:"cash_dividends"`
		StockDividends []alpacaStockDividend `json:"stock_dividends"`
	} `json:"corporate_actions"`
}

type alpacaCashDividend struct {
	Symbol      string  `json:"symbol"`
	Rate        float64 `json:"rate"`
	Special     bool    `json:"special"`
	ExDate      string  `json:"ex_date"`
	RecordDate  string  `json:"record_date"`
	PayableDate string  `json:"payable_date"`
	ProcessDate string  `json:"process_date"`
}

type alpacaStockDividend struct {
	Symbol      string  `json:"symbol"`
	Rate        float64 `json:"rate"`
	ExDate      string  `json:"ex_date"`
	RecordDate  string  `json:"record_date"`
	PayableDate string  `json:"payable_date"`
	ProcessDate string  `json:"process_date"`
}

// FetchIncomeEvents queries Alpaca for the requested symbols and window.
func (p *AlpacaProvider) FetchIncomeEvents(ctx context.Context, request IncomeEventRequest) ([]ProviderIncomeEvent, error) {
	symbols := make([]string, 0, len(request.Instruments))
	for _, inst := range request.Instruments {
		if inst.Symbol != "" {
			symbols = append(symbols, inst.Symbol)
		}
	}
	if len(symbols) == 0 {
		return []ProviderIncomeEvent{}, nil
	}

	q := url.Values{}
	q.Set("symbols", strings.Join(symbols, ","))
	q.Set("types", "cash_dividend,stock_dividend")
	if !request.Since.IsZero() {
		q.Set("start", request.Since.UTC().Format("2006-01-02"))
	}
	if !request.Until.IsZero() {
		q.Set("end", request.Until.UTC().Format("2006-01-02"))
	}
	endpoint := p.baseURL + "/v1/corporate-actions?" + q.Encode()

	body, err := p.client.GetJSON(ctx, endpoint, func(req *http.Request) {
		req.Header.Set("APCA-API-KEY-ID", p.keyID)
		req.Header.Set("APCA-API-SECRET-KEY", p.secretKey)
	})
	if err != nil {
		return nil, fmt.Errorf("alpaca income fetch failed: %w", err)
	}

	var parsed alpacaIncomeResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("alpaca income decode failed: %w", err)
	}

	now := time.Now().UTC()
	out := make([]ProviderIncomeEvent, 0)
	for _, d := range parsed.CorporateActions.CashDividends {
		payment := providerDate(d.PayableDate, d.ProcessDate)
		if d.Symbol == "" || payment == nil {
			continue
		}
		t := TypeCashDividend
		if d.Special {
			t = TypeSpecialDividend
		}
		out = append(out, ProviderIncomeEvent{
			ProviderEventID: fmt.Sprintf("%s:%s:%s", t, d.Symbol, payment.Format("2006-01-02")),
			Type:            t,
			Instrument:      InstrumentReference{Symbol: d.Symbol},
			AmountPerUnit:   d.Rate,
			Currency:        "USD",
			ExDate:          providerDate(d.ExDate),
			RecordDate:      providerDate(d.RecordDate),
			PaymentDate:     *payment,
			RetrievedAt:     now,
		})
	}
	for _, d := range parsed.CorporateActions.StockDividends {
		payment := providerDate(d.PayableDate, d.ProcessDate)
		if d.Symbol == "" || payment == nil {
			continue
		}
		out = append(out, ProviderIncomeEvent{
			ProviderEventID: fmt.Sprintf("%s:%s:%s", TypeStockDividend, d.Symbol, payment.Format("2006-01-02")),
			Type:            TypeStockDividend,
			Instrument:      InstrumentReference{Symbol: d.Symbol},
			AmountPerUnit:   d.Rate,
			Currency:        "USD",
			ExDate:          providerDate(d.ExDate),
			RecordDate:      providerDate(d.RecordDate),
			PaymentDate:     *payment,
			RetrievedAt:     now,
		})
	}
	return out, nil
}

// providerDate parses the first parseable YYYY-MM-DD value, returning nil when
// none is usable.
func providerDate(values ...string) *time.Time {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		t, err := time.Parse("2006-01-02", v)
		if err != nil {
			continue
		}
		t = t.UTC()
		return &t
	}
	return nil
}
