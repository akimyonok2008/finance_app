package competitions

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// Boundary types for a competition's shared observation sets (migration
// 0050). Every entry baselined or finalized for a competition must be priced
// from the SAME stored observation set for that boundary — never from a
// fresh provider call — so batching (baseline) or retries (finalize) can
// never leave different participants priced at different real-world moments.
const (
	BoundaryStart = "start"
	BoundaryEnd   = "end"
)

// Observation set statuses. A set only ever becomes sealed once a worker
// pass both (a) captures every symbol/pair its own batch of entries needed
// and (b) that batch was the LAST one needed to cover the boundary's entire
// entry population — never from (a) alone. A batch that satisfies only (a)
// leaves the set pending, so a later batch that first sees a different
// symbol or currency pair is never masked by an already-"complete" set (see
// captureObservations).
const (
	ObservationPending = "pending"
	ObservationSealed  = "sealed"
)

// observationProvider tags which provider captured a competition's boundary
// observations, for audit. The engine only has one configured price/FX
// source at a time, so this is a fixed label rather than per-symbol metadata.
const observationProvider = "live_quote"

// PriceObservation is one symbol's frozen price for a competition boundary.
// ObservedAt is when this row was captured/persisted by the worker; it is
// bookkeeping, not the price's real-world moment. ProviderTimestamp is the
// latter: the provider's own timestamp for the price actually used (a
// trading session's close for Methodology == prices.MethodologySessionClose,
// or the live-fetch instant for prices.MethodologyFallbackLatest). See
// captureSinglePrice.
type PriceObservation struct {
	InstrumentID       string
	VenueMIC           string
	Symbol             string
	Price              money.Price
	Currency           string
	ObservedAt         time.Time
	ProviderTimestamp  time.Time
	TradingSessionDate string // YYYY-MM-DD, set when Methodology == prices.MethodologySessionClose
	Methodology        string
}

func observationKey(instrumentID, symbol string) string {
	if instrumentID != "" {
		return "id:" + instrumentID
	}
	return "sym:" + symbol
}

// FxObservation is one currency pair's frozen conversion rate for a
// competition boundary.
type FxObservation struct {
	FromCurrency string
	ToCurrency   string
	Rate         money.Ratio
	ObservedAt   time.Time
}

// ObservationSet is one competition's boundary observation record
// (competition_observation_sets). Status stays 'pending' until the LAST
// worker pass over the boundary's entire entry population captures every
// symbol and currency pair that pass still needed — never merely "the most
// recent pass's own batch is fully captured" (see captureObservations).
type ObservationSet struct {
	ID            string
	CompetitionID string
	Boundary      string
	EffectiveAt   time.Time
	Provider      string
	Status        string
	CompletedAt   *time.Time
}

// ObservationRepository is the persistence boundary for shared competition
// boundary observations (competition_observation_sets and its two children).
// Implemented by PostgresCompetitionRepository and InMemoryCompetitionRepository.
type ObservationRepository interface {
	// EnsureObservationSet returns the competition+boundary's observation set,
	// creating it (status pending) at effectiveAt if this is the first pass to
	// need it. Idempotent and safe under concurrent workers: effectiveAt and
	// provider are only ever written on first creation.
	EnsureObservationSet(ctx context.Context, competitionID, boundary string, effectiveAt time.Time, provider string) (ObservationSet, error)
	// LoadObservations returns every price/FX row already captured for the
	// set, keyed by observationKey and by "FROM->TO" respectively.
	LoadObservations(ctx context.Context, setID string) (map[string]PriceObservation, map[string]FxObservation, error)
	// RecordObservations persists newly captured price/FX rows (idempotent:
	// a symbol or pair already stored is never overwritten) and, when sealed
	// is true, marks the set sealed. sealed must only ever be passed as true
	// once the caller has verified BOTH that this batch's own requested
	// symbols/pairs are fully captured AND that this batch was the last one
	// needed to cover the boundary's entire entry population (see
	// captureObservations) — never from batch-local completeness alone, or a
	// later batch that first sees a different symbol/pair goes unrecorded
	// behind an already-"sealed" set. Safe to call with empty
	// newPrices/newFX just to flip a set to sealed once nothing is missing.
	// A set that is already sealed is terminal: implementations must never
	// unseal it or accept further writes against it.
	RecordObservations(ctx context.Context, setID string, newPrices []PriceObservation, newFX []FxObservation, sealed bool, now time.Time) error
}

// priceSource is what baselineEntry/valueCompetitionEntry actually need to
// price an entry's composition. observationMemo (baseline.go) implements it
// by calling the live provider once per symbol/pair and caching the result
// for the rest of the pass — used for continuous ranking refresh, where
// "current price" is the intended semantics. boundaryObservations implements
// it by reading only from a persisted competition_observation_sets row —
// used for baseline/finalize, where every participant must see the exact
// same stored start/end observation regardless of which pass or batch values
// them.
type priceSource interface {
	priceOf(ctx context.Context, instrumentID, symbol string) (money.Price, string, error)
	toBase(ctx context.Context, value money.Amount, currency string) (money.Amount, error)
}

// errObservationNotCaptured means the requested symbol/pair has not yet been
// captured into the competition's persisted boundary observation set. It is
// not a permanent failure: the caller's retry-until-window logic (identical
// to a normal provider outage) will pick it up once a later pass captures it.
var errObservationNotCaptured = fmt.Errorf("observation not yet captured for this boundary")

// boundaryObservations is the read-only, in-memory view of one competition's
// boundary observation set for the duration of a single worker pass. It
// never calls a provider — everything it can answer was already captured and
// persisted (by this pass or an earlier one).
type boundaryObservations struct {
	prices map[string]PriceObservation
	fx     map[string]FxObservation // "FROM->TO"
}

var _ priceSource = (*boundaryObservations)(nil)

func fxPairKey(from, to string) string { return from + "->" + to }

func (b *boundaryObservations) priceOf(_ context.Context, instrumentID, symbol string) (money.Price, string, error) {
	obs, ok := b.prices[observationKey(instrumentID, symbol)]
	if !ok {
		return money.Price{}, "", errObservationNotCaptured
	}
	return obs.Price, obs.Currency, nil
}

func (b *boundaryObservations) toBase(_ context.Context, value money.Amount, currency string) (money.Amount, error) {
	if currency == fx.BaseCurrency {
		return value, nil
	}
	obs, ok := b.fx[fxPairKey(currency, fx.BaseCurrency)]
	if !ok {
		return money.Amount{}, errObservationNotCaptured
	}
	return value.MulRatio(obs.Rate), nil
}

// entrySymbolsAndPairs collects the distinct symbols and "FROM->TO" currency
// pairs a batch of entries needs priced, restricted to scoring-included
// components (mirrors baselineEntry/valueCompetitionEntry's own filter).
func entrySymbolsAndPairs(entries []CompetitionEntry) (map[string]PriceObservation, map[string]struct{}) {
	symbols := map[string]PriceObservation{}
	pairs := map[string]struct{}{}
	addPair := func(currency string) {
		if currency != fx.BaseCurrency {
			pairs[fxPairKey(currency, fx.BaseCurrency)] = struct{}{}
		}
	}
	for _, e := range entries {
		for _, snap := range e.Snapshots {
			if !snap.IncludedInScore {
				continue
			}
			symbols[observationKey(snap.InstrumentID, snap.Symbol)] = PriceObservation{InstrumentID: snap.InstrumentID, VenueMIC: snap.VenueMIC, Symbol: snap.Symbol}
			// The position's priced currency is only known once it's been
			// captured, so both the position currency and the cash currency
			// are captured directly below via the cash snapshots; equity/ETF
			// symbols report their own currency from the provider quote at
			// capture time (see captureObservations).
		}
		for _, cash := range e.CashSnapshots {
			if !cash.IncludedInScore {
				continue
			}
			addPair(cash.Currency)
		}
	}
	return symbols, pairs
}

// captureSinglePrice resolves one instrument's frozen boundary price. It
// prefers the provider's history at-or-before effectiveAt (methodology
// prices.MethodologySessionClose today, since the only history source wired
// up is Twelve Data daily bars) over a live quote captured whenever this
// worker pass happens to run: a live quote at capture time can be minutes,
// hours, or — on a retried pass — days after the competition's declared
// start/end instant, silently mispricing the boundary. When no historical
// provider is configured, or the historical lookup itself fails (outage,
// symbol not covered, predates the symbol's history), it falls back to
// GetLatestPrice, explicitly tagged prices.MethodologyFallbackLatest so the
// imprecision is auditable rather than indistinguishable from an exact
// boundary price. Returns ok=false only when neither path can price the
// symbol at all.
func (s *Service) captureSinglePrice(ctx context.Context, ref PriceObservation, effectiveAt, now time.Time) (PriceObservation, bool) {
	if hp, ok := s.prices.(prices.HistoricalPriceProvider); ok {
		if hist, err := hp.PriceAtOrBefore(ctx, ref.Symbol, effectiveAt); err == nil {
			return PriceObservation{
				InstrumentID: ref.InstrumentID, VenueMIC: ref.VenueMIC, Symbol: ref.Symbol,
				Price: money.PriceFromFloat64(hist.Price), Currency: hist.Currency, ObservedAt: now,
				ProviderTimestamp: hist.ProviderTimestamp, TradingSessionDate: hist.TradingSessionDate,
				Methodology: hist.Methodology,
			}, true
		}
	}
	quote, err := s.prices.GetLatestPrice(ctx, ref.Symbol)
	if err != nil {
		return PriceObservation{}, false
	}
	return PriceObservation{
		InstrumentID: ref.InstrumentID, VenueMIC: ref.VenueMIC, Symbol: ref.Symbol,
		Price: money.PriceFromFloat64(quote.Price), Currency: quote.Currency, ObservedAt: now,
		ProviderTimestamp: now, Methodology: prices.MethodologyFallbackLatest,
	}, true
}

// captureObservations ensures the competition+boundary observation set
// exists, fetches (once, ever) any of the requested symbols/currency pairs
// that have not yet been captured by this or an earlier pass, persists them,
// and returns the full stored view for valuation. A symbol's currency is
// only known once its quote is fetched, so FX pairs discovered that way are
// captured in the same pass they're first seen.
//
// lastBatch must be true only when the caller's entries are the final batch
// needed to sweep the boundary's entire entry population (e.g. baseline's
// "fewer entries returned than the batch limit", finalize's own lapComplete)
// — never merely "true once per worker tick". The set is sealed only when
// this batch's own requested symbols/pairs are fully captured AND lastBatch
// is true, so a competition spanning more than one batch is never marked
// sealed off the first batch alone while a later batch still holds
// symbols/pairs no earlier batch had reason to request (see
// ObservationSet's doc comment for the bug this prevents).
//
// Once a set is sealed, this function never fetches another symbol/pair for
// it: a request for something outside the frozen set is left unresolved
// (returned via errObservationNotCaptured at read time) rather than
// silently expanding a set that downstream code already treats as closed.
func (s *Service) captureObservations(ctx context.Context, repo ObservationRepository, competitionID, boundary string, effectiveAt time.Time, entries []CompetitionEntry, lastBatch bool, now time.Time) (*boundaryObservations, error) {
	set, err := repo.EnsureObservationSet(ctx, competitionID, boundary, effectiveAt, observationProvider)
	if err != nil {
		return nil, fmt.Errorf("ensure observation set: %w", err)
	}
	existingPrices, existingFX, err := repo.LoadObservations(ctx, set.ID)
	if err != nil {
		return nil, fmt.Errorf("load observations: %w", err)
	}

	if set.Status == ObservationSealed {
		return &boundaryObservations{prices: existingPrices, fx: existingFX}, nil
	}

	symbols, pairs := entrySymbolsAndPairs(entries)

	var newPrices []PriceObservation
	for key, ref := range symbols {
		if _, ok := existingPrices[key]; ok {
			continue
		}
		obs, ok := s.captureSinglePrice(ctx, ref, effectiveAt, now)
		if !ok {
			continue // left uncaptured; retried on the next pass within the operational window
		}
		existingPrices[key] = obs
		newPrices = append(newPrices, obs)
		if obs.Currency != fx.BaseCurrency {
			pairs[fxPairKey(obs.Currency, fx.BaseCurrency)] = struct{}{}
		}
	}

	var newFX []FxObservation
	for pair := range pairs {
		if _, ok := existingFX[pair]; ok {
			continue
		}
		from, to, ok := strings.Cut(pair, "->")
		if !ok {
			continue
		}
		rate, err := s.fx.Convert(ctx, 1, from, to)
		if err != nil {
			continue // left uncaptured; retried on the next pass
		}
		obs := FxObservation{FromCurrency: from, ToCurrency: to, Rate: money.RatioFromFloat64(rate), ObservedAt: now}
		existingFX[pair] = obs
		newFX = append(newFX, obs)
	}

	batchCaptured := true
	for key := range symbols {
		if _, ok := existingPrices[key]; !ok {
			batchCaptured = false
			break
		}
	}
	if batchCaptured {
		for pair := range pairs {
			if _, ok := existingFX[pair]; !ok {
				batchCaptured = false
				break
			}
		}
	}
	// Sealing requires BOTH: this batch's own requested symbols/pairs are
	// fully captured, AND this batch was the last one needed for the whole
	// boundary population — batch-local completeness alone is exactly the
	// premature-completion bug this guards against.
	sealed := batchCaptured && lastBatch

	if len(newPrices) > 0 || len(newFX) > 0 || (sealed && set.Status != ObservationSealed) {
		if err := repo.RecordObservations(ctx, set.ID, newPrices, newFX, sealed, now); err != nil {
			return nil, fmt.Errorf("record observations: %w", err)
		}
	}

	return &boundaryObservations{prices: existingPrices, fx: existingFX}, nil
}

// --- in-memory implementation -------------------------------------------------

type memoryObservationSet struct {
	set    ObservationSet
	prices map[string]PriceObservation
	fx     map[string]FxObservation
}

var _ ObservationRepository = (*InMemoryCompetitionRepository)(nil)

func (r *InMemoryCompetitionRepository) observationSets() map[string]*memoryObservationSet {
	if r.observations == nil {
		r.observations = map[string]*memoryObservationSet{}
	}
	return r.observations
}

func observationSetKey(competitionID, boundary string) string { return competitionID + "|" + boundary }

func (r *InMemoryCompetitionRepository) EnsureObservationSet(_ context.Context, competitionID, boundary string, effectiveAt time.Time, provider string) (ObservationSet, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sets := r.observationSets()
	key := observationSetKey(competitionID, boundary)
	if existing, ok := sets[key]; ok {
		return existing.set, nil
	}
	set := ObservationSet{
		ID: key, CompetitionID: competitionID, Boundary: boundary,
		EffectiveAt: effectiveAt.UTC(), Provider: provider, Status: ObservationPending,
	}
	sets[key] = &memoryObservationSet{set: set, prices: map[string]PriceObservation{}, fx: map[string]FxObservation{}}
	return set, nil
}

func (r *InMemoryCompetitionRepository) LoadObservations(_ context.Context, setID string) (map[string]PriceObservation, map[string]FxObservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sets := r.observationSets()
	entry, ok := sets[setID]
	if !ok {
		return map[string]PriceObservation{}, map[string]FxObservation{}, nil
	}
	prices := make(map[string]PriceObservation, len(entry.prices))
	for k, v := range entry.prices {
		prices[k] = v
	}
	fxRates := make(map[string]FxObservation, len(entry.fx))
	for k, v := range entry.fx {
		fxRates[k] = v
	}
	return prices, fxRates, nil
}

func (r *InMemoryCompetitionRepository) RecordObservations(_ context.Context, setID string, newPrices []PriceObservation, newFX []FxObservation, sealed bool, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	sets := r.observationSets()
	entry, ok := sets[setID]
	if !ok {
		return ErrCompetitionNotFound
	}
	if entry.set.Status == ObservationSealed {
		// Terminal: a sealed set never accepts further writes or reverts.
		return nil
	}
	for _, p := range newPrices {
		key := observationKey(p.InstrumentID, p.Symbol)
		if _, exists := entry.prices[key]; !exists {
			entry.prices[key] = p
		}
	}
	for _, f := range newFX {
		key := fxPairKey(f.FromCurrency, f.ToCurrency)
		if _, exists := entry.fx[key]; !exists {
			entry.fx[key] = f
		}
	}
	if sealed {
		entry.set.Status = ObservationSealed
		stamp := now.UTC()
		entry.set.CompletedAt = &stamp
	}
	return nil
}
