package instrument

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/providerhttp"
)

// IdentityQuery is what the caller knows about an instrument. Fields are
// optional; the adapter builds the most precise OpenFIGI mapping job the
// supplied fields allow (ISIN/CUSIP/FIGI beat ticker+exchange, which beats a
// bare ticker).
type IdentityQuery struct {
	Ticker       string
	ExchangeCode string
	MIC          string
	ISIN         string
	CUSIP        string
	FIGI         string
	SecurityType string // optional hint
}

// IdentityCandidate is one instrument an identity provider believes the query
// could refer to.
type IdentityCandidate struct {
	FIGI           string
	CompositeFIGI  string
	ShareClassFIGI string
	Ticker         string
	ExchangeCode   string
	SecurityType   string
	Name           string
	MIC            string
	Currency       string
}

// IdentityProvider maps external identifiers onto candidate instruments. It
// never decides between candidates: disambiguation is a domain concern and
// lives in Resolver.
type IdentityProvider interface {
	Resolve(ctx context.Context, query IdentityQuery) ([]IdentityCandidate, error)
}

// ErrInsufficientQuery is returned when a query carries no usable identifier.
var ErrInsufficientQuery = errors.New("openfigi: query carries no usable identifier")

// ErrProviderDisabled is returned when the adapter is constructed but OpenFIGI
// is switched off in configuration.
var ErrProviderDisabled = errors.New("openfigi: provider is disabled")

// OpenFIGIProvider maps identifiers via OpenFIGI's POST /v3/mapping endpoint.
//
// Deferred: /v3/search and /v3/filter (free-text discovery) are NOT implemented
// in this slice. Only exact-identifier mapping is needed for the resolution
// workflow; free-text search is a future slice.
type OpenFIGIProvider struct {
	baseURL string
	apiKey  string
	client  *providerhttp.Client
}

// OpenFIGIConfig configures the adapter. HTTPClient is injected so tests point
// it at an httptest server; nothing here ever dials the real API in tests.
type OpenFIGIConfig struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// NewOpenFIGIProvider builds the adapter. The API key is optional: OpenFIGI
// permits unauthenticated low-volume use. The key is stored for the request
// header only and is never logged or included in error strings.
func NewOpenFIGIProvider(cfg OpenFIGIConfig) *OpenFIGIProvider {
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = DefaultOpenFIGIBaseURL
	}
	return &OpenFIGIProvider{
		baseURL: base,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client:  providerhttp.New(cfg.HTTPClient, cfg.Timeout),
	}
}

// DefaultOpenFIGIBaseURL is OpenFIGI's public API host.
const DefaultOpenFIGIBaseURL = "https://api.openfigi.com"

var _ IdentityProvider = (*OpenFIGIProvider)(nil)

// mappingJob is one entry of the OpenFIGI POST /v3/mapping request array.
type mappingJob struct {
	IDType       string `json:"idType"`
	IDValue      string `json:"idValue"`
	ExchCode     string `json:"exchCode,omitempty"`
	MICCode      string `json:"micCode,omitempty"`
	SecurityType string `json:"securityType,omitempty"`
}

// mappingResult is one entry of the response array. OpenFIGI returns either
// {"data":[...]} or {"error":"..."} or {"warning":"..."} per job.
type mappingResult struct {
	Data []struct {
		FIGI                string `json:"figi"`
		CompositeFIGI       string `json:"compositeFIGI"`
		ShareClassFIGI      string `json:"shareClassFIGI"`
		Ticker              string `json:"ticker"`
		Name                string `json:"name"`
		ExchCode            string `json:"exchCode"`
		MICCode             string `json:"micCode"`
		SecurityType        string `json:"securityType"`
		SecurityType2       string `json:"securityType2"`
		MarketSector        string `json:"marketSector"`
		SecurityDescription string `json:"securityDescription"`
	} `json:"data"`
	Error   string `json:"error"`
	Warning string `json:"warning"`
}

// buildJob picks the most precise identifier the query offers.
func buildJob(q IdentityQuery) (mappingJob, error) {
	switch {
	case strings.TrimSpace(q.FIGI) != "":
		return mappingJob{IDType: "ID_BB_GLOBAL", IDValue: normalizeAliasValue(q.FIGI)}, nil
	case strings.TrimSpace(q.ISIN) != "":
		return mappingJob{IDType: "ID_ISIN", IDValue: normalizeAliasValue(q.ISIN)}, nil
	case strings.TrimSpace(q.CUSIP) != "":
		return mappingJob{IDType: "ID_CUSIP", IDValue: normalizeAliasValue(q.CUSIP)}, nil
	case strings.TrimSpace(q.Ticker) != "":
		job := mappingJob{
			IDType:       "TICKER",
			IDValue:      normalizeAliasValue(q.Ticker),
			ExchCode:     normalizeScope(q.ExchangeCode),
			MICCode:      normalizeScope(q.MIC),
			SecurityType: strings.TrimSpace(q.SecurityType),
		}
		return job, nil
	}
	return mappingJob{}, ErrInsufficientQuery
}

// Resolve issues a single-job mapping request and returns every candidate
// OpenFIGI reported. It never truncates to the first hit: an ambiguous result
// must stay visibly ambiguous to the caller.
func (p *OpenFIGIProvider) Resolve(ctx context.Context, query IdentityQuery) ([]IdentityCandidate, error) {
	job, err := buildJob(query)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal([]mappingJob{job})
	if err != nil {
		return nil, fmt.Errorf("openfigi: encode request: %w", err)
	}

	body, err := p.client.PostJSON(ctx, p.baseURL+"/v3/mapping", payload, func(req *http.Request) {
		if p.apiKey != "" {
			// OpenFIGI's documented auth header. Never logged.
			req.Header.Set("X-OPENFIGI-APIKEY", p.apiKey)
		}
	})
	if err != nil {
		// Rate limits and 5xx surface as errors after the shared retry budget
		// is exhausted; they are never flattened into "no candidates", which
		// would look identical to a genuine unresolved instrument.
		return nil, fmt.Errorf("openfigi: mapping request failed: %w", err)
	}

	var results []mappingResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("openfigi: decode response: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	res := results[0]
	if res.Error != "" {
		// "No identifier found." is OpenFIGI's normal empty answer, not a
		// failure: report it as zero candidates.
		if strings.Contains(strings.ToLower(res.Error), "no identifier found") {
			return nil, nil
		}
		return nil, fmt.Errorf("openfigi: mapping error: %s", res.Error)
	}

	out := make([]IdentityCandidate, 0, len(res.Data))
	for _, d := range res.Data {
		secType := d.SecurityType
		if secType == "" {
			secType = d.SecurityType2
		}
		name := d.Name
		if name == "" {
			name = d.SecurityDescription
		}
		out = append(out, IdentityCandidate{
			FIGI:           normalizeAliasValue(d.FIGI),
			CompositeFIGI:  normalizeAliasValue(d.CompositeFIGI),
			ShareClassFIGI: normalizeAliasValue(d.ShareClassFIGI),
			Ticker:         normalizeAliasValue(d.Ticker),
			ExchangeCode:   normalizeScope(d.ExchCode),
			MIC:            normalizeScope(d.MICCode),
			SecurityType:   secType,
			Name:           name,
		})
	}
	return out, nil
}
