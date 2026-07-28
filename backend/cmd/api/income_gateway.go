package main

import (
	"context"
	"errors"
	"time"

	"github.com/ardakimyonok/finance_app/internal/income"
	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// incomeGateway adapts the portfolio service to the income pipeline's
// PortfolioGateway. It applies income through the aggregate coordinator (atomic,
// idempotent, ranked-continuity-preserving) and marks every record as
// provider-sourced, system-generated activity — never user-reported. It cannot
// fabricate arbitrary income: the pipeline supplies a detected, eligibility-
// bounded amount and the coordinator enforces the ranked-performance rules.
type incomeGateway struct {
	svc *portfolio.Service
}

func (g incomeGateway) DiscoveryInstruments(ctx context.Context, since time.Time) ([]income.DiscoveryInstrument, error) {
	items, err := g.svc.IncomeDiscoveryInstruments(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]income.DiscoveryInstrument, 0, len(items))
	for _, item := range items {
		out = append(out, income.DiscoveryInstrument{
			InstrumentID: item.InstrumentID, Symbol: item.Symbol, AssetType: item.AssetType,
		})
	}
	return out, nil
}

func (g incomeGateway) HistoricalHolders(ctx context.Context, instrumentID, symbol string) ([]income.Holder, error) {
	holders, err := g.svc.IncomeHistoricalHolders(ctx, instrumentID, symbol)
	if err != nil {
		return nil, err
	}
	out := make([]income.Holder, 0, len(holders))
	for _, h := range holders {
		out = append(out, income.Holder{
			UserID: h.UserID, PortfolioID: h.PortfolioID, AssetType: h.AssetType, AcquiredAt: h.AcquiredAt,
		})
	}
	return out, nil
}

func (g incomeGateway) EligibleQuantity(ctx context.Context, userID, instrumentID, symbol string, asOf time.Time) (money.Quantity, error) {
	return g.svc.EligibleQuantity(ctx, userID, instrumentID, symbol, asOf)
}

// ApplyIncomeEvent routes EVERY component of one economic income event to the
// portfolio's atomic multi-component mutation. Every component's subtype (and
// therefore its performance effect) is derived from the trusted
// classification — never chosen by an API caller. The event applies through
// the aggregate coordinator in ONE locked transaction: all components commit
// together, under one activity group, with exactly one combined
// ranked-performance update, or none of them commit.
func (g incomeGateway) ApplyIncomeEvent(ctx context.Context, userID, requestID string, in income.AppliedIncomeEvent) error {
	components := make([]portfolio.IncomeComponentInput, 0, len(in.Components))
	for _, c := range in.Components {
		comp := portfolio.IncomeComponentInput{
			Amount:            c.Gross,
			Withholding:       c.Withholding,
			Fee:               c.Fee,
			Estimated:         c.Estimated,
			Reinvest:          c.Reinvest,
			ReinvestPrice:     c.ReinvestPrice,
			PriceMethod:       c.PriceMethod,
			TaxClassification: c.TaxClassification,
			Provenance:        portfolio.ProvenanceProviderReported,
		}
		switch c.Classification {
		case income.ClassStockDividend:
			comp.Subtype = portfolio.IncomeStockDividendSub
			comp.StockRatioNum = c.StockRatioNum
			comp.StockRatioDen = c.StockRatioDen
		case income.ClassReturnOfCapital:
			comp.Subtype = portfolio.IncomeReturnOfCapitalSub
		default:
			if c.Reinvest {
				comp.Subtype = portfolio.IncomeReinvestedDiv
			} else {
				comp.Subtype = incomeSubtype(c.Type)
			}
		}
		components = append(components, comp)
	}
	at := in.PaymentDate
	_, err := g.svc.ApplyIncomeEvent(ctx, userID, requestID, portfolio.IncomeEventInput{
		IncomeEventID: in.IncomeEventID,
		Symbol:        in.Symbol,
		AssetType:     in.AssetType,
		Currency:      in.Currency,
		PaymentDate:   at,
		Components:    components,
	})
	return err
}

// ApplyCorrection posts a signed compensating cash adjustment as system-
// generated activity: a positive delta as (other) income, a negative delta as a
// fee. The original activity is preserved; ranked performance is adjusted once.
func (g incomeGateway) ApplyCorrection(ctx context.Context, userID, requestID string, adj income.CorrectionAdjustment) error {
	now := time.Now().UTC()
	if adj.Delta.Sign() >= 0 {
		_, err := g.svc.RecordIncome(ctx, userID, requestID, portfolio.IncomeInput{
			Subtype: portfolio.IncomeOtherProvider, Symbol: adj.Symbol, Currency: adj.Currency,
			Amount: adj.Delta, IncomeEventID: adj.IncomeEventID, OccurredAt: &now,
			Provenance: portfolio.ProvenanceSystemGenerated,
		})
		return err
	}
	_, err := g.svc.RecordFee(ctx, userID, requestID, portfolio.FeeInput{
		Subtype: portfolio.FeeOther, Currency: adj.Currency, Amount: adj.Delta.Neg(), Symbol: adj.Symbol,
		LinkedActivityID: adj.IncomeEventID, Description: "income correction: " + adj.Reason,
		OccurredAt: &now, Provenance: portfolio.ProvenanceSystemGenerated,
	})
	return err
}

// incomeSubtype maps a normalized income type to the portfolio income subtype.
func incomeSubtype(t income.Type) portfolio.IncomeSubtype {
	switch t {
	case income.TypeSpecialDividend:
		return portfolio.IncomeSpecialDividend
	case income.TypeETFDistribution:
		return portfolio.IncomeETFDistribution
	case income.TypeMutualFundDist:
		return portfolio.IncomeMutualFundDist
	case income.TypeBondCoupon:
		return portfolio.IncomeBondCoupon
	case income.TypeFixedIncomeInt:
		return portfolio.IncomeFixedIncomeInt
	case income.TypeCashInterest:
		return portfolio.IncomeCashInterest
	case income.TypeCapitalGainsDist:
		return portfolio.IncomeCapitalGainsDist
	case income.TypeStakingReward:
		return portfolio.IncomeStakingReward
	case income.TypePaymentInLieu:
		return portfolio.IncomePaymentInLieu
	case income.TypeOtherProvider:
		return portfolio.IncomeOtherProvider
	default:
		return portfolio.IncomeCashDividend
	}
}

// incomeViewAdapter converts the income read-only views into the portfolio
// package's view shape (implements portfolio.IncomeEventViewReader).
type incomeViewAdapter struct {
	svc *income.Service
}

func (a incomeViewAdapter) ListIncomeEventViews(ctx context.Context, userID string) ([]portfolio.IncomeEventView, error) {
	views, err := a.svc.ListIncomeEventViews(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toPortfolioIncomeViews(views), nil
}

func (a incomeViewAdapter) GetIncomeEventView(ctx context.Context, userID, portfolioID, eventID string) (portfolio.IncomeEventView, bool, error) {
	v, ok, err := a.svc.GetIncomeEventView(ctx, userID, portfolioID, eventID)
	if err != nil || !ok {
		return portfolio.IncomeEventView{}, false, err
	}
	return toPortfolioIncomeView(v), true, nil
}

func (a incomeViewAdapter) CorrectIncomeEvent(ctx context.Context, userID string, c portfolio.IncomeCorrectionInput) error {
	err := a.svc.HandleCorrection(ctx, userID, income.Correction{
		IncomeEventID:           c.IncomeEventID,
		PortfolioID:             c.PortfolioID,
		Kind:                    income.CorrectionKind(c.Kind),
		RequestID:               c.RequestID,
		ActualNet:               c.ActualNet,
		ActualWithholding:       c.ActualWithholding,
		ActualFee:               c.ActualFee,
		ActualReinvestmentQty:   c.ActualReinvestmentQty,
		ActualReinvestmentPrice: c.ActualReinvestmentPrice,
	})
	// Translate income-domain errors into portfolio sentinels the HTTP layer maps.
	switch {
	case errors.Is(err, income.ErrEventNotFound):
		return portfolio.ErrIncomeEventNotFound
	case errors.Is(err, income.ErrNotApplied):
		return portfolio.ErrIncomeNotApplied
	case errors.Is(err, income.ErrInvalidCorrection):
		return portfolio.ErrInvalidIncomeCorrection
	default:
		return err
	}
}

func toPortfolioIncomeViews(views []income.IncomeEventView) []portfolio.IncomeEventView {
	out := make([]portfolio.IncomeEventView, 0, len(views))
	for _, v := range views {
		out = append(out, toPortfolioIncomeView(v))
	}
	return out
}

func toPortfolioIncomeView(v income.IncomeEventView) portfolio.IncomeEventView {
	item := portfolio.IncomeEventView{
		ID:            v.ID,
		EventType:     v.EventType,
		Symbol:        v.Symbol,
		Currency:      v.Currency,
		GrossAmount:   v.GrossAmount,
		Withholding:   v.Withholding,
		FeeAmount:     v.FeeAmount,
		NetAmount:     v.NetAmount,
		ReinvestedQty: v.ReinvestedQty,
		Estimated:     v.Estimated,
		Status:        v.Status,
		Provider:      v.Provider,
		Explanation:   v.Explanation,
		Correctable:   v.Correctable,
		System:        v.System,
	}
	if v.PaymentDate != nil {
		item.PaymentDate = v.PaymentDate.Format(time.RFC3339)
	}
	if v.AppliedAt != nil {
		item.AppliedAt = v.AppliedAt.Format(time.RFC3339)
	}
	return item
}
