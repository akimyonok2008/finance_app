package instrument

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/db"
)

// This is a PARITY suite: every case runs against BOTH the in-memory and the
// PostgreSQL repository through the same Repository interface, so a behavioural
// divergence between development and production storage fails the build.
//
// The Postgres leg is skipped unless DATABASE_URL_TEST points at a disposable
// database.

func newPGRepo(t *testing.T) Repository {
	t.Helper()
	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("DATABASE_URL_TEST not set; skipping Postgres repository parity leg")
	}
	pool, err := db.ConnectPostgres(context.Background(), url)
	require.NoError(t, err)
	require.NoError(t, db.RunMigrations(context.Background(), pool))
	t.Cleanup(pool.Close)
	return NewPostgresRepository(pool)
}

// uniq keeps parallel/repeat runs against a shared Postgres database from
// colliding on the active-alias unique index.
func uniq(prefix string) string {
	return strings.ToUpper(prefix + uuid.NewString()[:8])
}

func forEachRepo(t *testing.T, fn func(t *testing.T, repo Repository)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { fn(t, NewInMemoryRepository()) })
	t.Run("postgres", func(t *testing.T) { fn(t, newPGRepo(t)) })
}

func seedInstrument(t *testing.T, repo Repository, symbol, figi string) Instrument {
	t.Helper()
	in, err := repo.CreateInstrument(context.Background(), Instrument{
		FIGI:             figi,
		CurrentSymbol:    symbol,
		Name:             "Test Corp",
		AssetType:        "stock",
		ExchangeCode:     "UN",
		MIC:              "XNYS",
		Currency:         "USD",
		IdentityQuality:  QualityResolved,
		IdentityProvider: "openfigi",
	})
	require.NoError(t, err)
	require.NotEmpty(t, in.ID)
	return in
}

func TestRepository_CreateAndGetInstrument(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		in := seedInstrument(t, repo, uniq("AAP"), uniq("BBG000B9XR"))

		got, err := repo.GetInstrumentByID(ctx, in.ID)
		require.NoError(t, err)
		assert.Equal(t, in.ID, got.ID)
		assert.Equal(t, in.FIGI, got.FIGI)
		assert.Equal(t, "Test Corp", got.Name)
		assert.Equal(t, QualityResolved, got.IdentityQuality)
		assert.Equal(t, StatusActive, got.Status)

		_, err = repo.GetInstrumentByID(ctx, uuid.NewString())
		assert.ErrorIs(t, err, ErrInstrumentNotFound)
	})
}

func TestRepository_FindByAliasTickerAndExchange(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		ticker := uniq("TKR")
		in := seedInstrument(t, repo, ticker, uniq("BBG000C9XR"))
		_, err := repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: in.ID, AliasType: AliasTicker, AliasValue: ticker,
			ExchangeCode: "UN", MIC: "XNYS", Provider: "openfigi",
		})
		require.NoError(t, err)

		// ticker + exchange
		found, err := repo.FindInstrumentByAlias(ctx, AliasTicker, ticker, "UN", "")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, in.ID, found.ID)

		// ticker + MIC
		found, err = repo.FindInstrumentByAlias(ctx, AliasTicker, ticker, "", "XNYS")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, in.ID, found.ID)

		// lookup is case-insensitive in both backends
		found, err = repo.FindInstrumentByAlias(ctx, AliasTicker, lower(ticker), "", "")
		require.NoError(t, err)
		require.NotNil(t, found)

		// wrong exchange must NOT match
		found, err = repo.FindInstrumentByAlias(ctx, AliasTicker, ticker, "UW", "")
		require.NoError(t, err)
		assert.Nil(t, found)

		// unknown ticker: nil instrument, nil error (a miss is not a failure)
		found, err = repo.FindInstrumentByAlias(ctx, AliasTicker, uniq("NOPE"), "", "")
		require.NoError(t, err)
		assert.Nil(t, found)
	})
}

func TestRepository_ActiveAliasUniquenessIsEnforced(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		ticker := uniq("DUP")
		a := seedInstrument(t, repo, ticker, uniq("BBG000D9XR"))
		b := seedInstrument(t, repo, ticker+"X", uniq("BBG000E9XR"))

		_, err := repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: a.ID, AliasType: AliasTicker, AliasValue: ticker,
			ExchangeCode: "UN", MIC: "XNYS",
		})
		require.NoError(t, err)

		// Same (type, value, exchange, mic) while still active -> conflict.
		_, err = repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: b.ID, AliasType: AliasTicker, AliasValue: ticker,
			ExchangeCode: "UN", MIC: "XNYS",
		})
		assert.ErrorIs(t, err, ErrAliasConflict)

		// A different exchange is a legitimately different listing.
		_, err = repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: b.ID, AliasType: AliasTicker, AliasValue: ticker,
			ExchangeCode: "GY", MIC: "XETR",
		})
		assert.NoError(t, err)
	})
}

func TestRepository_FIGIUniquenessIsEnforced(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		figi := uniq("BBG000F9XR")
		seedInstrument(t, repo, uniq("FIG"), figi)
		_, err := repo.CreateInstrument(context.Background(), Instrument{
			CurrentSymbol: uniq("FIG"), FIGI: figi,
		})
		assert.ErrorIs(t, err, ErrAliasConflict)
	})
}

func TestRepository_UnresolvedInstrumentsDoNotCollideOnNullFIGI(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		_, err := repo.CreateInstrument(ctx, Instrument{CurrentSymbol: uniq("U1"), IdentityQuality: QualityUnresolved})
		require.NoError(t, err)
		// A second FIGI-less instrument must be allowed: '' is not an identity.
		_, err = repo.CreateInstrument(ctx, Instrument{CurrentSymbol: uniq("U2"), IdentityQuality: QualityUnresolved})
		assert.NoError(t, err)
	})
}

func TestRepository_CloseAliasHidesItFromActiveLookupButKeepsHistory(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		oldTicker := uniq("OLD")
		in := seedInstrument(t, repo, oldTicker, uniq("BBG000G9XR"))
		validFrom := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
		alias, err := repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: in.ID, AliasType: AliasTicker, AliasValue: oldTicker,
			ExchangeCode: "UN", MIC: "XNYS", ValidFrom: validFrom,
		})
		require.NoError(t, err)

		closedAt := time.Date(2022, 6, 9, 0, 0, 0, 0, time.UTC)
		require.NoError(t, repo.CloseAlias(ctx, alias.ID, closedAt))

		// Documented semantics: the plain lookup is ACTIVE-ONLY, so a closed
		// ticker no longer resolves.
		found, err := repo.FindInstrumentByAlias(ctx, AliasTicker, oldTicker, "UN", "")
		require.NoError(t, err)
		assert.Nil(t, found, "closed alias must not resolve via the active lookup")

		// ...but it is still resolvable as of a time inside its window, which is
		// what historical holdings need.
		found, err = repo.FindInstrumentByAliasAsOf(ctx, AliasTicker, oldTicker, "UN", "",
			time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, in.ID, found.ID)

		// After the close instant it is gone from the as-of lookup too.
		found, err = repo.FindInstrumentByAliasAsOf(ctx, AliasTicker, oldTicker, "UN", "",
			time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.Nil(t, found)

		// The historical row is retained, not deleted.
		aliases, err := repo.ListAliasesForInstrument(ctx, in.ID)
		require.NoError(t, err)
		require.Len(t, aliases, 1)
		require.NotNil(t, aliases[0].ValidTo)
		assert.True(t, closedAt.Equal(*aliases[0].ValidTo))

		// Double close is reported, not silently accepted.
		assert.ErrorIs(t, repo.CloseAlias(ctx, alias.ID, closedAt), ErrAliasNotActive)
		assert.ErrorIs(t, repo.CloseAlias(ctx, uuid.NewString(), closedAt), ErrAliasNotFound)
	})
}

func TestRepository_ClosingFreesTheActiveUniquenessSlot(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		ticker := uniq("REUSE")
		a := seedInstrument(t, repo, ticker, uniq("BBG000H9XR"))
		b := seedInstrument(t, repo, ticker, uniq("BBG000I9XR"))

		alias, err := repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: a.ID, AliasType: AliasTicker, AliasValue: ticker, ExchangeCode: "UN",
		})
		require.NoError(t, err)
		require.NoError(t, repo.CloseAlias(ctx, alias.ID, time.Now().UTC()))

		// Ticker reuse after a delisting must be possible.
		_, err = repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: b.ID, AliasType: AliasTicker, AliasValue: ticker, ExchangeCode: "UN",
		})
		assert.NoError(t, err)

		found, err := repo.FindInstrumentByAlias(ctx, AliasTicker, ticker, "UN", "")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, b.ID, found.ID, "the active alias wins the plain lookup")
	})
}

func TestRepository_AmbiguousTickerAcrossExchangesIsNotACoinFlip(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		ticker := uniq("AMB")
		a := seedInstrument(t, repo, ticker, uniq("BBG000J9XR"))
		b := seedInstrument(t, repo, ticker, uniq("BBG000K9XR"))
		_, err := repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: a.ID, AliasType: AliasTicker, AliasValue: ticker, ExchangeCode: "UN", MIC: "XNYS",
		})
		require.NoError(t, err)
		_, err = repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: b.ID, AliasType: AliasTicker, AliasValue: ticker, ExchangeCode: "GY", MIC: "XETR",
		})
		require.NoError(t, err)

		// Unscoped lookup is ambiguous -> explicit error, never an arbitrary pick.
		found, err := repo.FindInstrumentByAlias(ctx, AliasTicker, ticker, "", "")
		assert.ErrorIs(t, err, ErrAliasConflict)
		assert.Nil(t, found)

		// Scoping resolves it.
		found, err = repo.FindInstrumentByAlias(ctx, AliasTicker, ticker, "GY", "")
		require.NoError(t, err)
		require.NotNil(t, found)
		assert.Equal(t, b.ID, found.ID)
	})
}

func TestRepository_AliasValidationAndForeignKey(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		in := seedInstrument(t, repo, uniq("VAL"), uniq("BBG000L9XR"))

		_, err := repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: in.ID, AliasType: AliasType("nonsense"), AliasValue: "X",
		})
		assert.ErrorIs(t, err, ErrInvalidAlias)

		_, err = repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: in.ID, AliasType: AliasTicker, AliasValue: "   ",
		})
		assert.ErrorIs(t, err, ErrInvalidAlias)

		_, err = repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: uuid.NewString(), AliasType: AliasTicker, AliasValue: uniq("ORPH"),
		})
		assert.ErrorIs(t, err, ErrInstrumentNotFound)
	})
}

func TestRepository_FindActiveAliasAndUpdateSymbol(t *testing.T) {
	forEachRepo(t, func(t *testing.T, repo Repository) {
		ctx := context.Background()
		ticker := uniq("ACT")
		in := seedInstrument(t, repo, ticker, uniq("BBG000M9XR"))
		created, err := repo.CreateAlias(ctx, InstrumentAlias{
			InstrumentID: in.ID, AliasType: AliasTicker, AliasValue: ticker, ExchangeCode: "UN",
		})
		require.NoError(t, err)

		active, err := repo.FindActiveAlias(ctx, in.ID, AliasTicker)
		require.NoError(t, err)
		require.NotNil(t, active)
		assert.Equal(t, created.ID, active.ID)

		_, err = repo.FindActiveAlias(ctx, in.ID, AliasISIN)
		assert.ErrorIs(t, err, ErrAliasNotFound)

		newSymbol := uniq("NEW")
		when := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
		require.NoError(t, repo.UpdateInstrumentSymbol(ctx, in.ID, newSymbol, when))
		got, err := repo.GetInstrumentByID(ctx, in.ID)
		require.NoError(t, err)
		assert.Equal(t, newSymbol, got.CurrentSymbol)
		assert.Equal(t, in.FIGI, got.FIGI, "a rename must not touch the FIGI")

		assert.ErrorIs(t, repo.UpdateInstrumentSymbol(ctx, uuid.NewString(), "X", when), ErrInstrumentNotFound)
	})
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + 32
		}
	}
	return string(out)
}
