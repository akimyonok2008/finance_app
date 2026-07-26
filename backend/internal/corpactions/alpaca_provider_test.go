package corpactions

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const alpacaFixture = `{
  "corporate_actions": {
    "forward_splits": [
      {"symbol":"AAPL","new_rate":4,"old_rate":1,"process_date":"2020-08-31","ex_date":"2020-08-31","record_date":"2020-08-24","payable_date":"2020-08-28"}
    ],
    "reverse_splits": [
      {"symbol":"XYZ","new_rate":1,"old_rate":10,"process_date":"2021-05-03","ex_date":"2021-05-03","record_date":"2021-04-30","payable_date":"2021-05-03"}
    ],
    "name_changes": [
      {"old_symbol":"FB","new_symbol":"META","process_date":"2022-06-09"}
    ]
  }
}`

func newTestAlpaca(t *testing.T, handler http.HandlerFunc) (*AlpacaProvider, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	p, err := NewAlpacaProvider(AlpacaConfig{
		BaseURL:    srv.URL,
		KeyID:      "test-key-id",
		SecretKey:  "super-secret-value",
		Timeout:    2 * time.Second,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("constructing provider: %v", err)
	}
	return p, srv
}

func TestAlpacaProviderNormalizesSplitsAndNameChange(t *testing.T) {
	var gotPath, gotKeyHeader, gotSecretHeader string
	p, _ := newTestAlpaca(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKeyHeader = r.Header.Get("APCA-API-KEY-ID")
		gotSecretHeader = r.Header.Get("APCA-API-SECRET-KEY")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(alpacaFixture))
	})

	events, err := p.FetchActions(context.Background(), CorporateActionRequest{
		Instruments: []InstrumentReference{{Symbol: "AAPL"}, {Symbol: "XYZ"}, {Symbol: "FB"}},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotPath != "/v1/corporate-actions" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotKeyHeader != "test-key-id" || gotSecretHeader != "super-secret-value" {
		t.Fatal("auth headers were not sent")
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	fwd := events[0]
	if fwd.Type != TypeSplit || fwd.Source.Symbol != "AAPL" {
		t.Fatalf("unexpected forward split: %+v", fwd)
	}
	if fwd.RatioNumerator == nil || *fwd.RatioNumerator != 4 ||
		fwd.RatioDenominator == nil || *fwd.RatioDenominator != 1 {
		t.Fatalf("unexpected ratio: %+v", fwd)
	}
	if fwd.EffectiveAt.Format("2006-01-02") != "2020-08-31" {
		t.Fatalf("unexpected effective date: %v", fwd.EffectiveAt)
	}
	// Normalization must produce a verified, applicable event.
	norm := normalize(p.Name(), fwd, time.Now().UTC())
	if norm.Quality != QualityVerified || norm.Status != StatusValidated {
		t.Fatalf("unexpected normalized quality/status: %s/%s", norm.Quality, norm.Status)
	}

	rev := events[1]
	if rev.Type != TypeReverseSplit || *rev.RatioNumerator != 1 || *rev.RatioDenominator != 10 {
		t.Fatalf("unexpected reverse split: %+v", rev)
	}

	name := events[2]
	if name.Type != TypeSymbolChange || name.Source.Symbol != "FB" ||
		name.Target == nil || name.Target.Symbol != "META" {
		t.Fatalf("unexpected name change: %+v", name)
	}
}

// TestAlpacaProviderNeverLogsSecret captures everything written through slog
// during a fetch (including the error path) and asserts the API secret never
// appears.
func TestAlpacaProviderNeverLogsSecret(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	p, _ := newTestAlpaca(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	})

	_, err := p.FetchActions(context.Background(), CorporateActionRequest{
		Instruments: []InstrumentReference{{Symbol: "AAPL"}},
	})
	if err == nil {
		t.Fatal("expected an error for a 400 response")
	}
	if strings.Contains(err.Error(), "super-secret-value") || strings.Contains(err.Error(), "test-key-id") {
		t.Fatalf("credentials leaked into the error: %v", err)
	}
	if strings.Contains(buf.String(), "super-secret-value") {
		t.Fatalf("secret leaked into logs: %s", buf.String())
	}
}

// TestAlpacaProviderDoesNotRetryClientErrors proves a 400 is terminal.
func TestAlpacaProviderDoesNotRetryClientErrors(t *testing.T) {
	calls := 0
	p, _ := newTestAlpaca(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	})
	if _, err := p.FetchActions(context.Background(), CorporateActionRequest{
		Instruments: []InstrumentReference{{Symbol: "AAPL"}},
	}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 call, got %d", calls)
	}
}

func TestNewAlpacaProviderRequiresCredentials(t *testing.T) {
	if _, err := NewAlpacaProvider(AlpacaConfig{KeyID: "only-key"}); err == nil {
		t.Fatal("expected ErrMissingCredentials")
	}
}
