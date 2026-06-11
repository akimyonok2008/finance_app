# finance_app — Backend

The backend for a gamified real-portfolio tracking application. Users will
eventually enter their real investment portfolios and compare performance
anonymously on leaderboards. The backend is being built milestone by milestone.

- **Milestone 1 — Authentication foundation** ✅
- **Milestone 2 — Manual portfolio entry + performance tracking** ✅
- **Milestone 3 — Simple anonymous leaderboard** ✅
- **Milestone 4 — Weekly sprint competitions + achievements/badges** ✅
- **Milestone 5 — Correctness & fairness hardening** ✅ (symbol validation, FX
  normalization, sprint snapshots, dynamic sprints, achievement re-eval, JWT
  user checks)
- **Phase 3 — Persistence & infrastructure** ✅ (PostgreSQL + migrations, Redis
  leaderboard cache, background jobs, price caching, health/readiness,
  structured logging)

Still **out of scope**: brokerage integrations (Plaid/SnapTrade), WebSockets,
public unauthenticated leaderboards, Google/OAuth login, payments, and advanced
gamification.

## What Milestone 2 adds

An authenticated user can now:

1. Get or auto-create a single **Default Portfolio**.
2. Add manual **positions** (`AAPL`, `MSFT`, `NVDA`, `SPY`, `BTC-USD`,
   `ETH-USD`, `THYAO.IS`, `GARAN.IS`, `ASELS.IS`, …).
3. List, update, and delete their own positions.
4. Fetch the latest price for a symbol through a **`PriceProvider`**.
5. Get a **portfolio summary** with total cost basis, current value, gain/loss,
   gain/loss %, and a portfolio index starting at 100.

All portfolio/price routes require a valid JWT, and users can only ever see or
mutate their own data.

## Architecture

```
cmd/api/main.go            entrypoint: load config, build provider, wire deps
internal/
  auth/                    registration, login, JWT, /me, middleware  (milestone 1)
  portfolio/
    model.go               Portfolio, Position, summaries, asset types
    repository.go          Repository interface + InMemoryRepository
    calculator.go          pure cost-basis / gain-loss / index math
    service.go             validation, ownership enforcement, summary
    handler.go             HTTP adapters + error→status mapping
    errors.go              domain errors
    *_test.go              service / calculator / handler tests
  leaderboard/
    model.go               LeaderboardEntry (privacy-safe, the only shape served)
    service.go             UserProvider + PortfolioSummaryProvider interfaces, ranking
    handler.go             GET /leaderboard
    errors.go              domain errors
    *_test.go              service + handler + privacy tests
  competitions/            weekly sprint: join, status, sprint leaderboard
    model.go repository.go service.go handler.go errors.go *_test.go
  achievements/            badges: seeded catalogue, unlock evaluation, listing
    model.go repository.go service.go handler.go errors.go *_test.go
  fx/                      currency conversion behind FXProvider (USD base)
    model.go provider.go mock_provider.go provider_test.go
  clock/                   Clock interface (RealClock / FixedClock) for testable time
    clock.go
  db/                      pgxpool connection + embedded SQL migrations
    postgres.go migrations/*.sql
  jobs/                    ticker-based background worker (sprint upkeep, cache refresh)
    worker.go jobs_test.go
  prices/
    model.go               Price
    provider.go            PriceProvider interface + NewProvider factory
    mock_provider.go       MockPriceProvider (deterministic, for tests/dev)
    yahoo_provider.go      YahooFinanceProvider (finance-go, PROTOTYPE ONLY)
    handler.go             GET /prices/{symbol}
    provider_test.go       mock provider + factory tests
  httpx/json.go            shared JSON / error response helpers
  server/router.go         chi router wiring (public + JWT-protected groups)
  config/config.go         env-based config with dev defaults
```

**Layering:** `HTTP Handler → Service → Repository`, with prices behind the
`PriceProvider` interface. Business logic never imports `finance-go` — only
`internal/prices/yahoo_provider.go` does. Both the repository and the price
provider are interfaces, so storage and the price feed are swappable without
touching the service or handlers. The repositories already ship in two
implementations — in-memory and PostgreSQL (selected via `STORAGE_PROVIDER`) —
and the Yahoo prototype feed can later be replaced by a licensed market-data
provider the same way.

## Setup

Requires Go 1.22+. PostgreSQL 16 and Redis 7 are optional (the default
`memory` storage needs neither) and are provided via Docker Compose.

```bash
cd backend
go mod download

# Optional: start Postgres + Redis for the durable configuration
docker compose up -d
```

Configuration is read from environment variables (a `.env` is optional — the
app falls back to development defaults). See `.env.example`.

### Environment variables

| Variable                               | Default                  | Description                                                |
| -------------------------------------- | ------------------------ | ---------------------------------------------------------- |
| `APP_ENV`                              | `development`            | Environment label (logged at startup).                     |
| `PORT`                                 | `8080`                   | HTTP listen port.                                          |
| `JWT_SECRET`                           | `dev-secret-change-me`   | HMAC signing secret. **Change in production.**             |
| `JWT_EXPIRY_HOURS`                     | `24`                     | Token lifetime in hours.                                   |
| `STORAGE_PROVIDER`                     | `memory`                 | `memory` or `postgres`.                                    |
| `DATABASE_URL`                         | local `finance_app` DSN  | Postgres connection string (when `postgres`).              |
| `REDIS_URL`                            | *(empty — disabled)*     | e.g. `redis://localhost:6379/0`; enables caches.           |
| `PRICE_PROVIDER`                       | `mock`                   | Price source: `mock` or `yahoo`.                           |
| `PRICE_CACHE_TTL_SECONDS`              | `300`                    | How long quotes are cached.                                |
| `BASE_CURRENCY`                        | `USD`                    | Base currency for normalization.                           |
| `ENABLE_BACKGROUND_WORKERS`            | `false`                  | Run the ticker-based maintenance worker.                   |
| `LEADERBOARD_REFRESH_INTERVAL_SECONDS` | `60`                     | Worker tick interval.                                      |

### Storage providers & migrations

`STORAGE_PROVIDER=memory` (default) keeps everything in process memory — zero
infrastructure, data lost on restart. `STORAGE_PROVIDER=postgres` connects via
`DATABASE_URL` and **runs the embedded SQL migrations automatically on
startup** (tracked in a `schema_migrations` table, idempotent). Migration files
live in `internal/db/migrations/` and cover: users, portfolios, positions,
competitions, competition entries, sprint snapshot positions, achievements,
user achievements, and a price cache table. Both storage backends implement the
exact same repository interfaces, so handlers and services are unchanged.

To reset the local development database:

```bash
docker compose down -v && docker compose up -d   # drops the pgdata volume
```

## Run the tests

```bash
cd backend
go test ./...
```

Unit tests use `MockPriceProvider`, mock FX, miniredis, and in-memory
repositories — no network or infrastructure needed. **Postgres integration
tests** are skipped unless a test database is configured:

```bash
docker compose up -d postgres
DATABASE_URL_TEST="postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable" go test ./...
```

## Run the server

```bash
cd backend

# Zero-infrastructure mode (in-memory storage, mock prices):
go run ./cmd/api

# Durable mode (Postgres + Redis via docker compose):
docker compose up -d
STORAGE_PROVIDER=postgres REDIS_URL=redis://localhost:6379/0 \
  ENABLE_BACKGROUND_WORKERS=true go run ./cmd/api

# Real prototype prices via Yahoo / finance-go:
PRICE_PROVIDER=yahoo go run ./cmd/api
```

## Infrastructure (Phase 3)

### Redis leaderboard cache

Global and sprint rankings are cached in Redis **sorted sets**
(`leaderboard:global`, `leaderboard:competition:{id}`): members are user ids,
scores are performance percentages — no portfolio values, holdings, or emails
are ever written to Redis. Display metadata is joined from the user repository
at read time. The cache is an **optimization, not the source of truth**: if
Redis is disabled, empty, or unavailable, leaderboard endpoints transparently
fall back to live calculation, and the public response shape never changes.

### Background jobs

A simple ticker-based worker (`internal/jobs`) runs when
`ENABLE_BACKGROUND_WORKERS=true`, every `LEADERBOARD_REFRESH_INTERVAL_SECONDS`:

1. **Ensure current weekly sprint exists** (ISO-week derived).
2. **Refresh the global leaderboard cache** (skipped users are logged, never fatal).
3. **Refresh every active sprint leaderboard cache** from join-time snapshots.

Jobs are independent — one failing job never blocks the others. No queue or
broker yet; that's deliberate for this phase.

### Price caching

All price lookups go through `CachedPriceProvider`, which decorates the active
`PriceProvider` with a TTL cache (`PRICE_CACHE_TTL_SECONDS`, default 5 min) —
in-memory by default, Redis-backed when `REDIS_URL` is set. Repeated summary,
sprint, and leaderboard calculations therefore hit the upstream provider at
most once per symbol per TTL window. Cache failures are non-fatal; the provider
is always the fallback.

### Health & readiness

- `GET /health` → `200 {"status":"ok"}` whenever the process is alive.
- `GET /ready` → `200` only when configured dependencies respond (Postgres when
  `STORAGE_PROVIDER=postgres`, Redis when enabled); otherwise `503` with
  per-dependency status, plus `storage_provider` / `price_provider` info.

### Structured logging

The app logs via `log/slog`: startup configuration, dependency connections,
job runs, skipped leaderboard users, and provider failures. Logs **never**
include passwords, hashes, tokens, holdings, quantities, or portfolio values.

## Price providers

### MockPriceProvider

Deterministic, offline, used by all tests and ideal for local development.
Seeded quotes:

| Symbol     | Price    | Currency |
| ---------- | -------- | -------- |
| AAPL       | 195.00   | USD      |
| MSFT       | 430.00   | USD      |
| NVDA       | 130.00   | USD      |
| SPY        | 540.00   | USD      |
| BTC-USD    | 68000.00 | USD      |
| ETH-USD    | 3500.00  | USD      |
| THYAO.IS   | 295.00   | TRY      |
| GARAN.IS   | 120.00   | TRY      |
| ASELS.IS   | 85.00    | TRY      |

Unknown symbols return `ErrPriceUnavailable`.

### YahooFinanceProvider

Fetches real prices via [`finance-go`](https://github.com/piquette/finance-go).

> ⚠️ **PROTOTYPE ONLY.** `finance-go` reads an unofficial, unauthenticated
> Yahoo endpoint with no SLA, rate limits, or licensing guarantees. It is fine
> for a prototype but **must be replaced** by a production-grade, licensed
> market-data provider (e.g. Twelve Data, Finnhub, Polygon) before any real
> launch. Because all logic depends on the `PriceProvider` interface, swapping
> it is a single new file plus a `NewProvider` case.

## Portfolio calculation

Per position:

```
cost_basis           = quantity * average_buy_price
current_value        = quantity * current_price
gain_loss            = current_value - cost_basis
gain_loss_percentage = gain_loss / cost_basis * 100
```

Whole portfolio:

```
total_cost_basis     = sum(cost_basis)
current_value        = sum(current_value)
gain_loss            = current_value - total_cost_basis
gain_loss_percentage = gain_loss / total_cost_basis * 100
portfolio_index      = 100 * current_value / total_cost_basis
```

If total cost basis is zero (e.g. empty portfolio): `gain_loss_percentage = 0`
and `portfolio_index = 100`. Percentages and the index are rounded to two
decimals.

## Anonymous leaderboard (milestone 3)

`GET /leaderboard` ranks all users by portfolio performance while preserving
privacy. The product promise:

> **Compete with real portfolio performance without revealing wealth, holdings,
> or identity.**

**It exposes only:** `rank`, `display_name`, `avatar_key`,
`gain_loss_percentage`, `portfolio_index`.

**It never exposes** portfolio value, cost basis, dollar gain/loss, holdings,
symbols, quantities, average buy prices, portfolio id, user id, email, or
password data. The response type (`LeaderboardEntry`) physically has no fields
for those values, and an explicit test serializes the response and asserts none
of the forbidden keys appear.

**How it is calculated** (live, on each request):

```
GET /leaderboard
  -> list all users
  -> for each user, compute their portfolio summary (existing PortfolioService)
  -> keep only gain_loss_percentage and portfolio_index
  -> sort by gain_loss_percentage DESC, ties broken by display_name ASC
  -> assign sequential ranks (1, 2, 3, ...)
```

Edge cases:

- **Empty portfolio** → `gain_loss_percentage = 0`, `portfolio_index = 100`.
- **A user whose summary fails** (e.g. an un-priceable symbol) is **skipped**,
  so one bad portfolio never breaks the whole board. (A later iteration may
  surface partial-error metadata for internal monitoring.)

The service depends on two interfaces (`UserProvider`,
`PortfolioSummaryProvider`), so it is fully testable and can later be backed by
precomputed Redis rankings without changing the handler.

## Weekly sprint competitions (milestone 4)

A time-bound competition where users compete on performance **measured only from
the moment they join** — not lifetime returns.

- `GET /competitions` lists competitions. The prototype seeds **one active
  weekly sprint** (`weekly_2026_24`) on startup.
- `POST /competitions/{id}/join` records the user's **current portfolio value as
  a private starting baseline** and returns `starting_index = 100`. Joining is
  idempotent (a second join returns the existing entry, no duplicate). It is
  rejected with `400` if the portfolio is empty / zero-value, `404` if the
  competition doesn't exist, and `400` if it isn't active.
- `GET /competitions/{id}/me` returns your own `sprint_return_percentage`,
  `sprint_index`, and `current_rank` (or `joined: false` if you haven't joined).
- `GET /competitions/{id}/leaderboard` ranks participants by
  `sprint_return_percentage` desc (ties by `display_name` asc), sequential ranks.

Sprint math, from the private baseline captured at join:

```
sprint_return_percentage = (current_value - starting_value) / starting_value * 100
sprint_index             = 100 * current_value / starting_value
```

**Privacy:** the starting value is stored internally and **never** returned by
any API. The sprint leaderboard exposes only `rank`, `display_name`,
`avatar_key`, `sprint_return_percentage`, `sprint_index` — never starting value,
current value, cost basis, dollar gain/loss, holdings, symbols, quantities,
portfolio id, user id, or email. (Explicit tests serialize the responses and
assert none of those keys appear.)

## Achievements / badges (milestone 4)

`GET /achievements` returns the full badge catalogue with the calling user's
unlock state. Five seeded badges:

| key               | unlocks when…                                    |
| ----------------- | ------------------------------------------------ |
| `first_portfolio` | the user has at least one position               |
| `first_sprint`    | the user joins any competition                   |
| `green_portfolio` | portfolio `gain_loss_percentage > 0`             |
| `index_110`       | portfolio index `>= 110`                         |
| `top_10_sprint`   | the user ranks in the top 10 of a sprint         |

Evaluation is wired at the handler layer (no event bus): after **adding a
position** and after **viewing the summary** (`first_portfolio`,
`green_portfolio`, `index_110`); after **joining a sprint** (`first_sprint`);
and after **sprint status / leaderboard** (`top_10_sprint`). Unlocking is
**idempotent**. Each `AchievementResponse` exposes only `key`, `name`,
`description`, `icon_key`, `unlocked`, `unlocked_at` — no internal ids, and none
of the forbidden portfolio/identity fields.

The competition and achievement modules depend on small interfaces
(`UserProvider`, `PortfolioSummaryProvider`, `PositionProvider`,
`CompetitionRankProvider`, `AchievementEvaluator`), wired with thin adapters in
`main.go`, so neither imports the other (no cycle) and both are backed by
in-memory or PostgreSQL/Redis storage (selected via `STORAGE_PROVIDER`) without
touching handlers.

## Correctness & fairness (milestone 5)

### Symbol validation

Symbols are validated **before a position is saved or updated**, so unpriceable
tickers can never enter the repository (and therefore can never break a summary,
leaderboard, or sprint later). A symbol must:

- be trimmed and upper-cased (`aapl` → `AAPL`),
- be non-empty and ≤ 20 characters,
- contain only `A–Z`, `0–9`, `.`, and `-` (no spaces, `/`, `;`, quotes, emoji, …),
- be **priceable by the active provider**.

A bad symbol on create/update returns **`400`** with
`{"error": "unsupported or unpriceable symbol"}`. With the mock provider, the
priceable symbols are: `AAPL, MSFT, NVDA, SPY, BTC-USD, ETH-USD, THYAO.IS,
GARAN.IS, ASELS.IS`. `GET /prices/{symbol}` returns `400` for a malformed
symbol, `404` for a well-formed but unknown symbol, and `502` only for an
unexpected provider failure.

### Base-currency (USD) normalization

Mixed-currency portfolios are normalized to a single **base currency (USD)**
via an `FXProvider` before any totals are computed, so summing a USD position
and a TRY position is financially meaningful. Prototype mock rates (to USD):
`USD 1.0, TRY 0.031, EUR 1.08, GBP 1.27` (reverse conversions supported).

Per position: `cost_basis` / `current_value` stay in the position's local
currency, and `cost_basis_base` / `current_value_base` are the USD equivalents.
Portfolio totals (`total_cost_basis`, `current_value`, `gain_loss`,
`gain_loss_percentage`, `portfolio_index`) and **both leaderboards** are computed
from the base-currency values. Example: AAPL (10 @ 180→195 USD) + THYAO.IS
(100 @ 250→295 TRY, ×0.031) → cost 2575, value 2864.5, **+11.24%**, index 111.24.

> The FX rates are mock prototype values and must be replaced by a real FX feed
> (the `FXProvider` interface makes this a drop-in swap).

### Sprint snapshots (anti-gaming)

When a user joins a sprint, the backend captures a **private snapshot** of their
current positions and the price of each at join time (converted to USD). Sprint
status and the sprint leaderboard are computed **only from that snapshot** —
never from the live portfolio. So adding, editing, or deleting positions after
joining **cannot** change sprint composition or inflate sprint return. Joining
is rejected with `400` if the portfolio is empty, or if any position can't be
priced or FX-converted at join. Snapshot details (symbols, quantities, prices,
values) are internal and never appear in any API response.

### Dynamic weekly sprints

The active sprint is generated from the current **ISO week** via a `Clock`
abstraction — id `weekly_YYYY_WW` (e.g. `weekly_2026_24`), running Monday
00:00 UTC to the next Monday 00:00 UTC. Status (`upcoming`/`active`/`completed`)
is derived from the clock, and `GET /competitions` ensures the current sprint
exists. Nothing is hardcoded or goes stale.

### Achievement re-evaluation

`POST /achievements/evaluate` re-checks all of the caller's badges on demand
(portfolio badges always; sprint badges against the current sprint) and returns
the updated list — so badges don't go stale waiting on a read operation.

### JWT after an in-memory restart

Because users live in memory, a restart drops users while previously issued JWTs
remain syntactically valid. The protected middleware (and `/me`) verify the
token's user still exists; a valid token for a missing user returns **`401`**.

## API

All responses are JSON. Errors use a consistent envelope: `{"error": "message"}`.

### Auth (milestone 1)

| Method | Path             | Auth | Description              |
| ------ | ---------------- | ---- | ------------------------ |
| POST   | `/auth/register` | no   | Register, returns token  |
| POST   | `/auth/login`    | no   | Login, returns token     |
| GET    | `/me`            | yes  | Current user             |

### Portfolio & prices (milestone 2 — all require `Authorization: Bearer <token>`)

| Method | Path                              | Description                               | Codes               |
| ------ | --------------------------------- | ----------------------------------------- | ------------------- |
| GET    | `/portfolio`                      | Get/auto-create default portfolio         | 200, 401            |
| POST   | `/portfolio/positions`            | Add a position                            | 201, 400, 401       |
| GET    | `/portfolio/positions`            | List own positions                        | 200, 401            |
| PUT    | `/portfolio/positions/{id}`       | Update own position                       | 200, 400, 401, 404  |
| DELETE | `/portfolio/positions/{id}`       | Delete own position                       | 204, 401, 404       |
| GET    | `/portfolio/summary`              | Calculated portfolio summary              | 200, 401, 502       |
| GET    | `/prices/{symbol}`                | Latest price for a symbol                 | 200, 401, 502       |
| GET    | `/leaderboard`                    | Anonymous performance ranking             | 200, 401, 500       |
| GET    | `/competitions`                   | List competitions (one active sprint)     | 200, 401            |
| POST   | `/competitions/{id}/join`         | Join a sprint (locks private baseline)    | 200, 400, 401, 404  |
| GET    | `/competitions/{id}/me`           | Your own sprint status                    | 200, 401, 404       |
| GET    | `/competitions/{id}/leaderboard`  | Anonymous sprint ranking                  | 200, 401, 404       |
| GET    | `/achievements`                   | Your badges (unlocked + locked)           | 200, 401            |
| POST   | `/achievements/evaluate`          | Re-evaluate badges, return updated list   | 200, 401            |

> `POST /auth/register` accepts an optional `"avatar_key"` (e.g. `"fox"`,
> `"bull"`); it defaults to `"default"` and is used only as a cosmetic label on
> the leaderboard.

Status code conventions: invalid auth → `401`; validation/bad payload → `400`;
position missing **or owned by another user** → `404` (for privacy);
price-provider failure → `502`.

## curl examples

```bash
# 1. Register
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"StrongPassword123","display_name":"AlphaWolf_91"}'

# 2. Login (capture the token)
TOKEN=$(curl -s -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"StrongPassword123"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')

# 3. Get (auto-create) the default portfolio
curl http://localhost:8080/portfolio -H "Authorization: Bearer $TOKEN"

# 4. Add a US stock
curl -X POST http://localhost:8080/portfolio/positions \
  -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" \
  -d '{"symbol":"AAPL","asset_type":"stock","quantity":10,"average_buy_price":180,"currency":"USD"}'

# 5. Add a Turkish stock (BIST) — note the .IS suffix and TRY currency
curl -X POST http://localhost:8080/portfolio/positions \
  -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" \
  -d '{"symbol":"THYAO.IS","asset_type":"stock","quantity":100,"average_buy_price":250,"currency":"TRY"}'

# 6. Add a crypto position
curl -X POST http://localhost:8080/portfolio/positions \
  -H "Content-Type: application/json" -H "Authorization: Bearer $TOKEN" \
  -d '{"symbol":"BTC-USD","asset_type":"crypto","quantity":0.1,"average_buy_price":65000,"currency":"USD"}'

# 7. List positions
curl http://localhost:8080/portfolio/positions -H "Authorization: Bearer $TOKEN"

# 8. Portfolio summary (prices come from the configured PriceProvider)
curl http://localhost:8080/portfolio/summary -H "Authorization: Bearer $TOKEN"

# 9. Latest price for a symbol
curl http://localhost:8080/prices/AAPL     -H "Authorization: Bearer $TOKEN"
curl http://localhost:8080/prices/BTC-USD  -H "Authorization: Bearer $TOKEN"
curl http://localhost:8080/prices/THYAO.IS -H "Authorization: Bearer $TOKEN"

# 10. Anonymous leaderboard
curl -X GET http://localhost:8080/leaderboard -H "Authorization: Bearer $TOKEN"

# 11. Weekly sprint competitions
curl -X GET  http://localhost:8080/competitions                              -H "Authorization: Bearer $TOKEN"
curl -X POST http://localhost:8080/competitions/weekly_2026_24/join          -H "Authorization: Bearer $TOKEN"
curl -X GET  http://localhost:8080/competitions/weekly_2026_24/me            -H "Authorization: Bearer $TOKEN"
curl -X GET  http://localhost:8080/competitions/weekly_2026_24/leaderboard   -H "Authorization: Bearer $TOKEN"

# 12. My achievements / badges
curl -X GET  http://localhost:8080/achievements          -H "Authorization: Bearer $TOKEN"
curl -X POST http://localhost:8080/achievements/evaluate -H "Authorization: Bearer $TOKEN"
```

> Sprint leaderboards and achievements expose only anonymous performance and
> badge state. They never expose starting value, current value, cost basis,
> dollar gain/loss, holdings, symbols, quantities, email, or password data.

> The leaderboard exposes only anonymous performance metrics. It never exposes
> portfolio value, holdings, quantities, cost basis, dollar gain/loss, email, or
> password data.

### Testing Turkish (BIST) and crypto symbols

With `PRICE_PROVIDER=mock`, `THYAO.IS` (295 TRY), `GARAN.IS` (120 TRY),
`ASELS.IS` (85 TRY), `BTC-USD` (68000 USD), and `ETH-USD` (3500 USD) all resolve
immediately and feed into the summary calculation. With `PRICE_PROVIDER=yahoo`,
the same symbols are fetched live from Yahoo (BIST tickers use the `.IS`
suffix; crypto uses the `BASE-QUOTE` form like `BTC-USD`).

## Security & privacy

- All portfolio and price routes require a valid JWT.
- A user can never read or mutate another user's portfolio or positions;
  accessing a foreign position returns `404` (it is indistinguishable from a
  non-existent one).
- The portfolio summary only ever includes the authenticated user's data.
- Responses expose curated view structs — no internal repository or
  password fields leak.

## Current limitations

- With `STORAGE_PROVIDER=memory` (the default), all data is lost on restart and
  previously issued JWTs are rejected with `401` because their user no longer
  exists. With `STORAGE_PROVIDER=postgres`, data and sessions survive restarts.
- One default portfolio per user (multi-portfolio support comes later).
- The weekly sprint is generated from the current ISO week and ensured lazily
  (plus by the background worker); sprints are not auto-archived and there is
  still only one concurrent sprint.
- FX rates are mock prototype values and the Yahoo price provider is
  prototype-grade; swap the `FXProvider` / `PriceProvider` for licensed feeds
  before any launch.
- Background jobs are a single in-process ticker worker — no distributed queue,
  no horizontal scaling of workers yet.
- Achievement evaluation is best-effort and synchronous (no event bus / no
  notifications); a failed evaluation never blocks the main request.
- NUMERIC columns are mapped to float64 in Go; fine at prototype precision but
  a decimal type is warranted before real money accuracy matters.

## Next development steps

- Replace mock price and FX providers with licensed production feeds.
- Add sprint archival/history and distributed-safe background workers.
- Move money calculations from float64 to a decimal type.
- Continue the React frontend with dedicated sprint, leaderboard, and
  achievement pages.

Storage, caching, pricing, FX, and cross-module calls are behind interfaces, so
these changes can be made without rewriting the handlers.
