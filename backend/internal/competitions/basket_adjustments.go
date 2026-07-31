package competitions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ardakimyonok/finance_app/internal/corpactions"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// BasketAdjustmentProvider normalizes a frozen basket for splits and symbol
// changes effective after its common baseline. It deliberately excludes cash
// distributions and fees: schema v1 is a fixed-basket price-return model.
type BasketAdjustmentProvider interface {
	AdjustPosition(ctx context.Context, instrumentID, symbol string, quantity money.Quantity, from, through time.Time) (string, money.Quantity, error)
}

func (s *Service) SetBasketAdjustmentProvider(p BasketAdjustmentProvider) {
	s.basketAdjustments = p
}

func (s *Service) adjustedEntryBasket(ctx context.Context, comp Competition, entry CompetitionEntry, through time.Time) (CompetitionEntry, error) {
	if s.basketAdjustments == nil {
		// Identity-bearing engine snapshots must not silently score through an
		// unnormalized corporate-action interval. Memory tests and deployments
		// without identity data keep the original basket.
		return entry, nil
	}
	for i := range entry.Snapshots {
		snap := &entry.Snapshots[i]
		if !snap.IncludedInScore {
			continue
		}
		snap.OriginalQuantity = snap.Quantity
		symbol, quantity, err := s.basketAdjustments.AdjustPosition(ctx, snap.InstrumentID, snap.Symbol, snap.Quantity, comp.StartsAt, through)
		if err != nil {
			return CompetitionEntry{}, fmt.Errorf("normalize basket position %s: %w", snap.ID, err)
		}
		snap.Symbol, snap.Quantity = symbol, quantity
	}
	return entry, nil
}

// PostgresBasketAdjustmentProvider reads only trusted, applied split and
// symbol-change events. Ordering by effective time handles chains such as
// OLD->NEW followed by a later split under NEW.
type PostgresBasketAdjustmentProvider struct{ pool *pgxpool.Pool }

func NewPostgresBasketAdjustmentProvider(pool *pgxpool.Pool) *PostgresBasketAdjustmentProvider {
	return &PostgresBasketAdjustmentProvider{pool: pool}
}

func (p *PostgresBasketAdjustmentProvider) AdjustPosition(ctx context.Context, instrumentID, symbol string, quantity money.Quantity, from, through time.Time) (string, money.Quantity, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT normalized_payload
		FROM corporate_actions
		WHERE status='applied' AND event_type IN ('split','reverse_split','symbol_change')
		  AND effective_at > $1 AND effective_at <= $2
		ORDER BY effective_at, id
	`, from.UTC(), through.UTC())
	if err != nil {
		return "", money.Quantity{}, err
	}
	defer rows.Close()
	currentSymbol := symbol
	currentQuantity := quantity
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return "", money.Quantity{}, err
		}
		var action corpactions.CorporateAction
		if err := json.Unmarshal(payload, &action); err != nil {
			return "", money.Quantity{}, fmt.Errorf("decode corporate action: %w", err)
		}
		matches := action.Source.Symbol == currentSymbol
		if instrumentID != "" {
			// Stable identity is authoritative when present. Never fall back to a
			// ticker match, which could apply an unrelated reused symbol's event.
			matches = action.Source.InstrumentID == instrumentID
		}
		if !matches {
			continue
		}
		switch action.Type {
		case corpactions.TypeSplit, corpactions.TypeReverseSplit:
			if action.RatioNumerator == nil || action.RatioDenominator == nil || *action.RatioDenominator == 0 {
				return "", money.Quantity{}, fmt.Errorf("corporate action %s has an invalid split ratio", action.ID)
			}
			ratio := money.RatioFromFloat64(*action.RatioNumerator / *action.RatioDenominator)
			currentQuantity = currentQuantity.MulRatio(ratio)
		case corpactions.TypeSymbolChange:
			if action.Target == nil || action.Target.Symbol == "" {
				return "", money.Quantity{}, fmt.Errorf("corporate action %s has no target symbol", action.ID)
			}
			currentSymbol = action.Target.Symbol
		}
	}
	if err := rows.Err(); err != nil {
		return "", money.Quantity{}, err
	}
	return currentSymbol, currentQuantity, nil
}

var _ BasketAdjustmentProvider = (*PostgresBasketAdjustmentProvider)(nil)
