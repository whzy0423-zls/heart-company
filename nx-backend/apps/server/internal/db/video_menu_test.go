package db

import (
	"os"
	"strings"
	"testing"
)

func TestVideoMenuUsesGuidedWorkflowAndRemovesShortMode(t *testing.T) {
	raw, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, forbidden := range []string{"VideoProductionShort", "/video/production/short"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("deprecated short menu remains: %s", forbidden)
		}
	}
	for _, required := range []string{"/video/projects/workflow", "VideoProjectAdvancedWorkbench", "/video/projects/:id/workbench/advanced"} {
		if !strings.Contains(source, required) {
			t.Fatalf("guided menu contract missing: %s", required)
		}
	}
}
