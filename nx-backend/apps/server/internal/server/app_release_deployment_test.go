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

func TestClassroomProductionImageIncludesMediaProbeTools(t *testing.T) {
	raw, err := os.ReadFile("../../Dockerfile")
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(raw)
	if !strings.Contains(dockerfile, "apk add --no-cache ca-certificates tzdata ffmpeg") {
		t.Fatal("production server image must install the ffmpeg package for ffmpeg and ffprobe")
	}
}

func TestServerDeploymentAllowsGracefulShutdownWindow(t *testing.T) {
	composeRaw, err := os.ReadFile("../../../../../docker-compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	compose := string(composeRaw)
	serverStart := strings.Index(compose, "  server:\n")
	adminStart := strings.Index(compose, "  admin:\n")
	if serverStart < 0 || adminStart <= serverStart {
		t.Fatal("docker-compose.yml must contain server followed by admin service")
	}
	serverService := compose[serverStart:adminStart]
	if !strings.Contains(serverService, "stop_grace_period: 330s") {
		t.Fatal("server service must allow the 330s application shutdown window")
	}

	deployRaw, err := os.ReadFile("../../../../../DEPLOY.md")
	if err != nil {
		t.Fatal(err)
	}
	deploy := string(deployRaw)
	for _, required := range []string{
		"stop_grace_period: 330s",
		"TimeoutStopSec=330s",
	} {
		if !strings.Contains(deploy, required) {
			t.Fatalf("DEPLOY.md must document %q", required)
		}
	}
}
