package income

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IncomeEventProvider is the neutral provider boundary. Adapters for FMP, EODHD,
// Twelve Data, a brokerage feed, or a development source implement it; the
// pipeline consumes only normalized events. No provider type leaks into the
// portfolio domain.
type IncomeEventProvider interface {
	// Name identifies the provider for provenance and metrics.
	Name() string
	// FetchIncomeEvents returns raw provider events in the requested window.
	FetchIncomeEvents(ctx context.Context, request IncomeEventRequest) ([]ProviderIncomeEvent, error)
}

// requiresAccountData reports whether a type must originate from a brokerage /
// account-configured source rather than a market-data provider.
func requiresAccountData(t Type) bool {
	switch t {
	case TypeCashInterest, TypeStakingReward, TypePaymentInLieu:
		return true
	default:
		return false
	}
}

// ManualDevelopmentProvider is a deterministic, in-memory provider used for
// local development and tests. It needs no API keys and no network, so the
// pipeline runs end-to-end offline. Events are seeded explicitly. It reports a
// configurable channel (market vs brokerage) so account-specific events can be
// exercised.
type ManualDevelopmentProvider struct {
	name      string
	brokerage bool
	mu        sync.Mutex
	events    []ProviderIncomeEvent
}

// NewManualDevelopmentProvider builds an empty market-data development provider.
func NewManualDevelopmentProvider() *ManualDevelopmentProvider {
	return &ManualDevelopmentProvider{name: "manual_dev"}
}

// NewManualBrokerageProvider builds a development provider that is trusted to
// supply account-specific events (cash interest, staking, payment in lieu).
func NewManualBrokerageProvider() *ManualDevelopmentProvider {
	return &ManualDevelopmentProvider{name: "manual_brokerage", brokerage: true}
}

func (p *ManualDevelopmentProvider) Name() string { return p.name }

// Seed adds events the provider will return for matching instruments.
func (p *ManualDevelopmentProvider) Seed(events ...ProviderIncomeEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range events {
		if e.RetrievedAt.IsZero() {
			e.RetrievedAt = time.Now().UTC()
		}
		p.events = append(p.events, e)
	}
}

// FetchIncomeEvents returns seeded events whose instrument symbol is among the
// requested instruments and whose payment date falls within the window.
func (p *ManualDevelopmentProvider) FetchIncomeEvents(_ context.Context, request IncomeEventRequest) ([]ProviderIncomeEvent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	want := map[string]bool{}
	for _, inst := range request.Instruments {
		want[inst.Symbol] = true
	}
	out := make([]ProviderIncomeEvent, 0)
	for _, e := range p.events {
		// A market-data provider must not supply account-specific event types.
		if !p.brokerage && requiresAccountData(e.Type) {
			continue
		}
		if len(want) > 0 && !want[e.Instrument.Symbol] {
			continue
		}
		ref := e.PaymentDate
		if !request.Since.IsZero() && ref.Before(request.Since) {
			continue
		}
		if !request.Until.IsZero() && ref.After(request.Until) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// fingerprint is a stable hash of the terms that materially define an event, so
// a provider correction (changed amount, date, cancellation, components) is
// detectable.
func fingerprint(e ProviderIncomeEvent) string {
	var b []byte
	add := func(s string) { b = append(b, s...); b = append(b, '|') }
	addF := func(f float64) { add(strconv.FormatFloat(f, 'f', 8, 64)) }
	addT := func(t *time.Time) {
		if t == nil {
			add("-")
		} else {
			add(t.UTC().Format(time.RFC3339))
		}
	}
	add(string(e.Type))
	add(e.Instrument.InstrumentID)
	add(e.Instrument.Symbol)
	addF(e.AmountPerUnit)
	add(e.Currency)
	addT(e.ExDate)
	addT(e.RecordDate)
	add(e.PaymentDate.UTC().Format(time.RFC3339))
	comps := append([]ProviderComponent(nil), e.Components...)
	sort.Slice(comps, func(i, j int) bool { return comps[i].Type < comps[j].Type })
	for _, c := range comps {
		add(string(c.Type))
		addF(c.AmountPerUnit)
	}
	add(strconv.FormatBool(e.Cancelled))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// assessQuality derives a quality level from the completeness of an event's
// required terms.
func assessQuality(e ProviderIncomeEvent) Quality {
	if e.Cancelled {
		return QualityCancelled
	}
	if e.Instrument.Symbol == "" || e.Currency == "" || e.PaymentDate.IsZero() {
		return QualityIncomplete
	}
	if e.Type == TypeStockDividend {
		// A stock dividend needs a ratio, carried as AmountPerUnit (new shares per
		// held share).
		if e.AmountPerUnit <= 0 {
			return QualityIncomplete
		}
		return QualityVerified
	}
	if e.AmountPerUnit <= 0 {
		return QualityIncomplete
	}
	// A mixed distribution's components must reconcile to the headline amount.
	if len(e.Components) > 0 {
		var sum float64
		for _, c := range e.Components {
			sum += c.AmountPerUnit
		}
		if !within(sum, e.AmountPerUnit, 1e-6) {
			return QualityConflicting
		}
	}
	return QualityVerified
}

func within(a, b, tol float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= tol
}

// normalize converts a raw provider event into a stored normalized event and
// assigns a deterministic id (provider + provider_event_id).
func normalize(provider string, e ProviderIncomeEvent, now time.Time) IncomeEvent {
	q := assessQuality(e)
	status := StatusDetected
	switch {
	case e.Cancelled:
		status = StatusCancelled
	case q == QualityIncomplete || q == QualityConflicting:
		status = StatusUnresolved
	default:
		status = StatusScheduled
	}
	return IncomeEvent{
		ID:                fmt.Sprintf("%s:%s", provider, e.ProviderEventID),
		Provider:          provider,
		ProviderEventID:   e.ProviderEventID,
		Type:              e.Type,
		Instrument:        e.Instrument,
		AmountPerUnit:     e.AmountPerUnit,
		Currency:          e.Currency,
		Components:        e.Components,
		DeclarationAt:     e.DeclarationAt,
		ExDate:            e.ExDate,
		RecordDate:        e.RecordDate,
		PaymentDate:       e.PaymentDate,
		Frequency:         e.Frequency,
		Status:            status,
		Quality:           q,
		TaxClassification: e.TaxClassification,
		SourceURL:         e.SourceURL,
		RawFingerprint:    fingerprint(e),
		RetrievedAt:       e.RetrievedAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

// requestInstruments builds a de-duplicated instrument list from held symbols.
func requestInstruments(symbols []string) []InstrumentReference {
	sort.Strings(symbols)
	out := make([]InstrumentReference, 0, len(symbols))
	seen := map[string]bool{}
	for _, s := range symbols {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, InstrumentReference{Symbol: s})
	}
	return out
}

func discoveryRequestInstruments(items []DiscoveryInstrument) []InstrumentReference {
	sort.Slice(items, func(i, j int) bool {
		return discoveryKey(items[i].InstrumentID, items[i].Symbol) <
			discoveryKey(items[j].InstrumentID, items[j].Symbol)
	})
	out := make([]InstrumentReference, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		key := discoveryKey(item.InstrumentID, item.Symbol)
		if item.Symbol == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, InstrumentReference{InstrumentID: item.InstrumentID, Symbol: item.Symbol})
	}
	return out
}

func discoveryKey(instrumentID, symbol string) string {
	if instrumentID != "" {
		return "instrument:" + instrumentID + "|alias:" + normalizeDiscoverySymbol(symbol)
	}
	return "symbol:" + normalizeDiscoverySymbol(symbol)
}

func normalizeDiscoverySymbol(symbol string) string {
	return strings.ToUpper(strings.TrimSpace(symbol))
}
