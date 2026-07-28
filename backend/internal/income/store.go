package income

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Store persists normalized income events and their per-portfolio application
// records. Uniqueness is enforced on (provider, provider_event_id) and
// (income_event_id, portfolio_id) so an event is never applied twice.
type Store interface {
	// UpsertEvent inserts a new event or updates an existing one keyed by ID. It
	// returns changed=true when a material term (fingerprint) changed, which
	// signals a provider correction.
	UpsertEvent(ctx context.Context, ev IncomeEvent) (changed bool, err error)
	ListEventsByStatus(ctx context.Context, statuses ...Status) ([]IncomeEvent, error)
	GetEvent(ctx context.Context, id string) (IncomeEvent, bool, error)
	SetEventStatus(ctx context.Context, id string, status Status) error

	// ClaimApplication atomically creates (or claims) the (event, portfolio)
	// application in the "applying" state. It returns claimed=false when the
	// application already exists in a terminal state (applied/skipped) or is
	// being applied by another worker — the caller must then skip.
	ClaimApplication(ctx context.Context, eventID, portfolioID, userID string) (claimed bool, err error)
	CompleteApplication(ctx context.Context, app Application) error
	FailApplication(ctx context.Context, eventID, portfolioID, errorCode string, nextRetryAt time.Time) error
	SkipApplication(ctx context.Context, eventID, portfolioID, reason string) error
	GetApplication(ctx context.Context, eventID, portfolioID string) (Application, bool, error)
	ListApplicationsForUser(ctx context.Context, userID string) ([]Application, error)
}

// InMemoryStore is the process-local store used for development and tests. It
// mirrors the uniqueness and claiming semantics the Postgres store enforces.
type InMemoryStore struct {
	mu     sync.Mutex
	events map[string]IncomeEvent // id -> event
	apps   map[string]Application // eventID|portfolioID -> application
	now    func() time.Time
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		events: map[string]IncomeEvent{},
		apps:   map[string]Application{},
		now:    func() time.Time { return time.Now().UTC() },
	}
}

func (s *InMemoryStore) OnAccountDeleted(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, application := range s.apps {
		if application.UserID == userID {
			delete(s.apps, key)
		}
	}
	return nil
}

func appKey(eventID, portfolioID string) string { return eventID + "|" + portfolioID }

func (s *InMemoryStore) UpsertEvent(_ context.Context, ev IncomeEvent) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.events[ev.ID]
	if !ok {
		ev.CreatedAt = s.now()
		ev.UpdatedAt = ev.CreatedAt
		s.events[ev.ID] = ev
		return false, nil
	}
	changed := existing.RawFingerprint != ev.RawFingerprint
	if changed {
		ev.CreatedAt = existing.CreatedAt
		ev.UpdatedAt = s.now()
		if existing.Status == StatusApplied {
			// Material change after application: flag for the correction workflow.
			ev.Status = StatusSuperseded
		}
		s.events[ev.ID] = ev
	}
	return changed, nil
}

func (s *InMemoryStore) ListEventsByStatus(_ context.Context, statuses ...Status) ([]IncomeEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[Status]bool{}
	for _, st := range statuses {
		want[st] = true
	}
	out := make([]IncomeEvent, 0)
	for _, ev := range s.events {
		if len(want) == 0 || want[ev.Status] {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PaymentDate.Before(out[j].PaymentDate) })
	return out, nil
}

func (s *InMemoryStore) GetEvent(_ context.Context, id string) (IncomeEvent, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ev, ok := s.events[id]
	return ev, ok, nil
}

func (s *InMemoryStore) SetEventStatus(_ context.Context, id string, status Status) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev, ok := s.events[id]; ok {
		ev.Status = status
		ev.UpdatedAt = s.now()
		s.events[id] = ev
	}
	return nil
}

func (s *InMemoryStore) ClaimApplication(_ context.Context, eventID, portfolioID, userID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := appKey(eventID, portfolioID)
	if app, ok := s.apps[key]; ok {
		switch app.Status {
		case ApplicationApplied, ApplicationSkipped, ApplicationApplying, ApplicationCorrected:
			return false, nil // terminal or in-flight: do not double-apply
		}
	}
	now := s.now()
	s.apps[key] = Application{
		IncomeEventID: eventID, PortfolioID: portfolioID, UserID: userID,
		Status: ApplicationApplying, CreatedAt: now, UpdatedAt: now,
	}
	return true, nil
}

func (s *InMemoryStore) CompleteApplication(_ context.Context, app Application) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	// Respect the caller-set terminal status (applied on first credit, corrected
	// after a reconciliation) rather than forcing a single value, matching the
	// Postgres store.
	if app.Status == "" {
		app.Status = ApplicationApplied
	}
	if app.AppliedAt == nil {
		app.AppliedAt = &now
	}
	app.UpdatedAt = now
	if existing, ok := s.apps[appKey(app.IncomeEventID, app.PortfolioID)]; ok {
		app.CreatedAt = existing.CreatedAt
	}
	s.apps[appKey(app.IncomeEventID, app.PortfolioID)] = app
	return nil
}

func (s *InMemoryStore) FailApplication(_ context.Context, eventID, portfolioID, errorCode string, nextRetryAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := appKey(eventID, portfolioID)
	app := s.apps[key]
	app.IncomeEventID = eventID
	app.PortfolioID = portfolioID
	app.Status = ApplicationFailed
	app.ErrorCode = errorCode
	app.RetryCount++
	nra := nextRetryAt
	app.NextRetryAt = &nra
	app.UpdatedAt = s.now()
	s.apps[key] = app
	return nil
}

func (s *InMemoryStore) SkipApplication(_ context.Context, eventID, portfolioID, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := appKey(eventID, portfolioID)
	app := s.apps[key]
	app.IncomeEventID = eventID
	app.PortfolioID = portfolioID
	app.Status = ApplicationSkipped
	app.ErrorCode = reason
	app.UpdatedAt = s.now()
	s.apps[key] = app
	return nil
}

func (s *InMemoryStore) GetApplication(_ context.Context, eventID, portfolioID string) (Application, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	app, ok := s.apps[appKey(eventID, portfolioID)]
	return app, ok, nil
}

func (s *InMemoryStore) ListApplicationsForUser(_ context.Context, userID string) ([]Application, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Application, 0)
	for _, app := range s.apps {
		if app.UserID == userID {
			out = append(out, app)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
