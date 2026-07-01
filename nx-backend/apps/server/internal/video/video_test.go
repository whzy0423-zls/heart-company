package video

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/storage"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

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

	client := NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
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

	client := NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
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

	client := NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
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

	client := NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
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

	store := NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
	if _, err := store.Generate(context.Background(), GenerateInput{Prompt: "test", AspectRatio: "4:3"}); err == nil {
		t.Fatal("expected unsupported aspect ratio to be rejected")
	}
	if called {
		t.Fatal("unsupported aspect ratio should be rejected before calling video gateway")
	}
}

func TestGenerateRejectsSuccessfulCreateWithoutTaskID(t *testing.T) {
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

	store := NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		Model:   "video-ds-2.0-fast",
	})
	_, err := store.Generate(context.Background(), GenerateInput{Prompt: "test"})
	if err == nil {
		t.Fatal("expected missing task_id error")
	}
	if !strings.Contains(err.Error(), "task_id") {
		t.Fatalf("expected error to mention task_id, got %q", err.Error())
	}

	state := videoTestState(t, db)
	if state.insertCalls != 0 {
		t.Fatalf("expected no generation row to be inserted without task_id, got %d inserts", state.insertCalls)
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

	store := NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
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

	store := NewStore(db, nil, config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
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
	store := NewStore(db, nil, config.VideoConfig{APIBase: server.URL, APIKey: "test-key"}, uploader)
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
	store := NewStore(db, uploadasset.NewStore(db), config.VideoConfig{APIBase: server.URL, APIKey: "test-key"}, uploader)
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

	store := NewStore(db, uploadasset.NewStore(db), config.VideoConfig{APIBase: server.URL, APIKey: "test-key"})
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

	client := NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
	if _, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, []string{"https://example.com/input.mp3"}, 15, "9:16"); err != nil {
		t.Fatal(err)
	}
	audios, ok := gotBody["audios"].([]any)
	if !ok || len(audios) != 1 || audios[0] != "https://example.com/input.mp3" {
		t.Fatalf("expected audios to be forwarded, got %#v", gotBody["audios"])
	}
}

func TestCreateTaskRetriesEOFOnce(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
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

	client := NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
	result, err := client.CreateTask(context.Background(), "video-ds-2.0-fast", "test", nil, nil, nil, 15, "9:16")
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskID != "task-1" {
		t.Fatalf("expected task-1, got %q", result.TaskID)
	}
	if attempts != 2 {
		t.Fatalf("expected one retry after EOF, got %d attempts", attempts)
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

	client := NewClient(config.VideoConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})
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
	generation        Generation
	uploadAsset       map[string]driver.Value
	uploadAssetID     int64
	uploadCreateCalls int
	insertCalls       int
	statusUpdateCalls int
}

type recordingVideoUploader struct {
	url       string
	objectKey string
}

func (u *recordingVideoUploader) Upload(ctx context.Context, input storage.UploadInput) (storage.UploadResult, error) {
	return storage.UploadResult{
		Key:       u.objectKey,
		URL:       u.url,
		ObjectKey: u.objectKey,
		ObjectURL: u.url,
		Name:      input.Filename,
	}, nil
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

var (
	videoTestMu     sync.Mutex
	videoTestStates = map[string]*videoDBState{}
)

func openVideoTestDB(t *testing.T, state *videoDBState) *sql.DB {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
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
	return nil, errors.New("transactions are not supported")
}

func (c *videoTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q := strings.TrimSpace(query)
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
