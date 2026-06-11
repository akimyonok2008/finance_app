package leaderboard

import "errors"

// ErrListUsers wraps a failure to enumerate users. Because the service skips
// individual users whose summary fails, this is the main error a caller will
// see, and it maps to HTTP 500.
var ErrListUsers = errors.New("could not list users for leaderboard")
