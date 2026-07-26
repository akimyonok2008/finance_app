// Package providerhttp holds the shared HTTP transport behaviour used by the
// external market-data provider adapters (Alpaca, FMP): context-aware requests,
// a configurable timeout, and bounded retry with exponential backoff and jitter
// for transient failures. It deliberately knows nothing about any domain type so
// both the income and corpactions adapters can depend on it.
package providerhttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// MaxAttempts bounds the total number of attempts (1 initial + retries).
const MaxAttempts = 3

// baseBackoff is the first retry delay; it doubles per attempt.
const baseBackoff = 200 * time.Millisecond

// Client performs retrying, context-aware requests against a provider API. The
// underlying *http.Client is injected so tests can point it at an httptest
// server; there is deliberately no package-level default client.
type Client struct {
	HTTP    *http.Client
	Timeout time.Duration
	// Sleep is the delay function used between retries; tests override it to
	// keep runs fast. Defaults to time.Sleep.
	Sleep func(time.Duration)
}

// New builds a Client. When http is nil a dedicated client with the given
// timeout is created (never http.DefaultClient, which has no timeout).
func New(httpClient *http.Client, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{HTTP: httpClient, Timeout: timeout, Sleep: time.Sleep}
}

// StatusError reports a non-retryable, non-2xx provider response. The body is
// truncated; provider credentials are never included.
type StatusError struct {
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("provider responded %d: %s", e.StatusCode, e.Body)
}

// GetJSON issues a GET for url, applying header via the supplied decorator, and
// returns the response body on success. Retries are applied for 429, 5xx and
// transport errors; other 4xx fail immediately.
func (c *Client) GetJSON(ctx context.Context, url string, decorate func(*http.Request)) ([]byte, error) {
	return c.do(ctx, http.MethodGet, url, nil, decorate)
}

// PostJSON issues a POST with a JSON body, applying the same retry policy as
// GetJSON. The body is re-created per attempt so retries are safe. Only use it
// for idempotent provider endpoints (e.g. OpenFIGI mapping, which is a pure
// lookup despite being a POST).
func (c *Client) PostJSON(ctx context.Context, url string, body []byte, decorate func(*http.Request)) ([]byte, error) {
	return c.do(ctx, http.MethodPost, url, body, decorate)
}

func (c *Client) do(ctx context.Context, method, url string, body []byte, decorate func(*http.Request)) ([]byte, error) {
	var lastErr error
	for attempt := 1; attempt <= MaxAttempts; attempt++ {
		respBody, retryAfter, err := c.attempt(ctx, method, url, body, decorate)
		if err == nil {
			return respBody, nil
		}
		lastErr = err
		var statusErr *StatusError
		if ok := asStatusError(err, &statusErr); ok && !retryableStatus(statusErr.StatusCode) {
			return nil, err
		}
		if attempt == MaxAttempts {
			break
		}
		wait := retryAfter
		if wait <= 0 {
			wait = backoff(attempt)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		c.sleep(wait)
	}
	return nil, lastErr
}

func (c *Client) sleep(d time.Duration) {
	if c.Sleep != nil {
		c.Sleep(d)
		return
	}
	time.Sleep(d)
}

func (c *Client) attempt(ctx context.Context, method, url string, reqBody []byte, decorate func(*http.Request)) ([]byte, time.Duration, error) {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// A fresh reader per attempt keeps retries safe.
	var payload io.Reader
	if reqBody != nil {
		payload = bytes.NewReader(reqBody)
	}
	req, err := http.NewRequestWithContext(reqCtx, method, url, payload)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if decorate != nil {
		decorate(req)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, parseRetryAfter(resp.Header.Get("Retry-After")), &StatusError{
			StatusCode: resp.StatusCode,
			Body:       truncate(string(body), 256),
		}
	}
	if readErr != nil {
		return nil, 0, readErr
	}
	return body, 0, nil
}

func retryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

func asStatusError(err error, target **StatusError) bool {
	if se, ok := err.(*StatusError); ok {
		*target = se
		return true
	}
	return false
}

func backoff(attempt int) time.Duration {
	d := baseBackoff * time.Duration(1<<(attempt-1))
	jitter := time.Duration(rand.Int63n(int64(d/2) + 1))
	return d + jitter
}

func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
