package server

import (
	"os"
	"strings"
	"testing"
)

func TestVideoWorkflowRouteContracts(t *testing.T) {
	serverRaw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	routesRaw, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatal(err)
	}
	serverSource := string(serverRaw)
	routeSource := string(routesRaw)
	for _, path := range []string{
		"/api/video/projects-workflow/",
		"/api/video/generation-submissions/",
		"/api/video/projects-shots/from-script/",
		"/api/video/shots-generate-safe/",
		"/api/video/projects-batch-generate-safe/",
		"/api/video/generation-submissions/reconcile/",
		"/api/video/shots-video-versions/set/",
		"/api/video/projects-compose-safe/",
		"/api/video/projects-compose-safe-status/",
	} {
		if !strings.Contains(serverSource, path) {
			t.Errorf("server route registration missing %s", path)
		}
	}
	for _, handler := range []string{
		"videoWorkflowGet",
		"videoWorkflowImport",
		"videoWorkflowGenerate",
		"videoWorkflowBatchGenerate",
		"videoWorkflowSubmissionStatus",
		"videoWorkflowReconcile",
		"videoWorkflowCompose",
		"videoWorkflowComposeStatus",
	} {
		if !strings.Contains(routeSource, "func (s *Server) "+handler) {
			t.Errorf("workflow handler missing %s", handler)
		}
	}
	for _, status := range []string{
		"http.StatusBadRequest",
		"http.StatusAccepted",
		"http.StatusConflict",
		"http.StatusUnprocessableEntity",
	} {
		if !strings.Contains(routeSource, status) {
			t.Errorf("workflow error mapping missing %s", status)
		}
	}
}

func TestVideoWorkflowLegacyRoutesDelegateToSafeServices(t *testing.T) {
	raw, err := os.ReadFile("videoproject_routes.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, fragment := range []string{
		"GenerateShot(r.Context(), id, input.RequestKey)",
		"GenerateSafe(r.Context(), id, input.Items",
		"StartCompose(r.Context(), id, input)",
		"缺少生成请求键",
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("legacy safe delegation missing %q", fragment)
		}
	}
}
