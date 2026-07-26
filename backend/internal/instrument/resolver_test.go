package instrument

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyProvider counts calls so tests can PROVE the register-hit path never
// reaches the provider (and therefore never makes an HTTP request).
type spyProvider struct {
	calls      int
	queries    []IdentityQuery
	candidates []IdentityCandidate
	err        error
}

func (s *spyProvider) Resolve(_ context.Context, q IdentityQuery) ([]IdentityCandidate, error) {
	s.calls++
	s.queries = append(s.queries, q)
	return s.candidates, s.err
}

// fixedNow predates the 2022 ticker-change fixtures so alias windows are valid.
var fixedNow = time.Date(2019, 1, 15, 12, 0, 0, 0, time.UTC)

func newResolver(t *testing.T, provider IdentityProvider) (*Resolver, Repository) {
	t.Helper()
	repo := NewInMemoryRepository()
	r := NewResolver(repo, provider)
	r.Now = func() time.Time { return fixedNow }
	return r, repo
}

func TestResolver_AliasHitDoesNotCallTheProvider(t *testing.T) {
	spy := &spyProvider{candidates: []IdentityCandidate{{FIGI: "BBG_SHOULD_NOT_BE_USED"}}}
	r, repo := newResolver(t, spy)
	ctx := context.Background()

	in, err := repo.CreateInstrument(ctx, Instrument{
		FIGI: "BBG000B9XRY4", CurrentSymbol: "AAPL", IdentityQuality: QualityResolved,
	})
	require.NoError(t, err)
	_, err = repo.CreateAlias(ctx, InstrumentAlias{
		InstrumentID: in.ID, AliasType: AliasTicker, AliasValue: "AAPL", ExchangeCode: "UW",
	})
	require.NoError(t, err)

	got, quality, err := r.ResolveOrCreate(ctx, IdentityQuery{Ticker: "AAPL", ExchangeCode: "UW"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, in.ID, got.ID)
	assert.Equal(t, QualityResolved, quality)
	assert.Zero(t, spy.calls, "a register hit must never reach the identity provider")
}

func TestResolver_FIGIHitBeatsTicker(t *testing.T) {
	spy := &spyProvider{}
	r, repo := newResolver(t, spy)
	ctx := context.Background()

	in, err := repo.CreateInstrument(ctx, Instrument{FIGI: "BBG000B9XRY4", CurrentSymbol: "AAPL"})
	require.NoError(t, err)
	_, err = repo.CreateAlias(ctx, InstrumentAlias{
		InstrumentID: in.ID, AliasType: AliasFIGI, AliasValue: "BBG000B9XRY4",
	})
	require.NoError(t, err)

	// No ticker alias exists, but the FIGI resolves.
	got, _, err := r.ResolveOrCreate(ctx, IdentityQuery{FIGI: "bbg000b9xry4", Ticker: "WRONG"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, in.ID, got.ID)
	assert.Zero(t, spy.calls)
}

func TestResolver_MissWithSingleCandidateCreatesInstrumentAndAliases(t *testing.T) {
	spy := &spyProvider{candidates: []IdentityCandidate{{
		FIGI: "BBG000B9XRY4", CompositeFIGI: "BBG000B9XRY4", ShareClassFIGI: "BBG001S5N8V8",
		Ticker: "AAPL", ExchangeCode: "UW", SecurityType: "Common Stock", Name: "APPLE INC",
	}}}
	r, repo := newResolver(t, spy)
	ctx := context.Background()

	got, quality, err := r.ResolveOrCreate(ctx, IdentityQuery{Ticker: "AAPL", ExchangeCode: "UW"})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, QualityResolved, quality)
	assert.Equal(t, 1, spy.calls)

	assert.Equal(t, "BBG000B9XRY4", got.FIGI)
	assert.Equal(t, "AAPL", got.CurrentSymbol)
	assert.Equal(t, "APPLE INC", got.Name)
	assert.Equal(t, QualityResolved, got.IdentityQuality)
	assert.Equal(t, "openfigi", got.IdentityProvider)

	aliases, err := repo.ListAliasesForInstrument(ctx, got.ID)
	require.NoError(t, err)
	byType := map[AliasType]InstrumentAlias{}
	for _, a := range aliases {
		byType[a.AliasType] = a
	}
	require.Contains(t, byType, AliasTicker)
	require.Contains(t, byType, AliasFIGI)
	require.Contains(t, byType, AliasCompositeFIGI)
	require.Contains(t, byType, AliasShareClassFIGI)
	assert.Equal(t, "UW", byType[AliasTicker].ExchangeCode, "ticker aliases are exchange-scoped")
	assert.Empty(t, byType[AliasFIGI].ExchangeCode, "a FIGI is global, not exchange-scoped")
	assert.True(t, byType[AliasTicker].Active())

	// A second call for the same ticker is served from the register.
	again, _, err := r.ResolveOrCreate(ctx, IdentityQuery{Ticker: "AAPL", ExchangeCode: "UW"})
	require.NoError(t, err)
	require.NotNil(t, again)
	assert.Equal(t, got.ID, again.ID)
	assert.Equal(t, 1, spy.calls, "the second call must be a cache hit")
}

func TestResolver_MissWithZeroCandidatesIsUnresolvedAndWritesNothing(t *testing.T) {
	spy := &spyProvider{candidates: nil}
	r, repo := newResolver(t, spy)
	ctx := context.Background()

	got, quality, err := r.ResolveOrCreate(ctx, IdentityQuery{Ticker: "NOSUCH"})
	require.NoError(t, err, "an unknown ticker is an expected outcome, not an error")
	assert.Nil(t, got)
	assert.Equal(t, QualityUnresolved, quality)
	assert.Equal(t, 1, spy.calls)

	mem := repo.(*InMemoryRepository)
	assert.Empty(t, mem.instruments, "no instrument row may be created")
	assert.Empty(t, mem.aliases, "no alias row may be created")
}

func TestResolver_MissWithMultipleCandidatesIsAmbiguousAndWritesNothing(t *testing.T) {
	spy := &spyProvider{candidates: []IdentityCandidate{
		{FIGI: "BBG000BLNNH6", Ticker: "IBM", ExchangeCode: "UN"},
		{FIGI: "BBG000BLNNQ4", Ticker: "IBM", ExchangeCode: "GY"},
	}}
	r, repo := newResolver(t, spy)
	ctx := context.Background()

	res, err := r.ResolveDetailed(ctx, IdentityQuery{Ticker: "IBM"})
	require.NoError(t, err)
	assert.Nil(t, res.Instrument, "an arbitrary pick would mint a wrong identity")
	assert.Equal(t, QualityAmbiguous, res.Quality)
	require.Len(t, res.Candidates, 2, "both candidates must be exposed for diagnosis")

	mem := repo.(*InMemoryRepository)
	assert.Empty(t, mem.instruments)
	assert.Empty(t, mem.aliases)
}

func TestResolver_ProviderErrorIsPropagated(t *testing.T) {
	boom := errors.New("openfigi: mapping request failed: provider responded 429")
	spy := &spyProvider{err: boom}
	r, repo := newResolver(t, spy)

	got, quality, err := r.ResolveOrCreate(context.Background(), IdentityQuery{Ticker: "AAPL"})
	require.ErrorIs(t, err, boom, "a rate limit must not masquerade as 'unresolved'")
	assert.Nil(t, got)
	assert.Equal(t, QualityUnresolved, quality)
	assert.Empty(t, repo.(*InMemoryRepository).instruments)
}

func TestResolver_NilProviderDegradesToUnresolvedWithoutPanicking(t *testing.T) {
	r, _ := newResolver(t, nil)
	got, quality, err := r.ResolveOrCreate(context.Background(), IdentityQuery{Ticker: "AAPL"})
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Equal(t, QualityUnresolved, quality)
}

func TestResolver_ChangeTickerKeepsIdentityStable(t *testing.T) {
	spy := &spyProvider{candidates: []IdentityCandidate{{
		FIGI: "BBG000MM2P62", CompositeFIGI: "BBG000MM2P62",
		Ticker: "FB", ExchangeCode: "UW", Name: "FACEBOOK INC",
	}}}
	r, repo := newResolver(t, spy)
	ctx := context.Background()

	created, _, err := r.ResolveOrCreate(ctx, IdentityQuery{Ticker: "FB", ExchangeCode: "UW"})
	require.NoError(t, err)
	require.NotNil(t, created)
	originalFIGI := created.FIGI

	effective := time.Date(2022, 6, 9, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.ChangeTicker(ctx, created.ID, "META", "UW", "", effective, "manual"))

	// Identity is stable: same internal id, same FIGI.
	after, err := repo.GetInstrumentByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, after.ID)
	assert.Equal(t, originalFIGI, after.FIGI, "a rename must not touch the FIGI")
	assert.Equal(t, "META", after.CurrentSymbol)

	// The old ticker alias is closed, the new one is open.
	aliases, err := repo.ListAliasesForInstrument(ctx, created.ID)
	require.NoError(t, err)
	var oldAlias, newAlias *InstrumentAlias
	for i := range aliases {
		if aliases[i].AliasType != AliasTicker {
			continue
		}
		switch aliases[i].AliasValue {
		case "FB":
			oldAlias = &aliases[i]
		case "META":
			newAlias = &aliases[i]
		}
	}
	require.NotNil(t, oldAlias)
	require.NotNil(t, newAlias)
	require.NotNil(t, oldAlias.ValidTo)
	assert.True(t, effective.Equal(*oldAlias.ValidTo))
	assert.Nil(t, newAlias.ValidTo)
	assert.True(t, effective.Equal(newAlias.ValidFrom))
	assert.Equal(t, "manual", newAlias.Provider)

	// The new ticker resolves; the old one no longer does...
	got, err := repo.FindInstrumentByAlias(ctx, AliasTicker, "META", "UW", "")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)

	got, err = repo.FindInstrumentByAlias(ctx, AliasTicker, "FB", "UW", "")
	require.NoError(t, err)
	assert.Nil(t, got)

	// ...except as of a date inside the old window, which is exactly what
	// historical holdings need.
	got, err = repo.FindInstrumentByAliasAsOf(ctx, AliasTicker, "FB", "UW", "",
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)

	// The instrument is still reachable by its unchanged FIGI.
	got, err = repo.FindInstrumentByAlias(ctx, AliasFIGI, originalFIGI, "", "")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)
}

func TestResolver_ChangeTickerRejectsUnknownInstrumentAndEmptyTicker(t *testing.T) {
	r, _ := newResolver(t, &spyProvider{})
	ctx := context.Background()
	assert.ErrorIs(t, r.ChangeTicker(ctx, "00000000-0000-0000-0000-000000000000", "", "", "", fixedNow, ""), ErrInvalidAlias)
	assert.ErrorIs(t, r.ChangeTicker(ctx, "00000000-0000-0000-0000-000000000000", "META", "", "", fixedNow, ""), ErrInstrumentNotFound)
}

// TestResolver_ChangeTickerAgainstPostgres proves the workflow works against
// the real schema (partial unique index on active aliases), not only in memory.
func TestResolver_ChangeTickerAgainstPostgres(t *testing.T) {
	repo := newPGRepo(t)
	ctx := context.Background()
	ticker := uniq("FB")
	figi := uniq("BBG000MM2P")

	spy := &spyProvider{candidates: []IdentityCandidate{{
		FIGI: figi, CompositeFIGI: figi, Ticker: ticker, ExchangeCode: "UW",
	}}}
	r := NewResolver(repo, spy)
	r.Now = func() time.Time { return fixedNow }

	created, quality, err := r.ResolveOrCreate(ctx, IdentityQuery{Ticker: ticker, ExchangeCode: "UW"})
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, QualityResolved, quality)

	newTicker := uniq("META")
	effective := time.Date(2022, 6, 9, 0, 0, 0, 0, time.UTC)
	require.NoError(t, r.ChangeTicker(ctx, created.ID, newTicker, "UW", "", effective, "manual"))

	after, err := repo.GetInstrumentByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, newTicker, after.CurrentSymbol)
	assert.Equal(t, figi, after.FIGI)

	got, err := repo.FindInstrumentByAlias(ctx, AliasTicker, newTicker, "UW", "")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)

	got, err = repo.FindInstrumentByAlias(ctx, AliasTicker, ticker, "UW", "")
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = repo.FindInstrumentByAliasAsOf(ctx, AliasTicker, ticker, "UW", "",
		time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.ID, got.ID)
}
