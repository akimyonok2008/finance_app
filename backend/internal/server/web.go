package server

import (
	_ "embed"
	"net/http"
	"strings"
)

// indexHTML is the local test UI, embedded into the binary so it is served
// regardless of the working directory. It is a development convenience, not a
// production frontend.
//
//go:embed static/index.html
var indexHTML []byte

// serveIndex serves the single-page test UI at GET /.
func serveIndex(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(indexHTML)
}

// corsMiddleware builds the CORS policy from the deployment environment and
// an optional explicit allow-list (CORS_ALLOWED_ORIGINS):
//
//   - allowedOrigins configured: only a request Origin that exactly matches
//     one of them is reflected back; every other origin gets no CORS headers
//     at all, so the browser blocks the cross-origin response.
//   - no allow-list AND appEnv == "production": no CORS headers either. A
//     wildcard default here would let any third-party site read every
//     authenticated response as long as it could smuggle out a bearer token —
//     silently permissive is the wrong failure mode for a production API.
//   - no allow-list, any other environment: "*", preserving the previous
//     permissive behavior for local development and the embedded test UI.
func corsMiddleware(appEnv string, allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[o] = true
	}
	wildcard := len(allowed) == 0 && !strings.EqualFold(appEnv, "production")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			switch {
			case wildcard:
				w.Header().Set("Access-Control-Allow-Origin", "*")
			case origin != "" && allowed[origin]:
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Add("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
