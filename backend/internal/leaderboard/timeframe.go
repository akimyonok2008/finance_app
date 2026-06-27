package leaderboard

import (
	"strings"
	"time"
)

// Timeframe selects the ranking window. ALL is since each user's locked
// baseline; the others are trailing windows measured against the portfolio
// index recorded at (now - window).
type Timeframe string

const (
	Timeframe1W  Timeframe = "1W"
	Timeframe1M  Timeframe = "1M"
	Timeframe3M  Timeframe = "3M"
	Timeframe6M  Timeframe = "6M"
	Timeframe1Y  Timeframe = "1Y"
	TimeframeAll Timeframe = "ALL"
)

// ParseTimeframe normalizes a query value to a known Timeframe, defaulting to
// ALL for empty or unrecognized input (the endpoint never 400s on timeframe).
func ParseTimeframe(raw string) Timeframe {
	switch Timeframe(strings.ToUpper(strings.TrimSpace(raw))) {
	case Timeframe1W:
		return Timeframe1W
	case Timeframe1M:
		return Timeframe1M
	case Timeframe3M:
		return Timeframe3M
	case Timeframe6M:
		return Timeframe6M
	case Timeframe1Y:
		return Timeframe1Y
	default:
		return TimeframeAll
	}
}

// window returns the trailing duration for the timeframe. windowed=false means
// "since baseline" (ALL), which needs no historical snapshot.
func (tf Timeframe) window() (dur time.Duration, windowed bool) {
	const day = 24 * time.Hour
	switch tf {
	case Timeframe1W:
		return 7 * day, true
	case Timeframe1M:
		return 30 * day, true
	case Timeframe3M:
		return 90 * day, true
	case Timeframe6M:
		return 180 * day, true
	case Timeframe1Y:
		return 365 * day, true
	default:
		return 0, false
	}
}
