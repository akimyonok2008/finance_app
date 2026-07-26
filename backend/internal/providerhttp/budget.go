package providerhttp

import (
	"errors"
	"sync"
	"time"
)

// ErrBudgetExhausted signals that a provider's daily request allowance is spent.
// It is deliberately distinguishable from "no data": callers must not interpret
// an exhausted budget as an empty result set.
var ErrBudgetExhausted = errors.New("provider daily request budget exhausted")

// DailyBudget is an in-memory request counter that resets when the UTC day
// rolls over. A limit of zero or less means unlimited.
type DailyBudget struct {
	mu    sync.Mutex
	limit int
	used  int
	day   string
	// Now is overridable for tests.
	Now func() time.Time
}

// NewDailyBudget builds a counter with the given daily limit.
func NewDailyBudget(limit int) *DailyBudget {
	return &DailyBudget{limit: limit, Now: time.Now}
}

// Consume reserves one request, returning ErrBudgetExhausted when the daily
// limit is already reached. No network call should be made when it errors.
func (b *DailyBudget) Consume() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit <= 0 {
		return nil
	}
	now := time.Now
	if b.Now != nil {
		now = b.Now
	}
	day := now().UTC().Format("2006-01-02")
	if day != b.day {
		b.day = day
		b.used = 0
	}
	if b.used >= b.limit {
		return ErrBudgetExhausted
	}
	b.used++
	return nil
}

// Used reports the requests consumed in the current window.
func (b *DailyBudget) Used() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}
