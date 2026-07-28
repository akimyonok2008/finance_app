# Alarvest Backend

Go REST API for authentication, manual portfolio tracking, ranked performance,
benchmark achievements, public strategy discovery, and authenticated social
features.

## Portfolio financial semantics

`portfolio_cash_balances` stores non-negative multi-currency cash.
`portfolio_activities` is an immutable owner-private ledger for deposits,
withdrawals, buys, sales, and legacy opening balances. One open logical position
is aggregated per portfolio, symbol, asset type, and quote currency.

All activity commands run through the aggregate coordinator under the portfolio
row lock. Cash, positions, activity, ranked state, versions, audit, and outbox
writes commit or roll back together. Quotes and FX rates are pinned once per
mutation. Buys and sales settle immediately and use weighted-average basis; a
partial sale leaves remaining average basis unchanged. Sale proceeds are cash,
so realized gain is attribution and is not added to current value twice.

**Canonical sale contract.** `gross proceeds = quantity × execution price`,
`net proceeds = gross proceeds − sale fee`, `realized P&L = net proceeds −
allocated cost basis`. Cash is credited the **net** proceeds exactly once, and
realized P&L already has the sale fee netted in exactly once. The sale also
posts a separate `sell_fee` ledger row (grouped with the sale via
`activity_group_id`/`position_episode_id`) purely for fee-reporting and audit
visibility (`PortfolioSummary.fees.transaction_fees_base` still displays it) —
it is never deducted a second time. `PortfolioSummary.fees` discloses this via
`embedded_in_realized_pnl_base`; `ReconcilePortfolioFinancials` subtracts only
`total_fees_base − embedded_in_realized_pnl_base` (the standalone fees) from
economic attribution, so the sale fee is never double-counted.
`SellPreview` and the committed sell share the identical formula.

**Canonical purchase contract.** `gross cost = quantity × execution price`,
`total cash required = gross cost + purchase fee`. A position's **cost basis
includes the purchase price AND the buy fee** — the mirror of the sale contract
above, so basis is what the user actually paid. Across multiple buys within one
episode the basis is the fee-inclusive weighted average. The purchase posts a
separate `buy_fee` ledger row grouped with the buy via `activity_group_id`.

**Execution details and provenance.** A recorded buy or sell accepts an optional
`execution_price`, `fee` and `effective_at`. Omitted values default to the latest
tracked quote, zero, and now respectively, and the defaulting is recorded so an
estimate is never presented as a confirmed broker execution:

| column | values |
| --- | --- |
| `execution_price_source` | `user_recorded`, `provider_estimate`, `legacy_unknown` |
| `fee_source` | `user_recorded`, `default_zero`, `legacy_unknown` |

`occurred_at` is the **effective** instant (when the real-world trade happened);
`recorded_at` is when it was entered into the system. Backdated transactions
have `recorded_at > occurred_at`.

**Automatic purchase funding.** The user records the real-world action; Alarvest
infers the funding consequence. `automatic funding = max(quantity × execution
price + fee − available cash in the instrument's quote currency, 0)`. When the
shortfall is positive a neutral `deposit` activity is recorded for exactly that
amount, in the **instrument's quote currency only** — cash in other currencies is
never auto-converted. It carries metadata `funding_reason=purchase_shortfall`,
`automatic=true`, `linked_purchase_group_id`, and shares one `activity_group_id`
with the buy and the buy fee so the timeline presents them as a single purchase.
This is the DEFAULT: there is no insufficient-cash rejection for the normal case.
The portfolio-level `auto_fund_purchases` preference (default `true`) restores
strict rejection with `ErrInsufficientCash` when set to `false`.

**Ranked treatment of a purchase.** Automatic funding and the allocation of cash
into the instrument are ranked-**neutral** (moving your own money makes you
neither richer nor poorer). Only the fee is return-bearing, and exactly once. The
coordinator implements this with two chained pure checkpoints inside the one
transaction: a neutral checkpoint from `value_before` to `value_after + fee`,
then a return-bearing checkpoint on to `value_after`.

**Automatic closure detection.** There is exactly ONE sell operation; there is no
separate "close position" action. `remaining = available − sold`; when the
remainder falls within the quantity tolerance it is normalized to zero and the
episode closes through the existing closure path. A rebuy after a full sale
always opens a new episode.

**Historical (backdated) transactions — conservative policy only.** Alarvest does
**not** implement deterministic ranked-history replay and does not rebuild ranked
snapshots. `MutationCoordinator.validateHistorical` implements exactly this, and
nothing more:

1. No `effective_at`, or one dated now/later → current, always allowed.
2. A backdated **sale** is validated against the quantity the position actually
   held at `effective_at`, computed by replaying only the ledger's **quantity**
   effects chronologically (buys/opening balances/stock dividends add;
   sales/write-offs subtract; splits restate). This is bookkeeping replay, not
   ranked replay. Insufficient historical quantity is rejected with
   `ErrHistoricalQuantityInsufficient`.
3. The **trusted ranked history boundary** is the start of the UTC day of the
   most recent committed ranked checkpoint (`State.SegmentStartedAt`). Ranked
   snapshots are captured per UTC day, so the current day is not yet snapshotted
   and stays freely writable: anything dated at or after the boundary is applied
   exactly like a current-dated transaction.
4. A transaction dated **before** the boundary falls inside an already-trusted
   snapshot day. It is rejected with `ErrHistoricalRankedConflict` when it would
   change what that snapshot captured: every backdated sale, and every backdated
   buy into an instrument that already has any ledger history. The one allowed
   case is a backdated buy of an instrument with no ledger history at all, which
   opens a genuinely new episode.

Rejections happen before any write, so a refused historical transaction leaves
current state completely untouched.

Legacy create/update/delete handlers remain available to internal correction
tests but are no longer registered as public routes. Product clients use
`/portfolio/buys` and `/portfolio/sells`.

Trade routes:

| route | mutating | idempotency |
| --- | --- | --- |
| `POST /portfolio/buys/preview` | no | n/a |
| `POST /portfolio/buys` | yes | `Idempotency-Key` header (required) |
| `POST /portfolio/sells/preview` | no | n/a |
| `POST /portfolio/sells` | yes | `Idempotency-Key` header (required) |

Both preview routes are strictly read-only: they write no activity, cash,
position, episode, ranked or audit state, and never bump the aggregate version.
Both mutation routes require an `Idempotency-Key`; the key is scoped per
portfolio (`portfolio_mutation_audit (portfolio_id, request_id)`), and a retry
replays the committed result instead of applying a second effect.

**Position episodes.** A position's `id` is its durable episode identity: a
buy merges into the existing open position for that symbol/asset/currency
(same id, weighted-average basis), while a full sale closes the row and a
later rebuy of the same symbol always creates a **new** `positions` row —
never reopening the closed one. `portfolio_activities.position_episode_id` is
a first-class, FK-constrained column (migration `0018`) pointing at the
owning `positions.id`; it was previously derivable only from
`metadata_json`, which is still populated for backward read-compatibility but
is no longer the source of truth. `ListClosedPositions` reads the `positions`
row with `status = 'closed'` for identity/final fields, but its
**realized P&L, cost basis, and return percentage are aggregated from every
activity in the episode** (`Repository.ListActivitiesByPositionEpisode`,
indexed on `(portfolio_id, position_episode_id, occurred_at)` from migration
`0018`) — every partial sale plus the final sale/write-off — not just the last
sale's fields on the position row. A partial sale never closes an episode; a
rebuy of the same symbol after closure always starts a **new** episode and
never touches the old one's activities. Closed positions recorded before this
episode-ledger aggregation existed (or otherwise missing complete activity
history) fall back to the position row's own final snapshot rather than
failing or fabricating history.

**Return of capital is not ordinary income.** `RecordIncome` with
`return_of_capital` credits net cash and reduces the paying position's
remaining cost basis (floored at zero; any excess recorded separately), but is
excluded from `PortfolioSummary.income.total_income_base` — it is disclosed
separately as `income.return_of_capital_base` so reconciliation stays
consistent without double-counting the basis-reduction effect as ordinary
income. This is tracking only, never tax advice.

Migration `0019` aligns the Postgres `portfolio_activities` CHECK constraints
with the full `ActivityType` enum (the earlier `0014` migration only
whitelisted the first wave of income/fee types); a table-driven Postgres
integration test enumerates every `ActivityType` constant to prevent future
enum/whitelist drift.

**Corrections.** `POST /portfolio/activities/{id}/correction` reconciles a
user-recorded `deposit` or `withdrawal` to its actual amount by posting a
compensating deposit/withdrawal linked back via
`metadata.correction_of_activity_id` — the original activity is never edited,
and an activity can only be corrected once. `buy`/`sell` corrections are
rejected (`ErrCorrectionNotSupported`): safely reversing a trade would require
reconstructing everything that happened to the position afterward (partial
sells, closures, rebuys), which is out of scope; record an offsetting
buy/sell instead. This is distinct from the income correction path below,
which reconciles automatic/system-generated events.

### Return-bearing vs. neutral activities

Every activity is classified by trusted domain logic (`mutationEffect`) — the API
caller cannot choose it — into one of two performance effects:

- **Neutral** (`ApplyCheckpoint` re-baselines the ranked segment, so the index is
  identical immediately before and after): deposits, withdrawals, buys, sells,
  strategy replacement, and the neutral corporate actions (splits, symbol
  changes).
- **Return-bearing** (`ApplyReturnCheckpoint` preserves the segment baseline, so
  the value change flows straight into the ranked index): dividends,
  distributions, interest income, fees, and write-offs.

A direction guard asserts income never lowers and fees/write-offs never raise the
index; the existing neutrality guard still protects the neutral path.

### Income — automatic, provider-driven (`internal/income`)

**Users never enter ordinary income, and there is no generic income-creation
endpoint** — an unrestricted one could be abused to fabricate cash and ranked
returns. Routine investment income is detected, calculated, and applied
automatically by a background pipeline (`internal/income`) that is
provider-independent (a neutral `IncomeEventProvider` interface; a
`ManualDevelopmentProvider` needs no API keys). The pipeline: ingests provider
income events → normalizes them (idempotent on `(provider, provider_event_id)`) →
discovers affected portfolios → computes **historical** eligibility from the
immutable ledger (not current quantity) → credits cash (or reinvests) on the
**payment date** through the atomic coordinator → records a read-only timeline
entry. `(income_event_id, portfolio_id)` uniqueness + transactional claiming mean
two workers never credit the same event twice.

Endpoints are read-only (`GET /portfolio/income-events`,
`GET /portfolio/income-events/{id}`) plus a constrained, account-specific
correction (`POST /portfolio/income-events/{id}/correction`) that references an
existing event and reconciles it to the actual broker outcome via a compensating
adjustment — never arbitrary income.

Accounting semantics (each type has explicit eligibility + rules):

- **Ordinary income** (cash/special dividend, ETF/mutual-fund distribution,
  bond coupon, capital-gains distribution, interest, staking): adds cash in the
  **payment currency** (never auto-converted to base) and **raises** the ranked
  index once. **Gross, withholding, and net are stored distinctly**
  (`net = gross − withholding − fees`); the default policy credits *expected
  gross* marked estimated, correctable later from broker data. An optional
  withholding profile (default / per-type / per-symbol) is tracking, **not tax
  advice**.
- **Reinvested dividend**: two grouped, atomically-committed legs — the income
  leg (return-bearing) and a neutral buy leg that reinvests the **net** income;
  the price follows an explicit hierarchy (broker-reported → provider DRIP →
  market close on payment date → later reliable price), with the estimated
  quantity/method recorded. Income is counted exactly once.
- **Return of capital**: *not* ordinary income. Adds cash **and reduces the
  position's remaining cost basis** by the same amount (floored at zero; excess
  recorded separately). The cash credit is return-bearing once (its counterpart
  is the ex-date price drop); the distinguishing effect is the basis reduction,
  which changes future realized P&L, not the index. Tracking, not tax advice.
- **Stock dividend**: a quantity transformation — quantity × `(1 + num/den)`,
  per-share basis divided by the same factor, total basis and the ranked index
  preserved (**no artificial jump**), no cash.
- **Account-specific** events (cash interest, staking, payment in lieu) require a
  brokerage/account source and are never inferred from a market feed; a
  market-provider cash-interest event stays unresolved (never fabricated).
- Mixed distributions carry components applied independently by their own rule,
  reconciled to the headline amount.

### Fees (`/portfolio/fees`)

`management_fee`, `custody_fee`, and `other_fee` deduct cash from the specified
currency balance and **lower** the ranked index. A fee is rejected
(`ErrInsufficientCashForFee`) if the balance cannot cover it — cash never goes
negative and no debt/margin is created. Buy/sell transaction fees are modelled as
`buy_fee`/`sell_fee` activity types for future linked-fee capture.

### Corporate actions (`/portfolio/corporate-actions`)

- **Stock split / reverse split**: `new_qty = qty × numerator/denominator`, per-
  share baseline divided by the same ratio; total basis, economic value, and
  ranked index are unchanged. These are value-invariant — the ranked checkpoint
  pins `value_after = value_before` so a split-unadjusted quote for the same
  symbol cannot inject a phantom return. A real split-adjusted market feed then
  keeps the position value consistent going forward.
- **Symbol change**: quantity, basis, and ranked performance preserved; the
  position re-tickers and the old symbol is retained in the immutable history.
  Uses an explicit symbol-transition record (not a stable `instrument_id`; see
  limitations).
- **Write-off**: reduces a position to zero, closes it, records the full basis as
  a realized loss, and produces a genuine **negative** ranked return. It is not
  an external withdrawal. Other holdings and cash remain intact.

### Automatic corporate actions

Routine corporate actions are **applied automatically** by a background pipeline
(`internal/corpactions`) — users never enter splits, ticker changes, mergers, or
spin-offs, and there is no user-facing corporate-action or write-off endpoint.
The old `POST /portfolio/corporate-actions` is replaced by a read-only,
owner-private `GET /portfolio/corporate-actions` (plain-language results only:
event type, display symbol, effective date, status, explanation — no amounts,
quantities, basis, or provider payloads).

Pipeline: provider ingestion → normalization (+ fingerprint for corrections) →
stable-instrument matching → affected-portfolio discovery → validation →
automatic transformation through the aggregate coordinator → read-only
system-generated activity. The portfolio domain depends only on the neutral
`CorporateActionProvider` interface and normalized events, never on a specific
data provider. `ManualDevelopmentProvider` needs no API keys so the pipeline runs
offline; `TwelveDataProvider`/`FMPProvider`/`EODHDProvider`/`SECFilingProvider`
adapters are the extension points. Configure with `CORPORATE_ACTIONS_ENABLED`,
`CORPORATE_ACTION_PRIMARY_PROVIDER`, `CORPORATE_ACTION_POLL_INTERVAL`,
`CORPORATE_ACTION_LOOKBACK`, `CORPORATE_ACTION_RETRY_INTERVAL`.

### Selecting a real event-data provider

`INCOME_PRIMARY_PROVIDER` and `CORPORATE_ACTION_PRIMARY_PROVIDER` are no longer
documentation-only: `internal/providerfactory` builds the named adapter at
startup.

| Value | Behaviour |
| --- | --- |
| `manual_dev` (default) | Offline, deterministic, no API keys. Used for local development and the whole test suite. |
| `alpaca` | Real adapter over Alpaca `GET /v1/corporate-actions`. Requires `ALPACA_API_KEY_ID` and `ALPACA_API_SECRET_KEY`. Ingests forward/reverse splits and ticker changes as corporate actions, and cash/special/stock dividends as income. |
| `fmp` | Real adapter over Financial Modeling Prep `/dividends` and `/splits`. Requires `FMP_API_KEY`. Honours `FMP_DAILY_REQUEST_BUDGET`. |

Selecting `alpaca` or `fmp` without the matching credentials is a **startup
error** — the server exits rather than silently falling back to `manual_dev`,
which would make a credential-less deployment look healthy while ingesting
nothing. An unrecognized value fails the same way
(`unsupported income provider: <value>`).

Both real adapters use context-aware requests with a configurable timeout
(`ALPACA_REQUEST_TIMEOUT`, `FMP_REQUEST_TIMEOUT`, default `10s`) and bounded
retry with exponential backoff and jitter for 429/5xx/transport failures,
honouring `Retry-After`; other 4xx responses fail immediately. API secrets are
never logged and never appear in error messages. The FMP adapter refuses to make
a request once `FMP_DAILY_REQUEST_BUDGET` is spent and returns an explicit
budget-exhausted error, which is deliberately distinguishable from an empty
result.

`DATA_USAGE_MODE` (default `personal`) documents intent only. These provider
integrations are for **personal / local / internal use**; free-tier provider
terms do not cover public commercial redistribution of their data.

Fallback chaining (`INCOME_FALLBACK_PROVIDER`,
`CORPORATE_ACTION_FALLBACK_PROVIDER`) is **constructed but not yet chained**:
the factory validates and builds the named fallback adapter, but the services
still consume a single provider. Wiring the degradation policy is deferred.

**Applied automatically** (verified, effective, complete terms, stable match):
stock splits and reverse splits (quantity ×num/den, per-share basis ÷ the same
ratio — both are adjusted, not just the price; total basis, value, and ranked
index preserved) and straightforward ticker changes (quantity/basis/history/
ranked performance preserved, quote lookup re-tickered). **Left pending /
unresolved** for incomplete or complex data: mergers (stock/cash/mixed),
spin-offs, and delistings — the position stays visible and is retried when better
data arrives; a delisting **never** silently zeros a position. **Never inferred**
from price moves. Terminal cancellations with zero consideration are a genuine
loss (not neutralized) — reserved for the internal/admin path.

Eligibility uses the effective date: a holder who acquired the position after the
action took effect is skipped, so a buyer after a split is not split again.
Idempotency is enforced twice — a deterministic per-(event, portfolio) request id
through the coordinator, and a durable `corporate_action_applications` row with a
`(corporate_action_id, portfolio_id)` primary key — so multiple workers never
apply an event twice. Events are unique on `(provider, provider_event_id)`;
provider corrections update the pending event (or flag an applied event
`superseded` for an explicit correction workflow — never a silent rewrite).

This is user-reported tracking, not brokerage execution. The application does not
execute trades, connect to a brokerage, provide tax reporting, or give investment
advice. There is no margin, short selling, tax-lot accounting, options/futures,
automatic dividend forecasting, or automatic corporate-action detection. All
income, fees, and corporate actions are user-entered; provenance is recorded
(`user_reported`) and never claimed as provider-verified.

### No dividend double counting

The **user portfolio** is valued from observable live market quotes plus explicit
recorded cash income (`positions market value + cash balances`). It never uses
adjusted or total-return price series, so a recorded cash dividend is counted
exactly once. Adjusted/total-return series exist only on the **benchmark** side
(see the benchmark integrity section) and are never injected into live portfolio
valuation. Income and fees are attribution metrics, not extra assets or
liabilities added again to current value; sale/dividend proceeds already exist as
cash, and fees already reduced cash.

## Runtime

- Go 1.26, chi router, JWT HS256 authentication
- In-memory repositories by default; PostgreSQL through pgx for persistence
- Optional Redis cache for global and weekly leaderboards
- Deterministic mock market data by default; optional Twelve Data or prototype
  Yahoo adapters
- Static mock FX conversion for USD, TRY, EUR, and GBP
- Embedded SQL migrations applied at PostgreSQL startup

The main wiring is in `cmd/api/main.go`; HTTP routes are registered in
`internal/server/router.go`.

## Start

```bash
go run ./cmd/api
```

The default configuration needs no infrastructure:

```env
PORT=8080
STORAGE_PROVIDER=memory
PRICE_PROVIDER=mock
BASE_CURRENCY=USD
ENABLE_BACKGROUND_WORKERS=false
```

For persistent development:

```bash
docker compose up -d
STORAGE_PROVIDER=postgres \
DATABASE_URL="postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable" \
REDIS_URL="redis://localhost:6379/0" \
go run ./cmd/api
```

The repository-root Compose file runs API, frontend, PostgreSQL, and Redis.
This directory's Compose file runs infrastructure only.

## Configuration

See `.env.example` for every setting. Important groups:

| Setting | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | HTTP port |
| `JWT_SECRET` | development secret | HS256 signing key; change outside local development |
| `JWT_EXPIRY_HOURS` | `24` | App-token lifetime; no refresh token exists |
| `STORAGE_PROVIDER` | `memory` | `memory` or `postgres` |
| `DATABASE_URL` | local URL | Required by PostgreSQL mode |
| `REDIS_URL` | empty | Optional leaderboard cache |
| `PRICE_PROVIDER` | `mock` | `mock`, `twelvedata`, or prototype `yahoo` |
| `ENABLE_REAL_MARKET_DATA` | `false` | Must also be true for Twelve Data |
| `BENCHMARK_AWARD_MODE` | `verified_only` | `disabled`, `demo`, or `verified_only` — permanent benchmark award policy |
| `PRICE_CACHE_TTL_SECONDS` | `300` | Legacy/provider cache setting |
| `QUOTE_CACHE_TTL_SECONDS` | `600` | Market-data quote-cache lifetime |
| `QUOTE_STALE_AFTER_SECONDS` | `900` | Staleness label threshold |
| `QUOTE_ALLOW_STALE_ON_PROVIDER_ERROR` | `true` | Return cached data after provider failure |
| `ENABLE_BACKGROUND_WORKERS` | code default `false` | Competition/ranking/archive maintenance |
| `ENABLE_QUOTE_REFRESH_WORKER` | `false` | Active-symbol quote refresh |
| `GOOGLE_AUTH_ENABLED` | `false` | Enables Google ID-token login |

`.env.example` enables background workers for the persistent example even
though the code default is disabled.

## HTTP contract

Public:

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/` | Minimal local development page |
| GET | `/health` | Process health |
| GET | `/ready` | Dependency readiness and provider metadata |
| POST | `/auth/register` | Email/password registration |
| POST | `/auth/login` | Email/password login |
| POST | `/auth/google` | Optional Google credential exchange |

All routes below require `Authorization: Bearer <jwt>`.

| Area | Routes |
| --- | --- |
| Session | `GET /me` |
| Market data | `GET /instruments/search?q=...`, `GET /quotes?symbols=...` |
| Portfolio | `GET /portfolio`, `GET /portfolio/summary`, `GET /portfolio/archives?timeframe=1M`, `GET/POST /portfolio/positions`, `PUT/DELETE /portfolio/positions/{id}`, `GET /portfolio/positions/closed`, `POST /portfolio/positions/{id}/close` |
| Leaderboard | `GET /leaderboard?timeframe=ALL`, `GET /leaderboard/me?timeframe=ALL` |
| Competitions | `GET /competitions`, `POST /competitions/{id}/join`, `GET /competitions/{id}/me`, `GET /competitions/{id}/leaderboard` |
| Achievements | `GET /achievements`, `POST /achievements/evaluate` |
| Profiles | `GET/PATCH /profiles/me`, `GET /profiles/explore`, `GET /profiles/{handle}` |
| Strategy | `POST /strategy-portfolio/copy-preview`, `POST /strategy-portfolio/copy-from-profile`, `POST /strategy-portfolio/compare-profile` |
| Social | `POST/DELETE /social/follows/{handle}`, `GET /social/follow-state/{handle}`, `GET /social/following`, `GET /social/followers`, `GET /social/friends` |
| Messages | `GET/POST /dm/conversations`, `GET/POST /dm/conversations/{id}/messages` |

Apple verifier/service code exists but `/auth/apple` is not registered.

### Decimal JSON contract

Authoritative financial inputs (money, quantity, price, FX, index, NAV, and
weight) are decoded from their JSON token text into exact decimals. During the
compatibility window, clients may send either a JSON string (`"100.25"`) or a
bare JSON number (`100.25`); neither path converts through `float64`.
Exponent notation, NaN/Infinity, locale-formatted values such as `"1,234.56"`,
oversized input, and values beyond the field's precision policy are rejected as
invalid request bodies. Financial responses use canonical decimal strings.
Percentages used only for charts, coverage, risk statistics, or other explicitly
non-authoritative presentation may remain JSON numbers.

## Authentication

Password registration requires at least eight characters and stores a bcrypt
hash. Login issues an HS256 JWT. Protected middleware also confirms that the
token's user still exists; therefore tokens from in-memory mode stop working
after restart. There is no refresh, revocation, session list, or account
recovery flow.

Google login is optional. It verifies issuer, signature, expiry, and audience
against Google JWKS, then links verified emails or stores a `(provider, sub)`
identity. Configure the same client ID in the frontend.

## Portfolio model

Each user has one lazily created USD-default portfolio. A position accepts:

```json
{"symbol":"AAPL","asset_type":"stock","quantity":"2"}
```

Supported asset types are `stock`, `etf`, and `crypto`. The service normalizes
and validates the symbol, fetches a quote, verifies FX support, and stores the
quote as the immutable position baseline. Users do not provide a historical
buy price. Quantity is the only editable field.

- **Delete** physically removes an active entry and is intended for correction.
- **Close** records close price/date and realized base-currency return, retains
  the position as history, and does not execute a trade.
- **Copy weights** replaces all open positions with quantities derived from a
  neutral 100-base-unit notional. Closed history remains.

### Owner summary calculation

The private summary separates ranked performance from accounting metrics:

```text
ranked_return_percentage       = ranked_index - 100
open_holdings_market_value     = Σ(open quantity × current quote × FX)
cash_value                     = Σ(cash balance × FX)
current_portfolio_value        = open_holdings_market_value + cash_value
open_holdings_cost_basis       = Σ(remaining quantity × weighted-average basis × FX)
unrealized_pnl                 = open_holdings_market_value - open_holdings_cost_basis
realized_pnl                   = Σ(net sale proceeds - allocated basis)
```

For a complete deposit-funded ledger:

```text
total_economic_pnl = current_portfolio_value + withdrawals - deposits
```

Income and fees are reported separately for attribution; their asset effect is
already present in cash or reinvested shares. Sale proceeds, realized P&L,
income, and fees are therefore never counted twice. The reconciliation service
checks the valuation, unrealized-P&L, ranked-return, and complete-ledger
attribution identities within a one-cent tolerance.

The persistent ranked index is a chain-linked, capital-flow-neutral competitive
metric. It is never multiplied by cost basis or portfolio value to produce
dollar P&L. Zero open basis makes holdings return not applicable, while a
cash-only portfolio still has a valid current value. Portfolios with activity
predating the ledger expose `legacy_estimate` or `insufficient_history` and do
not fabricate exact total P&L.

Archive snapshots store owner-private summary and composition JSON after
mutations on a best-effort basis and from the daily worker. Supported windows
are `1W`, `1M`, `3M`, `6M`, and `1Y`.

## Ranked performance and leaderboards

Migration `0009_ranked_performance_state.sql` stores one state row per
portfolio, and `0010_portfolio_mutation_aggregate.sql` adds the aggregate
invariants that back the transactional write path:

- `portfolios.version` (aggregate version, `CHECK (version > 0)`)
- `UNIQUE (user_id)` on `portfolios` — one default portfolio per user, with a
  safe backfill that merges any legacy duplicates onto the oldest row
- `positions` checks: positive quantity, and open/closed close-field consistency
- `portfolio_archive_snapshots.captured_date` (generated) + a unique index on
  `(portfolio_id, captured_date)` — one daily snapshot per portfolio per UTC day
- `portfolio_mutation_audit` — privacy-safe mutation log with a unique
  `request_id` for idempotency
- `portfolio_outbox` — transactional outbox with a `claimed_at` lease

Migrations are forward-only and idempotent; no user data is dropped. Operational
recovery notes are in the migration header.

Migration `0024_financial_precision_numeric.sql` widens authoritative financial
columns to the current decimal policy and converts legacy quote/index
`DOUBLE PRECISION` columns to `NUMERIC` after explicitly rejecting
NaN/Infinity. That conversion preserves the value PostgreSQL currently holds;
it cannot reconstruct decimal digits already lost when historical values were
first stored as binary floating point.

The ranked formula:

```text
live_index = checkpoint_index × current_active_value / segment_start_value
ranked_return_pct = live_index - 100
```

Before a portfolio mutation, the coordinator values the old and new open sets
against one pinned quote/FX observation. It checkpoints the pre-mutation live
index and uses the post-mutation value as the next segment start. First funding
activates at 100; an empty portfolio pauses at the preserved index; refilling
resumes from it. Price or FX failures propagate instead of being converted to
zero, and NaN/Inf/zero/negative prices and rates are rejected outright.

### The portfolio mutation aggregate

Positions and ranked-performance state are ONE transactional aggregate. Every
production mutation — add, resize, delete, close, strategy replacement — goes
through `portfolio.MutationCoordinator.Apply`, which commits the position write,
the ranked checkpoint, the aggregate version bump, the audit record and the
outbox event together, or rolls all of them back.

```text
validate (no I/O)
  → pin quotes + FX outside the lock
  → BEGIN
  → SELECT ... FROM portfolios WHERE user_id = $1 FOR UPDATE
  → reload positions + ranked state through the transaction
  → coverage check; on a miss ROLLBACK and rebuild every input
  → value before → apply in memory → value after
  → pure checkpoint + neutrality assertion
  → write positions, ranked state, version, audit, outbox
  → COMMIT
  → (after commit) caches and derived projections
```

**Isolation and locking.** PostgreSQL default READ COMMITTED. Serializability
for a single portfolio comes from the explicit row lock, not the isolation
level, so no application-level input is ever reused across a concurrent change.
Different portfolios never contend. The in-memory store mirrors this with a
per-portfolio mutex plus staged writes published atomically on commit.

**No stale retries.** If the locked position set contains a symbol the pinned
valuation does not cover, the transaction is abandoned and *all* inputs are
rebuilt from current state (bounded to 3 attempts, then `409 Conflict`). A retry
never reuses a previous portfolio set or valuation.

**No bypass.** `performance.Service` is read-only; ranked state is written only
through `portfolio.AggregateTx`. There is deliberately no public checkpoint API
that could reintroduce a non-atomic mutation.

**Idempotency.** Mutations accept an `Idempotency-Key` header. The key is stored
with the audit record under a unique index, so a retried request (mobile
timeout, double-click, proxy retry) replays the original committed result
instead of applying the mutation twice.

**Derived work.** Private archive snapshots, ranked lifecycle snapshots, and
leaderboard cache sync run *after* commit, driven by durable
`portfolio.mutated` outbox events claimed with `FOR UPDATE SKIP LOCKED` plus a
`claimed_at` lease. A failing projection retries; it can never fail or roll
back a mutation. The cache projector rereads current ranked state instead of
trusting the event's historical status or score, so an old pause event delivered
after a newer resume cannot incorrectly remove an active user.

`ALL` ranks active users by live persistent index. If Redis contains a global
sorted set, it is served first; otherwise ranking is computed live. Active
valuation failures are logged and skipped. A paused user is explicitly REMOVED
from the Redis sorted set with idempotent `ZREM` (on the outbox event and again
during refresh); a resume uses `ZADD` with the continued ranked return. The
periodic refresh uses explicit member reconciliation: it enumerates the complete
sorted set, upserts active users, removes paused users, and removes members no
longer returned by the user repository. A temporary valuation failure preserves
that user's prior cached score until a later successful pass instead of treating
them as deleted. Cached reads also clean up missing-user members defensively.

This is explicit reconciliation rather than a temporary-key generation swap.
Concurrent operations are idempotent, but readers can observe a pass while it is
in progress. Redis is a cache only: an outage never rolls back or corrupts
committed portfolio state, failed outbox work remains retryable, and live
database ranking remains the source-of-truth fallback.

Migration `0011_ranked_performance_snapshots.sql` creates the canonical
high-precision ranked history used by timeframe leaderboards, achievements,
achievement progress, and public profile charts. It stores only ranked index
and lifecycle/quality metadata—never quantities, cost basis, segment value, or
absolute portfolio value.

The ranked snapshot policy is:

- configurable intraday buckets, `4h` by default;
- one canonical point per UTC date, retained permanently;
- immediate transition points for epoch initialization, pause, and resume;
- stale/partial/invalid observations are recorded as untrusted diagnostics and
  cannot be permanent-award boundaries;
- intraday retention defaults to 120 days; compaction requires a complete daily
  point and preserves transition and evidence-protected points.

Partial unique indexes enforce one intraday bucket and one daily point per
portfolio and tracking epoch. Inserts are idempotent across instances and may
replace an untrusted point with a complete point in the same bucket. Evaluation
work is claimed with `FOR UPDATE SKIP LOCKED`; permanent award uniqueness remains
database-enforced.

Window rankings use the latest complete ranked-index snapshot at or before the
cutoff:

```text
window_return = (current_index / cutoff_index - 1) × 100
display_index = 100 × current_index / cutoff_index
```

Snapshots before the user's tracking epoch or from another epoch are ignored. A
user without a sufficiently old post-epoch snapshot is excluded. There is no
interpolation.

Personal standing is calculated live. `previous_rank` and `rank_delta` are not
historically populated; `best_rank` currently mirrors the present rank.

## Weekly competitions

Competition APIs create an ISO-week sprint (Monday 00:00 UTC). Joining requires
a nonempty portfolio and freezes symbols, quantities, quote prices, and starting
base value. Later portfolio edits do not affect the entry.

```text
sprint_return = (current frozen-composition value / starting value - 1) × 100
sprint_index  = 100 × current value / starting value
```

The active frontend does not expose sprint screens. Completed standings are not
finalized; querying them later reprices the frozen composition with then-current
quotes. Provider failures skip an entrant.

## Achievements

The current catalogue is twenty code-defined, permanent benchmark badges:

- `30D`: cash/SGOV, SPY, GLD, and 60/40 recipes
- `90D`: VOO, VT, SCHD, 60/40, inflation shield, and commodity basket
- `6M`: Berkshire, All Weather, Munger, Graham, Lynch, and Swensen recipes
- `1Y`: Soros, quant, and Druckenmiller recipes

Evaluation loads each period window once and calculates:

```text
portfolio_return = (ending_ranked_index / starting_ranked_index - 1) × 100
```

The start is the latest complete point at or before the target boundary, or the
first complete point shortly after it when within the configured tolerance.
The ending point must be complete and no older than the configured freshness
limit. Both points must belong to one tracking epoch. The default eligibility
threshold is 90% for nominal history, active time, and trusted-data coverage;
this prevents a user from freezing a gain while paused through a benchmark
decline. The exact selected timestamps define both portfolio and benchmark
intervals. `6M` and `1Y` use calendar-aware subtraction.

Progress reports `building_history`, `insufficient_active_coverage`,
`insufficient_trusted_data`, `eligible_but_rule_not_met`, and — distinguishing
each benchmark integrity failure rather than a single generic state —
`benchmark_unadjusted`, `benchmark_stale`, `benchmark_unverified`,
`recipe_version_unavailable`, and `benchmark_unavailable`. A user may beat the
benchmark mathematically while the data is not trusted enough to award: in that
case progress shows the edge but explains that verified total-return data is
unavailable. Its 60% coverage plus 40% criteria bar is a presentation heuristic
capped at 99%, not an award probability.

### Benchmark data integrity (raw vs. total return)

A raw price-return series is **not** total return: it ignores reinvested
dividends and mishandles splits. A 2-for-1 split halves the raw close (a false
−50%); a dividend-paying ETF's raw return understates the holder's actual
return. Verified benchmark awards must therefore use adjusted-close or
total-return data.

Price series now carry provenance (`BenchmarkDataMetadata`): provider, price
type (`raw_close` / `adjusted_close` / `total_return`), whether dividends and
splits are incorporated, synthetic/stale flags, and a `DataQuality`
(`verified` > `acceptable` > `stale`/`synthetic` > `incomplete`/`invalid`). The
engine validates every component series (≥2 sorted, unique, finite, positive
points; price type meets the requirement; corporate-action handling known) and
derives the **weakest-leg** aggregate quality. `CalculateReturn` returns a
`BenchmarkReturnResult` — the number plus effective window, recipe version,
aggregate provenance, and a deterministic SHA-256 fingerprint of the inputs —
not a bare float.

Provider capabilities: the bundled **mock** provider is synthetic
(`is_synthetic`, quality `synthetic`) — preview/demo only. The configured
**Twelve Data** `/time_series` adapter returns **raw closes**, so it is labelled
`raw_close` with unknown corporate actions and quality `acceptable`; it can
power previews but **fails closed** for verified awards
(`ErrAdjustedDataUnavailable`). To award verified benchmark badges, a provider
must supply a native adjusted/total-return series, or raw prices plus dividend
and split events from which `BuildTotalReturnSeries` constructs one explicitly.
Raw close is never silently relabelled as adjusted close.

### Recipe versioning and no-look-ahead

Every recipe has an immutable version. Static recipes are code-defined as
`<ID>_v1` (e.g. `SPY_v1`, `BALANCED_60_40_v1`, `ALL_WEATHER_v1`). Dynamic
recipes (`BUFFETT_13F`) have one immutable version per authoritative SEC filing.
Selection is deterministic and avoids look-ahead: for an evaluation over
`[start, end]` the engine picks the newest version whose `publicly_known_at <=
start` and uses it for the whole window. Evaluating before any version was filed
returns `ErrRecipeVersionUnavailable` — the badge is not awarded. Versions are
also stored durably in `benchmark_recipe_versions` (migration `0013`) with
uniqueness, valid-range, and source-metadata/coverage constraints.

The Berkshire baskets are transcribed from the SEC EDGAR 13F-HR information
tables (CIK 0001067983), market-value weighted over the reliably-mapped
large-cap positions and renormalized to 1. Two versions ship:

| Version | Report period | Filed / public | Accession | Mapped coverage |
| --- | --- | --- | --- | --- |
| `BUFFETT_13F_2025Q1` | 2025-03-31 | 2025-05-15 | 0000950123-25-005701 | 92.67% |
| `BUFFETT_13F_2026Q1` | 2026-03-31 | 2026-05-15 | 0001193125-26-226661 | 96.81% |

Older versions are preserved (not overwritten) so historical evaluations remain
reproducible. Positions that cannot be mapped unambiguously to a supported
ticker (CUSIP ambiguity, unsupported securities) are excluded; the mapped
coverage is recorded and must stay at or above the documented 90% threshold, or
the version fails closed. Berkshire benchmarks are labelled proxy portfolios
inspired by the disclosed holdings, not replicas of Berkshire's own return.

### Award verification and mock/demo restrictions

`BENCHMARK_AWARD_MODE` (default `verified_only`) is the single award authority
and is independent of whether an API key exists:

- `disabled` — catalogue and progress only; no awards written.
- `demo` — synthetic/mock data may create awards, persistently marked `demo`;
  they never count as verified.
- `verified_only` — only real, adjusted/total-return, fresh, complete data with
  a valid recipe version produces an award, stamped `verified`. Synthetic, raw,
  stale, or incomplete data fails closed.

A permanent verified award requires: mode allows awards **and** aggregate
quality is `verified` **and** data is not synthetic/stale **and** all legs are
adjusted/total-return **and** the recipe version is valid. The `rules` engine
deciding the user beat the benchmark is necessary but not sufficient — the
`AwardEligibilityPolicy` independently decides whether the data is trustworthy.
At startup, `verified_only` with a mock-only provider is downgraded to
`disabled` with a prominent warning (and logged as an error in `production`), so
the UI never implies verified evaluation is active when it is not.

Benchmark recipes are daily-rebalanced (`daily_target_weight`) over dates common
to every component; weekends/holidays use the nearest common observation, and
missing components or insufficient common dates fail closed. The rebalancing
policy is recorded in recipe and evidence metadata; the Berkshire 13F basket is
a daily-rebalanced proxy of a disclosed buy-and-hold basket, labelled as such.

New awards store privacy-safe evidence (`evidence_version` 2, model
`ranked_snapshot_v1`): ranked epoch, boundary indexes/timestamps, coverage,
snapshot frequency, plus benchmark provenance — recipe version, verification
status, price methodology, data quality, providers/symbols, filing
accession/period, mapping coverage, and the benchmark input fingerprint. It
contains no monetary values, holdings, quantities, cost basis, or identifiers.
Once written, evidence is immutable — a DB trigger blocks in-place rewrite, so
later provider/recipe/config changes cannot silently alter history; corrections
go through explicit revocation and re-award under a new evidence version.
Boundary points are protected from compaction; evaluation is queued after a
complete snapshot and is retryable; awarded badges are skipped.

Existing awards remain permanent. Migration `0011` labels evidence without a
model as `archive_model_v0`/version `0`; it is surfaced with a legacy marker and
is never represented as ranked-snapshot verified. Legacy archives are not
backfilled into trusted history, so users must accumulate new ranked history
for newly verified long-period badges.

Legacy `achievements` tables remain in migrations, but current awards use
`user_benchmark_achievements`.

## Profiles, privacy, Explore, and strategy tools

Profiles start private with weight sharing disabled. Public profile access
requires `is_public`. Explore additionally requires `show_portfolio_weights`
and a nonempty allocation.

Public DTOs omit quantities, prices, values, cost basis, absolute gain/loss,
email, and internal IDs. With weights enabled they may include active symbols,
asset types, percentage weights, and closed symbols with return percentages.
Exposure, concentration, DNA/style, and aggregate performance-driver signals
are currently returned even when exact weights are hidden; this is a privacy
boundary callers must understand.

Profile headline performance and public history both use persistent ranked
performance. The profile benchmark panel is currently a placeholder. Private
portfolio archive history remains available only to the owner.

Explore search is independent of Featured and Similar collections. `q` matches
public handle, display name, or public symbol; `symbol` is an exact filter.
Featured, Similar, trending holdings, pagination, sorting, and timeframe
ranking are deterministic. Projection work is prototype-scale and can cause
N+1 repository/provider calls.

Compare uses the caller's private weights against an opted-in profile and
returns deterministic overlap, weight gaps, concentration, and canned learning
points. Copy is a template operation, not trading. The copy endpoint accepts
optional client-supplied weights and currently validates them without proving
they match the named source profile.

## Social and messages

Users can initially follow public profiles. Friendship is mutual follow.
Conversation creation and sending require friendship; participants may still
read existing history after an unfollow, but cannot send until friendship is
restored. Messages are trimmed and limited to 1,000 Unicode code points.

The API has no WebSockets, unread counts, notifications, blocking/reporting,
group messages, attachments, or message edit/delete endpoints. Database columns
for edited/deleted messages are not active behavior.

## Market data

`mock` is deterministic and supports the seeded US, crypto, and Turkish demo
symbols. Twelve Data is restricted to US stock/ETF search and quote workflows.
Yahoo is an unofficial prototype and does not support instrument search.

The quote endpoint accepts at most 25 symbols. Cache freshness and stale-on-
provider-error behavior are configurable. Rate limiting is process-local
(default 6 requests/minute and 500/day) with a two-minute 429 backoff; restart
resets counters. Provider delay metadata exists in DTOs but the Twelve Data
adapter does not currently populate it.

Historical prices are exposed internally for benchmark evaluation only.
The FX implementation is a fixed development table, not a live currency feed.

## Instrument identity

A ticker is not an identity: it changes (FB to META), it is reused after a
delisting, and it collides across exchanges. `internal/instrument` adds a stable
one. `instrument_master` holds an immutable internal UUID plus provider
identifiers (FIGI, composite/share-class FIGI, ISIN, CUSIP, CIK);
`instrument_aliases` holds every identifier the instrument is or was known by,
each with a `valid_from`/`valid_to` window. Uniqueness on tickers is scoped to
the ACTIVE window and to `(exchange_code, mic)`, never global.

Note the table is `instrument_master`, not `instruments`: `instruments`
(migration 0004) is the unrelated market-data search catalog and is untouched.

Lookups are active-only by default; `FindInstrumentByAliasAsOf` resolves an
identifier as it stood at a past instant, which is what historical holdings
need. `Resolver.ResolveOrCreate` checks the local register first and calls
OpenFIGI only on a miss. One candidate is `resolved` and creates the instrument
and its aliases; several candidates are `ambiguous` and create nothing (an
arbitrary pick would mint a wrong identity); none is `unresolved`, which is an
expected outcome rather than an error. A rate limit or 5xx surfaces as an error
and is never flattened into "no data". `Resolver.ChangeTicker` closes the old
ticker alias and opens a new one, leaving the FIGI and the internal id alone.

Configuration is `OPENFIGI_ENABLED`, `OPENFIGI_API_KEY` (optional — OpenFIGI
allows unauthenticated low-volume use; never logged), `OPENFIGI_BASE_URL` and
`OPENFIGI_REQUEST_TIMEOUT`.

**Integration status: standalone.** Nothing consumes this layer yet. Positions
and activities carry a nullable `instrument_id` column that is never populated;
the buy path, entitlement discovery and the income/corporate-action pipelines
still key on the ticker string, and legacy rows are not backfilled. OpenFIGI's
`/v3/search` and `/v3/filter` endpoints are not implemented. All of that is
future work.

## Current, legacy, and not implemented

- **Current:** portfolio lifecycle, atomic persistent ranked state, canonical
  ranked history for timeframe leaderboards/achievements/public profiles, DNA,
  Explore, follows/friends/DMs, PostgreSQL, and optional Redis/Twelve Data.
- **Legacy/transitional:** archive-model award evidence, private cost-basis
  archives, compatibility leaderboard fields, old achievement tables,
  prototype Yahoo, and competition/Arena code with no active frontend route.
- **Removed:** Coach routes, services, and UI.
- **Not implemented:** brokerage execution, AI advice, Apple HTTP login, live
  FX, finalized sprint results, token refresh, and production hardening.

## Verification

```bash
go test ./...
go vet ./...
```

PostgreSQL integration tests require:

```bash
DATABASE_URL_TEST="postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable" go test ./...
```

The embedded migration runner holds an application-specific PostgreSQL advisory
lock and applies each migration in its own transaction. Migration files must
therefore contain only operations PostgreSQL permits inside a transaction;
unsupported operations fail with the migration filename and require an explicit
future non-transactional mechanism rather than silently weakening atomicity.
