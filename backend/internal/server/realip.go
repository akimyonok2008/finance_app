package server

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// trustedRealIPMiddleware rewrites RemoteAddr only when the request arrived
// from an explicitly trusted proxy. It walks X-Forwarded-For from right to
// left, skipping trusted proxy hops, so a client cannot win by prepending a
// forged address to a header that the ingress appends to.
func trustedRealIPMiddleware(cidrs []string) func(http.Handler) http.Handler {
	trusted := make([]netip.Prefix, 0, len(cidrs))
	for _, cidr := range cidrs {
		if prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err == nil {
			trusted = append(trusted, prefix)
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			peer, ok := remoteIP(r.RemoteAddr)
			if !ok || !containsIP(trusted, peer) {
				next.ServeHTTP(w, r)
				return
			}

			if forwarded, ok := forwardedClientIP(r.Header.Get("X-Forwarded-For"), trusted); ok {
				r.RemoteAddr = forwarded.String()
			} else if real, err := netip.ParseAddr(strings.TrimSpace(r.Header.Get("X-Real-IP"))); err == nil {
				r.RemoteAddr = real.Unmap().String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

func remoteIP(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(host))
	return ip.Unmap(), err == nil
}

func containsIP(prefixes []netip.Prefix, ip netip.Addr) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func forwardedClientIP(header string, trusted []netip.Prefix) (netip.Addr, bool) {
	parts := strings.Split(header, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			continue
		}
		ip = ip.Unmap()
		if !containsIP(trusted, ip) {
			return ip, true
		}
	}
	return netip.Addr{}, false
}
