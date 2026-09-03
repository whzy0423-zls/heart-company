package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLocationDeploymentProxyBoundaries keeps the two checked-in container
// nginx templates aligned with the privacy boundary of the location API.
// The endpoint receives search terms and coordinates in the request body, so
// neither nginx access logs nor proxy buffering may retain those values.
func TestLocationDeploymentProxyBoundaries(t *testing.T) {
	root := locationDeploymentRepoRoot(t)
	paths := []string{
		"website-react/nginx.conf",
		"nx-backend/scripts/deploy/nginx.conf",
	}
	required := []string{
		"client_max_body_size 4k;",
		"proxy_pass http://backend;",
		"proxy_http_version 1.1;",
		`proxy_set_header Connection "";`,
		"proxy_request_buffering off;",
		"proxy_buffering off;",
		"proxy_cache off;",
		"access_log off;",
		"gzip off;",
		"proxy_set_header Host $host;",
		"proxy_set_header X-Real-IP $remote_addr;",
		"proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
		"proxy_set_header X-Forwarded-Proto $scheme;",
	}

	for _, relativePath := range paths {
		t.Run(relativePath, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relativePath)))
			if err != nil {
				t.Fatal(err)
			}
			config := string(raw)
			genericStart := strings.Index(config, "location /api/ {")
			if genericStart < 0 {
				t.Fatalf("%s is missing the generic /api/ proxy location", relativePath)
			}

			for _, route := range []string{
				"location = /api/app/locations/search",
				"location = /api/app/locations/reverse",
			} {
				start := strings.Index(config, route)
				if start < 0 {
					t.Fatalf("%s is missing %s", relativePath, route)
				}
				if start > genericStart {
					t.Fatalf("%s must declare %s before generic /api/", relativePath, route)
				}
				block := locationDeploymentBlock(t, config[start:])
				normalized := strings.Join(strings.Fields(block), " ")
				for _, directive := range required {
					want := strings.Join(strings.Fields(directive), " ")
					if !strings.Contains(normalized, want) {
						t.Errorf("%s %s missing %q; block=%q", relativePath, route, directive, block)
					}
				}
				if strings.Contains(normalized, "proxy_pass http://backend/") {
					t.Errorf("%s %s must not rewrite the request path with a trailing URI", relativePath, route)
				}
			}
		})
	}
}

func TestLocationDeploymentDocumentsSecretFreeConfiguration(t *testing.T) {
	root := locationDeploymentRepoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "DEPLOY.md"))
	if err != nil {
		t.Fatal(err)
	}
	document := string(raw)
	for _, name := range []string{"AMAP_WEB_SERVICE_KEY", "AMAP_LOCATION_ENABLED"} {
		if !strings.Contains(document, name) {
			t.Fatalf("DEPLOY.md must document %s", name)
		}
	}
	// Deployment documentation may name the variables, but must not turn the
	// Web Service key into a command-line assignment or print a sample secret.
	for _, line := range strings.Split(document, "\n") {
		if !strings.Contains(line, "AMAP_WEB_SERVICE_KEY") {
			continue
		}
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
		if strings.Contains(trimmed, "=") || strings.Contains(trimmed, "TOKEN") {
			t.Fatalf("DEPLOY.md must not contain a Web Service key value: %q", line)
		}
	}
}

func locationDeploymentRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate deployment test")
	}
	dir := filepath.Dir(filename)
	for range 8 {
		if _, err := os.Stat(filepath.Join(dir, "website-react", "nginx.conf")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "nx-backend", "scripts", "deploy", "nginx.conf")); err == nil {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate repository root from %s", filename)
	return ""
}

func locationDeploymentBlock(t *testing.T, config string) string {
	t.Helper()
	config = locationDeploymentStripComments(config)
	open := strings.IndexByte(config, '{')
	if open < 0 {
		t.Fatal("location is missing its opening brace")
	}
	depth := 0
	var quote byte
	escaped := false
	for index := open; index < len(config); index++ {
		char := config[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		switch char {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return config[:index+1]
			}
		}
	}
	t.Fatal("location is missing its closing brace")
	return ""
}

func locationDeploymentStripComments(config string) string {
	lines := strings.Split(config, "\n")
	for index, line := range lines {
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			lines[index] = line[:comment]
		}
	}
	return strings.Join(lines, "\n")
}
