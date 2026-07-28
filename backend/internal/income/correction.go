package income

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// CorrectionKind names the constrained, account-specific corrections a user may
// make to an already-DETECTED event. There is deliberately no "create income"
// kind: corrections reference an existing event and can only reconcile it to the
// actual broker outcome.
type CorrectionKind string

const (
	CorrectionActualNet          CorrectionKind = "actual_net"
	CorrectionActualWithholding  CorrectionKind = "actual_withholding"
	CorrectionActualFee          CorrectionKind = "actual_fee"
	CorrectionMarkNotReceived    CorrectionKind = "mark_not_received"
	CorrectionActualReinvestment CorrectionKind = "actual_reinvestment"
)

// Correction is a user-submitted, account-specific adjustment to a detected
// event. Amounts are the ACTUAL broker figures; the pipeline computes the
// compensating delta against what was originally applied.
type Correction struct {
	IncomeEventID string
	PortfolioID   string
	Kind          CorrectionKind
	RequestID     string

	ActualNet               money.Amount
	ActualWithholding       money.Amount
	ActualFee               money.Amount
	ActualReinvestmentQty   money.Quantity
	ActualReinvestmentPrice money.Price
}

// CorrectionAdjustment is the neutral compensating instruction handed to the
// gateway: a signed cash delta in Currency with an audit reason and a link back
// to the original event.
type CorrectionAdjustment struct {
	IncomeEventID string
	PortfolioID   string
	Symbol        string
	Currency      string
	// Delta is the signed net-cash change: positive credits, negative debits.
	Delta  money.Amount
	Reason string
}

var (
	// ErrEventNotFound is returned when a correction references an unknown event.
	ErrEventNotFound = errors.New("income event not found")
	// ErrNotApplied is returned when correcting an event that was never applied
	// to the portfolio (nothing to reconcile).
	ErrNotApplied = errors.New("income event has no application to correct")
	// ErrInvalidCorrection is returned for a malformed correction.
	ErrInvalidCorrection = errors.New("invalid correction")
)

// applyCorrection reconciles an applied event to the actual broker outcome using
// a compensating adjustment, preserving the original ledger activity. It is
// idempotent by RequestID (delegated to the gateway/coordinator).
func (s *Service) applyCorrection(ctx context.Context, userID string, c Correction) error {
	if c.IncomeEventID == "" || c.PortfolioID == "" || strings.TrimSpace(c.RequestID) == "" {
		return ErrInvalidCorrection
	}
	ev, ok, err := s.store.GetEvent(ctx, c.IncomeEventID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrEventNotFound
	}
	app, ok, err := s.store.GetApplication(ctx, c.IncomeEventID, c.PortfolioID)
	if err != nil {
		return err
	}
	if !ok || app.Status != ApplicationApplied {
		return ErrNotApplied
	}
	if app.UserID != userID {
		return ErrNotApplied // never reveal another user's application
	}

	// Determine the actual net after the correction and the compensating delta.
	actualNet, reason, err := s.correctedNet(app, c)
	if err != nil {
		return err
	}
	delta, quantizeErr := money.QuantizeCash(actualNet.Sub(app.NetAmount), app.CashCurrency)
	if quantizeErr != nil {
		return ErrInvalidCorrection
	}
	if !delta.IsZero() {
		adj := CorrectionAdjustment{
			IncomeEventID: ev.ID, PortfolioID: c.PortfolioID, Symbol: ev.Instrument.Symbol,
			Currency: app.CashCurrency, Delta: delta, Reason: reason,
		}
		if adj.Currency == "" {
			adj.Currency = ev.Currency
		}
		if err := s.gateway.ApplyCorrection(ctx, userID, c.RequestID, adj); err != nil {
			return err
		}
	}

	corrected := app
	corrected.Status = ApplicationCorrected
	corrected.NetAmount = actualNet
	corrected.WithholdingAmount = withholdingAfter(app, c)
	corrected.Estimated = false
	if err := s.store.CompleteApplication(ctx, corrected); err != nil {
		return err
	}
	// Reflect the correction on the event without deleting history.
	_ = s.store.SetEventStatus(ctx, ev.ID, StatusCorrected)
	s.metrics.Inc("income_events_corrected_total")
	s.metrics.Observe("income_reconciliation_difference", delta.Float64())
	return nil
}

// correctedNet computes the actual net cash after applying the correction and a
// human-readable audit reason.
func (s *Service) correctedNet(app Application, c Correction) (money.Amount, string, error) {
	switch c.Kind {
	case CorrectionMarkNotReceived:
		return money.ZeroAmount(), "marked not received", nil
	case CorrectionActualNet:
		if c.ActualNet.Sign() < 0 {
			return money.ZeroAmount(), "", ErrInvalidCorrection
		}
		return c.ActualNet, "corrected to actual net", nil
	case CorrectionActualWithholding:
		if c.ActualWithholding.Sign() < 0 || c.ActualWithholding.Cmp(app.GrossAmount) > 0 {
			return money.ZeroAmount(), "", ErrInvalidCorrection
		}
		return app.GrossAmount.Sub(c.ActualWithholding).Sub(app.FeeAmount), "corrected withholding", nil
	case CorrectionActualFee:
		if c.ActualFee.Sign() < 0 || c.ActualFee.Cmp(app.GrossAmount) > 0 {
			return money.ZeroAmount(), "", ErrInvalidCorrection
		}
		return app.GrossAmount.Sub(app.WithholdingAmount).Sub(c.ActualFee), "corrected broker fee", nil
	case CorrectionActualReinvestment:
		// Reinvestment quantity/price corrections do not change net income; the
		// net is preserved and only the recorded quantity changes.
		if c.ActualReinvestmentQty.Sign() < 0 {
			return money.ZeroAmount(), "", ErrInvalidCorrection
		}
		return app.NetAmount, "corrected reinvestment quantity", nil
	default:
		return money.ZeroAmount(), "", fmt.Errorf("%w: unknown kind %q", ErrInvalidCorrection, c.Kind)
	}
}

func withholdingAfter(app Application, c Correction) money.Amount {
	if c.Kind == CorrectionActualWithholding {
		return c.ActualWithholding
	}
	return app.WithholdingAmount
}
