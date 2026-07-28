# Financial Precision Migration Report

## Scope and result

This pass completes the authoritative-decimal migration started at merge
`b9e1c27`. It preserves the economic contracts in `backend/README.md`; it does
not change cost-basis, cash-flow neutrality, ranked-performance, income,
benchmark, or competition semantics.

The implementation uses `internal/money`, backed by
`shopspring/decimal`. Inputs are parsed from their original decimal text,
arithmetic stays typed and exact, division uses an explicit precision, and
round-half-even quantization occurs only at documented posting, persistence, or
presentation boundaries.

## Converted fields and paths

- Portfolio persistence now scans/writes amounts, quantities, prices, cost
  basis, realized P&L, cash, audit indexes, and outbox indexes directly as
  `money` types. In-memory and PostgreSQL repositories use the same semantics.
- The income pipeline stores and calculates eligible quantity,
  amount-per-unit, gross, withholding, fees, net, reinvestment quantity, and
  corrections with typed decimals. Mixed events, return of capital, stock
  dividends, and reinvestment retain their existing accounting rules.
- Ranked-performance `CheckpointIndex`, `SegmentStartValueBase`,
  `ValueBeforeBase`, `ValueAfterBase`, live index, and return inputs are exact.
  Neutral checkpointing compares exact values and persistence quantizes only
  once.
- Performance-history snapshots and window-return inputs are exact. Window
  return is calculated as `end / start - 1` before conversion for chart/risk
  presentation.
- Leaderboard live ordering compares exact ratios. Deterministic display-name
  tie-breaking is unchanged.
- Competition join snapshots use exact quantity, starting price, starting
  value, and index. Sprint returns and live ordering are exact.
- Redis cache APIs accept `money.Ratio`; conversion to IEEE-754 happens only
  inside the Redis sorted-set write. Redis reads are an unavoidable float
  boundary and are not authoritative.
- The previously completed benchmark engine remains exact for NAV, weights,
  prices, FX, virtual units, returns, rebalancing, and fingerprints.

## PostgreSQL

Migration `0024_financial_precision_numeric.sql` is the next migration after
the repository's actual highest existing migration, `0023`. It:

- rejects NaN and positive/negative Infinity in legacy floating financial
  columns before conversion;
- converts market quote prices to `NUMERIC(36,12)`;
- converts legacy leaderboard indexes to `NUMERIC(38,18)`;
- widens portfolio, cash, activity, ranked-state, ranked-snapshot, audit,
  outbox, archive, competition, and income financial columns to the applicable
  12- or 18-decimal-place policy.

The migration is forward-only and contains only operations permitted by the
existing one-transaction-per-file runner. Existing `NUMERIC` values never pass
through `float64`.

Historical values formerly stored as `DOUBLE PRECISION` retain the binary-float
value PostgreSQL held at migration time. Decimal conversion cannot reconstruct
digits already lost when those rows were originally written.

## HTTP decimal contract

Portfolio mutation inputs now decode directly into `money.Amount`,
`money.Quantity`, and `money.Price`. The decimal unmarshaler accepts both JSON
strings and bare JSON numbers for compatibility, but parses the token text
without a float conversion. It rejects exponent notation, NaN/Infinity, locale
separators, malformed input, and oversized values.

Converted authoritative response fields use the money types' canonical JSON
strings. Position quantity/baseline price, mutation ranked index, cash/activity
amounts, income amounts, and competition financial fields are included.
Charting, coverage, risk statistics, compatibility leaderboard projections,
and explicitly legacy archive/attribution projections remain numeric
presentation fields.

## Static enforcement

`internal/money/financial_float_enforcement_test.go` scans production structs in
`portfolio`, `performance`, `performancehistory`, `benchmark`, `income`,
`leaderboard`, and `competitions`. A new financial-looking `float32`/`float64`
field fails the test unless it is an explicitly documented exception or a
JSON-tagged presentation/result type.

Remaining intentional float categories are:

- chart points, drawdown/weekly-risk series, coverage ratios, percentiles, and
  presentation percentages;
- Redis sorted-set reads and market/FX provider interfaces that only expose
  IEEE-754 values;
- public/profile leaderboard projections after exact ordering;
- benchmark provider-mapping diagnostics and achievement evidence;
- legacy archive and attribution projections whose authoritative source
  ledger/ranked state is decimal;
- legacy position close-history presentation fields;
- non-authoritative strategy weight preferences and preview-only accumulators.

The complete field-by-field exception list and rationale live beside the test,
so additions require an intentional code review change.

## Tests added or strengthened

- Exact portfolio persistence and mutation tests cover fractional quantities,
  partial/full sales, zero closure, allocated basis, fees, realized P&L,
  concurrency, and idempotent identity.
- Exact income tests cover gross/withholding/fees/net identity, reinvestment,
  return of capital, stock dividends, and mixed events.
- Ranked-performance/history tests use exact index helpers and validate
  checkpoint and return invariants.
- Leaderboard and competition tests validate exact inputs, ordering, cache
  boundaries, and presentation results.
- The static core-domain float-field enforcement test prevents regression.

## Verification

Run from `backend/` on 2026-07-28:

| Command | Result |
| --- | --- |
| `gofmt -l .` | PASS, empty output |
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./...` | PASS |
| `go test -race -count=1 ./...` | PASS |

An earlier full run reproduced the documented unrelated
`internal/marketdata TestTwelveDataProvider_BatchQuoteSuccess` map-order flake
exactly (`AAPL` and `MSFT` reversed). The immediate full rerun passed that
package and the complete suite.

PostgreSQL integration tests were not run because `DATABASE_URL_TEST` was not
set in this environment; no reachable test database was supplied. Tests that
require it therefore used their documented skip path.

## Commits

- `f95f53c` — portfolio persistence precision and tests
- `1d4a7a4` — exact income pipeline
- `b020fd8` — exact ranked performance, history, leaderboard, competitions,
  and cache boundaries
- `077b293` — decimal API inputs, PostgreSQL numeric migration, and static
  enforcement
