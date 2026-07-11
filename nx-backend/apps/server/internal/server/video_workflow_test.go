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
	workflowRaw, err := os.ReadFile("../videoproject/workflow.go")
	if err != nil {
		t.Fatal(err)
	}
	serverSource := string(serverRaw)
	routeSource := string(routesRaw)
	workflowSource := string(workflowRaw)
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
	if !strings.Contains(routeSource, "result.GenerationMode = s.videoStore().GenerationMode()") {
		t.Error("workflow response does not assign the effective generation mode")
	}
	if !strings.Contains(workflowSource, "`json:\"generationMode\"`") {
		t.Error("workflow response type does not expose generationMode")
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

func TestServerStartupRecoversInterruptedComposeJobs(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, fragment := range []string{
		"RecoverInterruptedComposeJobs",
		"服务重启导致合成中断，请重试",
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("server startup compose recovery missing %q", fragment)
		}
	}
}

func TestServerStartupRecoversInterruptedSubmissionsBeforeRoutes(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	recovery := strings.Index(source, "RecoverInterruptedSubmissions")
	routes := strings.Index(source, "s.routes()")
	if recovery < 0 {
		t.Fatal("server startup submission recovery is missing")
	}
	if routes < 0 || recovery > routes {
		t.Fatal("submission recovery must finish before routes are exposed")
	}
	for _, fragment := range []string{
		"context.WithTimeout(context.Background(), 15*time.Second)",
		"上游请求结果不确定",
		"禁止重复提交",
		"本地演练视频生成中断",
	} {
		if !strings.Contains(source, fragment) {
			t.Errorf("server startup submission recovery missing %q", fragment)
		}
	}
}
