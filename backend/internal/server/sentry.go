package server

import (
	"net/http"

	sentryhttp "github.com/getsentry/sentry-go/http"
)

// sentryMiddleware reports panics recovered while serving a request to
// Sentry, then re-panics so middleware.Recoverer (which must run BEFORE this
// in the chain — see New) still turns it into a 500 response. It is safe to
// use unconditionally: sentry-go's capture calls are no-ops whenever
// sentry.Init was never called (no SENTRY_DSN configured), so this file adds
// nothing when Sentry is disabled.
var sentryHandler = sentryhttp.New(sentryhttp.Options{Repanic: true})

func sentryMiddleware(next http.Handler) http.Handler {
	return sentryHandler.Handle(next)
}
