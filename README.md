# Finance App

A privacy-first, gamified real-portfolio tracker. Users enter real holdings,
track percentage performance, create a locked strategy baseline, earn
achievements, compare ranked performance, and review public portfolio
composition without exposing wealth or position quantities.

The product direction is "Strava or Duolingo for investing," not a paper
trading simulator.

## Current Status

The repository contains a working full-stack app intended for personal use.
This configuration is intended for personal use, not public launch.

- React + TypeScript frontend with registration, login, a central dashboard
  hub, responsive portfolio management with symbol autocomplete and quote
  freshness badges, unified strategy leaderboard, an Explore hub for strategies,
  friends, and private messages, profiles, public-weight copy templates, and
  Portfolio Coach comparisons.
- Go REST API with JWT authentication, email/password auth, optional Google
  provider sign-in, portfolio calculations, strategy
  baselines, snapshot-backed timeframe leaderboards, personal standing,
  achievements, public profiles, mutual-only DMs, optional Twelve Data market
  data for USA stocks/ETFs, cached quotes, instrument search, and
  privacy-filtered Top 10 portfolio comparisons.
- Optional PostgreSQL persistence and Redis caching. For personal use,
  `STORAGE_PROVIDER=postgres` is recommended so accounts, positions, profiles,
  instruments, quotes, OAuth identities, social state, and messages survive
  restarts.
- Deterministic mock price and FX providers for local development and tests.
- Prototype 3A real market data is optional and disabled by default. Set
  `ENABLE_REAL_MARKET_DATA=true`, `PRICE_PROVIDER=twelvedata`, and
  `TWELVE_DATA_API_KEY` to use Twelve Data conservatively.

## Prototype 3 Handoff

Prototype 3 closes with these implemented workstreams:

- **Real portfolio core**: authenticated manual position entry, immutable
  baseline prices, quantity-only edits, FX-normalized owner summaries, and
  owner-only Dashboard/Portfolio views.
- **Market data foundation**: backend-owned instrument search and quotes,
  local seeded instruments for mock mode, Twelve Data stock/ETF search and quote
  support behind server-side caching/rate limits, quote freshness metadata, and
  no frontend provider keys.
- **Ranked strategy layer**: global leaderboard with timeframes, personal
  standing DTO, public profile enrichment, achievements, and public-safe
  strategy weight display.
- **Explore/social MVP**: public profile discovery, following/followers/friends,
  mutual-only one-to-one DMs, and no public feed or unauthenticated social data.
- **Portfolio Coach and copy flows**: private coach analysis, public Top 10 and
  profile comparison, copy-preview, and copy-from-public-weights into a fresh
  local baseline. No trades are executed.
- **Authentication**: email/password remains the default; Google Identity
  Services can be enabled with matching frontend/backend client IDs. Apple is
  not part of the active Prototype 3 product surface.

Key files for the next developer:

- Backend wiring: `backend/cmd/api/main.go`,
  `backend/internal/server/router.go`, `backend/internal/config/config.go`
- Market data: `backend/internal/marketdata/`,
  `frontend/src/components/portfolio/SymbolAutocomplete.tsx`,
  `frontend/src/components/portfolio/QuoteFreshnessBadge.tsx`
- Leaderboard/personal standing: `backend/internal/leaderboard/`,
  `frontend/src/components/leaderboard/YourStandingCard.tsx`
- Profiles, Explore, social, DMs: `backend/internal/profile/`,
  `backend/internal/social/`, `frontend/src/pages/Explore/`
- Portfolio Coach and copy flows: `backend/internal/coach/`,
  `backend/internal/strategy/`, `frontend/src/pages/PortfolioPage.tsx`
- Google auth: `backend/internal/auth/`, `frontend/src/pages/auth/`

## Privacy Model

The app separates absolute financial privacy from public composition:

- **Private owner views** such as Dashboard and Portfolio may show the
  authenticated user's full positions and monetary totals.
- **Strategy leaderboards** expose only opted-in profile details, rank,
  percentage return, ranked index, badges, and optional public percentage
  weights.
- **Portfolio Coach Top 10 comparisons** may show or use public composition:
  symbols, asset types, and percentage weights.
- **Compare/Copy flows** may use public symbols, asset types, and percentage
  weights only. Copying creates the user's own fresh baseline; it is not trading
  and never copies quantities or values.
- **Friends and DMs** are authenticated only. Users can follow public profiles;
  DMs are available only between mutual followers and show safe profile metadata
  plus message text.

No public or comparison surface may expose quantities, average buy prices,
current position prices, portfolio value, cost basis, absolute gain/loss,
starting value, portfolio/user IDs, emails, or brokerage identifiers.

Public weights describe portfolio allocation percentages only. They must not be
presented as monetary values or used to infer another user's wealth.

## What This App Is Not

Finance App does not place trades, connect to brokerages, import tax lots,
handle dividends/corporate actions, provide investment advice, process
payments, or claim public SaaS readiness. Market data is consumed for a
personal tracker through the backend only; public redistribution/compliance
hardening is outside the current scope.

## Run Locally

Start the backend in zero-infrastructure mode:

```bash
cd backend
go run ./cmd/api
```

In another terminal, start the frontend:

```bash
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173`, create an account, and add a supported demo
position.

Frontend defaults are documented in `frontend/.env.example`. Mock auth is
disabled by default and can be explicitly enabled with
`VITE_ENABLE_MOCK_AUTH=true`; the normal flow uses the Go auth endpoints.

To run with persistent PostgreSQL storage:

```bash
cd backend
docker compose up -d
STORAGE_PROVIDER=postgres \
DATABASE_URL="postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable" \
go run ./cmd/api
```

For persistent real-market-data development, put the values in `backend/.env`
instead of typing them every time:

```env
STORAGE_PROVIDER=postgres
DATABASE_URL=postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable
ENABLE_REAL_MARKET_DATA=true
PRICE_PROVIDER=twelvedata
TWELVE_DATA_API_KEY=your_twelve_data_key
GOOGLE_AUTH_ENABLED=true
GOOGLE_CLIENT_ID=your-google-client-id.apps.googleusercontent.com
```

Use the same Google Web Client ID in `frontend/.env` as
`VITE_GOOGLE_CLIENT_ID`, and add `http://localhost:5173` as an Authorized
JavaScript origin in Google Cloud. This Google Identity Services flow does not
need a redirect URI.

Then start:

```bash
cd backend
docker compose up -d
go run ./cmd/api
```

## Demo Market Data

Supported mock symbols:

```text
AAPL, MSFT, NVDA, SPY, BTC-USD, ETH-USD, THYAO.IS, GARAN.IS, ASELS.IS
```

Supported currencies: `USD`, `TRY`, `EUR`, `GBP`.

Position prices retain their quote currency while position value, gain/loss,
return, and the portfolio total are normalized to the user's base currency.

## Known Limitations

- Manual portfolio entry only; there is no brokerage integration.
- Google sign-in requires a provider client ID configured on both the frontend
  and backend; password auth stays available.
- Google OAuth setup is local-origin sensitive: `http://localhost:5173` and
  `http://127.0.0.1:5173` are different origins. The docs and Vite defaults use
  `http://localhost:5173`.
- Local development uses deterministic mock prices and FX rates by default.
- Twelve Data support is a conservative personal-use/free-tier foundation for
  USA stocks and ETFs first. It is cached server-side; the frontend never calls
  Twelve Data directly.
- The dashboard has no historical portfolio API, so its index path is
  illustrative and labeled as such.
- Timeframe leaderboard windows require index snapshots. `ALL` works
  immediately; `1W`, `1M`, `3M`, `6M`, and `1Y` exclude users until enough
  snapshot history has accrued.
- DMs are polling-based and mutual-follow gated; there are no WebSockets,
  notifications, group chats, attachments, reactions, or moderation tools yet.
- This is still a personal/local prototype. Public launch requires the blocker
  work listed below.

## Backup / Restore

For a local personal Postgres database:

```bash
cd backend
docker compose up -d
pg_dump "postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable" > finance_app_backup.sql
psql "postgres://postgres:postgres@localhost:5432/finance_app?sslmode=disable" < finance_app_backup.sql
```

Use a fresh dump before major code or migration experiments.

## Public Launch Blockers

Before any public launch, the app still needs legal/compliance review, market
data redistribution rights, production secrets management, real monitoring and
alerting, abuse/moderation controls for social features, account deletion/data
export workflows, hardened OAuth operations, and a real deployment/security
review.

## Verification

```bash
cd backend && go test ./...
cd frontend && npm run lint && npm run build
```

See [backend/README.md](backend/README.md) and
[frontend/README.md](frontend/README.md) for architecture and feature details.
