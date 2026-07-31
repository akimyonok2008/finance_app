package competitions

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// defaultBaselineWindow is how long after starts_at the baseline worker keeps
// retrying an entry whose start prices are unavailable before marking it
// baseline_failed with auditable evidence. Entries are never silently
// omitted: they are either baselined or explicitly failed.
const defaultBaselineWindow = time.Hour

// defaultBaselineBatch bounds how many pending entries one worker pass
// values per competition.
const defaultBaselineBatch = 200

// SetBaselineWindow overrides the operational window (d <= 0 ignored).
func (s *Service) SetBaselineWindow(d time.Duration) {
	if d > 0 {
		s.baselineWindow = d
	}
}

// AdvanceCompetitionLifecycles moves engine editions along their scheduled
// path as their timestamps arrive:
//
//	published            -> registration_open    at join_opens_at
//	registration_open    -> registration_closed  at join_closes_at
//	registration_closed  -> active               at starts_at
//	active               -> finalizing           at ends_at
//
// Every move goes through the guarded TransitionLifecycle, so a concurrent
// admin action (e.g. cancellation) wins and the scheduler simply skips.
// Legacy sprint rows are untouched — they have no stored lifecycle.
func (s *Service) AdvanceCompetitionLifecycles(ctx context.Context) error {
	editions, ok := s.repo.(EngineEntryRepository)
	if !ok {
		return nil
	}
	now := s.clock.Now().UTC()
	due := []struct {
		from string
		to   string
		at   func(*Competition) *time.Time
	}{
		{LifecyclePublished, LifecycleRegistrationOpen, func(c *Competition) *time.Time { return c.JoinOpensAt }},
		{LifecycleRegistrationOpen, LifecycleRegistrationClosed, func(c *Competition) *time.Time { return c.JoinClosesAt }},
		{LifecycleRegistrationClosed, LifecycleActive, func(c *Competition) *time.Time { t := c.StartsAt; return &t }},
		{LifecycleActive, LifecycleFinalizing, func(c *Competition) *time.Time { t := c.EndsAt; return &t }},
	}
	for _, step := range due {
		list, err := editions.ListEditionsByLifecycle(ctx, step.from)
		if err != nil {
			return err
		}
		for _, c := range list {
			at := step.at(&c)
			if at == nil || now.Before(*at) {
				continue
			}
			err := s.repo.(EditionRepository).TransitionLifecycle(ctx, c.ID, step.from, step.to, now)
			if err != nil {
				slog.Warn("competition_lifecycle_advance_failed",
					"competition_id", c.ID, "from", step.from, "to", step.to, "error", err)
				continue
			}
			slog.Info("competition_lifecycle_advanced",
				"competition_id", c.ID, "from", step.from, "to", step.to)
		}
	}
	return nil
}

// RunCompetitionBaselines establishes the common start-time baseline for
// every admitted entry of every started edition:
//
//  1. Price the frozen included composition from the competition's persisted
//     start observation set (competition_observation_sets), capturing any
//     symbol/pair not yet seen and never re-querying one already captured —
//     so every entry, in any batch or pass, is baselined from identical
//     observations (see observations.go, captureObservations).
//  2. eligible_starting_value_base = Σ included position values + included cash.
//  3. starting_weight per included position, index normalized to 100.
//  4. Entry becomes active (baseline completed) — or stays pending for retry,
//     or is explicitly failed once the operational window closes.
//
// Fail-closed by design: an entry that cannot be fully priced is never
// baselined partially and never silently dropped.
func (s *Service) RunCompetitionBaselines(ctx context.Context) (int, error) {
	repo, ok := s.repo.(EngineEntryRepository)
	obsRepo, ok2 := s.repo.(ObservationRepository)
	if !ok || !ok2 {
		return 0, nil
	}
	now := s.clock.Now().UTC()
	window := s.baselineWindow
	if window <= 0 {
		window = defaultBaselineWindow
	}

	editions, err := repo.ListBaselineDueEditions(ctx, now)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, comp := range editions {
		entries, err := repo.ListPendingBaselineEntries(ctx, comp.ID, defaultBaselineBatch)
		if err != nil {
			return completed, err
		}
		if len(entries) == 0 {
			continue
		}
		// One shared, persisted observation set per competition — captured
		// incrementally as new symbols/pairs are first seen across passes,
		// never re-queried for a symbol already captured. This is what makes
		// every entry's start-time price/FX identical regardless of which
		// 200-entry batch or which pass baselines it (see observations.go).
		memo, err := s.captureObservations(ctx, obsRepo, comp.ID, BoundaryStart, comp.StartsAt, entries, now)
		if err != nil {
			slog.Error("competition_baseline_observation_capture_failed", "competition_id", comp.ID, "error", err)
			continue
		}
		deadline := comp.StartsAt.Add(window)
		for _, entry := range entries {
			if err := s.baselineEntry(ctx, repo, entry, memo, now); err != nil {
				if now.After(deadline) {
					failErr := repo.FailBaseline(ctx, entry.ID, fmt.Sprintf("baseline window elapsed: %v", err))
					slog.Error("competition_baseline_failed",
						"competition_id", comp.ID, "entry_id", entry.ID, "error", err, "fail_recorded", failErr == nil)
				} else {
					slog.Warn("competition_baseline_retry",
						"competition_id", comp.ID, "entry_id", entry.ID, "error", err)
				}
				continue
			}
			completed++
		}
		slog.Info("competition_baseline_pass",
			"competition_id", comp.ID, "entries", len(entries), "completed_total", completed)
	}
	return completed, nil
}

// retryCompetitionBaseline runs the same bounded, persisted-observation
// baseline path as the scheduler, but for one explicitly selected edition.
func (s *Service) retryCompetitionBaseline(ctx context.Context, comp Competition) error {
	repo, ok := s.repo.(EngineEntryRepository)
	obsRepo, ok2 := s.repo.(ObservationRepository)
	if !ok || !ok2 {
		return fmt.Errorf("competition baseline repository is unavailable")
	}
	if comp.LifecycleStatus != LifecycleRegistrationClosed && comp.LifecycleStatus != LifecycleActive {
		return fmt.Errorf("baseline retry requires registration_closed or active lifecycle")
	}
	now := s.clock.Now().UTC()
	entries, err := repo.ListPendingBaselineEntries(ctx, comp.ID, defaultBaselineBatch)
	if err != nil || len(entries) == 0 {
		return err
	}
	memo, err := s.captureObservations(ctx, obsRepo, comp.ID, BoundaryStart, comp.StartsAt, entries, now)
	if err != nil {
		return err
	}
	var firstErr error
	for _, entry := range entries {
		if err := s.baselineEntry(ctx, repo, entry, memo, now); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// observationMemo shares one price/FX observation set across a baseline pass,
// so every entry valued in the pass sees identical observations. Floats exist
// only at the provider boundary, exactly like the legacy snapshot math.
type observationMemo struct {
	svc    *Service
	prices map[string]struct {
		price    money.Price
		currency string
	}
	fxRates map[string]money.Ratio // "FROM->TO"
}

var _ priceSource = (*observationMemo)(nil)

func newObservationMemo(s *Service) *observationMemo {
	return &observationMemo{
		svc: s,
		prices: map[string]struct {
			price    money.Price
			currency string
		}{},
		fxRates: map[string]money.Ratio{},
	}
}

func (m *observationMemo) priceOf(ctx context.Context, symbol string) (money.Price, string, error) {
	if hit, ok := m.prices[symbol]; ok {
		return hit.price, hit.currency, nil
	}
	quote, err := m.svc.prices.GetLatestPrice(ctx, symbol)
	if err != nil {
		return money.Price{}, "", err
	}
	exact := money.PriceFromFloat64(quote.Price)
	m.prices[symbol] = struct {
		price    money.Price
		currency string
	}{exact, quote.Currency}
	return exact, quote.Currency, nil
}

func (m *observationMemo) toBase(ctx context.Context, value money.Amount, currency string) (money.Amount, error) {
	if currency == fx.BaseCurrency {
		return value, nil
	}
	key := currency + "->" + fx.BaseCurrency
	rate, ok := m.fxRates[key]
	if !ok {
		// Derive the rate from a unit conversion once, then reuse it exactly.
		converted, err := m.svc.fx.Convert(ctx, 1, currency, fx.BaseCurrency)
		if err != nil {
			return money.Amount{}, err
		}
		rate = money.RatioFromFloat64(converted)
		m.fxRates[key] = rate
	}
	return value.MulRatio(rate), nil
}

// baselineEntry values one entry's included composition and persists the
// official baseline atomically. Any single unpriceable component aborts the
// whole entry (no partial baselines).
func (s *Service) baselineEntry(ctx context.Context, repo EngineEntryRepository, entry CompetitionEntry, memo priceSource, now time.Time) error {
	type valued struct {
		snapshotID string
		price      money.Price
		currency   string
		valueBase  money.Amount
	}
	var included []valued
	total := money.ZeroAmount()

	for _, snap := range entry.Snapshots {
		if !snap.IncludedInScore {
			continue
		}
		price, currency, err := memo.priceOf(ctx, snap.Symbol)
		if err != nil {
			return fmt.Errorf("price %s: %w", snap.Symbol, err)
		}
		local := snap.Quantity.MulPrice(price)
		base, err := memo.toBase(ctx, local, currency)
		if err != nil {
			return fmt.Errorf("fx %s: %w", currency, err)
		}
		included = append(included, valued{snapshotID: snap.ID, price: price, currency: currency, valueBase: base})
		total = total.Add(base)
	}
	for _, cash := range entry.CashSnapshots {
		if !cash.IncludedInScore {
			continue
		}
		base, err := memo.toBase(ctx, cash.Amount, cash.Currency)
		if err != nil {
			return fmt.Errorf("fx cash %s: %w", cash.Currency, err)
		}
		total = total.Add(base)
	}
	if total.Sign() <= 0 {
		return fmt.Errorf("baseline value is zero")
	}

	baselines := make([]PositionBaseline, 0, len(included))
	for _, v := range included {
		weight, err := v.valueBase.DivExact(total, 18)
		if err != nil {
			return fmt.Errorf("weight: %w", err)
		}
		baselines = append(baselines, PositionBaseline{
			SnapshotID: v.snapshotID, Price: v.price, PriceCurrency: v.currency,
			ValueBase: v.valueBase, Weight: weight, ObservedAt: now,
		})
	}
	return repo.CompleteBaseline(ctx, entry.ID, total, baselines, now)
}
