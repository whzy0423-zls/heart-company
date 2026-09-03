package main

import (
	"strings"
	"testing"
)

func TestRunRequiresCatalogDirectory(t *testing.T) {
	err := run([]string{"-validate-only", "-catalog", ""})
	if err == nil || !strings.Contains(err.Error(), "directory is required") {
		t.Fatalf("expected catalog directory error, got %v", err)
	}
}
