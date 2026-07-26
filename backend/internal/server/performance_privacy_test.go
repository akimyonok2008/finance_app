package server_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPrivacyUser registers a user with some real economic history so the new
// analytics actually have content to leak.
func newPrivacyUser(t *testing.T, h http.Handler, email, name, keyPrefix string) (token, userID string) {
	t.Helper()
	reg := doReq(t, h, http.MethodPost, "/auth/register", fmt.Sprintf(
		`{"email":%q,"password":"StrongPassword123","display_name":%q,"avatar_key":"fox"}`,
		email, name), "")
	require.Equal(t, http.StatusCreated, reg.Code, reg.Body.String())
	token = extractToken(t, reg.Body.String())
	userID = extractUserID(t, reg.Body.String())

	require.Equal(t, http.StatusCreated, doIdempotentReq(
		t, h, "/portfolio/deposits", `{"currency":"USD","amount":10000}`, token, keyPrefix+"-dep").Code)
	require.Equal(t, http.StatusCreated, doIdempotentReq(
		t, h, "/portfolio/buys", `{"symbol":"AAPL","asset_type":"stock","quantity":10}`, token, keyPrefix+"-buy").Code)
	return token, userID
}

// Every new analytic added in this pass lives behind owner authentication. An
// anonymous caller must get 401 — never a partially-populated payload.
func TestPerformanceAndStateEndpointsRequireOwnerAuthentication(t *testing.T) {
	h := newFullServer(t, nil)

	ownerPrivate := []string{
		"/performance/summary",
		"/performance/history?timeframe=1M",
		"/portfolio/summary",
		"/portfolio/positions",
		"/portfolio/positions/closed",
		"/portfolio/cash",
		"/portfolio/archives?timeframe=1M",
		"/activity",
	}
	for _, path := range ownerPrivate {
		t.Run(path, func(t *testing.T) {
			anonymous := doReq(t, h, http.MethodGet, path, "", "")
			assert.Equal(t, http.StatusUnauthorized, anonymous.Code,
				"%s must not be readable without a token", path)

			bogus := doReq(t, h, http.MethodGet, path, "", "not-a-real-token")
			assert.Equal(t, http.StatusUnauthorized, bogus.Code,
				"%s must not accept a forged token", path)
		})
	}
}

// The owner's identity always comes from the token. A user_id query parameter
// must be ignored, so there is no public-profile-shaped route to another user's
// economic breakdown, contributors, or risk analytics.
func TestPerformanceEndpointsIgnoreUserIDQueryParameterImpersonation(t *testing.T) {
	h := newFullServer(t, nil)

	victimToken, victimID := newPrivacyUser(t, h, "victim@example.com", "VictimUser_1", "victim")
	attackerToken, _ := newPrivacyUser(t, h, "attacker@example.com", "AttackerUser_1", "attacker")

	// The victim has real content in the analytic blocks.
	victimOwn := doReq(t, h, http.MethodGet, "/performance/summary", "", victimToken)
	require.Equal(t, http.StatusOK, victimOwn.Code)
	var victimPayload struct {
		Contributions struct {
			Contributors []struct {
				Symbol string `json:"symbol"`
			} `json:"contributors"`
		} `json:"contributions"`
	}
	require.NoError(t, json.Unmarshal(victimOwn.Body.Bytes(), &victimPayload))

	for _, path := range []string{
		"/performance/summary?user_id=" + victimID,
		"/performance/history?timeframe=1M&user_id=" + victimID,
		"/portfolio/summary?user_id=" + victimID,
	} {
		stolen := doReq(t, h, http.MethodGet, path, "", attackerToken)
		require.Equal(t, http.StatusOK, stolen.Code)
		assert.NotContains(t, stolen.Body.String(), victimID,
			"%s must never echo another user's id", path)
	}

	// The attacker's own performance summary has no instruments of its own
	// beyond what it bought; the point is it is not the victim's document.
	attackerOwn := doReq(t, h, http.MethodGet, "/performance/summary?user_id="+victimID, "", attackerToken)
	assert.NotContains(t, attackerOwn.Body.String(), victimID)
}

// newAnalyticFieldNames are the JSON keys introduced in this pass. None of them
// may appear in any PUBLIC payload.
var newAnalyticFieldNames = []string{
	"economic_breakdown",
	"standalone_fees_base",
	"attributed_total_base",
	"total_economic_pnl_base",
	"unattributed_base",
	"contributions",
	"contribution_percentage_points",
	"economic_result_base",
	"unattributed_percentage_points",
	"total_capital_base",
	"realized_pnl_base",
	"unrealized_pnl_base",
	"net_income_base",
	"current_drawdown_percentage",
	"positive_weeks_percentage",
	"best_month",
	"worst_month",
}

// The public leaderboard is percentage-and-rank only. None of the new economic,
// contributor, or risk fields may reach it.
func TestPublicLeaderboardExcludesNewEconomicAndRiskFields(t *testing.T) {
	h := newFullServer(t, nil)
	token, _ := newPrivacyUser(t, h, "board@example.com", "BoardUser_1", "board")

	board := doReq(t, h, http.MethodGet, "/leaderboard?timeframe=ALL", "", token)
	require.Equal(t, http.StatusOK, board.Code)
	body := board.Body.String()

	for _, field := range newAnalyticFieldNames {
		assert.NotContains(t, body, `"`+field+`"`,
			"public leaderboard must not expose %q", field)
	}
	// Nor the monetary base-currency figures in any form.
	assert.NotContains(t, body, "_base\":")
}

// The three layers stay separately owned: the portfolio-STATE DTO must not
// start carrying the performance layer's economic breakdown or contributors,
// even though one calculation feeds both.
func TestPortfolioSummaryDTODoesNotCarryPerformanceLayerBlocks(t *testing.T) {
	h := newFullServer(t, nil)
	token, _ := newPrivacyUser(t, h, "layers@example.com", "LayersUser_1", "layers")

	state := doReq(t, h, http.MethodGet, "/portfolio/summary", "", token)
	require.Equal(t, http.StatusOK, state.Code)
	stateBody := state.Body.String()
	assert.NotContains(t, stateBody, `"economic_breakdown"`)
	assert.NotContains(t, stateBody, `"contributions"`)

	// ...while the performance DTO does carry them, so the assertion above is
	// about ownership, not about the data being missing everywhere.
	perf := doReq(t, h, http.MethodGet, "/performance/summary", "", token)
	require.Equal(t, http.StatusOK, perf.Code)
	perfBody := perf.Body.String()
	assert.Contains(t, perfBody, `"economic_breakdown"`)
	assert.Contains(t, perfBody, `"contributions"`)
	assert.Contains(t, perfBody, `"standalone_fees_base"`)
}

// The economic breakdown served to the owner must reconcile with the ledger's
// own attribution identity, and must never be derived from the ranked index.
func TestPerformanceSummaryEconomicBreakdownReconciles(t *testing.T) {
	h := newFullServer(t, nil)
	token, _ := newPrivacyUser(t, h, "recon@example.com", "ReconUser_1", "recon")

	rec := doReq(t, h, http.MethodGet, "/performance/summary", "", token)
	require.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Ranked struct {
			Index float64 `json:"index"`
		} `json:"ranked"`
		EconomicBreakdown struct {
			RealizedPnLBase      float64  `json:"realized_pnl_base"`
			UnrealizedPnLBase    float64  `json:"unrealized_pnl_base"`
			NetIncomeBase        float64  `json:"net_income_base"`
			StandaloneFeesBase   float64  `json:"standalone_fees_base"`
			AttributedTotalBase  float64  `json:"attributed_total_base"`
			TotalEconomicPnLBase *float64 `json:"total_economic_pnl_base"`
			IsComplete           bool     `json:"is_complete"`
		} `json:"economic_breakdown"`
		Contributions struct {
			Basis     string `json:"basis"`
			Available bool   `json:"available"`
		} `json:"contributions"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))

	b := payload.EconomicBreakdown
	assert.InDelta(t,
		b.RealizedPnLBase+b.UnrealizedPnLBase+b.NetIncomeBase-b.StandaloneFeesBase,
		b.AttributedTotalBase, 0.011,
		"the breakdown must satisfy realized + unrealized + income - standalone fees")

	// The contribution scope limitation is always declared.
	assert.Equal(t, "since_inception", payload.Contributions.Basis)

	// Economic P&L is money; ranked index is a unitless competitive projection.
	// They must never be the same number by construction.
	if b.IsComplete && b.TotalEconomicPnLBase != nil {
		assert.NotEqual(t, payload.Ranked.Index, *b.TotalEconomicPnLBase)
	}
}

// The ranked-history payload's risk block must be present and truthful even
// with no snapshots: nil analytics plus reasons, never zeros.
func TestRankedHistoryRiskBlockIsTruthfulWithoutSnapshots(t *testing.T) {
	h := newFullServer(t, nil)
	token, _ := newPrivacyUser(t, h, "risk@example.com", "RiskUser_1", "risk")

	rec := doReq(t, h, http.MethodGet, "/performance/history?timeframe=1M", "", token)
	require.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Available bool `json:"available"`
		Risk      struct {
			MaxDrawdownPercentage     *float64 `json:"max_drawdown_percentage"`
			CurrentDrawdownPercentage *float64 `json:"current_drawdown_percentage"`
			PositiveWeeksPercentage   *float64 `json:"positive_weeks_percentage"`
			BestMonth                 *struct {
				Label string `json:"label"`
			} `json:"best_month"`
			WeeksReason     string `json:"weeks_reason"`
			MonthsReason    string `json:"months_reason"`
			CalculationBase string `json:"calculation_base"`
		} `json:"risk"`
		Benchmark struct {
			Available                  bool     `json:"available"`
			Reason                     string   `json:"reason"`
			DifferencePercentagePoints *float64 `json:"difference_percentage_points"`
		} `json:"benchmark"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))

	require.False(t, payload.Available)
	assert.Nil(t, payload.Risk.MaxDrawdownPercentage)
	assert.Nil(t, payload.Risk.CurrentDrawdownPercentage)
	assert.Nil(t, payload.Risk.PositiveWeeksPercentage)
	assert.Nil(t, payload.Risk.BestMonth)
	assert.Contains(t, payload.Risk.WeeksReason, "Not enough history")
	assert.Contains(t, payload.Risk.MonthsReason, "Not enough history")
	assert.Equal(t, "ranked_index", payload.Risk.CalculationBase)

	// No snapshots means no like-for-like benchmark window either.
	assert.False(t, payload.Benchmark.Available)
	assert.Nil(t, payload.Benchmark.DifferencePercentagePoints)
	assert.NotEmpty(t, payload.Benchmark.Reason)

	// Truthfulness check: the payload must not contain a fabricated 0 for the
	// analytics it just declared unavailable.
	assert.False(t, strings.Contains(rec.Body.String(), `"max_drawdown_percentage":0,`))
}
