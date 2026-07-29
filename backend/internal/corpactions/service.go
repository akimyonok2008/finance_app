package corpactions

import (
	"context"
	"fmt"
	"time"

	"github.com/ardakimyonok/finance_app/internal/instrument"
	"github.com/ardakimyonok/finance_app/internal/telemetry"
)

// Holder is a portfolio that currently holds a symbol, with the time it was
// acquired (earliest open-position creation). Used to check effective-date
// eligibility so an action is not applied to someone who acquired the position
// after the action took effect.
type Holder struct {
	UserID      string
	PortfolioID string
	AcquiredAt  time.Time
	// Symbol is the position's ACTUAL stored symbol, which may differ from
	// the event's Source.Symbol when the holder was discovered via
	// HoldersOfInstrument (a resolved identity survives ticker drift that a
	// symbol string does not). applyToHolder applies against this, never
	// against the event's symbol, so the mutation always targets the real
	// position.
	Symbol string
}

// PortfolioGateway is the narrow portfolio surface the pipeline needs. It is
// implemented by an adapter over the portfolio service, keeping this package
// free of portfolio-domain types.
type PortfolioGateway interface {
	ActiveSymbols(ctx context.Context) ([]string, error)
	HoldersOfSymbol(ctx context.Context, symbol string) ([]Holder, error)
	// HoldersOfInstrument is the identity-based counterpart of
	// HoldersOfSymbol, preferred over it whenever the event's source resolved
	// to a stable instrument identity (see Service.matchInstrument) — it
	// survives a ticker rename between when a position was opened and when
	// the corporate action is processed, which symbol matching cannot.
	HoldersOfInstrument(ctx context.Context, instrumentID string) ([]Holder, error)
	// ApplySplit applies a (reverse) split to a user's holding idempotently by
	// requestID. num/den are the split ratio.
	ApplySplit(ctx context.Context, userID, requestID, symbol string, num, den float64, effectiveAt time.Time) error
	// ApplySymbolChange re-tickers a user's holding idempotently by requestID.
	ApplySymbolChange(ctx context.Context, userID, requestID, oldSymbol, newSymbol string, effectiveAt time.Time) error
}

// Metrics is the observability sink. A nil metrics is a no-op.
type Metrics interface {
	Inc(name string)
}

type nopMetrics struct{}

func (nopMetrics) Inc(string) {}

// Service is the corporate-action ingestion and application pipeline.
type Service struct {
	provider CorporateActionProvider
	store    Store
	gateway  PortfolioGateway
	metrics  Metrics
	now      func() time.Time
	lookback time.Duration
	retryIn  time.Duration
	// identity resolves a provider's InstrumentReference to a stable
	// instrument identity (see matchInstrument). nil disables matching
	// entirely — every event stays exactly as symbol-based as before this
	// was wired, which is the correct degraded behavior for an environment
	// with no resolver configured.
	identity *instrument.Resolver
}

// NewService wires the pipeline.
func NewService(provider CorporateActionProvider, store Store, gateway PortfolioGateway) *Service {
	return &Service{
		provider: provider, store: store, gateway: gateway, metrics: nopMetrics{},
		now:      func() time.Time { return time.Now().UTC() },
		lookback: 7 * 24 * time.Hour,
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

// SetInstrumentResolver wires identity resolution into Ingest (see
// matchInstrument). nil (the zero value) is the safe default: matching is
// simply skipped and every event stays symbol-based, exactly as before this
// was introduced.
func (s *Service) SetInstrumentResolver(resolver *instrument.Resolver) {
	s.identity = resolver
}

// matchInstrument resolves ref's stable instrument identity in place, using
// the same FIGI/ISIN/CUSIP-first, then-ticker+MIC resolution order as the
// portfolio buy path (Service.resolveBuyIdentity). A miss is not an error —
// an unresolved or ambiguous provider event is an expected outcome, recorded
// via telemetry so the deployment can see how much of the corporate-action
// feed still has no stable identity to key on.
func (s *Service) matchInstrument(ctx context.Context, ref *InstrumentReference) {
	if s.identity == nil || ref == nil || ref.Symbol == "" {
		return
	}
	resolution, err := s.identity.ResolveDetailedAt(ctx, instrument.IdentityQuery{
		Ticker: ref.Symbol, ExchangeCode: ref.Exchange, MIC: ref.MIC,
		FIGI: ref.FIGI, ISIN: ref.ISIN, CUSIP: ref.CUSIP,
	}, nil)
	if err != nil {
		return
	}
	switch resolution.Quality {
	case instrument.QualityAmbiguous:
		telemetry.IncInstrumentResolutionAmbiguous()
	case instrument.QualityUnresolved:
		telemetry.IncInstrumentResolutionUnresolved()
	default:
		if resolution.Instrument != nil {
			ref.InstrumentID = resolution.Instrument.ID
		}
	}
}

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
	symbols, err := s.gateway.ActiveSymbols(ctx)
	if err != nil {
		return err
	}
	if len(symbols) == 0 {
		return nil
	}
	req := CorporateActionRequest{
		Instruments: requestInstruments(symbols),
		Since:       s.now().Add(-s.lookback),
		Until:       s.now().Add(90 * 24 * time.Hour), // include near-future announced events
	}
	raw, err := s.provider.FetchActions(ctx, req)
	if err != nil {
		return err
	}
	for _, e := range raw {
		s.metrics.Inc("corporate_actions_fetched_total")
		ev := normalize(s.provider.Name(), e, s.now())
		s.matchInstrument(ctx, &ev.Source)
		if ev.Target != nil {
			s.matchInstrument(ctx, ev.Target)
		}
		// Mergers/spin-offs/cash-mergers never auto-apply regardless of
		// identity (see autoAppliesType) — an unresolved source identity on
		// one of these is worth surfacing distinctly from the generic
		// "pending complete data" bucket, since it flags the review queue
		// this whole identity layer exists to feed.
		if !s.autoAppliesType(ev.Type) && ev.Source.InstrumentID == "" {
			s.metrics.Inc("corporate_action_quarantined_total")
			telemetry.IncCorporateActionQuarantined()
		}
		changed, err := s.store.UpsertEvent(ctx, ev)
		if err != nil {
			return err
		}
		s.metrics.Inc("corporate_actions_normalized_total")
		if changed {
			s.metrics.Inc("corporate_action_corrections_total")
		}
	}
	return nil
}

// Process applies every effective, high-quality, auto-applicable event to each
// eligible portfolio. Incomplete or complex events are left pending/unresolved.
// One portfolio's failure never aborts the others.
func (s *Service) Process(ctx context.Context) error {
	events, err := s.store.ListEventsByStatus(ctx,
		StatusDetected, StatusValidated, StatusScheduled, StatusUnresolved, StatusFailedRetryable)
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

func (s *Service) processEvent(ctx context.Context, ev CorporateAction) {
	if ev.Status == StatusCancelled || ev.Quality == QualityCancelled {
		return
	}
	// Complex or incomplete events are never auto-applied: keep the position
	// visible and wait for complete provider data.
	if !s.isAutoApplicable(ev) {
		if ev.Quality == QualityIncomplete || !s.autoAppliesType(ev.Type) {
			s.metrics.Inc("corporate_actions_pending_total")
			if ev.Status != StatusUnresolved {
				_ = s.store.SetEventStatus(ctx, ev.ID, StatusUnresolved)
			}
		}
		return
	}

	holders, err := s.gateway.HoldersOfSymbol(ctx, ev.Source.Symbol)
	if err != nil {
		return
	}
	// Identity-based discovery is additive, never a replacement: it catches
	// holders whose position.symbol has drifted from the provider's current
	// ticker (e.g. a rename this pipeline didn't apply), without risking
	// dropping any holder the existing symbol match already finds.
	if ev.Source.InstrumentID != "" {
		byInstrument, err := s.gateway.HoldersOfInstrument(ctx, ev.Source.InstrumentID)
		if err == nil {
			seen := make(map[string]bool, len(holders))
			for _, h := range holders {
				seen[h.PortfolioID] = true
			}
			for _, h := range byInstrument {
				if !seen[h.PortfolioID] {
					seen[h.PortfolioID] = true
					holders = append(holders, h)
				}
			}
		}
	}
	applied := 0
	for _, h := range holders {
		// Effective-date eligibility: a holder who acquired the position AFTER the
		// action took effect is not entitled to it.
		if h.AcquiredAt.After(ev.EffectiveAt) {
			_ = s.store.SkipApplication(ctx, ev.ID, h.PortfolioID, "acquired_after_effective")
			continue
		}
		claimed, err := s.store.ClaimApplication(ctx, ev.ID, h.PortfolioID, h.UserID)
		if err != nil || !claimed {
			if err == nil {
				s.metrics.Inc("corporate_action_duplicate_deliveries_total")
			}
			continue
		}
		if err := s.applyToHolder(ctx, ev, h); err != nil {
			s.metrics.Inc("corporate_actions_failed_total")
			_ = s.store.FailApplication(ctx, ev.ID, h.PortfolioID, err.Error(), s.now().Add(s.retryIn))
			continue
		}
		applied++
		s.metrics.Inc("corporate_actions_applied_total")
	}
	if applied > 0 || len(holders) == 0 {
		_ = s.store.SetEventStatus(ctx, ev.ID, StatusApplied)
	}
}

// applyToHolder performs the actual portfolio transformation through the
// gateway, which routes to the aggregate coordinator (atomic, idempotent).
func (s *Service) applyToHolder(ctx context.Context, ev CorporateAction, h Holder) error {
	requestID := fmt.Sprintf("ca:%s:%s", ev.ID, h.PortfolioID)
	// Apply against the holder's ACTUAL stored symbol (h.Symbol), never
	// ev.Source.Symbol directly: when h was discovered via
	// HoldersOfInstrument, the event's symbol string may not be what the
	// position is stored under (that's exactly the drift identity
	// resolution exists to survive), and the mutation coordinator matches
	// positions by symbol.
	symbol := h.Symbol
	if symbol == "" {
		symbol = ev.Source.Symbol
	}
	switch ev.Type {
	case TypeSplit, TypeReverseSplit:
		if err := s.gateway.ApplySplit(ctx, h.UserID, requestID, symbol,
			*ev.RatioNumerator, *ev.RatioDenominator, ev.EffectiveAt); err != nil {
			return err
		}
	case TypeSymbolChange:
		if err := s.gateway.ApplySymbolChange(ctx, h.UserID, requestID,
			symbol, ev.Target.Symbol, ev.EffectiveAt); err != nil {
			return err
		}
	default:
		return fmt.Errorf("type %s is not auto-applicable", ev.Type)
	}
	now := s.now()
	return s.store.CompleteApplication(ctx, Application{
		CorporateActionID: ev.ID, PortfolioID: h.PortfolioID, UserID: h.UserID,
		Status: ApplicationApplied, AppliedAt: &now,
	})
}

// autoAppliesType reports whether a type is ever auto-applied in this phase.
// Splits and straightforward symbol changes are; mergers, spin-offs, and
// delistings are held pending complete authoritative data.
func (s *Service) autoAppliesType(t Type) bool {
	return t == TypeSplit || t == TypeReverseSplit || t == TypeSymbolChange
}

// isAutoApplicable applies the full automatic-application policy.
func (s *Service) isAutoApplicable(ev CorporateAction) bool {
	return s.autoAppliesType(ev.Type) &&
		ev.Quality.meetsAutoApplyPolicy() &&
		ev.Quality == QualityVerified && // must be effective (assessQuality)
		s.termsComplete(ev)
}

func (s *Service) termsComplete(ev CorporateAction) bool {
	switch ev.Type {
	case TypeSplit, TypeReverseSplit:
		return ev.RatioNumerator != nil && ev.RatioDenominator != nil &&
			*ev.RatioNumerator > 0 && *ev.RatioDenominator > 0
	case TypeSymbolChange:
		return ev.Target != nil && ev.Target.Symbol != ""
	default:
		return false
	}
}
