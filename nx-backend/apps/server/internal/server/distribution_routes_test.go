package server

import (
	"os"
	"strings"
	"testing"
)

func TestDistributionRouteContract(t *testing.T) {
	b, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, route := range []string{
		"/api/public/distribution/invite",
		"/api/app/distribution/overview",
		"/api/app/distribution/profile",
		"/api/app/distribution/users",
		"/api/app/distribution/orders",
		"/api/agent/distribution/profile",
		"/api/agent/distribution/analytics",
		"/api/agent/distribution/agents",
		"/api/admin/distribution/agents",
		"/api/admin/distribution/analytics",
		"/api/admin/distribution/commissions",
		"/api/admin/distribution/settlements",
		"/api/admin/distribution/rules",
	} {
		if !strings.Contains(s, route) {
			t.Errorf("missing distribution route %q", route)
		}
	}
}
