package performancehistory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ardakimyonok/finance_app/internal/performance"
)

type Repository interface {
	Insert(ctx context.Context, snapshot Snapshot) (bool, error)
	List(ctx context.Context, userID string, from, to time.Time) ([]Snapshot, error)
	IndexAtOrBefore(ctx context.Context, userID string, cutoff, epoch time.Time) (float64, time.Time, bool, error)
	Latest(ctx context.Context, userID, portfolioID string, epoch time.Time) (Snapshot, bool, error)
	LatestTrustedCapturedAt(ctx context.Context, userID, portfolioID string, epoch time.Time) (time.Time, bool, error)
	StatusAtOrBefore(ctx context.Context, portfolioID string, epoch, at time.Time) (performance.Status, bool, error)
	Protect(ctx context.Context, ids ...string) error
	Compact(ctx context.Context, intradayBefore time.Time) (int64, error)
	ClaimEvaluations(ctx context.Context, limit int, retryBefore time.Time) ([]EvaluationClaim, error)
	CompleteEvaluation(ctx context.Context, snapshotID string) error
	FailEvaluation(ctx context.Context, snapshotID, cause string) error
}

type memoryRecord struct {
	Snapshot
	evaluationStatus string
	claimedAt        time.Time
}

type InMemoryRepository struct {
	mu      sync.Mutex
	records map[string]memoryRecord
	keys    map[string]string
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{records: map[string]memoryRecord{}, keys: map[string]string{}}
}

func snapshotKey(s Snapshot) string {
	switch s.Kind {
	case KindDaily:
		return s.PortfolioID + "|" + s.TrackingStartedAt.UTC().Format(time.RFC3339Nano) + "|daily|" + s.SnapshotDate
	case KindIntraday:
		if s.BucketStart == nil {
			return ""
		}
		return s.PortfolioID + "|" + s.TrackingStartedAt.UTC().Format(time.RFC3339Nano) + "|intraday|" + s.BucketStart.UTC().Format(time.RFC3339Nano)
	default:
		return s.PortfolioID + "|" + s.TrackingStartedAt.UTC().Format(time.RFC3339Nano) + "|transition|" + s.CapturedAt.UTC().Format(time.RFC3339Nano)
	}
}

func (r *InMemoryRepository) Insert(_ context.Context, s Snapshot) (bool, error) {
	if !s.Valid() || snapshotKey(s) == "" {
		return false, ErrInvalidSnapshot
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := snapshotKey(s)
	if existingID, ok := r.keys[key]; ok {
		existing := r.records[existingID]
		if existing.DataQualityStatus != QualityComplete && s.DataQualityStatus == QualityComplete {
			s.ID = existing.ID
			s.EvidenceProtected = existing.EvidenceProtected
			r.records[existingID] = memoryRecord{Snapshot: s, evaluationStatus: "pending"}
			return true, nil
		}
		return false, nil
	}
	status := "done"
	if s.DataQualityStatus == QualityComplete && s.Kind != KindDaily {
		status = "pending"
	}
	r.keys[key] = s.ID
	r.records[s.ID] = memoryRecord{Snapshot: s, evaluationStatus: status}
	return true, nil
}

func (r *InMemoryRepository) List(_ context.Context, userID string, from, to time.Time) ([]Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []Snapshot{}
	for _, record := range r.records {
		if record.UserID == userID && !record.CapturedAt.Before(from) && !record.CapturedAt.After(to) {
			out = append(out, record.Snapshot)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CapturedAt.Before(out[j].CapturedAt) })
	return out, nil
}

func (r *InMemoryRepository) IndexAtOrBefore(_ context.Context, userID string, cutoff, epoch time.Time) (float64, time.Time, bool, error) {
	points, _ := r.List(context.Background(), userID, epoch, cutoff)
	for i := len(points) - 1; i >= 0; i-- {
		if points[i].DataQualityStatus == QualityComplete && points[i].TrackingStartedAt.Equal(epoch) {
			return points[i].RankedIndex, points[i].CapturedAt, true, nil
		}
	}
	return 0, time.Time{}, false, nil
}

func (r *InMemoryRepository) Latest(_ context.Context, userID, portfolioID string, epoch time.Time) (Snapshot, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest Snapshot
	found := false
	for _, record := range r.records {
		s := record.Snapshot
		if s.UserID == userID && s.PortfolioID == portfolioID && s.TrackingStartedAt.Equal(epoch) &&
			(!found || s.CapturedAt.After(latest.CapturedAt)) {
			latest, found = s, true
		}
	}
	return latest, found, nil
}

func (r *InMemoryRepository) LatestTrustedCapturedAt(_ context.Context, userID, portfolioID string, epoch time.Time) (time.Time, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest time.Time
	for _, record := range r.records {
		s := record.Snapshot
		if s.UserID == userID && s.PortfolioID == portfolioID &&
			s.TrackingStartedAt.Equal(epoch) && s.DataQualityStatus == QualityComplete &&
			s.CapturedAt.After(latest) {
			latest = s.CapturedAt
		}
	}
	return latest, !latest.IsZero(), nil
}

func (r *InMemoryRepository) StatusAtOrBefore(_ context.Context, portfolioID string, epoch, at time.Time) (performance.Status, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest Snapshot
	found := false
	for _, record := range r.records {
		s := record.Snapshot
		if s.PortfolioID == portfolioID && s.TrackingStartedAt.Equal(epoch) &&
			!s.CapturedAt.After(at) && (!found || s.CapturedAt.After(latest.CapturedAt)) {
			latest, found = s, true
		}
	}
	return latest.RankingStatus, found, nil
}

func (r *InMemoryRepository) Protect(_ context.Context, ids ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		if record, ok := r.records[id]; ok {
			record.EvidenceProtected = true
			r.records[id] = record
		}
	}
	return nil
}

func (r *InMemoryRepository) OnAccountDeleted(_ context.Context, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, record := range r.records {
		if record.UserID == userID {
			delete(r.keys, snapshotKey(record.Snapshot))
			delete(r.records, id)
		}
	}
	return nil
}

func (r *InMemoryRepository) Compact(_ context.Context, before time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	dailyDates := map[string]bool{}
	for _, record := range r.records {
		if record.Kind == KindDaily && record.DataQualityStatus == QualityComplete {
			dailyDates[record.PortfolioID+"|"+record.TrackingStartedAt.Format(time.RFC3339Nano)+"|"+record.SnapshotDate] = true
		}
	}
	var removed int64
	for id, record := range r.records {
		if record.Kind != KindIntraday || !record.CapturedAt.Before(before) || record.EvidenceProtected {
			continue
		}
		key := record.PortfolioID + "|" + record.TrackingStartedAt.Format(time.RFC3339Nano) + "|" + record.CapturedAt.UTC().Format("2006-01-02")
		if !dailyDates[key] {
			continue
		}
		delete(r.keys, snapshotKey(record.Snapshot))
		delete(r.records, id)
		removed++
	}
	return removed, nil
}

func (r *InMemoryRepository) ClaimEvaluations(_ context.Context, limit int, retryBefore time.Time) ([]EvaluationClaim, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []EvaluationClaim{}
	for id, record := range r.records {
		if len(out) >= limit {
			break
		}
		if record.evaluationStatus == "pending" ||
			(record.evaluationStatus == "processing" && record.claimedAt.Before(retryBefore)) {
			record.evaluationStatus = "processing"
			record.claimedAt = time.Now().UTC()
			r.records[id] = record
			out = append(out, EvaluationClaim{SnapshotID: id, UserID: record.UserID})
		}
	}
	return out, nil
}

func (r *InMemoryRepository) CompleteEvaluation(_ context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.records[id]
	record.evaluationStatus = "done"
	r.records[id] = record
	return nil
}

func (r *InMemoryRepository) FailEvaluation(_ context.Context, id, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.records[id]
	record.evaluationStatus = "pending"
	r.records[id] = record
	return nil
}
