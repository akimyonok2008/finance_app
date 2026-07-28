package leaderboard

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ardakimyonok/finance_app/internal/money"
)

// SnapshotStore reads the canonical ranked-performance history used by
// trailing-window leaderboards.
type SnapshotStore interface {
	// IndexAtOrBefore returns the most recent recorded index at or before cutoff
	// and at or after notBefore, plus that snapshot's own timestamp. notBefore is
	// the user's ranking-epoch timestamp: legacy snapshots recorded before the
	// epoch are ignored so timeframe returns are never computed against a
	// manipulable pre-epoch index. A zero notBefore disables the lower bound.
	// found=false means there is no eligible history that old (the user is
	// excluded from that timeframe rather than mis-ranked). capturedAt lets the
	// caller additionally bound how far the found snapshot may sit before
	// cutoff — without it, a big gap in recorded history (a missed snapshot
	// window, a paused-then-resumed account) would silently stretch what's
	// labeled e.g. "1W" into a much longer real span.
	IndexAtOrBefore(ctx context.Context, userID string, cutoff, notBefore time.Time) (index money.IndexValue, capturedAt time.Time, found bool, err error)
}

type indexPoint struct {
	at    time.Time
	index money.IndexValue
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

func (s *InMemorySnapshotStore) Record(_ context.Context, userID string, index money.IndexValue, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pts := append(s.byUserID[userID], indexPoint{at: at.UTC(), index: index})
	// Keep chronological so reads can scan from the end.
	sort.Slice(pts, func(i, j int) bool { return pts[i].at.Before(pts[j].at) })
	s.byUserID[userID] = pts
	return nil
}

func (s *InMemorySnapshotStore) IndexAtOrBefore(_ context.Context, userID string, cutoff, notBefore time.Time) (money.IndexValue, time.Time, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pts := s.byUserID[userID]
	cutoff = cutoff.UTC()
	notBefore = notBefore.UTC()
	hasFloor := !notBefore.IsZero()
	// Walk newest→oldest; first point at/before cutoff (and at/after the epoch) wins.
	for i := len(pts) - 1; i >= 0; i-- {
		if pts[i].at.After(cutoff) {
			continue
		}
		if hasFloor && pts[i].at.Before(notBefore) {
			return money.ZeroIndexValue(), time.Time{}, false, nil // only pre-epoch history remains: ignore it
		}
		return pts[i].index, pts[i].at, true, nil
	}
	return money.ZeroIndexValue(), time.Time{}, false, nil
}
