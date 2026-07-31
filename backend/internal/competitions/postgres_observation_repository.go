package competitions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ardakimyonok/finance_app/internal/prices"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Postgres implementation of ObservationRepository (tables
// competition_observation_sets / competition_price_observations /
// competition_fx_observations, migration 0050).

var _ ObservationRepository = (*PostgresCompetitionRepository)(nil)

func (r *PostgresCompetitionRepository) EnsureObservationSet(ctx context.Context, competitionID, boundary string, effectiveAt time.Time, provider string) (ObservationSet, error) {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO competition_observation_sets (id, competition_id, boundary_type, effective_at, provider)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (competition_id, boundary_type) DO NOTHING
	`, uuid.NewString(), competitionID, boundary, effectiveAt.UTC(), provider)
	if err != nil {
		return ObservationSet{}, fmt.Errorf("competition repository: ensure observation set: %w", err)
	}

	var set ObservationSet
	err = r.pool.QueryRow(ctx, `
		SELECT id, competition_id, boundary_type, effective_at, provider, observation_status, completed_at
		FROM competition_observation_sets
		WHERE competition_id = $1 AND boundary_type = $2
	`, competitionID, boundary).Scan(
		&set.ID, &set.CompetitionID, &set.Boundary, &set.EffectiveAt, &set.Provider, &set.Status, &set.CompletedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ObservationSet{}, fmt.Errorf("competition repository: observation set vanished after insert")
		}
		return ObservationSet{}, fmt.Errorf("competition repository: load observation set: %w", err)
	}
	return set, nil
}

func (r *PostgresCompetitionRepository) LoadObservations(ctx context.Context, setID string) (map[string]PriceObservation, map[string]FxObservation, error) {
	prices := map[string]PriceObservation{}
	priceRows, err := r.pool.Query(ctx, `
		SELECT instrument_id, venue_mic, symbol, price, currency, provider_observed_at, price_methodology, trading_session_date
		FROM competition_price_observations WHERE observation_set_id = $1
	`, setID)
	if err != nil {
		return nil, nil, fmt.Errorf("competition repository: load price observations: %w", err)
	}
	for priceRows.Next() {
		var p PriceObservation
		var instrumentID, venueMIC *string
		var tradingSessionDate *time.Time
		if err := priceRows.Scan(&instrumentID, &venueMIC, &p.Symbol, &p.Price, &p.Currency, &p.ProviderTimestamp, &p.Methodology, &tradingSessionDate); err != nil {
			priceRows.Close()
			return nil, nil, fmt.Errorf("competition repository: scan price observation: %w", err)
		}
		if instrumentID != nil {
			p.InstrumentID = *instrumentID
		}
		if venueMIC != nil {
			p.VenueMIC = *venueMIC
		}
		if tradingSessionDate != nil {
			p.TradingSessionDate = tradingSessionDate.Format("2006-01-02")
		}
		prices[observationKey(p.InstrumentID, p.Symbol)] = p
	}
	if err := priceRows.Err(); err != nil {
		priceRows.Close()
		return nil, nil, err
	}
	priceRows.Close()

	fxRates := map[string]FxObservation{}
	fxRows, err := r.pool.Query(ctx, `
		SELECT from_currency, to_currency, rate, provider_observed_at
		FROM competition_fx_observations WHERE observation_set_id = $1
	`, setID)
	if err != nil {
		return nil, nil, fmt.Errorf("competition repository: load fx observations: %w", err)
	}
	defer fxRows.Close()
	for fxRows.Next() {
		var f FxObservation
		if err := fxRows.Scan(&f.FromCurrency, &f.ToCurrency, &f.Rate, &f.ObservedAt); err != nil {
			return nil, nil, fmt.Errorf("competition repository: scan fx observation: %w", err)
		}
		fxRates[fxPairKey(f.FromCurrency, f.ToCurrency)] = f
	}
	return prices, fxRates, fxRows.Err()
}

// RecordObservations writes newly captured rows and, when sealed, marks the
// set sealed — all in one transaction. ON CONFLICT DO NOTHING makes
// concurrent workers capturing the same symbol/pair a harmless race: whichever
// commits first wins, and the loser's LoadObservations on its next pass sees
// the winner's stored value instead of overwriting it. A set already sealed
// is left untouched entirely: sealing is terminal, and captureObservations
// (observations.go) never asks to write against a sealed set anyway — this
// is a defensive backstop, not the primary guard.
func (r *PostgresCompetitionRepository) RecordObservations(ctx context.Context, setID string, newPrices []PriceObservation, newFX []FxObservation, sealed bool, now time.Time) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("competition repository: begin record observations tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT observation_status FROM competition_observation_sets WHERE id = $1`, setID).Scan(&currentStatus); err != nil {
		return fmt.Errorf("competition repository: load observation set status: %w", err)
	}
	if currentStatus == ObservationSealed {
		return nil
	}

	for _, p := range newPrices {
		var instrumentID, venueMIC *string
		if p.InstrumentID != "" {
			instrumentID = &p.InstrumentID
		}
		if p.VenueMIC != "" {
			venueMIC = &p.VenueMIC
		}
		var tradingSessionDate *time.Time
		if p.TradingSessionDate != "" {
			if d, parseErr := time.Parse("2006-01-02", p.TradingSessionDate); parseErr == nil {
				tradingSessionDate = &d
			}
		}
		methodology := p.Methodology
		if methodology == "" {
			methodology = prices.MethodologyFallbackLatest
		}
		// ON CONFLICT targets the partial unique indexes from migration 0058:
		// instrument_id when present (authoritative identity), symbol as the
		// legacy/unresolved fallback. Postgres requires the inference
		// predicate to match the index's WHERE clause exactly, so the two
		// cases need separate statements.
		var err error
		if instrumentID != nil {
			_, err = tx.Exec(ctx, `
				INSERT INTO competition_price_observations (id, observation_set_id, instrument_id, venue_mic, symbol, price, currency, provider_observed_at, price_methodology, trading_session_date)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
				ON CONFLICT (observation_set_id, instrument_id) WHERE instrument_id IS NOT NULL DO NOTHING
			`, uuid.NewString(), setID, instrumentID, venueMIC, p.Symbol, p.Price, p.Currency, p.ProviderTimestamp.UTC(), methodology, tradingSessionDate)
		} else {
			_, err = tx.Exec(ctx, `
				INSERT INTO competition_price_observations (id, observation_set_id, instrument_id, venue_mic, symbol, price, currency, provider_observed_at, price_methodology, trading_session_date)
				VALUES ($1, $2, NULL, $3, $4, $5, $6, $7, $8, $9)
				ON CONFLICT (observation_set_id, symbol) WHERE instrument_id IS NULL DO NOTHING
			`, uuid.NewString(), setID, venueMIC, p.Symbol, p.Price, p.Currency, p.ProviderTimestamp.UTC(), methodology, tradingSessionDate)
		}
		if err != nil {
			return fmt.Errorf("competition repository: insert price observation: %w", err)
		}
	}
	for _, f := range newFX {
		if _, err := tx.Exec(ctx, `
			INSERT INTO competition_fx_observations (id, observation_set_id, from_currency, to_currency, rate, provider_observed_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (observation_set_id, from_currency, to_currency) DO NOTHING
		`, uuid.NewString(), setID, f.FromCurrency, f.ToCurrency, f.Rate, f.ObservedAt.UTC()); err != nil {
			return fmt.Errorf("competition repository: insert fx observation: %w", err)
		}
	}
	if sealed {
		if _, err := tx.Exec(ctx, `
			UPDATE competition_observation_sets
			SET observation_status = $1, completed_at = $2
			WHERE id = $3 AND observation_status <> $1
		`, ObservationSealed, now.UTC(), setID); err != nil {
			return fmt.Errorf("competition repository: seal observation set: %w", err)
		}
	}
	return tx.Commit(ctx)
}
