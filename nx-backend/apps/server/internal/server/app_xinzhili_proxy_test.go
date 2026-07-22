package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXinzhiliVoiceProxyConfig(t *testing.T) {
	repoRoot := appChatStreamTestRepoRoot(t)
	configPaths := []string{
		"website-react/nginx.conf",
		"nx-backend/scripts/deploy/nginx.conf",
	}
	requiredDirectives := []string{
		"client_max_body_size 11m;",
		"proxy_pass http://backend;",
		"proxy_http_version 1.1;",
		`proxy_set_header Connection "";`,
		"proxy_request_buffering off;",
		"proxy_buffering off;",
		"proxy_cache off;",
		"gzip off;",
		"proxy_connect_timeout 30s;",
		"proxy_read_timeout 180s;",
		"proxy_send_timeout 180s;",
		"proxy_set_header Host $host;",
		"proxy_set_header X-Real-IP $remote_addr;",
		"proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
		"proxy_set_header X-Forwarded-Proto $scheme;",
	}

	for _, relativePath := range configPaths {
		t.Run(relativePath, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
			if err != nil {
				t.Fatal(err)
			}
			config := string(body)
			exactLocation := "location = /api/app/xinzhili/turns/stream"
			exactStart := strings.Index(config, exactLocation)
			if exactStart < 0 {
				t.Fatalf("%s missing exact xinzhili voice streaming location", relativePath)
			}
			genericStart := strings.Index(config, "location /api/")
			if genericStart < 0 {
				t.Fatalf("%s missing generic /api/ location", relativePath)
			}
			if exactStart > genericStart {
				t.Fatalf("%s exact xinzhili voice location must appear before generic /api/ location", relativePath)
			}

			block := appChatNginxLocationBlock(t, config[exactStart:])
			firstLine := strings.TrimSpace(strings.SplitN(block, "{", 2)[0])
			if firstLine != exactLocation {
				t.Fatalf("%s matched wrong location header %q", relativePath, firstLine)
			}
			normalized := strings.Join(strings.Fields(block), " ")
			for _, directive := range requiredDirectives {
				if !strings.Contains(normalized, strings.Join(strings.Fields(directive), " ")) {
					t.Errorf("%s xinzhili voice location missing %q; block=%q", relativePath, directive, block)
				}
			}
		})
	}
}
