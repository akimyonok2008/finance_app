# Error tracking and alerting

## Error tracking (Sentry)

Both the backend and frontend report to Sentry when a DSN is configured.
There is no default/shared DSN — reporting is opt-in per environment.

- **Backend**: set `SENTRY_DSN` (see `backend/.env.example`). Wired in
  `cmd/api/main.go` (`sentry.Init`) and `internal/server/sentry.go`
  (`sentryhttp` middleware). It captures panics recovered while serving an
  HTTP request, tagged with `AttachStacktrace` and the `Environment`
  (`APP_ENV`).
- **Frontend**: set `VITE_SENTRY_DSN` (see `frontend/.env.example`). Wired in
  `frontend/src/sentry.ts` and `frontend/src/main.tsx`, which wraps the app in
  `Sentry.ErrorBoundary` so a render crash shows a fallback screen instead of a
  blank page, and reports the error.

**Known gap:** background-worker failures (outbox processing, ranked-snapshot
compaction, corporate-action/income polling — see `backend/internal/jobs/`)
currently only go to structured `slog` logs, not Sentry. Each `RunOnce` already
catches and logs its own errors so one bad tick doesn't crash the process; none
of those log sites call `sentry.CaptureException`. If these jobs need paging
rather than log-scraping, that's separate wiring, not covered here.

## Metrics (Prometheus)

`GET /metrics` (see `internal/server/metrics.go`) is served on the *operations*
router (`NewOperations`), bound to loopback (`OPERATIONS_ADDR`,
default `127.0.0.1:9090`) by design — it's meant to be scraped by a Prometheus
server reachable only from your own infra/orchestrator network, never
exposed publicly.

There is currently no Prometheus server, scrape config, or Alertmanager
deployed anywhere in this repo — `/metrics` exists so a deployment has
something to point a scraper at, but nothing scrapes it today. If you stand up
a managed Prometheus (e.g. Grafana Cloud) or a self-hosted one, point it at
`OPERATIONS_ADDR:9090/metrics` and alert on `http_requests_total` (error rate:
5xx / total) and `http_request_duration_seconds` (latency).

## Leaderboard projection health (log signals)

The global leaderboard is served from a generation-based Postgres projection
that is deliberately stale-tolerant: reads keep serving the last promoted
generation no matter its age, because the alternative (an O(all-users) live
valuation per request) is the outage amplifier, not the fix. That makes these
structured log lines the health signal — staleness never surfaces as request
errors:

- `leaderboard_projection_stale_served` (WARN, per read): the active
  generation is past its adaptive freshness window
  (`max(30m, 2 x last cycle duration)`, capped at 24h). Occasional entries
  around a slow cycle are fine; a sustained stream means the refresh pipeline
  has stopped promoting — check `leaderboard_ranking_refresh_*` logs, the
  leader elector, and database health.
- `leaderboard_live_compute_declined` (WARN): only possible on a cold start
  at scale — no generation has EVER been promoted and the population exceeds
  the live-compute bound, so boards/standings are degrading softly. Should
  disappear once the first refresh lap completes; if it persists, the first
  cycle is failing (`leaderboard_ranking_cycle_complete_skipped` /
  `_failed`).
- `leaderboard_ranking_refresh_completed ... promoted=true` (INFO): the
  heartbeat. Expected roughly once per `ceil(users / REFRESH_BATCH_SIZE)`
  worker ticks; its absence over several hours is the earliest sign the
  pipeline is stuck.

## Runbooks

The only operational runbook today is
[`backup-restore.md`](backup-restore.md) (schema/data recovery). There is no
runbook yet for "Sentry paged, now what" — writing one productively needs a
real on-call rotation and escalation policy to point at, which don't exist yet
either.
