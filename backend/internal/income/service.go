package income

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// Holder is a portfolio that currently holds a symbol, with the acquisition time
// (earliest open-position creation) and the position's asset type (used when a
// reinvestment must match or create the position).
type Holder struct {
	UserID      string
	PortfolioID string
	AssetType   string
	AcquiredAt  time.Time
}

type DiscoveryInstrument struct {
	InstrumentID string
	Symbol       string
	AssetType    string
}

// AppliedIncome is the neutral, provider-free instruction the pipeline hands to
// the portfolio gateway for ONE economic component of an event. Gross,
// withholding and fee are separated; the gateway credits net cash (or reinvests
// it) and records the immutable ledger activity.
type AppliedIncome struct {
	IncomeEventID     string
	Type              Type
	Classification    Classification
	Symbol            string
	AssetType         string
	Currency          string
	Reinvest          bool
	Gross             float64
	Withholding       float64
	Fee               float64
	ReinvestPrice     float64 // 0 → use current market price (estimated)
	PriceMethod       string
	StockRatioNum     float64
	StockRatioDen     float64
	Estimated         bool
	PaymentDate       time.Time
	TaxClassification string
}

// PortfolioGateway is the narrow portfolio surface the pipeline needs. It is
// implemented by an adapter over the portfolio service, keeping this package
// free of portfolio-domain types.
type PortfolioGateway interface {
	DiscoveryInstruments(ctx context.Context, since time.Time) ([]DiscoveryInstrument, error)
	HistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]Holder, error)
	// EligibleQuantity reconstructs the user's holding of symbol as of asOf from
	// the immutable ledger (historical entitlement, not current quantity).
	EligibleQuantity(ctx context.Context, userID, instrumentID, symbol string, asOf time.Time) (float64, error)
	// ApplyIncome credits (or reinvests) one income component idempotently by
	// requestID, through the aggregate coordinator.
	ApplyIncome(ctx context.Context, userID, requestID string, in AppliedIncome) error
	// ApplyCorrection posts a compensating cash adjustment (positive credits,
	// negative debits) for an already-applied event, idempotently by requestID.
	// It preserves the original activity and cannot fabricate arbitrary income.
	ApplyCorrection(ctx context.Context, userID, requestID string, adj CorrectionAdjustment) error
}

// Metrics is the observability sink. A nil metrics is a no-op.
type Metrics interface {
	Inc(name string)
	Observe(name string, value float64)
}

type nopMetrics struct{}

func (nopMetrics) Inc(string)              {}
func (nopMetrics) Observe(string, float64) {}

// Preferences captures the once-configured income handling for the single-user
// phase: whether income is held as cash or reinvested, with optional per-symbol
// overrides, plus the withholding profile and the estimated-gross policy.
type Preferences struct {
	ReinvestByDefault bool
	ReinvestSymbols   map[string]bool
	CashSymbols       map[string]bool
	Withholding       WithholdingProfile
	// UseEstimatedGross credits expected gross (marked estimated) when no actual
	// broker data exists. When false, income with unknown net stays pending.
	UseEstimatedGross bool
}

func (p Preferences) reinvest(symbol string) bool {
	s := strings.ToUpper(strings.TrimSpace(symbol))
	if p.CashSymbols != nil && p.CashSymbols[s] {
		return false
	}
	if p.ReinvestSymbols != nil && p.ReinvestSymbols[s] {
		return true
	}
	return p.ReinvestByDefault
}

// Service is the income ingestion and application pipeline.
type Service struct {
	provider IncomeEventProvider
	store    Store
	gateway  PortfolioGateway
	metrics  Metrics
	prefs    Preferences
	now      func() time.Time
	lookback time.Duration
	retryIn  time.Duration
}

// NewService wires the pipeline with the default policy: credit expected gross
// automatically (marked estimated), hold as cash, no withholding.
func NewService(provider IncomeEventProvider, store Store, gateway PortfolioGateway) *Service {
	return &Service{
		provider: provider, store: store, gateway: gateway, metrics: nopMetrics{},
		prefs:    Preferences{UseEstimatedGross: true},
		now:      func() time.Time { return time.Now().UTC() },
		lookback: 120 * 24 * time.Hour,
		retryIn:  time.Hour,
	}
}

func (s *Service) SetClock(now func() time.Time) { s.now = now }
func (s *Service) SetMetrics(m Metrics) {
	if m != nil {
		s.metrics = m
	}
}
func (s *Service) SetLookback(d time.Duration) {
	if d > 0 {
		s.lookback = d
	}
}
func (s *Service) SetRetryInterval(d time.Duration) {
	if d > 0 {
		s.retryIn = d
	}
}
func (s *Service) SetPreferences(p Preferences) { s.prefs = p }

// RunOnce performs a full ingest + process cycle. The background worker calls it
// on the configured interval.
func (s *Service) RunOnce(ctx context.Context) error {
	if err := s.Ingest(ctx); err != nil {
		return err
	}
	return s.Process(ctx)
}

// Ingest fetches provider events for currently-held symbols, normalizes them,
// and upserts them idempotently (detecting corrections).
func (s *Service) Ingest(ctx context.Context) error {
	since := s.now().Add(-s.lookback)
	instruments, err := s.gateway.DiscoveryInstruments(ctx, since)
	if err != nil {
		return err
	}
	// Keep provider aliases for events already in a non-terminal lifecycle even
	// when the portfolio episode has since disappeared from the current view.
	pending, err := s.store.ListEventsByStatus(ctx,
		StatusDetected, StatusScheduled, StatusEligibilityCalcuated, StatusAwaitingPayment,
		StatusReadyToApply, StatusUnresolved, StatusFailedRetryable, StatusProcessedNoEntitlement)
	if err != nil {
		return err
	}
	byKey := map[string]DiscoveryInstrument{}
	for _, item := range instruments {
		byKey[discoveryKey(item.InstrumentID, item.Symbol)] = item
	}
	for _, ev := range pending {
		item := DiscoveryInstrument{InstrumentID: ev.Instrument.InstrumentID, Symbol: ev.Instrument.Symbol}
		byKey[discoveryKey(item.InstrumentID, item.Symbol)] = item
	}
	instruments = instruments[:0]
	for _, item := range byKey {
		instruments = append(instruments, item)
	}
	if len(instruments) == 0 {
		return nil
	}
	req := IncomeEventRequest{
		Instruments: discoveryRequestInstruments(instruments),
		Since:       since,
		Until:       s.now().Add(90 * 24 * time.Hour), // include near-future announced events
	}
	raw, err := s.provider.FetchIncomeEvents(ctx, req)
	if err != nil {
		return err
	}
	for _, e := range raw {
		if e.Instrument.InstrumentID == "" {
			if item, ok := byKey[discoveryKey("", e.Instrument.Symbol)]; ok {
				e.Instrument.InstrumentID = item.InstrumentID
			} else {
				for _, item := range instruments {
					if normalizeDiscoverySymbol(item.Symbol) == normalizeDiscoverySymbol(e.Instrument.Symbol) {
						e.Instrument.InstrumentID = item.InstrumentID
						break
					}
				}
			}
		}
		s.metrics.Inc("income_events_fetched_total")
		ev := normalize(s.provider.Name(), e, s.now())
		changed, err := s.store.UpsertEvent(ctx, ev)
		if err != nil {
			return err
		}
		s.metrics.Inc("income_events_normalized_total")
		if changed {
			s.metrics.Inc("income_events_corrected_total")
		}
		if ev.Status == StatusCancelled {
			s.metrics.Inc("income_events_cancelled_total")
		}
	}
	return nil
}

// Process applies every effective, high-quality, due income event to each
// eligible portfolio. Announced-but-not-yet-paid events are left scheduled;
// incomplete/conflicting events stay unresolved. One portfolio's failure never
// aborts the others.
func (s *Service) Process(ctx context.Context) error {
	events, err := s.store.ListEventsByStatus(ctx,
		StatusDetected, StatusScheduled, StatusEligibilityCalcuated, StatusAwaitingPayment,
		StatusReadyToApply, StatusUnresolved, StatusFailedRetryable, StatusProcessedNoEntitlement)
	if err != nil {
		return err
	}
	for _, ev := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.processEvent(ctx, ev)
	}
	return nil
}

func (s *Service) processEvent(ctx context.Context, ev IncomeEvent) {
	if ev.Status == StatusCancelled || ev.Quality == QualityCancelled {
		return // a cancelled event is never credited
	}
	if ev.Status == StatusSuperseded {
		return // handled by the correction workflow, not re-applied
	}
	// Incomplete/conflicting or account-specific-without-brokerage events wait for
	// complete authoritative data.
	if !ev.Quality.meetsAutoApplyPolicy() || requiresAccountData(ev.Type) && !s.trustedForAccountData(ev) {
		if ev.Status != StatusUnresolved {
			_ = s.store.SetEventStatus(ctx, ev.ID, StatusUnresolved)
		}
		s.metrics.Inc("income_event_eligibility_failures_total")
		return
	}
	// Payment-date semantics: never credit before the payment date.
	if s.now().Before(ev.PaymentDate) {
		if ev.Status != StatusAwaitingPayment {
			_ = s.store.SetEventStatus(ctx, ev.ID, StatusAwaitingPayment)
		}
		return
	}

	holders, err := s.gateway.HistoricalHolders(ctx, ev.Instrument.InstrumentID, ev.Instrument.Symbol)
	if err != nil {
		return
	}
	applied := 0
	for _, h := range holders {
		if s.applyToHolder(ctx, ev, h) {
			applied++
		}
	}
	if applied > 0 {
		_ = s.store.SetEventStatus(ctx, ev.ID, StatusApplied)
	} else {
		_ = s.store.SetEventStatus(ctx, ev.ID, StatusProcessedNoEntitlement)
	}
}

// trustedForAccountData reports whether the ingesting provider is authorised to
// supply this account-specific event. The manual dev provider filters these out,
// so an account-specific event only reaches here from a brokerage adapter.
func (s *Service) trustedForAccountData(ev IncomeEvent) bool {
	return strings.Contains(ev.Provider, "brokerage") || strings.Contains(ev.Provider, "broker")
}

// applyToHolder computes eligibility and amounts for one portfolio and applies
// every component atomically. It returns true when the holder was credited.
func (s *Service) applyToHolder(ctx context.Context, ev IncomeEvent, h Holder) bool {
	entitlementDate := ev.entitlementDate()
	// Do not use the position row's CreatedAt as an entitlement shortcut:
	// backdated trades are recorded later than their economic OccurredAt. The
	// immutable activity ledger below is the authoritative cutoff calculation.
	eligible, err := s.gateway.EligibleQuantity(ctx, h.UserID, ev.Instrument.InstrumentID, ev.Instrument.Symbol, entitlementDate)
	if err != nil {
		s.metrics.Inc("income_event_eligibility_failures_total")
		return false
	}
	if !(eligible > 0) || math.IsInf(eligible, 0) || math.IsNaN(eligible) {
		return false
	}

	claimed, err := s.store.ClaimApplication(ctx, ev.ID, h.PortfolioID, h.UserID)
	if err != nil || !claimed {
		if err == nil {
			s.metrics.Inc("income_event_duplicate_deliveries_total")
		}
		return false
	}

	components := s.components(ev)
	reinvest := s.prefs.reinvest(ev.Instrument.Symbol)
	var totalGross, totalWithholding, totalNet, reinvestQty float64
	estimatedAny := false
	for i, comp := range components {
		in := s.buildApplied(ev, comp, h, eligible, reinvest, i)
		requestID := fmt.Sprintf("inc:%s:%s:%d", ev.ID, h.PortfolioID, i)
		if err := s.gateway.ApplyIncome(ctx, h.UserID, requestID, in); err != nil {
			s.metrics.Inc("income_event_application_failures_total")
			_ = s.store.FailApplication(ctx, ev.ID, h.PortfolioID, err.Error(), s.now().Add(s.retryIn))
			return false
		}
		totalGross += in.Gross
		totalWithholding += in.Withholding
		totalNet += in.Gross - in.Withholding - in.Fee
		if in.Estimated {
			estimatedAny = true
		}
		if in.Reinvest && in.ReinvestPrice > 0 {
			reinvestQty += (in.Gross - in.Withholding - in.Fee) / in.ReinvestPrice
		}
	}

	now := s.now()
	_ = s.store.CompleteApplication(ctx, Application{
		IncomeEventID: ev.ID, PortfolioID: h.PortfolioID, UserID: h.UserID,
		Status: ApplicationApplied, EligibleQuantity: eligible,
		GrossAmount: round2(totalGross), WithholdingAmount: round2(totalWithholding),
		NetAmount: round2(totalNet), CashCurrency: ev.Currency,
		ReinvestmentQuantity: reinvestQty, Estimated: estimatedAny, AppliedAt: &now,
	})
	s.metrics.Inc("income_events_applied_total")
	s.metrics.Observe("income_event_application_lag_seconds", now.Sub(ev.PaymentDate).Seconds())
	return true
}

// components returns the economic slices of an event. A mixed distribution
// yields one AppliedIncome per component; everything else yields one.
func (s *Service) components(ev IncomeEvent) []ProviderComponent {
	if len(ev.Components) > 0 {
		return ev.Components
	}
	return []ProviderComponent{{Type: ev.Type, AmountPerUnit: ev.AmountPerUnit}}
}

// buildApplied computes gross/withholding/net for one component and packages the
// neutral instruction for the gateway.
func (s *Service) buildApplied(ev IncomeEvent, comp ProviderComponent, h Holder, eligible float64, reinvest bool, idx int) AppliedIncome {
	compType := comp.Type
	if compType == "" {
		compType = ev.Type
	}
	class := Classify(compType)
	in := AppliedIncome{
		IncomeEventID: ev.ID, Type: compType, Classification: class,
		Symbol: ev.Instrument.Symbol, AssetType: h.AssetType, Currency: ev.Currency,
		PaymentDate: ev.PaymentDate, TaxClassification: ev.TaxClassification,
	}
	if class == ClassStockDividend {
		// AmountPerUnit carries new-shares-per-held-share.
		in.StockRatioNum = comp.AmountPerUnit
		in.StockRatioDen = 1
		return in
	}
	gross := round2(eligible * comp.AmountPerUnit)
	in.Gross = gross
	// Withholding is an OPTIONAL account-level estimate; default credits gross.
	rate := s.prefs.Withholding.Rate(compType, ev.Instrument.Symbol)
	if rate > 0 {
		in.Withholding = round2(gross * rate)
	}
	// Reinvestment only applies to ordinary cash income (never return of capital).
	if reinvest && class == ClassOrdinary {
		in.Reinvest = true
		in.PriceMethod = "market_close_on_payment_date"
	}
	// Estimated-gross policy: without actual broker data, the gross is a provider
	// expectation.
	in.Estimated = s.prefs.UseEstimatedGross
	return in
}

// HandleCorrection records an account-specific correction to an already-detected
// event as a compensating adjustment. It never fabricates new income and is
// idempotent by requestID.
func (s *Service) HandleCorrection(ctx context.Context, userID string, c Correction) error {
	return s.applyCorrection(ctx, userID, c)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
