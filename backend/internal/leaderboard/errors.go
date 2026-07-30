package leaderboard

import "errors"

// ErrListUsers wraps a failure to enumerate users. Because the service skips
// individual users whose summary fails, this is the main error a caller will
// see, and it maps to HTTP 500.
var ErrListUsers = errors.New("could not list users for leaderboard")

// ErrRankingUnavailable means no ranking-projection generation has ever been
// promoted AND the population is too large for the O(N) live path (see
// Service.liveComputeAllowed). It only occurs during a cold start at scale —
// once the first refresh cycle completes it can never recur, because reads
// prefer even a stale projection over live computation. Request-path callers
// degrade softly (empty board, "preparing" standing, unranked); background
// callers (the Explore projection rebuild) propagate it so their previous
// generation stays published.
var ErrRankingUnavailable = errors.New("global rankings are still being prepared")
