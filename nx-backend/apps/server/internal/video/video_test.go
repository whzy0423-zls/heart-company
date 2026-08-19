package video

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

func allowLocalTestClient(client *Client) *Client {
	// Most legacy HTTP tests construct VideoConfig directly instead of going
	// through config.Load, whose production default injects this contract.
	if client.gatewayContract.Name == "" && client.gatewayContract.Version == "" && client.gatewayContract.References.Mode == "" {
		client.gatewayContract = LegacyFlatContract()
	}
	client.urlAllowed = func(string) bool { return true }
	client.client.Transport = &http.Transport{
		DisableKeepAlives: true,
		Proxy:             http.ProxyFromEnvironment,
	}
	return client
}

func allowLocalTestStore(store *Store) *Store {
	store.generationMode = config.VideoGenerationModePaid
	store.client = allowLocalTestClient(store.client)
	return store
}

func TestNewStoreDefaultsGenerationModeToDemo(t *testing.T) {
	tests := []struct {
		name string
		mode string
	}{
		{name: "omitted"},
		{name: "unknown", mode: "unexpected"},
		{name: "paid with different case", mode: "PAID"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore(nil, nil, config.VideoConfig{Mode: tc.mode})
			if got := store.GenerationMode(); got != config.VideoGenerationModeDemo {
				t.Fatalf("GenerationMode() = %q, want demo", got)
			}
		})
	}
}

func TestNewStoreUsesPaidModeOnlyWhenExplicit(t *testing.T) {
	store := NewStore(nil, nil, config.VideoConfig{Mode: config.VideoGenerationModePaid})
	if got := store.GenerationMode(); got != config.VideoGenerationModePaid {
		t.Fatalf("GenerationMode() = %q, want paid", got)
	}
}

func TestPreparedSubmissionSnapshotContainsGenerationMode(t *testing.T) {
	store := NewStore(nil, nil, config.VideoConfig{Mode: config.VideoGenerationModeDemo})
	prepared, err := store.prepareGenerationSubmission(
		GenerateInput{RequestKey: submissionKeyOne},
		store.defaultModel,
		"local demo",
		5,
		"16:9",
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(prepared.snapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if got := snapshot["generationMode"]; got != config.VideoGenerationModeDemo {
		t.Fatalf("snapshot generationMode = %#v, want demo", got)
	}
}

func TestGenerateDemoCompletesLocallyAndReusesRequestKey(t *testing.T) {
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "demo must stay local", http.StatusTeapot)
	}))
	defer provider.Close()

	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	path := t.TempDir() + "/demo.mp4"
	if err := os.WriteFile(path, []byte("demo-video-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	renderer := &recordingDemoRenderer{result: DemoVideo{
		Duration: 5,
		FPS:      24,
		Height:   640,
		Path:     path,
		Width:    360,
	}}
	uploader := &recordingVideoUploader{
		url:       "/api/uploads/video/generated/demo.mp4",
		objectKey: "video/generated/demo.mp4",
	}
	store := NewStore(database, nil, config.VideoConfig{
		APIBase: provider.URL,
		APIKey:  "must-not-be-used",
		Mode:    config.VideoGenerationModeDemo,
	}, uploader)
	store.demoRenderer = renderer
	input := GenerateInput{
		AspectRatio: "9:16",
		Prompt:      "local demo",
		RequestKey:  submissionKeyOne,
		Seconds:     5,
		ShotID:      "9",
	}

	first, err := store.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if providerCalls.Load() != 0 {
		t.Fatalf("demo generation contacted provider %d times", providerCalls.Load())
	}
	if renderer.calls != 1 || uploader.uploadCalls != 1 || state.uploadCreateCalls != 1 {
		t.Fatalf("demo work calls: render=%d upload=%d asset=%d, want 1 each", renderer.calls, uploader.uploadCalls, state.uploadCreateCalls)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("same request key returned generations %q and %q", first.ID, second.ID)
	}
	if first.Provider != "demo" || first.Status != "completed" || first.SubmissionStatus != SubmissionCompleted {
		t.Fatalf("demo generation did not complete: %+v", first)
	}
	if first.VideoAssetID != "7" || first.VideoURL != uploader.url {
		t.Fatalf("demo upload linkage = asset %q url %q", first.VideoAssetID, first.VideoURL)
	}
	if first.Width != 360 || first.Height != 640 || first.Duration != 5 || first.FPS != 24 {
		t.Fatalf("demo metadata = %+v", first)
	}
	if string(uploader.data) != "demo-video-bytes" {
		t.Fatalf("uploaded data = %q", uploader.data)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rendered temp file was not removed: %v", err)
	}
}

func TestGenerateDemoFailuresReleaseActiveSubmission(t *testing.T) {
	tests := []struct {
		name          string
		renderError   error
		uploaderError error
	}{
		{name: "renderer", renderError: errors.New("render failed")},
		{name: "uploader", uploaderError: errors.New("upload failed")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &videoDBState{}
			database := openVideoTestDB(t, state)
			defer database.Close()
			path := t.TempDir() + "/demo.mp4"
			if err := os.WriteFile(path, []byte("demo-video-bytes"), 0o600); err != nil {
				t.Fatal(err)
			}
			renderer := &recordingDemoRenderer{
				err:    tc.renderError,
				result: DemoVideo{Duration: 5, FPS: 24, Height: 360, Path: path, Width: 640},
			}
			uploader := &recordingVideoUploader{err: tc.uploaderError}
			store := NewStore(database, nil, config.VideoConfig{Mode: config.VideoGenerationModeDemo}, uploader)
			store.demoRenderer = renderer
			input := GenerateInput{
				Prompt:     "local demo",
				RequestKey: submissionKeyOne,
				Seconds:    5,
				ShotID:     "9",
			}

			if _, err := store.Generate(context.Background(), input); err == nil {
				t.Fatal("expected local demo failure")
			}
			failed, err := store.submissions.GetByRequestKey(context.Background(), input.RequestKey)
			if err != nil {
				t.Fatal(err)
			}
			if failed.Status != SubmissionCancelled {
				t.Fatalf("failed demo submission remained active: %+v", failed)
			}

			input.RequestKey = submissionKeyTwo
			if _, err := store.Generate(context.Background(), input); err == nil {
				t.Fatal("replacement request did not reach local demo work")
			}
			if renderer.calls != 2 {
				t.Fatalf("replacement request render calls = %d, want 2", renderer.calls)
			}
		})
	}
}

func TestGenerateRejectsRequestKeyReplayAcrossGenerationModes(t *testing.T) {
	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	path := t.TempDir() + "/demo.mp4"
	if err := os.WriteFile(path, []byte("demo-video-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	demo := NewStore(database, nil, config.VideoConfig{Mode: config.VideoGenerationModeDemo}, &recordingVideoUploader{
		url: "/api/uploads/video/generated/demo.mp4",
	})
	demo.demoRenderer = &recordingDemoRenderer{result: DemoVideo{
		Duration: 5, FPS: 24, Height: 360, Path: path, Width: 640,
	}}
	input := GenerateInput{Prompt: "same key", RequestKey: submissionKeyOne, Seconds: 5}
	if _, err := demo.Generate(context.Background(), input); err != nil {
		t.Fatal(err)
	}

	paid := NewStore(database, nil, config.VideoConfig{
		Mode:            config.VideoGenerationModePaid,
		GatewayContract: LegacyFlatContract(),
	})
	_, err := paid.Generate(context.Background(), input)
	var conflict *RequestKeyConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("cross-mode replay error = %T, want *RequestKeyConflictError: %v", err, err)
	}
}

func TestQueryTaskIncludesSeconds(t *testing.T) {
	var gotSeconds string
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSeconds = r.URL.Query().Get("seconds")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "processing",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	if _, err := client.QueryTask(context.Background(), "task-1", 15); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/videos/task-1" {
		t.Fatalf("expected task status path /v1/videos/task-1, got %q", gotPath)
	}
	if gotSeconds != "" {
		t.Fatalf("expected no seconds query on status request, got %q", gotSeconds)
	}
}

func TestCreateTaskIncludesSecondsInQuery(t *testing.T) {
	var gotBody map[string]any
	var gotSeconds string
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSeconds = r.URL.Query().Get("seconds")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	if _, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, nil, 15, "9:16"); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/videos" {
		t.Fatalf("expected create path /v1/videos, got %q", gotPath)
	}
	if gotSeconds != "" {
		t.Fatalf("expected no seconds query on create request, got %q", gotSeconds)
	}
	if gotBody["seconds"] != "15" {
		t.Fatalf("expected seconds=\"15\" in create body, got %#v", gotBody["seconds"])
	}
}

func TestCreateTaskIncludesAspectRatio(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	if _, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, nil, 15, "9:16"); err != nil {
		t.Fatal(err)
	}
	if gotBody["aspect_ratio"] != "9:16" {
		t.Fatalf("expected aspect_ratio=9:16 in create body, got %#v", gotBody["aspect_ratio"])
	}
}

func TestCreateTaskDefaultsAspectRatioTo16By9(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	if _, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, nil, 15, ""); err != nil {
		t.Fatal(err)
	}
	if gotBody["aspect_ratio"] != "16:9" {
		t.Fatalf("expected default aspect_ratio=16:9 in create body, got %#v", gotBody["aspect_ratio"])
	}
}

func TestGenerateRejectsUnsupportedAspectRatio(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()
	db := openVideoTestDB(t, &videoDBState{})
	defer db.Close()

	store := allowLocalTestStore(NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	if _, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", AspectRatio: "4:3"}); err == nil {
		t.Fatal("expected unsupported aspect ratio to be rejected")
	}
	if called {
		t.Fatal("unsupported aspect ratio should be rejected before calling video gateway")
	}
}

func TestGenerateRejectsStaleCapabilityVersion(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
	}))
	defer server.Close()
	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))

	_, err := store.Generate(context.Background(), GenerateInput{
		Prompt:            "test",
		RequestKey:        submissionKeyOne,
		CapabilityVersion: "stale-version",
	})
	assertValidationCode(t, err, "capability_version_stale")
	if attempts.Load() != 0 {
		t.Fatalf("stale capability version reached create POST %d times", attempts.Load())
	}
}

func TestGenerateNormalizedUsesSharedValidatorBeforePost(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
	}))
	defer server.Close()
	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	caps := store.Capabilities("video-ds-2.0")
	seed := 7

	_, err := store.GenerateNormalized(context.Background(), GenerateRequest{
		Model:             caps.Model,
		Prompt:            "test",
		Duration:          15,
		AspectRatio:       "16:9",
		TaskMode:          "reference",
		RequestKey:        submissionKeyOne,
		CapabilityVersion: caps.CapabilityVersion,
		Seed:              &seed,
	}, GenerationContext{})
	assertValidationCode(t, err, "seed_unsupported")
	if attempts.Load() != 0 {
		t.Fatalf("unsupported normalized request reached create POST %d times", attempts.Load())
	}
}

func TestGenerateNormalizedPersistsProjectShotContext(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-normalized"},
		})
	}))
	defer server.Close()
	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	caps := store.Capabilities("video-ds-2.0")

	generation, err := store.GenerateNormalized(context.Background(), GenerateRequest{
		Model:             caps.Model,
		Prompt:            "normalized project shot",
		Duration:          15,
		AspectRatio:       "16:9",
		TaskMode:          "reference",
		RequestKey:        submissionKeyOne,
		CapabilityVersion: caps.CapabilityVersion,
		References: []Reference{
			{ID: "image-1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/character.png", SortOrder: 1},
		},
	}, GenerationContext{ProjectID: "3", ShotID: "9"})
	if err != nil {
		t.Fatal(err)
	}
	if generation.RequestKey != submissionKeyOne || generation.SubmissionStatus != SubmissionAccepted {
		t.Fatalf("generation = %+v", generation)
	}
	submission, err := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if err != nil {
		t.Fatal(err)
	}
	if submission.ProjectID != "3" || submission.ShotID != "9" {
		t.Fatalf("submission context = %+v", submission)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGenerateTreatsSuccessfulCreateWithoutTaskIDAsAmbiguous(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status": "queued",
			},
		})
	}))
	defer server.Close()
	db := openVideoTestDB(t, &videoDBState{})
	defer db.Close()

	store := allowLocalTestStore(NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		Model:   "video-ds-2.0-fast",
	}))
	_, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	var unknown *UnknownOutcomeError
	if !errors.As(err, &unknown) {
		t.Fatalf("missing task id error type = %T, want *UnknownOutcomeError: %v", err, err)
	}
	if unknown.RequestKey != submissionKeyOne || unknown.SubmissionID == "" {
		t.Fatalf("unknown outcome identity = %+v", unknown)
	}
	if !unknown.Persisted {
		t.Fatal("missing-task outcome should report persisted recovery state")
	}

	state := videoTestState(t, db)
	if state.insertCalls != 0 {
		t.Fatalf("expected no generation row to be inserted without task_id, got %d inserts", state.insertCalls)
	}
	submission, getErr := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if submission.Status != SubmissionUnknownOutcome || submission.UpstreamTaskID != "" {
		t.Fatalf("missing-task submission = %+v", submission)
	}
}

func TestRefreshClearsOldErrorWhenFailedTaskBecomesQueued(t *testing.T) {
	state := &videoDBState{
		generation: Generation{
			ID:           "42",
			Provider:     "newapi",
			Model:        "video-ds-2.0-fast",
			Prompt:       "test",
			TaskID:       "task-1",
			Seconds:      15,
			AspectRatio:  "16:9",
			Status:       "failed",
			ErrorMessage: "previous failure",
		},
	}
	db := openVideoTestDB(t, state)
	defer db.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()

	store := allowLocalTestStore(NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	result, err := store.Refresh(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "queued" {
		t.Fatalf("expected queued, got %q", result.Status)
	}
	if result.ErrorMessage != "" {
		t.Fatalf("expected old error message to be cleared, got %q", result.ErrorMessage)
	}
	if state.statusUpdateCalls != 1 {
		t.Fatalf("expected one status update, got %d", state.statusUpdateCalls)
	}
}

func TestRefreshMarksCompletedTaskFailedWhenContentUnavailable(t *testing.T) {
	state := &videoDBState{
		generation: Generation{
			ID:          "42",
			Provider:    "newapi",
			Model:       "video-ds-2.0-fast",
			Prompt:      "test",
			TaskID:      "task-1",
			Seconds:     15,
			AspectRatio: "16:9",
			Status:      "queued",
		},
	}
	db := openVideoTestDB(t, state)
	defer db.Close()
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/task-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"status":    "completed",
					"task_id":   "task-1",
					"video_url": serverURL + "/result.mp4",
				},
			})
		case "/v1/videos/task-1/content":
			http.Error(w, "missing content", http.StatusNotFound)
		case "/result.mp4":
			http.Error(w, "forbidden", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	defer server.Close()

	store := allowLocalTestStore(NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	result, err := store.Refresh(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed when completed task content cannot be downloaded, got %q", result.Status)
	}
	if result.VideoURL != "" {
		t.Fatalf("expected no broken video url to be stored, got %q", result.VideoURL)
	}
	if !strings.Contains(result.ErrorMessage, "下载") {
		t.Fatalf("expected download error message, got %q", result.ErrorMessage)
	}
}

func TestGenerateStoresTaskWithoutCreatingAssetBeforeCompletion(t *testing.T) {
	state := &videoDBState{}
	db := openVideoTestDB(t, state)
	defer db.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/videos" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()
	uploader := &recordingVideoUploader{url: "https://cdn.example.com/video/generated/result.mp4", objectKey: "video/generated/result.mp4"}
	store := allowLocalTestStore(NewStore(db, nil, config.VideoConfig{APIBase: server.URL, APIKey: "test-key"}, uploader))
	result, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", Model: "video-ds-2.0-fast"})
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "task-1" {
		t.Fatalf("expected task id to be stored, got %q", result.TaskID)
	}
	if state.uploadCreateCalls != 0 {
		t.Fatalf("expected no upload asset before task completion, got %d inserts", state.uploadCreateCalls)
	}
}

func TestGenerateReusesRequestKey(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-reused",
			},
		})
	}))
	defer server.Close()

	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	input := GenerateInput{
		Prompt:     "same intent",
		RequestKey: submissionKeyOne,
	}

	first, err := store.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("generation ids = %q/%q", first.ID, second.ID)
	}
	if first.RequestKey != submissionKeyOne || second.RequestKey != submissionKeyOne {
		t.Fatalf("request keys = %q/%q", first.RequestKey, second.RequestKey)
	}
	if first.SubmissionID == "" || first.SubmissionID != second.SubmissionID {
		t.Fatalf("submission ids = %q/%q", first.SubmissionID, second.SubmissionID)
	}
	if first.SubmissionStatus != SubmissionAccepted || second.SubmissionStatus != SubmissionAccepted {
		t.Fatalf("submission statuses = %q/%q", first.SubmissionStatus, second.SubmissionStatus)
	}
}

func TestGenerateReusesRequestKeyWithExplicitReferences(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-reference-reused"},
		})
	}))
	defer server.Close()
	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	input := GenerateInput{
		Prompt:     "same referenced intent",
		RequestKey: submissionKeyOne,
		References: []Reference{{
			ID: "image-1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/character.png",
		}},
	}

	first, err := store.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 1 || first.ID != second.ID {
		t.Fatalf("attempts = %d, ids = %q/%q", attempts.Load(), first.ID, second.ID)
	}
}

func TestGenerateRejectsRequestKeyReuseWhenAdvancedIntentChanges(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*GenerateInput)
	}{
		{
			name: "resolution",
			mutate: func(input *GenerateInput) {
				input.Resolution = "720P"
			},
		},
		{
			name: "resolution omitted",
			mutate: func(input *GenerateInput) {
				input.Resolution = ""
			},
		},
		{
			name: "generate audio",
			mutate: func(input *GenerateInput) {
				value := false
				input.GenerateAudio = &value
			},
		},
		{
			name: "generate audio omitted",
			mutate: func(input *GenerateInput) {
				input.GenerateAudio = nil
			},
		},
		{
			name: "task mode",
			mutate: func(input *GenerateInput) {
				input.TaskMode = "edit"
			},
		},
		{
			name: "references",
			mutate: func(input *GenerateInput) {
				input.References = []Reference{{
					ID: "image-1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/character.png",
				}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"status": "queued", "task_id": "task-advanced-intent"},
				})
			}))
			defer server.Close()
			database := openVideoTestDB(t, &videoDBState{})
			defer database.Close()
			contract := configuredMapperContract()
			store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
				APIBase:         server.URL,
				APIKey:          "test-key",
				GatewayContract: contract,
			}))
			generateAudio := true
			input := GenerateInput{
				Model:         "video-ds-2.0",
				Prompt:        "advanced intent",
				Seconds:       10,
				AspectRatio:   "9:16",
				Resolution:    "1080P",
				GenerateAudio: &generateAudio,
				TaskMode:      "reference",
				RequestKey:    submissionKeyOne,
			}
			if _, err := store.Generate(context.Background(), input); err != nil {
				t.Fatal(err)
			}
			changed := input
			tt.mutate(&changed)
			_, err := store.Generate(context.Background(), changed)
			var conflict *RequestKeyConflictError
			if !errors.As(err, &conflict) {
				t.Fatalf("error = %T %v, want *RequestKeyConflictError", err, err)
			}
			if attempts.Load() != 1 {
				t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
			}
		})
	}
}

func TestGenerateRejectsRequestKeyReuseWhenReferenceRoleChangesAcrossInputShapes(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-reference-role"},
		})
	}))
	defer server.Close()
	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	contract := configuredMapperContract()
	contract.Limits = config.MediaLimits{MaxImages: 4, MaxVideos: 3, MaxAudios: 1, MaxVideoSecondsTotal: 15, MaxAudioSecondsTotal: 15}
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: contract,
	}))
	const referenceURL = "https://cdn.example.com/frame.png"
	first := GenerateInput{
		Model:       "video-ds-2.0",
		Prompt:      "reference role intent",
		Seconds:     10,
		AspectRatio: "9:16",
		TaskMode:    "reference",
		RequestKey:  submissionKeyOne,
		References: []Reference{{
			ID: "image-1", Kind: "image", Role: "first_frame", URL: referenceURL,
		}},
	}
	if _, err := store.Generate(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	changed := first
	changed.References = nil
	changed.Images = []string{referenceURL}
	_, err := store.Generate(context.Background(), changed)
	var conflict *RequestKeyConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want *RequestKeyConflictError", err, err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGenerateNormalizedRejectsRequestKeyReuseWhenUsageNoteChanges(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-usage-note"},
		})
	}))
	defer server.Close()
	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	capabilities := store.Capabilities("video-ds-2.0")
	request := GenerateRequest{
		Model:             capabilities.Model,
		Prompt:            "usage note intent",
		Duration:          10,
		AspectRatio:       "16:9",
		TaskMode:          "reference",
		RequestKey:        submissionKeyOne,
		CapabilityVersion: capabilities.CapabilityVersion,
		References: []Reference{{
			ID: "image-1", Kind: "image", Role: "reference_image", URL: "https://cdn.example.com/character.png", UsageNote: "人物外观",
		}},
	}
	if _, err := store.GenerateNormalized(context.Background(), request, GenerationContext{}); err != nil {
		t.Fatal(err)
	}
	request.References[0].UsageNote = "人物服饰"
	_, err := store.GenerateNormalized(context.Background(), request, GenerationContext{})
	var conflict *RequestKeyConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T %v, want *RequestKeyConflictError", err, err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGenerateReusesAcceptedRequestAfterGatewayCapabilityChange(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-historical"},
		})
	}))
	defer server.Close()

	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	contractV1 := LegacyFlatContract()
	storeV1 := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: contractV1,
	}))
	input := GenerateInput{Prompt: "historical intent", RequestKey: submissionKeyOne}
	first, err := storeV1.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	contractV2 := LegacyFlatContract()
	contractV2.Version = "2"
	storeV2 := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		Model:           "as-sd2.0-fast",
		GatewayContract: contractV2,
	}))
	second, err := storeV2.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.TaskID != "task-historical" {
		t.Fatalf("historical generation = first %+v second %+v", first, second)
	}
	if second.SubmissionStatus != SubmissionAccepted {
		t.Fatalf("historical submission status = %q", second.SubmissionStatus)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGenerateConcurrentRequestKeyPostsOnce(t *testing.T) {
	var attempts atomic.Int32
	received := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			close(received)
		}
		<-release
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-concurrent",
			},
		})
	}))
	defer server.Close()

	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	input := GenerateInput{Prompt: "same intent", RequestKey: submissionKeyOne}

	firstDone := make(chan error, 1)
	go func() {
		_, err := store.Generate(context.Background(), input)
		firstDone <- err
	}()
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("first request did not reach fake gateway")
	}

	_, err := store.Generate(context.Background(), input)
	var inProgress *SubmissionInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("second error = %T, want *SubmissionInProgressError: %v", err, err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGenerateRejectsDifferentKeyWhileShotSubmissionIsActive(t *testing.T) {
	var attempts atomic.Int32
	received := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			close(received)
		}
		<-release
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-shot-lock"},
		})
	}))
	defer server.Close()

	database := openVideoTestDB(t, &videoDBState{})
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	firstInput := GenerateInput{
		Prompt:     "shot intent",
		ProjectID:  "3",
		ShotID:     "9",
		RequestKey: submissionKeyOne,
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := store.Generate(context.Background(), firstInput)
		firstDone <- err
	}()
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("first shot request did not reach fake gateway")
	}

	secondInput := firstInput
	secondInput.RequestKey = submissionKeyTwo
	_, err := store.Generate(context.Background(), secondInput)
	var active *ActiveSubmissionError
	if !errors.As(err, &active) {
		t.Fatalf("second error = %T, want *ActiveSubmissionError: %v", err, err)
	}
	if active.Existing.RequestKey != submissionKeyOne || active.ShotID != "9" {
		t.Fatalf("active submission error = %+v", active)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestRefreshUsesPublicObjectURLWhenFallbackDownloadSucceeds(t *testing.T) {
	state := &videoDBState{
		generation: Generation{
			ID:          "42",
			Provider:    "newapi",
			Model:       "video-ds-2.0-fast",
			Prompt:      "test",
			TaskID:      "task-1",
			Seconds:     15,
			AspectRatio: "16:9",
			Status:      "queued",
		},
	}
	db := openVideoTestDB(t, state)
	defer db.Close()
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/task-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"status":    "completed",
					"task_id":   "task-1",
					"video_url": serverURL + "/result.mp4",
					"duration":  float64(15),
					"fps":       float64(30),
					"width":     float64(1280),
					"height":    float64(720),
				},
			})
		case "/v1/videos/task-1/content":
			http.Error(w, "missing content", http.StatusNotFound)
		case "/result.mp4":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	serverURL = server.URL
	defer server.Close()

	uploader := &recordingVideoUploader{url: "https://cdn.example.com/video/generated/result.mp4", objectKey: "video/generated/result.mp4"}
	store := allowLocalTestStore(NewStore(db, uploadasset.NewStore(db), config.VideoConfig{APIBase: server.URL, APIKey: "test-key"}, uploader))
	result, err := store.Refresh(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "completed" {
		t.Fatalf("expected completed when fallback download succeeds, got %q", result.Status)
	}
	if result.VideoURL != "https://cdn.example.com/video/generated/result.mp4" {
		t.Fatalf("expected public object url, got %q", result.VideoURL)
	}
	if state.uploadCreateCalls != 1 {
		t.Fatalf("expected one upload asset insert, got %d", state.uploadCreateCalls)
	}
	if got := state.uploadAsset["object_url"]; got != "https://cdn.example.com/video/generated/result.mp4" {
		t.Fatalf("expected public object url to be stored, got %#v", got)
	}
}

func TestRefreshMirrorsSubmissionCompleted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/videos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "queued", "task_id": "task-complete"},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/task-complete":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"status":   "completed",
					"task_id":  "task-complete",
					"duration": float64(15),
					"fps":      float64(30),
					"width":    float64(1280),
					"height":   float64(720),
				},
			})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/videos/task-complete/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	uploader := &recordingVideoUploader{url: "https://cdn.example.com/video/generated/result.mp4", objectKey: "video/generated/result.mp4"}
	store := allowLocalTestStore(NewStore(database, uploadasset.NewStore(database), config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}, uploader))

	created, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := store.Refresh(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != "completed" {
		t.Fatalf("generation status = %q", completed.Status)
	}
	submission, err := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Status != SubmissionCompleted {
		t.Fatalf("submission status = %q, want completed", submission.Status)
	}
}

func TestRefreshMirrorsSubmissionFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "queued", "task_id": "task-failed"},
			})
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"status":        "failed",
					"task_id":       "task-failed",
					"error_message": "upstream rejected render",
				},
			})
		}
	}))
	defer server.Close()

	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))

	created, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Refresh(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" {
		t.Fatalf("generation status = %q", failed.Status)
	}
	submission, err := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Status != SubmissionFailed {
		t.Fatalf("submission status = %q, want failed", submission.Status)
	}
}

func TestRefreshKeepsReconciledSubmissionAsAuditOutcome(t *testing.T) {
	var createAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			createAttempts.Add(1)
			http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":        "failed",
				"task_id":       "task-reconciled",
				"error_message": "render failed",
			},
		})
	}))
	defer server.Close()

	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	_, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	var unknown *UnknownOutcomeError
	if !errors.As(err, &unknown) {
		t.Fatalf("setup error = %T, want *UnknownOutcomeError: %v", err, err)
	}
	reconciled, err := store.ReconcileSubmission(context.Background(), submissionKeyOne, "task-reconciled")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.Refresh(context.Background(), reconciled.Generation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" || failed.SubmissionStatus != SubmissionReconciled {
		t.Fatalf("refreshed generation = %+v", failed)
	}
	submission, err := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Status != SubmissionReconciled {
		t.Fatalf("submission status = %q, want reconciled audit outcome", submission.Status)
	}
	if createAttempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", createAttempts.Load())
	}
}

func TestRefreshMarksCompletedTaskFailedWhenUploadHasNoPublicObjectURL(t *testing.T) {
	state := &videoDBState{
		generation: Generation{
			ID:          "42",
			Provider:    "newapi",
			Model:       "video-ds-2.0-fast",
			Prompt:      "test",
			TaskID:      "task-1",
			Seconds:     15,
			AspectRatio: "16:9",
			Status:      "queued",
		},
	}
	db := openVideoTestDB(t, state)
	defer db.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/videos/task-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"status":  "completed",
					"task_id": "task-1",
				},
			})
		case "/v1/videos/task-1/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	store := allowLocalTestStore(NewStore(db, uploadasset.NewStore(db), config.VideoConfig{APIBase: server.URL, APIKey: "test-key"}))
	result, err := store.Refresh(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed without public object url, got %q", result.Status)
	}
	if result.VideoURL != "" {
		t.Fatalf("expected no local preview url to be stored as completed video, got %q", result.VideoURL)
	}
	if !strings.Contains(result.ErrorMessage, "文件桶公网") {
		t.Fatalf("expected public object url error, got %q", result.ErrorMessage)
	}
}

func TestCreateTaskIncludesReferenceAudios(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	if _, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, []string{"https://example.com/input.mp3"}, 15, "9:16"); err != nil {
		t.Fatal(err)
	}
	audios, ok := gotBody["audios"].([]any)
	if !ok || len(audios) != 1 || audios[0] != "https://example.com/input.mp3" {
		t.Fatalf("expected audios to be forwarded, got %#v", gotBody["audios"])
	}
}

func TestCreateTaskLegacyArraysUseCanonicalReferences(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-1"},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	_, err := client.CreateTask(
		context.Background(),
		"video-ds-2.0-fast",
		"test",
		[]string{"https://example.com/same.png", "https://example.com/same.png"},
		nil,
		nil,
		15,
		"9:16",
	)
	assertValidationCode(t, err, "duplicate_reference")
	if attempts != 0 {
		t.Fatalf("duplicate legacy references reached create POST %d times, want 0", attempts)
	}
}

func TestGenerateLegacyArraysUseCanonicalDuplicateRules(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-1"},
		})
	}))
	defer server.Close()
	db := openVideoTestDB(t, &videoDBState{})
	defer db.Close()

	store := allowLocalTestStore(NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	_, err := store.Generate(context.Background(), GenerateInput{
		Prompt: "test",
		Images: []string{" https://example.com/same.png ", "https://example.com/same.png"},
	})
	assertValidationCode(t, err, "duplicate_reference")
	if got := attempts.Load(); got != 0 {
		t.Fatalf("duplicate GenerateInput references reached create POST %d times, want 0", got)
	}
}

func TestCreateTaskUsesDeclaredIdempotencyHeader(t *testing.T) {
	tests := []struct {
		name          string
		header        string
		wantRequestID string
	}{
		{name: "declared header uses exact request key", header: "Idempotency-Key", wantRequestID: "123e4567-e89b-12d3-a456-426614174000"},
		{name: "blank header sends no idempotency key", header: "", wantRequestID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotHeader http.Header
			var gotBody map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHeader = r.Header.Clone()
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Fatal(err)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"status": "queued", "task_id": "task-1"},
				})
			}))
			defer server.Close()

			contract := LegacyFlatContract()
			contract.Idempotency.Header = tt.header
			client := allowLocalTestClient(NewClient(config.VideoConfig{
				APIBase:         server.URL,
				APIKey:          "test-key",
				GatewayContract: contract,
			}))
			_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
				Model:       "video-ds-2.0-fast",
				Prompt:      "test",
				Duration:    15,
				AspectRatio: "9:16",
				TaskMode:    "reference",
				RequestKey:  "123e4567-e89b-12d3-a456-426614174000",
			}, CanonicalReferences{})
			if err != nil {
				t.Fatal(err)
			}

			if got := gotHeader.Get("Idempotency-Key"); got != tt.wantRequestID {
				t.Fatalf("Idempotency-Key = %q, want %q", got, tt.wantRequestID)
			}
			if tt.header == "" && gotHeader.Get("X-Request-Key") != "" {
				t.Fatalf("blank contract synthesized X-Request-Key = %q", gotHeader.Get("X-Request-Key"))
			}
			if gotHeader.Get("seconds") != "" || gotHeader.Get("aspect_ratio") != "" {
				t.Fatalf("contract body fields leaked into headers: %#v", gotHeader)
			}
			if _, exists := gotBody["requestKey"]; exists {
				t.Fatalf("requestKey leaked into undeclared JSON body: %#v", gotBody)
			}
		})
	}
}

func TestCreateTaskRequiresValidRequestKeyForDeclaredIdempotencyHeader(t *testing.T) {
	tests := []struct {
		name string
		key  string
		code string
	}{
		{name: "empty", key: "", code: "request_key_required"},
		{name: "blank", key: " \t ", code: "request_key_required"},
		{name: "not uuid", key: "req-123", code: "request_key_invalid"},
		{name: "control character", key: "123e4567-e89b-12d3-a456-426614174000\n", code: "request_key_invalid"},
		{name: "zero uuid", key: "00000000-0000-0000-0000-000000000000", code: "request_key_invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"status": "queued", "task_id": "task-1"},
				})
			}))
			defer server.Close()

			contract := LegacyFlatContract()
			contract.Idempotency.Header = "Idempotency-Key"
			client := allowLocalTestClient(NewClient(config.VideoConfig{
				APIBase:         server.URL,
				APIKey:          "test-key",
				GatewayContract: contract,
			}))
			_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
				Model:       "video-ds-2.0-fast",
				Prompt:      "test",
				Duration:    15,
				AspectRatio: "9:16",
				TaskMode:    "reference",
				RequestKey:  tt.key,
			}, CanonicalReferences{})
			validationErr := assertValidationCode(t, err, tt.code)
			if validationErr.Field != "requestKey" {
				t.Fatalf("validation field = %q, want requestKey", validationErr.Field)
			}
			if attempts.Load() != 0 {
				t.Fatalf("invalid request key reached create POST %d times", attempts.Load())
			}
		})
	}
}

func TestCreateTaskFailsClosedForExplicitInvalidGatewayContract(t *testing.T) {
	for _, tt := range []struct {
		name      string
		raw       string
		wantField string
	}{
		{name: "malformed legacy JSON", raw: "{", wantField: "gatewayContract"},
		{name: "invalid parsed legacy JSON", raw: `{"references":{"mode":"future_items"}}`, wantField: "gatewayContract.references.mode"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"status": "queued", "task_id": "task-1"},
				})
			}))
			defer server.Close()

			t.Setenv("VIDEO_API_BASE", server.URL)
			t.Setenv("VIDEO_API_KEY", "test-key")
			t.Setenv("VIDEO_GATEWAY_CONTRACT", "legacy_flat_v1")
			t.Setenv("VIDEO_GATEWAY_CONTRACT_VERSION", "1")
			t.Setenv("VIDEO_GATEWAY_CONTRACT_JSON", tt.raw)
			client := NewClient(config.Load().Video)
			client.urlAllowed = func(string) bool { return true }
			client.client.Transport = &http.Transport{DisableKeepAlives: true}

			_, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, nil, 15, "9:16")
			validationErr := assertValidationCode(t, err, "gateway_contract_invalid")
			if validationErr.Field != tt.wantField {
				t.Fatalf("validation field = %q, want %q", validationErr.Field, tt.wantField)
			}
			if got := attempts.Load(); got != 0 {
				t.Fatalf("explicit invalid gateway contract reached create POST %d times, want 0", got)
			}
		})
	}
}

func TestCreateTaskRejectsUndeclaredSmartDurationBeforePost(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-1"},
		})
	}))
	defer server.Close()

	contract := configuredMapperContract()
	contract.Duration.ValueType = "string"
	contract.Duration.ValueMap = nil
	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: contract,
	}))
	_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
		Model:       "video-ds-2.0",
		Prompt:      "test",
		Duration:    -1,
		AspectRatio: "9:16",
		TaskMode:    "reference",
	}, CanonicalReferences{})
	validationErr := assertValidationCode(t, err, "gateway_value_not_declared")
	if validationErr.Field != "duration" {
		t.Fatalf("validation field = %q, want duration", validationErr.Field)
	}
	if got := attempts.Load(); got != 0 {
		t.Fatalf("undeclared smart duration reached create POST %d times, want 0", got)
	}
}

func TestCreateTaskValidatesTaskModeBeforePost(t *testing.T) {
	t.Run("unknown mode", func(t *testing.T) {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "queued", "task_id": "task-1"},
			})
		}))
		defer server.Close()

		client := allowLocalTestClient(NewClient(config.VideoConfig{
			APIBase:         server.URL,
			APIKey:          "test-key",
			GatewayContract: configuredMapperContract(),
		}))
		_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
			Model:       "video-ds-2.0",
			Prompt:      "test",
			Duration:    12,
			AspectRatio: "9:16",
			TaskMode:    "future-mode",
		}, CanonicalReferences{})
		validationErr := assertValidationCode(t, err, "task_mode_unsupported")
		if validationErr.Field != "taskMode" {
			t.Fatalf("validation field = %q, want taskMode", validationErr.Field)
		}
		if got := attempts.Load(); got != 0 {
			t.Fatalf("unknown task mode reached create POST %d times, want 0", got)
		}
	})

	t.Run("known but undeclared mode", func(t *testing.T) {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "queued", "task_id": "task-1"},
			})
		}))
		defer server.Close()

		contract := configuredMapperContract()
		contract.DeclaredModes = []string{"reference"}
		client := allowLocalTestClient(NewClient(config.VideoConfig{
			APIBase:         server.URL,
			APIKey:          "test-key",
			GatewayContract: contract,
		}))
		canonical := mustCanonicalReferences(t, []Reference{
			{ID: "1", Kind: "video", Role: "edit_target", URL: "v1"},
		})
		_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
			Model:       "video-ds-2.0",
			Prompt:      "test",
			Duration:    12,
			AspectRatio: "9:16",
			TaskMode:    "edit",
		}, canonical)
		validationErr := assertValidationCode(t, err, "gateway_task_mode_not_declared")
		if validationErr.Field != "taskMode" {
			t.Fatalf("validation field = %q, want taskMode", validationErr.Field)
		}
		if got := attempts.Load(); got != 0 {
			t.Fatalf("undeclared task mode reached create POST %d times, want 0", got)
		}
	})

	t.Run("declared mode can be expressed by content role without field", func(t *testing.T) {
		var attempts atomic.Int32
		var gotBody map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"status": "queued", "task_id": "task-1"},
			})
		}))
		defer server.Close()

		contract := configuredMapperContract()
		contract.TaskMode = config.FieldEncoding{}
		client := allowLocalTestClient(NewClient(config.VideoConfig{
			APIBase:         server.URL,
			APIKey:          "test-key",
			GatewayContract: contract,
		}))
		canonical := mustCanonicalReferences(t, []Reference{
			{ID: "1", Kind: "video", Role: "edit_target", URL: "v1"},
		})
		result, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
			Model:       "video-ds-2.0",
			Prompt:      "test",
			Duration:    12,
			AspectRatio: "9:16",
			TaskMode:    "edit",
		}, canonical)
		if err != nil {
			t.Fatal(err)
		}
		if result.TaskID != "task-1" || attempts.Load() != 1 {
			t.Fatalf("result=%+v attempts=%d, want one accepted POST", result, attempts.Load())
		}
		if _, exists := gotBody["operation"]; exists {
			t.Fatalf("undeclared task mode field leaked into body: %#v", gotBody)
		}
		items, ok := gotBody["content_items"].([]any)
		if !ok || len(items) != 1 {
			t.Fatalf("content_items = %#v, want one role-encoded item", gotBody["content_items"])
		}
		item, ok := items[0].(map[string]any)
		if !ok || item["edit_target"] != true || item["video_url"] != "v1" {
			t.Fatalf("role-encoded item = %#v", items[0])
		}
	})
}

func TestCreateTaskValidatesOriginalGatewayContractBeforePost(t *testing.T) {
	tests := []struct {
		name       string
		contract   func() config.GatewayContractConfig
		references []Reference
		wantCode   string
		wantField  string
	}{
		{
			name: "missing name",
			contract: func() config.GatewayContractConfig {
				contract := LegacyFlatContract()
				contract.Name = ""
				return contract
			},
			wantCode:  "gateway_contract_invalid",
			wantField: "gatewayContract.name",
		},
		{
			name: "missing version",
			contract: func() config.GatewayContractConfig {
				contract := LegacyFlatContract()
				contract.Version = ""
				return contract
			},
			wantCode:  "gateway_contract_invalid",
			wantField: "gatewayContract.version",
		},
		{
			name: "invalid field name",
			contract: func() config.GatewayContractConfig {
				contract := LegacyFlatContract()
				contract.Duration.Name = "payload[seconds]"
				return contract
			},
			wantCode:  "gateway_contract_invalid",
			wantField: "gatewayContract.duration.name",
		},
		{
			name: "reserved idempotency header",
			contract: func() config.GatewayContractConfig {
				contract := LegacyFlatContract()
				contract.Idempotency.Header = "Authorization"
				return contract
			},
			wantCode:  "gateway_contract_invalid",
			wantField: "gatewayContract.idempotency.header",
		},
		{
			name: "invalid reference role field",
			contract: func() config.GatewayContractConfig {
				contract := configuredMapperContract()
				contract.References.RoleFields["reference_image"] = "role[reference]"
				return contract
			},
			references: []Reference{{ID: "1", Kind: "image", Role: "reference_image", URL: "i1"}},
			wantCode:   "gateway_contract_invalid",
			wantField:  "gatewayContract.references.roleFields",
		},
		{
			name: "duplicate task mode encoding",
			contract: func() config.GatewayContractConfig {
				contract := configuredMapperContract()
				contract.TaskMode = config.FieldEncoding{
					Name:      "task_mode",
					ValueType: "int",
					ValueMap:  map[string]string{"reference": "1", "edit": "1", "extend": "2"},
				}
				return contract
			},
			wantCode:  "gateway_contract_invalid",
			wantField: "gatewayContract.taskMode",
		},
		{
			name: "unknown reference mode",
			contract: func() config.GatewayContractConfig {
				contract := LegacyFlatContract()
				contract.References.Mode = "future_items"
				contract.TaskMode = config.FieldEncoding{Name: "task_mode", ValueType: "string"}
				return contract
			},
			wantCode:  "gateway_contract_invalid",
			wantField: "gatewayContract.references.mode",
		},
		{
			name: "missing reference media field",
			contract: func() config.GatewayContractConfig {
				contract := configuredMapperContract()
				contract.References.VideoField = ""
				return contract
			},
			references: []Reference{{ID: "1", Kind: "video", Role: "reference_video", URL: "v1"}},
			wantCode:   "gateway_contract_invalid",
			wantField:  "gatewayContract.references.videoField",
		},
		{
			name: "zero contract",
			contract: func() config.GatewayContractConfig {
				return config.GatewayContractConfig{}
			},
			wantCode:  "gateway_contract_invalid",
			wantField: "gatewayContract",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"status": "queued", "task_id": "task-1"},
				})
			}))
			defer server.Close()

			client := NewClient(config.VideoConfig{
				APIBase:         server.URL,
				APIKey:          "test-key",
				GatewayContract: tt.contract(),
			})
			client.urlAllowed = func(string) bool { return true }
			client.client.Transport = &http.Transport{DisableKeepAlives: true}
			canonical := mustCanonicalReferences(t, tt.references)
			_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
				Model:       "video-ds-2.0-fast",
				Prompt:      "test",
				Duration:    15,
				AspectRatio: "9:16",
				TaskMode:    "reference",
				RequestKey:  "req-123",
			}, canonical)
			validationErr := assertValidationCode(t, err, tt.wantCode)
			if validationErr.Field != tt.wantField {
				t.Fatalf("validation field = %q, want %q", validationErr.Field, tt.wantField)
			}
			if got := attempts.Load(); got != 0 {
				t.Fatalf("invalid contract reached create POST %d times, want 0", got)
			}
		})
	}
}

func TestCreateTaskDoesNotRetryPost(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Fatal(err)
			}
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-1",
			},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	_, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, nil, 15, "9:16")
	var ambiguous *AmbiguousTransportError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("CreateTask() error type = %T, want *AmbiguousTransportError: %v", err, err)
	}
	if _, exposesCause := any(ambiguous).(interface{ Unwrap() error }); exposesCause {
		t.Fatal("ambiguous transport error must not publicly unwrap its network cause")
	}
	if strings.Contains(err.Error(), server.URL) {
		t.Fatalf("ambiguous error exposed gateway URL: %q", err.Error())
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("create POST attempts = %d, want exactly 1", got)
	}
}

func TestCreateTaskClassifiesUnconfirmed2xxOutcomes(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		ambiguous bool
		wantTask  string
	}{
		{name: "truncated json", response: `{"data":`, ambiguous: true},
		{name: "missing task id", response: `{"data":{"status":"queued"}}`, ambiguous: true},
		{name: "confirmed task id", response: `{"data":{"status":"queued","task_id":"task-1"}}`, wantTask: "task-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				if _, err := io.Copy(io.Discard, r.Body); err != nil {
					t.Fatal(err)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tt.response)
			}))
			defer server.Close()

			contract := LegacyFlatContract()
			contract.Idempotency.Header = "Idempotency-Key"
			client := allowLocalTestClient(NewClient(config.VideoConfig{
				APIBase:         server.URL,
				APIKey:          "api-secret",
				GatewayContract: contract,
			}))
			const requestKey = "123e4567-e89b-12d3-a456-426614174000"
			const prompt = "TOP SECRET PROMPT"
			result, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
				Model:       "video-ds-2.0-fast",
				Prompt:      prompt,
				Duration:    15,
				AspectRatio: "9:16",
				TaskMode:    "reference",
				RequestKey:  requestKey,
			}, CanonicalReferences{})

			if tt.ambiguous {
				var ambiguous *AmbiguousTransportError
				if !errors.As(err, &ambiguous) {
					t.Fatalf("CreateNormalizedTask() error type = %T, want *AmbiguousTransportError: %v", err, err)
				}
				if ambiguous.RequestKey != requestKey || ambiguous.Operation != "create_video_task" {
					t.Fatalf("ambiguous context = requestKey %q operation %q", ambiguous.RequestKey, ambiguous.Operation)
				}
				if _, exposesCause := any(ambiguous).(interface{ Unwrap() error }); exposesCause {
					t.Fatal("ambiguous outcome must not publicly unwrap its raw cause")
				}
				for _, secret := range []string{requestKey, prompt, "api-secret", server.URL} {
					if strings.Contains(err.Error(), secret) {
						t.Fatalf("ambiguous error exposed secret/context %q: %q", secret, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if result.TaskID != tt.wantTask {
					t.Fatalf("task id = %q, want %q", result.TaskID, tt.wantTask)
				}
			}
			if got := attempts.Load(); got != 1 {
				t.Fatalf("create POST attempts = %d, want exactly 1", got)
			}
		})
	}
}

func TestCreateTaskTreatsOversized2xxBodyAsAmbiguous(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"task_id":"task-1"}`)
		_, _ = io.WriteString(w, strings.Repeat(" ", 4*1024*1024))
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
		Model:       "video-ds-2.0-fast",
		Prompt:      "test",
		Duration:    15,
		AspectRatio: "9:16",
		TaskMode:    "reference",
	}, CanonicalReferences{})
	var ambiguous *AmbiguousTransportError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("oversized 2xx error type = %T, want *AmbiguousTransportError: %v", err, err)
	}
	if ambiguous.StatusCode != http.StatusOK || attempts.Load() != 1 {
		t.Fatalf("ambiguous status=%d attempts=%d", ambiguous.StatusCode, attempts.Load())
	}
}

func TestCreateTaskTreats2xxBodyReadFailureAsAmbiguous(t *testing.T) {
	client := NewClient(config.VideoConfig{
		APIBase:         "https://gateway.example.com",
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	})
	client.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if trace := httptrace.ContextClientTrace(request.Context()); trace != nil && trace.WroteHeaders != nil {
			trace.WroteHeaders()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       &failingCreateResponseBody{},
			Request:    request,
		}, nil
	})

	_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
		Model:       "video-ds-2.0-fast",
		Prompt:      "test",
		Duration:    15,
		AspectRatio: "9:16",
		TaskMode:    "reference",
	}, CanonicalReferences{})
	var ambiguous *AmbiguousTransportError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("2xx read failure type = %T, want *AmbiguousTransportError: %v", err, err)
	}
	if ambiguous.StatusCode != http.StatusOK {
		t.Fatalf("ambiguous status = %d, want 200", ambiguous.StatusCode)
	}
}

type failingCreateResponseBody struct {
	sent bool
}

func (b *failingCreateResponseBody) Read(buffer []byte) (int, error) {
	if !b.sent {
		b.sent = true
		return copy(buffer, []byte(`{"data":`)), nil
	}
	return 0, io.ErrUnexpectedEOF
}

func (b *failingCreateResponseBody) Close() error {
	return nil
}

func TestCreateTaskHTTPStatusMatrix(t *testing.T) {
	definiteStatuses := []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusMethodNotAllowed,
		http.StatusNotAcceptable,
		http.StatusGone,
		http.StatusLengthRequired,
		http.StatusPreconditionFailed,
		http.StatusRequestEntityTooLarge,
		http.StatusRequestURITooLong,
		http.StatusUnsupportedMediaType,
		http.StatusRequestedRangeNotSatisfiable,
		http.StatusExpectationFailed,
		http.StatusUnprocessableEntity,
	}
	ambiguousStatuses := []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
		http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusLocked,
		http.StatusFailedDependency,
		http.StatusTooEarly,
		http.StatusPreconditionRequired,
		http.StatusTooManyRequests,
		http.StatusRequestHeaderFieldsTooLarge,
		http.StatusUnavailableForLegalReasons,
		499,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
		418,
	}

	tests := make([]struct {
		status    int
		ambiguous bool
	}, 0, len(definiteStatuses)+len(ambiguousStatuses))
	for _, status := range definiteStatuses {
		tests = append(tests, struct {
			status    int
			ambiguous bool
		}{status: status})
	}
	for _, status := range ambiguousStatuses {
		tests = append(tests, struct {
			status    int
			ambiguous bool
		}{status: status, ambiguous: true})
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status)+"_"+strconv.Itoa(tt.status), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `SECRET UPSTREAM BODY: api-secret TOP SECRET PROMPT https://asset.example/private req-123`)
			}))
			defer server.Close()

			contract := LegacyFlatContract()
			contract.Idempotency.Header = "Idempotency-Key"
			client := allowLocalTestClient(NewClient(config.VideoConfig{
				APIBase:         server.URL,
				APIKey:          "api-secret",
				GatewayContract: contract,
			}))
			const requestKey = "123e4567-e89b-12d3-a456-426614174000"
			_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
				Model:       "video-ds-2.0-fast",
				Prompt:      "TOP SECRET PROMPT",
				Duration:    15,
				AspectRatio: "9:16",
				TaskMode:    "reference",
				RequestKey:  requestKey,
			}, CanonicalReferences{})

			if tt.ambiguous {
				var ambiguous *AmbiguousTransportError
				if !errors.As(err, &ambiguous) {
					t.Fatalf("HTTP %d error type = %T, want *AmbiguousTransportError: %v", tt.status, err, err)
				}
				if ambiguous.StatusCode != tt.status || ambiguous.RequestKey != requestKey {
					t.Fatalf("ambiguous context = status %d requestKey %q", ambiguous.StatusCode, ambiguous.RequestKey)
				}
				if _, exposesCause := any(ambiguous).(interface{ Unwrap() error }); exposesCause {
					t.Fatal("AmbiguousTransportError must not publicly unwrap its raw cause")
				}
			} else {
				var rejected *CreateTaskError
				if !errors.As(err, &rejected) {
					t.Fatalf("HTTP %d error type = %T, want *CreateTaskError: %v", tt.status, err, err)
				}
				if rejected.Code != "gateway_request_rejected" || rejected.StatusCode != tt.status || rejected.Message == "" {
					t.Fatalf("definite rejection = %+v", rejected)
				}
				if _, exposesCause := any(rejected).(interface{ Unwrap() error }); exposesCause {
					t.Fatal("CreateTaskError must not publicly unwrap its raw cause")
				}
			}
			for _, secret := range []string{"SECRET UPSTREAM BODY", "api-secret", "TOP SECRET PROMPT", "https://asset.example/private", requestKey, server.URL} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("HTTP %d error exposed %q: %q", tt.status, secret, err.Error())
				}
			}
			if got := attempts.Load(); got != 1 {
				t.Fatalf("HTTP %d create POST attempts = %d, want 1", tt.status, got)
			}
		})
	}
}

func TestGenerateStoresUnknownOutcome(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	state := &videoDBState{}
	db := openVideoTestDB(t, state)
	defer db.Close()

	store := allowLocalTestStore(NewStore(db, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	input := GenerateInput{Prompt: "test", RequestKey: submissionKeyOne}
	_, err := store.Generate(context.Background(), input)
	var unknown *UnknownOutcomeError
	if !errors.As(err, &unknown) {
		t.Fatalf("Generate() error type = %T, want *UnknownOutcomeError: %v", err, err)
	}
	if unknown.RequestKey != submissionKeyOne || unknown.SubmissionID == "" {
		t.Fatalf("unknown outcome identity = %+v", unknown)
	}
	if !unknown.Persisted {
		t.Fatal("unknown outcome should report persisted recovery state")
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("create POST attempts = %d, want 1", got)
	}
	if state.insertCalls != 0 {
		t.Fatalf("ambiguous HTTP outcome wrote %d failed generations, want 0", state.insertCalls)
	}
	submission, err := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if err != nil {
		t.Fatal(err)
	}
	if submission.Status != SubmissionUnknownOutcome || submission.GenerationID != "" {
		t.Fatalf("unknown outcome submission = %+v", submission)
	}

	_, err = store.Generate(context.Background(), input)
	if !errors.As(err, &unknown) {
		t.Fatalf("replay error type = %T, want *UnknownOutcomeError: %v", err, err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("replay create POST attempts = %d, want 1", got)
	}
}

func TestGenerateKeepsUnknownClassificationWhenStateUpdateFails(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	state.submissions.unknownTransitionFailures = 1
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	input := GenerateInput{Prompt: "test", RequestKey: submissionKeyOne}

	_, err := store.Generate(context.Background(), input)
	var unknown *UnknownOutcomeError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %T, want *UnknownOutcomeError: %v", err, err)
	}
	if unknown.RequestKey != submissionKeyOne || unknown.SubmissionID == "" || unknown.Persisted {
		t.Fatalf("unknown outcome identity = %+v", unknown)
	}
	submission, getErr := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if submission.Status != SubmissionSubmitting {
		t.Fatalf("submission status = %q, want submitting after injected persistence failure", submission.Status)
	}

	_, err = store.Generate(context.Background(), input)
	var inProgress *SubmissionInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("replay error = %T, want *SubmissionInProgressError: %v", err, err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGeneratePersistsUnknownAfterRequestContextCancellation(t *testing.T) {
	var attempts atomic.Int32
	received := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			close(received)
		}
		<-release
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "too-late"},
		})
	}))
	defer server.Close()
	defer close(release)

	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
		TimeoutSeconds:  5,
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()

	_, err := store.Generate(ctx, GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	var unknown *UnknownOutcomeError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %T, want *UnknownOutcomeError: %v", err, err)
	}
	if !unknown.Persisted {
		t.Fatal("request cancellation must not cancel unknown-outcome persistence")
	}
	select {
	case <-received:
	default:
		t.Fatal("request context expired before the create request reached the gateway")
	}
	submission, getErr := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if submission.Status != SubmissionUnknownOutcome {
		t.Fatalf("submission status = %q, want unknown_outcome", submission.Status)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGenerateReturnsTaskIDWhenLocalLinkFails(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"status":  "queued",
				"task_id": "task-42",
			},
		})
	}))
	defer server.Close()

	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	state.submissions.recordTaskFailures = 1
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "private-api-key",
		GatewayContract: LegacyFlatContract(),
	}))
	input := GenerateInput{
		Prompt:     "private prompt",
		Images:     []string{"https://asset.example/private-image"},
		RequestKey: submissionKeyOne,
	}

	_, err := store.Generate(context.Background(), input)
	var linkage *LocalLinkageError
	if !errors.As(err, &linkage) {
		t.Fatalf("error = %T, want *LocalLinkageError: %v", err, err)
	}
	if linkage.RequestKey != submissionKeyOne || linkage.SubmissionID == "" || linkage.TaskID != "task-42" {
		t.Fatalf("local linkage identity = %+v", linkage)
	}
	for _, secret := range []string{"private-api-key", "private prompt", "private-image"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("local linkage error exposed %q: %q", secret, err.Error())
		}
	}
	if attempts.Load() != 1 || state.insertCalls != 0 {
		t.Fatalf("attempts=%d generation inserts=%d, want 1/0", attempts.Load(), state.insertCalls)
	}
	submission, getErr := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if submission.Status != SubmissionSubmitting || submission.GenerationID != "" {
		t.Fatalf("reconcilable submission = %+v", submission)
	}

	_, err = store.Generate(context.Background(), input)
	var inProgress *SubmissionInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("replay error = %T, want *SubmissionInProgressError: %v", err, err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("replay create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestGenerateRecoversWhenAcceptedGenerationReloadFails(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-reload"},
		})
	}))
	defer server.Close()

	state := &videoDBState{generationQueryFailures: 1}
	database := openVideoTestDB(t, state)
	defer database.Close()
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	input := GenerateInput{Prompt: "test", RequestKey: submissionKeyOne}

	_, err := store.Generate(context.Background(), input)
	var linkage *LocalLinkageError
	if !errors.As(err, &linkage) {
		t.Fatalf("error = %T, want *LocalLinkageError: %v", err, err)
	}
	if linkage.RequestKey != submissionKeyOne || linkage.SubmissionID == "" || linkage.TaskID != "task-reload" {
		t.Fatalf("local linkage identity = %+v", linkage)
	}
	submission, getErr := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if submission.Status != SubmissionAccepted || submission.GenerationID == "" {
		t.Fatalf("accepted submission = %+v", submission)
	}

	recovered, err := store.Generate(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.ID != submission.GenerationID || recovered.TaskID != "task-reload" {
		t.Fatalf("recovered generation = %+v", recovered)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func TestReconcileSubmissionSameTaskIsIdempotent(t *testing.T) {
	store, state, attempts := newUnknownOutcomeStore(t)
	ctx := context.Background()

	first, err := store.ReconcileSubmission(ctx, submissionKeyOne, "task-42")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.ReconcileSubmission(ctx, submissionKeyOne, "task-42")
	if err != nil {
		t.Fatal(err)
	}
	if first.Submission.Status != SubmissionReconciled || second.Submission.Status != SubmissionReconciled {
		t.Fatalf("reconciled statuses = %q/%q", first.Submission.Status, second.Submission.Status)
	}
	if first.Submission.ID != second.Submission.ID || first.Generation.ID == "" || first.Generation.ID != second.Generation.ID {
		t.Fatalf("reconcile identities = first %+v second %+v", first, second)
	}
	if first.Generation.TaskID != "task-42" || first.Generation.SubmissionStatus != SubmissionReconciled {
		t.Fatalf("reconciled generation = %+v", first.Generation)
	}
	if state.insertCalls != 1 {
		t.Fatalf("generation inserts = %d, want 1", state.insertCalls)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts after reconciliation = %d, want 1", attempts.Load())
	}
}

func TestReconcileSubmissionRejectsDifferentTask(t *testing.T) {
	store, state, attempts := newUnknownOutcomeStore(t)
	ctx := context.Background()

	if _, err := store.ReconcileSubmission(ctx, submissionKeyOne, "task-42"); err != nil {
		t.Fatal(err)
	}
	_, err := store.ReconcileSubmission(ctx, submissionKeyOne, "task-other")
	var conflict *ReconciliationTaskConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T, want *ReconciliationTaskConflictError: %v", err, err)
	}
	if conflict.Code != "reconciliation_task_conflict" {
		t.Fatalf("code = %q", conflict.Code)
	}
	if state.insertCalls != 1 || attempts.Load() != 1 {
		t.Fatalf("generation inserts=%d create POST attempts=%d, want 1/1", state.insertCalls, attempts.Load())
	}
}

func TestReconcileSubmissionFromSubmitting(t *testing.T) {
	store, state, attempts := newLocalLinkFailureStore(t, 1, 0)

	result, err := store.ReconcileSubmission(context.Background(), submissionKeyOne, "task-42")
	if err != nil {
		t.Fatal(err)
	}
	if result.Submission.Status != SubmissionReconciled || result.Submission.UpstreamTaskID != "task-42" {
		t.Fatalf("submission = %+v", result.Submission)
	}
	if result.Generation.ID == "" || result.Generation.TaskID != "task-42" {
		t.Fatalf("generation = %+v", result.Generation)
	}
	if state.insertCalls != 1 || attempts.Load() != 1 {
		t.Fatalf("generation inserts=%d create POST attempts=%d, want 1/1", state.insertCalls, attempts.Load())
	}
}

func TestReconcileSubmissionReusesExistingGeneration(t *testing.T) {
	store, state, attempts := newLocalLinkFailureStore(t, 0, 1)
	if state.insertCalls != 1 {
		t.Fatalf("precondition generation inserts = %d, want 1", state.insertCalls)
	}
	existingID := state.generation.ID

	result, err := store.ReconcileSubmission(context.Background(), submissionKeyOne, "task-42")
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.ID != existingID {
		t.Fatalf("generation id = %q, want existing %q", result.Generation.ID, existingID)
	}
	if state.insertCalls != 1 || attempts.Load() != 1 {
		t.Fatalf("generation inserts=%d create POST attempts=%d, want 1/1", state.insertCalls, attempts.Load())
	}
}

func newUnknownOutcomeStore(t *testing.T) (*Store, *videoDBState, *atomic.Int32) {
	t.Helper()
	attempts := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)
	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	_, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	var unknown *UnknownOutcomeError
	if !errors.As(err, &unknown) {
		t.Fatalf("setup error = %T, want *UnknownOutcomeError: %v", err, err)
	}
	return store, state, attempts
}

func newLocalLinkFailureStore(t *testing.T, recordTaskFailures, acceptTransitionFailures int) (*Store, *videoDBState, *atomic.Int32) {
	t.Helper()
	attempts := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-42"},
		})
	}))
	t.Cleanup(server.Close)
	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	state.submissions.recordTaskFailures = recordTaskFailures
	state.submissions.acceptTransitionFailures = acceptTransitionFailures
	store := allowLocalTestStore(NewStore(database, nil, config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	_, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	var linkage *LocalLinkageError
	if !errors.As(err, &linkage) {
		t.Fatalf("setup error = %T, want *LocalLinkageError: %v", err, err)
	}
	return store, state, attempts
}

func TestCreateTaskDoesNotFollowRedirect(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var sourceAttempts atomic.Int32
			var targetAttempts atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetAttempts.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"status": "queued", "task_id": "redirected-task"},
				})
			}))
			defer target.Close()

			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sourceAttempts.Add(1)
				if r.Header.Get("Authorization") != "Bearer api-secret" || r.Header.Get("Idempotency-Key") != "123e4567-e89b-12d3-a456-426614174000" {
					t.Errorf("source headers = %#v", r.Header)
				}
				http.Redirect(w, r, target.URL+"/capture", status)
			}))
			defer source.Close()

			contract := LegacyFlatContract()
			contract.Idempotency.Header = "Idempotency-Key"
			client := allowLocalTestClient(NewClient(config.VideoConfig{
				APIBase:         source.URL,
				APIKey:          "api-secret",
				GatewayContract: contract,
			}))
			_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
				Model:       "video-ds-2.0-fast",
				Prompt:      "test",
				Duration:    15,
				AspectRatio: "9:16",
				TaskMode:    "reference",
				RequestKey:  "123e4567-e89b-12d3-a456-426614174000",
			}, CanonicalReferences{})
			var ambiguous *AmbiguousTransportError
			if !errors.As(err, &ambiguous) {
				t.Fatalf("redirect error type = %T, want *AmbiguousTransportError: %v", err, err)
			}
			if ambiguous.StatusCode != status {
				t.Fatalf("ambiguous status = %d, want %d", ambiguous.StatusCode, status)
			}
			if sourceAttempts.Load() != 1 || targetAttempts.Load() != 0 {
				t.Fatalf("source attempts=%d target attempts=%d, want 1/0", sourceAttempts.Load(), targetAttempts.Load())
			}
		})
	}
}

func TestQueryTaskStillFollowsAllowedRedirect(t *testing.T) {
	var targetAttempts atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetAttempts.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-1"},
		})
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/task", http.StatusFound)
	}))
	defer source.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: source.URL,
		APIKey:  "test-key",
	}))
	result, err := client.QueryTask(context.Background(), "task-1", 15)
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "task-1" || targetAttempts.Load() != 1 {
		t.Fatalf("result=%+v target attempts=%d", result, targetAttempts.Load())
	}
}

func TestCreateTaskClassifiesPreWriteFailuresAsNotSubmitted(t *testing.T) {
	t.Run("pre-canceled context", func(t *testing.T) {
		var attempts atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
		}))
		defer server.Close()
		client := allowLocalTestClient(NewClient(config.VideoConfig{
			APIBase:         server.URL,
			APIKey:          "api-secret",
			GatewayContract: LegacyFlatContract(),
		}))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := createTestTaskError(client, ctx)
		assertRequestNotSubmitted(t, err)
		if attempts.Load() != 0 {
			t.Fatalf("pre-canceled request reached server %d times", attempts.Load())
		}
	})

	t.Run("dns failure", func(t *testing.T) {
		client := NewClient(config.VideoConfig{
			APIBase:         "https://secret.gateway.invalid",
			APIKey:          "api-secret",
			GatewayContract: LegacyFlatContract(),
		})
		client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.DNSError{Err: "no such host", Name: "secret.gateway.invalid"}
		})
		assertRequestNotSubmitted(t, createTestTaskError(client, context.Background()))
	})

	t.Run("connection refused", func(t *testing.T) {
		client := NewClient(config.VideoConfig{
			APIBase:         "http://127.0.0.1:1",
			APIKey:          "api-secret",
			GatewayContract: LegacyFlatContract(),
		})
		client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		})
		assertRequestNotSubmitted(t, createTestTaskError(client, context.Background()))
	})

	t.Run("tls handshake failure", func(t *testing.T) {
		var attempts atomic.Int32
		server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts.Add(1)
		}))
		server.Config.ErrorLog = log.New(io.Discard, "", 0)
		server.StartTLS()
		defer server.Close()

		client := NewClient(config.VideoConfig{
			APIBase:         server.URL,
			APIKey:          "api-secret",
			GatewayContract: LegacyFlatContract(),
		})
		client.client.Transport = &http.Transport{DisableKeepAlives: true}
		assertRequestNotSubmitted(t, createTestTaskError(client, context.Background()))
		if attempts.Load() != 0 {
			t.Fatalf("TLS handshake failure reached handler %d times", attempts.Load())
		}
	})
}

func TestCreateTaskRejectsUnsafeAPIBaseBeforePost(t *testing.T) {
	tests := []struct {
		name    string
		apiBase string
		secrets []string
	}{
		{
			name:    "parse failure",
			apiBase: "https://gateway.example.com/%zz/private-token",
			secrets: []string{"%zz", "private-token"},
		},
		{
			name:    "credentials",
			apiBase: "https://private-user:super-secret@gateway.example.com/base",
			secrets: []string{"private-user", "super-secret"},
		},
		{
			name:    "path traversal",
			apiBase: "https://gateway.example.com/safe/../private-path",
			secrets: []string{"private-path"},
		},
		{
			name:    "encoded path traversal",
			apiBase: "https://gateway.example.com/safe/%2e%2e/private-path",
			secrets: []string{"private-path"},
		},
		{
			name:    "query",
			apiBase: "https://gateway.example.com/base?token=private-query",
			secrets: []string{"private-query"},
		},
		{
			name:    "fragment",
			apiBase: "https://gateway.example.com/base#private-fragment",
			secrets: []string{"private-fragment"},
		},
		{
			name:    "non HTTP scheme",
			apiBase: "ftp://gateway.example.com/private-path",
			secrets: []string{"private-path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attempts atomic.Int32
			client := NewClient(config.VideoConfig{
				APIBase:         tt.apiBase,
				APIKey:          "api-secret",
				GatewayContract: LegacyFlatContract(),
			})
			client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
				attempts.Add(1)
				return nil, errors.New("unexpected transport call")
			})

			err := createTestTaskError(client, context.Background())
			definite := assertRequestNotSubmitted(t, err)
			if _, exposesCause := any(definite).(interface{ Unwrap() error }); exposesCause {
				t.Fatal("request-not-submitted error must not publicly unwrap its raw cause")
			}
			for _, secret := range append([]string{tt.apiBase}, tt.secrets...) {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("unsafe API base error exposed %q: %q", secret, err.Error())
				}
			}
			if got := attempts.Load(); got != 0 {
				t.Fatalf("unsafe API base reached RoundTrip %d times, want 0", got)
			}
		})
	}
}

func TestGeneratePersistsOnlySafeAPIBaseFailureMessage(t *testing.T) {
	state := &videoDBState{}
	db := openVideoTestDB(t, state)
	defer db.Close()
	const apiBase = "https://private-user:super-secret@gateway.example.com/private-path"
	store := NewStore(db, nil, config.VideoConfig{
		APIBase:         apiBase,
		APIKey:          "api-secret",
		GatewayContract: LegacyFlatContract(),
		Mode:            config.VideoGenerationModePaid,
	})
	var attempts atomic.Int32
	store.client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, errors.New("unexpected transport call")
	})

	_, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", RequestKey: submissionKeyOne})
	definite := assertRequestNotSubmitted(t, err)
	if got := attempts.Load(); got != 0 {
		t.Fatalf("unsafe API base reached RoundTrip %d times, want 0", got)
	}
	if state.insertCalls != 0 {
		t.Fatalf("pre-submit failure wrote %d generation rows, want 0", state.insertCalls)
	}
	submission, getErr := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if submission.Status != SubmissionCancelled {
		t.Fatalf("submission status = %q, want cancelled", submission.Status)
	}
	if submission.ErrorMessage != definite.Message {
		t.Fatalf("stored error message = %q, want safe message %q", submission.ErrorMessage, definite.Message)
	}
	for _, secret := range []string{apiBase, "private-user", "super-secret", "private-path", "api-secret"} {
		if strings.Contains(submission.ErrorMessage, secret) {
			t.Fatalf("stored error message exposed %q: %q", secret, submission.ErrorMessage)
		}
	}
}

func TestGenerateReportsTerminalStatePersistenceFailure(t *testing.T) {
	state := &videoDBState{}
	database := openVideoTestDB(t, state)
	defer database.Close()
	state.submissions.cancelTransitionFailures = 1
	store := NewStore(database, nil, config.VideoConfig{
		APIBase:         "https://private-user:super-secret@gateway.example/private-path",
		APIKey:          "private-api-key",
		GatewayContract: LegacyFlatContract(),
		Mode:            config.VideoGenerationModePaid,
	})
	var attempts atomic.Int32
	store.client.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return nil, errors.New("unexpected transport call")
	})
	input := GenerateInput{Prompt: "private prompt", RequestKey: submissionKeyOne}

	_, err := store.Generate(context.Background(), input)
	var persistence *SubmissionPersistenceError
	if !errors.As(err, &persistence) {
		t.Fatalf("error = %T, want *SubmissionPersistenceError: %v", err, err)
	}
	if persistence.RequestKey != submissionKeyOne || persistence.SubmissionID == "" || persistence.IntendedStatus != SubmissionCancelled {
		t.Fatalf("persistence error identity = %+v", persistence)
	}
	if persistence.OutcomeCode != "request_not_submitted" {
		t.Fatalf("outcome code = %q", persistence.OutcomeCode)
	}
	for _, secret := range []string{"private-user", "super-secret", "private-path", "private-api-key", "private prompt"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("persistence error exposed %q: %q", secret, err.Error())
		}
	}
	submission, getErr := store.submissions.GetByRequestKey(context.Background(), submissionKeyOne)
	if getErr != nil {
		t.Fatal(getErr)
	}
	if submission.Status != SubmissionSubmitting {
		t.Fatalf("submission status = %q, want submitting after injected terminal persistence failure", submission.Status)
	}
	_, err = store.Generate(context.Background(), input)
	var inProgress *SubmissionInProgressError
	if !errors.As(err, &inProgress) {
		t.Fatalf("replay error = %T, want *SubmissionInProgressError: %v", err, err)
	}
	if attempts.Load() != 0 {
		t.Fatalf("transport attempts = %d, want 0", attempts.Load())
	}
}

func TestCreateTaskTreatsResponseWaitTimeoutAsAmbiguous(t *testing.T) {
	var attempts atomic.Int32
	bodyRead := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		close(bodyRead)
		<-release
	}))
	defer server.Close()
	defer close(release)

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase:         server.URL,
		APIKey:          "test-key",
		GatewayContract: LegacyFlatContract(),
	}))
	client.client.Timeout = 100 * time.Millisecond
	err := createTestTaskError(client, context.Background())
	select {
	case <-bodyRead:
	default:
		t.Fatal("server did not read create request body before timeout")
	}
	var ambiguous *AmbiguousTransportError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("response wait timeout type = %T, want *AmbiguousTransportError: %v", err, err)
	}
	if attempts.Load() != 1 {
		t.Fatalf("create POST attempts = %d, want 1", attempts.Load())
	}
}

func createTestTaskError(client *Client, ctx context.Context) error {
	_, err := client.CreateNormalizedTask(ctx, GenerateRequest{
		Model:       "video-ds-2.0-fast",
		Prompt:      "TOP SECRET PROMPT",
		Duration:    15,
		AspectRatio: "9:16",
		TaskMode:    "reference",
	}, CanonicalReferences{})
	return err
}

func assertRequestNotSubmitted(t *testing.T, err error) *CreateTaskError {
	t.Helper()
	var definite *CreateTaskError
	if !errors.As(err, &definite) {
		t.Fatalf("error type = %T, want *CreateTaskError: %v", err, err)
	}
	if definite.Code != "request_not_submitted" || definite.StatusCode != 0 || definite.Message == "" {
		t.Fatalf("request-not-submitted error = %+v", definite)
	}
	var ambiguous *AmbiguousTransportError
	if errors.As(err, &ambiguous) {
		t.Fatalf("pre-write failure was ambiguous: %+v", ambiguous)
	}
	for _, secret := range []string{"secret.gateway.invalid", "127.0.0.1:1", "api-secret", "TOP SECRET PROMPT"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("request-not-submitted error exposed %q: %q", secret, err.Error())
		}
	}
	return definite
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestCreateTaskDisablesTransportBodyReplay(t *testing.T) {
	contract := LegacyFlatContract()
	contract.Idempotency.Header = "Idempotency-Key"
	client := NewClient(config.VideoConfig{
		APIBase:         "https://gateway.example.com",
		APIKey:          "test-key",
		GatewayContract: contract,
	})
	probe := &createPostReplayProbe{}
	client.client.Transport = probe

	_, err := client.CreateNormalizedTask(context.Background(), GenerateRequest{
		Model:       "video-ds-2.0-fast",
		Prompt:      "test",
		Duration:    15,
		AspectRatio: "9:16",
		TaskMode:    "reference",
		RequestKey:  "123e4567-e89b-12d3-a456-426614174000",
	}, CanonicalReferences{})
	var ambiguous *AmbiguousTransportError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("CreateNormalizedTask() error type = %T, want *AmbiguousTransportError: %v", err, err)
	}
	if probe.attempts != 1 {
		t.Fatalf("transport attempts = %d, want 1", probe.attempts)
	}
	if probe.getBodyPresent {
		t.Fatal("create POST exposed GetBody, allowing net/http transport replay")
	}
	if probe.idempotencyKey != "123e4567-e89b-12d3-a456-426614174000" {
		t.Fatalf("Idempotency-Key = %q, want UUID request key", probe.idempotencyKey)
	}
}

func TestQueryTaskStillRetriesSafeGet(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attempts.Add(1)
		if attempt == 1 {
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("test server does not support hijacking")
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"status": "queued", "task_id": "task-1"},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	result, err := client.QueryTask(context.Background(), "task-1", 15)
	if err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); result.TaskID != "task-1" || got != 2 {
		t.Fatalf("safe GET result=%+v attempts=%d, want task-1 after 2 attempts", result, got)
	}
}

type createPostReplayProbe struct {
	attempts       int
	getBodyPresent bool
	idempotencyKey string
}

func (p *createPostReplayProbe) RoundTrip(request *http.Request) (*http.Response, error) {
	p.attempts++
	p.getBodyPresent = request.GetBody != nil
	p.idempotencyKey = request.Header.Get("Idempotency-Key")
	if trace := httptrace.ContextClientTrace(request.Context()); trace != nil && trace.WroteHeaders != nil {
		trace.WroteHeaders()
	}
	return nil, &net.OpError{Op: "write", Net: "tcp", Err: io.EOF}
}

func TestCreateTaskDoesNotExposeRejectedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "invalid image_url: image url returned 403",
				"type":    "invalid_request_error",
			},
		})
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	_, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", []string{"https://example.com/private.png"}, nil, nil, 15, "16:9")
	var rejected *CreateTaskError
	if !errors.As(err, &rejected) {
		t.Fatalf("gateway error type = %T, want *CreateTaskError: %v", err, err)
	}
	if rejected.Code != "gateway_request_rejected" || rejected.StatusCode != http.StatusBadRequest {
		t.Fatalf("rejected error = %+v", rejected)
	}
	for _, secret := range []string{"invalid image_url", "https://example.com/private.png", "test-key", `{"error"`} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("rejected error exposed %q: %q", secret, err.Error())
		}
	}
}

func TestDownloadRejectsPrivateOrLocalURL(t *testing.T) {
	client := NewClient(config.VideoConfig{APIBase: "https://video.example.com", APIKey: "test-key"})
	_, _, err := client.Download(context.Background(), "http://127.0.0.1:8080/private.mp4")
	if err == nil {
		t.Fatal("expected local download URL to be rejected")
	}
	if !strings.Contains(err.Error(), "公网") {
		t.Fatalf("expected actionable public URL error, got %v", err)
	}
}

func TestIsRetryableNetworkError(t *testing.T) {
	if !isRetryableNetworkError(&net.OpError{Op: "read", Err: io.EOF}) {
		t.Fatal("expected EOF network error to be retryable")
	}
}

func TestDownloadTaskContentUsesContentEndpoint(t *testing.T) {
	var gotPath string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("mp4-data"))
	}))
	defer server.Close()

	client := allowLocalTestClient(NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	}))
	data, contentType, err := client.DownloadTaskContent(context.Background(), "task-1")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/videos/task-1/content" {
		t.Fatalf("expected content path /v1/videos/task-1/content, got %q", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("expected bearer auth, got %q", gotAuth)
	}
	if string(data) != "mp4-data" || contentType != "video/mp4" {
		t.Fatalf("unexpected content %q %q", string(data), contentType)
	}
}

func TestParseTaskTreatsProgressCompleteWithVideoURLAsCompleted(t *testing.T) {
	task := parseTask(map[string]any{
		"progress":  float64(100),
		"video_url": "http://example.com/result.mp4",
	})
	if task.Status != "completed" {
		t.Fatalf("expected completed, got %q", task.Status)
	}
	if task.URL != "http://example.com/result.mp4" {
		t.Fatalf("expected video url, got %q", task.URL)
	}
}

func TestParseTaskReadsSizeMetadata(t *testing.T) {
	task := parseTask(map[string]any{
		"duration": float64(15),
		"size":     "1280x720",
	})
	if task.Width != 1280 || task.Height != 720 {
		t.Fatalf("expected 1280x720, got %dx%d", task.Width, task.Height)
	}
}

func TestShouldSkipRefreshAllowsFailedTaskWithoutVideoToRetry(t *testing.T) {
	item := Generation{
		Status: "failed",
		TaskID: "task-1",
	}
	if shouldSkipRefresh(item) {
		t.Fatal("failed task with task_id and no video should be refreshed again")
	}
}

func TestShouldSkipRefreshStopsCompletedTaskWithVideo(t *testing.T) {
	item := Generation{
		Status:   "completed",
		TaskID:   "task-1",
		VideoURL: "/api/upload-assets/1",
	}
	if !shouldSkipRefresh(item) {
		t.Fatal("completed task with local video should not refresh again")
	}
}

func TestShouldSkipRefreshStopsRecordWithoutTask(t *testing.T) {
	item := Generation{
		Status: "failed",
	}
	if !shouldSkipRefresh(item) {
		t.Fatal("record without task_id should not refresh")
	}
}

func TestIsPublicHTTPURLRejectsPrivateAndLocalHosts(t *testing.T) {
	for _, raw := range []string{
		"http://localhost/a.mp4",
		"http://localhost./a.mp4",
		"http://foo.localhost/a.mp4",
		"http://127.0.0.1/a.mp4",
		"http://10.0.0.2/a.mp4",
		"http://172.16.0.2/a.mp4",
		"http://192.168.1.2/a.mp4",
		"http://169.254.1.2/a.mp4",
	} {
		if isPublicHTTPURL(raw) {
			t.Fatalf("expected %s to be rejected as non-public", raw)
		}
	}
	if !isPublicHTTPURL("https://cdn.example.com/a.mp4") {
		t.Fatal("expected public CDN URL to be accepted")
	}
}

func TestListGenerationsFiltersRecordsWithoutTaskID(t *testing.T) {
	condition, args := generationListCondition(nil)
	if condition != "task_id <> ''" {
		t.Fatal("ListGenerations should hide records without task_id from the generation history")
	}
	if len(args) != 0 {
		t.Fatalf("expected no args for default list condition, got %+v", args)
	}
}

func init() {
	sql.Register("video-test", videoTestDriver{})
}

type videoDBState struct {
	generation              Generation
	submissions             *submissionTestState
	generationQueryFailures int
	uploadAsset             map[string]driver.Value
	uploadAssetID           int64
	uploadCreateCalls       int
	insertCalls             int
	statusUpdateCalls       int
}

type recordingVideoUploader struct {
	data        []byte
	err         error
	url         string
	objectKey   string
	uploadCalls int
}

func (u *recordingVideoUploader) Upload(ctx context.Context, input storage.UploadInput) (storage.UploadResult, error) {
	u.uploadCalls++
	if u.err != nil {
		return storage.UploadResult{}, u.err
	}
	data, err := io.ReadAll(input.Reader)
	if err != nil {
		return storage.UploadResult{}, err
	}
	u.data = append([]byte(nil), data...)
	return storage.UploadResult{
		Key:       u.objectKey,
		URL:       u.url,
		ObjectKey: u.objectKey,
		ObjectURL: u.url,
		Name:      input.Filename,
	}, nil
}

type recordingDemoRenderer struct {
	calls  int
	err    error
	result DemoVideo
}

func (r *recordingDemoRenderer) Render(context.Context, DemoRenderInput) (DemoVideo, error) {
	r.calls++
	return r.result, r.err
}

type videoTestDriver struct{}

type videoTestConnector struct {
	state *videoDBState
}

type videoTestConn struct {
	state *videoDBState
}

type videoTestRows struct {
	columns []string
	values  []driver.Value
	read    bool
}

type videoTestResult int64

type videoTestTx struct{}

var (
	videoTestMu     sync.Mutex
	videoTestStates = map[string]*videoDBState{}
)

func openVideoTestDB(t *testing.T, state *videoDBState) *sql.DB {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	if state.submissions == nil {
		state.submissions = &submissionTestState{byKey: map[string]Submission{}}
	}
	videoTestMu.Lock()
	videoTestStates[name] = state
	videoTestMu.Unlock()
	t.Cleanup(func() {
		videoTestMu.Lock()
		delete(videoTestStates, name)
		videoTestMu.Unlock()
	})
	db, err := sql.Open("video-test", name)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func videoTestState(t *testing.T, db *sql.DB) *videoDBState {
	t.Helper()
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var state *videoDBState
	err = conn.Raw(func(raw any) error {
		c, ok := raw.(*videoTestConn)
		if !ok {
			return fmtError("unexpected raw connection")
		}
		state = c.state
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (videoTestDriver) Open(name string) (driver.Conn, error) {
	videoTestMu.Lock()
	defer videoTestMu.Unlock()
	state := videoTestStates[name]
	if state == nil {
		return nil, fmtError("missing video test db state")
	}
	return &videoTestConn{state: state}, nil
}

func (videoTestDriver) OpenConnector(name string) (driver.Connector, error) {
	videoTestMu.Lock()
	defer videoTestMu.Unlock()
	state := videoTestStates[name]
	if state == nil {
		return nil, fmtError("missing video test db state")
	}
	return videoTestConnector{state: state}, nil
}

func (c videoTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &videoTestConn{state: c.state}, nil
}

func (videoTestConnector) Driver() driver.Driver {
	return videoTestDriver{}
}

func (c *videoTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported")
}

func (c *videoTestConn) Close() error {
	return nil
}

func (c *videoTestConn) Begin() (driver.Tx, error) {
	return videoTestTx{}, nil
}

func (c *videoTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return videoTestTx{}, nil
}

func (videoTestTx) Commit() error   { return nil }
func (videoTestTx) Rollback() error { return nil }

func (c *videoTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q := strings.TrimSpace(query)
	if strings.Contains(q, "video_generation_submissions") {
		return (&submissionTestConn{state: c.state.submissions}).QueryContext(context.Background(), query, args)
	}
	switch {
	case strings.Contains(q, "SELECT nextval(pg_get_serial_sequence('upload_assets','id'))"):
		return singleRow([]string{"nextval"}, []driver.Value{int64(7)}), nil
	case strings.Contains(q, "INSERT INTO upload_assets"):
		c.state.uploadCreateCalls++
		c.state.uploadAsset = map[string]driver.Value{
			"id":           int64(7),
			"key":          namedString(args, 2),
			"name":         namedString(args, 3),
			"content_type": namedString(args, 5),
			"size":         namedInt64(args, 6),
			"data":         namedBytes(args, 7),
			"object_key":   namedString(args, 8),
			"object_url":   namedString(args, 9),
		}
		return singleRow([]string{"id", "key", "name", "content_type", "size", "data", "object_key", "object_url"}, []driver.Value{int64(7), namedString(args, 2), namedString(args, 3), namedString(args, 5), namedInt64(args, 6), namedBytes(args, 7), namedString(args, 8), namedString(args, 9)}), nil
	case strings.Contains(q, "FROM upload_assets WHERE id=$1"):
		if c.state.uploadAsset == nil {
			return nil, sql.ErrNoRows
		}
		return singleRow([]string{"id", "key", "name", "content_type", "size", "data", "object_key", "object_url"}, []driver.Value{c.state.uploadAsset["id"], c.state.uploadAsset["key"], c.state.uploadAsset["name"], c.state.uploadAsset["content_type"], c.state.uploadAsset["size"], c.state.uploadAsset["data"], c.state.uploadAsset["object_key"], c.state.uploadAsset["object_url"]}), nil
	case strings.Contains(q, "INSERT INTO video_generations"):
		c.state.insertCalls++
		if strings.Contains(q, "VALUES ('demo'") {
			c.state.generation = Generation{
				ID:           "42",
				Provider:     "demo",
				Model:        namedString(args, 1),
				Prompt:       namedString(args, 2),
				ImageURL:     namedString(args, 3),
				TaskID:       namedString(args, 7),
				Seconds:      int(namedInt64(args, 8)),
				AspectRatio:  namedString(args, 9),
				VideoAssetID: strconv.FormatInt(namedInt64(args, 10), 10),
				VideoURL:     namedString(args, 11),
				Duration:     namedFloat64(args, 12),
				FPS:          namedFloat64(args, 13),
				Width:        int(namedInt64(args, 14)),
				Height:       int(namedInt64(args, 15)),
				Status:       "completed",
			}
			return singleRow([]string{"id"}, []driver.Value{"42"}), nil
		}
		if strings.Contains(q, "used_images") {
			c.state.generation = Generation{
				ID:          "42",
				Provider:    "newapi",
				Model:       namedString(args, 1),
				Prompt:      namedString(args, 2),
				ImageURL:    namedString(args, 3),
				TaskID:      namedString(args, 7),
				Seconds:     int(namedInt64(args, 8)),
				AspectRatio: namedString(args, 9),
				Status:      namedString(args, 10),
			}
			return singleRow([]string{"id"}, []driver.Value{"42"}), nil
		}
		if strings.Contains(q, "'failed'") {
			c.state.generation = Generation{
				ID:           "42",
				Provider:     "newapi",
				Model:        namedString(args, 1),
				Prompt:       namedString(args, 2),
				ImageURL:     namedString(args, 3),
				Seconds:      int(namedInt64(args, 4)),
				AspectRatio:  namedString(args, 5),
				Status:       "failed",
				ErrorMessage: namedString(args, 6),
			}
			return singleRow([]string{"id"}, []driver.Value{"42"}), nil
		}
		c.state.generation = Generation{
			ID:          "42",
			Provider:    "newapi",
			Model:       namedString(args, 1),
			Prompt:      namedString(args, 2),
			ImageURL:    namedString(args, 3),
			TaskID:      namedString(args, 4),
			Seconds:     int(namedInt64(args, 5)),
			AspectRatio: namedString(args, 6),
			Status:      namedString(args, 7),
		}
		return singleRow([]string{"id"}, []driver.Value{"42"}), nil
	case strings.Contains(q, "FROM video_generations"):
		if c.state.generationQueryFailures > 0 {
			c.state.generationQueryFailures--
			return nil, errors.New("injected generation reload failure")
		}
		if c.state.generation.ID == "" {
			return nil, sql.ErrNoRows
		}
		return generationRow(c.state.generation), nil
	default:
		return nil, fmtError("unexpected query: " + q)
	}
}

func (c *videoTestConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	q := strings.TrimSpace(query)
	switch {
	case strings.Contains(q, "UPDATE video_generations SET status=$1"):
		c.state.statusUpdateCalls++
		c.state.generation.Status = namedString(args, 1)
		if strings.Contains(q, "error_message") {
			c.state.generation.ErrorMessage = ""
		}
		return videoTestResult(1), nil
	case strings.Contains(q, "UPDATE video_generations SET status='failed'"):
		c.state.statusUpdateCalls++
		c.state.generation.Status = "failed"
		c.state.generation.ErrorMessage = namedString(args, 1)
		c.state.generation.VideoURL = ""
		return videoTestResult(1), nil
	case strings.Contains(q, "UPDATE video_generations") && strings.Contains(q, "status='completed'"):
		c.state.statusUpdateCalls++
		c.state.generation.Status = "completed"
		if strings.Contains(q, "video_asset_id") {
			c.state.generation.VideoAssetID = namedString(args, 1)
			c.state.generation.VideoURL = namedString(args, 2)
		} else {
			c.state.generation.VideoURL = namedString(args, 1)
		}
		c.state.generation.ErrorMessage = ""
		return videoTestResult(1), nil
	default:
		return nil, fmtError("unexpected exec: " + q)
	}
}

func (r *videoTestRows) Columns() []string {
	return r.columns
}

func (r *videoTestRows) Close() error {
	return nil
}

func (r *videoTestRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	copy(dest, r.values)
	r.read = true
	return nil
}

func (videoTestResult) LastInsertId() (int64, error) {
	return 0, nil
}

func (r videoTestResult) RowsAffected() (int64, error) {
	return int64(r), nil
}

func singleRow(columns []string, values []driver.Value) driver.Rows {
	return &videoTestRows{columns: columns, values: values}
}

func generationRow(item Generation) driver.Rows {
	now := time.Now()
	return singleRow(
		[]string{
			"id", "provider", "model", "prompt", "image_url", "task_id", "seconds", "aspect_ratio",
			"video_asset_id", "video_url", "duration", "fps", "width", "height",
			"status", "error_message", "create_time", "update_time",
		},
		[]driver.Value{
			item.ID, item.Provider, item.Model, item.Prompt, item.ImageURL, item.TaskID, int64(item.Seconds), item.AspectRatio,
			item.VideoAssetID, item.VideoURL, item.Duration, item.FPS, int64(item.Width), int64(item.Height),
			item.Status, item.ErrorMessage, now, now,
		},
	)
}

func namedString(args []driver.NamedValue, ordinal int) string {
	for _, arg := range args {
		if arg.Ordinal == ordinal {
			v, _ := arg.Value.(string)
			return v
		}
	}
	return ""
}

func namedInt64(args []driver.NamedValue, ordinal int) int64 {
	for _, arg := range args {
		if arg.Ordinal == ordinal {
			switch v := arg.Value.(type) {
			case int64:
				return v
			case int:
				return int64(v)
			}
		}
	}
	return 0
}

func namedFloat64(args []driver.NamedValue, ordinal int) float64 {
	for _, arg := range args {
		if arg.Ordinal == ordinal {
			switch v := arg.Value.(type) {
			case float64:
				return v
			case int64:
				return float64(v)
			}
		}
	}
	return 0
}

func namedBytes(args []driver.NamedValue, ordinal int) []byte {
	for _, arg := range args {
		if arg.Ordinal == ordinal {
			switch v := arg.Value.(type) {
			case []byte:
				return append([]byte(nil), v...)
			case string:
				return []byte(v)
			}
		}
	}
	return nil
}

func fmtError(message string) error {
	return errors.New(message)
}
