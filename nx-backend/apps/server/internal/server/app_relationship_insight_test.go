package server

import (
	"os"
	"strings"
	"testing"
)

func TestRelationshipInsightRoutesAreRegistered(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, route := range []string{
		`/api/app/direct/conversations/`,
		`/api/app/relationship-insights/`,
	} {
		if !strings.Contains(source, route) {
			t.Fatalf("missing relationship insight route %s", route)
		}
	}
}
