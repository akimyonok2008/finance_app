package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/money"
)

type rankedHistoryResponse struct {
	Timeframe                 string            `json:"timeframe"`
	Available                 bool              `json:"available"`
	Reason                    string            `json:"reason"`
	StartingIndex             *money.IndexValue `json:"starting_index"`
	EndingIndex               *money.IndexValue `json:"ending_index"`
	TimeframeReturnPercentage *float64          `json:"timeframe_return_percentage"`
	MaxDrawdownPercentage     *float64          `json:"max_drawdown_percentage"`
	Points                    []struct {
		CapturedAt         string           `json:"captured_at"`
		RankedIndex        money.IndexValue `json:"ranked_index"`
		ReturnPercentage   float64          `json:"return_percentage"`
		DrawdownPercentage float64          `json:"drawdown_percentage"`
	} `json:"points"`
}

// GET /performance/history must read the CANONICAL ranked snapshot history —
// the same source the leaderboard and achievement evidence use — not the
// private portfolio valuation archive. The archive is deposit/withdrawal
// sensitive and reports `portfolio_index`/`gain_loss_percentage`, which this
// contract must never carry.
func TestPerformanceHistory_ReadsCanonicalRankedSnapshots(t *testing.T) {
	h, historySvc, _ := newFullServerWithHistory(t, nil)

	reg := doReq(t, h, http.MethodPost, "/auth/register",
		`{"email":"ranked@example.com","password":"StrongPassword123","display_name":"RankedUser_1","avatar_key":"fox"}`, "")
	require.Equal(t, http.StatusCreated, reg.Code)
	userID := extractUserID(t, reg.Body.String())
	login := doReq(t, h, http.MethodPost, "/auth/login",
		`{"email":"ranked@example.com","password":"StrongPassword123"}`, "")
	require.Equal(t, http.StatusOK, login.Code)
	token := extractToken(t, login.Body.String())

	require.Equal(t, http.StatusCreated,
		doIdempotentReq(t, h, "/portfolio/deposits", `{"currency":"USD","amount":5000}`, token, "rh-deposit").Code)
	require.Equal(t, http.StatusCreated,
		doIdempotentReq(t, h, "/portfolio/buys", `{"symbol":"AAPL","asset_type":"stock","quantity":10}`, token, "rh-buy").Code)

	// Before any snapshot exists the endpoint must say so truthfully instead of
	// reporting zeroed analytics.
	empty := decodeRankedHistory(t, doReq(t, h, http.MethodGet, "/performance/history?timeframe=1M", "", token))
	assert.False(t, empty.Available)
	assert.Nil(t, empty.TimeframeReturnPercentage)
	assert.Nil(t, empty.MaxDrawdownPercentage)
	assert.NotEmpty(t, empty.Reason)

	// Capture a canonical ranked snapshot (what the snapshot worker does).
	written, quality, err := historySvc.RecordCurrent(context.Background(), userID)
	require.NoError(t, err)
	require.Positive(t, written)
	require.Equal(t, "complete", string(quality))

	got := decodeRankedHistory(t, doReq(t, h, http.MethodGet, "/performance/history?timeframe=1M", "", token))
	require.True(t, got.Available)
	require.NotEmpty(t, got.Points)

	// Agrees with the canonical service the leaderboard/achievements read.
	canonical, err := historySvc.RankedHistory(context.Background(), userID, "1M")
	require.NoError(t, err)
	require.True(t, canonical.Available)
	require.Len(t, got.Points, len(canonical.Points))
	assert.InDelta(t, canonical.EndingIndex.Float64(), got.EndingIndex.Float64(), 1e-9)
	assert.InDelta(t, *canonical.TimeframeReturnPercentage, *got.TimeframeReturnPercentage, 1e-9)
	assert.InDelta(t, 100.0, got.Points[0].RankedIndex.Float64(), 1e-9)

	// Regression guard: the archive contract must not leak back in here.
	body := doReq(t, h, http.MethodGet, "/performance/history?timeframe=1M", "", token).Body.String()
	assert.NotContains(t, body, `"portfolio_index"`)
	assert.NotContains(t, body, `"gain_loss_percentage"`)
	assert.NotContains(t, body, `"total_cost_basis"`)
	assert.Contains(t, body, `"ranked_index"`)
}

func TestPerformanceHistory_RequiresAuth(t *testing.T) {
	h := newFullServer(t, nil)
	rec := doReq(t, h, http.MethodGet, "/performance/history", "", "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func extractUserID(t *testing.T, body string) string {
	t.Helper()
	var payload struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &payload))
	require.NotEmpty(t, payload.User.ID, "register response must contain a user id")
	return payload.User.ID
}

func decodeRankedHistory(t *testing.T, rec interface{ Result() *http.Response }) rankedHistoryResponse {
	t.Helper()
	resp := rec.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out rankedHistoryResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	return out
}
