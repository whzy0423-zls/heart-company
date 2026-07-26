package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXinzhiliRealtimeNginxContract(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "..", ".."))
	paths := []string{
		filepath.Join(root, "nx-backend", "scripts", "deploy", "nginx.conf"),
		filepath.Join(root, "website-react", "nginx.conf"),
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(raw)
		start := strings.Index(text, "location = /api/app/xinzhili/realtime")
		if start < 0 {
			t.Fatalf("%s missing exact xinzhili realtime location", path)
		}
		blockEnd := strings.Index(text[start:], "\n    }")
		if blockEnd < 0 {
			t.Fatalf("%s has unterminated xinzhili realtime location", path)
		}
		block := text[start : start+blockEnd]
		for _, required := range []string{
			"proxy_http_version 1.1;",
			"proxy_set_header Upgrade $http_upgrade;",
			"proxy_set_header Connection \"upgrade\";",
			"proxy_buffering off;",
			"proxy_request_buffering off;",
			"gzip off;",
			"proxy_read_timeout 3600s;",
			"proxy_send_timeout 3600s;",
			"access_log off;",
		} {
			if !strings.Contains(block, required) {
				t.Errorf("%s xinzhili realtime location missing %q", path, required)
			}
		}
	}
}
