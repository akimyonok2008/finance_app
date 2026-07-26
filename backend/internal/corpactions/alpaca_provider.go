package corpactions

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

// alpacaProviderName is the provenance name recorded on every normalized event
// this adapter produces.
const alpacaProviderName = "alpaca"

// ErrMissingCredentials is returned when an adapter is constructed without the
// credentials its API requires. Selection of a real provider must fail loudly
// rather than silently degrade to the development provider.
var ErrMissingCredentials = errors.New("corpactions: provider credentials missing")

// AlpacaConfig configures the Alpaca corporate-actions adapter.
type AlpacaConfig struct {
	BaseURL   string
	KeyID     string
	SecretKey string
	Timeout   time.Duration
	// HTTPClient is injected so tests can target an httptest server. Never a
	// package-level global.
	HTTPClient *http.Client
}

// AlpacaProvider is a real CorporateActionProvider backed by Alpaca's
// /v1/corporate-actions market-data endpoint.
//
// Only the event types the domain models today are ingested: forward splits,
// reverse splits and ticker (name) changes. Alpaca's cash- and stock-dividend
// announcements are income events and are handled by the income package's
// Alpaca adapter, not here.
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

// alpacaResponse mirrors the subset of the Alpaca payload this adapter reads.
type alpacaResponse struct {
	CorporateActions struct {
		ForwardSplits []alpacaSplit      `json:"forward_splits"`
		ReverseSplits []alpacaSplit      `json:"reverse_splits"`
		NameChanges   []alpacaNameChange `json:"name_changes"`
	} `json:"corporate_actions"`
}

type alpacaSplit struct {
	Symbol      string  `json:"symbol"`
	NewRate     float64 `json:"new_rate"`
	OldRate     float64 `json:"old_rate"`
	ProcessDate string  `json:"process_date"`
	ExDate      string  `json:"ex_date"`
	RecordDate  string  `json:"record_date"`
	PayableDate string  `json:"payable_date"`
}

type alpacaNameChange struct {
	OldSymbol   string `json:"old_symbol"`
	NewSymbol   string `json:"new_symbol"`
	ProcessDate string `json:"process_date"`
}

// FetchActions queries Alpaca for the requested symbols and window and returns
// normalized raw provider events.
func (p *AlpacaProvider) FetchActions(ctx context.Context, request CorporateActionRequest) ([]ProviderCorporateAction, error) {
	symbols := make([]string, 0, len(request.Instruments))
	for _, inst := range request.Instruments {
		if inst.Symbol != "" {
			symbols = append(symbols, inst.Symbol)
		}
	}
	if len(symbols) == 0 {
		return []ProviderCorporateAction{}, nil
	}

	q := url.Values{}
	q.Set("symbols", strings.Join(symbols, ","))
	q.Set("types", "forward_split,reverse_split,name_change")
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
		// The error never carries the request URL or credentials.
		return nil, fmt.Errorf("alpaca corporate-actions fetch failed: %w", err)
	}

	var parsed alpacaResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("alpaca corporate-actions decode failed: %w", err)
	}

	now := time.Now().UTC()
	out := make([]ProviderCorporateAction, 0)
	for _, s := range parsed.CorporateActions.ForwardSplits {
		if e, ok := alpacaSplitToEvent(TypeSplit, s, now); ok {
			out = append(out, e)
		}
	}
	for _, s := range parsed.CorporateActions.ReverseSplits {
		if e, ok := alpacaSplitToEvent(TypeReverseSplit, s, now); ok {
			out = append(out, e)
		}
	}
	for _, n := range parsed.CorporateActions.NameChanges {
		if e, ok := alpacaNameChangeToEvent(n, now); ok {
			out = append(out, e)
		}
	}
	return out, nil
}

func alpacaSplitToEvent(t Type, s alpacaSplit, now time.Time) (ProviderCorporateAction, bool) {
	if s.Symbol == "" {
		return ProviderCorporateAction{}, false
	}
	effective := alpacaDate(s.ExDate)
	if effective == nil {
		effective = alpacaDate(s.ProcessDate)
	}
	if effective == nil {
		return ProviderCorporateAction{}, false
	}
	num, den := s.NewRate, s.OldRate
	event := ProviderCorporateAction{
		ProviderEventID:  fmt.Sprintf("%s:%s:%s", t, s.Symbol, effective.Format("2006-01-02")),
		Type:             t,
		Source:           InstrumentReference{Symbol: s.Symbol},
		EffectiveAt:      *effective,
		RecordAt:         alpacaDate(s.RecordDate),
		PayableAt:        alpacaDate(s.PayableDate),
		RatioNumerator:   &num,
		RatioDenominator: &den,
		Effective:        !effective.After(now),
		RetrievedAt:      now,
	}
	return event, true
}

func alpacaNameChangeToEvent(n alpacaNameChange, now time.Time) (ProviderCorporateAction, bool) {
	if n.OldSymbol == "" || n.NewSymbol == "" {
		return ProviderCorporateAction{}, false
	}
	effective := alpacaDate(n.ProcessDate)
	if effective == nil {
		return ProviderCorporateAction{}, false
	}
	target := InstrumentReference{Symbol: n.NewSymbol}
	return ProviderCorporateAction{
		ProviderEventID: fmt.Sprintf("%s:%s:%s", TypeSymbolChange, n.OldSymbol, effective.Format("2006-01-02")),
		Type:            TypeSymbolChange,
		Source:          InstrumentReference{Symbol: n.OldSymbol},
		Target:          &target,
		EffectiveAt:     *effective,
		Effective:       !effective.After(now),
		RetrievedAt:     now,
	}, true
}

// alpacaDate parses an Alpaca YYYY-MM-DD date, returning nil when absent or
// malformed.
func alpacaDate(v string) *time.Time {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return &t
}
