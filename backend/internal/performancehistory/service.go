package performancehistory

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

type RankedProvider interface {
	CurrentRankedPerformance(ctx context.Context, userID string) (*performance.RankedPerformance, error)
}

type Config struct {
	IntradayInterval     time.Duration
	BoundaryTolerance    time.Duration
	EndFreshness         time.Duration
	MaxTrustedGap        time.Duration
	EligibilityThreshold float64
	IntradayRetention    time.Duration
}

func DefaultConfig() Config {
	return Config{
		IntradayInterval:     4 * time.Hour,
		BoundaryTolerance:    36 * time.Hour,
		EndFreshness:         8 * time.Hour,
		MaxTrustedGap:        36 * time.Hour,
		EligibilityThreshold: 0.90,
		IntradayRetention:    120 * 24 * time.Hour,
	}
}

type Service struct {
	repo   Repository
	ranked RankedProvider
	cfg    Config
	now    func() time.Time
}

func NewService(repo Repository, ranked RankedProvider, cfg Config) *Service {
	defaults := DefaultConfig()
	if cfg.IntradayInterval <= 0 {
		cfg.IntradayInterval = defaults.IntradayInterval
	}
	if cfg.BoundaryTolerance <= 0 {
		cfg.BoundaryTolerance = defaults.BoundaryTolerance
	}
	if cfg.EndFreshness <= 0 {
		cfg.EndFreshness = defaults.EndFreshness
	}
	if cfg.MaxTrustedGap <= 0 {
		cfg.MaxTrustedGap = defaults.MaxTrustedGap
	}
	if cfg.EligibilityThreshold <= 0 || cfg.EligibilityThreshold > 1 {
		cfg.EligibilityThreshold = defaults.EligibilityThreshold
	}
	if cfg.IntradayRetention <= 0 {
		cfg.IntradayRetention = defaults.IntradayRetention
	}
	return &Service{repo: repo, ranked: ranked, cfg: cfg, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) SetClock(now func() time.Time) { s.now = now }
func (s *Service) EligibilityThreshold() float64 { return s.cfg.EligibilityThreshold }
func (s *Service) SnapshotFrequency() string     { return s.cfg.IntradayInterval.String() }

func bucketStart(at time.Time, interval time.Duration) time.Time {
	return at.UTC().Truncate(interval)
}

func quality(value string) QualityStatus {
	switch QualityStatus(value) {
	case QualityComplete, QualityStale, QualityPartial, QualityInvalid:
		return QualityStatus(value)
	default:
		return QualityInvalid
	}
}

// RecordCurrent persists one interval-bucket point and one canonical UTC-daily
// point. Database uniqueness makes concurrent workers idempotent.
func (s *Service) RecordCurrent(ctx context.Context, userID string) (int, QualityStatus, error) {
	ranked, err := s.ranked.CurrentRankedPerformance(ctx, userID)
	if err != nil {
		return 0, QualityInvalid, err
	}
	if ranked.PortfolioID == "" || ranked.TrackingStartedAt.IsZero() ||
		ranked.RankedIndex <= 0 || math.IsNaN(ranked.RankedIndex) || math.IsInf(ranked.RankedIndex, 0) {
		return 0, QualityInvalid, ErrInvalidSnapshot
	}
	now := s.now()
	q := quality(ranked.DataQualityStatus)
	asOf := ranked.ValuationAsOf
	if asOf.IsZero() {
		asOf = now
	}
	bucket := bucketStart(now, s.cfg.IntradayInterval)
	base := Snapshot{
		PortfolioID: ranked.PortfolioID, UserID: userID,
		TrackingStartedAt: ranked.TrackingStartedAt.UTC(),
		RankedIndex:       ranked.RankedIndex, RankingStatus: ranked.Status,
		CapturedAt: now, ValuationAsOf: asOf.UTC(),
		DataQualityStatus: q, CreatedAt: now,
	}
	intraday := base
	intraday.ID = uuid.NewString()
	intraday.Kind = KindIntraday
	intraday.BucketStart = &bucket
	daily := base
	daily.ID = uuid.NewString()
	daily.Kind = KindDaily
	daily.SnapshotDate = now.Format("2006-01-02")

	count := 0
	if wrote, err := s.repo.Insert(ctx, intraday); err != nil {
		return count, q, err
	} else if wrote {
		count++
	}
	if wrote, err := s.repo.Insert(ctx, daily); err != nil {
		return count, q, err
	} else if wrote {
		count++
	}
	return count, q, nil
}

func (s *Service) RecordTransition(ctx context.Context, input performance.TransitionSnapshot) error {
	q := quality(input.DataQualityStatus)
	snapshot := Snapshot{
		ID: uuid.NewString(), PortfolioID: input.PortfolioID, UserID: input.UserID,
		TrackingStartedAt: input.TrackingStartedAt.UTC(),
		RankedIndex:       input.RankedIndex, RankingStatus: input.Status,
		CapturedAt: input.CapturedAt.UTC(), Kind: KindTransition,
		ValuationAsOf: input.ValuationAsOf.UTC(), DataQualityStatus: q,
		CreatedAt: s.now(),
	}
	_, err := s.repo.Insert(ctx, snapshot)
	return err
}

// RecordTransitionIfChanged persists a precise lifecycle boundary while
// suppressing ordinary same-status portfolio mutations.
func (s *Service) RecordTransitionIfChanged(ctx context.Context, input performance.TransitionSnapshot) (bool, error) {
	previous, found, err := s.repo.StatusAtOrBefore(
		ctx, input.PortfolioID, input.TrackingStartedAt, input.CapturedAt.Add(-time.Nanosecond),
	)
	if err != nil {
		return false, err
	}
	if found && previous == input.Status {
		return false, nil
	}
	if err := s.RecordTransition(ctx, input); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) RecordCurrentTransition(ctx context.Context, userID string) (bool, error) {
	ranked, err := s.ranked.CurrentRankedPerformance(ctx, userID)
	if err != nil {
		return false, err
	}
	now := s.now()
	asOf := ranked.ValuationAsOf
	if asOf.IsZero() {
		asOf = now
	}
	return s.RecordTransitionIfChanged(ctx, performance.TransitionSnapshot{
		PortfolioID: ranked.PortfolioID, UserID: userID,
		TrackingStartedAt: ranked.TrackingStartedAt,
		RankedIndex:       ranked.RankedIndex, Status: ranked.Status,
		CapturedAt: now, ValuationAsOf: asOf,
		DataQualityStatus: ranked.DataQualityStatus,
	})
}

func (s *Service) IndexAtOrBefore(ctx context.Context, userID string, cutoff, epoch time.Time) (float64, bool, error) {
	return s.repo.IndexAtOrBefore(ctx, userID, cutoff, epoch)
}

// Series returns complete points from the newest epoch only. It is suitable for
// ranked charts and never mixes legacy or cost-basis archive history.
func (s *Service) Series(ctx context.Context, userID string, start, end time.Time) ([]Snapshot, error) {
	points, err := s.repo.List(ctx, userID, start, end)
	if err != nil {
		return nil, err
	}
	points = collapse(points)
	var epoch time.Time
	for i := len(points) - 1; i >= 0; i-- {
		if points[i].DataQualityStatus == QualityComplete {
			epoch = points[i].TrackingStartedAt
			break
		}
	}
	out := []Snapshot{}
	for _, point := range points {
		if point.DataQualityStatus == QualityComplete && point.TrackingStartedAt.Equal(epoch) {
			out = append(out, point)
		}
	}
	return out, nil
}

// Window selects deterministic trusted boundaries and calculates lifecycle and
// data-quality coverage across the exact selected interval.
func (s *Service) Window(ctx context.Context, userID string, targetStart, targetEnd time.Time) (Window, error) {
	if !targetEnd.After(targetStart) {
		return Window{}, ErrWindowNotReady
	}
	points, err := s.repo.List(ctx, userID, targetStart.Add(-s.cfg.BoundaryTolerance), targetEnd)
	if err != nil {
		return Window{}, err
	}
	points = collapse(points)
	var end Snapshot
	for i := len(points) - 1; i >= 0; i-- {
		p := points[i]
		if p.DataQualityStatus == QualityComplete && !p.CapturedAt.After(targetEnd) {
			end = p
			break
		}
	}
	if end.ID == "" || targetEnd.Sub(end.CapturedAt) > s.cfg.EndFreshness {
		return Window{}, ErrWindowNotReady
	}
	epoch := end.TrackingStartedAt
	eligible := make([]Snapshot, 0, len(points))
	for _, p := range points {
		if p.TrackingStartedAt.Equal(epoch) && !p.CapturedAt.Before(epoch) {
			eligible = append(eligible, p)
		}
	}
	var start Snapshot
	for _, p := range eligible {
		if p.DataQualityStatus != QualityComplete {
			continue
		}
		if !p.CapturedAt.After(targetStart) {
			if start.ID == "" || p.CapturedAt.After(start.CapturedAt) {
				start = p
			}
		}
	}
	if start.ID == "" {
		for _, p := range eligible {
			if p.DataQualityStatus == QualityComplete && p.CapturedAt.After(targetStart) &&
				p.CapturedAt.Sub(targetStart) <= s.cfg.BoundaryTolerance {
				start = p
				break
			}
		}
	}
	if start.ID == "" || absDuration(start.CapturedAt.Sub(targetStart)) > s.cfg.BoundaryTolerance ||
		!end.CapturedAt.After(start.CapturedAt) {
		return Window{}, ErrWindowNotReady
	}
	windowPoints := []Snapshot{}
	for _, p := range eligible {
		if !p.CapturedAt.Before(start.CapturedAt) && !p.CapturedAt.After(end.CapturedAt) {
			windowPoints = append(windowPoints, p)
		}
	}
	duration := end.CapturedAt.Sub(start.CapturedAt)
	nominal := targetEnd.Sub(targetStart)
	historyCoverage := clamp(float64(duration) / float64(nominal))
	var active, trusted time.Duration
	for i := 0; i+1 < len(windowPoints); i++ {
		left, right := windowPoints[i], windowPoints[i+1]
		span := right.CapturedAt.Sub(left.CapturedAt)
		if span <= 0 {
			continue
		}
		if left.RankingStatus == performance.StatusActive {
			active += span
		}
		if left.DataQualityStatus == QualityComplete && right.DataQualityStatus == QualityComplete &&
			span <= s.cfg.MaxTrustedGap {
			trusted += span
		}
	}
	return Window{
		StartSnapshot: start, EndSnapshot: end, Points: windowPoints,
		HistoryCoverage: historyCoverage,
		ActiveCoverage:  clamp(float64(active) / float64(duration)),
		TrustedCoverage: clamp(float64(trusted) / float64(duration)),
	}, nil
}

func collapse(points []Snapshot) []Snapshot {
	sort.Slice(points, func(i, j int) bool {
		if points[i].CapturedAt.Equal(points[j].CapturedAt) {
			left, right := snapshotKindPriority(points[i].Kind), snapshotKindPriority(points[j].Kind)
			if left == right {
				return points[i].ID < points[j].ID
			}
			return left < right
		}
		return points[i].CapturedAt.Before(points[j].CapturedAt)
	})
	out := make([]Snapshot, 0, len(points))
	for _, point := range points {
		if len(out) > 0 && point.CapturedAt.Equal(out[len(out)-1].CapturedAt) {
			if point.Kind == KindTransition {
				out[len(out)-1] = point
			}
			continue
		}
		out = append(out, point)
	}
	return out
}

func snapshotKindPriority(kind SnapshotKind) int {
	switch kind {
	case KindDaily:
		return 0
	case KindIntraday:
		return 1
	case KindTransition:
		return 2
	default:
		return 3
	}
}

func (s *Service) ProtectEvidence(ctx context.Context, ids ...string) error {
	return s.repo.Protect(ctx, ids...)
}

func (s *Service) Compact(ctx context.Context) (int64, error) {
	return s.repo.Compact(ctx, s.now().Add(-s.cfg.IntradayRetention))
}

func (s *Service) ClaimEvaluations(ctx context.Context, limit int) ([]EvaluationClaim, error) {
	return s.repo.ClaimEvaluations(ctx, limit, s.now().Add(-15*time.Minute))
}

func (s *Service) CompleteEvaluation(ctx context.Context, id string) error {
	return s.repo.CompleteEvaluation(ctx, id)
}

func (s *Service) FailEvaluation(ctx context.Context, id string, err error) error {
	cause := "evaluation failed"
	if err != nil {
		cause = err.Error()
	}
	return s.repo.FailEvaluation(ctx, id, cause)
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func clamp(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func IsNotReady(err error) bool { return errors.Is(err, ErrWindowNotReady) }
