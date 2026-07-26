package income

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fmpDividendsFixture = `[
  {"symbol":"AAPL","date":"2024-02-09","recordDate":"2024-02-12","paymentDate":"2024-02-15","declarationDate":"2024-02-01","dividend":0.24,"frequency":"Quarterly"}
]`

func TestFMPProviderNormalizesDividend(t *testing.T) {
	var gotPath, gotQuerySymbol, gotAPIKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuerySymbol = r.URL.Query().Get("symbol")
		gotAPIKey = r.URL.Query().Get("apikey")
		_, _ = w.Write([]byte(fmpDividendsFixture))
	}))
	defer srv.Close()

	p, err := NewFMPProvider(FMPConfig{
		BaseURL: srv.URL, APIKey: "test-api-key", Timeout: 2 * time.Second,
		DailyRequestBudget: 10, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	events, err := p.FetchIncomeEvents(context.Background(), IncomeEventRequest{
		Instruments: []InstrumentReference{{Symbol: "AAPL"}},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotPath != "/dividends" || gotQuerySymbol != "AAPL" || gotAPIKey != "test-api-key" {
		t.Fatalf("unexpected request: path=%s symbol=%s", gotPath, gotQuerySymbol)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != TypeCashDividend || e.Instrument.Symbol != "AAPL" || e.AmountPerUnit != 0.24 {
		t.Fatalf("unexpected event: %+v", e)
	}
	if e.PaymentDate.Format("2006-01-02") != "2024-02-15" {
		t.Fatalf("unexpected payment date: %v", e.PaymentDate)
	}
	if e.ExDate == nil || e.ExDate.Format("2006-01-02") != "2024-02-09" {
		t.Fatalf("unexpected ex date: %+v", e.ExDate)
	}
	if e.Frequency != "Quarterly" {
		t.Fatalf("unexpected frequency: %q", e.Frequency)
	}
	if norm := normalize(p.Name(), e, time.Now().UTC()); norm.Quality != QualityVerified {
		t.Fatalf("expected verified normalization, got %s", norm.Quality)
	}
}

// TestFMPProviderBudgetExhaustion issues budget+1 requests and asserts the last
// one never reaches the server and reports the distinguishable budget signal.
func TestFMPProviderBudgetExhaustion(t *testing.T) {
	const budget = 3
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	p, err := NewFMPProvider(FMPConfig{
		BaseURL: srv.URL, APIKey: "k", Timeout: 2 * time.Second,
		DailyRequestBudget: budget, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	req := IncomeEventRequest{Instruments: []InstrumentReference{{Symbol: "AAPL"}}}
	for i := 0; i < budget; i++ {
		if _, err := p.FetchIncomeEvents(context.Background(), req); err != nil {
			t.Fatalf("call %d unexpectedly failed: %v", i+1, err)
		}
	}
	if calls != budget {
		t.Fatalf("expected %d server calls, got %d", budget, calls)
	}

	_, err = p.FetchIncomeEvents(context.Background(), req)
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("expected ErrBudgetExhausted, got %v", err)
	}
	if calls != budget {
		t.Fatalf("budget-exhausted call still hit the server: %d calls", calls)
	}
}

func TestFMPProviderRequiresAPIKey(t *testing.T) {
	_, err := NewFMPProvider(FMPConfig{BaseURL: "http://example.invalid"})
	if !errors.Is(err, ErrMissingCredentials) {
		t.Fatalf("expected ErrMissingCredentials, got %v", err)
	}
}

func TestFMPProviderErrorDoesNotLeakAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer srv.Close()

	p, err := NewFMPProvider(FMPConfig{
		BaseURL: srv.URL, APIKey: "leaky-key-value", Timeout: 2 * time.Second,
		DailyRequestBudget: 5, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	_, err = p.FetchIncomeEvents(context.Background(), IncomeEventRequest{
		Instruments: []InstrumentReference{{Symbol: "AAPL"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "leaky-key-value") {
		t.Fatalf("api key leaked: %v", err)
	}
}
