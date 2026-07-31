package competitions

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
)

type AdminHandler struct{ svc *AdminService }

func NewAdminHandler(svc *AdminService) *AdminHandler { return &AdminHandler{svc: svc} }

type createDefinitionRequest struct {
	Slug               string          `json:"slug"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	Category           string          `json:"category"`
	IconKey            string          `json:"icon_key"`
	PresentationConfig json.RawMessage `json:"presentation_config"`
	Enabled            *bool           `json:"enabled"`
}

type createVersionRequest struct {
	EligibilityRules json.RawMessage `json:"eligibility_rules"`
	ScoringRules     json.RawMessage `json:"scoring_rules"`
	ScheduleDefaults json.RawMessage `json:"schedule_defaults"`
	DisplayRules     json.RawMessage `json:"display_rules"`
}

type createEditionRequest struct {
	Name              string     `json:"name"`
	Type              string     `json:"type"`
	DefinitionID      string     `json:"definition_id"`
	DefinitionVersion int64      `json:"definition_version,omitempty"`
	JoinOpensAt       *time.Time `json:"join_opens_at"`
	JoinClosesAt      *time.Time `json:"join_closes_at"`
	StartsAt          time.Time  `json:"starts_at"`
	EndsAt            time.Time  `json:"ends_at"`
}

type reasonRequest struct {
	Reason string `json:"reason"`
}

func (h *AdminHandler) ListDefinitions(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.ListDefinitions(r.Context())
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *AdminHandler) CreateDefinition(w http.ResponseWriter, r *http.Request) {
	var req createDefinitionRequest
	if httpx.DecodeJSON(r, &req) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	out, err := h.svc.CreateDefinition(r.Context(), adminActor(r), requestID(r), Definition{
		Slug: req.Slug, Name: req.Name, Description: req.Description, Category: req.Category,
		IconKey: req.IconKey, PresentationConfigJSON: req.PresentationConfig, IsEnabled: enabled,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *AdminHandler) ValidateRules(w http.ResponseWriter, r *http.Request) {
	var req createVersionRequest
	if httpx.DecodeJSON(r, &req) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	eligibility, scoring, err := h.svc.ValidateRules(req.EligibilityRules, req.ScoringRules)
	if err != nil {
		httpx.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{"valid": false, "error": err.Error()})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"valid": true, "eligibility": eligibility, "scoring": scoring})
}

func (h *AdminHandler) CreateVersion(w http.ResponseWriter, r *http.Request) {
	var req createVersionRequest
	if httpx.DecodeJSON(r, &req) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.CreateVersion(r.Context(), adminActor(r), requestID(r), chi.URLParam(r, "definitionId"), DefinitionVersion{
		EligibilityRulesJSON: req.EligibilityRules, ScoringRulesJSON: req.ScoringRules,
		ScheduleDefaultsJSON: req.ScheduleDefaults, DisplayRulesJSON: req.DisplayRules,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *AdminHandler) CreateEdition(w http.ResponseWriter, r *http.Request) {
	var req createEditionRequest
	if httpx.DecodeJSON(r, &req) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.CreateEdition(r.Context(), adminActor(r), requestID(r), Competition{
		Name: req.Name, Type: req.Type, DefinitionID: req.DefinitionID, DefinitionVersion: req.DefinitionVersion,
		JoinOpensAt: req.JoinOpensAt, JoinClosesAt: req.JoinClosesAt, StartsAt: req.StartsAt, EndsAt: req.EndsAt,
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, out)
}

func (h *AdminHandler) Publish(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Publish(r.Context(), adminActor(r), requestID(r), chi.URLParam(r, "competitionId"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *AdminHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	var req reasonRequest
	if httpx.DecodeJSON(r, &req) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	out, err := h.svc.Cancel(r.Context(), adminActor(r), requestID(r), chi.URLParam(r, "competitionId"), req.Reason)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *AdminHandler) Inspect(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Inspect(r.Context(), chi.URLParam(r, "competitionId"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *AdminHandler) Retry(w http.ResponseWriter, r *http.Request) {
	var req reasonRequest
	if httpx.DecodeJSON(r, &req) != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	err := h.svc.Retry(r.Context(), adminActor(r), requestID(r), chi.URLParam(r, "competitionId"), chi.URLParam(r, "job"), req.Reason)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"status": "completed"})
}

func (h *AdminHandler) Audit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := h.svc.Audit(r.Context(), chi.URLParam(r, "competitionId"), limit)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func adminActor(r *http.Request) string {
	id, _ := auth.UserIDFromContext(r.Context())
	return id
}

func requestID(r *http.Request) string { return middleware.GetReqID(r.Context()) }

func writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCompetitionNotFound), errors.Is(err, ErrDefinitionNotFound), errors.Is(err, ErrDefinitionVersionNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrDefinitionExists), errors.Is(err, ErrDefinitionVersionExists),
		errors.Is(err, ErrEditionExists), errors.Is(err, ErrLifecycleConflict), errors.Is(err, ErrInvalidLifecycleTransition):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidRuleDocument):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	}
}
