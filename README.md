# Finance App

A privacy-first, gamified real-portfolio tracker. Users enter real holdings,
track percentage performance, create a locked strategy baseline, earn
achievements, compare ranked performance, and review public portfolio
composition without exposing wealth or position quantities.

The product direction is "Strava or Duolingo for investing," not a paper
trading simulator.

## Current Status

The repository contains a working full-stack prototype:

- React + TypeScript frontend with registration, login, dashboard, responsive
  portfolio management, unified strategy leaderboard, Explore, profiles, and
  Portfolio Coach.
- Go REST API with JWT authentication, portfolio calculations, strategy
  baselines, ranked leaderboards, achievements, public profiles, and
  privacy-filtered Top 10 portfolio comparisons.
- Optional PostgreSQL persistence and Redis caching.
- Deterministic mock price and FX providers for local development and tests.

## Privacy Model

The app separates absolute financial privacy from public composition:

- **Private owner views** such as Dashboard and Portfolio may show the
  authenticated user's full positions and monetary totals.
- **Strategy leaderboards** expose only opted-in profile details, rank,
  percentage return, ranked index, badges, and optional public percentage
  weights.
- **Portfolio Coach Top 10 comparisons** may show or use public composition:
  symbols, asset types, and percentage weights.

No public or comparison surface may expose quantities, average buy prices,
current position prices, portfolio value, cost basis, absolute gain/loss,
starting value, portfolio/user IDs, emails, or brokerage identifiers.

Public weights describe portfolio allocation percentages only. They must not be
presented as monetary values or used to infer another user's wealth.

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

To run with persistent PostgreSQL storage and Redis:

```bash
cd backend
docker compose up -d
STORAGE_PROVIDER=postgres \
REDIS_URL=redis://localhost:6379/0 \
ENABLE_BACKGROUND_WORKERS=true \
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

## Prototype 1 Limitations

- Manual portfolio entry only; there is no brokerage integration.
- Google sign-in is not production OAuth and is hidden unless mock auth is
  explicitly enabled.
- Local development uses deterministic mock prices and FX rates by default.
- The dashboard has no historical portfolio API, so its index path is
  illustrative and labeled as such.
- Leaderboard timeframe tabs are sent to the API, but the current backend
  returns the same active strategy ranking until timeframe-specific ranking is
  implemented.

## Verification

```bash
cd backend && go test ./...
cd frontend && npm run lint && npm run build
```

See [backend/README.md](backend/README.md) and
[frontend/README.md](frontend/README.md) for architecture and feature details.
