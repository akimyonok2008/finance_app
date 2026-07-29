package server

import (
	"net/http"

	"github.com/ardakimyonok/finance_app/internal/httpx"
)

const (
	globalBodyLimit  int64 = 1 << 20 // 1 MiB safety net for every public route.
	authBodyLimit    int64 = 32 << 10
	socialBodyLimit  int64 = 16 << 10
	profileBodyLimit int64 = 64 << 10
)

func maxBodyBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				httpx.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
