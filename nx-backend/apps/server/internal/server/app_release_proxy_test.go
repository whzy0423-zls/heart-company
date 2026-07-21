package server

import (
	"os"
	"strings"
	"testing"
)

func TestAppReleaseProxyConfiguration(t *testing.T) {
	admin, err := os.ReadFile("../../../../scripts/deploy/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	website, err := os.ReadFile("../../../../../website-react/nginx.conf")
	if err != nil {
		t.Fatal(err)
	}
	a := string(admin)
	w := string(website)
	if !strings.Contains(a, "location = /api/app-releases/upload") || !strings.Contains(a, "client_max_body_size 301m") || !strings.Contains(a, "proxy_request_buffering off") {
		t.Fatal("admin upload proxy contract missing")
	}
	if !strings.Contains(a, "location = /api/public/app-release/download") || !strings.Contains(a, "proxy_buffering off") || !strings.Contains(a, "proxy_cache off") {
		t.Fatal("admin download proxy contract missing")
	}
	if !strings.Contains(w, "location = /api/public/app-release/download") || !strings.Contains(w, "proxy_buffering off") || !strings.Contains(w, "proxy_cache off") {
		t.Fatal("website download proxy contract missing")
	}
	if strings.Contains(w, "client_max_body_size 301m") {
		t.Fatal("website must not expose upload override")
	}
}
