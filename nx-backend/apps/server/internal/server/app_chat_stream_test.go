package server

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
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
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
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
			if err := emit("第一段。"); err != nil {
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
			return "第一段。第二段", nil
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
	connected, err := readAppChatSSEEvent(reader)
	if err != nil {
		close(releaseSecond)
		t.Fatal(err)
	}
	if connected != ": connected\n\n" {
		close(releaseSecond)
		t.Fatalf("unexpected initial frame: %q", connected)
	}
	firstEvent, err := readAppChatSSEEvent(reader)
	if err != nil {
		close(releaseSecond)
		t.Fatal(err)
	}
	if !strings.Contains(firstEvent, "event: delta\n") || !strings.Contains(firstEvent, `"content":"第一段。"`) {
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

func TestAppChatAskCleansAndFormatsAnswerBeforeReturningAndSaving(t *testing.T) {
	store := newFakeAppChatStreamStore()
	generator := &hygieneAppChatGenerator{
		answer: "当前通过 Codex CLI 运行。你可以继续描述困扰。再慢慢说清楚。",
	}
	httpServer := newAppChatStreamHTTPServer(t, store, generator)
	defer httpServer.Close()

	response, err := http.Post(httpServer.URL+"/api/app/chat/sessions/42/ask", "application/json", strings.NewReader(`{"question":"请原样输出"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload struct {
		Data askResponse `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	want := "你可以继续描述困扰。\n再慢慢说清楚。"
	if payload.Data.Answer.Answer != want {
		t.Fatalf("response answer = %q, want %q", payload.Data.Answer.Answer, want)
	}
	messages, _ := store.savedMessagesAndSources()
	if len(messages) != 2 || messages[1].Content != want {
		t.Fatalf("saved messages = %#v, want cleaned assistant answer", messages)
	}
}

func TestAppChatAskStreamNeverEmitsRestrictedTermsAndPersistsFormattedAnswer(t *testing.T) {
	store := newFakeAppChatStreamStore()
	generator := &hygieneAppChatGenerator{
		answer:       "当前通过 Codex CLI 运行。你可以继续描述困扰。再慢慢说清楚。",
		streamChunks: []string{"当前通过 Co", "dex CLI 运行。", "你可以继续描述困扰。", "再慢慢说清楚。"},
	}
	body := performAppChatStreamRequest(t, store, generator, context.Background(), nil)
	if strings.Contains(strings.ToLower(body), "codex") || strings.Contains(strings.ToLower(body), "cli") {
		t.Fatalf("stream leaked restricted terms: %q", body)
	}
	if !strings.Contains(body, `"content":"你可以继续描述困扰。"`) ||
		!strings.Contains(body, `"content":"\n再慢慢说清楚。"`) {
		t.Fatalf("stream did not emit formatted safe sentences: %q", body)
	}
	messages, _ := store.savedMessagesAndSources()
	want := "你可以继续描述困扰。\n再慢慢说清楚。"
	if len(messages) != 2 || messages[1].Content != want {
		t.Fatalf("saved messages = %#v, want %q", messages, want)
	}
}

func TestAppChatAskStreamPassesCurrentConversationCardToGenerator(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.cardID = 88
	var captured rag.GenerateInput
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			captured = input
			if err := emit("结合当前人物卡回答"); err != nil {
				return "", err
			}
			return "结合当前人物卡回答", nil
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.db = newAppAnalyticsUnitDB(t, "overview_error")
	s.appChatProfilesForCardOverride = func(_ context.Context, userID, cardID int64) (rag.UserProfile, rag.ConversationCard) {
		if userID != 7 || cardID != 88 {
			t.Fatalf("profile lookup userID=%d cardID=%d, want 7/88", userID, cardID)
		}
		return rag.UserProfile{Nickname: "小林", MainType: 9}, rag.ConversationCard{
			CardType: "secondary",
			Name:     "妈妈",
			Relation: "家人",
			MainType: 2,
			WingType: 1,
			Profile:  `{"primaryMotivation":"希望被需要"}`,
		}
	}
	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"她为什么总替我做决定？"}`))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: 7}))

	s.appChatRouter(writer, req)

	if body := writer.BodyString(); !strings.Contains(body, "event: done\n") {
		t.Fatalf("missing done event: %q", body)
	}
	card := captured.ConversationCard
	if card.CardType != "secondary" || card.Name != "妈妈" || card.Relation != "家人" || card.MainType != 2 || card.WingType != 1 || !strings.Contains(card.Profile, "希望被需要") {
		t.Fatalf("stream generator conversation card = %+v, want current secondary card", card)
	}
}

func TestAppChatAskStreamWritesHeartbeatWhileProviderWaits(t *testing.T) {
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			close(providerStarted)
			select {
			case <-releaseProvider:
			case <-ctx.Done():
				return "", ctx.Err()
			}
			if err := emit("回答"); err != nil {
				return "", err
			}
			return "回答", nil
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatHeartbeatInterval = 15 * time.Millisecond
	s.chatProviderIdleTimeout = time.Second
	httpServer := newAppChatStreamHTTPServerForServer(t, s)
	defer httpServer.Close()

	response, err := http.Post(httpServer.URL+"/api/app/chat/sessions/42/ask/stream", "application/json", strings.NewReader(`{"question":"怎么做？"}`))
	if err != nil {
		close(releaseProvider)
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if frame, err := readAppChatSSEEvent(reader); err != nil || frame != ": connected\n\n" {
		close(releaseProvider)
		t.Fatalf("connected frame = %q, err=%v", frame, err)
	}
	select {
	case <-providerStarted:
	case <-time.After(time.Second):
		close(releaseProvider)
		t.Fatal("provider did not start")
	}
	if frame, err := readAppChatSSEEvent(reader); err != nil || frame != ": ping\n\n" {
		close(releaseProvider)
		t.Fatalf("heartbeat frame = %q, err=%v", frame, err)
	}

	close(releaseProvider)
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
}

func TestAppChatAskStreamProviderIdleIgnoresHeartbeatAndDoesNotSave(t *testing.T) {
	providerFinished := make(chan struct{})
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, _ rag.StreamEmitter) (string, error) {
			defer close(providerFinished)
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatTimeout = 2 * time.Second
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = 55 * time.Millisecond
	writer := newAppChatBlockingStreamWriter()
	req := newAppChatStreamRequest(context.Background())

	started := time.Now()
	s.appChatRouter(writer, req)
	elapsed := time.Since(started)

	select {
	case <-providerFinished:
	case <-time.After(time.Second):
		t.Fatal("provider worker did not exit after idle timeout")
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("idle timeout was not effective: elapsed=%s", elapsed)
	}
	body := writer.BodyString()
	if strings.Count(body, ": ping\n\n") < 2 {
		t.Fatalf("heartbeat did not run while provider was idle: %q", body)
	}
	if got := strings.Count(body, "event: error\n"); got != 1 {
		t.Fatalf("error terminal count = %d, want 1; body=%q", got, body)
	}
	if strings.Contains(body, "event: done\n") {
		t.Fatalf("idle stream unexpectedly completed: %q", body)
	}
	if store.saveCallCount() != 0 {
		t.Fatalf("SavePair called %d times after idle timeout", store.saveCallCount())
	}
}

func TestAppChatAskStreamValidDeltasResetProviderIdle(t *testing.T) {
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			for _, delta := range []string{"一", "二", "三"} {
				if err := emit(delta); err != nil {
					return "", err
				}
				select {
				case <-time.After(35 * time.Millisecond):
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}
			return "一二三", nil
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = 60 * time.Millisecond
	writer := newAppChatBlockingStreamWriter()
	s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))

	body := writer.BodyString()
	if got := strings.Count(body, "event: delta\n"); got != 1 {
		t.Fatalf("sentence-buffered delta count = %d, want 1; body=%q", got, body)
	}
	if got := strings.Count(body, "event: done\n"); got != 1 {
		t.Fatalf("done terminal count = %d, want 1; body=%q", got, body)
	}
	if strings.Contains(body, "event: error\n") {
		t.Fatalf("valid delta resets still timed out: %q", body)
	}
	if store.saveCallCount() != 1 {
		t.Fatalf("SavePair called %d times, want 1", store.saveCallCount())
	}
}

func TestAppChatAskStreamProviderIdleStopsBeforeBlockedSavePair(t *testing.T) {
	saveStarted := make(chan struct{})
	releaseSave := make(chan struct{})
	store := newFakeAppChatStreamStore()
	store.beforeSaveReturn = func(context.Context) {
		close(saveStarted)
		<-releaseSave
	}
	s := newAppChatStreamServer(store, successfulAppChatGenerator("完整回答"))
	s.chatTimeout = 750 * time.Millisecond
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = 35 * time.Millisecond
	writer := newAppChatBlockingStreamWriter()
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))
	}()

	select {
	case <-saveStarted:
	case <-time.After(time.Second):
		t.Fatal("SavePair did not start")
	}
	select {
	case <-handlerDone:
		close(releaseSave)
		t.Fatalf("provider idle ended stream during persistence: %q", writer.BodyString())
	case <-time.After(120 * time.Millisecond):
	}

	close(releaseSave)
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after SavePair was released")
	}
	body := writer.BodyString()
	if strings.Count(body, "event: done\n") != 1 || strings.Contains(body, "event: error\n") {
		t.Fatalf("blocked successful save terminal was wrong: %q", body)
	}
}

func TestAppChatAskStreamCommittedSaveReturningAtTotalDeadlineStillSendsMessageID(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.messageID = 88
	store.beforeSaveReturn = func(ctx context.Context) {
		<-ctx.Done()
	}
	s := newAppChatStreamServer(store, successfulAppChatGenerator("完整回答"))
	s.chatTimeout = 45 * time.Millisecond
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = time.Second
	writer := newAppChatBlockingStreamWriter()

	s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))

	body := writer.BodyString()
	if strings.Count(body, "event: done\n") != 1 || strings.Count(body, "event: error\n") != 0 || !strings.Contains(body, `"messageId":88`) {
		t.Fatalf("save committed at deadline lost its terminal: %q", body)
	}
	if store.saveCallCount() != 1 {
		t.Fatalf("SavePair called %d times, want 1", store.saveCallCount())
	}
}

func TestAppChatAskStreamUncommittedSaveTimeoutEmitsOneError(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.saveErr = context.DeadlineExceeded
	store.beforeSaveReturn = func(ctx context.Context) {
		<-ctx.Done()
	}
	s := newAppChatStreamServer(store, successfulAppChatGenerator("完整回答"))
	s.chatTimeout = 45 * time.Millisecond
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = time.Second
	writer := newAppChatBlockingStreamWriter()

	s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))

	body := writer.BodyString()
	if strings.Count(body, "event: error\n") != 1 || strings.Contains(body, "event: done\n") {
		t.Fatalf("uncommitted save timeout terminal count was wrong: %q", body)
	}
	if store.saveCallCount() != 1 {
		t.Fatalf("SavePair called %d times, want 1", store.saveCallCount())
	}
}

func TestAppChatAskStreamClientDisconnectCancelsBlockedSaveWithoutWorkerLeak(t *testing.T) {
	saveStarted := make(chan struct{})
	saveFinished := make(chan struct{})
	store := newFakeAppChatStreamStore()
	store.beforeSaveReturn = func(ctx context.Context) {
		close(saveStarted)
		<-ctx.Done()
		close(saveFinished)
	}
	s := newAppChatStreamServer(store, successfulAppChatGenerator("完整回答"))
	s.chatHeartbeatInterval = time.Second
	s.chatProviderIdleTimeout = time.Second
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	writer := newAppChatBlockingStreamWriter()
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appChatRouter(writer, newAppChatStreamRequest(requestCtx))
	}()

	select {
	case <-saveStarted:
	case <-time.After(time.Second):
		t.Fatal("SavePair did not start")
	}
	cancelRequest()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not return after client disconnect")
	}
	select {
	case <-saveFinished:
	case <-time.After(time.Second):
		t.Fatal("SavePair worker did not observe client cancellation")
	}
	body := writer.BodyString()
	if strings.Contains(body, "event: done\n") || strings.Contains(body, "event: error\n") {
		t.Fatalf("client disconnect received an unexpected terminal: %q", body)
	}
}

func TestAppChatAskStreamConnectedWriteFailureDoesNotStartPipeline(t *testing.T) {
	providerStarted := make(chan struct{}, 1)
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error) {
			providerStarted <- struct{}{}
			return "unexpected", nil
		},
	}
	s := newAppChatStreamServer(store, generator)
	writer := &failingFrameAppChatStreamWriter{header: make(http.Header), failFrame: ": connected\n\n"}
	s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))

	select {
	case <-providerStarted:
		t.Fatal("provider started after connected frame write failed")
	default:
	}
	if store.saveCallCount() != 0 {
		t.Fatalf("SavePair called %d times", store.saveCallCount())
	}
}

func TestAppChatAskStreamHeartbeatWriteFailureCancelsWorker(t *testing.T) {
	providerFinished := make(chan struct{})
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, _ rag.StreamEmitter) (string, error) {
			defer close(providerFinished)
			<-ctx.Done()
			return "", ctx.Err()
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatHeartbeatInterval = 10 * time.Millisecond
	s.chatProviderIdleTimeout = time.Second
	writer := &failingFrameAppChatStreamWriter{header: make(http.Header), failFrame: ": ping\n\n"}
	s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))

	select {
	case <-providerFinished:
	case <-time.After(time.Second):
		t.Fatal("provider worker leaked after heartbeat write failed")
	}
	if store.saveCallCount() != 0 {
		t.Fatalf("SavePair called %d times", store.saveCallCount())
	}
}

func TestAppChatAskStreamWriterPumpNeverWritesFramesConcurrently(t *testing.T) {
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			var answer strings.Builder
			for range 30 {
				answer.WriteByte('x')
				if err := emit("x"); err != nil {
					return "", err
				}
			}
			return answer.String(), nil
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatHeartbeatInterval = time.Millisecond
	s.chatProviderIdleTimeout = time.Second
	writer := newConcurrencyCheckingAppChatStreamWriter()
	s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))

	if writer.concurrent.Load() {
		t.Fatal("ResponseWriter was written concurrently")
	}
	if got := strings.Count(writer.BodyString(), "event: done\n"); got != 1 {
		t.Fatalf("done terminal count = %d, want 1", got)
	}
}

func TestAppChatAskStreamFlushesConnectedBeforeSlowSummary(t *testing.T) {
	summaryStarted := make(chan struct{})
	releaseSummary := make(chan struct{})
	store := newFakeAppChatStreamStore()
	store.contextStarted = summaryStarted
	store.contextRelease = releaseSummary
	generator := successfulAppChatGenerator("完整回答")
	s := newAppChatStreamServer(store, generator)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/chat/sessions/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 7})
		s.appChatRouter(w, r.WithContext(ctx))
	})
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Post(httpServer.URL+"/api/app/chat/sessions/42/ask/stream", "application/json", strings.NewReader(`{"question":"怎么做？"}`))
	if err != nil {
		close(releaseSummary)
		t.Fatalf("request did not receive the connected frame before summary completed: %v", err)
	}
	defer response.Body.Close()

	frame, err := readAppChatSSEEvent(bufio.NewReader(response.Body))
	if err != nil {
		close(releaseSummary)
		t.Fatal(err)
	}
	if frame != ": connected\n\n" {
		close(releaseSummary)
		t.Fatalf("first SSE frame = %q, want connected comment", frame)
	}
	select {
	case <-summaryStarted:
	case <-time.After(time.Second):
		close(releaseSummary)
		t.Fatal("summary did not start after the stream connected")
	}

	close(releaseSummary)
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
}

func TestAppChatAskStreamOutlivesServerWriteTimeout(t *testing.T) {
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			select {
			case <-time.After(80 * time.Millisecond):
			case <-ctx.Done():
				return "", ctx.Err()
			}
			const answer = "延迟回答。"
			if err := emit(answer); err != nil {
				return "", err
			}
			return answer, nil
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatTimeout = 300 * time.Millisecond
	s.chatHeartbeatInterval = 200 * time.Millisecond
	s.chatProviderIdleTimeout = 250 * time.Millisecond

	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/chat/sessions/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 7})
		s.appChatRouter(w, r.WithContext(ctx))
	})
	httpServer := httptest.NewUnstartedServer(mux)
	httpServer.Config.WriteTimeout = 30 * time.Millisecond
	httpServer.Start()
	defer httpServer.Close()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Post(httpServer.URL+"/api/app/chat/sessions/42/ask/stream", "application/json", strings.NewReader(`{"question":"怎么做？"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(body), "event: done\n"); got != 1 {
		t.Fatalf("done terminal count = %d, want 1; body=%q", got, body)
	}
}

func TestAppChatAskStreamFlushesConnectedBeforePreferenceRead(t *testing.T) {
	preferenceReadStarted := make(chan struct{})
	releasePreferenceRead := make(chan struct{})
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("完整回答"))
	s.userPreferences = &blockingAppChatPreferenceStore{started: preferenceReadStarted, release: releasePreferenceRead}
	httpServer := newAppChatStreamHTTPServerForServer(t, s)
	defer httpServer.Close()

	client := &http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Post(httpServer.URL+"/api/app/chat/sessions/42/ask/stream", "application/json", strings.NewReader(`{"question":"怎么做？"}`))
	if err != nil {
		close(releasePreferenceRead)
		t.Fatalf("request did not receive connected before preference read: %v", err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if frame, err := readAppChatSSEEvent(reader); err != nil || frame != ": connected\n\n" {
		close(releasePreferenceRead)
		t.Fatalf("connected frame = %q, err=%v", frame, err)
	}
	select {
	case <-preferenceReadStarted:
	case <-time.After(time.Second):
		close(releasePreferenceRead)
		t.Fatal("preference read did not start")
	}

	close(releasePreferenceRead)
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatal(err)
	}
}

func TestAppChatAskStreamDeltaWriteFailureCancelsWorkerWithoutSaving(t *testing.T) {
	providerFinished := make(chan struct{})
	store := newFakeAppChatStreamStore()
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			defer close(providerFinished)
			for range 1000 {
				if err := emit("x"); err != nil {
					return "", err
				}
			}
			return strings.Repeat("x", 1000), nil
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatHeartbeatInterval = time.Second
	s.chatProviderIdleTimeout = 2 * time.Second
	writer := &failingFrameAppChatStreamWriter{header: make(http.Header), failFrame: "event: delta\n"}
	s.appChatRouter(writer, newAppChatStreamRequest(context.Background()))

	select {
	case <-providerFinished:
	case <-time.After(time.Second):
		t.Fatal("provider worker leaked after delta write failed")
	}
	if store.saveCallCount() != 0 {
		t.Fatalf("SavePair called %d times", store.saveCallCount())
	}
}

func TestAppChatStreamTimingBoundsIdleByTotalDeadline(t *testing.T) {
	s := &Server{
		chatHeartbeatInterval:   time.Minute,
		chatProviderIdleTimeout: time.Minute,
	}
	heartbeat, idle := s.appChatStreamTiming(80 * time.Millisecond)
	if idle <= 0 || idle >= 80*time.Millisecond {
		t.Fatalf("idle timeout = %s, want positive and shorter than total", idle)
	}
	if heartbeat <= 0 || heartbeat >= idle {
		t.Fatalf("heartbeat interval = %s, want positive and shorter than idle %s", heartbeat, idle)
	}
}

func TestAppChatStreamLifecycleArbitratesTimeoutAndPersistence(t *testing.T) {
	t.Run("timeout wins", func(t *testing.T) {
		lifecycle := &appChatStreamLifecycle{}
		if !lifecycle.stopBeforePersistence() {
			t.Fatal("timeout failed to stop generation")
		}
		if lifecycle.beginPersistence() {
			t.Fatal("SavePair entered after timeout won")
		}
	})
	t.Run("persistence wins", func(t *testing.T) {
		lifecycle := &appChatStreamLifecycle{}
		if !lifecycle.beginPersistence() {
			t.Fatal("SavePair failed to enter persistence")
		}
		if lifecycle.stopBeforePersistence() {
			t.Fatal("timeout overrode persistence")
		}
	})
}

func TestAppChatStreamClosedWorkerStillEmitsTotalTimeoutTerminal(t *testing.T) {
	s := &Server{
		chatHeartbeatInterval:   time.Second,
		chatProviderIdleTimeout: time.Second,
	}
	missingTerminal := 0
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		events := make(chan appChatStreamEvent)
		close(events)
		writer := newAppChatBlockingStreamWriter()
		s.pumpAppChatStream(ctx, func() {}, context.Background(), writer, writer, events, &appChatStreamLifecycle{}, 7, 42, time.Now(), time.Second)
		if strings.Count(writer.BodyString(), "event: error\n") != 1 {
			missingTerminal++
		}
	}
	if missingTerminal != 0 {
		t.Fatalf("%d canceled worker closures omitted the total-timeout terminal", missingTerminal)
	}
}

func TestAppChatStreamQueuedDoneWinsOverExpiredContext(t *testing.T) {
	s := &Server{
		chatHeartbeatInterval:   time.Second,
		chatProviderIdleTimeout: time.Second,
	}
	wrongTerminal := 0
	for range 100 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		events := make(chan appChatStreamEvent, 1)
		events <- appChatStreamEvent{
			kind: appChatStreamDone,
			response: askResponse{
				Answer:    rag.Answer{Answer: "已保存"},
				MessageID: 88,
			},
		}
		close(events)
		writer := newAppChatBlockingStreamWriter()
		s.pumpAppChatStream(ctx, func() {}, context.Background(), writer, writer, events, &appChatStreamLifecycle{}, 7, 42, time.Now(), time.Second)
		body := writer.BodyString()
		if strings.Count(body, "event: done\n") != 1 || strings.Count(body, "event: error\n") != 0 || !strings.Contains(body, `"messageId":88`) {
			wrongTerminal++
		}
	}
	if wrongTerminal != 0 {
		t.Fatalf("%d queued committed results lost to the expired context", wrongTerminal)
	}
}

func TestAppChatStreamPersistencePhaseWaitsForDelayedCommittedTerminalAfterDeadline(t *testing.T) {
	s := &Server{
		chatHeartbeatInterval:   time.Second,
		chatProviderIdleTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan appChatStreamEvent, 1)
	lifecycle := &appChatStreamLifecycle{}
	if !lifecycle.beginPersistence() {
		t.Fatal("failed to enter persistence phase")
	}
	// Queueing the phase before cancellation models SavePair entering its
	// may-commit window while the committed terminal is not enqueued yet.
	events <- appChatStreamEvent{kind: appChatStreamPersistenceStarted}
	cancel()

	releaseDone := make(chan struct{})
	go func() {
		<-releaseDone
		events <- appChatStreamEvent{
			kind: appChatStreamDone,
			response: askResponse{
				Answer:    rag.Answer{Answer: "已保存"},
				MessageID: 88,
			},
		}
		close(events)
	}()

	writer := newAppChatBlockingStreamWriter()
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		s.pumpAppChatStream(ctx, func() {}, context.Background(), writer, writer, events, lifecycle, 7, 42, time.Now(), time.Second)
	}()
	select {
	case <-pumpDone:
		// The buggy pump exits here with a timeout before the committed result
		// can be enqueued. Continue so the terminal assertion shows the defect.
	case <-time.After(20 * time.Millisecond):
	}
	close(releaseDone)
	select {
	case <-pumpDone:
	case <-time.After(time.Second):
		t.Fatal("pump did not finish after committed terminal was released")
	}

	body := writer.BodyString()
	if strings.Count(body, "event: done\n") != 1 || strings.Count(body, "event: error\n") != 0 || !strings.Contains(body, `"messageId":88`) {
		t.Fatalf("committed terminal lost during enqueue window: %q", body)
	}
}

func TestAppChatAskStreamCommittedSaveDoneSurvivesBlockedPostSaveMemory(t *testing.T) {
	memoryStarted := make(chan struct{})
	releaseMemory := make(chan struct{})
	memoryFinished := make(chan struct{})
	memoryContext := make(chan context.Context, 1)
	database := sql.OpenDB(&blockingAppChatExecConnector{
		started:  memoryStarted,
		release:  releaseMemory,
		finished: memoryFinished,
		contexts: memoryContext,
	})
	defer database.Close()

	store := newFakeAppChatStreamStore()
	store.cardID = 9
	store.messageID = 123
	s := newAppChatStreamServer(store, successfulAppChatGenerator("完整回答"))
	s.db = database
	s.chatTimeout = 60 * time.Millisecond
	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"记住我喜欢安静"}`))
	req = req.WithContext(context.WithValue(context.Background(), appContextKey{}, auth.UserInfo{ID: 7}))
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appChatRouter(writer, req)
	}()

	select {
	case <-memoryStarted:
	case <-time.After(time.Second):
		t.Fatal("post-save memory side effect did not start")
	}
	postSaveCtx := <-memoryContext
	deadline, ok := postSaveCtx.Deadline()
	if !ok {
		close(releaseMemory)
		t.Fatal("post-save memory context is not bounded")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > 3*time.Second {
		close(releaseMemory)
		t.Fatalf("post-save memory deadline remaining = %s, want (0, 3s]", remaining)
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		close(releaseMemory)
		t.Fatal("handler did not finish within its bounded deadline")
	}
	close(releaseMemory)
	select {
	case <-memoryFinished:
	case <-time.After(time.Second):
		t.Fatal("bounded post-save memory worker did not exit")
	}

	body := writer.BodyString()
	if got := strings.Count(body, "event: done\n"); got != 1 {
		t.Fatalf("done terminal count = %d, want 1; body=%q", got, body)
	}
	if strings.Contains(body, "event: error\n") {
		t.Fatalf("committed save was reported as an error: %q", body)
	}
	if !strings.Contains(body, `"messageId":123`) {
		t.Fatalf("done event missing committed message id: %q", body)
	}
	if store.saveCallCount() != 1 {
		t.Fatalf("SavePair called %d times, want 1", store.saveCallCount())
	}
}

func TestAppChatAskStreamDoneDoesNotWaitForFallbackPreferenceLock(t *testing.T) {
	store := newFakeAppChatStreamStore()
	s := newAppChatStreamServer(store, successfulAppChatGenerator("完整回答"))
	s.userPreferences = newFakeAppChatPreferenceStore()
	s.preferenceExtractor = &fakeAppChatPreferenceExtractor{}
	s.preferenceAsyncSlots = make(chan struct{}, 1)
	s.preferenceAsyncTimeout = time.Second

	lockedState := make(chan *appChatPreferenceTurnState, 1)
	store.onSave = func() {
		s.preferenceTurnsMu.Lock()
		state := s.preferenceTurns[7]
		s.preferenceTurnsMu.Unlock()
		if state == nil {
			lockedState <- nil
			return
		}
		state.mu.Lock()
		lockedState <- state
	}

	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"以后回答语气更成熟一点"}`))
	req = req.WithContext(context.WithValue(context.Background(), appContextKey{}, auth.UserInfo{ID: 7}))
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appChatRouter(writer, req)
	}()

	var state *appChatPreferenceTurnState
	select {
	case state = <-lockedState:
		if state == nil {
			t.Fatal("preference turn state was missing at SavePair")
		}
	case <-time.After(time.Second):
		t.Fatal("SavePair did not lock the preference state")
	}
	finishedWhileLocked := false
	select {
	case <-handlerDone:
		finishedWhileLocked = true
	case <-time.After(80 * time.Millisecond):
	}
	reservedWhileLocked := state.pending.Load() == 1
	state.mu.Unlock()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler did not finish after preference state was unlocked")
	}
	if !finishedWhileLocked {
		t.Fatal("committed done waited for fallback preference scheduling")
	}
	if !reservedWhileLocked {
		t.Fatal("fallback ownership was not reserved before handler return")
	}
	body := writer.BodyString()
	if strings.Count(body, "event: done\n") != 1 || strings.Contains(body, "event: error\n") {
		t.Fatalf("committed terminal was wrong: %q", body)
	}
	deadline := time.Now().Add(time.Second)
	for state.pending.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state.pending.Load() != 0 {
		t.Fatal("fallback worker did not release its pending reservation")
	}
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
	if got := strings.Count(body, "event: error\n"); got != 1 {
		t.Fatalf("error terminal count = %d, want 1; body=%q", got, body)
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
	if got := strings.Count(body, "event: error\n"); got != 1 {
		t.Fatalf("error terminal count = %d, want 1; body=%q", got, body)
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

func TestAppChatHandlerTotalTimeoutCancelsBeforeProviderLimit(t *testing.T) {
	store := newFakeAppChatStreamStore()
	providerLimit := time.Second
	observedDeadline := make(chan time.Duration, 1)
	generator := &controlledAppChatStreamingGenerator{
		generateStream: func(ctx context.Context, _ rag.GenerateInput, _ rag.StreamEmitter) (string, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				observedDeadline <- 0
			} else {
				observedDeadline <- time.Until(deadline)
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(providerLimit):
				return "late", nil
			}
		},
	}
	s := newAppChatStreamServer(store, generator)
	s.chatTimeout = 40 * time.Millisecond
	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"怎么做？"}`))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: 7}))

	started := time.Now()
	s.appChatRouter(writer, req)
	elapsed := time.Since(started)

	if elapsed >= providerLimit/2 {
		t.Fatalf("handler exceeded its business timeout: elapsed=%s providerLimit=%s", elapsed, providerLimit)
	}
	if observed := <-observedDeadline; observed <= 0 || observed > 80*time.Millisecond {
		t.Fatalf("generator observed wrong handler deadline: %s", observed)
	}
	body := writer.BodyString()
	if strings.Count(body, "event: error\n") != 1 || strings.Contains(body, "event: done\n") {
		t.Fatalf("pre-persistence timeout terminal count was wrong: %q", body)
	}
	if store.saveCallCount() != 0 {
		t.Fatalf("SavePair called %d times before persistence, want 0", store.saveCallCount())
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

type hygieneAppChatGenerator struct {
	answer       string
	streamChunks []string
}

func (g *hygieneAppChatGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return g.answer, nil
}

func (g *hygieneAppChatGenerator) GenerateStream(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	for _, chunk := range g.streamChunks {
		if err := emit(chunk); err != nil {
			return "", err
		}
	}
	return g.answer, nil
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
	mu                 sync.Mutex
	messages           []chat.Message
	messageID          int64
	saveErrorMessageID int64
	saveCalls          int
	contextCalls       int
	lastSources        json.RawMessage
	saveErr            error
	onSave             func()
	beforeSaveReturn   func(context.Context)
	contextStarted     chan struct{}
	contextRelease     chan struct{}
	cardID             int64
}

func newFakeAppChatStreamStore() *fakeAppChatStreamStore {
	return &fakeAppChatStreamStore{messageID: 51}
}

func (s *fakeAppChatStreamStore) GetSession(context.Context, int64, int64) (chat.Session, error) {
	return chat.Session{ID: 42, CardID: s.cardID}, nil
}

func (s *fakeAppChatStreamStore) GetConversationState(ctx context.Context, _ int64) (chat.ConversationState, error) {
	s.mu.Lock()
	s.contextCalls++
	s.mu.Unlock()
	if s.contextStarted != nil {
		close(s.contextStarted)
		select {
		case <-s.contextRelease:
		case <-ctx.Done():
			return chat.ConversationState{}, ctx.Err()
		}
	}
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

func (s *fakeAppChatStreamStore) SavePair(ctx context.Context, sessionID int64, question, answer string, sources json.RawMessage) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	s.lastSources = append(json.RawMessage(nil), sources...)
	if s.onSave != nil {
		s.onSave()
	}
	if s.saveErr != nil {
		if s.beforeSaveReturn != nil {
			s.beforeSaveReturn(ctx)
		}
		return s.saveErrorMessageID, s.saveErr
	}
	if s.beforeSaveReturn != nil {
		s.beforeSaveReturn(ctx)
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

func (s *fakeAppChatStreamStore) contextCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contextCalls
}

func (s *fakeAppChatStreamStore) savedMessagesAndSources() ([]chat.Message, json.RawMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]chat.Message(nil), s.messages...), append(json.RawMessage(nil), s.lastSources...)
}

type emptyAppChatRAGStore struct{ ragDocumentStore }

func (*emptyAppChatRAGStore) EnabledDocuments(context.Context) ([]rag.Document, error) {
	return nil, nil
}

type blockingAppChatPreferenceStore struct {
	started chan struct{}
	release chan struct{}
}

type blockingAppChatExecConnector struct {
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
	contexts chan context.Context
}

func (c *blockingAppChatExecConnector) Connect(context.Context) (driver.Conn, error) {
	return &blockingAppChatExecConn{connector: c}, nil
}

func (*blockingAppChatExecConnector) Driver() driver.Driver {
	return blockingAppChatExecDriver{}
}

type blockingAppChatExecDriver struct{}

func (blockingAppChatExecDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type blockingAppChatExecConn struct {
	connector *blockingAppChatExecConnector
}

func (*blockingAppChatExecConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unsupported")
}

func (*blockingAppChatExecConn) Close() error { return nil }

func (*blockingAppChatExecConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions unsupported")
}

func (c *blockingAppChatExecConn) ExecContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	if c.connector.contexts != nil {
		c.connector.contexts <- ctx
	}
	close(c.connector.started)
	defer close(c.connector.finished)
	select {
	case <-c.connector.release:
		return driver.RowsAffected(1), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *blockingAppChatPreferenceStore) List(ctx context.Context, _ int64) ([]userpreference.Preference, error) {
	close(s.started)
	select {
	case <-s.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (*blockingAppChatPreferenceStore) Apply(context.Context, int64, []userpreference.Mutation) error {
	return nil
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
	return newAppChatStreamHTTPServerForServer(t, newAppChatStreamServer(store, generator))
}

func newAppChatStreamHTTPServerForServer(t *testing.T, s *Server) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/app/chat/sessions/", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), appContextKey{}, auth.UserInfo{ID: 7})
		s.appChatRouter(w, r.WithContext(ctx))
	})
	return httptest.NewServer(mux)
}

func newAppChatStreamRequest(ctx context.Context) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(`{"question":"怎么做？"}`))
	return req.WithContext(context.WithValue(ctx, appContextKey{}, auth.UserInfo{ID: 7}))
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
	s.appChatRouter(writer, newAppChatStreamRequest(ctx))
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

type failingFrameAppChatStreamWriter struct {
	header    http.Header
	body      bytes.Buffer
	failFrame string
}

func (w *failingFrameAppChatStreamWriter) Header() http.Header { return w.header }
func (w *failingFrameAppChatStreamWriter) WriteHeader(int)     {}
func (w *failingFrameAppChatStreamWriter) Flush()              {}
func (w *failingFrameAppChatStreamWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), w.failFrame) {
		return 0, errors.New("forced stream write failure")
	}
	return w.body.Write(p)
}

type concurrencyCheckingAppChatStreamWriter struct {
	header     http.Header
	bodyMu     sync.Mutex
	body       bytes.Buffer
	writing    atomic.Int32
	concurrent atomic.Bool
}

func newConcurrencyCheckingAppChatStreamWriter() *concurrencyCheckingAppChatStreamWriter {
	return &concurrencyCheckingAppChatStreamWriter{header: make(http.Header)}
}

func (w *concurrencyCheckingAppChatStreamWriter) Header() http.Header { return w.header }
func (w *concurrencyCheckingAppChatStreamWriter) WriteHeader(int)     {}
func (w *concurrencyCheckingAppChatStreamWriter) Flush()              {}
func (w *concurrencyCheckingAppChatStreamWriter) Write(p []byte) (int, error) {
	if w.writing.Add(1) != 1 {
		w.concurrent.Store(true)
	}
	defer w.writing.Add(-1)
	time.Sleep(200 * time.Microsecond)
	w.bodyMu.Lock()
	defer w.bodyMu.Unlock()
	return w.body.Write(p)
}

func (w *concurrencyCheckingAppChatStreamWriter) BodyString() string {
	w.bodyMu.Lock()
	defer w.bodyMu.Unlock()
	return w.body.String()
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

func appChatNginxStripComments(config string) string {
	var result strings.Builder
	result.Grow(len(config))
	var quote byte
	escaped := false
	for index := 0; index < len(config); index++ {
		char := config[index]
		if escaped {
			result.WriteByte(char)
			escaped = false
			continue
		}
		if quote != 0 {
			result.WriteByte(char)
			if char == '\\' {
				escaped = true
			} else if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			result.WriteByte(char)
			continue
		}
		if char == '#' {
			for index < len(config) && config[index] != '\n' {
				index++
			}
			if index < len(config) {
				result.WriteByte('\n')
			}
			continue
		}
		result.WriteByte(char)
	}
	return result.String()
}

func appChatNginxLocationHasDirective(block, directive string) bool {
	open := strings.IndexByte(block, '{')
	close := strings.LastIndexByte(block, '}')
	if open < 0 || close <= open {
		return false
	}
	want := strings.Join(strings.Fields(directive), " ")
	body := block[open+1 : close]
	statementStart := 0
	var quote byte
	escaped := false
	for index := 0; index < len(body); index++ {
		char := body[index]
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
		if char != ';' {
			continue
		}
		statement := strings.Join(strings.Fields(body[statementStart:index]), " ")
		if statement+";" == want {
			return true
		}
		statementStart = index + 1
	}
	return false
}
