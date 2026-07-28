package portfolio

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ardakimyonok/finance_app/internal/auth"
	"github.com/ardakimyonok/finance_app/internal/httpx"
	"github.com/ardakimyonok/finance_app/internal/money"
)

// AchievementEvaluator lets the portfolio handler trigger achievement checks
// after a position change or summary view, without importing the achievements
// package (avoids an import cycle). Optional — a nil evaluator skips triggers.
type AchievementEvaluator interface {
	EvaluatePortfolioAchievements(ctx context.Context, userID string) error
}

// Handler adapts HTTP requests to the portfolio Service. Every handler assumes
// it runs behind auth.RequireAuth and reads the user id from the context.
type Handler struct {
	svc        *Service
	evaluator  AchievementEvaluator      // optional
	corpView   CorporateActionViewReader // optional; read-only automatic-adjustments view
	incomeView IncomeEventViewReader     // optional; read-only automatic-income view
}

// NewHandler constructs a portfolio Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// CorporateActionView is the owner-private, read-only projection of an
// automatically-applied (or pending) corporate action. It carries a
// plain-language explanation and never exposes private amounts, quantities,
// basis, or internal provider payloads.
type CorporateActionView struct {
	ID            string `json:"id"`
	EventType     string `json:"event_type"`
	DisplaySymbol string `json:"display_symbol"`
	EffectiveAt   string `json:"effective_at,omitempty"`
	Status        string `json:"status"`
	Explanation   string `json:"explanation"`
	System        bool   `json:"system_generated"`
	AppliedAt     string `json:"applied_at,omitempty"`
}

// CorporateActionViewReader supplies a user's read-only corporate-action views.
// It is implemented by the corpactions service and injected so the portfolio
// package never depends on the corporate-action pipeline.
type CorporateActionViewReader interface {
	ListCorporateActionViews(ctx context.Context, userID string) ([]CorporateActionView, error)
}

// SetCorporateActionView attaches the read-only automatic-adjustments reader.
func (h *Handler) SetCorporateActionView(r CorporateActionViewReader) {
	h.corpView = r
}

// IncomeEventView is the owner-private, read-only projection of an automatically
// detected and applied income event. The owner may see their own amounts; these
// figures must never appear on any public surface.
type IncomeEventView struct {
	ID            string  `json:"id"`
	EventType     string  `json:"event_type"`
	Symbol        string  `json:"symbol"`
	Currency      string  `json:"currency"`
	GrossAmount   float64 `json:"gross_amount"`
	Withholding   float64 `json:"withholding_amount"`
	FeeAmount     float64 `json:"fee_amount"`
	NetAmount     float64 `json:"net_amount"`
	ReinvestedQty float64 `json:"reinvestment_quantity,omitempty"`
	Estimated     bool    `json:"estimated"`
	Status        string  `json:"status"`
	Provider      string  `json:"provider"`
	Explanation   string  `json:"explanation"`
	Correctable   bool    `json:"correctable"`
	PaymentDate   string  `json:"payment_date,omitempty"`
	AppliedAt     string  `json:"applied_at,omitempty"`
	System        bool    `json:"system_generated"`
}

// IncomeCorrectionInput is a constrained, account-specific correction to an
// already-detected event. It cannot create arbitrary income — it references an
// existing event and reconciles it to the actual broker outcome.
type IncomeCorrectionInput struct {
	IncomeEventID           string
	PortfolioID             string
	Kind                    string
	RequestID               string
	ActualNet               float64
	ActualWithholding       float64
	ActualFee               float64
	ActualReinvestmentQty   float64
	ActualReinvestmentPrice float64
}

// IncomeEventViewReader supplies a user's read-only income views and the
// constrained correction path. It is implemented by the income service and
// injected so the portfolio package never depends on the income pipeline.
type IncomeEventViewReader interface {
	ListIncomeEventViews(ctx context.Context, userID string) ([]IncomeEventView, error)
	GetIncomeEventView(ctx context.Context, userID, portfolioID, eventID string) (IncomeEventView, bool, error)
	CorrectIncomeEvent(ctx context.Context, userID string, c IncomeCorrectionInput) error
}

// SetIncomeEventView attaches the read-only automatic-income reader.
func (h *Handler) SetIncomeEventView(r IncomeEventViewReader) {
	h.incomeView = r
}

// SetAchievementEvaluator attaches an optional achievement evaluator that fires
// (best-effort) after a position is added and after the summary is computed.
func (h *Handler) SetAchievementEvaluator(e AchievementEvaluator) {
	h.evaluator = e
}

// evaluatePortfolio fires the achievement evaluator if one is attached. Errors
// are intentionally ignored so badge evaluation never breaks the main request.
func (h *Handler) evaluatePortfolio(ctx context.Context, userID string) {
	if h.evaluator != nil {
		_ = h.evaluator.EvaluatePortfolioAchievements(ctx, userID)
	}
}

// --- response views ----------------------------------------------------------

type portfolioView struct {
	ID                string `json:"id"`
	UserID            string `json:"user_id"`
	Name              string `json:"name"`
	Currency          string `json:"currency"`
	AutoFundPurchases bool   `json:"auto_fund_purchases"`
}

// positionView is the owner-private position shape. BaselinePrice is the price
// locked at add time (today's market price) — there is no average buy price in
// the product.
type positionView struct {
	ID                string  `json:"id"`
	Symbol            string  `json:"symbol"`
	InstrumentID      string  `json:"instrument_id,omitempty"`
	AssetType         string  `json:"asset_type"`
	Quantity          float64 `json:"quantity"`
	BaselinePrice     float64 `json:"baseline_price"`
	Currency          string  `json:"currency"`
	Status            string  `json:"status"`
	PositionEpisodeID string  `json:"position_episode_id"`
	OpenedAt          string  `json:"opened_at"`
}

// positionRequest is the create payload: no price, no currency — the baseline
// is locked server-side at the current market quote.
type positionRequest struct {
	Symbol    string  `json:"symbol"`
	AssetType string  `json:"asset_type"`
	Quantity  float64 `json:"quantity"`
}

// updatePositionRequest allows quantity changes only; the symbol and locked
// baseline price are immutable after creation.
type updatePositionRequest struct {
	Quantity float64 `json:"quantity"`
}

type cashFlowRequest struct {
	Currency   string  `json:"currency"`
	Amount     float64 `json:"amount"`
	OccurredAt string  `json:"occurred_at,omitempty"`
}

type buyRequest struct {
	Symbol       string  `json:"symbol"`
	ExchangeCode string  `json:"exchange_code,omitempty"`
	MIC          string  `json:"mic,omitempty"`
	AssetType    string  `json:"asset_type"`
	Quantity     float64 `json:"quantity"`
	// Optional real execution details. Omitted price/fee/date default to the
	// latest tracked quote / zero / now and are labelled as estimates.
	ExecutionPrice float64 `json:"execution_price,omitempty"`
	Fee            float64 `json:"fee,omitempty"`
	EffectiveAt    string  `json:"effective_at,omitempty"`
}

type sellRequest struct {
	PositionID     string  `json:"position_id,omitempty"`
	Symbol         string  `json:"symbol,omitempty"`
	Quantity       float64 `json:"quantity"`
	ExecutionPrice float64 `json:"execution_price,omitempty"`
	Fee            float64 `json:"fee,omitempty"`
	EffectiveAt    string  `json:"effective_at,omitempty"`
}

type activityMutationView struct {
	Position         *positionView `json:"position,omitempty"`
	Activity         *ActivityView `json:"activity,omitempty"`
	PortfolioVersion int64         `json:"portfolio_version"`
	RankedIndex      float64       `json:"ranked_index"`
	RankingStatus    string        `json:"ranking_status"`
}

func toPositionView(p *Position) positionView {
	return positionView{
		ID:                p.ID,
		Symbol:            p.Symbol,
		InstrumentID:      p.InstrumentID,
		AssetType:         p.AssetType,
		Quantity:          p.Quantity.Float64(),
		BaselinePrice:     p.AverageBuyPrice.Float64(),
		Currency:          p.Currency,
		Status:            positionStatus(p),
		PositionEpisodeID: p.ID,
		OpenedAt:          p.CreatedAt.Format(time.RFC3339),
	}
}

func (r positionRequest) toInput() PositionInput {
	return PositionInput{
		Symbol:    r.Symbol,
		AssetType: r.AssetType,
		Quantity:  money.QuantityFromFloat64(r.Quantity),
	}
}

// --- handlers ----------------------------------------------------------------

// GetPortfolio handles GET /portfolio.
func (h *Handler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	pf, err := h.svc.GetOrCreateDefaultPortfolio(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, portfolioView{
		ID: pf.ID, UserID: pf.UserID, Name: pf.Name, Currency: pf.Currency,
		AutoFundPurchases: pf.AutoFundPurchases,
	})
}

type portfolioSettingsRequest struct {
	AutoFundPurchases *bool `json:"auto_fund_purchases,omitempty"`
}

// UpdatePortfolioSettings handles PATCH /portfolio/settings. Currently the
// only setting is auto_fund_purchases (default true): when disabled, a buy
// that would need automatic funding is rejected with ErrInsufficientCash
// instead of silently drawing an implicit deposit — the "buys require
// sufficient cash" behavior the README describes, for users who want it.
func (h *Handler) UpdatePortfolioSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	var req portfolioSettingsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AutoFundPurchases != nil {
		if err := h.svc.SetAutoFundPurchases(r.Context(), userID, *req.AutoFundPurchases); err != nil {
			writeServiceError(w, err)
			return
		}
	}
	pf, err := h.svc.GetOrCreateDefaultPortfolio(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, portfolioView{
		ID: pf.ID, UserID: pf.UserID, Name: pf.Name, Currency: pf.Currency,
		AutoFundPurchases: pf.AutoFundPurchases,
	})
}

// AddPosition handles POST /portfolio/positions.
func (h *Handler) AddPosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	var req positionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.Mutate(r.Context(), MutationRequest{
		Kind: MutationAdd, UserID: userID, RequestID: idempotencyKey(r),
		Input: req.toInput(),
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	// First position (and any positive/index-110 state) may unlock badges.
	h.evaluatePortfolio(r.Context(), userID)
	httpx.WriteJSON(w, http.StatusCreated, toPositionView(res.Position))
}

// ListPositions handles GET /portfolio/positions.
func (h *Handler) ListPositions(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positions, err := h.svc.ListPositions(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	views := make([]positionView, 0, len(positions))
	for _, p := range positions {
		views = append(views, toPositionView(p))
	}
	httpx.WriteJSON(w, http.StatusOK, views)
}

func (h *Handler) ListCash(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	balances, total, err := h.svc.CashBalances(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"cash_balances": balances, "total_cash_value_base": round2(total),
		"base_currency": "USD",
	})
}

func (h *Handler) ListActivities(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	activities, err := h.svc.Activities(r.Context(), uid, limit)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, activities)
}

func (h *Handler) ActivityList(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	response, err := h.svc.ActivityList(
		r.Context(), uid, r.URL.Query().Get("category"), r.URL.Query().Get("symbol"),
		limit, offset,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) ActivityDetail(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	response, err := h.svc.ActivityDetail(r.Context(), uid, chi.URLParam(r, "activityId"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, response)
}

func (h *Handler) DepositCash(w http.ResponseWriter, r *http.Request) {
	h.cashFlow(w, r, MutationDeposit)
}

func (h *Handler) WithdrawCash(w http.ResponseWriter, r *http.Request) {
	h.cashFlow(w, r, MutationWithdrawal)
}

func (h *Handler) cashFlow(w http.ResponseWriter, r *http.Request, kind MutationKind) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req cashFlowRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var occurredAt *time.Time
	if req.OccurredAt != "" {
		parsed, err := time.Parse(time.RFC3339, req.OccurredAt)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "occurred_at must be ISO-8601")
			return
		}
		occurredAt = &parsed
	}
	input := CashFlowInput{Currency: req.Currency, Amount: money.AmountFromFloat64(req.Amount), OccurredAt: occurredAt}
	requestID, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	var res MutationResult
	var err error
	if kind == MutationDeposit {
		res, err = h.svc.DepositCash(r.Context(), uid, requestID, input)
	} else {
		res, err = h.svc.WithdrawCash(r.Context(), uid, requestID, input)
	}
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, mutationView(res))
}

func (h *Handler) BuyPosition(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req buyRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	requestID, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	effectiveAt, ok := parseEffectiveAt(w, req.EffectiveAt)
	if !ok {
		return
	}
	res, err := h.svc.BuyPosition(r.Context(), uid, requestID, BuyInput{
		Symbol: req.Symbol, AssetType: req.AssetType, Quantity: money.QuantityFromFloat64(req.Quantity),
		ExchangeCode: req.ExchangeCode, MIC: req.MIC,
		ExecutionPrice: money.PriceFromFloat64(req.ExecutionPrice), Fee: money.AmountFromFloat64(req.Fee), EffectiveAt: effectiveAt,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, mutationView(res))
}

func (h *Handler) SellPosition(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req sellRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	requestID, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	effectiveAt, ok := parseEffectiveAt(w, req.EffectiveAt)
	if !ok {
		return
	}
	res, err := h.svc.SellPosition(r.Context(), uid, requestID, SellInput{
		PositionID: req.PositionID, Symbol: req.Symbol, Quantity: money.QuantityFromFloat64(req.Quantity),
		ExecutionPrice: money.PriceFromFloat64(req.ExecutionPrice), Fee: money.AmountFromFloat64(req.Fee), EffectiveAt: effectiveAt,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, mutationView(res))
}

// PreviewBuy is NON-MUTATING: it never writes activities, cash, positions,
// episodes, ranked state or audit records, and therefore takes no
// Idempotency-Key.
func (h *Handler) PreviewBuy(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req buyRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	effectiveAt, ok := parseEffectiveAt(w, req.EffectiveAt)
	if !ok {
		return
	}
	preview, err := h.svc.PreviewBuy(r.Context(), uid, BuyInput{
		Symbol: req.Symbol, AssetType: req.AssetType, Quantity: money.QuantityFromFloat64(req.Quantity),
		ExchangeCode: req.ExchangeCode, MIC: req.MIC,
		ExecutionPrice: money.PriceFromFloat64(req.ExecutionPrice), Fee: money.AmountFromFloat64(req.Fee), EffectiveAt: effectiveAt,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, preview)
}

func (h *Handler) PreviewSell(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	var req sellRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	sellEffectiveAt, ok := parseEffectiveAt(w, req.EffectiveAt)
	if !ok {
		return
	}
	preview, err := h.svc.PreviewSell(r.Context(), uid, SellInput{
		PositionID: req.PositionID, Symbol: req.Symbol, Quantity: money.QuantityFromFloat64(req.Quantity),
		ExecutionPrice: money.PriceFromFloat64(req.ExecutionPrice), Fee: money.AmountFromFloat64(req.Fee), EffectiveAt: sellEffectiveAt,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, preview)
}

func mutationView(res MutationResult) activityMutationView {
	view := activityMutationView{
		PortfolioVersion: res.PortfolioVersion, RankedIndex: res.RankedIndexAfter,
		RankingStatus: string(res.RankingStatus),
	}
	if res.Position != nil {
		position := toPositionView(res.Position)
		view.Position = &position
	}
	if res.Activity != nil {
		activity := ActivityView{
			ID: res.Activity.ID, Type: res.Activity.Type, Symbol: res.Activity.Symbol, InstrumentID: res.Activity.InstrumentID,
			AssetType: res.Activity.AssetType, Currency: res.Activity.Currency,
			Quantity: res.Activity.Quantity, UnitPrice: res.Activity.UnitPrice,
			GrossAmount:                res.Activity.GrossAmount,
			CostBasisAllocated:         res.Activity.CostBasisAllocated,
			RealizedGainLossBase:       res.Activity.RealizedGainLossBase,
			RealizedGainLossPercentage: res.Activity.RealizedGainLossPercentage,
			OccurredAt:                 res.Activity.OccurredAt.Format(time.RFC3339),
			PortfolioVersion:           res.PortfolioVersion,
		}
		view.Activity = &activity
	}
	return view
}

// ListClosedPositions handles GET /portfolio/positions/closed.
func (h *Handler) ListClosedPositions(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positions, err := h.svc.ListClosedPositions(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, positions)
}

// UpdatePosition handles PUT /portfolio/positions/{positionId}.
func (h *Handler) UpdatePosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positionID := chi.URLParam(r, "positionId")
	var req updatePositionRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := h.svc.Mutate(r.Context(), MutationRequest{
		Kind: MutationResize, UserID: userID, RequestID: idempotencyKey(r),
		PositionID: positionID, Quantity: req.Quantity,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toPositionView(res.Position))
}

// ClosePosition handles POST /portfolio/positions/{positionId}/close.
func (h *Handler) ClosePosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positionID := chi.URLParam(r, "positionId")
	res, err := h.svc.Mutate(r.Context(), MutationRequest{
		Kind: MutationClose, UserID: userID, RequestID: idempotencyKey(r),
		PositionID: positionID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	h.evaluatePortfolio(r.Context(), userID)
	if res.Closed == nil {
		// Idempotent replay of a close whose summary is no longer reconstructable.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, *res.Closed)
}

// WriteOffPosition handles POST /portfolio/positions/{positionId}/write-off.
// It is deliberately narrow: it only succeeds when the position's symbol has
// no available market price (a delisting, a coverage gap, a provider
// switch) — every other mutation on the account revalues every held symbol,
// so such a position would otherwise brick the account permanently. A
// position whose symbol can still be priced is rejected (409): it must be
// sold normally, so this can never be used to erase a losing but tradeable
// position from realized results.
func (h *Handler) WriteOffPosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	requestID, ok := requiredIdempotencyKey(w, r)
	if !ok {
		return
	}
	positionID := chi.URLParam(r, "positionId")
	res, err := h.svc.WriteOffUnpriceablePosition(r.Context(), userID, requestID, positionID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	h.evaluatePortfolio(r.Context(), userID)
	if res.Closed == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, *res.Closed)
}

// DeletePosition handles DELETE /portfolio/positions/{positionId}.
func (h *Handler) DeletePosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := userID(w, r)
	if !ok {
		return
	}
	positionID := chi.URLParam(r, "positionId")
	if _, err := h.svc.Mutate(r.Context(), MutationRequest{
		Kind: MutationDelete, UserID: userID, RequestID: idempotencyKey(r),
		PositionID: positionID,
	}); err != nil {
		writeServiceError(w, err)
		return
	}
	h.evaluatePortfolio(r.Context(), userID)
	w.WriteHeader(http.StatusNoContent)
}

// Summary handles GET /portfolio/summary.
func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	summary, err := h.svc.Summary(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	// Viewing the summary may unlock green_portfolio / index_110.
	h.evaluatePortfolio(r.Context(), uid)
	httpx.WriteJSON(w, http.StatusOK, summary)
}

func (h *Handler) PerformanceSummary(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	summary, err := h.svc.Summary(r.Context(), uid)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, PerformanceSummaryResponse{
		BaseCurrency: summary.BaseCurrency,
		Ranked:       summary.RankedPerformance,
		Economic:     summary.EconomicPerformance,
		Attribution: PerformanceAttribution{
			UnrealizedPnLBase: summary.OpenHoldings.UnrealizedPnLBase,
			RealizedPnLBase:   summary.Realized.RealizedPnLBase,
			IncomeBase:        summary.Income.TotalIncomeBase,
			FeesBase:          summary.Fees.TotalFeesBase,
		},
		Reconciliation:    summary.Reconciliation,
		EconomicBreakdown: summary.EconomicAttribution,
		Contributions:     summary.Contributions,
	})
}

// Archives handles GET /portfolio/archives?timeframe=1M.
func (h *Handler) Archives(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(w, r)
	if !ok {
		return
	}
	archives, err := h.svc.Archives(r.Context(), uid, r.URL.Query().Get("timeframe"))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, archives)
}

// --- helpers -----------------------------------------------------------------

// userID extracts the authenticated user id, writing a 401 if absent (which
// should not happen behind RequireAuth).
func userID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, ok := auth.UserIDFromContext(r.Context())
	if !ok || id == "" {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	return id, true
}

// writeServiceError maps domain errors to HTTP status codes.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSymbolRequired),
		errors.Is(err, ErrInvalidAssetType),
		errors.Is(err, ErrInvalidQuantity),
		errors.Is(err, ErrUnsupportedSymbol),
		errors.Is(err, ErrUnsupportedCurrency),
		errors.Is(err, ErrInvalidCashAmount),
		errors.Is(err, ErrInvalidIncomeAmount),
		errors.Is(err, ErrUnsupportedIncomeType),
		errors.Is(err, ErrUnsupportedFeeType),
		errors.Is(err, ErrInvalidFeeAmount),
		errors.Is(err, ErrInvalidCorporateAction),
		errors.Is(err, ErrInvalidSplitRatio),
		errors.Is(err, ErrInvalidIncomeCorrection),
		errors.Is(err, ErrInvalidReinvestment),
		errors.Is(err, ErrInvalidSalePrice),
		errors.Is(err, ErrInvalidSaleFee),
		errors.Is(err, ErrInvalidBuyPrice),
		errors.Is(err, ErrInvalidBuyFee),
		errors.Is(err, ErrHistoricalExecutionPriceRequired),
		errors.Is(err, ErrImplausibleExecutionPrice),
		errors.Is(err, ErrCorrectionNotSupported),
		errors.Is(err, ErrNothingToCorrect):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrPositionNotFound),
		errors.Is(err, ErrInstrumentNotFound),
		errors.Is(err, ErrIncomeEventNotFound),
		errors.Is(err, ErrIncomeNotApplied),
		errors.Is(err, ErrActivityNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrPositionClosed),
		errors.Is(err, ErrSymbolChangeConflict),
		errors.Is(err, ErrActivityAlreadyCorrected),
		errors.Is(err, ErrPositionIsPriceable):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInsufficientCash),
		errors.Is(err, ErrInsufficientCashForFee),
		errors.Is(err, ErrHistoricalRankedConflict),
		errors.Is(err, ErrHistoricalQuantityInsufficient),
		errors.Is(err, ErrHistoricalEpisodeNotOpen),
		errors.Is(err, ErrInstrumentIdentityAmbiguous),
		errors.Is(err, ErrInstrumentIdentityUnresolvedConflict),
		errors.Is(err, ErrInvalidSaleQuantity):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrDuplicateActivity):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrMutationConflict):
		// The portfolio kept changing underneath the mutation. Nothing was
		// committed; the client may safely retry (ideally with the same
		// Idempotency-Key).
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrInvalidWeights):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrPriceProvider):
		httpx.WriteError(w, http.StatusBadGateway, "could not fetch prices from provider")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

// idempotencyKey reads the optional Idempotency-Key header. When present, a
// retried request (mobile timeout, double-click, reverse-proxy retry) replays
// the original committed result instead of applying the mutation twice.
func idempotencyKey(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Idempotency-Key"))
}

func requiredIdempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := idempotencyKey(r)
	if key == "" {
		httpx.WriteError(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return "", false
	}
	if len(key) > 200 {
		httpx.WriteError(w, http.StatusBadRequest, "Idempotency-Key header is too long")
		return "", false
	}
	return key, true
}
