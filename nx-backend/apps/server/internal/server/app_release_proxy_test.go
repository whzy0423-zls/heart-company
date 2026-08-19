package server

import (
	"os"
	"path/filepath"
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

func TestAppReleaseDownloadProxyPreservesRangeRequests(t *testing.T) {
	repoRoot := appChatStreamTestRepoRoot(t)
	configPaths := []string{
		"website-react/nginx.conf",
		"nx-backend/scripts/deploy/nginx.conf",
	}
	requiredDirectives := []string{
		"proxy_set_header Range $http_range;",
		"proxy_set_header If-Range $http_if_range;",
		"proxy_force_ranges on;",
	}

	for _, relativePath := range configPaths {
		t.Run(relativePath, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
			if err != nil {
				t.Fatal(err)
			}
			config := appChatNginxStripComments(string(body))
			location := "location ~ ^/api/public/app-releases/[0-9]+/download$"
			start := strings.Index(config, location)
			if start < 0 {
				t.Fatalf("%s missing versioned app release download location", relativePath)
			}
			block := appChatNginxLocationBlock(t, config[start:])
			for _, directive := range requiredDirectives {
				if !appChatNginxLocationHasDirective(block, directive) {
					t.Errorf("%s app release download location missing %q; block=%q", relativePath, directive, block)
				}
			}
		})
	}
}
