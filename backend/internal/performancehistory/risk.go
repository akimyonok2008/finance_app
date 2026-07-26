package performancehistory

import (
	"math"
	"time"
)

// RiskConsistency is the Risk & Consistency block of the Performance tab.
//
// Every field is derived from the CANONICAL ranked index only — never from
// portfolio value — because deposits and withdrawals move value without being
// a gain or a loss and would fabricate drawdowns and winning weeks.
//
// Missing analytics are nil, never 0: "not enough history" and "flat" are
// different facts and the UI must be able to tell them apart.
type RiskConsistency struct {
	// MaxDrawdownPercentage is the worst peak-to-trough of the ranked index over
	// the selected window (non-positive). Reuses MaxDrawdownPercent.
	MaxDrawdownPercentage *float64 `json:"max_drawdown_percentage"`
	// CurrentDrawdownPercentage is the final index versus its running peak
	// (non-positive). 0 means the window ends at a new high.
	CurrentDrawdownPercentage *float64 `json:"current_drawdown_percentage"`

	// PositiveWeeksPercentage is positive complete calendar weeks divided by all
	// complete calendar weeks. The current, still-running week is excluded.
	PositiveWeeksPercentage *float64 `json:"positive_weeks_percentage"`
	PositiveWeeks           int      `json:"positive_weeks"`
	CompleteWeeks           int      `json:"complete_weeks"`

	// Best/Worst month cover complete calendar months only.
	BestMonth       *PeriodReturn `json:"best_month"`
	WorstMonth      *PeriodReturn `json:"worst_month"`
	CompleteMonths  int           `json:"complete_months"`
	WeeksReason     string        `json:"weeks_reason,omitempty"`
	MonthsReason    string        `json:"months_reason,omitempty"`
	DrawdownReason  string        `json:"drawdown_reason,omitempty"`
	CalculationBase string        `json:"calculation_base"`
}

// PeriodReturn is one complete calendar period's ranked return.
type PeriodReturn struct {
	// Label is the period key: "2026-03" for a month.
	Label            string  `json:"label"`
	ReturnPercentage float64 `json:"return_percentage"`
}

const (
	reasonNotEnoughWeeks  = "Not enough history: fewer than one complete calendar week."
	reasonNotEnoughMonths = "Not enough history: fewer than one complete calendar month."
)

// indexPoint is the minimal shape the period math needs.
type indexPoint struct {
	at    time.Time
	index float64
}

// weekStart returns the UTC Monday 00:00 of the week containing t.
func weekStart(t time.Time) time.Time {
	utc := t.UTC()
	day := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(day.Weekday()) + 6) % 7 // Monday = 0
	return day.AddDate(0, 0, -offset)
}

// monthStart returns the UTC first-of-month 00:00 of the month containing t.
func monthStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

// periodReturns buckets points into calendar periods and returns each COMPLETE
// period's return, chained across period boundaries.
//
// A period's return is `closing index of the period / closing index of the
// previous period - 1`. The first period in the series has no previous close,
// so it is measured from the first observation inside it instead; that is the
// only defensible starting anchor available and it is a complete period all the
// same. Periods that are still running at `now` are excluded entirely.
func periodReturns(points []indexPoint, now time.Time, startOf func(time.Time) time.Time, next func(time.Time) time.Time, label func(time.Time) string) []PeriodReturn {
	if len(points) == 0 {
		return nil
	}

	type bucket struct {
		start        time.Time
		first, close float64
	}
	buckets := make([]bucket, 0, len(points))
	for _, p := range points {
		if p.index <= 0 || math.IsNaN(p.index) || math.IsInf(p.index, 0) {
			continue
		}
		start := startOf(p.at)
		if n := len(buckets); n > 0 && buckets[n-1].start.Equal(start) {
			buckets[n-1].close = p.index
			continue
		}
		buckets = append(buckets, bucket{start: start, first: p.index, close: p.index})
	}

	currentStart := startOf(now)
	out := make([]PeriodReturn, 0, len(buckets))
	for i, b := range buckets {
		// Still-running period: excluded.
		if !b.start.Before(currentStart) {
			continue
		}
		// A gap in coverage (no observation in the intervening periods) means the
		// previous close is not the period's true opening level; anchor on the
		// bucket's own first observation instead of chaining across the gap.
		opening := b.first
		if i > 0 && next(buckets[i-1].start).Equal(b.start) {
			opening = buckets[i-1].close
		}
		if opening <= 0 {
			continue
		}
		out = append(out, PeriodReturn{
			Label:            label(b.start),
			ReturnPercentage: round4((b.close/opening - 1) * 100),
		})
	}
	return out
}

// CalculateRiskConsistency derives the Risk & Consistency block from a ranked
// snapshot series. `now` bounds which calendar periods count as complete.
//
// It deliberately lives here in the performancehistory service and not in React:
// the browser must never derive performance statistics itself.
func CalculateRiskConsistency(points []indexPoint, now time.Time) RiskConsistency {
	out := RiskConsistency{CalculationBase: "ranked_index"}

	if len(points) == 0 {
		out.DrawdownReason = reasonNoSnapshots
		out.WeeksReason = reasonNotEnoughWeeks
		out.MonthsReason = reasonNotEnoughMonths
		return out
	}

	indexes := make([]float64, 0, len(points))
	for _, p := range points {
		indexes = append(indexes, p.index)
	}
	series := DrawdownSeriesPercent(indexes)
	maxDrawdown := round4(MaxDrawdownPercent(indexes))
	current := round4(series[len(series)-1])
	out.MaxDrawdownPercentage = &maxDrawdown
	out.CurrentDrawdownPercentage = &current

	weeks := periodReturns(points, now, weekStart,
		func(t time.Time) time.Time { return t.AddDate(0, 0, 7) },
		func(t time.Time) string { return t.Format("2006-01-02") })
	out.CompleteWeeks = len(weeks)
	if len(weeks) == 0 {
		out.WeeksReason = reasonNotEnoughWeeks
	} else {
		for _, w := range weeks {
			if w.ReturnPercentage > 0 {
				out.PositiveWeeks++
			}
		}
		share := round4(float64(out.PositiveWeeks) / float64(len(weeks)) * 100)
		out.PositiveWeeksPercentage = &share
	}

	months := periodReturns(points, now, monthStart,
		func(t time.Time) time.Time { return t.AddDate(0, 1, 0) },
		func(t time.Time) string { return t.Format("2006-01") })
	out.CompleteMonths = len(months)
	if len(months) == 0 {
		out.MonthsReason = reasonNotEnoughMonths
	} else {
		best, worst := months[0], months[0]
		for _, m := range months[1:] {
			if m.ReturnPercentage > best.ReturnPercentage {
				best = m
			}
			if m.ReturnPercentage < worst.ReturnPercentage {
				worst = m
			}
		}
		out.BestMonth = &best
		out.WorstMonth = &worst
	}
	return out
}
