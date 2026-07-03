package marketdata

import (
	"sync"
	"time"
)

type RequestLimiter struct {
	mu           sync.Mutex
	maxPerMin    int
	dailyBudget  int
	windowStart  time.Time
	windowCount  int
	dayStart     time.Time
	dayCount     int
	backoffUntil time.Time
}

func NewRequestLimiter(maxPerMinute, dailyBudget int) *RequestLimiter {
	if maxPerMinute <= 0 {
		maxPerMinute = 6
	}
	if dailyBudget <= 0 {
		dailyBudget = 500
	}
	now := time.Now().UTC()
	return &RequestLimiter{
		maxPerMin: maxPerMinute, dailyBudget: dailyBudget,
		windowStart: now, dayStart: now,
	}
}

func (l *RequestLimiter) Allow() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().UTC()
	if now.Before(l.backoffUntil) {
		return ErrProviderRateLimited
	}
	if now.Sub(l.windowStart) >= time.Minute {
		l.windowStart = now
		l.windowCount = 0
	}
	if now.YearDay() != l.dayStart.YearDay() || now.Year() != l.dayStart.Year() {
		l.dayStart = now
		l.dayCount = 0
	}
	if l.dayCount >= l.dailyBudget {
		return ErrDailyBudgetExhausted
	}
	if l.windowCount >= l.maxPerMin {
		return ErrProviderRateLimited
	}
	l.windowCount++
	l.dayCount++
	return nil
}

func (l *RequestLimiter) Backoff(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	until := time.Now().UTC().Add(d)
	if until.After(l.backoffUntil) {
		l.backoffUntil = until
	}
}
