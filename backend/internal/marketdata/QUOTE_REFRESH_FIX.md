# Quote Refresh Worker: Hard Failure at 26+ Symbols

## Issue Fixed

The quote refresh worker was silently failing once 26+ distinct symbols existed across all users.

**Why:**
- Worker fetches all active symbols via `ListActiveSymbols()`
- Passes entire list to `RefreshSymbols()` → `Quotes()`
- `Quotes()` has a hard limit: `MaxQuoteBatchSize = 25` symbols per batch
- No chunking logic existed, so batches > 25 were rejected outright

**Impact:**
- Three diversified users = ~26 distinct symbols = permanent worker failure
- Every refresh pass logged: `quote refresh failed: ErrInvalidSymbols`
- Prices then only move when a user requests them
- Stale prices (up to 30 days old) feed valuations and leaderboard

**Failure mode:** Silent. No errors in logs, just `"quote refresh failed"` with no details.

---

## Fix Applied

### Changes Made

**File:** `service.go:189-207`

**Before:**
```go
func (s *Service) RefreshSymbols(ctx context.Context, symbols []string) (int, error) {
	resp, err := s.Quotes(ctx, symbols)  // ❌ Fails if len(symbols) > 25
	if err != nil {
		return 0, err
	}
	return len(resp.Quotes), nil
}
```

**After:**
```go
func (s *Service) RefreshSymbols(ctx context.Context, symbols []string) (int, error) {
	normalized := dedupeSymbols(symbols)
	if len(normalized) == 0 {
		return 0, nil
	}
	totalQuotes := 0
	for i := 0; i < len(normalized); i += s.cfg.MaxQuoteBatchSize {
		end := i + s.cfg.MaxQuoteBatchSize
		if end > len(normalized) {
			end = len(normalized)
		}
		resp, err := s.Quotes(ctx, normalized[i:end])
		if err != nil {
			return totalQuotes, err  // Return partial results on error
		}
		totalQuotes += len(resp.Quotes)
	}
	return totalQuotes, nil
}
```

**Key improvements:**
1. Chunks symbols into batches of ≤ `MaxQuoteBatchSize` (25)
2. Calls `Quotes()` for each chunk independently
3. Accumulates quote counts across chunks
4. Returns partial count on error (maintains existing semantics)

### Test Coverage Added

**File:** `service_test.go`

Three new tests verify the fix:

```go
// 60 symbols now process in 3 calls (25 + 25 + 10) instead of hard-failing
TestService_RefreshSymbolsChunksLargeSymbolLists()

// Deduplication still works before chunking
TestService_RefreshSymbolsDeduplicates()

// Partial results returned if a chunk fails
TestService_RefreshSymbolsReturnsErrorButPartialCount()
```

All tests pass. ✅

---

## What Still Needs Fixing

### Per-Replica Rate Limit Budget

The worker's second problem remains unsolved: **per-replica rate limiting**.

With 2 replicas and a budget of 500 req/day:
- Replica A uses up 500 requests
- Replica B uses up another 500 requests
- **Total: 1000 req/day** (exceeds provider's 500 limit)

**Impact:** One or both replicas hit quota and stop refreshing after ~250 requests.

**Why:** Each replica has its own in-process `RequestLimiter` with independent counters.

**Status:** Documented in [`RATE_LIMITING.md`](./RATE_LIMITING.md) with 5 solution options:
1. Config adjustment (quickest)
2. Leader election (recommended)
3. Database-backed limiter
4. Redis-backed limiter
5. Staggered refresh intervals

See [`RATE_LIMITING.md`](./RATE_LIMITING.md) for implementation details.

---

## Commit Message

```
Fix quote refresh worker hard-failure at 26+ symbols

The worker was passing all symbols to RefreshSymbols → Quotes at once,
which rejected batches > MaxQuoteBatchSize (25). With 26+ distinct tickers
across all users, every refresh pass failed silently, causing prices to
only update on user request instead of via the background worker.

Implement chunking in RefreshSymbols to batch symbols into groups of ≤ 25,
process each chunk independently, and accumulate results. Maintain existing
error semantics: return partial count on mid-chunk failure.

Fixes: quote refresh worker silently failing in multi-user deployments
Relates: per-replica rate limiting budget issue (documented in RATE_LIMITING.md)
```

---

## Verification

To verify the fix works:

1. **Create 60+ distinct symbols** across multiple users:
   ```bash
   # Add holdings to users such that total distinct symbols > 25
   ```

2. **Run refresh worker:**
   ```bash
   # Manually trigger or wait for scheduled interval
   # Should see: "quote refresh completed: symbols=60, quotes=60"
   ```

3. **Check prices are updated:**
   ```sql
   SELECT COUNT(*) FROM market_quotes WHERE fetched_at > now() - interval '10 minutes';
   ```

4. **Run tests:**
   ```bash
   cd backend && go test ./internal/marketdata -v -run RefreshSymbols
   ```

---

## Related Issues

- **Quote refresh worker**: This fix ✅
- **Per-replica rate limiting**: See [`RATE_LIMITING.md`](./RATE_LIMITING.md)
- **Config requirement**: `ENABLE_QUOTE_REFRESH_WORKER=true` in production (required in `config.go:331-333`)

---

## Future Work

Implement one of the solutions in [`RATE_LIMITING.md`](./RATE_LIMITING.md) to fix per-replica budget waste. Recommendation: **Option B (Leader Election)** for true distributed coordination with zero budget waste.
