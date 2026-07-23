# Alarvest Backend

Go REST API for authentication, manual portfolio tracking, ranked performance,
benchmark achievements, public strategy discovery, and authenticated social
features.

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
{"symbol":"AAPL","asset_type":"stock","quantity":2}
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

There is no cash account, transaction ledger, partial close, tax-lot model,
deposit/withdrawal flow, dividend, fee, split, or corporate-action processing.

### Owner summary calculation

For active positions:

```text
active_cost    = Σ(baseline price × quantity × FX)
active_current = Σ(current quote × quantity × FX)
unrealized     = active_current - active_cost
```

For closed positions, stored cost and realized gain are retained:

```text
total_cost    = active_cost + closed_cost
current_value = active_current + closed_cost + realized_gain
gain_loss     = unrealized + realized_gain
return_pct    = gain_loss / total_cost × 100
index         = 100 × current_value / total_cost
```

Zero cost produces index `100`. This current-basket index is not account-level
time-weighted return. Adding or resizing positions can dilute or change it.

Archive snapshots store owner-private summary and composition JSON after
mutations on a best-effort basis and from the daily worker. Supported windows
are `1W`, `1M`, `3M`, `6M`, and `1Y`.

## Ranked performance and leaderboards

Migration `0009_ranked_performance_state.sql` stores one state row per
portfolio:

```text
live_index = checkpoint_index × current_active_value / segment_start_value
ranked_return_pct = live_index - 100
```

Before a portfolio mutation, the service values the old and new open sets with
a shared quote/FX cache. It checkpoints the pre-mutation live index and uses the
post-mutation value as the next segment start. First funding activates at 100;
an empty portfolio pauses at the preserved index; refilling resumes from it.
Price or FX failures propagate instead of being converted to zero.

Important consistency limit: the position write and ranked-state checkpoint
are separate repository operations. They are not one PostgreSQL transaction.
If a checkpoint fails after a position write, the API may return an error even
though that position mutation persisted.

`ALL` ranks active users by live persistent index. If Redis contains a global
sorted set, it is served first; otherwise ranking is computed live. Active
valuation failures are logged and skipped. A paused user is skipped during
refresh but an old Redis member is not explicitly removed, so cached results
can temporarily retain a paused user.

Window rankings use the latest ranked-index snapshot at or before the cutoff:

```text
window_return = (current_index / cutoff_index - 1) × 100
display_index = 100 × current_index / cutoff_index
```

Snapshots before the user's tracking epoch are ignored. A user without a
sufficiently old post-epoch snapshot is excluded. There is no interpolation.
The worker records snapshots each refresh tick and currently has no pruning
policy.

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

Evaluation requires archive coverage of at least 90% of the nominal period.
Portfolio return comes from the first and last archive `portfolio_index`; it is
not the persistent ranked index or true TWR. Benchmark recipes are
daily-rebalanced over common dates. Twelve Data history supplies close prices,
not adjusted/total-return data. The Berkshire constituents are a hard-coded
2025 Q1 snapshot.

Awards are idempotent and retain original evidence. Internal per-badge failures
are skipped, with no retry queue. Locked progress is a presentation heuristic:
60% history readiness plus 40% criteria progress, capped at 99%; it is not an
award probability. In mock-price environments, synthetic benchmark history can
produce permanent awards.

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

Profile headline performance uses persistent ranked state when available.
Profile history uses archive index points and may therefore diverge from the
headline. The profile benchmark panel is currently a placeholder.

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

## Current, legacy, and not implemented

- **Current:** portfolio lifecycle, persistent ranked state, timeframe
  leaderboards, benchmark awards, DNA, profiles, Explore, follows/friends/DMs,
  PostgreSQL, and optional Redis/Twelve Data.
- **Legacy/transitional:** archive-index public history and achievements,
  compatibility leaderboard fields, old achievement tables, prototype Yahoo,
  and competition/Arena code with no active frontend route.
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
