package performancehistory

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
)

// Timeframe identifiers accepted by the canonical ranked-history endpoint.
const (
	Timeframe1W  = "1W"
	Timeframe1M  = "1M"
	Timeframe3M  = "3M"
	Timeframe6M  = "6M"
	Timeframe1Y  = "1Y"
	TimeframeAll = "ALL"
)

// ErrNoStartingIndex is returned when a timeframe return is requested without a
// usable (strictly positive) starting index.
var ErrNoStartingIndex = errors.New("performancehistory: starting ranked index must be positive")

// ParseTimeframe normalizes a raw query value, defaulting to 1M.
func ParseTimeframe(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case Timeframe1W:
		return Timeframe1W
	case Timeframe3M:
		return Timeframe3M
	case Timeframe6M:
		return Timeframe6M
	case Timeframe1Y:
		return Timeframe1Y
	case TimeframeAll, "MAX":
		return TimeframeAll
	default:
		return Timeframe1M
	}
}

// TimeframeWindow returns the lookback duration for a normalized timeframe.
// TimeframeAll reports ok=false: the caller should read from the epoch instead.
func TimeframeWindow(timeframe string) (time.Duration, bool) {
	const day = 24 * time.Hour
	switch timeframe {
	case Timeframe1W:
		return 7 * day, true
	case Timeframe3M:
		return 90 * day, true
	case Timeframe6M:
		return 180 * day, true
	case Timeframe1Y:
		return 365 * day, true
	case TimeframeAll:
		return 0, false
	default:
		return 30 * day, true
	}
}

// TimeframeReturnPercent computes `ending/starting - 1` as a percentage.
//
// It is deliberately NOT `ending - 100`: a timeframe rarely starts at index 100,
// and subtracting 100 would silently report the since-inception return instead
// of the selected period's return.
func TimeframeReturnPercent(startingIndex, endingIndex float64) (float64, error) {
	if startingIndex <= 0 || math.IsNaN(startingIndex) || math.IsInf(startingIndex, 0) {
		return 0, ErrNoStartingIndex
	}
	if math.IsNaN(endingIndex) || math.IsInf(endingIndex, 0) {
		return 0, ErrNoStartingIndex
	}
	return (endingIndex/startingIndex - 1) * 100, nil
}

// DrawdownSeriesPercent converts a ranked-index series into the running
// drawdown series `index_t / running_peak_t - 1`, expressed as a non-positive
// percentage.
//
// Drawdown MUST be computed from the ranked index, never from portfolio value:
// deposits and withdrawals move value without being a gain or a loss and would
// fabricate or erase drawdowns.
func DrawdownSeriesPercent(indexes []float64) []float64 {
	out := make([]float64, 0, len(indexes))
	peak := math.Inf(-1)
	for _, idx := range indexes {
		if idx > peak {
			peak = idx
		}
		if peak <= 0 || math.IsInf(peak, -1) {
			out = append(out, 0)
			continue
		}
		out = append(out, (idx/peak-1)*100)
	}
	return out
}

// MaxDrawdownPercent returns the worst (most negative) drawdown in the series,
// as a non-positive percentage. An empty series has no drawdown: 0.
func MaxDrawdownPercent(indexes []float64) float64 {
	worst := 0.0
	for _, dd := range DrawdownSeriesPercent(indexes) {
		if dd < worst {
			worst = dd
		}
	}
	return worst
}

// RankedHistoryPoint is one canonical ranked-history point. It carries ranked
// projections only — never values, quantities, or cost basis.
type RankedHistoryPoint struct {
	CapturedAt         string  `json:"captured_at"`
	RankedIndex        float64 `json:"ranked_index"`
	ReturnPercentage   float64 `json:"return_percentage"`
	DrawdownPercentage float64 `json:"drawdown_percentage"`
	RankingStatus      string  `json:"ranking_status"`
}

// RankedHistory is the response of the canonical ranked-history read. When
// Available is false the analytics are absent rather than zero, so the UI can
// show a truthful empty state instead of a fabricated flat line.
type RankedHistory struct {
	Timeframe                 string               `json:"timeframe"`
	From                      string               `json:"from"`
	To                        string               `json:"to"`
	Available                 bool                 `json:"available"`
	Reason                    string               `json:"reason,omitempty"`
	Points                    []RankedHistoryPoint `json:"points"`
	StartingIndex             *float64             `json:"starting_index,omitempty"`
	EndingIndex               *float64             `json:"ending_index,omitempty"`
	TimeframeReturnPercentage *float64             `json:"timeframe_return_percentage,omitempty"`
	MaxDrawdownPercentage     *float64             `json:"max_drawdown_percentage,omitempty"`
}

const reasonNoSnapshots = "Performance history will appear after the first trusted snapshot."

// RankedHistory reads the CANONICAL ranked snapshot history (the same source
// the leaderboard and benchmark achievements treat as truth) and derives the
// timeframe return and drawdown series from it.
//
// It never reads the private portfolio valuation archive: that archive is
// cost-basis/value history and is distorted by deposits and withdrawals.
func (s *Service) RankedHistory(ctx context.Context, userID, rawTimeframe string) (RankedHistory, error) {
	timeframe := ParseTimeframe(rawTimeframe)
	to := s.now()
	from := time.Time{}
	if window, bounded := TimeframeWindow(timeframe); bounded {
		from = to.Add(-window)
	}

	points, err := s.Series(ctx, userID, from, to)
	if err != nil {
		return RankedHistory{}, err
	}

	out := RankedHistory{
		Timeframe: timeframe,
		To:        to.UTC().Format(time.RFC3339),
		Points:    []RankedHistoryPoint{},
	}
	if !from.IsZero() {
		out.From = from.UTC().Format(time.RFC3339)
	} else if len(points) > 0 {
		out.From = points[0].CapturedAt.UTC().Format(time.RFC3339)
	}

	if len(points) == 0 {
		out.Reason = reasonNoSnapshots
		return out, nil
	}

	indexes := make([]float64, 0, len(points))
	for _, p := range points {
		indexes = append(indexes, p.RankedIndex)
	}
	drawdowns := DrawdownSeriesPercent(indexes)
	starting := indexes[0]
	ending := indexes[len(indexes)-1]

	for i, p := range points {
		ret := 0.0
		if value, err := TimeframeReturnPercent(starting, indexes[i]); err == nil {
			ret = value
		}
		out.Points = append(out.Points, RankedHistoryPoint{
			CapturedAt:         p.CapturedAt.UTC().Format(time.RFC3339),
			RankedIndex:        round4(p.RankedIndex),
			ReturnPercentage:   round4(ret),
			DrawdownPercentage: round4(drawdowns[i]),
			RankingStatus:      string(p.RankingStatus),
		})
	}

	timeframeReturn, err := TimeframeReturnPercent(starting, ending)
	if err != nil {
		out.Reason = reasonNoSnapshots
		return out, nil
	}
	maxDrawdown := round4(MaxDrawdownPercent(indexes))
	startRounded, endRounded := round4(starting), round4(ending)
	returnRounded := round4(timeframeReturn)

	out.Available = true
	out.StartingIndex = &startRounded
	out.EndingIndex = &endRounded
	out.TimeframeReturnPercentage = &returnRounded
	out.MaxDrawdownPercentage = &maxDrawdown
	return out, nil
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}
