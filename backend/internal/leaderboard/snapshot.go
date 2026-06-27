package leaderboard

import (
	"context"
	"sort"
	"sync"
	"time"
)

// SnapshotStore persists periodic portfolio-index points per user so the
// leaderboard can compute trailing-window (1W/1M/…) performance. The background
// worker records one point per user per tick via the service's RefreshCache.
//
// TODO: prune points older than the longest supported window (1Y) to bound
// growth; for the prototype we keep them all.
type SnapshotStore interface {
	// Record stores a user's portfolio index at a point in time.
	Record(ctx context.Context, userID string, index float64, at time.Time) error
	// IndexAtOrBefore returns the most recent recorded index at or before cutoff.
	// found=false means there is no history that old (caller falls back to the
	// since-baseline value).
	IndexAtOrBefore(ctx context.Context, userID string, cutoff time.Time) (index float64, found bool, err error)
}

type indexPoint struct {
	at    time.Time
	index float64
}

// InMemorySnapshotStore keeps per-user index time-series in memory. Used by the
// memory storage provider and tests; history is naturally lost on restart.
type InMemorySnapshotStore struct {
	mu       sync.RWMutex
	byUserID map[string][]indexPoint
}

func NewInMemorySnapshotStore() *InMemorySnapshotStore {
	return &InMemorySnapshotStore{byUserID: make(map[string][]indexPoint)}
}

func (s *InMemorySnapshotStore) Record(_ context.Context, userID string, index float64, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pts := append(s.byUserID[userID], indexPoint{at: at.UTC(), index: index})
	// Keep chronological so reads can scan from the end.
	sort.Slice(pts, func(i, j int) bool { return pts[i].at.Before(pts[j].at) })
	s.byUserID[userID] = pts
	return nil
}

func (s *InMemorySnapshotStore) IndexAtOrBefore(_ context.Context, userID string, cutoff time.Time) (float64, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pts := s.byUserID[userID]
	cutoff = cutoff.UTC()
	// Walk newest→oldest; first point at/before cutoff wins.
	for i := len(pts) - 1; i >= 0; i-- {
		if !pts[i].at.After(cutoff) {
			return pts[i].index, true, nil
		}
	}
	return 0, false, nil
}
