package competitions

import (
	"context"
	"errors"
	"testing"
)

type testCompetitionOutbox struct {
	events            []CompetitionOutboxEvent
	processed, failed int
}

func (s *testCompetitionOutbox) Claim(context.Context, int) ([]CompetitionOutboxEvent, error) {
	events := s.events
	s.events = nil
	return events, nil
}
func (s *testCompetitionOutbox) Processed(context.Context, string) error      { s.processed++; return nil }
func (s *testCompetitionOutbox) Failed(context.Context, string, string) error { s.failed++; return nil }

type testCompetitionAchievements struct {
	calls  int
	failAt int
}

func (a *testCompetitionAchievements) EvaluateCompetitionFinalizationAchievements(context.Context, string, string) error {
	a.calls++
	if a.calls == a.failAt {
		return errors.New("temporary")
	}
	return nil
}

func TestCompetitionAchievementProjectorSettlesOnlyCompleteEvent(t *testing.T) {
	store := &testCompetitionOutbox{events: []CompetitionOutboxEvent{
		{ID: "u1-event", EventType: CompetitionFinalizedEvent, CompetitionID: "competition", ParticipantID: "u1"},
		{ID: "u2-event", EventType: CompetitionFinalizedEvent, CompetitionID: "competition", ParticipantID: "u2"},
	}}
	achievements := &testCompetitionAchievements{failAt: 2}
	p := NewCompetitionAchievementProjector(store, achievements, 0)
	if err := p.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.processed != 1 || store.failed != 1 {
		t.Fatalf("participants must settle independently: processed=%d failed=%d", store.processed, store.failed)
	}
	store.events = []CompetitionOutboxEvent{{ID: "u2-event", EventType: CompetitionFinalizedEvent, CompetitionID: "competition", ParticipantID: "u2"}}
	achievements.failAt = 0
	if err := p.ProcessOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.processed != 2 || achievements.calls != 3 {
		t.Fatalf("successful replay was not settled")
	}
}
