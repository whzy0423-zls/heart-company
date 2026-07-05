package realip

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxyCIDRs parses explicit trusted proxy CIDRs. A plain IP is
// accepted and converted to a single-host prefix.
func ParseTrustedProxyCIDRs(values []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(value); err == nil {
			prefixes = append(prefixes, prefix.Masked())
			continue
		}
		addr, err := netip.ParseAddr(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR/IP %q", value)
		}
		bits := 32
		if addr.Is6() {
			bits = 128
		}
		prefixes = append(prefixes, netip.PrefixFrom(addr, bits))
	}
	return prefixes, nil
}

// FromRequest returns the client IP. Forwarded headers are ignored unless the
// immediate peer is in explicitly configured trusted proxy CIDRs.
func FromRequest(r *http.Request, trustedProxies []netip.Prefix) string {
	remote := RemoteAddr(r)
	remoteAddr, err := netip.ParseAddr(remote)
	if err != nil || !isTrusted(remoteAddr, trustedProxies) {
		return remote
	}
	if ip := forwardedClientIP(r.Header.Get("X-Forwarded-For"), trustedProxies); ip != "" {
		return ip
	}
	if ip := parseHeaderIP(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	return remote
}

// RemoteAddr returns the immediate peer IP and intentionally ignores all
// forwarding headers. Use this for fail-closed logging/analytics paths.
func RemoteAddr(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func isTrusted(addr netip.Addr, trustedProxies []netip.Prefix) bool {
	for _, prefix := range trustedProxies {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func forwardedClientIP(value string, trustedProxies []netip.Prefix) string {
	parts := strings.Split(value, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		raw := strings.TrimSpace(parts[i])
		addr, err := netip.ParseAddr(raw)
		if err != nil {
			continue
		}
		if isTrusted(addr, trustedProxies) {
			continue
		}
		return addr.String()
	}
	return ""
}

func parseHeaderIP(value string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return addr.String()
}
