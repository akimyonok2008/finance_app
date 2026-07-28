# Rate Limiting: Per-Replica Budget Issue

## Problem

The quote refresh worker uses a per-instance `RequestLimiter` with in-process state (mutex + counters). When running multiple replicas, each replica has its own independent rate limiter with a full allocation of the budget.

**Example:**
- API provider allows: 6 req/min, 500 req/day
- Deployment: 2 replicas
- **Actual usage:** 12 req/min, 1000 req/day ❌ (exceeds provider limits)

### Why This Matters

1. **Silent quota exhaustion**: The second replica's requests get rejected after ~250 requests (half of 500)
2. **Uneven load**: If one replica hits quota first, it stops refreshing while others continue
3. **Scale pain**: Adding more replicas makes the problem exponentially worse

## Root Cause

[`RequestLimiter`](./rate_limiter.go) is instantiated per replica in [`main.go`](../../../cmd/api/main.go):

```go
// Each replica creates its own limiter with independent state
limiter: NewRequestLimiter(cfg.MaxPerMinute, cfg.DailyBudget)
```

The counters live in memory and are never shared between processes.

## Solutions

### Option A: Configuration Adjustment ⭐ (Quickest)

**When:** You know your replica count in advance
**Effort:** 5 minutes
**No code changes required**

Divide the rate limit by the number of replicas in deployment config:

```bash
# Single replica (current dev/staging)
TWELVE_DATA_DAILY_REQUEST_BUDGET=500

# Production with 2 replicas
TWELVE_DATA_DAILY_REQUEST_BUDGET=250  # 500 / 2

# Production with 3 replicas
TWELVE_DATA_DAILY_REQUEST_BUDGET=166  # 500 / 3
```

**Pros:**
- Zero code changes
- Works immediately
- No new dependencies

**Cons:**
- Wastes quota if you deploy fewer replicas than configured
- Requires manual math per deployment
- Can't scale elastically (replicas must be pre-decided)

---

### Option B: Leader Election ⭐⭐ (Recommended)

**When:** You want true distributed coordination
**Effort:** ~30 minutes
**New code:** ~50 lines (distributed lock helper)

Only one replica at a time executes the refresh job. Uses database-based pessimistic locking:

```go
func (w *QuoteRefreshWorker) refresh(ctx context.Context) {
	// Try to acquire lock; other replicas wait or skip
	acquired, err := w.acquireLeadershipLock(ctx, 30*time.Second)
	if !acquired {
		slog.Debug("quote refresh skipped: another replica is refreshing")
		return
	}
	defer w.releaseLeadershipLock(ctx)

	// ... proceed with refresh (only this replica executes)
}
```

**Database lock pseudocode:**
```sql
BEGIN;
SELECT pg_advisory_xact_lock(
  hashtext('quote_refresh_worker')
);
-- Do the work
COMMIT;
```

**Pros:**
- ✅ True single authority = zero budget waste
- ✅ Scales to any replica count automatically
- ✅ Automatic failover if leader replica dies
- ✅ Zero extra latency per request
- ✅ Uses existing PostgreSQL

**Cons:**
- Refresh job is idle on standby replicas
- Requires lock implementation
- If leader hangs, refresh pauses until lock timeout

**Implementation notes:**
- Use PostgreSQL `pg_advisory_xact_lock()` for advisory locks
- Timeout should be longer than longest refresh cycle (recommend: refresh_interval × 2)
- Holds lock only during refresh, not continuously
- On lock timeout, log warning but don't crash (allows self-healing)

---

### Option C: Database-Backed Rate Limiter

**When:** You want distributed quota tracking without leader election
**Effort:** ~1 hour
**New code:** ~150 lines + migration

Store minute-by-minute and daily request counts in PostgreSQL. All replicas query/update shared state:

```go
limiter := NewDatabaseRateLimiter(db, "twelvedata", maxPerMin, dailyBudget)
if err := limiter.Allow(ctx); err != nil {
	return err // rate limited or budget exceeded
}
```

**How it works:**
1. Each `Allow()` call does an upsert: `INSERT ... ON CONFLICT ... DO UPDATE`
2. Returns current counts + backoff_until time
3. All replicas check the same row in `provider_rate_limits` table

**Pros:**
- ✅ True shared budget across replicas
- ✅ No leader election needed
- ✅ Per-minute and per-day granularity

**Cons:**
- ❌ One DB query per rate limit check (latency overhead)
- ❌ Creates many rows: 1440 rows/day × number of providers (row cleanup needed)
- ❌ More complex than leader election
- Higher DB load

**Migration needed:**
```sql
CREATE TABLE provider_rate_limits (
    provider TEXT NOT NULL,
    window_key TEXT NOT NULL,        -- YYYY-MM-DDTHH:MM
    day_key TEXT NOT NULL,            -- YYYY-MM-DD
    request_count INTEGER DEFAULT 0,
    daily_count INTEGER DEFAULT 0,
    backoff_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ,
    PRIMARY KEY (provider, window_key, day_key)
);
```

**Cleanup needed** (e.g., via cron job):
```sql
-- Delete rows older than 7 days
DELETE FROM provider_rate_limits
WHERE updated_at < now() - interval '7 days';
```

---

### Option D: Redis-Backed Rate Limiter

**When:** You already run Redis or want the fastest solution
**Effort:** ~45 minutes
**New dependencies:** Redis client library

Use Redis atomic increments (much faster than DB):

```go
limiter := NewRedisRateLimiter(redisClient, "twelvedata", maxPerMin, dailyBudget)
if err := limiter.Allow(ctx); err != nil {
	return err // rate limited
}
```

**How it works:**
1. Keys: `rate_limit:{provider}:{minute}` and `rate_limit:{provider}:{day}`
2. Each allow increments + checks TTL
3. Uses Lua script for atomic check-and-increment

**Pros:**
- ✅ Fast (sub-millisecond)
- ✅ Truly distributed
- ✅ Built-in TTL (no cleanup needed)
- ✅ Simple implementation

**Cons:**
- ❌ Requires Redis deployment
- ❌ Another dependency
- If Redis down, need fallback behavior

---

### Option E: Staggered Refresh Intervals

**When:** You want simple load spreading without shared state
**Effort:** ~15 minutes
**New code:** ~10 lines

Each replica refreshes on a staggered schedule:

```go
replicaID := getReplicaID() // "0", "1", "2", ... from env/hostname
replicaIDNum := parseint(replicaID)
numReplicas := getNumReplicas() // from config or service discovery

// Spread refresh times: replica 0 at T+0, replica 1 at T+interval/3, etc.
startDelay := time.Duration(replicaIDNum) * (refreshInterval / time.Duration(numReplicas))
```

**Pros:**
- ✅ Zero shared state needed
- ✅ Simple to implement
- ✅ Spreads load across time

**Cons:**
- ❌ Doesn't solve budget (still 500×N total requests)
- ❌ Requires knowing replica count
- If replicas scale elastically, needs recalculation

---

## Recommendation Matrix

| Factor | Option A | Option B | Option C | Option D | Option E |
|--------|----------|----------|----------|----------|----------|
| **Effort** | 5 min | 30 min | 1 hr | 45 min | 15 min |
| **Code changes** | 0 | ~50 LOC | ~150 LOC | ~100 LOC | ~10 LOC |
| **New dependencies** | None | None | None | Redis | None |
| **Scales elastically** | ❌ | ✅ | ✅ | ✅ | ❌ |
| **Budget waste** | Yes | ❌ Zero | ❌ Zero | ❌ Zero | Yes |
| **Latency impact** | None | None | +1 DB query | <1ms | None |
| **DB overhead** | None | Minimal | High | None | None |
| **Complexity** | Lowest | Low | Medium | Medium | Low |

## Recommended Path

**Short term (immediate relief):**
- Use **Option A** to divide budget by current replica count
- Takes 5 minutes
- Solves the problem until you scale

**Long term (proper fix):**
- Implement **Option B (Leader Election)**
- Does the work on only one replica
- Scales to any replica count
- Zero budget waste
- ~30 minutes of work

## Implementation Checklist for Option B

- [ ] Create `distributed_lock.go` with PostgreSQL advisory lock helper
- [ ] Add `QuoteRefreshWorker.acquireLeadershipLock()` method
- [ ] Add `QuoteRefreshWorker.releaseLeadershipLock()` method
- [ ] Update `worker.go:refresh()` to acquire lock before work
- [ ] Add logging for lock acquisition/release
- [ ] Test: verify only one replica runs refresh with multiple goroutines
- [ ] Test: verify refresh resumes on lock timeout
- [ ] Update deployment docs with lock timeout recommendation
- [ ] Consider: should lock timeout equal `2 × refresh_interval`?

## References

- Current rate limiter: [`rate_limiter.go`](./rate_limiter.go)
- Worker: [`worker.go`](./worker.go)
- Configuration: [`config.go`](../../../internal/config/config.go)
- TwelveData provider: [`twelvedata_provider.go`](./twelvedata_provider.go)
