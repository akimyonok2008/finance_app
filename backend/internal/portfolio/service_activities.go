package portfolio

import (
	"context"
	"sort"
	"strings"
	"time"
)

// SymbolHolder identifies a portfolio holding a symbol and when it was acquired
// (earliest open-position creation). The automatic corporate-action pipeline
// uses it to discover affected portfolios and check effective-date eligibility.
type SymbolHolder struct {
	UserID      string
	PortfolioID string
	AssetType   string
	AcquiredAt  time.Time
}

type IncomeDiscoveryInstrument struct {
	InstrumentID string
	Symbol       string
	AssetType    string
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

// EligibleQuantity reconstructs a user's holding of symbol AS OF asOf from the
// immutable activity ledger, so dividend entitlement uses HISTORICAL holdings
// rather than the current quantity. It sums buy-like activities and subtracts
// sell-like activities whose OccurredAt is on or before asOf, and applies split
// / stock-dividend ratios that took effect on or before asOf. A holder who has
// since sold still receives the dividend for shares held on the ex-date.
//
// This is portfolio tracking, not a tax lot engine: it assumes a single default
// portfolio per user and per-symbol netting.
func (s *Service) EligibleQuantity(ctx context.Context, userID, instrumentID, symbol string, asOf time.Time) (float64, error) {
	// A high explicit limit: the Postgres reader caps a non-positive limit at 100,
	// but eligibility must consider the full history.
	activities, err := s.repo.ListActivities(ctx, userID, 100000)
	if err != nil {
		return 0, err
	}
	// Oldest-first so ratio adjustments apply to the quantity accumulated before
	// the corporate action.
	sortActivitiesOldestFirst(activities)
	sym := normalizeSymbol(symbol)
	var qty float64
	for _, a := range activities {
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
				qty += a.Quantity.Float64()
			}
		case ActivitySell:
			if a.Quantity != nil {
				qty -= a.Quantity.Float64()
			}
		case ActivityStockSplit, ActivityStockDividend:
			if f := ratioFactor(a); f > 0 {
				qty *= f
			}
		case ActivityReverseSplit:
			if f := ratioFactor(a); f > 0 {
				qty *= f
			}
		case ActivityWriteOff:
			qty = 0
		}
	}
	if qty < 0 {
		qty = 0
	}
	return qty, nil
}

// ratioFactor extracts the multiplicative factor a split / stock-dividend
// activity applied to quantity. Splits store numerator/denominator; a stock
// dividend stores the additive ratio (factor = 1 + num/den).
func ratioFactor(a Activity) float64 {
	num, _ := a.Metadata["ratio_numerator"].(float64)
	den, _ := a.Metadata["ratio_denominator"].(float64)
	if num <= 0 || den <= 0 {
		return 0
	}
	if a.Type == ActivityStockDividend {
		return 1 + num/den
	}
	return num / den
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
