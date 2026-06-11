package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/ardakimyonok/finance_app/internal/httpx"
)

// ReadinessCheck is a named dependency probe (postgres, redis, ...). Check
// returns nil when the dependency is reachable.
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// healthHandler answers GET /health: 200 whenever the process is alive.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyHandler answers GET /ready: 200 only when every dependency check passes,
// 503 otherwise. Static info (storage/price provider) is included for operators.
func readyHandler(checks []ReadinessCheck, info map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		resp := make(map[string]string, len(checks)+len(info)+1)
		for k, v := range info {
			resp[k] = v
		}

		allOK := true
		for _, c := range checks {
			if err := c.Check(ctx); err != nil {
				allOK = false
				resp[c.Name] = "error"
				slog.Error("readiness check failed", "dependency", c.Name, "error", err)
				continue
			}
			resp[c.Name] = "ok"
		}

		if allOK {
			resp["status"] = "ready"
			httpx.WriteJSON(w, http.StatusOK, resp)
			return
		}
		resp["status"] = "not_ready"
		httpx.WriteJSON(w, http.StatusServiceUnavailable, resp)
	}
}
