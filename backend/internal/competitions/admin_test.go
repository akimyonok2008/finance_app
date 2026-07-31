package competitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/ardakimyonok/finance_app/internal/clock"
)

type adminDefinitionRepo struct {
	defs     map[string]Definition
	versions map[string]DefinitionVersion
}

func (r *adminDefinitionRepo) CreateDefinition(_ context.Context, d Definition) error {
	for _, existing := range r.defs {
		if existing.Slug == d.Slug {
			return ErrDefinitionExists
		}
	}
	r.defs[d.ID] = d
	return nil
}
func (r *adminDefinitionRepo) GetDefinition(_ context.Context, id string) (*Definition, error) {
	d, ok := r.defs[id]
	if !ok {
		return nil, ErrDefinitionNotFound
	}
	return &d, nil
}
func (r *adminDefinitionRepo) GetDefinitionBySlug(_ context.Context, slug string) (*Definition, error) {
	for _, d := range r.defs {
		if d.Slug == slug {
			return &d, nil
		}
	}
	return nil, ErrDefinitionNotFound
}
func (r *adminDefinitionRepo) ListDefinitions(_ context.Context, _ bool) ([]Definition, error) {
	out := make([]Definition, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d)
	}
	return out, nil
}
func (r *adminDefinitionRepo) CreateDefinitionVersion(_ context.Context, v DefinitionVersion) error {
	d, ok := r.defs[v.DefinitionID]
	if !ok {
		return ErrDefinitionNotFound
	}
	key := fmt.Sprintf("%s:%d", v.DefinitionID, v.Version)
	if _, exists := r.versions[key]; exists || v.Version != d.CurrentVersion+1 {
		return ErrDefinitionVersionExists
	}
	r.versions[key] = v
	d.CurrentVersion = v.Version
	r.defs[d.ID] = d
	return nil
}
func (r *adminDefinitionRepo) GetDefinitionVersion(_ context.Context, id string, version int64) (*DefinitionVersion, error) {
	v, ok := r.versions[fmt.Sprintf("%s:%d", id, version)]
	if !ok {
		return nil, ErrDefinitionVersionNotFound
	}
	return &v, nil
}

type adminOpsRepo struct{ audit []AdminAuditRecord }

func (r *adminOpsRepo) RecordAdminAudit(_ context.Context, a AdminAuditRecord) error {
	r.audit = append(r.audit, a)
	return nil
}
func (r *adminOpsRepo) OperationalStatus(context.Context, string) (*AdminOperationalStatus, error) {
	return &AdminOperationalStatus{}, nil
}
func (r *adminOpsRepo) ListAdminAudit(_ context.Context, competitionID string, limit int) ([]AdminAuditRecord, error) {
	return r.audit, nil
}

func validAdminRules() (json.RawMessage, json.RawMessage) {
	return json.RawMessage(`{"schema_version":1,"all":[{"code":"positions","label":"Has a position","metric":"position_count","operator":"gte","value":"1"}]}`),
		json.RawMessage(`{"schema_version":1,"scope":"full_portfolio","include_cash":true}`)
}

func TestAdminServiceCreatesImmutableVersionAndDraftEdition(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	defs := &adminDefinitionRepo{defs: map[string]Definition{}, versions: map[string]DefinitionVersion{}}
	ops := &adminOpsRepo{}
	editions := NewInMemoryCompetitionRepository()
	engine := NewService(editions, nil, nil, nil, nil, &clock.FixedClock{Time: now})
	admin := NewAdminService(engine, defs, editions, ops)
	admin.clock = func() time.Time { return now }

	def, err := admin.CreateDefinition(ctx, "actor", "request", Definition{Slug: "etf-battle", Name: "ETF Battle", IsEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	eligibility, scoring := validAdminRules()
	version, err := admin.CreateVersion(ctx, "actor", "request", def.ID, DefinitionVersion{
		EligibilityRulesJSON: eligibility, ScoringRulesJSON: scoring,
	})
	if err != nil {
		t.Fatal(err)
	}
	if version.Version != 1 || version.CreatedBy != "actor" {
		t.Fatalf("unexpected version: %+v", version)
	}
	joinOpen, joinClose := now.Add(time.Hour), now.Add(2*time.Hour)
	edition, err := admin.CreateEdition(ctx, "actor", "request", Competition{
		Name: "August ETF Battle", DefinitionID: def.ID,
		JoinOpensAt: &joinOpen, JoinClosesAt: &joinClose,
		StartsAt: now.Add(3 * time.Hour), EndsAt: now.Add(27 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if edition.LifecycleStatus != LifecycleDraft || edition.DefinitionVersion != 1 || len(edition.RulesSnapshotJSON) == 0 {
		t.Fatalf("edition was not stamped as an immutable draft: %+v", edition)
	}
	if len(ops.audit) != 3 {
		t.Fatalf("want 3 audit events, got %d", len(ops.audit))
	}
}

func TestAdminServiceRejectsInvalidRulesBeforePersistence(t *testing.T) {
	ctx := context.Background()
	defs := &adminDefinitionRepo{
		defs:     map[string]Definition{"d": {ID: "d", Slug: "test", Name: "Test", IsEnabled: true}},
		versions: map[string]DefinitionVersion{},
	}
	admin := NewAdminService(nil, defs, nil, &adminOpsRepo{})
	_, err := admin.CreateVersion(ctx, "actor", "request", "d", DefinitionVersion{
		EligibilityRulesJSON: json.RawMessage(`{"schema_version":99}`),
		ScoringRulesJSON:     json.RawMessage(`{"schema_version":1,"scope":"full_portfolio","include_cash":true}`),
	})
	if !errors.Is(err, ErrInvalidRuleDocument) {
		t.Fatalf("want invalid rule document, got %v", err)
	}
	if len(defs.versions) != 0 {
		t.Fatal("invalid version was persisted")
	}
}
