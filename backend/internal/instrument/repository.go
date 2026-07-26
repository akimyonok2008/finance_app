package instrument

import (
	"context"
	"strings"
	"time"
)

// Repository is the persistence boundary for the identity register. Every
// method takes the caller's context so cancellation and deadlines propagate —
// none fabricates a context.Background().
//
// Lookup semantics (shared by every implementation, enforced by the parity
// suite in repository_contract_test.go):
//
//   - FindInstrumentByAlias matches only ACTIVE aliases (valid_to IS NULL).
//     A closed alias is historical and is never returned by the plain lookup;
//     use FindInstrumentByAliasAsOf to resolve an identifier as of a past time.
//   - exchangeCode and mic are optional filters. Empty string means "any": a
//     ticker-only query matches an alias regardless of its exchange. When
//     supplied they must match exactly. A ticker+exchange query is therefore
//     strictly more precise than a ticker-only one.
//   - When a filterless query matches several instruments the result is
//     ambiguous and (nil, ErrAliasConflict) is returned rather than an
//     arbitrary pick.
type Repository interface {
	CreateInstrument(ctx context.Context, in Instrument) (Instrument, error)
	GetInstrumentByID(ctx context.Context, id string) (Instrument, error)
	UpdateInstrumentSymbol(ctx context.Context, id, currentSymbol string, updatedAt time.Time) error

	FindInstrumentByAlias(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string) (*Instrument, error)
	// FindInstrumentByAliasAsOf resolves an identifier as it stood at asOf,
	// including aliases that have since been closed. This is what historical
	// holdings ("who held FB in 2019") need.
	FindInstrumentByAliasAsOf(ctx context.Context, aliasType AliasType, aliasValue, exchangeCode, mic string, asOf time.Time) (*Instrument, error)

	CreateAlias(ctx context.Context, alias InstrumentAlias) (InstrumentAlias, error)
	// CloseAlias ends an alias's validity window. Closing an already-closed
	// alias returns ErrAliasNotActive.
	CloseAlias(ctx context.Context, aliasID string, validTo time.Time) error
	ListAliasesForInstrument(ctx context.Context, instrumentID string) ([]InstrumentAlias, error)
	// FindActiveAlias returns the active alias row itself (needed by ticker
	// changes, which must close a specific row).
	FindActiveAlias(ctx context.Context, instrumentID string, aliasType AliasType) (*InstrumentAlias, error)
}

// normalizeAliasValue upper-cases and trims identifier values so lookups are
// case-insensitive in a way both backends agree on. Tickers, FIGIs, ISINs and
// CUSIPs are all case-insensitive alphanumeric identifiers.
func normalizeAliasValue(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

func normalizeScope(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}
