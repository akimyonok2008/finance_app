package competitions

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ardakimyonok/finance_app/internal/competitions/rules"
)

var definitionSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

const (
	AdminJobBaseline     = "baseline"
	AdminJobRanking      = "ranking"
	AdminJobFinalization = "finalization"
)

type AdminAuditRecord struct {
	ID            string          `json:"id"`
	ActorUserID   string          `json:"actor_user_id,omitempty"`
	Action        string          `json:"action"`
	TargetType    string          `json:"target_type"`
	TargetID      string          `json:"target_id"`
	CompetitionID string          `json:"competition_id,omitempty"`
	RequestID     string          `json:"request_id,omitempty"`
	Reason        string          `json:"reason,omitempty"`
	Details       json.RawMessage `json:"details"`
	Succeeded     bool            `json:"succeeded"`
	ErrorMessage  string          `json:"error_message,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

type AdminOperationalStatus struct {
	Competition       Competition              `json:"competition"`
	EntryStatusCounts map[string]int           `json:"entry_status_counts"`
	BaselineCounts    map[string]int           `json:"baseline_status_counts"`
	Disqualifications []AdminFailureDetail     `json:"disqualifications"`
	RankingFailures   []RankingGeneration      `json:"ranking_generations"`
	ObservationSets   []AdminObservationStatus `json:"observation_sets"`
	ResultCount       int                      `json:"result_count"`
}

type AdminFailureDetail struct {
	EntryID string `json:"entry_id"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
}

type AdminObservationStatus struct {
	BoundaryType string     `json:"boundary_type"`
	Status       string     `json:"status"`
	EffectiveAt  time.Time  `json:"effective_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	PriceCount   int        `json:"price_count"`
	FXCount      int        `json:"fx_count"`
}

type CompetitionAdminRepository interface {
	RecordAdminAudit(context.Context, AdminAuditRecord) error
	OperationalStatus(context.Context, string) (*AdminOperationalStatus, error)
	ListAdminAudit(context.Context, string, int) ([]AdminAuditRecord, error)
}

type AdminService struct {
	engine      *Service
	definitions DefinitionRepository
	editions    EditionRepository
	operations  CompetitionAdminRepository
	clock       func() time.Time
}

func NewAdminService(engine *Service, definitions DefinitionRepository, editions EditionRepository, operations CompetitionAdminRepository) *AdminService {
	return &AdminService{engine: engine, definitions: definitions, editions: editions, operations: operations, clock: func() time.Time { return time.Now().UTC() }}
}

func (s *AdminService) ValidateRules(eligibility, scoring json.RawMessage) (rules.Eligibility, rules.Scoring, error) {
	return rules.ValidateDefinitionVersionPayloads(eligibility, scoring)
}

func (s *AdminService) ListDefinitions(ctx context.Context) ([]Definition, error) {
	return s.definitions.ListDefinitions(ctx, false)
}

func (s *AdminService) CreateDefinition(ctx context.Context, actor, requestID string, def Definition) (*Definition, error) {
	def.ID = uuid.NewString()
	def.Slug = strings.TrimSpace(strings.ToLower(def.Slug))
	def.Name = strings.TrimSpace(def.Name)
	if !definitionSlugPattern.MatchString(def.Slug) || def.Name == "" {
		return nil, fmt.Errorf("slug and name are required; slug must be lowercase kebab-case")
	}
	if len(def.PresentationConfigJSON) == 0 {
		def.PresentationConfigJSON = json.RawMessage(`{}`)
	}
	err := s.definitions.CreateDefinition(ctx, def)
	auditErr := s.audit(ctx, actor, requestID, "definition.create", "definition", def.ID, "", "", def, err)
	if err != nil {
		return nil, err
	}
	if auditErr != nil {
		return nil, auditErr
	}
	return s.definitions.GetDefinition(ctx, def.ID)
}

func (s *AdminService) CreateVersion(ctx context.Context, actor, requestID, definitionID string, v DefinitionVersion) (*DefinitionVersion, error) {
	def, err := s.definitions.GetDefinition(ctx, definitionID)
	if err != nil {
		return nil, err
	}
	if _, _, err = s.ValidateRules(v.EligibilityRulesJSON, v.ScoringRulesJSON); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRuleDocument, err)
	}
	v.DefinitionID, v.Version, v.CreatedBy = definitionID, def.CurrentVersion+1, actor
	if len(v.ScheduleDefaultsJSON) == 0 {
		v.ScheduleDefaultsJSON = json.RawMessage(`{}`)
	}
	if len(v.DisplayRulesJSON) == 0 {
		v.DisplayRulesJSON = json.RawMessage(`{}`)
	}
	err = s.definitions.CreateDefinitionVersion(ctx, v)
	auditErr := s.audit(ctx, actor, requestID, "definition_version.create", "definition_version", fmt.Sprintf("%s:%d", definitionID, v.Version), "", "", map[string]any{"definition_id": definitionID, "version": v.Version}, err)
	if err != nil {
		return nil, err
	}
	if auditErr != nil {
		return nil, auditErr
	}
	return s.definitions.GetDefinitionVersion(ctx, definitionID, v.Version)
}

func (s *AdminService) CreateEdition(ctx context.Context, actor, requestID string, edition Competition) (*Competition, error) {
	def, err := s.definitions.GetDefinition(ctx, edition.DefinitionID)
	if err != nil {
		return nil, err
	}
	if !def.IsEnabled || def.CurrentVersion == 0 {
		return nil, fmt.Errorf("definition is disabled or has no rule version")
	}
	if edition.DefinitionVersion == 0 {
		edition.DefinitionVersion = def.CurrentVersion
	}
	v, err := s.definitions.GetDefinitionVersion(ctx, edition.DefinitionID, edition.DefinitionVersion)
	if err != nil {
		return nil, err
	}
	_, scoring, err := s.ValidateRules(v.EligibilityRulesJSON, v.ScoringRulesJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRuleDocument, err)
	}
	if edition.JoinOpensAt == nil || edition.JoinClosesAt == nil ||
		!edition.JoinOpensAt.Before(*edition.JoinClosesAt) ||
		edition.JoinClosesAt.After(edition.StartsAt) || !edition.StartsAt.Before(edition.EndsAt) {
		return nil, fmt.Errorf("schedule must satisfy join_opens_at < join_closes_at <= starts_at < ends_at")
	}
	edition.ID = uuid.NewString()
	edition.Name = strings.TrimSpace(edition.Name)
	if edition.Name == "" {
		return nil, fmt.Errorf("edition name is required")
	}
	if edition.Type == "" {
		edition.Type = "engine"
	}
	edition.LifecycleStatus, edition.Status = LifecycleDraft, LifecycleDraft
	edition.CreatedAt = s.clock()
	edition.ScoringScope = scoring.Scope
	edition.RulesSnapshotJSON, err = BuildRulesSnapshot(*v)
	if err != nil {
		return nil, err
	}
	err = s.editions.CreateEdition(ctx, edition)
	auditErr := s.audit(ctx, actor, requestID, "edition.create", "edition", edition.ID, edition.ID, "", map[string]any{"definition_id": edition.DefinitionID, "definition_version": edition.DefinitionVersion}, err)
	if err != nil {
		return nil, err
	}
	if auditErr != nil {
		return nil, auditErr
	}
	return s.engine.GetCompetition(ctx, edition.ID)
}

func (s *AdminService) Publish(ctx context.Context, actor, requestID, competitionID string) (*Competition, error) {
	return s.transition(ctx, actor, requestID, competitionID, LifecyclePublished, "edition.publish", "")
}

func (s *AdminService) Cancel(ctx context.Context, actor, requestID, competitionID, reason string) (*Competition, error) {
	if strings.TrimSpace(reason) == "" {
		return nil, fmt.Errorf("cancellation reason is required")
	}
	return s.transition(ctx, actor, requestID, competitionID, LifecycleCancelled, "edition.cancel", reason)
}

func (s *AdminService) transition(ctx context.Context, actor, requestID, competitionID, to, action, reason string) (*Competition, error) {
	comp, err := s.engine.GetCompetition(ctx, competitionID)
	if err != nil {
		return nil, err
	}
	if to == LifecyclePublished && (comp.JoinOpensAt == nil || !s.clock().Before(comp.JoinOpensAt.UTC())) {
		return nil, fmt.Errorf("cannot publish after the join window has opened")
	}
	err = s.editions.TransitionLifecycle(ctx, competitionID, comp.LifecycleStatus, to, s.clock())
	auditErr := s.audit(ctx, actor, requestID, action, "edition", competitionID, competitionID, reason, map[string]string{"from": comp.LifecycleStatus, "to": to}, err)
	if err != nil {
		return nil, err
	}
	if auditErr != nil {
		return nil, auditErr
	}
	return s.engine.GetCompetition(ctx, competitionID)
}

func (s *AdminService) Inspect(ctx context.Context, competitionID string) (*AdminOperationalStatus, error) {
	return s.operations.OperationalStatus(ctx, competitionID)
}

func (s *AdminService) Audit(ctx context.Context, competitionID string, limit int) ([]AdminAuditRecord, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.operations.ListAdminAudit(ctx, competitionID, limit)
}

func (s *AdminService) Retry(ctx context.Context, actor, requestID, competitionID, job, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("retry reason is required")
	}
	comp, err := s.engine.GetCompetition(ctx, competitionID)
	if err != nil {
		return err
	}
	switch job {
	case AdminJobBaseline:
		err = s.engine.retryCompetitionBaseline(ctx, *comp)
	case AdminJobRanking:
		err = s.engine.retryCompetitionRanking(ctx, *comp)
	case AdminJobFinalization:
		err = s.engine.retryCompetitionFinalization(ctx, *comp)
	default:
		return fmt.Errorf("unsupported job %q", job)
	}
	auditErr := s.audit(ctx, actor, requestID, "job.retry."+job, "job", competitionID+":"+job, competitionID, reason, map[string]string{"job": job}, err)
	if err != nil {
		return err
	}
	return auditErr
}

func (s *AdminService) audit(ctx context.Context, actor, requestID, action, targetType, targetID, competitionID, reason string, details any, operationErr error) error {
	raw, _ := json.Marshal(details)
	rec := AdminAuditRecord{ID: uuid.NewString(), ActorUserID: actor, Action: action, TargetType: targetType, TargetID: targetID, CompetitionID: competitionID, RequestID: requestID, Reason: reason, Details: raw, Succeeded: operationErr == nil, CreatedAt: s.clock()}
	if operationErr != nil {
		rec.ErrorMessage = operationErr.Error()
	}
	if err := s.operations.RecordAdminAudit(ctx, rec); err != nil {
		return fmt.Errorf("competition admin audit failed: %w", err)
	}
	return nil
}
