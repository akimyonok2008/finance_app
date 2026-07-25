package corpactions

import (
	"context"
	"fmt"
	"time"
)

// CorporateActionView is the owner-private, read-only projection surfaced to the
// user. It carries a plain-language explanation and NO private amounts,
// quantities, basis, or provider payloads.
type CorporateActionView struct {
	ID            string
	EventType     string
	DisplaySymbol string
	EffectiveAt   time.Time
	Status        string
	Explanation   string
	AppliedAt     *time.Time
}

// ListCorporateActionViews returns a user's automatic-adjustment history and
// pending items, newest first.
func (s *Service) ListCorporateActionViews(ctx context.Context, userID string) ([]CorporateActionView, error) {
	apps, err := s.store.ListApplicationsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]CorporateActionView, 0, len(apps))
	for _, app := range apps {
		ev, ok, err := s.store.GetEvent(ctx, app.CorporateActionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, CorporateActionView{
			ID:            ev.ID,
			EventType:     string(ev.Type),
			DisplaySymbol: displaySymbol(ev),
			EffectiveAt:   ev.EffectiveAt,
			Status:        userStatus(app.Status),
			Explanation:   explanation(ev, app.Status),
			AppliedAt:     app.AppliedAt,
		})
	}
	return out, nil
}

func displaySymbol(ev CorporateAction) string {
	if ev.Type == TypeSymbolChange && ev.Target != nil {
		return ev.Target.Symbol
	}
	return ev.Source.Symbol
}

// userStatus maps an internal application status to plain user-facing wording.
func userStatus(st ApplicationStatus) string {
	switch st {
	case ApplicationApplied:
		return "Applied automatically"
	case ApplicationApplying, ApplicationPending:
		return "Processing"
	case ApplicationSkipped:
		return "Not applicable to your holding"
	case ApplicationFailed:
		return "Awaiting confirmed market data"
	default:
		return "Processing"
	}
}

// explanation is a concise, non-technical description. It never includes
// quantities, basis, or consideration amounts.
func explanation(ev CorporateAction, st ApplicationStatus) string {
	sym := displaySymbol(ev)
	switch ev.Type {
	case TypeSplit:
		if ev.RatioNumerator != nil && ev.RatioDenominator != nil {
			return fmt.Sprintf("%s split %s-for-%s. Your quantity and cost basis were adjusted automatically; portfolio value and ranked performance were preserved.",
				ev.Source.Symbol, trim(*ev.RatioNumerator), trim(*ev.RatioDenominator))
		}
		return fmt.Sprintf("%s stock split applied automatically.", ev.Source.Symbol)
	case TypeReverseSplit:
		return fmt.Sprintf("%s reverse split applied automatically. Your position and performance history were preserved.", ev.Source.Symbol)
	case TypeSymbolChange:
		return fmt.Sprintf("%s changed ticker to %s. Your position and performance history were preserved.", ev.Source.Symbol, sym)
	case TypeStockMerger, TypeCashMerger, TypeMixedMerger:
		if st == ApplicationApplied {
			return fmt.Sprintf("%s was converted under the announced merger terms. Your position was adjusted automatically.", ev.Source.Symbol)
		}
		return fmt.Sprintf("A corporate action affecting %s is being processed. Your position remains visible while complete market data is confirmed.", ev.Source.Symbol)
	case TypeSpinOff:
		return fmt.Sprintf("A spin-off affecting %s is being processed. No action is required.", ev.Source.Symbol)
	case TypeDelisting:
		return fmt.Sprintf("%s is under a corporate action. Your position remains visible while a resolution is confirmed.", ev.Source.Symbol)
	default:
		return fmt.Sprintf("A corporate action affecting %s is being processed.", ev.Source.Symbol)
	}
}

func trim(f float64) string {
	return fmt.Sprintf("%g", f)
}
