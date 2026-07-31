package instrument

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// InMemoryRepository is the process-local implementation used for
// zero-infrastructure development and tests. It mirrors the PostgreSQL
// semantics that matter: active-alias uniqueness, FIGI uniqueness, validity
// windows and ambiguity detection.
type InMemoryRepository struct {
	mu          sync.RWMutex
	instruments map[string]Instrument
	aliases     map[string]InstrumentAlias
	venues      map[string]Venue
	issuers     map[string]Issuer
}

// NewInMemoryRepository returns an empty in-memory identity register.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		instruments: make(map[string]Instrument),
		aliases:     make(map[string]InstrumentAlias),
		venues:      make(map[string]Venue),
		issuers:     make(map[string]Issuer),
	}
}

var _ Repository = (*InMemoryRepository)(nil)

func (r *InMemoryRepository) CreateInstrument(ctx context.Context, in Instrument) (Instrument, error) {
	if err := ctx.Err(); err != nil {
		return Instrument{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if in.ID == "" {
		in.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if in.CreatedAt.IsZero() {
		in.CreatedAt = now
	}
	in.UpdatedAt = now
	if in.Status == "" {
		in.Status = StatusActive
	}
	if in.IdentityQuality == "" {
		in.IdentityQuality = QualityUnresolved
	}
	if in.Sector == "" {
		in.Sector = SectorUnknown
	}
	in.FIGI = normalizeAliasValue(in.FIGI)
	in.CompositeFIGI = normalizeAliasValue(in.CompositeFIGI)
	in.ShareClassFIGI = normalizeAliasValue(in.ShareClassFIGI)
	in.CurrentSymbol = normalizeAliasValue(in.CurrentSymbol)

	if in.FIGI != "" {
		for _, existing := range r.instruments {
			if existing.FIGI == in.FIGI {
				return Instrument{}, ErrAliasConflict
			}
		}
	}
	r.instruments[in.ID] = in
	return in, nil
}

func (r *InMemoryRepository) GetInstrumentByID(ctx context.Context, id string) (Instrument, error) {
	if err := ctx.Err(); err != nil {
		return Instrument{}, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	in, ok := r.instruments[id]
	if !ok {
		return Instrument{}, ErrInstrumentNotFound
	}
	return in, nil
}

func (r *InMemoryRepository) UpdateInstrumentSymbol(ctx context.Context, id, currentSymbol string, updatedAt time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	in, ok := r.instruments[id]
	if !ok {
		return ErrInstrumentNotFound
	}
	// Identity fields (FIGI/ISIN/CUSIP) are deliberately untouched: a rename
	// does not change what the instrument IS.
	in.CurrentSymbol = normalizeAliasValue(currentSymbol)
	in.UpdatedAt = updatedAt.UTC()
	r.instruments[id] = in
	return nil
}

func (r *InMemoryRepository) CreateAlias(ctx context.Context, alias InstrumentAlias) (InstrumentAlias, error) {
	if err := ctx.Err(); err != nil {
		return InstrumentAlias{}, err
	}
	if !alias.AliasType.Valid() || normalizeAliasValue(alias.AliasValue) == "" {
		return InstrumentAlias{}, ErrInvalidAlias
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.instruments[alias.InstrumentID]; !ok {
		return InstrumentAlias{}, ErrInstrumentNotFound
	}
	if alias.ID == "" {
		alias.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if alias.ValidFrom.IsZero() {
		alias.ValidFrom = now
	}
	alias.ValidFrom = alias.ValidFrom.UTC()
	if alias.ValidTo != nil {
		v := alias.ValidTo.UTC()
		alias.ValidTo = &v
	}
	alias.CreatedAt = now
	alias.AliasValue = normalizeAliasValue(alias.AliasValue)
	alias.ExchangeCode = normalizeScope(alias.ExchangeCode)
	alias.MIC = normalizeScope(alias.MIC)

	// Mirrors instrument_aliases_active_key: at most one ACTIVE alias per
	// (type, value, exchange, mic).
	if alias.ValidTo == nil {
		for _, existing := range r.aliases {
			if existing.ValidTo == nil &&
				existing.AliasType == alias.AliasType &&
				existing.AliasValue == alias.AliasValue &&
				existing.ExchangeCode == alias.ExchangeCode &&
				existing.MIC == alias.MIC {
				return InstrumentAlias{}, ErrAliasConflict
			}
		}
	}
	r.aliases[alias.ID] = alias
	return alias, nil
}

func (r *InMemoryRepository) CloseAlias(ctx context.Context, aliasID string, validTo time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	alias, ok := r.aliases[aliasID]
	if !ok {
		return ErrAliasNotFound
	}
	if alias.ValidTo != nil {
		return ErrAliasNotActive
	}
	v := validTo.UTC()
	alias.ValidTo = &v
	r.aliases[aliasID] = alias
	return nil
}

func (r *InMemoryRepository) ListAliasesForInstrument(ctx context.Context, instrumentID string) ([]InstrumentAlias, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]InstrumentAlias, 0)
	for _, a := range r.aliases {
		if a.InstrumentID == instrumentID {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ValidFrom.Equal(out[j].ValidFrom) {
			return out[i].ID < out[j].ID
		}
		return out[i].ValidFrom.Before(out[j].ValidFrom)
	})
	return out, nil
}

func (r *InMemoryRepository) FindActiveAlias(ctx context.Context, instrumentID string, aliasType AliasType) (*InstrumentAlias, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, a := range r.aliases {
		if a.InstrumentID == instrumentID && a.AliasType == aliasType && a.ValidTo == nil {
			found := a
			return &found, nil
		}
	}
	return nil, ErrAliasNotFound
}

func (r *InMemoryRepository) FindInstrumentByAlias(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string) (*Instrument, error) {
	return r.findByAlias(ctx, aliasType, aliasValue, exchangeCode, mic, nil)
}

func (r *InMemoryRepository) FindInstrumentByAliasAsOf(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string, asOf time.Time) (*Instrument, error) {
	t := asOf.UTC()
	return r.findByAlias(ctx, aliasType, aliasValue, exchangeCode, mic, &t)
}

func (r *InMemoryRepository) findByAlias(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string, asOf *time.Time) (*Instrument, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value := normalizeAliasValue(aliasValue)
	if value == "" || !aliasType.Valid() {
		return nil, ErrInvalidAlias
	}
	ex := normalizeScope(exchangeCode)
	m := normalizeScope(mic)

	r.mu.RLock()
	defer r.mu.RUnlock()

	matched := make(map[string]bool)
	for _, a := range r.aliases {
		if a.AliasType != aliasType || a.AliasValue != value {
			continue
		}
		if ex != "" && a.ExchangeCode != ex {
			continue
		}
		if m != "" && a.MIC != m {
			continue
		}
		if asOf == nil {
			if a.ValidTo != nil {
				continue
			}
		} else if !a.ActiveAt(*asOf) {
			continue
		}
		matched[a.InstrumentID] = true
	}
	switch len(matched) {
	case 0:
		return nil, nil
	case 1:
		for id := range matched {
			in := r.instruments[id]
			return &in, nil
		}
	}
	// Several distinct instruments share the identifier under this scope: the
	// caller must narrow with an exchange or MIC rather than get a coin flip.
	return nil, ErrAliasConflict
}

func (r *InMemoryRepository) FindVenueByMIC(ctx context.Context, mic string) (*Venue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	mic = normalizeScope(mic)
	if mic == "" {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, v := range r.venues {
		if v.MIC == mic {
			found := v
			return &found, nil
		}
	}
	return nil, nil
}

func (r *InMemoryRepository) CreateVenue(ctx context.Context, v Venue) (Venue, error) {
	if err := ctx.Err(); err != nil {
		return Venue{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	v.MIC = normalizeScope(v.MIC)
	for _, existing := range r.venues {
		if v.MIC != "" && existing.MIC == v.MIC {
			return existing, nil
		}
	}
	if v.ID == "" {
		v.ID = uuid.NewString()
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	r.venues[v.ID] = v
	return v, nil
}

func (r *InMemoryRepository) FindIssuerByCIK(ctx context.Context, cik string) (*Issuer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cik = normalizeAliasValue(cik)
	if cik == "" {
		return nil, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, iss := range r.issuers {
		if iss.CIK == cik {
			found := iss
			return &found, nil
		}
	}
	return nil, nil
}

func (r *InMemoryRepository) CreateIssuer(ctx context.Context, iss Issuer) (Issuer, error) {
	if err := ctx.Err(); err != nil {
		return Issuer{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	iss.CIK = normalizeAliasValue(iss.CIK)
	for _, existing := range r.issuers {
		if iss.CIK != "" && existing.CIK == iss.CIK {
			return existing, nil
		}
	}
	if iss.ID == "" {
		iss.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if iss.CreatedAt.IsZero() {
		iss.CreatedAt = now
	}
	iss.UpdatedAt = now
	r.issuers[iss.ID] = iss
	return iss, nil
}

func (r *InMemoryRepository) SetInstrumentVenueAndIssuer(ctx context.Context, instrumentID, venueID, issuerID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	in, ok := r.instruments[instrumentID]
	if !ok {
		return ErrInstrumentNotFound
	}
	in.VenueID = venueID
	in.IssuerID = issuerID
	in.UpdatedAt = time.Now().UTC()
	r.instruments[instrumentID] = in
	return nil
}
