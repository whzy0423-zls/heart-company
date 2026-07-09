package server

import (
	"os"
	"strings"
	"testing"
)

func TestAppProfileCalibrationRoutesAreRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, route := range []string{
		"/api/app/daily-quiz/today",
		"/api/app/daily-quiz/progress",
		"/api/app/daily-quiz/answer",
		"/api/app/daily-quiz/complete",
		"/api/app/reassessment/latest",
		"/api/app/reassessment/",
	} {
		if !strings.Contains(source, route) {
			t.Fatalf("expected server routes to include %s", route)
		}
	}
}
