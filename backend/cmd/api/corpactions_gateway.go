package main

import (
	"context"
	"time"

	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/portfolio"
)

// corpActionGateway adapts the portfolio service to the corporate-action
// pipeline's PortfolioGateway. It applies transformations through the aggregate
// coordinator (atomic, idempotent, ranked-continuity-preserving) and marks them
// as provider-sourced, system-generated activity — never user-reported.
type corpActionGateway struct {
	svc *portfolio.Service
}

func (g corpActionGateway) ActiveSymbols(ctx context.Context) ([]string, error) {
	return g.svc.ActiveSymbols(ctx)
}

func (g corpActionGateway) HoldersOfSymbol(ctx context.Context, symbol string) ([]corpactions.Holder, error) {
	holders, err := g.svc.HoldersOfSymbol(ctx, symbol)
	if err != nil {
		return nil, err
	}
	out := make([]corpactions.Holder, 0, len(holders))
	for _, h := range holders {
		out = append(out, corpactions.Holder{UserID: h.UserID, PortfolioID: h.PortfolioID, AcquiredAt: h.AcquiredAt, Symbol: h.Symbol})
	}
	return out, nil
}

func (g corpActionGateway) HoldersOfInstrument(ctx context.Context, instrumentID string) ([]corpactions.Holder, error) {
	holders, err := g.svc.HoldersOfInstrument(ctx, instrumentID)
	if err != nil {
		return nil, err
	}
	out := make([]corpactions.Holder, 0, len(holders))
	for _, h := range holders {
		out = append(out, corpactions.Holder{UserID: h.UserID, PortfolioID: h.PortfolioID, AcquiredAt: h.AcquiredAt, Symbol: h.Symbol})
	}
	return out, nil
}

func (g corpActionGateway) ApplySplit(ctx context.Context, userID, requestID, symbol string, num, den float64, effectiveAt time.Time) error {
	subtype := portfolio.CorpStockSplit
	if num < den {
		subtype = portfolio.CorpReverseSplit
	}
	at := effectiveAt
	_, err := g.svc.RecordCorporateAction(ctx, userID, requestID, portfolio.CorpActionInput{
		Subtype:          subtype,
		Symbol:           symbol,
		RatioNumerator:   num,
		RatioDenominator: den,
		OccurredAt:       &at,
		Provenance:       portfolio.ProvenanceProviderReported,
	})
	return err
}

func (g corpActionGateway) ApplySymbolChange(ctx context.Context, userID, requestID, oldSymbol, newSymbol string, effectiveAt time.Time) error {
	at := effectiveAt
	_, err := g.svc.RecordCorporateAction(ctx, userID, requestID, portfolio.CorpActionInput{
		Subtype:    portfolio.CorpSymbolChange,
		Symbol:     oldSymbol,
		NewSymbol:  newSymbol,
		OccurredAt: &at,
		Provenance: portfolio.ProvenanceProviderReported,
	})
	return err
}

// corpActionViewAdapter converts the corpactions read-only views into the
// portfolio package's view shape (implements portfolio.CorporateActionViewReader).
type corpActionViewAdapter struct {
	svc *corpactions.Service
}

func (a corpActionViewAdapter) ListCorporateActionViews(ctx context.Context, userID string) ([]portfolio.CorporateActionView, error) {
	views, err := a.svc.ListCorporateActionViews(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]portfolio.CorporateActionView, 0, len(views))
	for _, v := range views {
		item := portfolio.CorporateActionView{
			ID:            v.ID,
			EventType:     v.EventType,
			DisplaySymbol: v.DisplaySymbol,
			Status:        v.Status,
			Explanation:   v.Explanation,
			System:        true,
		}
		if !v.EffectiveAt.IsZero() {
			item.EffectiveAt = v.EffectiveAt.Format(time.RFC3339)
		}
		if v.AppliedAt != nil {
			item.AppliedAt = v.AppliedAt.Format(time.RFC3339)
		}
		out = append(out, item)
	}
	return out, nil
}
