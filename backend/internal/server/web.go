package server

import (
	_ "embed"
	"net/http"
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

// devCORS allows the embedded test UI to call the API from any origin (e.g. a
// preview panel or a file:// page). It is permissive on purpose for local
// development; a production deployment should restrict allowed origins.
func devCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
