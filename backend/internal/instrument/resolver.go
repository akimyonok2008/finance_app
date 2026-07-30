package instrument

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Resolution is the full outcome of a resolve attempt. Callers that only care
// about the happy path can ignore everything but Instrument and Quality;
// callers that must diagnose an ambiguous ticker get the candidate list instead
// of a silently-picked winner.
type Resolution struct {
	Instrument *Instrument
	Quality    IdentityQuality
	// Candidates is populated only when Quality is QualityAmbiguous.
	Candidates []IdentityCandidate
}

// Resolver turns an external identifier into a stable internal identity,
// consulting the local register first and the identity provider only on a miss.
type Resolver struct {
	repo     Repository
	provider IdentityProvider
	// Now is injected so tests get deterministic validity windows.
	Now func() time.Time
	// ProviderName is recorded on created instruments/aliases as provenance.
	ProviderName string
}

// NewResolver wires a resolver. provider may be nil, in which case a register
// miss is reported as QualityUnresolved without any network call — that is the
// correct degraded behaviour when OPENFIGI_ENABLED is false.
func NewResolver(repo Repository, provider IdentityProvider) *Resolver {
	return &Resolver{
		repo:         repo,
		provider:     provider,
		Now:          func() time.Time { return time.Now().UTC() },
		ProviderName: "openfigi",
	}
}

func (r *Resolver) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// ResolveOrCreate returns the instrument matching query, creating it from a
// single unambiguous provider candidate when the local register has no match.
//
// Outcomes:
//   - a register hit                  -> (instrument, quality from the row, nil)
//   - miss + exactly one candidate    -> (instrument, QualityResolved, nil) and
//     the instrument + its ticker/FIGI/composite-FIGI aliases are created
//   - miss + zero candidates          -> (nil, QualityUnresolved, nil). NOT an
//     error: an unknown ticker is an expected outcome, and returning an error
//     would push callers into treating it as a failure.
//   - miss + several candidates       -> (nil, QualityAmbiguous, nil) with the
//     candidates exposed via ResolveDetailed. Nothing is written: minting an
//     arbitrary identity is worse than having none.
//   - provider/transport failure      -> (nil, QualityUnresolved, err)
func (r *Resolver) ResolveOrCreate(ctx context.Context, query IdentityQuery) (*Instrument, IdentityQuality, error) {
	res, err := r.ResolveDetailed(ctx, query)
	return res.Instrument, res.Quality, err
}

// ResolveDetailed is ResolveOrCreate plus the candidate list behind an
// ambiguous outcome.
func (r *Resolver) ResolveDetailed(ctx context.Context, query IdentityQuery) (Resolution, error) {
	return r.ResolveDetailedAt(ctx, query, nil)
}

// ResolveDetailedAt resolves time-scoped aliases for historical portfolio
// transactions, so a former ticker such as FB can retain META's stable ID.
func (r *Resolver) ResolveDetailedAt(ctx context.Context, query IdentityQuery, asOf *time.Time) (Resolution, error) {
	if existing, err := r.lookupLocalAt(ctx, query, asOf); err != nil {
		return Resolution{Quality: QualityUnresolved}, err
	} else if existing != nil {
		quality := existing.IdentityQuality
		if quality == "" {
			quality = QualityUnresolved
		}
		return Resolution{Instrument: existing, Quality: quality}, nil
	}

	if r.provider == nil {
		return Resolution{Quality: QualityUnresolved}, nil
	}
	candidates, err := r.provider.Resolve(ctx, query)
	if err != nil {
		if errors.Is(err, ErrInsufficientQuery) {
			return Resolution{Quality: QualityUnresolved}, err
		}
		return Resolution{Quality: QualityUnresolved}, err
	}
	switch len(candidates) {
	case 0:
		return Resolution{Quality: QualityUnresolved}, nil
	case 1:
		in, err := r.create(ctx, query, candidates[0])
		if err != nil {
			return Resolution{Quality: QualityUnresolved}, err
		}
		return Resolution{Instrument: in, Quality: QualityResolved}, nil
	default:
		return Resolution{Quality: QualityAmbiguous, Candidates: candidates}, nil
	}
}

// lookupLocal checks the register in decreasing order of identifier strength.
// A FIGI/ISIN/CUSIP hit is authoritative; ticker+exchange beats a bare ticker,
// which can legitimately be ambiguous across exchanges.
func (r *Resolver) lookupLocal(ctx context.Context, query IdentityQuery) (*Instrument, error) {
	return r.lookupLocalAt(ctx, query, nil)
}

func (r *Resolver) lookupLocalAt(ctx context.Context, query IdentityQuery, asOf *time.Time) (*Instrument, error) {
	attempts := []struct {
		typ           AliasType
		value         string
		exchange, mic string
		tolerateAmbig bool
	}{
		{AliasFIGI, query.FIGI, "", "", false},
		{AliasISIN, query.ISIN, "", "", false},
		{AliasCUSIP, query.CUSIP, "", "", false},
		{AliasTicker, query.Ticker, query.ExchangeCode, query.MIC, false},
	}
	for _, a := range attempts {
		if normalizeAliasValue(a.value) == "" {
			continue
		}
		var found *Instrument
		var err error
		if asOf != nil {
			found, err = r.repo.FindInstrumentByAliasAsOf(ctx, a.typ, a.value, a.exchange, a.mic, asOf.UTC())
		} else {
			found, err = r.repo.FindInstrumentByAlias(ctx, a.typ, a.value, a.exchange, a.mic)
		}
		if err != nil {
			if errors.Is(err, ErrAliasConflict) && a.tolerateAmbig {
				continue
			}
			if errors.Is(err, ErrInvalidAlias) {
				continue
			}
			return nil, err
		}
		if found != nil {
			return found, nil
		}
	}
	return nil, nil
}

// create writes the instrument and its identifier aliases. The instrument row
// is written first so the alias FK resolves; if any alias insert fails the
// instrument is left in place carrying whatever aliases succeeded, and the
// caller sees the error — no silently half-identified row is returned.
func (r *Resolver) create(ctx context.Context, query IdentityQuery, c IdentityCandidate) (*Instrument, error) {
	now := r.now()
	exchange := c.ExchangeCode
	if exchange == "" {
		exchange = normalizeScope(query.ExchangeCode)
	}
	mic := c.MIC
	if mic == "" {
		mic = normalizeScope(query.MIC)
	}
	ticker := c.Ticker
	if ticker == "" {
		ticker = normalizeAliasValue(query.Ticker)
	}

	// Opportunistic venue resolution: link to the first-class venue when we
	// have a MIC to key it by. A miss here never blocks instrument creation —
	// venue_id is a denormalization aid, not a required identity component.
	venueID := ""
	if mic != "" {
		if v, verr := r.FindOrCreateVenue(ctx, mic, exchange); verr == nil && v != nil {
			venueID = v.ID
		}
	}

	in, err := r.repo.CreateInstrument(ctx, Instrument{
		ID:               uuid.NewString(),
		FIGI:             c.FIGI,
		CompositeFIGI:    c.CompositeFIGI,
		ShareClassFIGI:   c.ShareClassFIGI,
		ISIN:             normalizeAliasValue(query.ISIN),
		CUSIP:            normalizeAliasValue(query.CUSIP),
		CurrentSymbol:    ticker,
		Name:             c.Name,
		SecurityType:     c.SecurityType,
		ExchangeCode:     exchange,
		MIC:              mic,
		VenueID:          venueID,
		Currency:         c.Currency,
		Status:           StatusActive,
		IdentityQuality:  QualityResolved,
		IdentityProvider: r.ProviderName,
		CreatedAt:        now,
		UpdatedAt:        now,
	})
	if err != nil {
		return nil, fmt.Errorf("instrument: create resolved instrument: %w", err)
	}

	aliases := []struct {
		typ   AliasType
		value string
		scope bool // ticker aliases are exchange/MIC-scoped; global ids are not
	}{
		{AliasTicker, ticker, true},
		{AliasFIGI, c.FIGI, false},
		{AliasCompositeFIGI, c.CompositeFIGI, false},
		{AliasShareClassFIGI, c.ShareClassFIGI, false},
		{AliasISIN, query.ISIN, false},
		{AliasCUSIP, query.CUSIP, false},
	}
	for _, a := range aliases {
		if normalizeAliasValue(a.value) == "" {
			continue
		}
		alias := InstrumentAlias{
			InstrumentID: in.ID,
			AliasType:    a.typ,
			AliasValue:   a.value,
			ValidFrom:    now,
			Provider:     r.ProviderName,
		}
		if a.scope {
			alias.ExchangeCode = exchange
			alias.MIC = mic
			alias.VenueID = venueID
		}
		if _, err := r.repo.CreateAlias(ctx, alias); err != nil {
			// A conflicting alias means another instrument already owns this
			// identifier; surface it rather than leaving two identities that
			// both claim the same FIGI.
			return nil, fmt.Errorf("instrument: create alias %s=%s: %w", a.typ, a.value, err)
		}
	}
	return &in, nil
}

// ResolveTickerAsOf resolves a bare ticker against the LOCAL register only
// (no provider call) as of asOf, for legacy-record backfill: a position or
// activity has only a symbol string and a transaction date, never a FIGI/
// ISIN/exchange — so ticker-only, time-scoped lookup is the strongest
// evidence available. QualityAmbiguous means more than one local instrument
// used that ticker at asOf (never guessed at); QualityUnresolved means none
// did (an unknown/never-registered ticker is an expected outcome, not an
// error).
func (r *Resolver) ResolveTickerAsOf(ctx context.Context, ticker string, asOf time.Time) (*Instrument, IdentityQuality, error) {
	in, err := r.repo.FindInstrumentByAliasAsOf(ctx, AliasTicker, ticker, "", "", asOf)
	if err != nil {
		if errors.Is(err, ErrAliasConflict) {
			return nil, QualityAmbiguous, nil
		}
		if errors.Is(err, ErrInvalidAlias) {
			return nil, QualityUnresolved, nil
		}
		return nil, QualityUnresolved, err
	}
	if in == nil {
		return nil, QualityUnresolved, nil
	}
	return in, QualityResolved, nil
}

// ResolveProviderSymbol translates a stable instrument identity into the
// ticker a market-data provider should be queried with as of asOf, instead
// of trusting a possibly-stale symbol string stored on a position from
// however long ago it was opened. A provider-specific alias (AliasProvider
// Symbol scoped to provider) wins when one exists; otherwise the current (or
// as-of) generic ticker alias is used. Returns "" (not an error) when the
// instrument has no resolvable alias — an expected outcome for an
// instrument created before any alias was recorded, which callers should
// treat as "fall back to the stored symbol" and count as a provider-mapping
// miss.
func (r *Resolver) ResolveProviderSymbol(ctx context.Context, instrumentID, provider string, asOf time.Time) (string, error) {
	if instrumentID == "" {
		return "", nil
	}
	aliases, err := r.repo.ListAliasesForInstrument(ctx, instrumentID)
	if err != nil {
		return "", fmt.Errorf("instrument: resolve provider symbol: %w", err)
	}
	var providerMatch, tickerMatch *InstrumentAlias
	for i := range aliases {
		a := aliases[i]
		if !a.ActiveAt(asOf) {
			continue
		}
		switch {
		case a.AliasType == AliasProviderSymbol && provider != "" && a.Provider == provider:
			providerMatch = &a
		case a.AliasType == AliasTicker:
			tickerMatch = &a
		}
	}
	if providerMatch != nil {
		return providerMatch.AliasValue, nil
	}
	if tickerMatch != nil {
		return tickerMatch.AliasValue, nil
	}
	return "", nil
}

// FindOrCreateVenue resolves the first-class venue for a MIC, creating it
// (keyed on the MIC uniqueness constraint) if this is the first instrument
// seen at that listing. mic must be non-empty; exchangeCode is a hint used
// only when creating a new row.
// InstrumentByID reads one instrument from the identity register by its
// stable internal id — a plain lookup with no provider calls. Used by
// read-side consumers (e.g. competition snapshot classification) that hold a
// position's instrument_id and need its stable metadata (asset type, MIC,
// listing country, currency).
func (r *Resolver) InstrumentByID(ctx context.Context, id string) (*Instrument, error) {
	in, err := r.repo.GetInstrumentByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return &in, nil
}

func (r *Resolver) FindOrCreateVenue(ctx context.Context, mic, exchangeCode string) (*Venue, error) {
	mic = normalizeScope(mic)
	if mic == "" {
		return nil, nil
	}
	if existing, err := r.repo.FindVenueByMIC(ctx, mic); err != nil {
		return nil, fmt.Errorf("instrument: find venue: %w", err)
	} else if existing != nil {
		return existing, nil
	}
	v, err := r.repo.CreateVenue(ctx, Venue{MIC: mic, ExchangeCode: normalizeScope(exchangeCode)})
	if err != nil {
		return nil, fmt.Errorf("instrument: create venue: %w", err)
	}
	return &v, nil
}

// FindOrCreateIssuer resolves the first-class issuer for a CIK, creating it
// if this is the first instrument seen for that issuer. cik must be
// non-empty; name is used only when creating a new row.
func (r *Resolver) FindOrCreateIssuer(ctx context.Context, cik, name string) (*Issuer, error) {
	cik = normalizeAliasValue(cik)
	if cik == "" {
		return nil, nil
	}
	if existing, err := r.repo.FindIssuerByCIK(ctx, cik); err != nil {
		return nil, fmt.Errorf("instrument: find issuer: %w", err)
	} else if existing != nil {
		return existing, nil
	}
	iss, err := r.repo.CreateIssuer(ctx, Issuer{CIK: cik, Name: name})
	if err != nil {
		return nil, fmt.Errorf("instrument: create issuer: %w", err)
	}
	return &iss, nil
}

// ChangeTicker records a ticker change (e.g. FB -> META). The instrument's
// identity is untouched: figi/isin/cusip and the internal id stay exactly as
// they were. Only the ticker alias window moves and current_symbol updates, so
// historical holdings recorded under the old ticker remain resolvable via
// FindInstrumentByAliasAsOf.
func (r *Resolver) ChangeTicker(ctx context.Context, instrumentID, newTicker, exchangeCode, mic string, effectiveAt time.Time, provider string) error {
	if normalizeAliasValue(newTicker) == "" {
		return ErrInvalidAlias
	}
	in, err := r.repo.GetInstrumentByID(ctx, instrumentID)
	if err != nil {
		return err
	}
	effectiveAt = effectiveAt.UTC()

	// Close the currently-active ticker alias, if any. Its absence is not fatal
	// (an instrument may have been registered by ISIN alone).
	active, err := r.repo.FindActiveAlias(ctx, in.ID, AliasTicker)
	if err != nil && !errors.Is(err, ErrAliasNotFound) {
		return err
	}
	if active != nil {
		if err := r.repo.CloseAlias(ctx, active.ID, effectiveAt); err != nil {
			return fmt.Errorf("instrument: close old ticker alias: %w", err)
		}
	}

	scopeExchange := normalizeScope(exchangeCode)
	scopeMIC := normalizeScope(mic)
	if scopeExchange == "" && active != nil {
		scopeExchange = active.ExchangeCode
	}
	if scopeMIC == "" && active != nil {
		scopeMIC = active.MIC
	}
	if provider == "" {
		provider = r.ProviderName
	}

	if _, err := r.repo.CreateAlias(ctx, InstrumentAlias{
		InstrumentID: in.ID,
		AliasType:    AliasTicker,
		AliasValue:   newTicker,
		ExchangeCode: scopeExchange,
		MIC:          scopeMIC,
		ValidFrom:    effectiveAt,
		Provider:     provider,
	}); err != nil {
		return fmt.Errorf("instrument: open new ticker alias: %w", err)
	}
	if err := r.repo.UpdateInstrumentSymbol(ctx, in.ID, newTicker, effectiveAt); err != nil {
		return fmt.Errorf("instrument: update current symbol: %w", err)
	}
	return nil
}
