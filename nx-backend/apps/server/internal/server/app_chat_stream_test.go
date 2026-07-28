package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestWriteAppChatSSEWritesDeltaEventAndFlushes(t *testing.T) {
	var body bytes.Buffer
	flusher := &appChatStreamTestFlusher{}

	if err := writeAppChatSSE(&body, flusher, "delta", map[string]string{"content": "hello"}); err != nil {
		t.Fatalf("writeAppChatSSE returned error: %v", err)
	}

	got := body.String()
	if !strings.Contains(got, "event: delta\n") {
		t.Fatalf("SSE output missing delta event line: %q", got)
	}
	if !strings.Contains(got, `data: {"content":"hello"}`+"\n\n") {
		t.Fatalf("SSE output missing JSON data line: %q", got)
	}
	if flusher.flushes != 1 {
		t.Fatalf("Flush called %d times, want 1", flusher.flushes)
	}
}

func TestAppChatSessionIDFromPathParsesStreamAndAskSuffixes(t *testing.T) {
	id, ok := appChatSessionIDFromPath("/api/app/chat/sessions/1/ask/stream", "/ask/stream")
	if !ok || id != 1 {
		t.Fatalf("stream path parsed as id=%d ok=%v, want id=1 ok=true", id, ok)
	}

	id, ok = appChatSessionIDFromPath("/api/app/chat/sessions/2/ask", "/ask")
	if !ok || id != 2 {
		t.Fatalf("ask path parsed as id=%d ok=%v, want id=2 ok=true", id, ok)
	}
}

func TestAppChatSessionIDFromPathRejectsInvalidPaths(t *testing.T) {
	invalidPaths := []string{
		"/api/app/chat/sessions/not-a-number/ask/stream",
		"/api/app/chat/sessions/0/ask/stream",
		"1/ask/stream",
		"/api/app/chat/session/1/ask/stream",
		"/api/app/chat/sessions/1/ask/stream/extra",
	}

	for _, path := range invalidPaths {
		if id, ok := appChatSessionIDFromPath(path, "/ask/stream"); ok {
			t.Fatalf("appChatSessionIDFromPath(%q) = id=%d ok=true, want ok=false", path, id)
		}
	}
}

func TestAppChatStreamingProxyConfig(t *testing.T) {
	repoRoot := appChatStreamTestRepoRoot(t)
	configPaths := []string{
		"website-react/nginx.conf",
		"nx-backend/scripts/deploy/nginx.conf",
	}
	requiredDirectives := []string{
		"proxy_pass http://backend;",
		"proxy_http_version 1.1;",
		`proxy_set_header Connection "";`,
		"proxy_buffering off;",
		"proxy_cache off;",
		"gzip off;",
		"proxy_connect_timeout 30s;",
		"proxy_read_timeout 180s;",
		"proxy_send_timeout 180s;",
		"proxy_set_header Host $host;",
		"proxy_set_header X-Real-IP $remote_addr;",
		"proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;",
		"proxy_set_header X-Forwarded-Proto $scheme;",
	}

	for _, relativePath := range configPaths {
		t.Run(relativePath, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
			if err != nil {
				t.Fatal(err)
			}
			config := string(body)
			focusedStart := strings.Index(config, "location ^~ /api/app/chat/")
			if focusedStart < 0 {
				t.Fatalf("%s missing focused app chat streaming location", relativePath)
			}
			genericStart := strings.Index(config, "location /api/")
			if genericStart < 0 {
				t.Fatalf("%s missing generic /api/ location", relativePath)
			}
			if focusedStart > genericStart {
				t.Fatalf("%s focused app chat location must appear before generic /api/ location", relativePath)
			}

			block := appChatNginxLocationBlock(t, config[focusedStart:])
			normalized := strings.Join(strings.Fields(block), " ")
			for _, directive := range requiredDirectives {
				if !strings.Contains(normalized, strings.Join(strings.Fields(directive), " ")) {
					t.Errorf("%s app chat location missing %q; block=%q", relativePath, directive, block)
				}
			}
		})
	}
}

func appChatStreamTestRepoRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller could not locate app chat stream test")
	}
	dir := filepath.Dir(filename)
	for range 8 {
		if _, err := os.Stat(filepath.Join(dir, "website-react", "nginx.conf")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "nx-backend", "scripts", "deploy", "nginx.conf")); err == nil {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate repository root from %s", filename)
	return ""
}

func appChatNginxLocationBlock(t *testing.T, config string) string {
	t.Helper()
	config = appChatNginxStripComments(config)
	open := strings.IndexByte(config, '{')
	if open < 0 {
		t.Fatal("nginx location missing opening brace")
	}
	depth := 0
	var quote byte
	escaped := false
	for index := open; index < len(config); index++ {
		char := config[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		switch char {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return config[:index+1]
			}
		}
	}
	t.Fatal("nginx location missing closing brace")
	return ""
}

func TestAppChatAskStreamFlushesFirstDeltaBeforeGenerationCompletes(t *testing.T) {
	firstEmitted := make(chan struct{})
	releaseSecond := make(chan struct{})
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			if err := emit("第一段"); err != nil {
				return "", err
			}
			close(firstEmitted)
			select {
			case <-releaseSecond:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			if err := emit("第二段"); err != nil {
				return "", err
			}
			return "第一段第二段", nil
		},
	}
	httpServer := newAppChatStreamHTTPServer(t, store, generator)
	defer httpServer.Close()

	response, err := http.Post(httpServer.URL+"/api/app/chat/sessions/42/ask/stream", "application/json", strings.NewReader(`{"question":"怎么做？"}`))
	if err != nil {
		close(releaseSecond)
		t.Fatal(err)
	}
	defer response.Body.Close()

	if got := response.Header.Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q", got)
	}

	select {
	case <-firstEmitted:
	case <-time.After(2 * time.Second):
		close(releaseSecond)
		t.Fatal("generator did not emit first delta")
	}
	reader := bufio.NewReader(response.Body)
	initialEvent, err := readAppChatSSEEvent(reader)
	if err != nil {
		close(releaseSecond)
		t.Fatal(err)
	}
	if initialEvent != ": ping\n\n" {
		close(releaseSecond)
		t.Fatalf("unexpected initial event: %q", initialEvent)
	}
	firstEvent, err := readAppChatSSEEvent(reader)
	if err != nil {
		close(releaseSecond)
		t.Fatal(err)
	}
	if !strings.Contains(firstEvent, "event: delta\n") || !strings.Contains(firstEvent, `"content":"第一段"`) {
		close(releaseSecond)
		t.Fatalf("unexpected first event: %q", firstEvent)
	}
	if store.saveCallCount() != 0 {
		close(releaseSecond)
		t.Fatal("answer was saved before generation completed")
	}

	close(releaseSecond)
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
}

func TestAppChatStreamInitialFlushAndHeartbeatArriveBeforeFirstDelta(t *testing.T) {
	generatorStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseDone := make(chan struct{})
	defer func() {
		select {
		case <-releaseFirst:
		default:
			close(releaseFirst)
		}
		select {
		case <-releaseDone:
		default:
			close(releaseDone)
		}
	}()

	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			close(generatorStarted)
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			if err := emit("第一段"); err != nil {
				return "", err
			}
			select {
			case <-releaseDone:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			return "第一段", nil
		},
	}
	s := newAppChatStreamServer(newFakeAppChatStreamStore(), generator)
	s.chatHeartbeatInterval = 20 * time.Millisecond
	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/chat/sessions/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 7})
		s.appChatRouter(w, r.WithContext(ctx))
	})
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	responseCh := make(chan *http.Response, 1)
	errorCh := make(chan error, 1)
	go func() {
		response, err := http.Post(httpServer.URL+"/api/app/chat/sessions/42/ask/stream", "application/json", strings.NewReader(`{"question":"怎么做？"}`))
		if err != nil {
			errorCh <- err
			return
		}
		responseCh <- response
	}()

	select {
	case <-generatorStarted:
	case err := <-errorCh:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("generator did not start")
	}

	var response *http.Response
	select {
	case response = <-responseCh:
	case err := <-errorCh:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("response headers and initial SSE comment were not flushed before first delta")
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	firstEvent, err := readAppChatSSEEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if firstEvent != ": ping\n\n" {
		t.Fatalf("first SSE event = %q, want initial ping", firstEvent)
	}

	select {
	case <-time.After(30 * time.Millisecond):
	case <-response.Request.Context().Done():
		t.Fatal(response.Request.Context().Err())
	}
	heartbeat, err := readAppChatSSEEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if heartbeat != ": ping\n\n" {
		t.Fatalf("heartbeat = %q, want ping comment", heartbeat)
	}

	close(releaseFirst)
	deltaEvent, err := readAppChatSSEEvent(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(deltaEvent, "event: delta\n") || !strings.Contains(deltaEvent, "第一段") {
		t.Fatalf("unexpected delta event: %q", deltaEvent)
	}
	select {
	case <-releaseDone:
		t.Fatal("generation completed before the test released it")
	default:
	}
	close(releaseDone)
}

func TestAppChatAskStreamDoesNotSavePartialAnswerWhenGenerationFails(t *testing.T) {
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			if err := emit("只有一段"); err != nil {
				return "", err
			}
			return "", errors.New("upstream failed")
		},
	}

	body := performAppChatStreamRequest(t, store, generator, context.Background(), nil)

	if store.saveCallCount() != 0 {
		t.Fatalf("SavePair called %d times, want 0", store.saveCallCount())
	}
	if !strings.Contains(body, "event: error\n") {
		t.Fatalf("SSE output missing error event: %q", body)
	}
	if strings.Contains(body, "event: done\n") {
		t.Fatalf("SSE output unexpectedly contains done event: %q", body)
	}
}

func TestAppChatAskStreamSavesBeforeDoneWithMessageID(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.messageID = 91
	writer := newOrderedAppChatStreamWriter(store)

	body := performAppChatStreamRequest(t, store, successfulAppChatGenerator("完整回答"), context.Background(), writer)

	if got := strings.Join(writer.order, ","); got != "delta,save,done" {
		t.Fatalf("stream order = %q, want delta,save,done", got)
	}
	if !strings.Contains(body, `"messageId":91`) {
		t.Fatalf("done event missing message id: %q", body)
	}
}

func TestAppChatAskStreamSaveFailureEmitsErrorWithoutDone(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.saveErr = errors.New("database unavailable")

	body := performAppChatStreamRequest(t, store, successfulAppChatGenerator("完整回答"), context.Background(), nil)

	if store.saveCallCount() != 1 {
		t.Fatalf("SavePair called %d times, want 1", store.saveCallCount())
	}
	if !strings.Contains(body, "event: error\n") {
		t.Fatalf("SSE output missing error event after save failure: %q", body)
	}
	if strings.Contains(body, "event: done\n") {
		t.Fatalf("SSE output unexpectedly contains done after save failure: %q", body)
	}
}

func TestAppChatAskStreamKeepsSavedPairWhenDoneWriteFails(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.messageID = 73
	writer := &failingDoneAppChatStreamWriter{header: make(http.Header)}

	performAppChatStreamRequest(t, store, successfulAppChatGenerator("完整回答"), context.Background(), writer)

	messages, err := store.ListMessages(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "怎么做？" || messages[1].Content != "完整回答" {
		t.Fatalf("saved history = %#v", messages)
	}
}

func TestAppChatAskStreamClientCancellationDoesNotSavePartialPair(t *testing.T) {
	store := newFakeAppChatStreamStore()
	started := make(chan struct{})
	finished := make(chan struct{})
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			defer close(finished)
			if err := emit("部分回答"); err != nil {
				return "", err
			}
			close(started)
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	writer := &appChatBlockingStreamWriter{header: make(http.Header)}
	done := make(chan struct{})
	go func() {
		defer close(done)
		performAppChatStreamRequest(t, store, generator, ctx, writer)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not start")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("generator did not observe cancellation")
	}
	<-done
	if store.saveCallCount() != 0 {
		t.Fatalf("SavePair called %d times after cancellation, want 0", store.saveCallCount())
	}
}

type appChatStreamTestFlusher struct {
	flushes int
}

func (f *appChatStreamTestFlusher) Flush() {
	f.flushes++
}

type controlledAppChatStreamingGenerator struct {
	generateStream func(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error)
}

func (g *controlledAppChatStreamingGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "", errors.New("unexpected non-streaming generation")
}

func (g *controlledAppChatStreamingGenerator) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	return g.generateStream(ctx, input, emit)
}

func successfulAppChatGenerator(answer string) rag.Generator {
	return &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			if err := emit(answer); err != nil {
				return "", err
			}
			return answer, nil
		},
	}
}

type fakeAppChatStreamStore struct {
	appChatStore
	mu        sync.Mutex
	messages  []chat.Message
	messageID int64
	saveCalls int
	saveErr   error
	onSave    func()
}

func newFakeAppChatStreamStore() *fakeAppChatStreamStore {
	return &fakeAppChatStreamStore{messageID: 51}
}

func (s *fakeAppChatStreamStore) GetSession(context.Context, int64, int64) (chat.Session, error) {
	return chat.Session{ID: 42}, nil
}

func (s *fakeAppChatStreamStore) GetConversationState(context.Context, int64) (chat.ConversationState, error) {
	return chat.ConversationState{}, nil
}

func (s *fakeAppChatStreamStore) ListMessagesAfter(context.Context, int64, int64) ([]chat.Message, error) {
	return nil, nil
}

func (s *fakeAppChatStreamStore) ListRecentMessages(context.Context, int64, int) ([]chat.Message, error) {
	return nil, nil
}

func (s *fakeAppChatStreamStore) UpdateConversationSummary(context.Context, int64, int64, string, int64) (bool, error) {
	return true, nil
}

func (s *fakeAppChatStreamStore) SavePair(_ context.Context, sessionID int64, question, answer string, _ json.RawMessage) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	if s.onSave != nil {
		s.onSave()
	}
	if s.saveErr != nil {
		return 0, s.saveErr
	}
	s.messages = append(s.messages,
		chat.Message{SessionID: sessionID, Role: "user", Content: question},
		chat.Message{ID: s.messageID, SessionID: sessionID, Role: "assistant", Content: answer},
	)
	return s.messageID, nil
}

func (s *fakeAppChatStreamStore) ListMessages(context.Context, int64) ([]chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]chat.Message(nil), s.messages...), nil
}

func (s *fakeAppChatStreamStore) saveCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveCalls
}

type emptyAppChatRAGStore struct{ ragDocumentStore }

func (*emptyAppChatRAGStore) EnabledDocuments(context.Context) ([]rag.Document, error) {
	return nil, nil
}

func newAppChatStreamServer(store appChatStore, generator rag.Generator) *Server {
	return &Server{
		env:         config.Env{SiteConfig: filepath.Join("..", "..", "..", "shared", "site-config.json")},
		appChat:     store,
		ragGen:      generator,
		ragDocs:     &emptyAppChatRAGStore{},
		chatTimeout: 5 * time.Second,
	}
}

func newAppChatStreamHTTPServer(t *testing.T, store appChatStore, generator rag.Generator) *httptest.Server {
	t.Helper()
	s := newAppChatStreamServer(store, generator)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/chat/sessions/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 7})
		s.appChatRouter(w, r.WithContext(ctx))
	})
	return httptest.NewServer(mux)
}

func performAppChatStreamRequest(t *testing.T, store *fakeAppChatStreamStore, generator rag.Generator, ctx context.Context, writer http.ResponseWriter) string {
	t.Helper()
	s := newAppChatStreamServer(store, generator)
	if writer == nil {
		writer = newAppChatBlockingStreamWriter()
	}
	if ordered, ok := writer.(*orderedAppChatStreamWriter); ok {
		store.onSave = func() { ordered.order = append(ordered.order, "save") }
	}
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"怎么做？"}`))
	req = req.WithContext(context.WithValue(ctx, appContextKey{}, auth.UserInfo{ID: 7}))
	s.appChatRouter(writer, req)
	switch typed := writer.(type) {
	case interface{ BodyString() string }:
		return typed.BodyString()
	default:
		return ""
	}
}

func readAppChatSSEEvent(reader *bufio.Reader) (string, error) {
	var event strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		event.WriteString(line)
		if line == "\n" {
			return event.String(), nil
		}
	}
}

type appChatBlockingStreamWriter struct {
	header http.Header
	body   bytes.Buffer
}

func newAppChatBlockingStreamWriter() *appChatBlockingStreamWriter {
	return &appChatBlockingStreamWriter{header: make(http.Header)}
}

func (w *appChatBlockingStreamWriter) Header() http.Header         { return w.header }
func (w *appChatBlockingStreamWriter) WriteHeader(int)             {}
func (w *appChatBlockingStreamWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *appChatBlockingStreamWriter) Flush()                      {}
func (w *appChatBlockingStreamWriter) BodyString() string          { return w.body.String() }

type orderedAppChatStreamWriter struct {
	*appChatBlockingStreamWriter
	order []string
}

func newOrderedAppChatStreamWriter(_ *fakeAppChatStreamStore) *orderedAppChatStreamWriter {
	return &orderedAppChatStreamWriter{appChatBlockingStreamWriter: newAppChatBlockingStreamWriter()}
}

func (w *orderedAppChatStreamWriter) Write(p []byte) (int, error) {
	text := string(p)
	if strings.Contains(text, "event: delta\n") {
		w.order = append(w.order, "delta")
	}
	if strings.Contains(text, "event: done\n") {
		w.order = append(w.order, "done")
	}
	return w.appChatBlockingStreamWriter.Write(p)
}

type failingDoneAppChatStreamWriter struct {
	header http.Header
	body   bytes.Buffer
}

func (w *failingDoneAppChatStreamWriter) Header() http.Header { return w.header }
func (w *failingDoneAppChatStreamWriter) WriteHeader(int)     {}
func (w *failingDoneAppChatStreamWriter) Flush()              {}
func (w *failingDoneAppChatStreamWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "event: done\n") {
		return 0, errors.New("client disconnected before done")
	}
	return w.body.Write(p)
}
