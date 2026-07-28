package benchmark

import (
	"fmt"
	"sort"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// CorporateAction is a raw dividend or split event used to construct a
// total-return / adjusted series from raw closes. Providers that expose raw
// prices plus events let us build trustworthy data explicitly rather than
// mislabeling raw close as adjusted.
type CorporateAction struct {
	Date         string      // YYYY-MM-DD, ex-date
	CashDividend money.Price // per-share cash distribution (>0 means a dividend)
	SplitRatio   money.Ratio // shares after / shares before (2.0 = 2-for-1); 0/1 means none
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
		// money.Decimal parsing already rejects NaN/Inf, so only the
		// positivity check remains here.
		value := pointValue(p, s.Metadata.PriceType)
		if value.Sign() <= 0 {
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

func pointValue(p PricePoint, priceType PriceType) money.Price {
	if priceType == PriceTypeRawClose {
		// Compatibility for legacy test/providers. New raw adapters populate
		// RawClose and leave AdjustedClose empty.
		if p.RawClose.IsZero() {
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
	closeByDate := make(map[string]money.Price, len(pts))
	for _, p := range pts {
		closeByDate[p.Date] = p.AdjustedClose
	}

	// Cumulative back-adjust factor applied to all dates strictly before each
	// event date. We compute a per-date factor by processing events newest-first.
	acts := append([]CorporateAction(nil), actions...)
	sort.Slice(acts, func(i, j int) bool { return acts[i].Date > acts[j].Date }) // newest first

	one := money.MustRatio("1")
	factor := make([]money.Ratio, len(pts))
	for i := range factor {
		factor[i] = one
	}
	for _, a := range acts {
		f := one
		if a.SplitRatio.Sign() > 0 && !a.SplitRatio.Equal(one.Decimal) {
			inv, err := one.DivExact(a.SplitRatio, intermediatePrecision)
			if err != nil {
				return BenchmarkPriceSeries{}, fmt.Errorf("%w: %s bad corporate action on %s", ErrInvalidBenchmarkSeries, symbol, a.Date)
			}
			f = f.Mul(inv)
		}
		if a.CashDividend.Sign() > 0 {
			// Reference close is the trading day immediately before the ex-date.
			prevClose := priorClose(pts, a.Date)
			if prevClose.Sign() > 0 {
				divRatio, err := a.CashDividend.DivExact(prevClose, intermediatePrecision)
				if err != nil {
					return BenchmarkPriceSeries{}, fmt.Errorf("%w: %s bad corporate action on %s", ErrInvalidBenchmarkSeries, symbol, a.Date)
				}
				f = f.Mul(one.Sub(divRatio))
			}
		}
		if f.Sign() <= 0 {
			return BenchmarkPriceSeries{}, fmt.Errorf("%w: %s bad corporate action on %s", ErrInvalidBenchmarkSeries, symbol, a.Date)
		}
		// Apply to every point strictly before the ex-date.
		for i, p := range pts {
			if p.Date < a.Date {
				factor[i] = factor[i].Mul(f)
			}
		}
	}

	out := make([]PricePoint, len(pts))
	for i, p := range pts {
		adjusted := money.QuantizePrice(p.AdjustedClose.MulRatio(factor[i]))
		out[i] = PricePoint{Date: p.Date, AdjustedClose: adjusted}
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
func priorClose(sorted []PricePoint, ref string) money.Price {
	var last money.Price
	for _, p := range sorted {
		if p.Date < ref {
			last = p.AdjustedClose
			continue
		}
		break
	}
	return last
}
