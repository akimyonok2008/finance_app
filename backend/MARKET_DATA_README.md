# Market Data & Quote Refresh: Known Issues & Solutions

## Overview

The quote refresh worker maintains fresh market prices for all holdings across all users. This document outlines two issues found in production:

1. **✅ FIXED: Hard failure at 26+ symbols** — Worker silently stopped refreshing
2. **⚠️ OPEN: Per-replica rate limiting** — Budget shared incorrectly across replicas

---

## Issue 1: Quote Refresh Worker Hard-Failure at 26+ Symbols ✅

### Status: FIXED

The worker hard-failed once 26+ distinct symbols existed across all users, causing prices to only update when explicitly requested by users.

**Fix:** Implemented chunking in `RefreshSymbols()` to process symbols in batches of ≤ 25.

**Details:** See [`internal/marketdata/QUOTE_REFRESH_FIX.md`](./internal/marketdata/QUOTE_REFRESH_FIX.md)

**Test coverage:** 3 new tests in `service_test.go` verify chunking behavior.

---

## Issue 2: Per-Replica Rate Limiting ⚠️

### Status: OPEN (Documented, not yet implemented)

With multiple replicas running, each has its own `RequestLimiter`. The API provider's quota is wasted:

- **Limit:** 500 requests/day (from provider)
- **2 replicas:** Each uses 500 = 1000 total (exceeds limit)
- **Result:** Quota exhausted after ~250 requests per replica

### Solutions

Five options documented in [`internal/marketdata/RATE_LIMITING.md`](./internal/marketdata/RATE_LIMITING.md):

| Option | Effort | Complexity | Recommendation |
|--------|--------|------------|-----------------|
| **A. Config adjustment** | 5 min | Trivial | Quick fix for known replica count |
| **B. Leader election** | 30 min | Low | **Recommended** — true distributed coordination |
| **C. Database-backed limiter** | 1 hr | Medium | Works but adds DB overhead |
| **D. Redis-backed limiter** | 45 min | Medium | Fast but requires Redis |
| **E. Staggered intervals** | 15 min | Low | Load spreading, doesn't solve budget |

**Recommended for next sprint:** Implement **Option B (Leader Election)**
- One replica at a time refreshes quotes
- Zero budget waste
- Automatic failover if leader fails
- ~30 minutes of work

### Quick Workaround (Right Now)

Divide the daily budget by the number of production replicas in your deployment:

```bash
# If running 2 replicas with a 500 req/day provider limit:
export TWELVE_DATA_DAILY_REQUEST_BUDGET=250

# If running 3 replicas:
export TWELVE_DATA_DAILY_REQUEST_BUDGET=166
```

---

## Architecture

### Quote Refresh Flow

```
QuoteRefreshWorker.Start()
  └─ Periodic ticker (every 10 min by default)
     └─ refresh()
        └─ ListActiveSymbols()  [gets all distinct symbols across users]
           └─ RefreshSymbols()  [chunked into batches of ≤ 25]
              └─ Quotes()       [for each batch, fetch fresh prices]
                 └─ provider.GetQuotes()  [calls TwelveData API]
                    └─ RequestLimiter.Allow()  [checks rate limit]
```

### Key Files

- **Worker:** [`internal/marketdata/worker.go`](./internal/marketdata/worker.go)
- **Service:** [`internal/marketdata/service.go`](./internal/marketdata/service.go) — `RefreshSymbols()` does chunking
- **Rate Limiter:** [`internal/marketdata/rate_limiter.go`](./internal/marketdata/rate_limiter.go)
- **TwelveData Provider:** [`internal/marketdata/twelvedata_provider.go`](./internal/marketdata/twelvedata_provider.go)
- **Configuration:** [`internal/config/config.go`](./internal/config/config.go) — rate limit settings

---

## Monitoring

### Check if Refresh is Running

```sql
-- See latest refresh logs
SELECT * FROM log WHERE message LIKE 'quote refresh%' ORDER BY created_at DESC LIMIT 10;

-- Or check if prices are recent
SELECT COUNT(*) as fresh_quotes
FROM market_quotes
WHERE fetched_at > now() - interval '10 minutes';

-- Or check for stale quotes
SELECT COUNT(*) as stale_quotes
FROM market_quotes
WHERE is_stale = true;
```

### Alert on Failure

```sql
-- Alert if no successful refresh in last 20 minutes
SELECT COUNT(*) as failed_cycles
FROM log
WHERE message = 'quote refresh failed'
  AND created_at > now() - interval '20 minutes';
```

### Rate Limit Status

Check if provider is rate-limiting:
```sql
SELECT message, count(*) as occurrences
FROM log
WHERE message LIKE '%rate%' OR message LIKE '%budget%'
GROUP BY message
ORDER BY occurrences DESC;
```

---

## Configuration

### Rate Limiting Settings

```bash
# API provider and credentials
PRICE_PROVIDER=twelvedata
TWELVE_DATA_API_KEY=...
ENABLE_REAL_MARKET_DATA=true

# Rate limiting (TwelveData defaults)
TWELVE_DATA_MAX_REQUESTS_PER_MINUTE=6        # Provider limit
TWELVE_DATA_DAILY_REQUEST_BUDGET=500         # Provider limit
# ⚠️ IMPORTANT: Divide by replica count if running multiple replicas

# Cache settings
QUOTE_CACHE_TTL=10m                         # How long to cache a quote
QUOTE_STALE_AFTER=15m                       # When quote marked stale if not refreshed
ENABLE_QUOTE_REFRESH_WORKER=true            # Required in production
```

### Production Requirements

In production (`APP_ENV=production`), the following are enforced:

```go
// config.go:331-333
if !c.EnableQuoteRefreshWorker {
    return fmt.Errorf("APP_ENV=production requires ENABLE_QUOTE_REFRESH_WORKER=true")
}
```

So production **requires** the worker to be enabled. This is why the hard-failure bug (Issue 1) was critical — the platform shipped with a permanently-broken worker.

---

## Testing

### Unit Tests

```bash
cd backend
go test ./internal/marketdata -v

# Just the chunking tests
go test ./internal/marketdata -v -run RefreshSymbols
```

### Integration Test (Manual)

1. Create 30+ distinct symbols across users
2. Wait for next refresh cycle (10 min by default)
3. Verify: `quote refresh completed: symbols=30, quotes=30` in logs
4. Check: `market_quotes.updated_at` should be recent

---

## Next Steps

1. **Short term:** Use config workaround (divide budget by replica count)
2. **Medium term:** Implement Option B from [`RATE_LIMITING.md`](./internal/marketdata/RATE_LIMITING.md)
3. **Long term:** Monitor stale price rates to catch any regressions

---

## References

- TwelveData API docs: https://twelvedata.com/
- Issue tracking: See [`QUOTE_REFRESH_FIX.md`](./internal/marketdata/QUOTE_REFRESH_FIX.md) and [`RATE_LIMITING.md`](./internal/marketdata/RATE_LIMITING.md)
- Deployment guide: `DEPLOYMENT.md` (rate limit settings per environment)
