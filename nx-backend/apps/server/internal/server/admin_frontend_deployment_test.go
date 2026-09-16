package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminFrontendDeploymentSurvivesHashedAssetRotation(t *testing.T) {
	repoRoot := appChatStreamTestRepoRoot(t)

	nginxBody, err := os.ReadFile(filepath.Join(repoRoot, "nx-backend", "scripts", "deploy", "nginx.conf"))
	if err != nil {
		t.Fatal(err)
	}
	nginx := appChatNginxStripComments(string(nginxBody))
	for _, required := range []string{
		"location = /index.html",
		`add_header Cache-Control "no-cache, no-store, must-revalidate" always;`,
		"location ~* ^/(?:js|jse|css)/",
		"try_files $uri =404;",
		`add_header Cache-Control "public, max-age=31536000, immutable" always;`,
	} {
		if !strings.Contains(nginx, required) {
			t.Errorf("admin nginx config missing %q", required)
		}
	}

	mainBody, err := os.ReadFile(filepath.Join(repoRoot, "nx-backend", "apps", "web-antd", "src", "main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	mainSource := string(mainBody)
	for _, required := range []string{
		"vite:preloadError",
		"event.preventDefault()",
		"sessionStorage",
		"window.location.replace",
	} {
		if !strings.Contains(mainSource, required) {
			t.Errorf("admin entrypoint missing deployment recovery behavior %q", required)
		}
	}
}
