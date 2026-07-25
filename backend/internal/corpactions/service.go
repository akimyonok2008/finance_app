package corpactions

import (
	"context"
	"fmt"
	"time"
)

// Holder is a portfolio that currently holds a symbol, with the time it was
// acquired (earliest open-position creation). Used to check effective-date
// eligibility so an action is not applied to someone who acquired the position
// after the action took effect.
type Holder struct {
	UserID      string
	PortfolioID string
	AcquiredAt  time.Time
}

// PortfolioGateway is the narrow portfolio surface the pipeline needs. It is
// implemented by an adapter over the portfolio service, keeping this package
// free of portfolio-domain types.
type PortfolioGateway interface {
	ActiveSymbols(ctx context.Context) ([]string, error)
	HoldersOfSymbol(ctx context.Context, symbol string) ([]Holder, error)
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
	switch ev.Type {
	case TypeSplit, TypeReverseSplit:
		if err := s.gateway.ApplySplit(ctx, h.UserID, requestID, ev.Source.Symbol,
			*ev.RatioNumerator, *ev.RatioDenominator, ev.EffectiveAt); err != nil {
			return err
		}
	case TypeSymbolChange:
		if err := s.gateway.ApplySymbolChange(ctx, h.UserID, requestID,
			ev.Source.Symbol, ev.Target.Symbol, ev.EffectiveAt); err != nil {
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
