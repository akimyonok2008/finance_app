package benchmark

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// CorporateAction is a raw dividend or split event used to construct a
// total-return / adjusted series from raw closes. Providers that expose raw
// prices plus events let us build trustworthy data explicitly rather than
// mislabeling raw close as adjusted.
type CorporateAction struct {
	Date         string  // YYYY-MM-DD, ex-date
	CashDividend float64 // per-share cash distribution (>0 means a dividend)
	SplitRatio   float64 // shares after / shares before (2.0 = 2-for-1); 0/1 means none
}

// validateSeries checks a single component's series against the requirement and
// returns the (possibly downgraded) effective quality. It never trusts naming:
// a series claiming adjusted prices with unknown corporate-action handling is
// treated as not-total-return-capable.
//
// The returned error is one of the typed benchmark-data errors so callers can
// react precisely; a nil error means the series is usable at the returned
// quality for the given requirement.
func validateSeries(s BenchmarkPriceSeries, req SeriesRequirement) (DataQuality, error) {
	pts := s.Points
	if len(pts) < 2 {
		return DataQualityIncomplete, fmt.Errorf("%w: %s has %d points", ErrIncompleteSeries, s.Symbol, len(pts))
	}

	// Structural validation: sorted, unique, finite, positive.
	var prevDate string
	for i, p := range pts {
		if p.Date == "" {
			return DataQualityInvalid, fmt.Errorf("%w: %s empty date", ErrInvalidBenchmarkSeries, s.Symbol)
		}
		if _, err := time.Parse(dateLayout, p.Date); err != nil {
			return DataQualityInvalid, fmt.Errorf("%w: %s bad date %q", ErrInvalidBenchmarkSeries, s.Symbol, p.Date)
		}
		if i > 0 {
			if p.Date < prevDate {
				return DataQualityInvalid, fmt.Errorf("%w: %s dates not sorted", ErrInvalidBenchmarkSeries, s.Symbol)
			}
			if p.Date == prevDate {
				return DataQualityInvalid, fmt.Errorf("%w: %s duplicate date %s", ErrInvalidBenchmarkSeries, s.Symbol, p.Date)
			}
		}
		value := pointValue(p, s.Metadata.PriceType)
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return DataQualityInvalid, fmt.Errorf("%w: %s non-finite price", ErrInvalidBenchmarkSeries, s.Symbol)
		}
		if value <= 0 {
			return DataQualityInvalid, fmt.Errorf("%w: %s non-positive price", ErrInvalidBenchmarkSeries, s.Symbol)
		}
		prevDate = p.Date
	}

	m := s.Metadata

	// Metadata self-consistency.
	if m.IsTotalReturn && !m.PriceType.IsTotalReturnEquivalent() {
		return DataQualityInvalid, fmt.Errorf("%w: %s claims total-return with price_type %s", ErrInvalidBenchmarkSeries, s.Symbol, m.PriceType)
	}

	// Synthetic policy.
	if m.IsSynthetic {
		if !req.AllowSynthetic {
			return DataQualitySynthetic, fmt.Errorf("%w: %s", ErrSyntheticDataNotAllowed, s.Symbol)
		}
		return DataQualitySynthetic, nil
	}

	// Stale policy.
	if m.IsStale && !req.AllowStale {
		return DataQualityStale, fmt.Errorf("%w: %s", ErrStaleBenchmarkData, s.Symbol)
	}

	// Adjustment / total-return policy. Corporate-action handling must be known.
	totalReturnCapable := m.PriceType.IsTotalReturnEquivalent() && m.CorpActionsKnown
	if req.RequireTotalReturn && (!totalReturnCapable || !m.IsTotalReturn) {
		return DataQualityAcceptable, fmt.Errorf("%w: %s", ErrTotalReturnUnavailable, s.Symbol)
	}
	if req.RequireAdjusted && !totalReturnCapable {
		return DataQualityAcceptable, fmt.Errorf("%w: %s (price_type=%s, corp_actions_known=%v)", ErrAdjustedDataUnavailable, s.Symbol, m.PriceType, m.CorpActionsKnown)
	}

	// Effective quality: real data that is total-return-capable and fresh is
	// verified; real raw-close data is only acceptable.
	if m.IsStale {
		return DataQualityStale, nil
	}
	if totalReturnCapable {
		return DataQualityVerified, nil
	}
	return DataQualityAcceptable, nil
}

func pointValue(p PricePoint, priceType PriceType) float64 {
	if priceType == PriceTypeRawClose {
		// Compatibility for legacy test/providers. New raw adapters populate
		// RawClose and leave AdjustedClose empty.
		if p.RawClose == 0 {
			return p.AdjustedClose
		}
		return p.RawClose
	}
	return p.AdjustedClose
}

// BuildTotalReturnSeries constructs an adjusted (total-return-equivalent) series
// from raw closes plus corporate actions, back-adjusting historical prices so
// that neither splits nor reinvested dividends create false returns.
//
// It uses the standard back-adjustment: walking from newest to oldest, each
// event on date d multiplies all prior raw closes by a factor
//
//	f = (1 - dividend/close_{d-1}) * (1/splitRatio)
//
// so the resulting series' simple returns equal total returns. The output is
// labelled PriceTypeAdjustedClose with corporate actions known.
func BuildTotalReturnSeries(symbol string, raw []PricePoint, actions []CorporateAction, base BenchmarkDataMetadata) (BenchmarkPriceSeries, error) {
	if len(raw) < 2 {
		return BenchmarkPriceSeries{}, fmt.Errorf("%w: %s", ErrIncompleteSeries, symbol)
	}
	pts := append([]PricePoint(nil), raw...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Date < pts[j].Date })

	// Index raw close by date for dividend-relative computation.
	closeByDate := make(map[string]float64, len(pts))
	for _, p := range pts {
		closeByDate[p.Date] = p.AdjustedClose
	}

	// Cumulative back-adjust factor applied to all dates strictly before each
	// event date. We compute a per-date factor by processing events newest-first.
	acts := append([]CorporateAction(nil), actions...)
	sort.Slice(acts, func(i, j int) bool { return acts[i].Date > acts[j].Date }) // newest first

	factor := make([]float64, len(pts))
	for i := range factor {
		factor[i] = 1.0
	}
	for _, a := range acts {
		f := 1.0
		if a.SplitRatio > 0 && a.SplitRatio != 1 {
			f *= 1.0 / a.SplitRatio
		}
		if a.CashDividend > 0 {
			// Reference close is the trading day immediately before the ex-date.
			prevClose := priorClose(pts, a.Date)
			if prevClose > 0 {
				f *= 1.0 - a.CashDividend/prevClose
			}
		}
		if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return BenchmarkPriceSeries{}, fmt.Errorf("%w: %s bad corporate action on %s", ErrInvalidBenchmarkSeries, symbol, a.Date)
		}
		// Apply to every point strictly before the ex-date.
		for i, p := range pts {
			if p.Date < a.Date {
				factor[i] *= f
			}
		}
	}

	out := make([]PricePoint, len(pts))
	for i, p := range pts {
		out[i] = PricePoint{Date: p.Date, AdjustedClose: round(p.AdjustedClose*factor[i], 8)}
	}

	meta := base
	meta.PriceType = PriceTypeAdjustedClose
	meta.IsAdjusted = true
	meta.IsTotalReturn = true
	meta.IncludesDividends = true
	meta.IncludesSplits = true
	meta.CorpActionsKnown = true
	if meta.Quality == "" {
		meta.Quality = DataQualityVerified
	}
	return BenchmarkPriceSeries{Symbol: symbol, Points: out, Metadata: meta}, nil
}

// priorClose returns the raw close on the latest date strictly before ref.
func priorClose(sorted []PricePoint, ref string) float64 {
	var last float64
	for _, p := range sorted {
		if p.Date < ref {
			last = p.AdjustedClose
			continue
		}
		break
	}
	return last
}
