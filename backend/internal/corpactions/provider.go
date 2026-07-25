package corpactions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

// CorporateActionProvider is the neutral provider boundary. Adapters for
// Twelve Data, FMP, EODHD, SEC EDGAR, or a development source implement it; the
// pipeline consumes only normalized events. No provider type leaks into the
// portfolio domain.
type CorporateActionProvider interface {
	// Name identifies the provider for provenance and metrics.
	Name() string
	// FetchActions returns raw provider events in the requested window.
	FetchActions(ctx context.Context, request CorporateActionRequest) ([]ProviderCorporateAction, error)
}

// ManualDevelopmentProvider is a deterministic, in-memory provider used for
// local development and tests. It requires no API keys and no network, so the
// pipeline can be exercised end-to-end offline. Events are seeded explicitly.
type ManualDevelopmentProvider struct {
	name   string
	mu     sync.Mutex
	events []ProviderCorporateAction
}

// NewManualDevelopmentProvider builds an empty development provider.
func NewManualDevelopmentProvider() *ManualDevelopmentProvider {
	return &ManualDevelopmentProvider{name: "manual_dev"}
}

func (p *ManualDevelopmentProvider) Name() string { return p.name }

// Seed adds an event the provider will return for matching instruments.
func (p *ManualDevelopmentProvider) Seed(events ...ProviderCorporateAction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range events {
		if e.RetrievedAt.IsZero() {
			e.RetrievedAt = time.Now().UTC()
		}
		p.events = append(p.events, e)
	}
}

// FetchActions returns seeded events whose source symbol is among the requested
// instruments and whose effective date falls within the window.
func (p *ManualDevelopmentProvider) FetchActions(_ context.Context, request CorporateActionRequest) ([]ProviderCorporateAction, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	want := map[string]bool{}
	for _, inst := range request.Instruments {
		want[inst.Symbol] = true
	}
	out := make([]ProviderCorporateAction, 0)
	for _, e := range p.events {
		if len(want) > 0 && !want[e.Source.Symbol] {
			continue
		}
		if !request.Since.IsZero() && e.EffectiveAt.Before(request.Since) {
			continue
		}
		if !request.Until.IsZero() && e.EffectiveAt.After(request.Until) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// fingerprint is a stable hash of the terms that materially define an event, so
// a provider correction (changed ratio, date, cancellation) is detectable.
func fingerprint(e ProviderCorporateAction) string {
	var b []byte
	add := func(s string) { b = append(b, s...); b = append(b, '|') }
	addF := func(f *float64) {
		if f == nil {
			add("-")
		} else {
			add(strconv.FormatFloat(*f, 'f', 8, 64))
		}
	}
	add(string(e.Type))
	add(e.Source.Symbol)
	if e.Target != nil {
		add(e.Target.Symbol)
	} else {
		add("-")
	}
	add(e.EffectiveAt.UTC().Format(time.RFC3339))
	addF(e.RatioNumerator)
	addF(e.RatioDenominator)
	addF(e.CashPerShare)
	addF(e.ParentBasisPct)
	addF(e.ChildBasisPct)
	add(strconv.FormatBool(e.Cancelled))
	add(strconv.FormatBool(e.Effective))
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// assessQuality derives a quality level from the completeness of an event's
// required terms. Incomplete complex events are never auto-applied.
func assessQuality(e ProviderCorporateAction) Quality {
	if e.Cancelled {
		return QualityCancelled
	}
	switch e.Type {
	case TypeSplit, TypeReverseSplit:
		if e.RatioNumerator == nil || e.RatioDenominator == nil ||
			*e.RatioNumerator <= 0 || *e.RatioDenominator <= 0 {
			return QualityIncomplete
		}
	case TypeSymbolChange:
		if e.Target == nil || e.Target.Symbol == "" {
			return QualityIncomplete
		}
	case TypeStockMerger:
		if e.Target == nil || e.RatioNumerator == nil || e.RatioDenominator == nil {
			return QualityIncomplete
		}
	case TypeCashMerger:
		if e.CashPerShare == nil || e.CashCurrency == nil {
			return QualityIncomplete
		}
	case TypeMixedMerger:
		if e.Target == nil || e.RatioNumerator == nil || e.RatioDenominator == nil ||
			e.CashPerShare == nil || e.CashCurrency == nil {
			return QualityIncomplete
		}
	case TypeSpinOff:
		if e.Target == nil || e.RatioNumerator == nil || e.RatioDenominator == nil ||
			e.ParentBasisPct == nil || e.ChildBasisPct == nil {
			return QualityIncomplete
		}
	}
	if !e.Effective {
		// Announced but not yet in effect: usable data, but not applicable yet.
		return QualityHighConfidence
	}
	return QualityVerified
}

// normalize converts a raw provider event into a stored normalized event and
// assigns a deterministic id (provider + provider_event_id).
func normalize(provider string, e ProviderCorporateAction, now time.Time) CorporateAction {
	q := assessQuality(e)
	status := StatusDetected
	switch {
	case e.Cancelled:
		status = StatusCancelled
	case q == QualityIncomplete:
		status = StatusUnresolved
	case q.meetsAutoApplyPolicy() && e.Effective:
		status = StatusValidated
	default:
		status = StatusScheduled
	}
	return CorporateAction{
		ID:               fmt.Sprintf("%s:%s", provider, e.ProviderEventID),
		Provider:         provider,
		ProviderEventID:  e.ProviderEventID,
		Type:             e.Type,
		Source:           e.Source,
		Target:           e.Target,
		AnnouncementAt:   e.AnnouncementAt,
		EffectiveAt:      e.EffectiveAt,
		RecordAt:         e.RecordAt,
		PayableAt:        e.PayableAt,
		RatioNumerator:   e.RatioNumerator,
		RatioDenominator: e.RatioDenominator,
		CashPerShare:     e.CashPerShare,
		CashCurrency:     e.CashCurrency,
		ParentBasisPct:   e.ParentBasisPct,
		ChildBasisPct:    e.ChildBasisPct,
		Status:           status,
		Quality:          q,
		SourceURL:        e.SourceURL,
		RawFingerprint:   fingerprint(e),
		RetrievedAt:      e.RetrievedAt,
		CreatedAt:        now,
		UpdatedAt:        now,
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
