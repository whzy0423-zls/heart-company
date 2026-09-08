package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDirectRealtimeProxyUpgradesWebSocket(t *testing.T) {
	repoRoot := appChatStreamTestRepoRoot(t)
	configPaths := []string{
		"website-react/nginx.conf",
		"website-react-motion/nginx.conf",
		"nx-backend/scripts/deploy/nginx.conf",
	}
	requiredDirectives := []string{
		"proxy_http_version 1.1;",
		"proxy_set_header Upgrade $http_upgrade;",
		`proxy_set_header Connection "upgrade";`,
		"proxy_buffering off;",
		"proxy_read_timeout 3600s;",
	}

	for _, relativePath := range configPaths {
		t.Run(relativePath, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
			if err != nil {
				t.Fatal(err)
			}
			config := appChatNginxStripComments(string(body))
			location := "location = /api/app/direct-realtime/ws"
			start := strings.Index(config, location)
			if start < 0 {
				t.Fatalf("%s missing direct chat websocket location", relativePath)
			}
			block := appChatNginxLocationBlock(t, config[start:])
			for _, directive := range requiredDirectives {
				if !appChatNginxLocationHasDirective(block, directive) {
					t.Errorf("%s direct chat websocket location missing %q; block=%q", relativePath, directive, block)
				}
			}
		})
	}
}
