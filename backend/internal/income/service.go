package income

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
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
	Gross             money.Amount
	Withholding       money.Amount
	Fee               money.Amount
	ReinvestPrice     money.Price // 0 → use current market price (estimated)
	PriceMethod       string
	StockRatioNum     money.Ratio
	StockRatioDen     money.Ratio
	Estimated         bool
	PaymentDate       time.Time
	TaxClassification string
}

// AppliedIncomeComponent is one economic slice of a mixed income event (e.g.
// the ordinary-income, capital-gains-distribution, and return-of-capital
// slices of a single ETF distribution), in the neutral shape the gateway
// converts to a portfolio-domain component.
type AppliedIncomeComponent struct {
	Type              Type
	Classification    Classification
	Reinvest          bool
	Gross             money.Amount
	Withholding       money.Amount
	Fee               money.Amount
	ReinvestPrice     money.Price
	PriceMethod       string
	StockRatioNum     money.Ratio
	StockRatioDen     money.Ratio
	Estimated         bool
	TaxClassification string
}

// AppliedIncomeEvent is the neutral instruction the pipeline hands to the
// portfolio gateway for a FULL economic income event, possibly mixing several
// components (ordinary income, capital-gains distribution, return of capital,
// stock dividend, ...). The gateway applies every component ATOMICALLY: all
// components commit together in one transaction or none do.
type AppliedIncomeEvent struct {
	IncomeEventID string
	Symbol        string
	AssetType     string
	Currency      string
	PaymentDate   time.Time
	Components    []AppliedIncomeComponent
}

// PortfolioGateway is the narrow portfolio surface the pipeline needs. It is
// implemented by an adapter over the portfolio service, keeping this package
// free of portfolio-domain types.
type PortfolioGateway interface {
	DiscoveryInstruments(ctx context.Context, since time.Time) ([]DiscoveryInstrument, error)
	HistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]Holder, error)
	// EligibleQuantity reconstructs the user's holding of symbol as of asOf from
	// the immutable ledger (historical entitlement, not current quantity).
	EligibleQuantity(ctx context.Context, userID, instrumentID, symbol string, asOf time.Time) (money.Quantity, error)
	// ApplyIncomeEvent credits (and/or reinvests) EVERY component of one
	// economic income event atomically, through the aggregate coordinator: one
	// locked transaction, one activity group, one combined ranked-performance
	// update. requestID must be ONE deterministic key per (income event,
	// portfolio) pair — a retry with the same key replays the committed result
	// instead of applying anything twice. If any component cannot be applied
	// safely, no component is applied and a non-nil error is returned.
	ApplyIncomeEvent(ctx context.Context, userID, requestID string, in AppliedIncomeEvent) error
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
	// Do not use the position row's CreatedAt as an entitlement shortcut: a
	// position can accumulate ledger activity (partial sales, corporate
	// actions) whose OccurredAt differs from when the row was first created.
	// The immutable activity ledger below is the authoritative cutoff
	// calculation.
	eligible, err := s.gateway.EligibleQuantity(ctx, h.UserID, ev.Instrument.InstrumentID, ev.Instrument.Symbol, entitlementDate)
	if err != nil {
		s.metrics.Inc("income_event_eligibility_failures_total")
		return false
	}
	if eligible.Sign() <= 0 {
		return false
	}

	claimed, err := s.store.ClaimApplication(ctx, ev.ID, h.PortfolioID, h.UserID)
	if err != nil || !claimed {
		if err == nil {
			s.metrics.Inc("income_event_duplicate_deliveries_total")
		}
		return false
	}

	rawComponents := s.components(ev)
	reinvest := s.prefs.reinvest(ev.Instrument.Symbol)

	// Build EVERY component of this economic event BEFORE calling the gateway.
	// The whole event is applied in ONE call under ONE deterministic request ID
	// per (income event, portfolio) pair, so a retry replays the committed
	// result and partial application (some components committed, some not) is
	// impossible: either every component of this event commits atomically, or
	// none of them do.
	appliedEvent := AppliedIncomeEvent{
		IncomeEventID: ev.ID, Symbol: ev.Instrument.Symbol, AssetType: h.AssetType,
		Currency: ev.Currency, PaymentDate: ev.PaymentDate,
		Components: make([]AppliedIncomeComponent, 0, len(rawComponents)),
	}
	totalGross := money.ZeroAmount()
	totalWithholding := money.ZeroAmount()
	totalFee := money.ZeroAmount()
	totalNet := money.ZeroAmount()
	reinvestQty := money.ZeroQuantity()
	estimatedAny := false
	for _, comp := range rawComponents {
		in := s.buildApplied(ev, comp, h, eligible, reinvest)
		appliedEvent.Components = append(appliedEvent.Components, in)
		totalGross = totalGross.Add(in.Gross)
		totalWithholding = totalWithholding.Add(in.Withholding)
		totalFee = totalFee.Add(in.Fee)
		totalNet = totalNet.Add(in.Gross.Sub(in.Withholding).Sub(in.Fee))
		if in.Estimated {
			estimatedAny = true
		}
		if in.Reinvest && in.ReinvestPrice.Sign() > 0 {
			qty, divErr := in.Gross.Sub(in.Withholding).Sub(in.Fee).DivByPrice(in.ReinvestPrice, money.ScaleQuantity)
			if divErr == nil {
				reinvestQty = reinvestQty.Add(qty)
			}
		}
	}

	// One deterministic request ID per (income event, portfolio) pair — never
	// per component — so a retry after a committed event creates no
	// duplicate cash, shares, basis reduction, activities, or ranked return.
	requestID := fmt.Sprintf("inc:%s:%s", ev.ID, h.PortfolioID)
	if err := s.gateway.ApplyIncomeEvent(ctx, h.UserID, requestID, appliedEvent); err != nil {
		// Mark retryable, never partially successful: the whole atomic mutation
		// either committed or it didn't, so there is nothing partially applied to
		// reconcile here.
		s.metrics.Inc("income_event_application_failures_total")
		_ = s.store.FailApplication(ctx, ev.ID, h.PortfolioID, err.Error(), s.now().Add(s.retryIn))
		return false
	}

	now := s.now()
	_ = s.store.CompleteApplication(ctx, Application{
		IncomeEventID: ev.ID, PortfolioID: h.PortfolioID, UserID: h.UserID,
		Status: ApplicationApplied, EligibleQuantity: eligible,
		GrossAmount: totalGross, WithholdingAmount: totalWithholding, FeeAmount: totalFee,
		NetAmount: totalNet, CashCurrency: ev.Currency,
		ReinvestmentQuantity: money.QuantizeQuantity(reinvestQty), Estimated: estimatedAny, AppliedAt: &now,
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

// buildApplied computes gross/withholding/net for one component of an event
// and packages it as the neutral per-component instruction the gateway
// bundles, with every other component, into one atomic AppliedIncomeEvent.
func (s *Service) buildApplied(ev IncomeEvent, comp ProviderComponent, h Holder, eligible money.Quantity, reinvest bool) AppliedIncomeComponent {
	compType := comp.Type
	if compType == "" {
		compType = ev.Type
	}
	class := Classify(compType)
	in := AppliedIncomeComponent{
		Type: compType, Classification: class, TaxClassification: ev.TaxClassification,
	}
	if class == ClassStockDividend {
		// AmountPerUnit carries new-shares-per-held-share.
		in.StockRatioNum = money.MustRatio(comp.AmountPerUnit.String())
		in.StockRatioDen = money.MustRatio("1")
		return in
	}
	gross, err := money.QuantizeCash(eligible.MulPrice(comp.AmountPerUnit), ev.Currency)
	if err != nil {
		return in
	}
	in.Gross = gross
	// Withholding is an OPTIONAL account-level estimate; default credits gross.
	rate := s.prefs.Withholding.Rate(compType, ev.Instrument.Symbol)
	if rate.Sign() > 0 {
		in.Withholding, _ = money.QuantizeCash(gross.MulRatio(rate), ev.Currency)
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
