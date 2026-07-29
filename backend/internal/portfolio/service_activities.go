package portfolio

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/telemetry"
)

// SymbolHolder identifies a portfolio holding a symbol and when it was acquired
// (earliest open-position creation). The automatic corporate-action pipeline
// uses it to discover affected portfolios and check effective-date eligibility.
type SymbolHolder struct {
	UserID      string
	PortfolioID string
	AssetType   string
	AcquiredAt  time.Time
	// Symbol is the position's actual stored symbol, which may differ from
	// whatever symbol/instrument a caller queried by (see HoldersOfInstrument).
	Symbol string
}

type IncomeDiscoveryInstrument struct {
	InstrumentID string
	Symbol       string
	AssetType    string
}

// ActiveInstrument is a distinct (instrument_id, symbol) pair currently held
// in an open, identity-resolved position. See Service.ActiveInstruments.
type ActiveInstrument struct {
	InstrumentID string
	Symbol       string
}

// Reconciliation confidence/status values (mirrors migration 0037's CHECK
// constraints on identity_reconciliation_queue).
const (
	ReconciliationConfidenceHigh   = "high"
	ReconciliationConfidenceMedium = "medium"
	ReconciliationConfidenceLow    = "low"

	ReconciliationStatusPending  = "pending"
	ReconciliationStatusResolved = "resolved"
	ReconciliationStatusRejected = "rejected"
)

// ReconciliationItem is a legacy position or activity row the backfill job
// could not resolve to a stable instrument identity unambiguously — either
// because more than one local instrument shares the ticker, or none does.
// It is never guessed at; an administrator reviews and assigns (or rejects)
// it via the identity-reconciliation endpoints.
type ReconciliationItem struct {
	ID                    string
	TableName             string // "positions" | "portfolio_activities"
	RecordID              string
	Symbol                string
	CandidateInstrumentID string // best guess, if any; empty when zero candidates
	Evidence              string
	Confidence            string
	Status                string
	CreatedAt             time.Time
	ResolvedAt            *time.Time
	ResolvedBy            string
}

func (s *Service) IncomeDiscoveryInstruments(ctx context.Context, since time.Time) ([]IncomeDiscoveryInstrument, error) {
	return s.repo.ListIncomeDiscoveryInstruments(ctx, since)
}

func (s *Service) IncomeHistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]SymbolHolder, error) {
	return s.repo.ListIncomeHistoricalHolders(ctx, instrumentID, symbol)
}

// ActiveSymbols returns every currently-held symbol across all portfolios.
func (s *Service) ActiveSymbols(ctx context.Context) ([]string, error) {
	return s.repo.ListActiveSymbols(ctx)
}

// ActiveInstruments returns every currently-held (instrument_id, symbol) pair
// that has a resolved identity, so the corporate-action pipeline can fetch
// provider events by stable identity for the share of holdings that have one.
func (s *Service) ActiveInstruments(ctx context.Context) ([]ActiveInstrument, error) {
	return s.repo.ListActiveInstruments(ctx)
}

// HoldersOfSymbol returns the portfolios that currently hold symbol, each with
// its earliest acquisition time.
func (s *Service) HoldersOfSymbol(ctx context.Context, symbol string) ([]SymbolHolder, error) {
	positions, err := s.repo.ListOpenPositionsBySymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	byPortfolio := map[string]SymbolHolder{}
	for _, p := range positions {
		h, ok := byPortfolio[p.PortfolioID]
		if !ok || p.CreatedAt.Before(h.AcquiredAt) {
			byPortfolio[p.PortfolioID] = SymbolHolder{
				UserID: p.UserID, PortfolioID: p.PortfolioID, AssetType: p.AssetType, AcquiredAt: p.CreatedAt,
				Symbol: p.Symbol,
			}
		}
	}
	out := make([]SymbolHolder, 0, len(byPortfolio))
	for _, h := range byPortfolio {
		out = append(out, h)
	}
	return out, nil
}

// HoldersOfInstrument is the identity-based counterpart of HoldersOfSymbol,
// used when a corporate-action event resolved to a stable instrument
// identity rather than being matched only by ticker.
func (s *Service) HoldersOfInstrument(ctx context.Context, instrumentID string) ([]SymbolHolder, error) {
	positions, err := s.repo.ListOpenPositionsByInstrumentID(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	byPortfolio := map[string]SymbolHolder{}
	for _, p := range positions {
		h, ok := byPortfolio[p.PortfolioID]
		if !ok || p.CreatedAt.Before(h.AcquiredAt) {
			byPortfolio[p.PortfolioID] = SymbolHolder{
				UserID: p.UserID, PortfolioID: p.PortfolioID, AssetType: p.AssetType, AcquiredAt: p.CreatedAt,
				Symbol: p.Symbol,
			}
		}
	}
	out := make([]SymbolHolder, 0, len(byPortfolio))
	for _, h := range byPortfolio {
		out = append(out, h)
	}
	return out, nil
}

// RecordIncome applies a normalized income event to a portfolio. It is invoked
// by the automatic income pipeline (or a restricted correction path), never by
// an arbitrary user request. The performance effect and mutation kind are
// derived from the trusted subtype — the caller cannot select them. Everything
// commits atomically through the coordinator.
func (s *Service) RecordIncome(ctx context.Context, userID, requestID string, input IncomeInput) (MutationResult, error) {
	kind := MutationIncome
	switch input.Subtype {
	case IncomeReinvestedDiv:
		kind = MutationReinvestedDividend
	case IncomeReturnOfCapitalSub:
		kind = MutationReturnOfCapital
	case IncomeStockDividendSub:
		kind = MutationStockDividend
	}
	return s.Mutate(ctx, MutationRequest{
		Kind: kind, UserID: userID, RequestID: requestID, Income: input,
	})
}

// ApplyIncomeEvent applies EVERY component of one economic income event
// (ordinary income, ETF/fund distribution, capital-gains distribution, return
// of capital, stock dividend, ...) atomically: one locked transaction, one
// activity_group_id shared by every leg, one combined cash/basis effect, and
// exactly one ranked-performance update for the whole event. RequestID should
// be a single deterministic key per (income event ID, portfolio ID) pair — a
// retry with the same RequestID replays the already-committed result instead
// of applying anything twice. If any component cannot be safely applied
// atomically, the WHOLE event is rejected and nothing is written.
func (s *Service) ApplyIncomeEvent(ctx context.Context, userID, requestID string, in IncomeEventInput) (MutationResult, error) {
	return s.Mutate(ctx, MutationRequest{
		Kind: MutationIncomeEvent, UserID: userID, RequestID: requestID, IncomeEvent: in,
	})
}

// EligibleQuantity reconstructs a user's holding of symbol AS OF asOf from the
// immutable activity ledger, so dividend entitlement uses HISTORICAL holdings
// rather than the current quantity. It sums buy-like activities and subtracts
// sell-like activities whose OccurredAt is on or before asOf, and applies split
// / stock-dividend ratios that took effect on or before asOf. A holder who has
// since sold still receives the dividend for shares held on the ex-date.
//
// This is portfolio tracking, not a tax lot engine: it assumes a single default
// portfolio per user and per-symbol netting.
func (s *Service) EligibleQuantity(ctx context.Context, userID, instrumentID, symbol string, asOf time.Time) (money.Quantity, error) {
	if instrumentID == "" {
		// No resolved identity for this income event's instrument: eligible-
		// quantity calculation falls back to symbol-string matching.
		telemetry.IncLegacySymbolFallback()
	}
	return s.repo.EligibleQuantity(ctx, userID, instrumentID, symbol, asOf)
}

func eligibleQuantityFromActivities(activities []Activity, instrumentID, symbol string, asOf time.Time) money.Quantity {
	// Oldest-first so ratio adjustments apply to the quantity accumulated before
	// the corporate action.
	sortActivitiesOldestFirst(activities)
	sym := normalizeSymbol(symbol)
	// A corrected buy/sell posts a NEW activity carrying the corrected
	// quantity/price rather than editing the original (see
	// Service.CorrectActivity), so the original's own quantity effect must be
	// excluded here or it would double-count alongside the correction.
	correctedAway := make(map[string]bool)
	for _, a := range activities {
		if id, _ := a.Metadata["correction_of_activity_id"].(string); id != "" {
			correctedAway[id] = true
		}
	}

	qty := money.ZeroQuantity()
	for _, a := range activities {
		if correctedAway[a.ID] {
			continue
		}
		matches := instrumentID != "" && a.InstrumentID == instrumentID
		if !matches && instrumentID == "" {
			matches = normalizeSymbol(a.Symbol) == sym
		}
		if !matches {
			continue
		}
		if a.OccurredAt.After(asOf) {
			continue
		}
		switch a.Type {
		case ActivityBuy, ActivityOpeningBalance, ActivityReinvestedDividend:
			if a.Quantity != nil {
				qty = qty.Add(*a.Quantity)
			}
		case ActivitySell:
			if a.Quantity != nil {
				qty = qty.Sub(*a.Quantity)
			}
		case ActivityStockSplit, ActivityStockDividend:
			if f := ratioFactor(a); f.Sign() > 0 {
				qty = qty.MulRatio(f)
			}
		case ActivityReverseSplit:
			if f := ratioFactor(a); f.Sign() > 0 {
				qty = qty.MulRatio(f)
			}
		case ActivityWriteOff:
			qty = money.ZeroQuantity()
		}
	}
	if qty.Sign() < 0 {
		qty = money.ZeroQuantity()
	}
	return money.QuantizeQuantity(qty)
}

// ratioFactor extracts the multiplicative factor a split / stock-dividend
// activity applied to quantity. Splits store numerator/denominator; a stock
// dividend stores the additive ratio (factor = 1 + num/den).
func ratioFactor(a Activity) money.Ratio {
	num, numOK := metadataRatio(a.Metadata["ratio_numerator"])
	den, denOK := metadataRatio(a.Metadata["ratio_denominator"])
	if !numOK || !denOK || num.Sign() <= 0 || den.Sign() <= 0 {
		return money.ZeroRatio()
	}
	fraction, err := num.DivExact(den, money.ScaleQuantity)
	if err != nil {
		return money.ZeroRatio()
	}
	if a.Type == ActivityStockDividend {
		return money.MustRatio("1").Add(fraction)
	}
	return fraction
}

func metadataRatio(value any) (money.Ratio, bool) {
	switch v := value.(type) {
	case string:
		r, err := money.ParseRatio(v)
		return r, err == nil
	case float64: // legacy JSON metadata written before exact-decimal migration
		return money.RatioFromFloat64(v), true
	default:
		return money.ZeroRatio(), false
	}
}

func normalizeSymbol(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

func sortActivitiesOldestFirst(activities []Activity) {
	sort.SliceStable(activities, func(i, j int) bool {
		return activities[i].OccurredAt.Before(activities[j].OccurredAt)
	})
}

// RecordFee records a standalone management/custody/other fee. It reduces cash
// and lowers ranked performance, and never creates negative cash.
func (s *Service) RecordFee(ctx context.Context, userID, requestID string, input FeeInput) (MutationResult, error) {
	return s.Mutate(ctx, MutationRequest{
		Kind: MutationFee, UserID: userID, RequestID: requestID, Fee: input,
	})
}

// RecordCorporateAction records a user-reported corporate action. Splits and
// symbol changes are neutral transformations (ranked index preserved); a
// write-off is a return-bearing loss.
func (s *Service) RecordCorporateAction(ctx context.Context, userID, requestID string, input CorpActionInput) (MutationResult, error) {
	var kind MutationKind
	switch input.Subtype {
	case CorpStockSplit, CorpReverseSplit:
		kind = MutationSplit
	case CorpSymbolChange:
		kind = MutationSymbolChange
	case CorpWriteOff:
		kind = MutationWriteOff
	default:
		return MutationResult{}, ErrInvalidCorporateAction
	}
	return s.Mutate(ctx, MutationRequest{
		Kind: kind, UserID: userID, RequestID: requestID, CorpAction: input,
	})
}
