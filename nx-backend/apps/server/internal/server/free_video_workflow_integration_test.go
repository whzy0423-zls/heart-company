package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	dbstore "nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/video"
	"nine-xing/nx-backend/apps/server/internal/videoproject"
)

type workflowAPIResponse struct {
	Code    int             `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

func TestFreeVideoWorkflowRealHTTPClosure(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run the real free-video workflow closure")
	}

	var providerCalls atomic.Int64
	tripwire := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "paid provider must not be called", http.StatusTeapot)
	}))
	defer tripwire.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	database, err := dbstore.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("open isolated database: %v", err)
	}
	defer database.Close()

	root := t.TempDir()
	siteConfig := filepath.Join(root, "site-config.json")
	if err := os.WriteFile(siteConfig, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	env := config.Env{
		AdminPassword: "123456",
		AdminUsername: "admin",
		AppEnv:        "test",
		JWTSecret:     "free-workflow-test-secret",
		SiteConfig:    siteConfig,
		UploadDir:     filepath.Join(root, "uploads"),
		Video: config.VideoConfig{
			APIBase: tripwire.URL,
			APIKey:  "must-not-be-used",
			Mode:    config.VideoGenerationModeDemo,
		},
	}
	backend := httptest.NewServer(New(env, database))
	defer backend.Close()
	client := backend.Client()

	var login struct {
		AccessToken string `json:"accessToken"`
	}
	workflowRequest(t, client, http.MethodPost, backend.URL+"/api/auth/login", "", map[string]string{
		"password": "123456",
		"username": "admin",
	}, &login)
	if login.AccessToken == "" {
		t.Fatal("login did not return an access token")
	}

	var project videoproject.Project
	workflowRequest(t, client, http.MethodPost, backend.URL+"/api/video/projects", login.AccessToken, map[string]string{
		"name":          "free-workflow-closure",
		"scriptContent": "A local placeholder shot.",
	}, &project)
	if project.ID == "" || project.ScriptRevision != 1 {
		t.Fatalf("unexpected project: %+v", project)
	}

	var imported videoproject.CreateShotsFromScriptResult
	workflowRequest(t, client, http.MethodPost, backend.URL+"/api/video/projects-shots/from-script/"+project.ID, login.AccessToken, map[string]any{
		"items":          []map[string]any{{"content": "A local placeholder shot.", "index": 0}},
		"scriptRevision": project.ScriptRevision,
	}, &imported)
	if len(imported.Created) != 1 || imported.Created[0].ShotID == "" {
		t.Fatalf("unexpected script import: %+v", imported)
	}
	shotID := imported.Created[0].ShotID

	var workflow videoproject.WorkflowStatus
	workflowRequest(t, client, http.MethodGet, backend.URL+"/api/video/projects-workflow/"+project.ID, login.AccessToken, nil, &workflow)
	if workflow.GenerationMode != config.VideoGenerationModeDemo {
		t.Fatalf("workflow mode = %q, want demo", workflow.GenerationMode)
	}

	requestKey := "11111111-1111-4111-8111-111111111111"
	var batch videoproject.BatchGenerateResult
	workflowRequest(t, client, http.MethodPost, backend.URL+"/api/video/projects-batch-generate-safe/"+project.ID, login.AccessToken, map[string]any{
		"items": []map[string]string{{"requestKey": requestKey, "shotId": shotID}},
	}, &batch)
	if batch.SuccessCount != 1 || len(batch.ShotResults) != 1 || batch.ShotResults[0].GenerationID == "" {
		t.Fatalf("unexpected batch result: %+v", batch)
	}
	generationID := batch.ShotResults[0].GenerationID

	var generation video.Generation
	workflowRequest(t, client, http.MethodGet, backend.URL+"/api/video/generations/"+generationID, login.AccessToken, nil, &generation)
	if generation.Provider != "demo" || generation.Status != "completed" || !strings.HasPrefix(generation.VideoURL, "/api/uploads/") {
		t.Fatalf("unexpected local generation: %+v", generation)
	}
	probeWorkflowVideo(t, client, backend.URL+generation.VideoURL, login.AccessToken)

	var selected videoproject.Shot
	workflowRequest(t, client, http.MethodPost, backend.URL+"/api/video/shots-video-versions/set/"+shotID+"/"+generationID, login.AccessToken, nil, &selected)
	if selected.SelectedGenerationID != generationID {
		t.Fatalf("generation was not explicitly selected: %+v", selected)
	}

	var job videoproject.ComposeJob
	workflowRequest(t, client, http.MethodPost, backend.URL+"/api/video/projects-compose-safe/"+project.ID, login.AccessToken, map[string]string{
		"transition": "none",
	}, &job)
	deadline := time.Now().Add(30 * time.Second)
	for job.Status != "completed" && time.Now().Before(deadline) {
		if job.Status == "failed" {
			t.Fatalf("compose failed: %+v", job)
		}
		time.Sleep(100 * time.Millisecond)
		workflowRequest(t, client, http.MethodGet, fmt.Sprintf("%s/api/video/projects-compose-safe-status/%s/%s", backend.URL, project.ID, job.ID), login.AccessToken, nil, &job)
	}
	if job.Status != "completed" || !strings.HasPrefix(job.VideoURL, "/api/uploads/") {
		t.Fatalf("compose did not complete locally: %+v", job)
	}
	probeWorkflowVideo(t, client, backend.URL+job.VideoURL, login.AccessToken)
	if got := providerCalls.Load(); got != 0 {
		t.Fatalf("paid provider tripwire received %d requests, want 0", got)
	}
	t.Logf("zero-charge closure: project=%s shot=%s generation=%s job=%s provider_calls=0", project.ID, shotID, generationID, job.ID)
}

func workflowRequest(t *testing.T, client *http.Client, method, requestURL, token string, input any, output any) {
	t.Helper()
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, requestURL, body)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var envelope workflowAPIResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("decode %s %s response: %v: %s", method, requestURL, err, raw)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Code != 0 {
		t.Fatalf("%s %s failed: status=%d code=%d message=%s body=%s", method, requestURL, response.StatusCode, envelope.Code, envelope.Message, raw)
	}
	if output != nil {
		if err := json.Unmarshal(envelope.Data, output); err != nil {
			t.Fatalf("decode %s %s data: %v: %s", method, requestURL, err, envelope.Data)
		}
	}
}

func probeWorkflowVideo(t *testing.T, client *http.Client, videoURL, token string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, videoURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("retrieve video %s: status=%d", videoURL, response.StatusCode)
	}
	path := filepath.Join(t.TempDir(), "video.mp4")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(file, response.Body); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", path)
	if output, err := command.CombinedOutput(); err != nil || strings.TrimSpace(string(output)) == "" {
		t.Fatalf("ffprobe rejected %s: %v: %s", videoURL, err, output)
	}
}
