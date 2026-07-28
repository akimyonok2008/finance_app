package performancehistory

import (
	"errors"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
	"github.com/ardakimyonok/finance_app/internal/performance"
)

type SnapshotKind string

const (
	KindIntraday   SnapshotKind = "intraday"
	KindDaily      SnapshotKind = "daily"
	KindTransition SnapshotKind = "transition"
)

type QualityStatus string

const (
	QualityComplete QualityStatus = "complete"
	QualityStale    QualityStatus = "stale"
	QualityPartial  QualityStatus = "partial"
	QualityInvalid  QualityStatus = "invalid"
)

var (
	ErrInvalidSnapshot = errors.New("invalid ranked snapshot")
	ErrWindowNotReady  = errors.New("ranked snapshot window is not eligible")
)

// Snapshot contains only ranked-index and lifecycle metadata. It intentionally
// excludes values, quantities, cost basis, and segment starting value.
type Snapshot struct {
	ID                string
	PortfolioID       string
	UserID            string
	TrackingStartedAt time.Time
	RankedIndex       money.IndexValue
	RankingStatus     performance.Status
	CapturedAt        time.Time
	Kind              SnapshotKind
	BucketStart       *time.Time
	SnapshotDate      string
	ValuationAsOf     time.Time
	DataQualityStatus QualityStatus
	EvidenceProtected bool
	CreatedAt         time.Time
}

func (s Snapshot) Valid() bool {
	baseValid := s.ID != "" && s.PortfolioID != "" && s.UserID != "" &&
		!s.TrackingStartedAt.IsZero() && !s.CapturedAt.IsZero() &&
		!s.ValuationAsOf.IsZero() &&
		s.RankedIndex.Sign() > 0 &&
		(s.RankingStatus == performance.StatusActive || s.RankingStatus == performance.StatusPaused) &&
		(s.DataQualityStatus == QualityComplete || s.DataQualityStatus == QualityStale ||
			s.DataQualityStatus == QualityPartial || s.DataQualityStatus == QualityInvalid)
	if !baseValid {
		return false
	}
	switch s.Kind {
	case KindIntraday:
		return s.BucketStart != nil && s.SnapshotDate == ""
	case KindDaily:
		return s.BucketStart == nil && s.SnapshotDate != ""
	case KindTransition:
		return s.BucketStart == nil && s.SnapshotDate == ""
	default:
		return false
	}
}

type Window struct {
	StartSnapshot   Snapshot
	EndSnapshot     Snapshot
	Points          []Snapshot
	HistoryCoverage float64
	ActiveCoverage  float64
	TrustedCoverage float64
}

func (w Window) Eligible(threshold float64) bool {
	return w.StartSnapshot.ID != "" && w.EndSnapshot.ID != "" &&
		w.StartSnapshot.TrackingStartedAt.Equal(w.EndSnapshot.TrackingStartedAt) &&
		w.HistoryCoverage >= threshold &&
		w.ActiveCoverage >= threshold &&
		w.TrustedCoverage >= threshold
}

type EvaluationClaim struct {
	SnapshotID string
	UserID     string
}

type BatchResult struct {
	UsersProcessed    int
	SnapshotsCreated  int
	SkippedValuations int
	StaleSnapshots    int
	Failures          int
}
