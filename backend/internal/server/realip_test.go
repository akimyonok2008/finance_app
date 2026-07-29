package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func observedRemoteAddr(t *testing.T, trusted []string, peer, xff, realIP string) string {
	t.Helper()
	var observed string
	handler := trustedRealIPMiddleware(trusted)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observed = r.RemoteAddr
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = peer
	req.Header.Set("X-Forwarded-For", xff)
	req.Header.Set("X-Real-IP", realIP)
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return observed
}

func TestTrustedRealIPIgnoresForwardingHeadersFromDirectClient(t *testing.T) {
	got := observedRemoteAddr(t, []string{"10.0.0.0/8"}, "203.0.113.10:4321", "198.51.100.7", "198.51.100.8")
	assert.Equal(t, "203.0.113.10:4321", got)
}

func TestTrustedRealIPAcceptsForwardingHeaderFromTrustedPeer(t *testing.T) {
	got := observedRemoteAddr(t, []string{"10.0.0.0/8"}, "10.1.2.3:4321", "198.51.100.7", "")
	assert.Equal(t, "198.51.100.7", got)
}

func TestTrustedRealIPWalksProxyChainRightToLeft(t *testing.T) {
	got := observedRemoteAddr(t, []string{"10.0.0.0/8"}, "10.1.2.3:4321", "192.0.2.99, 198.51.100.7, 10.2.3.4", "")
	assert.Equal(t, "198.51.100.7", got, "client-prepended values must not override the nearest untrusted hop")
}

func TestTrustedRealIPTrustsNoHeadersByDefault(t *testing.T) {
	got := observedRemoteAddr(t, nil, "203.0.113.10:4321", "198.51.100.7", "")
	assert.Equal(t, "203.0.113.10:4321", got)
}
