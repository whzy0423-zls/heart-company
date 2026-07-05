package realip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFromRequestIgnoresForwardedHeadersByDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.12:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	req.Header.Set("X-Real-IP", "198.51.100.10")

	if got := FromRequest(req, nil); got != "10.0.0.12" {
		t.Fatalf("expected immediate peer IP when no trusted proxies are configured, got %q", got)
	}
}

func TestFromRequestUsesForwardedHeaderFromConfiguredProxy(t *testing.T) {
	prefixes, err := ParseTrustedProxyCIDRs([]string{"10.0.0.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.12:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.9, 10.0.0.12")

	if got := FromRequest(req, prefixes); got != "198.51.100.9" {
		t.Fatalf("expected forwarded client IP from configured proxy, got %q", got)
	}
}

func TestFromRequestIgnoresSpoofedForwardedPrefixFromConfiguredProxy(t *testing.T) {
	prefixes, err := ParseTrustedProxyCIDRs([]string{"10.0.0.0/24"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.12:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.200, 203.0.113.9")

	if got := FromRequest(req, prefixes); got != "203.0.113.9" {
		t.Fatalf("expected nearest non-trusted forwarded IP, got %q", got)
	}
}

func TestParseTrustedProxyCIDRsAcceptsSingleIP(t *testing.T) {
	prefixes, err := ParseTrustedProxyCIDRs([]string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Real-IP", "203.0.113.8")

	if got := FromRequest(req, prefixes); got != "203.0.113.8" {
		t.Fatalf("expected forwarded client IP from single trusted IP, got %q", got)
	}
}

func TestParseTrustedProxyCIDRsRejectsInvalidValues(t *testing.T) {
	if _, err := ParseTrustedProxyCIDRs([]string{"not-a-cidr"}); err == nil {
		t.Fatal("expected invalid CIDR to be rejected")
	}
}
