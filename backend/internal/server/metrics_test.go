package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ardakimyonok/finance_app/internal/server"
)

func TestMetrics_EndpointServesPrometheusFormat(t *testing.T) {
	handler := server.NewOperations(server.Deps{})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "# HELP")
	assert.Contains(t, rec.Body.String(), "# TYPE")
}

func TestMetrics_RecordsRequestCountAndRoutePattern(t *testing.T) {
	publicHandler := server.New(server.Deps{AppEnv: "development"})
	handler := server.NewOperations(server.Deps{})

	// Hit a route a few times, then confirm it shows up in /metrics labeled
	// by its ROUTE PATTERN, not the raw path (bounded cardinality).
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		publicHandler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	handler.ServeHTTP(metricsRec, metricsReq)

	body := metricsRec.Body.String()
	assert.Contains(t, body, `http_requests_total{method="GET",route="/health",status="200"}`)
}
