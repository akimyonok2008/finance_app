package instrument

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every test here talks to an httptest.Server. Nothing in this file (or this
// package's tests) dials api.openfigi.com or any other real host.

func newTestProvider(t *testing.T, handler http.HandlerFunc, apiKey string) (*OpenFIGIProvider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p := NewOpenFIGIProvider(OpenFIGIConfig{
		BaseURL:    srv.URL,
		APIKey:     apiKey,
		HTTPClient: srv.Client(),
		Timeout:    2 * time.Second,
	})
	// Keep retry backoff instant so a 429 test does not sleep for seconds.
	p.client.Sleep = func(time.Duration) {}
	return p, srv
}

const appleMappingResponse = `[{"data":[{
  "figi":"BBG000B9XRY4",
  "name":"APPLE INC",
  "ticker":"AAPL",
  "exchCode":"UW",
  "compositeFIGI":"BBG000B9XRY4",
  "securityType":"Common Stock",
  "marketSector":"Equity",
  "shareClassFIGI":"BBG001S5N8V8",
  "securityType2":"Common Stock",
  "securityDescription":"AAPL"
}]}]`

func TestOpenFIGI_TickerAndExchangeReturnsOneCandidate(t *testing.T) {
	var gotPath, gotMethod, gotContentType string
	var gotBody []byte
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(appleMappingResponse))
	}, "")

	got, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "aapl", ExchangeCode: "uw"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "BBG000B9XRY4", got[0].FIGI)
	assert.Equal(t, "BBG000B9XRY4", got[0].CompositeFIGI)
	assert.Equal(t, "BBG001S5N8V8", got[0].ShareClassFIGI)
	assert.Equal(t, "AAPL", got[0].Ticker)
	assert.Equal(t, "UW", got[0].ExchangeCode)
	assert.Equal(t, "Common Stock", got[0].SecurityType)
	assert.Equal(t, "APPLE INC", got[0].Name)

	assert.Equal(t, "/v3/mapping", gotPath)
	assert.Equal(t, http.MethodPost, gotMethod)
	assert.Equal(t, "application/json", gotContentType)

	// The mapping job must be an ARRAY of jobs with the documented field names.
	var jobs []map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &jobs))
	require.Len(t, jobs, 1)
	assert.Equal(t, "TICKER", jobs[0]["idType"])
	assert.Equal(t, "AAPL", jobs[0]["idValue"])
	assert.Equal(t, "UW", jobs[0]["exchCode"])
	_, hasMIC := jobs[0]["micCode"]
	assert.False(t, hasMIC, "empty scope fields must be omitted, not sent blank")
}

func TestOpenFIGI_ISINQueryUsesISINJob(t *testing.T) {
	var jobs []map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &jobs)
		_, _ = w.Write([]byte(appleMappingResponse))
	}, "")

	got, err := p.Resolve(context.Background(), IdentityQuery{
		ISIN: "us0378331005", Ticker: "IGNORED",
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, jobs, 1)
	assert.Equal(t, "ID_ISIN", jobs[0]["idType"], "a stronger identifier must win over the ticker")
	assert.Equal(t, "US0378331005", jobs[0]["idValue"])
}

func TestOpenFIGI_MICQueryIsSent(t *testing.T) {
	var jobs []map[string]any
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &jobs)
		_, _ = w.Write([]byte(appleMappingResponse))
	}, "")

	_, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "AAPL", MIC: "xnas"})
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	assert.Equal(t, "XNAS", jobs[0]["micCode"])
}

func TestOpenFIGI_MultipleCandidatesAreAllReturned(t *testing.T) {
	const body = `[{"data":[
      {"figi":"BBG000BLNNH6","ticker":"IBM","exchCode":"UN","compositeFIGI":"BBG000BLNNH6","name":"IBM CORP"},
      {"figi":"BBG000BLNNQ4","ticker":"IBM","exchCode":"GY","compositeFIGI":"BBG000BLNNQ4","name":"IBM CORP"},
      {"figi":"BBG000BLNNS0","ticker":"IBM","exchCode":"LN","compositeFIGI":"BBG000BLNNS0","name":"IBM CORP"}
    ]}]`
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}, "")

	got, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "IBM"})
	require.NoError(t, err)
	require.Len(t, got, 3, "an ambiguous mapping must stay visibly ambiguous")
	assert.Equal(t, "BBG000BLNNH6", got[0].FIGI)
	assert.Equal(t, "BBG000BLNNQ4", got[1].FIGI)
	assert.Equal(t, "BBG000BLNNS0", got[2].FIGI)
}

func TestOpenFIGI_EmptyAndNoIdentifierFoundAreUnresolved(t *testing.T) {
	cases := map[string]string{
		"empty data array":    `[{"data":[]}]`,
		"empty top array":     `[]`,
		"no identifier found": `[{"error":"No identifier found."}]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}, "")
			got, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "NOSUCH"})
			require.NoError(t, err, "an unknown instrument is an outcome, not a failure")
			assert.Empty(t, got)
		})
	}
}

func TestOpenFIGI_OtherJobErrorIsSurfaced(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"error":"Invalid idType value."}]`))
	}, "")
	_, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "X"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid idType value.")
}

func TestOpenFIGI_RateLimitIsSurfacedAsError(t *testing.T) {
	var calls int
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"message":"rate limit"}`))
	}, "")

	got, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "AAPL"})
	require.Error(t, err, "a 429 must never be swallowed into an empty result")
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "429")
	assert.Equal(t, 3, calls, "429 is retried up to the shared attempt budget")
}

func TestOpenFIGI_ServerErrorIsSurfaced(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, "")
	_, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "AAPL"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestOpenFIGI_MalformedJSONIsAnError(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	}, "")
	_, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "AAPL"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}

func TestOpenFIGI_APIKeyHeaderIsSentAndNeverLogged(t *testing.T) {
	const key = "super-secret-openfigi-key"
	var gotKey string
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(io.Discard) })

	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-OPENFIGI-APIKEY")
		w.WriteHeader(http.StatusTooManyRequests) // also exercise the error path
	}, key)

	_, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "AAPL"})
	require.Error(t, err)
	assert.Equal(t, key, gotKey, "the key goes in OpenFIGI's documented header")
	assert.NotContains(t, err.Error(), key, "the key must never leak into an error string")
	assert.NotContains(t, logs.String(), key, "the key must never be logged")
}

func TestOpenFIGI_UnauthenticatedRequestSendsNoKeyHeader(t *testing.T) {
	var present bool
	p, _ := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["X-Openfigi-Apikey"]
		_, _ = w.Write([]byte(appleMappingResponse))
	}, "")
	_, err := p.Resolve(context.Background(), IdentityQuery{Ticker: "AAPL"})
	require.NoError(t, err)
	assert.False(t, present, "unauthenticated low-volume use must send no key header")
}

func TestOpenFIGI_EmptyQueryDoesNotCallTheAPI(t *testing.T) {
	var calls int
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(appleMappingResponse))
	}, "")
	_, err := p.Resolve(context.Background(), IdentityQuery{})
	assert.ErrorIs(t, err, ErrInsufficientQuery)
	assert.Zero(t, calls)
}

func TestOpenFIGI_DefaultBaseURLIsTheDocumentedHost(t *testing.T) {
	p := NewOpenFIGIProvider(OpenFIGIConfig{})
	assert.Equal(t, DefaultOpenFIGIBaseURL, p.baseURL)
	assert.True(t, strings.HasPrefix(p.baseURL, "https://"))
}

func TestOpenFIGI_ContextCancellationIsRespected(t *testing.T) {
	p, _ := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(appleMappingResponse))
	}, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := p.Resolve(ctx, IdentityQuery{Ticker: "AAPL"})
	require.Error(t, err)
}
