package portfolio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/fx"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
	"github.com/ardakimyonok/finance_app/internal/prices"
)

// MutationKind identifies the requested portfolio mutation.
type MutationKind string

const (
	MutationAdd        MutationKind = "add"
	MutationResize     MutationKind = "resize"
	MutationDelete     MutationKind = "delete"
	MutationClose      MutationKind = "close"
	MutationReplace    MutationKind = "replace"
	MutationDeposit    MutationKind = "deposit"
	MutationWithdrawal MutationKind = "withdrawal"
	MutationBuy        MutationKind = "buy"
	MutationSell       MutationKind = "sell"

	// Return-bearing mutations (change value without neutralizing the index).
	MutationIncome             MutationKind = "income"
	MutationReinvestedDividend MutationKind = "reinvested_dividend"
	MutationReturnOfCapital    MutationKind = "return_of_capital"
	MutationFee                MutationKind = "fee"
	MutationWriteOff           MutationKind = "write_off"

	// Neutral corporate-action mutations.
	MutationSplit         MutationKind = "split"
	MutationSymbolChange  MutationKind = "symbol_change"
	MutationStockDividend MutationKind = "stock_dividend"
)

// MutationRequest describes one portfolio mutation. RequestID makes the mutation
// idempotent: a retry with the same id returns the original committed result
// instead of applying the change twice.
type MutationRequest struct {
	Kind      MutationKind
	UserID    string
	RequestID string

	Input      PositionInput         // add
	PositionID string                // resize / delete / close / write_off
	Quantity   float64               // resize
	Weights    []StrategyWeightInput // replace
	CashFlow   CashFlowInput
	Buy        BuyInput
	Sell       SellInput

	// Return-bearing / corporate-action payloads. Only the one matching Kind is
	// used.
	Income     IncomeInput
	Fee        FeeInput
	CorpAction CorpActionInput
}

// MutationResult is the committed outcome of a mutation.
type MutationResult struct {
	Position          *Position
	Closed            *ClosedPositionSummary
	PortfolioVersion  int64
	RankedIndexBefore float64
	RankedIndexAfter  float64
	RankingStatus     performance.Status
	Activity          *Activity
	// Duplicate is true when an idempotent retry replayed an earlier commit.
	Duplicate bool
}

// errRebuildValuation is an internal sentinel: the locked position set contained
// a symbol the pinned valuation did not cover, so the transaction is abandoned
// and ALL inputs are rebuilt from current state. It never escapes Apply.
var errRebuildValuation = errors.New("valuation snapshot incomplete; rebuild required")

// PositionReader is a best-effort, non-transactional read used only to pre-warm
// the quote snapshot before the aggregate lock is taken. Correctness never
// depends on it: the authoritative position set is always re-read under the lock
// and a coverage check forces a rebuild if this pre-read was stale.
type PositionReader interface {
	ListPositionsByUser(ctx context.Context, userID string) ([]*Position, error)
}

// MutationCoordinator is THE portfolio write path. Every production mutation —
// add, resize, delete, close, strategy replacement — goes through Apply, which
// commits the position change, the ranked-performance checkpoint, the portfolio
// aggregate version, the audit record and the outbox event in ONE transaction.
//
// Transaction & locking strategy:
//   - Isolation: PostgreSQL default READ COMMITTED. Serializability for a single
//     portfolio comes from an explicit row lock, not from the isolation level, so
//     no application-level input is ever reused across a concurrent change.
//   - Lock: SELECT ... FROM portfolios WHERE user_id = $1 FOR UPDATE. This
//     serializes all mutations of one portfolio while leaving different
//     portfolios fully concurrent.
//   - Market data is fetched BEFORE the lock so no network call happens while the
//     row is held. After locking, the pinned snapshot is checked for coverage of
//     the (authoritative) locked position set; on a miss the transaction rolls
//     back, the missing quotes are fetched outside the lock, and the entire
//     calculation is rebuilt — never retried with stale values.
type MutationCoordinator struct {
	store    AggregateStore
	reader   PositionReader
	provider prices.PriceProvider
	fx       fx.FXProvider
	now      func() time.Time
	history  TrustedSnapshotBoundary
}

type TrustedSnapshotBoundary interface {
	LatestTrustedSnapshotAt(ctx context.Context, userID, portfolioID string, epoch time.Time) (time.Time, bool, error)
}

// NewMutationCoordinator wires the single portfolio write path.
func NewMutationCoordinator(store AggregateStore, reader PositionReader, provider prices.PriceProvider, fxp fx.FXProvider) *MutationCoordinator {
	return &MutationCoordinator{
		store:    store,
		reader:   reader,
		provider: provider,
		fx:       fxp,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// SetClock overrides the time source (tests).
func (c *MutationCoordinator) SetClock(now func() time.Time) { c.now = now }

func (c *MutationCoordinator) SetTrustedSnapshotBoundary(boundary TrustedSnapshotBoundary) {
	c.history = boundary
}

// maxMutationAttempts bounds valuation rebuilds. Each attempt rebuilds every
// input from current state; this is not a retry of a stale calculation.
const maxMutationAttempts = 3

// Apply executes a mutation inside the aggregate transaction boundary.
func (c *MutationCoordinator) Apply(ctx context.Context, req MutationRequest) (MutationResult, error) {
	// 1. Static validation, before any I/O. Strategy replacement validates every
	//    target symbol and weight here, so a bad allocation can never reach the
	//    destructive phase.
	seed, err := c.validate(&req)
	if err != nil {
		return MutationResult{}, err
	}

	// 2. Pin one market observation per symbol/currency, outside the lock.
	valuation := newValuation(c.now())
	for _, currency := range []string{"USD", "TRY", "EUR", "GBP"} {
		if err := c.observeCurrency(ctx, valuation, currency); err != nil {
			return MutationResult{}, err
		}
	}
	// Symbols the CLIENT asked for: an unpriceable one is a bad request, not an
	// upstream outage, so it maps to 400 rather than 502.
	if err := c.observe(ctx, valuation, seed); err != nil {
		if errors.Is(err, ErrPriceProvider) {
			return MutationResult{}, ErrUnsupportedSymbol
		}
		return MutationResult{}, err
	}
	// Symbols already held: a failure here is an upstream problem (502), except
	// for a write-off's own target symbol (see observeHeldPositions) — a
	// write-off exists specifically to unstick a position whose symbol cannot
	// be priced at all, so requiring a market price for it here would defeat
	// the whole point.
	if pre, err := c.reader.ListPositionsByUser(ctx, req.UserID); err == nil {
		if err := c.observeHeldPositions(ctx, valuation, req, pre); err != nil {
			return MutationResult{}, err
		}
	}

	for attempt := 0; attempt < maxMutationAttempts; attempt++ {
		var (
			result  MutationResult
			missing []string
		)
		err := c.store.WithLockedPortfolio(ctx, req.UserID, func(ctx context.Context, tx AggregateTx) error {
			res, miss, err := c.applyLocked(ctx, tx, req, valuation)
			if err != nil {
				return err
			}
			if len(miss) > 0 {
				missing = miss
				return errRebuildValuation // roll back; nothing was written
			}
			result = res
			return nil
		})
		if errors.Is(err, errRebuildValuation) {
			// Composition changed before we locked. Fetch the missing quotes
			// outside the lock and rebuild every input from current state.
			if err := c.observeMissingAfterRebuild(ctx, valuation, req, missing); err != nil {
				return MutationResult{}, err
			}
			continue
		}
		if err != nil {
			return MutationResult{}, err
		}
		return result, nil
	}
	return MutationResult{}, ErrMutationConflict
}

// observeHeldPositions prices every open position needed for the ranked-index
// before/after checkpoint. A write-off's own target symbol is exempt from
// requiring a real quote: planWriteOff realizes the position's KNOWN COST
// BASIS as a loss regardless of any market price, so the only thing missing a
// quote would break is the "value before" checkpoint calculation — which this
// pins with a synthetic, clearly-marked cost-basis observation instead of a
// fabricated market price. Every other held symbol still requires a real
// quote: this narrows the write-off exemption to exactly the mutation kind
// (and exactly the symbol) it exists for.
func (c *MutationCoordinator) observeHeldPositions(ctx context.Context, val *Valuation, req MutationRequest, positions []*Position) error {
	exemptSymbol := ""
	if req.Kind == MutationWriteOff {
		exemptSymbol = strings.ToUpper(strings.TrimSpace(req.CorpAction.Symbol))
	}

	toObserve := make([]string, 0, len(positions))
	for _, sym := range openSymbols(positions) {
		if exemptSymbol != "" && sym == exemptSymbol {
			continue
		}
		toObserve = append(toObserve, sym)
	}
	if err := c.observe(ctx, val, toObserve); err != nil {
		return err
	}
	if exemptSymbol == "" {
		return nil
	}
	if _, ok := val.Quote(exemptSymbol); ok {
		return nil // priced normally after all; no fallback needed
	}

	var exemptPosition *Position
	for _, p := range positions {
		if p.Symbol == exemptSymbol && positionStatus(p) == PositionStatusOpen {
			exemptPosition = p
			break
		}
	}
	if exemptPosition == nil {
		return nil // nothing open under that symbol; planWriteOff rejects it
	}
	rate, ok := val.rates[exemptPosition.Currency]
	if !ok || !finitePositive(rate) {
		return ErrUnsupportedCurrency
	}
	val.pinCostBasisFallback(exemptSymbol, exemptPosition.AverageBuyPrice.Float64(), exemptPosition.Currency)
	return nil
}

// observeMissingAfterRebuild re-prices symbols that became newly relevant
// after a composition change was detected inside the lock, applying the same
// write-off exemption as observeHeldPositions.
func (c *MutationCoordinator) observeMissingAfterRebuild(ctx context.Context, val *Valuation, req MutationRequest, missing []string) error {
	if req.Kind != MutationWriteOff {
		return c.observe(ctx, val, missing)
	}
	positions, err := c.reader.ListPositionsByUser(ctx, req.UserID)
	if err != nil {
		return c.observe(ctx, val, missing) // fall back to the original behavior
	}
	return c.observeHeldPositions(ctx, val, req, positions)
}

// EnsureRankedEpoch initializes ranked tracking for an existing portfolio that
// has no state yet, WITHOUT rewriting existing state. This is the deployment
// backfill: a non-empty portfolio starts a new epoch at index 100 with its
// current value as the first segment; an empty one starts paused at 100.
//
// It runs inside the SAME aggregate transaction boundary as mutations, so epoch
// initialization can never race a concurrent mutation's own initialization —
// one of them takes the lock first and the other observes the committed state.
// Idempotent: a portfolio that already has state is left untouched.
func (c *MutationCoordinator) EnsureRankedEpoch(ctx context.Context, userID string) error {
	valuation := newValuation(c.now())
	pre, err := c.reader.ListPositionsByUser(ctx, userID)
	if err != nil {
		return err
	}
	if err := c.observe(ctx, valuation, openSymbols(pre)); err != nil {
		return err
	}

	for attempt := 0; attempt < maxMutationAttempts; attempt++ {
		var missing []string
		err := c.store.WithLockedPortfolio(ctx, userID, func(ctx context.Context, tx AggregateTx) error {
			if _, found, err := tx.RankedState(ctx); err != nil {
				return err
			} else if found {
				return nil // already tracked; nothing to do
			}
			all := tx.Positions()
			if miss := valuation.Missing(openSymbols(all)); len(miss) > 0 {
				missing = miss
				return errRebuildValuation
			}
			value, hasActive, err := valuation.ValueTracked(openPositions(all), tx.CashBalances())
			if err != nil {
				return err
			}
			state := performance.ActivateState(tx.Portfolio().ID, userID, value, hasActive, valuation.ObservedAt)
			return tx.PutRankedState(ctx, state, true, 0)
		})
		if errors.Is(err, errRebuildValuation) {
			if err := c.observe(ctx, valuation, missing); err != nil {
				return err
			}
			continue
		}
		return err
	}
	return ErrMutationConflict
}

// applyLocked runs entirely inside the transaction with the portfolio row locked.
// It returns a non-empty `missing` slice when the pinned valuation does not cover
// the authoritative position set, which aborts the transaction for a rebuild.
func (c *MutationCoordinator) applyLocked(ctx context.Context, tx AggregateTx, req MutationRequest, val *Valuation) (MutationResult, []string, error) {
	// Idempotency: a duplicate request replays the committed result and applies
	// nothing. Checked inside the lock so two concurrent retries cannot both pass.
	if req.RequestID != "" {
		if audit, found, err := tx.FindAuditByRequestID(ctx, req.RequestID); err != nil {
			return MutationResult{}, nil, err
		} else if found {
			return c.replay(ctx, tx, audit), nil, nil
		}
	}

	pf := tx.Portfolio()
	all := tx.Positions()
	oldOpen := openPositions(all)
	oldCash := tx.CashBalances()

	// Coverage check against the AUTHORITATIVE locked set.
	if miss := val.Missing(openSymbols(all)); len(miss) > 0 {
		return MutationResult{}, miss, nil
	}

	// Build the post-mutation state in memory. Ownership and open status are
	// verified here — inside the transaction — never from a pre-lock read.
	plan, err := c.plan(req, pf, all, oldCash, val)
	if err != nil {
		return MutationResult{}, nil, err
	}
	if plan.newCash == nil {
		plan.newCash = oldCash
	}

	valueBefore, hadActiveBefore, err := val.ValueTracked(oldOpen, oldCash)
	if err != nil {
		return MutationResult{}, nil, err
	}
	var valueAfter float64
	var hasActiveAfter bool
	if plan.valueInvariant {
		// A pure re-expression of the same economic holding (split, symbol change).
		// Pin the after-value to the before-value so the transformed symbol's quote
		// (which may differ, be unpriced, or be split-unadjusted) cannot move ranked
		// performance. We deliberately do NOT value newOpen here: the post-action
		// symbol need not be in the pinned snapshot.
		valueAfter, hasActiveAfter = valueBefore, hadActiveBefore
	} else {
		valueAfter, hasActiveAfter, err = val.ValueTracked(plan.newOpen, plan.newCash)
		if err != nil {
			return MutationResult{}, nil, err
		}
	}

	// Ranked state: read (or initialize) through the same transaction.
	state, found, err := tx.RankedState(ctx)
	if err != nil {
		return MutationResult{}, nil, err
	}
	if !found {
		// First time this portfolio enters ranked tracking. Activating from the
		// PRE-mutation value means an existing portfolio starts its epoch at
		// exactly 100 and the mutation itself stays neutral.
		state = performance.ActivateState(pf.ID, req.UserID, valueBefore, hadActiveBefore, val.ObservedAt)
		// Seed at version 0 so the checkpoint below lands on version 1: the
		// version must advance exactly ONCE per committed mutation, including the
		// one that initializes the state. This seed is never persisted as-is.
		state.Version = 0
	}
	versionBefore := state.Version

	// Conservative historical-transaction policy, evaluated against the trusted
	// ranked boundary BEFORE any write happens (see validateHistorical).
	if err := c.validateHistorical(ctx, tx, req, all, state, found); err != nil {
		return MutationResult{}, nil, err
	}

	checkpoint := performance.CheckpointInput{
		PortfolioID:     pf.ID,
		UserID:          req.UserID,
		ValueBeforeBase: valueBefore,
		HasActiveBefore: hadActiveBefore,
		ValueAfterBase:  valueAfter,
		HasActiveAfter:  hasActiveAfter,
		At:              val.ObservedAt,
	}
	indexBefore := performance.CalculateIndexBeforeMutation(state, checkpoint)

	// The performance effect is decided by trusted domain logic from the mutation
	// kind — never by the caller. Neutral mutations re-baseline the segment so the
	// index is unchanged; return-bearing events (income, fees, write-offs)
	// preserve the segment so the value change flows into the index.
	effect := mutationEffect(req.Kind)
	if plan.performanceEffect != "" {
		effect = plan.performanceEffect
	}
	var next performance.State
	var indexAfter float64

	if plan.returnValueBase != 0 {
		// Mixed mutation: a neutral reallocation plus a return-bearing fee.
		// Stage 1 re-baselines the segment at the neutral value (index
		// unchanged); stage 2 preserves that baseline so only the fee moves the
		// index. Two chained pure transitions, one transaction, one commit.
		neutralAfter := valueAfter - plan.returnValueBase
		mid := performance.ApplyCheckpoint(state, performance.CheckpointInput{
			PortfolioID: pf.ID, UserID: req.UserID,
			ValueBeforeBase: valueBefore, HasActiveBefore: hadActiveBefore,
			ValueAfterBase: neutralAfter, HasActiveAfter: hasActiveAfter,
			At: val.ObservedAt,
		})
		if err := performance.ValidateState(mid); err != nil {
			return MutationResult{}, nil, err
		}
		indexMid := performance.CalculateCurrentIndex(mid, neutralAfter)
		if !neutral(indexBefore, indexMid) {
			return MutationResult{}, nil, ErrRankedInvariant
		}
		next = performance.ApplyReturnCheckpoint(mid, performance.CheckpointInput{
			PortfolioID: pf.ID, UserID: req.UserID,
			ValueBeforeBase: neutralAfter, HasActiveBefore: hasActiveAfter,
			ValueAfterBase: valueAfter, HasActiveAfter: hasActiveAfter,
			At: val.ObservedAt,
		})
		if err := performance.ValidateState(next); err != nil {
			return MutationResult{}, nil, err
		}
		indexAfter = performance.CalculateCurrentIndex(next, valueAfter)
		// A fee must never raise the ranked index.
		if plan.returnValueBase < 0 && indexAfter > indexMid+1e-9*math.Max(1, math.Abs(indexMid)) {
			return MutationResult{}, nil, ErrRankedInvariant
		}
	} else {
		switch effect {
		case PerformanceEffectReturn:
			next = performance.ApplyReturnCheckpoint(state, checkpoint)
			if err := performance.ValidateState(next); err != nil {
				return MutationResult{}, nil, err
			}
			indexAfter = performance.CalculateCurrentIndex(next, valueAfter)
			// Direction guard: income must not lower the index and a fee/write-off
			// must not raise it. This catches a misclassified plan before it corrupts
			// ranked history.
			if err := assertReturnDirection(req.Kind, indexBefore, indexAfter); err != nil {
				return MutationResult{}, nil, err
			}
		default:
			next = performance.ApplyCheckpoint(state, checkpoint)
			if err := performance.ValidateState(next); err != nil {
				return MutationResult{}, nil, err
			}
			indexAfter = performance.CalculateCurrentIndex(next, valueAfter)
			// Neutrality guard: a neutral mutation must never generate ranked return.
			// Violating it aborts the transaction rather than committing corrupted
			// ranked history.
			if !neutral(indexBefore, indexAfter) {
				return MutationResult{}, nil, ErrRankedInvariant
			}
		}
	}

	// --- writes, all inside this transaction ---
	if err := plan.write(ctx, tx); err != nil {
		return MutationResult{}, nil, err
	}
	if err := tx.PutRankedState(ctx, next, !found, versionBefore); err != nil {
		return MutationResult{}, nil, err
	}
	newVersion := pf.Version + 1
	if err := tx.SetPortfolioVersion(ctx, newVersion); err != nil {
		return MutationResult{}, nil, err
	}
	if plan.activity != nil {
		plan.activity.PortfolioVersion = newVersion
		prepareActivity(plan.activity)
		if err := tx.RecordActivity(ctx, *plan.activity); err != nil {
			return MutationResult{}, nil, err
		}
	}
	// Grouped legs of one economic event (e.g. the buy leg of a reinvested
	// dividend) commit in the same transaction and carry the same version.
	for i := range plan.extraActivities {
		extra := plan.extraActivities[i]
		extra.PortfolioVersion = newVersion
		prepareActivity(&extra)
		if err := tx.RecordActivity(ctx, extra); err != nil {
			return MutationResult{}, nil, err
		}
	}

	audit := MutationAudit{
		ID:                       uuid.NewString(),
		RequestID:                req.RequestID,
		PortfolioID:              pf.ID,
		UserID:                   req.UserID,
		MutationType:             string(req.Kind),
		PortfolioVersionBefore:   pf.Version,
		PortfolioVersionAfter:    newVersion,
		PerformanceVersionBefore: versionBefore,
		PerformanceVersionAfter:  next.Version,
		RankedIndexBefore:        indexBefore,
		RankedIndexAfter:         indexAfter,
		OccurredAt:               val.ObservedAt,
		ResultPositionID:         plan.resultPositionID,
	}
	if err := tx.RecordAudit(ctx, audit); err != nil {
		return MutationResult{}, nil, err
	}
	// One event per committed mutation, carrying the aggregate version. Derived
	// work (private archive snapshot, ranked lifecycle history, leaderboard
	// cache, projections) is
	// driven from this after commit.
	if err := tx.AppendOutbox(ctx, OutboxEvent{
		ID:                uuid.NewString(),
		EventType:         EventPortfolioMutated,
		AggregateType:     "portfolio",
		AggregateID:       pf.ID,
		AggregateVersion:  newVersion,
		UserID:            req.UserID,
		RankedIndex:       indexAfter,
		RankingStatus:     string(next.Status),
		TrackingStartedAt: next.TrackingStartedAt,
		ValuationAsOf:     val.ValuationAsOf,
		DataQualityStatus: val.DataQualityStatus,
		CreatedAt:         val.ObservedAt,
	}); err != nil {
		return MutationResult{}, nil, err
	}

	return MutationResult{
		Position:          plan.resultPosition,
		Closed:            plan.resultClosed,
		PortfolioVersion:  newVersion,
		RankedIndexBefore: indexBefore,
		RankedIndexAfter:  indexAfter,
		RankingStatus:     next.Status,
		Activity:          plan.activity,
	}, nil, nil
}

// quantityTolerance is the shared epsilon for quantity comparisons. Quantities
// are float64 in this codebase (a deliberate, documented limitation — decimal
// migration is out of scope), so equality is never tested exactly.
const quantityTolerance = 1e-9

// validateHistorical implements the CONSERVATIVE historical-transaction policy.
//
// It is deliberately NOT a deterministic replay engine: Alarvest does not
// recompute ranked history. The exact rules, in order:
//
//  1. A transaction with no effective_at (or one dated now/later) is current.
//  2. Every backdated trade requires an explicit execution price; a live quote
//     is never substituted for a historical fill.
//  3. A backdated sale is validated only against its selected durable position
//     episode. The episode must already be open and must contain enough units at
//     effective_at. This is quantity bookkeeping, not ranked-history replay.
//  4. The trusted boundary is the exact captured_at of the latest complete
//     canonical ranked snapshot in the current tracking epoch. Every buy or sale
//     before that instant is rejected. There is no exception for a new symbol.
//
// Nothing has been written when this runs, so a rejection leaves current state
// completely untouched.
func (c *MutationCoordinator) validateHistorical(
	ctx context.Context, tx AggregateTx, req MutationRequest, all []*Position,
	state performance.State, haveState bool,
) error {
	var effectiveAt *time.Time
	var selected *Position
	switch req.Kind {
	case MutationBuy:
		effectiveAt = req.Buy.EffectiveAt
	case MutationSell:
		effectiveAt = req.Sell.EffectiveAt
		if position, err := findSalePosition(all, req); err == nil {
			selected = position
		} else {
			return err
		}
	default:
		return nil
	}
	if effectiveAt == nil {
		return nil
	}
	at := effectiveAt.UTC()
	if !at.Before(c.now()) {
		return nil // current-dated (or clock skew); nothing historical about it
	}
	if (req.Kind == MutationBuy && req.Buy.ExecutionPrice.IsZero()) ||
		(req.Kind == MutationSell && req.Sell.ExecutionPrice.IsZero()) {
		return ErrHistoricalExecutionPriceRequired
	}

	ledger, err := tx.LedgerActivities(ctx)
	if err != nil {
		return err
	}

	// Historical quantity validation for sales.
	if req.Kind == MutationSell {
		held, opened := historicalEpisodeQuantity(ledger, selected.ID, at)
		if !opened {
			return ErrHistoricalEpisodeNotOpen
		}
		// Exact decimal comparison: no epsilon tolerance.
		if req.Sell.Quantity.Cmp(held) > 0 {
			return fmt.Errorf("%w: the position held only %s units at %s",
				ErrHistoricalQuantityInsufficient, held.String(), at.Format(time.RFC3339))
		}
	}

	// Everything at or after the trusted ranked boundary behaves normally.
	if !haveState || c.history == nil {
		return nil
	}
	boundary, found, err := c.history.LatestTrustedSnapshotAt(
		ctx, req.UserID, tx.Portfolio().ID, state.TrackingStartedAt,
	)
	if err != nil {
		return err
	}
	if !found || !at.Before(boundary) {
		return nil
	}
	return ErrHistoricalRankedConflict
}

// historicalEpisodeQuantity replays quantity effects for one durable episode,
// using exact decimal arithmetic throughout (no epsilon banding needed: a
// full sale followed by no further activity lands on exactly zero).
func historicalEpisodeQuantity(ledger []Activity, episodeID string, at time.Time) (money.Quantity, bool) {
	ordered := make([]Activity, 0, len(ledger))
	for _, activity := range ledger {
		if activity.PositionEpisodeID == episodeID && !activity.OccurredAt.After(at) {
			ordered = append(ordered, activity)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].OccurredAt.Equal(ordered[j].OccurredAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].OccurredAt.Before(ordered[j].OccurredAt)
	})
	total := money.ZeroQuantity()
	opened := false
	for _, activity := range ordered {
		if activity.Quantity == nil {
			continue
		}
		switch activity.Type {
		case ActivityBuy, ActivityOpeningBalance:
			opened = true
			total = total.Add(*activity.Quantity)
		case ActivityStockDividend, ActivityReinvestedDividend:
			total = total.Add(*activity.Quantity)
		case ActivitySell, ActivityWriteOff:
			total = total.Sub(*activity.Quantity)
		case ActivityStockSplit, ActivityReverseSplit:
			// Splits restate the running total rather than adding to it.
			total = *activity.Quantity
		}
	}
	if total.Sign() < 0 {
		total = money.ZeroQuantity()
	}
	return total, opened && total.Sign() > 0
}

// neutral reports whether two ranked indexes are equal within a relative
// floating-point tolerance. Chain-linked segment ratios cannot be compared with
// exact equality.
func neutral(before, after float64) bool {
	if !isFinite(before) || !isFinite(after) {
		return false
	}
	scale := math.Max(1, math.Abs(before))
	return math.Abs(before-after) <= 1e-9*scale
}

// isIncomeKind reports whether a return-bearing mutation adds value (income) as
// opposed to removing it (fee / write-off).
func isIncomeKind(req MutationRequest) bool {
	switch req.Kind {
	case MutationReinvestedDividend, MutationReturnOfCapital:
		return true
	case MutationIncome:
		return req.Income.Subtype != "" // all income subtypes are additive
	default:
		return false // MutationFee, MutationWriteOff are subtractive
	}
}

// assertReturnDirection guards that a return-bearing event moved the ranked index
// in the economically correct direction: income never lowers it, fees and
// write-offs never raise it. A tiny tolerance permits an exactly-zero income/fee
// (already rejected earlier) and floating-point noise.
func assertReturnDirection(kind MutationKind, before, after float64) error {
	if !isFinite(before) || !isFinite(after) {
		return ErrRankedInvariant
	}
	tol := 1e-9 * math.Max(1, math.Abs(before))
	switch kind {
	case MutationIncome, MutationReinvestedDividend, MutationReturnOfCapital:
		if after < before-tol {
			return ErrRankedInvariant
		}
	case MutationFee, MutationWriteOff:
		if after > before+tol {
			return ErrRankedInvariant
		}
	}
	return nil
}

// replay reconstructs the result of an already-committed mutation for an
// idempotent retry. No state is changed.
func (c *MutationCoordinator) replay(ctx context.Context, tx AggregateTx, audit MutationAudit) MutationResult {
	res := MutationResult{
		PortfolioVersion:  audit.PortfolioVersionAfter,
		RankedIndexBefore: audit.RankedIndexBefore,
		RankedIndexAfter:  audit.RankedIndexAfter,
		Duplicate:         true,
	}
	if state, found, err := tx.RankedState(ctx); err == nil && found {
		res.RankingStatus = state.Status
	}
	if activity, found, err := tx.FindActivityByRequestID(ctx, audit.RequestID); err == nil && found {
		res.Activity = &activity
	}
	if audit.ResultPositionID != "" {
		for _, p := range tx.Positions() {
			if p.ID == audit.ResultPositionID {
				copied := *p
				res.Position = &copied
				break
			}
		}
	}
	return res
}

// mutationPlan is the in-memory outcome of a mutation: the resulting open
// position set plus the write to perform inside the transaction.
type mutationPlan struct {
	newOpen          []*Position
	newCash          []CashBalance
	write            func(ctx context.Context, tx AggregateTx) error
	resultPosition   *Position
	resultClosed     *ClosedPositionSummary
	resultPositionID string
	activity         *Activity
	// extraActivities are additional grouped activity rows committed atomically
	// with the primary one (e.g. the buy leg of a reinvested dividend).
	extraActivities []Activity
	// valueInvariant marks a transformation whose economic value is unchanged by
	// definition (split, symbol change). The ranked checkpoint then uses
	// value_after = value_before, guaranteeing neutrality regardless of how the
	// shared market quote keys the transformed symbol.
	valueInvariant    bool
	performanceEffect PerformanceEffect
	// returnValueBase splits a mixed mutation's base-currency value change into
	// a NEUTRAL part and a RETURN-BEARING part. It is the return-bearing portion
	// (negative for a fee). When non-zero the coordinator applies two chained
	// checkpoints: a neutral one from value_before to (value_after -
	// returnValueBase), then a return-bearing one on to value_after. This makes
	// an automatically-funded purchase ranked-neutral while its fee lowers the
	// ranked index exactly once.
	returnValueBase float64
	// buyCashUsed / buyFunding are reported for previews and diagnostics.
	buyCashUsed float64
	buyFunding  float64
}

// plan validates the request against the LOCKED state and computes the
// post-mutation open set. It performs no I/O.
func (c *MutationCoordinator) plan(req MutationRequest, pf *Portfolio, all []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	oldOpen := openPositions(all)

	switch req.Kind {
	case MutationDeposit, MutationWithdrawal:
		currency := req.CashFlow.Currency
		current, found := cashAmount(oldCash, currency)
		// Exact decimal comparison: no epsilon tolerance. A withdrawal that
		// exceeds the held balance is rejected outright.
		if req.Kind == MutationWithdrawal && (!found || current.Cmp(req.CashFlow.Amount) < 0) {
			return mutationPlan{}, ErrInsufficientCash
		}
		nextAmount := current.Add(req.CashFlow.Amount)
		activityType := ActivityDeposit
		if req.Kind == MutationWithdrawal {
			nextAmount = current.Sub(req.CashFlow.Amount)
			activityType = ActivityWithdrawal
		}
		now := val.ObservedAt
		occurredAt := now
		if req.CashFlow.OccurredAt != nil {
			occurredAt = req.CashFlow.OccurredAt.UTC()
		}
		balance := CashBalance{
			PortfolioID: pf.ID, Currency: currency, Amount: nextAmount,
			CreatedAt: now, UpdatedAt: now,
		}
		if existing, ok := cashBalance(oldCash, currency); ok {
			balance.CreatedAt = existing.CreatedAt
		}
		metadata := trackingMetadata(val)
		if req.CashFlow.CorrectionOf != "" {
			metadata["correction_of_activity_id"] = req.CashFlow.CorrectionOf
			if req.CashFlow.CorrectionReason != "" {
				metadata["correction_reason"] = req.CashFlow.CorrectionReason
			}
		}
		activity := &Activity{
			ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID,
			UserID: req.UserID, Type: activityType, Currency: currency,
			GrossAmount: req.CashFlow.Amount, OccurredAt: occurredAt,
			CreatedAt: now, Metadata: metadata,
		}
		return mutationPlan{
			newOpen: oldOpen, newCash: replaceCash(oldCash, balance),
			write: func(ctx context.Context, tx AggregateTx) error {
				return tx.PutCashBalance(ctx, balance)
			},
			activity: activity,
		}, nil

	case MutationBuy:
		quote, ok := val.Quote(req.Buy.Symbol)
		if !ok {
			return mutationPlan{}, ErrPriceProvider
		}
		now := val.ObservedAt

		// Execution details with provenance. Omitted price/fee/date default to
		// the latest tracked quote / zero / now, and are labelled as estimates
		// rather than presented as confirmed broker executions.
		executionPrice := req.Buy.ExecutionPrice
		priceSource := PriceSourceUserRecorded
		if executionPrice.IsZero() {
			executionPrice = money.PriceFromFloat64(quote.Price)
			priceSource = PriceSourceProviderEstimate
		}
		if executionPrice.Sign() <= 0 {
			return mutationPlan{}, ErrInvalidBuyPrice
		}
		// validateLiveExecutionPrice still takes float64 (it also compares
		// against quote.Price, which stays float64 in this pass); .Float64()
		// is a documented boundary conversion, not further decimal math.
		if err := validateLiveExecutionPrice(req.Buy.EffectiveAt, priceSource, executionPrice.Float64(), quote.Price); err != nil {
			return mutationPlan{}, err
		}
		fee := req.Buy.Fee
		feeSource := FeeSourceUserRecorded
		if fee.IsZero() {
			feeSource = FeeSourceDefaultZero
		}
		occurred := now
		if req.Buy.EffectiveAt != nil {
			occurred = req.Buy.EffectiveAt.UTC()
		}

		// Canonical purchase contract: gross cost = quantity * execution
		// price; cost basis includes gross cost AND the buy fee. All exact
		// decimal arithmetic, no intermediate rounding.
		gross := req.Buy.Quantity.MulPrice(executionPrice)
		totalRequiredAmt := gross.Add(fee)
		available, _ := cashAmount(oldCash, quote.Currency)

		// Automatic purchase funding: funding = max(quantity*price + fee -
		// available_cash, 0), computed with exact decimal arithmetic so no
		// epsilon snapping is needed to land on exactly zero. Funding is
		// created in the INSTRUMENT'S QUOTE CURRENCY only — cash in other
		// currencies is never auto-converted.
		fundingAmt := totalRequiredAmt.Sub(available)
		if fundingAmt.Sign() < 0 {
			fundingAmt = money.ZeroAmount()
		}
		if fundingAmt.Sign() > 0 && !pf.AutoFundPurchases {
			return mutationPlan{}, ErrInsufficientCash
		}
		cashUsedAmt := totalRequiredAmt
		if available.Cmp(totalRequiredAmt) < 0 {
			cashUsedAmt = available
		}
		nextCashAmt := available.Add(fundingAmt).Sub(totalRequiredAmt)
		if nextCashAmt.Sign() < 0 {
			return mutationPlan{}, ErrInsufficientCash
		}
		funding := fundingAmt.Float64()
		cashUsed := cashUsedAmt.Float64()

		balance, _ := cashBalance(oldCash, quote.Currency)
		balance.PortfolioID = pf.ID
		balance.Currency = quote.Currency
		balance.Amount = nextCashAmt
		balance.UpdatedAt = now
		if balance.CreatedAt.IsZero() {
			balance.CreatedAt = now
		}

		// Basis convention: cost basis includes the purchase price AND the buy
		// fee, so a rebought position's basis is what the user actually paid.
		// Weighted-average across buys within one episode.
		var result *Position
		var create bool
		for _, position := range oldOpen {
			sameInstrument := req.Buy.InstrumentID != "" && position.InstrumentID == req.Buy.InstrumentID
			legacyMatch := req.Buy.InstrumentID == "" && req.Buy.IdentityQuality == "" &&
				position.InstrumentID == "" &&
				position.Symbol == req.Buy.Symbol
			unresolvedConflict := req.Buy.InstrumentID == "" && req.Buy.IdentityQuality != "" &&
				position.InstrumentID == "" && position.Symbol == req.Buy.Symbol &&
				position.AssetType == req.Buy.AssetType && position.Currency == quote.Currency
			if unresolvedConflict {
				return mutationPlan{}, ErrInstrumentIdentityUnresolvedConflict
			}
			if (sameInstrument || legacyMatch) && position.AssetType == req.Buy.AssetType &&
				position.Currency == quote.Currency {
				merged := *position
				// Fee-inclusive weighted-average basis across buys in one
				// episode: new_avg = (existing_qty*existing_avg + gross + fee)
				// / (existing_qty + new_qty), computed as exact decimal Amount
				// arithmetic then divided to the policy price precision only
				// at this posting boundary (QuantizePrice, round-half-even).
				totalQuantity := position.Quantity.Add(req.Buy.Quantity)
				existingBasis := position.Quantity.MulPrice(position.AverageBuyPrice)
				newBasis := existingBasis.Add(gross).Add(fee)
				avgPrice, err := newBasis.DivByQuantity(totalQuantity, money.ScalePrice)
				if err != nil {
					return mutationPlan{}, err
				}
				merged.AverageBuyPrice = money.QuantizePrice(avgPrice)
				merged.Quantity = money.QuantizeQuantity(totalQuantity)
				merged.UpdatedAt = now
				result = &merged
				break
			}
		}
		if result == nil {
			// No open episode for this instrument: a rebuy after a full sale
			// always starts a NEW episode (a new positions row, which is the
			// durable episode identity).
			create = true
			avgPrice, err := totalRequiredAmt.DivByQuantity(req.Buy.Quantity, money.ScalePrice)
			if err != nil {
				return mutationPlan{}, err
			}
			result = &Position{
				ID: uuid.NewString(), UserID: req.UserID, PortfolioID: pf.ID,
				Symbol: req.Buy.Symbol, InstrumentID: req.Buy.InstrumentID, AssetType: req.Buy.AssetType,
				Quantity: money.QuantizeQuantity(req.Buy.Quantity), AverageBuyPrice: money.QuantizePrice(avgPrice),
				Currency: quote.Currency, Status: PositionStatusOpen,
				CreatedAt: occurred, UpdatedAt: now,
			}
		}

		// One purchase = one activity group: [automatic funding] + buy + [fee].
		groupID := uuid.NewString()
		buyMeta := activityMeta(trackingMetadata(val), PerformanceEffectNeutral, ProvenanceUserReported, map[string]any{
			"activity_group_id":           groupID,
			"execution_price_source":      priceSource,
			"fee_source":                  feeSource,
			"effective_at":                occurred.Format(time.RFC3339Nano),
			"recorded_at":                 now.Format(time.RFC3339Nano),
			"instrument_identity_quality": req.Buy.IdentityQuality,
		})
		if fundingAmt.Sign() > 0 {
			buyMeta["automatic_funding_amount"] = funding
		}
		activity := &Activity{
			ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID,
			UserID: req.UserID, Type: ActivityBuy, Symbol: result.Symbol,
			InstrumentID: result.InstrumentID,
			AssetType:    result.AssetType, Currency: quote.Currency,
			Quantity: quantityPointer(req.Buy.Quantity), UnitPrice: pricePointer(executionPrice),
			GrossAmount: gross, OccurredAt: occurred, CreatedAt: now,
			Metadata: buyMeta, PositionEpisodeID: result.ID, GroupID: groupID,
			FeeAmount: fee, NetAmount: totalRequiredAmt,
		}

		var extraActivities []Activity
		if fundingAmt.Sign() > 0 {
			// Reuses the existing neutral deposit activity type (no new
			// Postgres-constrained type). Metadata marks it as automatic and
			// links it to the purchase group.
			extraActivities = append(extraActivities, Activity{
				ID: uuid.NewString(), PortfolioID: pf.ID, UserID: req.UserID,
				Type: ActivityDeposit, Currency: quote.Currency,
				GrossAmount: fundingAmt, OccurredAt: occurred, CreatedAt: now,
				GroupID: groupID, Origin: "system_generated",
				Metadata: activityMeta(trackingMetadata(val), PerformanceEffectNeutral, ProvenanceSystemGenerated, map[string]any{
					"activity_group_id":        groupID,
					"funding_reason":           "purchase_shortfall",
					"automatic":                true,
					"linked_purchase_group_id": groupID,
					"effective_at":             occurred.Format(time.RFC3339Nano),
					"recorded_at":              now.Format(time.RFC3339Nano),
				}),
			})
		}
		if fee.Sign() > 0 {
			extraActivities = append(extraActivities, Activity{
				ID: uuid.NewString(), PortfolioID: pf.ID, UserID: req.UserID,
				Type: ActivityBuyFee, Symbol: result.Symbol, Currency: quote.Currency,
				InstrumentID: result.InstrumentID,
				GrossAmount:  fee, OccurredAt: occurred, CreatedAt: now,
				GroupID: groupID, PositionEpisodeID: result.ID,
				Metadata: activityMeta(trackingMetadata(val), PerformanceEffectReturn, ProvenanceUserReported, map[string]any{
					"component":         "purchase_fee",
					"activity_group_id": groupID,
					"fee_source":        feeSource,
				}),
			})
		}

		// Ranked semantics: the automatic funding and the allocation of cash into
		// the instrument are NEUTRAL (the user is no richer or poorer for moving
		// their own money). Only the fee is a genuine, return-bearing loss, and
		// it must be counted exactly once. returnValueBase carries that portion.
		return mutationPlan{
			newOpen: replaceOrAppendPosition(oldOpen, result), newCash: replaceCash(oldCash, balance),
			write: func(ctx context.Context, tx AggregateTx) error {
				if err := tx.PutCashBalance(ctx, balance); err != nil {
					return err
				}
				if create {
					return tx.CreatePosition(ctx, result)
				}
				return tx.UpdatePosition(ctx, result)
			},
			resultPosition: result, resultPositionID: result.ID, activity: activity,
			extraActivities: extraActivities,
			returnValueBase: -fee.Float64() * val.rates[quote.Currency],
			buyCashUsed:     cashUsed, buyFunding: funding,
		}, nil

	case MutationSell, MutationClose:
		existing, err := findSalePosition(all, req)
		if err != nil {
			return mutationPlan{}, err
		}
		quantity := req.Sell.Quantity
		if req.Kind == MutationClose {
			quantity = existing.Quantity
		}
		// Exact decimal comparison: no epsilon tolerance for the sale-quantity
		// bound.
		if quantity.Sign() <= 0 || quantity.Cmp(existing.Quantity) > 0 {
			return mutationPlan{}, ErrInvalidSaleQuantity
		}
		quote, ok := val.Quote(existing.Symbol)
		if !ok {
			return mutationPlan{}, ErrPriceProvider
		}
		now := val.ObservedAt
		executionPrice := req.Sell.ExecutionPrice
		priceSource := PriceSourceUserRecorded
		if executionPrice.IsZero() {
			executionPrice = money.PriceFromFloat64(quote.Price)
			priceSource = PriceSourceProviderEstimate
		}
		if err := validateLiveExecutionPrice(req.Sell.EffectiveAt, priceSource, executionPrice.Float64(), quote.Price); err != nil {
			return mutationPlan{}, err
		}
		fee := req.Sell.Fee
		feeSource := FeeSourceUserRecorded
		if fee.IsZero() {
			feeSource = FeeSourceDefaultZero
		}
		// Canonical sale contract, exact decimal arithmetic throughout:
		// gross proceeds = quantity * execution price; net proceeds = gross
		// proceeds - sale fee; realized P&L = net proceeds - allocated cost
		// basis. Allocated basis uses the position's EXISTING average price,
		// so a partial sale never touches the remaining position's basis.
		proceeds := quantity.MulPrice(executionPrice)
		if fee.Cmp(proceeds) >= 0 {
			return mutationPlan{}, ErrInvalidSaleFee
		}
		netProceeds := proceeds.Sub(fee)
		allocatedBasis := quantity.MulPrice(existing.AverageBuyPrice)
		// basisRate/saleRate stay float64 (FX is out of scope for this
		// section); realizedBase/realizedPct are documented boundary
		// conversions back into decimal at the point they're persisted.
		basisRate := val.rates[existing.Currency]
		saleRate := val.rates[quote.Currency]
		realizedBaseF := netProceeds.Float64()*saleRate - allocatedBasis.Float64()*basisRate
		realizedBase := money.AmountFromFloat64(realizedBaseF)
		basisBaseF := allocatedBasis.Float64() * basisRate
		realizedPct := 0.0
		if basisBaseF > 0 {
			realizedPct = realizedBaseF / basisBaseF * 100
		}
		balance, _ := cashBalance(oldCash, quote.Currency)
		balance.PortfolioID = pf.ID
		balance.Currency = quote.Currency
		balance.Amount = balance.Amount.Add(netProceeds)
		balance.UpdatedAt = now
		if balance.CreatedAt.IsZero() {
			balance.CreatedAt = now
		}
		occurred := now
		if req.Sell.EffectiveAt != nil {
			occurred = req.Sell.EffectiveAt.UTC()
		}
		groupID := uuid.NewString()
		activity := &Activity{
			ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID,
			UserID: req.UserID, Type: ActivitySell, Symbol: existing.Symbol,
			InstrumentID: existing.InstrumentID,
			AssetType:    existing.AssetType, Currency: quote.Currency,
			Quantity: quantityPointer(quantity), UnitPrice: pricePointer(executionPrice),
			GrossAmount: proceeds, CostBasisAllocated: amountPointer(allocatedBasis),
			RealizedGainLossBase:       amountPointer(realizedBase),
			RealizedGainLossPercentage: floatPointer(realizedPct),
			OccurredAt:                 occurred, CreatedAt: now,
			Metadata: activityMeta(trackingMetadata(val), PerformanceEffectNeutral, ProvenanceUserReported, map[string]any{
				"activity_group_id":      groupID,
				"execution_price_source": priceSource,
				"fee_source":             feeSource,
				"effective_at":           occurred.Format(time.RFC3339Nano),
				"recorded_at":            now.Format(time.RFC3339Nano),
			}),
			GroupID: groupID, PositionEpisodeID: existing.ID,
			FeeAmount: fee, NetAmount: netProceeds,
		}
		var extraActivities []Activity
		if fee.Sign() > 0 {
			extraActivities = append(extraActivities, Activity{
				ID: uuid.NewString(), PortfolioID: pf.ID, UserID: req.UserID,
				Type: ActivitySellFee, Symbol: existing.Symbol, Currency: quote.Currency,
				InstrumentID: existing.InstrumentID,
				GrossAmount:  fee, OccurredAt: occurred, CreatedAt: now,
				GroupID: groupID, PositionEpisodeID: existing.ID,
				Metadata: activityMeta(trackingMetadata(val), PerformanceEffectReturn, ProvenanceUserReported, map[string]any{
					"component": "sale_fee",
				}),
			})
		}
		effect := PerformanceEffectNeutral
		if fee.Sign() > 0 || !neutral(executionPrice.Float64(), quote.Price) {
			effect = PerformanceEffectReturn
		}
		// Automatic closure detection: exact-zero-after-quantization, not a
		// float epsilon band. A full sale (sold == held) subtracts to exactly
		// zero under exact decimal arithmetic; QuantizeQuantity applies the
		// policy posting-boundary scale before the zero check.
		remainingQty := money.QuantizeQuantity(existing.Quantity.Sub(quantity))
		if !remainingQty.IsZero() {
			remaining := *existing
			remaining.Quantity = remainingQty
			remaining.UpdatedAt = now
			return mutationPlan{
				newOpen: replaceInSet(oldOpen, existing.ID, &remaining),
				newCash: replaceCash(oldCash, balance),
				write: func(ctx context.Context, tx AggregateTx) error {
					if err := tx.PutCashBalance(ctx, balance); err != nil {
						return err
					}
					return tx.UpdatePosition(ctx, &remaining)
				},
				resultPosition: &remaining, resultPositionID: remaining.ID,
				activity: activity, extraActivities: extraActivities,
				performanceEffect: effect,
			}, nil
		}
		closePriceF := executionPrice.Float64()
		closed := *existing
		closed.Status = PositionStatusClosed
		closed.ClosedAt = &occurred
		closed.ClosePrice = &closePriceF
		closed.CloseCurrency = quote.Currency
		closed.RealizedGainLossBase = round2(realizedBaseF)
		closed.RealizedGainLossPercentage = round2(realizedPct)
		closed.UpdatedAt = now
		view := ClosedPositionSummary{
			ID: closed.ID, Symbol: closed.Symbol, AssetType: closed.AssetType,
			Quantity: quantity.Float64(), BaselinePrice: closed.AverageBuyPrice.Float64(),
			BaselineCurrency: closed.Currency, ClosePrice: round2(closePriceF),
			ClosePriceCurrency: quote.Currency, ClosedAt: occurred.Format(time.RFC3339),
			RealizedGainLossBase:       round2(realizedBaseF),
			RealizedGainLossPercentage: round2(realizedPct),
			ClosedCostBasisBase:        round2(basisBaseF),
			BaseCurrency:               fx.BaseCurrency,
		}
		return mutationPlan{
			newOpen: removeFromSet(oldOpen, existing.ID),
			newCash: replaceCash(oldCash, balance),
			write: func(ctx context.Context, tx AggregateTx) error {
				if err := tx.PutCashBalance(ctx, balance); err != nil {
					return err
				}
				return tx.UpdatePosition(ctx, &closed)
			},
			resultClosed: &view, resultPositionID: closed.ID, activity: activity,
			extraActivities: extraActivities, performanceEffect: effect,
		}, nil

	case MutationAdd:
		quote, ok := val.Quote(req.Input.Symbol)
		if !ok {
			return mutationPlan{}, ErrPriceProvider
		}
		now := val.ObservedAt
		pos := &Position{
			ID:              uuid.NewString(),
			UserID:          req.UserID,
			PortfolioID:     pf.ID,
			Symbol:          req.Input.Symbol,
			AssetType:       req.Input.AssetType,
			Quantity:        req.Input.Quantity,
			AverageBuyPrice: money.PriceFromFloat64(quote.Price), // baseline locked at the observed price
			Currency:        quote.Currency,                      // quote currency of the baseline
			Status:          PositionStatusOpen,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		return mutationPlan{
			newOpen:          append(append([]*Position(nil), oldOpen...), pos),
			write:            func(ctx context.Context, tx AggregateTx) error { return tx.CreatePosition(ctx, pos) },
			resultPosition:   pos,
			resultPositionID: pos.ID,
		}, nil

	case MutationResize:
		existing, err := requireOpen(all, req.PositionID)
		if err != nil {
			return mutationPlan{}, err
		}
		updated := *existing
		// req.Quantity (MutationRequest.Quantity) stays float64: it's a
		// resize-only external API field, out of this section's scope.
		updated.Quantity = money.QuantizeQuantity(money.QuantityFromFloat64(req.Quantity))
		updated.UpdatedAt = val.ObservedAt
		return mutationPlan{
			newOpen:          replaceInSet(oldOpen, req.PositionID, &updated),
			write:            func(ctx context.Context, tx AggregateTx) error { return tx.UpdatePosition(ctx, &updated) },
			resultPosition:   &updated,
			resultPositionID: updated.ID,
		}, nil

	case MutationDelete:
		if _, err := requireOpen(all, req.PositionID); err != nil {
			return mutationPlan{}, err
		}
		id := req.PositionID
		return mutationPlan{
			newOpen: removeFromSet(oldOpen, id),
			write:   func(ctx context.Context, tx AggregateTx) error { return tx.DeletePosition(ctx, id) },
		}, nil

	case MutationReplace:
		now := val.ObservedAt
		totalValue, hasValue, err := val.ValueTracked(oldOpen, oldCash)
		if err != nil {
			return mutationPlan{}, err
		}
		if !hasValue {
			return mutationPlan{}, ErrInvalidWeights
		}
		replacements := make([]*Position, 0, len(req.Weights))
		targetCashBase := 0.0
		for _, w := range req.Weights {
			if w.AssetType == AssetTypeCash {
				targetCashBase += totalValue * (w.WeightPercentage / 100)
				continue
			}
			quote, ok := val.Quote(w.Symbol)
			if !ok {
				return mutationPlan{}, ErrPriceProvider
			}
			rate, ok := val.rates[quote.Currency]
			if !ok {
				return mutationPlan{}, ErrUnsupportedCurrency
			}
			quantity := totalValue * (w.WeightPercentage / 100) / (quote.Price * rate)
			if !finitePositive(quantity) {
				return mutationPlan{}, ErrInvalidWeights
			}
			replacements = append(replacements, &Position{
				ID:              uuid.NewString(),
				UserID:          req.UserID,
				PortfolioID:     pf.ID,
				Symbol:          w.Symbol,
				AssetType:       w.AssetType,
				Quantity:        money.QuantizeQuantity(money.QuantityFromFloat64(quantity)),
				AverageBuyPrice: money.PriceFromFloat64(quote.Price),
				Currency:        quote.Currency,
				Status:          PositionStatusOpen,
				CreatedAt:       now,
				UpdatedAt:       now,
			})
		}
		nextCash := append([]CashBalance(nil), oldCash...)
		for i := range nextCash {
			nextCash[i].Amount = money.ZeroAmount()
			nextCash[i].UpdatedAt = now
		}
		if targetCashBase > 0 {
			usd, _ := cashBalance(nextCash, fx.BaseCurrency)
			usd.PortfolioID = pf.ID
			usd.Currency = fx.BaseCurrency
			usd.Amount = money.AmountFromFloat64(targetCashBase)
			usd.UpdatedAt = now
			if usd.CreatedAt.IsZero() {
				usd.CreatedAt = now
			}
			nextCash = replaceCash(nextCash, usd)
		}
		return mutationPlan{
			newOpen: replacements, newCash: nextCash,
			write: func(ctx context.Context, tx AggregateTx) error {
				for _, balance := range nextCash {
					if err := tx.PutCashBalance(ctx, balance); err != nil {
						return err
					}
				}
				return tx.ReplaceOpenPositions(ctx, replacements)
			},
		}, nil

	case MutationIncome:
		return c.planIncome(req, pf, oldOpen, oldCash, val)

	case MutationReinvestedDividend:
		return c.planReinvestedDividend(req, pf, oldOpen, oldCash, val)

	case MutationReturnOfCapital:
		return c.planReturnOfCapital(req, pf, oldOpen, oldCash, val)

	case MutationStockDividend:
		return c.planStockDividend(req, pf, oldOpen, oldCash, val)

	case MutationFee:
		return c.planFee(req, pf, oldOpen, oldCash, val)

	case MutationWriteOff:
		return c.planWriteOff(req, pf, oldOpen, oldCash, val)

	case MutationSplit:
		return c.planSplit(req, pf, oldOpen, oldCash, val)

	case MutationSymbolChange:
		return c.planSymbolChange(req, pf, oldOpen, oldCash, val)
	}
	return mutationPlan{}, ErrUnsupportedMutation
}

// --- income / fee / corporate-action plans -----------------------------------

// planIncome adds a cash dividend, ETF distribution, or interest income as cash
// in its declared currency. It is return-bearing: the cash increase raises the
// ranked index.
func (c *MutationCoordinator) planIncome(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	currency := req.Income.Currency
	if _, ok := val.rates[currency]; !ok {
		return mutationPlan{}, ErrUnsupportedCurrency
	}
	if err := validateIncomeAmounts(req.Income); err != nil {
		return mutationPlan{}, err
	}
	net := round2(req.Income.NetCash())
	now := val.ObservedAt
	current, _ := cashAmount(oldCash, currency)
	balance := CashBalance{PortfolioID: pf.ID, Currency: currency, Amount: current.Add(money.AmountFromFloat64(net)), UpdatedAt: now, CreatedAt: now}
	if existing, ok := cashBalance(oldCash, currency); ok {
		balance.CreatedAt = existing.CreatedAt
	}
	// Symbol-specific income (dividends, distributions, coupons, etc.) must
	// reference the paying instrument's open position episode so the closed-
	// episode aggregator can find it later; account-level income (interest,
	// other-provider income with no symbol) has no episode to attach to.
	episodeID, instrumentID, assetType := "", "", ""
	if req.Income.Symbol != "" {
		if pos := findOpenBySymbol(oldOpen, req.Income.Symbol); pos != nil {
			episodeID = pos.ID
			instrumentID = pos.InstrumentID
			assetType = pos.AssetType
		}
	}
	activity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: incomeActivityType(req.Income.Subtype), Symbol: req.Income.Symbol,
		InstrumentID: instrumentID, AssetType: assetType,
		Currency: currency, GrossAmount: money.AmountFromFloat64(net), OccurredAt: occurredAt(req.Income.OccurredAt, now),
		CreatedAt: now, PositionEpisodeID: episodeID,
		Metadata: activityMeta(trackingMetadata(val), PerformanceEffectReturn, req.Income.Provenance,
			incomeMetadata(req.Income, net)),
	}
	return mutationPlan{
		newOpen: oldOpen, newCash: replaceCash(oldCash, balance),
		write:    func(ctx context.Context, tx AggregateTx) error { return tx.PutCashBalance(ctx, balance) },
		activity: activity,
	}, nil
}

// validateIncomeAmounts rejects non-finite, non-positive gross, negative
// deductions, or deductions that exceed the gross amount (which would fabricate
// a negative income).
func validateIncomeAmounts(in IncomeInput) error {
	if !finitePositive(in.Amount) {
		return ErrInvalidIncomeAmount
	}
	if !isFinite(in.Withholding) || in.Withholding < 0 || !isFinite(in.Fee) || in.Fee < 0 {
		return ErrInvalidIncomeAmount
	}
	if in.Withholding+in.Fee > in.Amount+1e-9 {
		return ErrInvalidIncomeAmount
	}
	return nil
}

// incomeMetadata builds the auditable, separated income breakdown stored on the
// immutable ledger activity: gross, withholding, fee, and net are distinct.
func incomeMetadata(in IncomeInput, net float64) map[string]any {
	m := map[string]any{
		"income_subtype": string(in.Subtype),
		"gross_amount":   round2(in.Amount),
		"net_amount":     net,
	}
	if in.Withholding > 0 {
		m["withholding_amount"] = round2(in.Withholding)
	}
	if in.Fee > 0 {
		m["income_fee_amount"] = round2(in.Fee)
	}
	if in.Estimated {
		m["estimated"] = true
	}
	if in.IncomeEventID != "" {
		m["income_event_id"] = in.IncomeEventID
	}
	if in.TaxClassification != "" {
		m["tax_classification"] = in.TaxClassification
	}
	return m
}

// planReinvestedDividend records income and immediately reinvests it into the
// paying instrument at the current tracked market price. The income component is
// return-bearing (+D); the buy component is a neutral internal allocation, so the
// net ranked effect reflects the income exactly once.
func (c *MutationCoordinator) planReinvestedDividend(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	quote, ok := val.Quote(req.Income.Symbol)
	if !ok {
		return mutationPlan{}, ErrPriceProvider
	}
	if quote.Currency != req.Income.Currency {
		// Reinvestment happens in the instrument's quote currency.
		req.Income.Currency = quote.Currency
	}
	if _, ok := val.rates[quote.Currency]; !ok {
		return mutationPlan{}, ErrUnsupportedCurrency
	}
	if err := validateIncomeAmounts(req.Income); err != nil {
		return mutationPlan{}, err
	}
	// The buy leg reinvests the NET income (after withholding/fees), so ranked
	// performance reflects the income exactly once.
	net := round2(req.Income.NetCash())
	netAmt := money.AmountFromFloat64(net)
	// Reinvestment price hierarchy: an explicit broker/DRIP execution price when
	// supplied, otherwise the current tracked market price (estimated).
	price := quote.Price
	method := "market_close_on_payment_date"
	if finitePositive(req.Income.ReinvestPrice) {
		price = req.Income.ReinvestPrice
		method = "broker_reported"
	}
	if req.Income.PriceMethod != "" {
		method = req.Income.PriceMethod
	}
	if !finitePositive(price) {
		return mutationPlan{}, ErrInvalidReinvestment
	}
	priceAmt := money.PriceFromFloat64(price)
	now := val.ObservedAt
	// Reinvest the net income at the chosen price, exact decimal division to
	// the policy quantity precision.
	qty, err := netAmt.DivByPrice(priceAmt, money.ScaleQuantity)
	if err != nil || qty.Sign() <= 0 {
		return mutationPlan{}, ErrInvalidReinvestment
	}
	qty = money.QuantizeQuantity(qty)
	estimated := req.Income.Estimated || method != "broker_reported"
	// The dividend enters and leaves cash in the same transaction; net cash is
	// unchanged, so no negative-cash check is needed.
	var result *Position
	var create bool
	for _, position := range oldOpen {
		if position.Symbol == req.Income.Symbol && position.AssetType == req.Income.AssetType && position.Currency == quote.Currency {
			merged := *position
			totalQty := position.Quantity.Add(qty)
			// Add the reinvested (net) amount to weighted-average basis, exact
			// decimal arithmetic, quantized to the policy price precision only
			// at this posting boundary.
			existingBasis := position.Quantity.MulPrice(position.AverageBuyPrice)
			newBasis := existingBasis.Add(netAmt)
			avgPrice, err := newBasis.DivByQuantity(totalQty, money.ScalePrice)
			if err != nil {
				return mutationPlan{}, err
			}
			merged.AverageBuyPrice = money.QuantizePrice(avgPrice)
			merged.Quantity = money.QuantizeQuantity(totalQty)
			merged.UpdatedAt = now
			result = &merged
			break
		}
	}
	if result == nil {
		create = true
		result = &Position{
			ID: uuid.NewString(), UserID: req.UserID, PortfolioID: pf.ID,
			Symbol: req.Income.Symbol, AssetType: req.Income.AssetType,
			Quantity: qty, AverageBuyPrice: money.QuantizePrice(priceAmt), Currency: quote.Currency,
			Status: PositionStatusOpen, CreatedAt: now, UpdatedAt: now,
		}
	}
	groupID := uuid.NewString()
	incomeMeta := incomeMetadata(req.Income, net)
	incomeMeta["income_subtype"] = string(IncomeReinvestedDiv)
	incomeMeta["activity_group_id"] = groupID
	incomeMeta["component"] = "income"
	incomeMeta["reinvestment_price_method"] = method
	incomeMeta["reinvestment_quantity"] = qty.Float64()
	incomeMeta["reinvestment_estimated"] = estimated
	incomeActivity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: ActivityReinvestedDividend, Symbol: req.Income.Symbol, AssetType: req.Income.AssetType,
		InstrumentID: result.InstrumentID,
		Currency:     quote.Currency, GrossAmount: netAmt, OccurredAt: occurredAt(req.Income.OccurredAt, now),
		CreatedAt: now, GroupID: groupID, PositionEpisodeID: result.ID,
		Metadata: activityMeta(trackingMetadata(val), PerformanceEffectReturn, req.Income.Provenance, incomeMeta),
	}
	buyLeg := Activity{
		ID: uuid.NewString(), RequestID: "", PortfolioID: pf.ID, UserID: req.UserID,
		Type: ActivityBuy, Symbol: req.Income.Symbol, AssetType: req.Income.AssetType,
		InstrumentID: result.InstrumentID,
		Currency:     quote.Currency, Quantity: quantityPointer(qty), UnitPrice: pricePointer(priceAmt),
		GrossAmount: netAmt, OccurredAt: now, CreatedAt: now,
		GroupID: groupID, PositionEpisodeID: result.ID,
		Metadata: activityMeta(trackingMetadata(val), PerformanceEffectNeutral, ProvenanceSystemGenerated, map[string]any{
			"activity_group_id":         groupID,
			"component":                 "reinvestment_buy",
			"reinvestment_price_method": method,
			"reinvestment_estimated":    estimated,
		}),
	}
	return mutationPlan{
		newOpen: replaceOrAppendPosition(oldOpen, result), newCash: oldCash,
		write: func(ctx context.Context, tx AggregateTx) error {
			if create {
				return tx.CreatePosition(ctx, result)
			}
			return tx.UpdatePosition(ctx, result)
		},
		resultPosition: result, resultPositionID: result.ID,
		activity: incomeActivity, extraActivities: []Activity{buyLeg},
	}, nil
}

// planReturnOfCapital credits the (net) distribution as cash AND reduces the
// paying position's remaining cost basis by the same amount. Unlike an ordinary
// dividend it is not counted as ordinary income for attribution; basis is never
// driven below zero — any excess over remaining basis is recorded separately.
//
// Ranked policy: the cash credit is return-bearing exactly once (its economic
// counterpart is the security's ex-date price drop). The distinguishing effect
// is the basis reduction, which changes future realized P&L, not the index.
func (c *MutationCoordinator) planReturnOfCapital(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	currency := req.Income.Currency
	if _, ok := val.rates[currency]; !ok {
		return mutationPlan{}, ErrUnsupportedCurrency
	}
	if err := validateIncomeAmounts(req.Income); err != nil {
		return mutationPlan{}, err
	}
	existing := findOpenBySymbol(oldOpen, req.Income.Symbol)
	if existing == nil {
		return mutationPlan{}, ErrInstrumentNotFound
	}
	net := round2(req.Income.NetCash())
	netAmt := money.AmountFromFloat64(net)
	now := val.ObservedAt

	// Reduce remaining basis by the net distribution, floored at zero (cost
	// basis must never go negative). Exact decimal arithmetic throughout.
	remainingBasis := existing.Quantity.MulPrice(existing.AverageBuyPrice)
	basisReduction := netAmt
	excessOverBasis := money.ZeroAmount()
	if basisReduction.Cmp(remainingBasis) > 0 {
		excessOverBasis = basisReduction.Sub(remainingBasis)
		basisReduction = remainingBasis
	}
	updated := *existing
	if existing.Quantity.Sign() > 0 {
		newBasis := remainingBasis.Sub(basisReduction)
		avgPrice, err := newBasis.DivByQuantity(existing.Quantity, money.ScalePrice)
		if err != nil {
			return mutationPlan{}, err
		}
		updated.AverageBuyPrice = money.QuantizePrice(avgPrice)
	}
	updated.UpdatedAt = now

	current, _ := cashAmount(oldCash, currency)
	balance := CashBalance{PortfolioID: pf.ID, Currency: currency, Amount: current.Add(netAmt), UpdatedAt: now, CreatedAt: now}
	if existingCash, ok := cashBalance(oldCash, currency); ok {
		balance.CreatedAt = existingCash.CreatedAt
	}

	meta := incomeMetadata(req.Income, net)
	meta["income_subtype"] = string(IncomeReturnOfCapitalSub)
	meta["basis_reduction"] = basisReduction.Float64()
	meta["excess_over_basis"] = excessOverBasis.Float64()
	meta["accounting_policy"] = "return_of_capital_basis_reduction"
	activity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: ActivityReturnOfCapital, Symbol: req.Income.Symbol, AssetType: existing.AssetType,
		InstrumentID: existing.InstrumentID,
		Currency:     currency, GrossAmount: netAmt, CostBasisAllocated: amountPointer(basisReduction),
		OccurredAt: occurredAt(req.Income.OccurredAt, now), CreatedAt: now,
		PositionEpisodeID: existing.ID,
		Metadata:          activityMeta(trackingMetadata(val), PerformanceEffectReturn, req.Income.Provenance, meta),
	}
	return mutationPlan{
		newOpen: replaceOrAppendPosition(oldOpen, &updated), newCash: replaceCash(oldCash, balance),
		write: func(ctx context.Context, tx AggregateTx) error {
			if err := tx.UpdatePosition(ctx, &updated); err != nil {
				return err
			}
			return tx.PutCashBalance(ctx, balance)
		},
		resultPosition: &updated, resultPositionID: updated.ID, activity: activity,
	}, nil
}

// planStockDividend increases quantity by (1 + num/den) and divides the
// per-share baseline by the same factor, preserving total cost basis and value.
// It creates no cash and is a NEUTRAL transformation, so it never produces a
// ranked-performance jump.
func (c *MutationCoordinator) planStockDividend(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	existing := findOpenBySymbol(oldOpen, req.Income.Symbol)
	if existing == nil {
		return mutationPlan{}, ErrInstrumentNotFound
	}
	num, den := req.Income.StockRatioNum, req.Income.StockRatioDen
	if !finitePositive(num) || !finitePositive(den) {
		return mutationPlan{}, ErrInvalidSplitRatio
	}
	factor := 1 + num/den
	if !finitePositive(factor) || factor <= 1 {
		return mutationPlan{}, ErrInvalidSplitRatio
	}
	now := val.ObservedAt
	factorRatio := money.RatioFromFloat64(factor)
	updated := *existing
	updated.Quantity = money.QuantizeQuantity(existing.Quantity.MulRatio(factorRatio))
	// Average cost can be a repeating decimal after a stock dividend. Carry
	// enough guard digits for the authoritative total cost basis to round
	// exactly at its 18dp persistence boundary; treating average cost as a
	// 12dp market quote here would introduce basis drift.
	avgPrice, err := existing.AverageBuyPrice.DivRatio(factorRatio, money.ScaleCostBasis+money.ScaleQuantity)
	if err != nil {
		return mutationPlan{}, ErrInvalidSplitRatio
	}
	updated.AverageBuyPrice = avgPrice
	updated.UpdatedAt = now
	activity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: ActivityStockDividend, Symbol: req.Income.Symbol, AssetType: existing.AssetType,
		InstrumentID: existing.InstrumentID,
		Currency:     existing.Currency, Quantity: quantityPointer(updated.Quantity.Sub(existing.Quantity)),
		OccurredAt: occurredAt(req.Income.OccurredAt, now), CreatedAt: now,
		PositionEpisodeID: existing.ID,
		Metadata: activityMeta(trackingMetadata(val), PerformanceEffectNeutral, req.Income.Provenance, map[string]any{
			"income_subtype":    string(IncomeStockDividendSub),
			"ratio_numerator":   num,
			"ratio_denominator": den,
			"income_event_id":   nonEmpty(req.Income.IncomeEventID),
		}),
	}
	return mutationPlan{
		newOpen:        replaceOrAppendPosition(oldOpen, &updated),
		newCash:        oldCash,
		write:          func(ctx context.Context, tx AggregateTx) error { return tx.UpdatePosition(ctx, &updated) },
		resultPosition: &updated, resultPositionID: updated.ID, activity: activity,
	}, nil
}

// planFee deducts a standalone management/custody/other fee from the specified
// cash balance. It is return-bearing negative and must never create negative
// cash.
func (c *MutationCoordinator) planFee(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	currency := req.Fee.Currency
	if _, ok := val.rates[currency]; !ok {
		return mutationPlan{}, ErrUnsupportedCurrency
	}
	current, found := cashAmount(oldCash, currency)
	feeAmt := money.AmountFromFloat64(req.Fee.Amount)
	// Exact decimal comparison: no epsilon tolerance for the insufficient-cash
	// check.
	if !found || current.Cmp(feeAmt) < 0 {
		return mutationPlan{}, ErrInsufficientCashForFee
	}
	now := val.ObservedAt
	balance, _ := cashBalance(oldCash, currency)
	balance.PortfolioID = pf.ID
	balance.Currency = currency
	balance.Amount = current.Sub(feeAmt)
	balance.UpdatedAt = now
	if balance.CreatedAt.IsZero() {
		balance.CreatedAt = now
	}
	activity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: feeActivityType(req.Fee.Subtype), Symbol: strings.ToUpper(strings.TrimSpace(req.Fee.Symbol)),
		Currency: currency, GrossAmount: feeAmt, OccurredAt: occurredAt(req.Fee.OccurredAt, now),
		CreatedAt: now, Metadata: activityMeta(trackingMetadata(val), PerformanceEffectReturn, req.Fee.Provenance, map[string]any{
			"fee_subtype":        string(req.Fee.Subtype),
			"linked_activity_id": nonEmpty(req.Fee.LinkedActivityID),
			"description":        nonEmpty(req.Fee.Description),
		}),
	}
	return mutationPlan{
		newOpen:  oldOpen,
		newCash:  replaceCash(oldCash, balance),
		write:    func(ctx context.Context, tx AggregateTx) error { return tx.PutCashBalance(ctx, balance) },
		activity: activity,
	}, nil
}

// planWriteOff reduces a position to zero and closes it, realizing the full basis
// as a loss. It is return-bearing negative.
func (c *MutationCoordinator) planWriteOff(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	existing := findOpenBySymbol(oldOpen, req.CorpAction.Symbol)
	if existing == nil {
		return mutationPlan{}, ErrInstrumentNotFound
	}
	now := val.ObservedAt
	basisRate := val.rates[existing.Currency]
	allocatedBasis := existing.Quantity.MulPrice(existing.AverageBuyPrice)
	realizedBaseF := -allocatedBasis.Float64() * basisRate // full loss
	realizedBase := money.AmountFromFloat64(realizedBaseF)
	basisBaseF := allocatedBasis.Float64() * basisRate
	closed := *existing
	zero := 0.0
	closed.Status = PositionStatusClosed
	closed.ClosedAt = &now
	closed.ClosePrice = &zero
	closed.CloseCurrency = existing.Currency
	closed.RealizedGainLossBase = round2(realizedBaseF)
	closed.RealizedGainLossPercentage = -100
	closed.UpdatedAt = now
	view := ClosedPositionSummary{
		ID: closed.ID, Symbol: closed.Symbol, AssetType: closed.AssetType, Quantity: existing.Quantity.Float64(),
		BaselinePrice: closed.AverageBuyPrice.Float64(), BaselineCurrency: closed.Currency, ClosePrice: 0,
		ClosePriceCurrency: existing.Currency, ClosedAt: now.Format(time.RFC3339),
		RealizedGainLossBase: round2(realizedBaseF), RealizedGainLossPercentage: -100,
		ClosedCostBasisBase: round2(basisBaseF), BaseCurrency: fx.BaseCurrency,
	}
	activity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: ActivityWriteOff, Symbol: existing.Symbol, AssetType: existing.AssetType, Currency: existing.Currency,
		InstrumentID: existing.InstrumentID,
		Quantity:     quantityPointer(existing.Quantity), GrossAmount: money.ZeroAmount(),
		CostBasisAllocated: amountPointer(allocatedBasis), RealizedGainLossBase: amountPointer(realizedBase),
		RealizedGainLossPercentage: floatPointer(-100), OccurredAt: occurredAt(req.CorpAction.OccurredAt, now),
		CreatedAt: now, PositionEpisodeID: existing.ID,
		Metadata: activityMeta(trackingMetadata(val), PerformanceEffectReturn, req.CorpAction.Provenance, map[string]any{
			"corporate_action": string(CorpWriteOff),
		}),
	}
	return mutationPlan{
		newOpen: removeFromSet(oldOpen, existing.ID), newCash: oldCash,
		write:        func(ctx context.Context, tx AggregateTx) error { return tx.UpdatePosition(ctx, &closed) },
		resultClosed: &view, resultPositionID: closed.ID, activity: activity,
	}, nil
}

// planSplit multiplies quantity by the ratio and divides the per-share baseline
// by it, leaving total basis and economic value unchanged (value-invariant).
func (c *MutationCoordinator) planSplit(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	ratio := req.CorpAction.RatioNumerator / req.CorpAction.RatioDenominator
	if !finitePositive(ratio) {
		return mutationPlan{}, ErrInvalidSplitRatio
	}
	existing := findOpenBySymbol(oldOpen, req.CorpAction.Symbol)
	if existing == nil {
		return mutationPlan{}, ErrInstrumentNotFound
	}
	now := val.ObservedAt
	ratioRatio := money.RatioFromFloat64(ratio)
	updated := *existing
	updated.Quantity = money.QuantizeQuantity(existing.Quantity.MulRatio(ratioRatio))
	// Splits have the same repeating-decimal issue as stock dividends. Preserve
	// guard digits so total cost basis remains exact at its 18dp boundary.
	avgPrice, divErr := existing.AverageBuyPrice.DivRatio(ratioRatio, money.ScaleCostBasis+money.ScaleQuantity)
	if divErr != nil {
		return mutationPlan{}, ErrInvalidSplitRatio
	}
	updated.AverageBuyPrice = avgPrice
	updated.UpdatedAt = now
	if updated.Quantity.Sign() <= 0 || updated.AverageBuyPrice.Sign() <= 0 {
		return mutationPlan{}, ErrInvalidSplitRatio
	}
	subtype := CorpStockSplit
	if ratio < 1 {
		subtype = CorpReverseSplit
	}
	activity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: splitActivityType(subtype), Symbol: existing.Symbol, AssetType: existing.AssetType,
		InstrumentID: existing.InstrumentID,
		Currency:     existing.Currency, Quantity: quantityPointer(updated.Quantity), OccurredAt: occurredAt(req.CorpAction.OccurredAt, now),
		CreatedAt: now, PositionEpisodeID: existing.ID,
		Metadata: activityMeta(trackingMetadata(val), PerformanceEffectNeutral, req.CorpAction.Provenance, map[string]any{
			"corporate_action":  string(subtype),
			"ratio_numerator":   req.CorpAction.RatioNumerator,
			"ratio_denominator": req.CorpAction.RatioDenominator,
		}),
	}
	return mutationPlan{
		newOpen: replaceInSet(oldOpen, existing.ID, &updated), newCash: oldCash,
		write:          func(ctx context.Context, tx AggregateTx) error { return tx.UpdatePosition(ctx, &updated) },
		resultPosition: &updated, resultPositionID: updated.ID, activity: activity, valueInvariant: true,
	}, nil
}

// planSymbolChange re-tickers a position, preserving quantity, basis, and ranked
// performance. The old symbol is retained in the immutable activity history.
func (c *MutationCoordinator) planSymbolChange(req MutationRequest, pf *Portfolio, oldOpen []*Position, oldCash []CashBalance, val *Valuation) (mutationPlan, error) {
	existing := findOpenBySymbol(oldOpen, req.CorpAction.Symbol)
	if existing == nil {
		return mutationPlan{}, ErrInstrumentNotFound
	}
	if other := findOpenBySymbol(oldOpen, req.CorpAction.NewSymbol); other != nil && other.ID != existing.ID {
		return mutationPlan{}, ErrSymbolChangeConflict
	}
	now := val.ObservedAt
	updated := *existing
	updated.Symbol = req.CorpAction.NewSymbol
	updated.UpdatedAt = now
	activity := &Activity{
		ID: uuid.NewString(), RequestID: req.RequestID, PortfolioID: pf.ID, UserID: req.UserID,
		Type: ActivitySymbolChange, Symbol: req.CorpAction.NewSymbol, AssetType: existing.AssetType,
		InstrumentID: existing.InstrumentID,
		Currency:     existing.Currency, OccurredAt: occurredAt(req.CorpAction.OccurredAt, now),
		CreatedAt: now, PositionEpisodeID: existing.ID,
		Metadata: activityMeta(trackingMetadata(val), PerformanceEffectNeutral, req.CorpAction.Provenance, map[string]any{
			"corporate_action": string(CorpSymbolChange),
			"old_symbol":       existing.Symbol,
			"new_symbol":       req.CorpAction.NewSymbol,
		}),
	}
	return mutationPlan{
		newOpen: replaceInSet(oldOpen, existing.ID, &updated), newCash: oldCash,
		write:          func(ctx context.Context, tx AggregateTx) error { return tx.UpdatePosition(ctx, &updated) },
		resultPosition: &updated, resultPositionID: updated.ID, activity: activity, valueInvariant: true,
	}, nil
}

// --- small helpers for the new activity plans --------------------------------

func occurredAt(supplied *time.Time, fallback time.Time) time.Time {
	if supplied != nil {
		return supplied.UTC()
	}
	return fallback
}

func nonEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func findOpenBySymbol(open []*Position, symbol string) *Position {
	for _, p := range open {
		if p.Symbol == symbol {
			return p
		}
	}
	return nil
}

// incomeSymbolOptional reports whether an income subtype may be recorded against
// a cash balance without naming a paying instrument (interest and generic
// provider income). Issuer/fund distributions always require a symbol.
func incomeSymbolOptional(sub IncomeSubtype) bool {
	switch sub {
	case IncomeInterest, IncomeCashInterest, IncomeOtherProvider:
		return true
	default:
		return false
	}
}

func incomeActivityType(sub IncomeSubtype) ActivityType {
	switch sub {
	case IncomeETFDistribution, IncomeMutualFundDist:
		return ActivityETFDistribution
	case IncomeCapitalGainsDist:
		return ActivityCapitalGainsDistribution
	case IncomeBondCoupon, IncomeFixedIncomeInt:
		return ActivityBondCoupon
	case IncomeCashInterest:
		return ActivityCashInterest
	case IncomeInterest:
		return ActivityInterestIncome
	case IncomeStakingReward:
		return ActivityStakingReward
	case IncomeOtherProvider, IncomePaymentInLieu:
		return ActivityOtherIncome
	default:
		return ActivityCashDividend
	}
}

func feeActivityType(sub FeeSubtype) ActivityType {
	switch sub {
	case FeeCustody:
		return ActivityCustodyFee
	case FeeOther:
		return ActivityOtherFee
	default:
		return ActivityManagementFee
	}
}

func splitActivityType(sub CorpActionSubtype) ActivityType {
	if sub == CorpReverseSplit {
		return ActivityReverseSplit
	}
	return ActivityStockSplit
}

// requireOpen finds a position within the locked aggregate and asserts it is
// open. A position belonging to another user is simply absent from the locked
// set, so this collapses to ErrPositionNotFound and never discloses existence.
func requireOpen(all []*Position, positionID string) (*Position, error) {
	for _, p := range all {
		if p.ID != positionID {
			continue
		}
		if positionStatus(p) != PositionStatusOpen {
			return nil, ErrPositionClosed
		}
		return p, nil
	}
	return nil, ErrPositionNotFound
}

// validate performs all static, I/O-free validation and returns the symbols the
// mutation itself introduces (which must be priced before the lock).
func (c *MutationCoordinator) validate(req *MutationRequest) ([]string, error) {
	switch req.Kind {
	case MutationDeposit, MutationWithdrawal:
		req.CashFlow.Currency = strings.ToUpper(strings.TrimSpace(req.CashFlow.Currency))
		if !supportedCurrency(req.CashFlow.Currency) {
			return nil, ErrUnsupportedCurrency
		}
		// Decimal amounts cannot be NaN/Inf by construction; only the sign needs
		// checking (exact, no epsilon).
		if req.CashFlow.Amount.Sign() <= 0 {
			return nil, ErrInvalidCashAmount
		}
		return nil, nil
	case MutationBuy:
		clean, err := validateAndNormalize(PositionInput{
			Symbol: req.Buy.Symbol, AssetType: req.Buy.AssetType,
			Quantity: req.Buy.Quantity,
		})
		if err != nil {
			return nil, err
		}
		if !req.Buy.ExecutionPrice.IsZero() && req.Buy.ExecutionPrice.Sign() <= 0 {
			return nil, ErrInvalidBuyPrice
		}
		if req.Buy.EffectiveAt != nil && req.Buy.EffectiveAt.UTC().Before(c.now()) &&
			req.Buy.ExecutionPrice.IsZero() {
			return nil, ErrHistoricalExecutionPriceRequired
		}
		if req.Buy.Fee.Sign() < 0 {
			return nil, ErrInvalidBuyFee
		}
		req.Buy = BuyInput{
			Symbol: clean.Symbol, AssetType: clean.AssetType, Quantity: clean.Quantity,
			ExecutionPrice: req.Buy.ExecutionPrice, Fee: req.Buy.Fee,
			EffectiveAt: req.Buy.EffectiveAt, InstrumentID: req.Buy.InstrumentID,
			ExchangeCode: req.Buy.ExchangeCode, MIC: req.Buy.MIC,
			IdentityQuality: req.Buy.IdentityQuality,
		}
		return []string{clean.Symbol}, nil
	case MutationSell:
		if req.Sell.Quantity.Sign() <= 0 {
			return nil, ErrInvalidSaleQuantity
		}
		if !req.Sell.ExecutionPrice.IsZero() && req.Sell.ExecutionPrice.Sign() <= 0 {
			return nil, ErrInvalidSalePrice
		}
		if req.Sell.EffectiveAt != nil && req.Sell.EffectiveAt.UTC().Before(c.now()) &&
			req.Sell.ExecutionPrice.IsZero() {
			return nil, ErrHistoricalExecutionPriceRequired
		}
		if req.Sell.Fee.Sign() < 0 {
			return nil, ErrInvalidSaleFee
		}
		req.Sell.Symbol = strings.ToUpper(strings.TrimSpace(req.Sell.Symbol))
		req.Sell.PositionID = strings.TrimSpace(req.Sell.PositionID)
		if req.Sell.Symbol == "" && req.Sell.PositionID == "" {
			return nil, ErrPositionNotFound
		}
		if req.Sell.Symbol != "" {
			symbol, err := prices.ValidateAndNormalizeSymbol(req.Sell.Symbol)
			if err != nil {
				return nil, ErrUnsupportedSymbol
			}
			req.Sell.Symbol = symbol
			return []string{symbol}, nil
		}
		return nil, nil
	case MutationAdd:
		clean, err := validateAndNormalize(req.Input)
		if err != nil {
			return nil, err
		}
		req.Input = clean
		return []string{clean.Symbol}, nil

	case MutationResize:
		if !finitePositive(req.Quantity) {
			return nil, ErrInvalidQuantity
		}
		return nil, nil

	case MutationDelete, MutationClose:
		if strings.TrimSpace(req.PositionID) == "" {
			return nil, ErrPositionNotFound
		}
		return nil, nil

	case MutationReplace:
		clean, symbols, err := normalizeStrategyWeights(req.Weights)
		if err != nil {
			return nil, err
		}
		req.Weights = clean
		return symbols, nil

	case MutationIncome:
		req.Income.Currency = strings.ToUpper(strings.TrimSpace(req.Income.Currency))
		if !supportedCurrency(req.Income.Currency) {
			return nil, ErrUnsupportedCurrency
		}
		if !finitePositive(req.Income.Amount) {
			return nil, ErrInvalidIncomeAmount
		}
		switch req.Income.Subtype {
		case IncomeCashDividend, IncomeSpecialDividend, IncomeETFDistribution,
			IncomeMutualFundDist, IncomeCapitalGainsDist, IncomeBondCoupon,
			IncomeFixedIncomeInt, IncomeInterest, IncomeCashInterest,
			IncomeStakingReward, IncomePaymentInLieu, IncomeOtherProvider:
		default:
			return nil, ErrUnsupportedIncomeType
		}
		if s := strings.ToUpper(strings.TrimSpace(req.Income.Symbol)); s != "" {
			sym, err := prices.ValidateAndNormalizeSymbol(s)
			if err != nil {
				return nil, ErrUnsupportedSymbol
			}
			req.Income.Symbol = sym
		} else if !incomeSymbolOptional(req.Income.Subtype) {
			// Dividends/distributions must name the paying instrument; interest and
			// other-provider income may be recorded against cash without a symbol.
			return nil, ErrSymbolRequired
		}
		return nil, nil // cash added directly; nothing new to price

	case MutationReinvestedDividend:
		req.Income.Currency = strings.ToUpper(strings.TrimSpace(req.Income.Currency))
		if !supportedCurrency(req.Income.Currency) {
			return nil, ErrUnsupportedCurrency
		}
		if !finitePositive(req.Income.Amount) {
			return nil, ErrInvalidIncomeAmount
		}
		clean, err := validateAndNormalize(PositionInput{
			Symbol: req.Income.Symbol, AssetType: req.Income.AssetType, Quantity: money.QuantityFromFloat64(1),
		})
		if err != nil {
			return nil, err
		}
		req.Income.Symbol = clean.Symbol
		req.Income.AssetType = clean.AssetType
		return []string{clean.Symbol}, nil // must price the symbol to reinvest

	case MutationReturnOfCapital:
		req.Income.Currency = strings.ToUpper(strings.TrimSpace(req.Income.Currency))
		if !supportedCurrency(req.Income.Currency) {
			return nil, ErrUnsupportedCurrency
		}
		if !finitePositive(req.Income.Amount) {
			return nil, ErrInvalidIncomeAmount
		}
		sym, err := prices.ValidateAndNormalizeSymbol(strings.ToUpper(strings.TrimSpace(req.Income.Symbol)))
		if err != nil {
			return nil, ErrUnsupportedSymbol
		}
		req.Income.Symbol = sym
		return nil, nil // the paying position is already held; only basis + cash change

	case MutationStockDividend:
		if !finitePositive(req.Income.StockRatioNum) || !finitePositive(req.Income.StockRatioDen) {
			return nil, ErrInvalidSplitRatio
		}
		sym, err := prices.ValidateAndNormalizeSymbol(strings.ToUpper(strings.TrimSpace(req.Income.Symbol)))
		if err != nil {
			return nil, ErrUnsupportedSymbol
		}
		req.Income.Symbol = sym
		return nil, nil // quantity transformation on a held position; nothing new to price

	case MutationFee:
		req.Fee.Currency = strings.ToUpper(strings.TrimSpace(req.Fee.Currency))
		if !supportedCurrency(req.Fee.Currency) {
			return nil, ErrUnsupportedCurrency
		}
		if !finitePositive(req.Fee.Amount) {
			return nil, ErrInvalidFeeAmount
		}
		switch req.Fee.Subtype {
		case FeeManagement, FeeCustody, FeeOther:
		default:
			return nil, ErrUnsupportedFeeType
		}
		return nil, nil

	case MutationWriteOff:
		sym, err := prices.ValidateAndNormalizeSymbol(strings.ToUpper(strings.TrimSpace(req.CorpAction.Symbol)))
		if err != nil {
			return nil, ErrUnsupportedSymbol
		}
		req.CorpAction.Symbol = sym
		return nil, nil // the position is already held and priced

	case MutationSplit:
		if !finitePositive(req.CorpAction.RatioNumerator) || !finitePositive(req.CorpAction.RatioDenominator) {
			return nil, ErrInvalidSplitRatio
		}
		sym, err := prices.ValidateAndNormalizeSymbol(strings.ToUpper(strings.TrimSpace(req.CorpAction.Symbol)))
		if err != nil {
			return nil, ErrUnsupportedSymbol
		}
		req.CorpAction.Symbol = sym
		return nil, nil

	case MutationSymbolChange:
		src, err := prices.ValidateAndNormalizeSymbol(strings.ToUpper(strings.TrimSpace(req.CorpAction.Symbol)))
		if err != nil {
			return nil, ErrUnsupportedSymbol
		}
		dst, err := prices.ValidateAndNormalizeSymbol(strings.ToUpper(strings.TrimSpace(req.CorpAction.NewSymbol)))
		if err != nil {
			return nil, ErrUnsupportedSymbol
		}
		if src == dst {
			return nil, ErrInvalidCorporateAction
		}
		req.CorpAction.Symbol = src
		req.CorpAction.NewSymbol = dst
		return nil, nil
	}
	return nil, ErrUnsupportedMutation
}

func supportedCurrency(currency string) bool {
	switch currency {
	case "USD", "TRY", "EUR", "GBP":
		return true
	default:
		return false
	}
}

func cashAmount(balances []CashBalance, currency string) (money.Amount, bool) {
	balance, ok := cashBalance(balances, currency)
	return balance.Amount, ok
}

func cashBalance(balances []CashBalance, currency string) (CashBalance, bool) {
	for _, balance := range balances {
		if balance.Currency == currency {
			return balance, true
		}
	}
	return CashBalance{}, false
}

func replaceCash(balances []CashBalance, next CashBalance) []CashBalance {
	out := append([]CashBalance(nil), balances...)
	for i := range out {
		if out[i].Currency == next.Currency {
			out[i] = next
			return out
		}
	}
	return append(out, next)
}

func replaceOrAppendPosition(positions []*Position, next *Position) []*Position {
	for _, position := range positions {
		if position.ID == next.ID {
			return replaceInSet(positions, next.ID, next)
		}
	}
	return append(append([]*Position(nil), positions...), next)
}

func findSalePosition(all []*Position, req MutationRequest) (*Position, error) {
	if req.Sell.PositionID != "" || req.PositionID != "" {
		id := firstNonEmpty(req.Sell.PositionID, req.PositionID)
		return requireOpen(all, id)
	}
	var found *Position
	for _, position := range all {
		if positionStatus(position) != PositionStatusOpen || position.Symbol != req.Sell.Symbol {
			continue
		}
		if found != nil {
			return nil, ErrMutationConflict
		}
		found = position
	}
	if found == nil {
		return nil, ErrPositionNotFound
	}
	return found, nil
}

func floatPointer(value float64) *float64 { return &value }

func quantityPointer(value money.Quantity) *money.Quantity { return &value }
func pricePointer(value money.Price) *money.Price          { return &value }
func amountPointer(value money.Amount) *money.Amount       { return &value }

func prepareActivity(activity *Activity) {
	if activity.Metadata == nil {
		activity.Metadata = map[string]any{}
	}
	if activity.Origin == "" {
		activity.Origin = activityOrigin(*activity)
	}
	if activity.Status == "" {
		activity.Status = "completed"
	}
	if activity.GroupID == "" {
		activity.GroupID, _ = activity.Metadata["activity_group_id"].(string)
	}
	if activity.PositionEpisodeID == "" {
		activity.PositionEpisodeID, _ = activity.Metadata["position_episode_id"].(string)
	}
	activity.Metadata["origin"] = activity.Origin
	activity.Metadata["status"] = activity.Status
	if activity.GroupID != "" {
		activity.Metadata["activity_group_id"] = activity.GroupID
	}
	if activity.PositionEpisodeID != "" {
		activity.Metadata["position_episode_id"] = activity.PositionEpisodeID
	}
	if activity.FeeAmount.Sign() > 0 {
		activity.Metadata["fee_amount"] = activity.FeeAmount.String()
	}
	if !activity.NetAmount.IsZero() {
		activity.Metadata["net_amount"] = activity.NetAmount.String()
	}
}

func trackingMetadata(val *Valuation) map[string]any {
	return map[string]any{
		"tracking_only":       true,
		"settlement":          "immediate",
		"valuation_as_of":     val.ValuationAsOf.Format(time.RFC3339Nano),
		"data_quality_status": val.DataQualityStatus,
	}
}

// normalizeStrategyWeights validates the complete target allocation before any
// destructive change: unique priceable symbols, valid asset types, positive
// weights totalling 100.
func normalizeStrategyWeights(weights []StrategyWeightInput) ([]StrategyWeightInput, []string, error) {
	if len(weights) == 0 {
		return nil, nil, ErrInvalidWeights
	}
	seen := map[string]bool{}
	symbols := make([]string, 0, len(weights))
	out := make([]StrategyWeightInput, 0, len(weights))
	var total float64
	for _, item := range weights {
		assetType := strings.ToLower(strings.TrimSpace(item.AssetType))
		symbol := strings.ToUpper(strings.TrimSpace(item.Symbol))
		if assetType == AssetTypeCash && symbol == "CASH" {
			if seen[symbol] || !finitePositive(item.WeightPercentage) {
				return nil, nil, ErrInvalidWeights
			}
			seen[symbol] = true
			total += item.WeightPercentage
			out = append(out, StrategyWeightInput{
				Symbol: symbol, AssetType: assetType, WeightPercentage: item.WeightPercentage,
			})
			continue
		}
		var err error
		symbol, err = prices.ValidateAndNormalizeSymbol(symbol)
		if err != nil {
			return nil, nil, ErrUnsupportedSymbol
		}
		if seen[symbol] {
			return nil, nil, ErrInvalidWeights
		}
		seen[symbol] = true
		if !validAssetTypes[assetType] {
			return nil, nil, ErrInvalidAssetType
		}
		if !finitePositive(item.WeightPercentage) {
			return nil, nil, ErrInvalidWeights
		}
		total += item.WeightPercentage
		symbols = append(symbols, symbol)
		out = append(out, StrategyWeightInput{Symbol: symbol, AssetType: assetType, WeightPercentage: item.WeightPercentage})
	}
	if math.Abs(total-100) > 0.5 {
		return nil, nil, ErrInvalidWeights
	}
	return out, symbols, nil
}
