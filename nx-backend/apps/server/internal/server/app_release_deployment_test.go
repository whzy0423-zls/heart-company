package server

import (
	"os"
	"strings"
	"testing"
)

func TestAppReleaseDeploymentConfiguration(t *testing.T) {
	files := []string{
		"../../../../../.env.example",
		"../../.env.example",
		"../../../../../docker-compose.yml",
		"../../Dockerfile",
		"../../../../../DEPLOY.md",
	}
	var combined strings.Builder
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		combined.Write(raw)
	}
	text := combined.String()
	for _, required := range []string{"APP_RELEASE_PACKAGE_NAME", "APP_RELEASE_CERT_SHA256", "/data/uploads/app-releases", "301 MiB", "app-releases.tgz"} {
		if !strings.Contains(text, required) {
			t.Fatalf("deployment files must document %q", required)
		}
	}
}
