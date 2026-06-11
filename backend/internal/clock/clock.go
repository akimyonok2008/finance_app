// Package clock provides a tiny time abstraction so time-dependent logic (such
// as which weekly sprint is active) can be tested deterministically.
package clock

import "time"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// RealClock returns the actual wall-clock time (UTC).
type RealClock struct{}

// Now returns the current UTC time.
func (RealClock) Now() time.Time { return time.Now().UTC() }

// FixedClock returns a fixed time, settable by tests. The Time field may be
// reassigned to advance the clock.
type FixedClock struct{ Time time.Time }

// Now returns the fixed time.
func (c *FixedClock) Now() time.Time { return c.Time }
