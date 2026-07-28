package income

import (
	"context"
	"fmt"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// IncomeEventView is the OWNER-PRIVATE, read-only projection of an automatically
// detected and applied income event. The owner may see their own amounts; these
// figures must never appear on any public surface (leaderboard, profile,
// achievements) — public ranked return may reflect income, but the amount stays
// private.
type IncomeEventView struct {
	ID            string         `json:"id"`
	EventType     string         `json:"event_type"`
	Symbol        string         `json:"symbol"`
	Currency      string         `json:"currency"`
	GrossAmount   money.Amount   `json:"gross_amount"`
	Withholding   money.Amount   `json:"withholding_amount"`
	FeeAmount     money.Amount   `json:"fee_amount"`
	NetAmount     money.Amount   `json:"net_amount"`
	ReinvestedQty money.Quantity `json:"reinvestment_quantity,omitempty"`
	Estimated     bool           `json:"estimated"`
	Status        string         `json:"status"`
	Provider      string         `json:"provider"`
	Explanation   string         `json:"explanation"`
	Correctable   bool           `json:"correctable"`
	PaymentDate   *time.Time     `json:"payment_date,omitempty"`
	AppliedAt     *time.Time     `json:"applied_at,omitempty"`
	System        bool           `json:"system_generated"`
}

// ListIncomeEventViews returns a user's automatic-income history and pending
// items, newest first.
func (s *Service) ListIncomeEventViews(ctx context.Context, userID string) ([]IncomeEventView, error) {
	apps, err := s.store.ListApplicationsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]IncomeEventView, 0, len(apps))
	for _, app := range apps {
		if app.Status == ApplicationSkipped {
			continue // not applicable to this holding; nothing to show
		}
		ev, ok, err := s.store.GetEvent(ctx, app.IncomeEventID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, toView(ev, app))
	}
	return out, nil
}

// GetIncomeEventView returns one event view for the owner, or ok=false.
func (s *Service) GetIncomeEventView(ctx context.Context, userID, portfolioID, eventID string) (IncomeEventView, bool, error) {
	ev, ok, err := s.store.GetEvent(ctx, eventID)
	if err != nil || !ok {
		return IncomeEventView{}, false, err
	}
	app, ok, err := s.store.GetApplication(ctx, eventID, portfolioID)
	if err != nil {
		return IncomeEventView{}, false, err
	}
	if !ok || app.UserID != userID {
		return IncomeEventView{}, false, nil
	}
	return toView(ev, app), true, nil
}

func toView(ev IncomeEvent, app Application) IncomeEventView {
	v := IncomeEventView{
		ID:            ev.ID,
		EventType:     string(ev.Type),
		Symbol:        ev.Instrument.Symbol,
		Currency:      app.CashCurrency,
		GrossAmount:   app.GrossAmount,
		Withholding:   app.WithholdingAmount,
		FeeAmount:     app.FeeAmount,
		NetAmount:     app.NetAmount,
		ReinvestedQty: app.ReinvestmentQuantity,
		Estimated:     app.Estimated,
		Status:        userStatus(app.Status),
		Provider:      ev.Provider,
		Explanation:   explanation(ev, app),
		Correctable:   app.Status == ApplicationApplied,
		AppliedAt:     app.AppliedAt,
		System:        true,
	}
	if v.Currency == "" {
		v.Currency = ev.Currency
	}
	if !ev.PaymentDate.IsZero() {
		pd := ev.PaymentDate
		v.PaymentDate = &pd
	}
	return v
}

func userStatus(st ApplicationStatus) string {
	switch st {
	case ApplicationApplied:
		return "Credited automatically"
	case ApplicationApplying, ApplicationPending:
		return "Processing"
	case ApplicationCorrected:
		return "Corrected"
	case ApplicationFailed:
		return "Awaiting confirmed data"
	default:
		return "Processing"
	}
}

// explanation is a concise, plain-language description for the owner.
func explanation(ev IncomeEvent, app Application) string {
	sym := ev.Instrument.Symbol
	switch Classify(ev.Type) {
	case ClassStockDividend:
		return fmt.Sprintf("%s paid a stock dividend. Your quantity and per-share cost basis were adjusted automatically; total basis and ranked performance were preserved.", sym)
	case ClassReturnOfCapital:
		return fmt.Sprintf("%s return of capital credited automatically. Cash was added and your remaining cost basis was reduced — this is portfolio tracking, not tax advice.", sym)
	default:
		base := fmt.Sprintf("%s %s credited automatically. No action was required.", sym, humanType(ev.Type))
		if app.Estimated {
			base += " Amount is an estimated gross figure and may be reconciled with broker data later."
		}
		return base
	}
}

func humanType(t Type) string {
	switch t {
	case TypeCashDividend:
		return "dividend"
	case TypeSpecialDividend:
		return "special dividend"
	case TypeETFDistribution:
		return "ETF distribution"
	case TypeMutualFundDist:
		return "fund distribution"
	case TypeBondCoupon, TypeFixedIncomeInt:
		return "bond coupon"
	case TypeCapitalGainsDist:
		return "capital-gains distribution"
	case TypeCashInterest:
		return "interest"
	case TypeStakingReward:
		return "staking reward"
	case TypePaymentInLieu:
		return "payment in lieu"
	default:
		return "income"
	}
}
