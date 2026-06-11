# Finance App Frontend

Premium, dark-mode-first React interface for the gamified real-portfolio
tracker. The frontend talks directly to the Go API and currently provides
authentication, a read-only game dashboard, manual portfolio management, a
global leaderboard, and a unified competition Arena.

## Implemented Screens

### Authentication

- Register and sign in with the real backend.
- JWT and user session persisted in `localStorage`.
- Protected application routes.
- Automatic session clearing and redirect to `/login` after a `401`.
- Optional mock-auth fallback through `VITE_ENABLE_MOCK_AUTH`.

### Dashboard

- Portfolio index, value, cost basis, gain/loss, and percentage performance.
- Deterministic prototype index chart until historical snapshots are available.
- Current weekly sprint status and privacy-safe sprint standings.
- Personal global rank and full privacy-safe leaderboard detail view.
- Trophy case preview with links into the Arena.
- Working Dashboard and Portfolio navigation, refresh, logout, skeletons,
  partial errors, and new-user onboarding.
- Responsive bento layout for desktop and mobile.

The dashboard is intentionally read-only. Sprint joining, sprint standings,
and the complete achievement collection live in the Arena.

### Portfolio

- Portfolio summary cards.
- Add, edit, and delete positions.
- Desktop form and positions table.
- Mobile add drawer and position cards.
- Currency-aware position pricing plus base-currency value, gain/loss, and
  FX-normalized return.
- Loading, empty, validation-error, and confirmation states.
- Query invalidation keeps portfolio, dashboard ranking, and achievements fresh.

### Leaderboard and Arena

- Privacy-safe global leaderboard with client-side filtering and pagination.
- Current competition join flow and cohort standings.
- Achievement evaluation and complete trophy collection.

## Stack

- React 19 + TypeScript + Vite
- React Router
- Tailwind CSS v3
- Hand-rolled shadcn-style UI primitives with Radix and Vaul
- TanStack Query v5
- React Hook Form + Zod
- Recharts
- Framer Motion
- Lucide React
- Sonner

## Run

The Go API must be available at `http://localhost:8080` by default.

```bash
# Terminal 1
cd backend
go run ./cmd/api

# Terminal 2
cd frontend
npm install
npm run dev
```

Open `http://localhost:5173`.

## Configuration

Copy `.env.example` or provide equivalent values:

```env
VITE_API_BASE_URL=http://localhost:8080
VITE_AUTH_LOGIN_PATH=/auth/login
VITE_AUTH_REGISTER_PATH=/auth/register
VITE_ENABLE_MOCK_AUTH=false
```

The real JWT is stored under `finance_app_token`; the authenticated user is
stored under `finance_app_user`.

## Routes

| Route | Status | Purpose |
| --- | --- | --- |
| `/login` | Implemented | Sign in |
| `/register` | Implemented | Create an account |
| `/dashboard` | Implemented | Performance and game overview |
| `/portfolio` | Implemented | Manage holdings |
| `/leaderboard` | Implemented | Privacy-safe global leaderboard |
| `/arena` | Implemented | Sprint join, cohort ranking, and trophies |
| `/sprint` | Redirected | Arena replaces the separate sprint page |
| `/achievements` | Redirected | Arena replaces the separate achievement page |

Unknown and unimplemented routes currently redirect to `/dashboard`.

## Project Structure

```text
src/
  api/             authenticated API client and domain API helpers
  auth/            provider, storage, protected route, and auth context
  components/
    portfolio/     position forms, tables, cards, dialogs, and summaries
    ui/            reusable UI primitives
  hooks/           TanStack Query hooks and centralized query keys
  pages/
    auth/          login and registration
    Dashboard/     bento dashboard and formatters
    arena/         competition and achievement experience
    leaderboard/   privacy-safe global standings
    PortfolioPage.tsx
  types/           API and form types
  utils/           formatting and class-name helpers
```

## Server-State Rules

Portfolio mutations invalidate:

```text
positions
portfolioSummary
leaderboard
leaderboardMe
achievements
```

This is required because a position change can affect portfolio values,
personal rank, global standings, and badge eligibility.

## Demo Market Data

Supported mock symbols:

```text
AAPL, MSFT, NVDA, SPY, BTC-USD, ETH-USD, THYAO.IS, GARAN.IS, ASELS.IS
```

Supported currencies: `USD`, `TRY`, `EUR`, `GBP`.

## Verification

```bash
npm run lint
npm run build
```

Both commands currently pass. The production build warns that the main
JavaScript chunk exceeds 500 kB; route-based code splitting is planned as more
screens are added.

## Known Frontend Gaps

- The dashboard chart is derived from the current index because the backend
  does not yet expose portfolio history.
- Personal leaderboard matching currently uses the authenticated display name
  because the backend does not expose `/leaderboard/me`.
- Sprint and achievements are unified in the Arena screen.
- The current achievement API exposes unlock state but not numeric progress, so
  legacy locked badges display `0 / 1` and unlocked badges display `1 / 1`.
- Timeframe filtering, a separate `me` row, and server pagination require a
  future personalized leaderboard backend contract.
- Google sign-in is a visual prototype only and is disabled unless mock auth is
  enabled.
- There is no automated frontend test suite yet; build, lint, and browser QA
  are the current verification steps.
