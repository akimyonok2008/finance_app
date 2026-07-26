package income

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const alpacaIncomeFixture = `{
  "corporate_actions": {
    "cash_dividends": [
      {"symbol":"AAPL","rate":0.24,"special":false,"ex_date":"2024-02-09","record_date":"2024-02-12","payable_date":"2024-02-15","process_date":"2024-02-15"},
      {"symbol":"COST","rate":15.0,"special":true,"ex_date":"2024-01-10","record_date":"2024-01-11","payable_date":"2024-01-12","process_date":"2024-01-12"}
    ],
    "stock_dividends": [
      {"symbol":"XYZ","rate":0.05,"ex_date":"2024-03-01","record_date":"2024-03-02","payable_date":"2024-03-05","process_date":"2024-03-05"}
    ]
  }
}`

func TestAlpacaIncomeProviderNormalizesDividends(t *testing.T) {
	var secretHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secretHeader = r.Header.Get("APCA-API-SECRET-KEY")
		_, _ = w.Write([]byte(alpacaIncomeFixture))
	}))
	defer srv.Close()

	p, err := NewAlpacaProvider(AlpacaConfig{
		BaseURL: srv.URL, KeyID: "key", SecretKey: "secret",
		Timeout: 2 * time.Second, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}

	events, err := p.FetchIncomeEvents(context.Background(), IncomeEventRequest{
		Instruments: []InstrumentReference{{Symbol: "AAPL"}, {Symbol: "COST"}, {Symbol: "XYZ"}},
	})
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if secretHeader != "secret" {
		t.Fatal("secret header not sent")
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Type != TypeCashDividend || events[0].AmountPerUnit != 0.24 {
		t.Fatalf("unexpected cash dividend: %+v", events[0])
	}
	if events[0].PaymentDate.Format("2006-01-02") != "2024-02-15" {
		t.Fatalf("unexpected payment date: %v", events[0].PaymentDate)
	}
	if events[1].Type != TypeSpecialDividend {
		t.Fatalf("special flag should map to special_dividend: %+v", events[1])
	}
	if events[2].Type != TypeStockDividend || events[2].AmountPerUnit != 0.05 {
		t.Fatalf("unexpected stock dividend: %+v", events[2])
	}
	if norm := normalize(p.Name(), events[2], time.Now().UTC()); norm.Quality != QualityVerified {
		t.Fatalf("expected verified stock dividend, got %s", norm.Quality)
	}
}

func TestAlpacaIncomeProviderRetriesServerErrors(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(alpacaIncomeFixture))
	}))
	defer srv.Close()

	p, err := NewAlpacaProvider(AlpacaConfig{
		BaseURL: srv.URL, KeyID: "key", SecretKey: "secret",
		Timeout: 2 * time.Second, HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	// Keep the retry backoff instant for the test.
	p.client.Sleep = func(time.Duration) {}

	events, err := p.FetchIncomeEvents(context.Background(), IncomeEventRequest{
		Instruments: []InstrumentReference{{Symbol: "AAPL"}},
	})
	if err != nil {
		t.Fatalf("expected the retry to succeed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 attempts, got %d", calls)
	}
	if len(events) == 0 {
		t.Fatal("expected events after retry")
	}
}
