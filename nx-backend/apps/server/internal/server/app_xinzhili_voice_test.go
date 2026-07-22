package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

func TestAppXinzhiliVoiceTurnRejectsOversizedRequestWithoutMultipartResidue(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	s := &Server{
		xinzhiliConfigLoader: func(context.Context) (modelconfig.XinzhiliVoiceConfig, error) { return readyXinzhiliVoiceConfig(), nil },
		xinzhiliMemberCheck:  func(context.Context, int64) error { return nil },
	}
	req := newXinzhiliMultipartRequest(t, bytes.Repeat([]byte("a"), xinzhiliMaxRequestBytes), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d; body=%s", res.Code, http.StatusRequestEntityTooLarge, res.Body.String())
	}
	if body := res.Body.String(); !strings.Contains(body, "音频文件无效或过大") {
		t.Fatalf("oversized response body = %s", body)
	}
	assertNoMultipartResidue(t, tmpDir)
}

func TestAppXinzhiliVoiceTurnKeepsMalformedMultipartAsBadRequest(t *testing.T) {
	s := &Server{
		xinzhiliConfigLoader: func(context.Context) (modelconfig.XinzhiliVoiceConfig, error) { return readyXinzhiliVoiceConfig(), nil },
		xinzhiliMemberCheck:  func(context.Context, int64) error { return nil },
	}
	req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/turns/stream", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=broken")
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "音频上传格式不正确") {
		t.Fatalf("malformed response = %d %s", res.Code, res.Body.String())
	}
}

func TestAppXinzhiliVoiceTurnRateLimitsPerUserBeforePaidWork(t *testing.T) {
	asrCalls := 0
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.chatLimiter = newFixedWindowRateLimiter(1, time.Minute)
	s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) {
		asrCalls++
		return "", context.DeadlineExceeded
	}
	s.ragGen = generatorFunc(func(context.Context, rag.GenerateInput) (string, error) {
		t.Fatal("rate-limited request must not call the generator")
		return "", nil
	})
	s.xinzhiliSynthesize = func(context.Context, string) ([]byte, string, error) {
		t.Fatal("rate-limited request must not call TTS")
		return nil, "", nil
	}

	perform := func(userID int64) *httptest.ResponseRecorder {
		req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
		req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: userID}))
		res := httptest.NewRecorder()
		s.appXinzhiliVoiceTurnStream(res, req)
		return res
	}

	first := perform(7)
	if first.Code != http.StatusOK {
		t.Fatalf("first request status = %d, body=%s", first.Code, first.Body.String())
	}
	limited := perform(7)
	if limited.Code != http.StatusTooManyRequests || !strings.Contains(limited.Body.String(), "请求过于频繁，请稍后再试") {
		t.Fatalf("limited response = %d %s", limited.Code, limited.Body.String())
	}
	otherUser := perform(8)
	if otherUser.Code != http.StatusOK {
		t.Fatalf("different user status = %d, body=%s", otherUser.Code, otherUser.Body.String())
	}
	if asrCalls != 2 {
		t.Fatalf("ASR calls = %d, want 2", asrCalls)
	}
}

func TestAppXinzhiliVoiceTurnWaitsForIdleTTSWorkerAfterStreamWriteFailure(t *testing.T) {
	workerStarted := make(chan struct{})
	workerExited := make(chan struct{})
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		<-workerStarted
		if err := emit("还没有形成完整句子"); err != nil {
			return "", err
		}
		return "还没有形成完整句子", nil
	})
	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	writer := &failingXinzhiliSSEWriter{header: make(http.Header), failEvent: "text_delta"}

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appXinzhiliVoiceTurnStreamWithRuntimeHooks(writer, req, xinzhiliMultipartMemory, xinzhiliVoiceRuntimeHooks{
			onTTSWorkerStart: func() { close(workerStarted) },
			onTTSWorkerExit:  func() { close(workerExited) },
		})
	}()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler remained blocked waiting for the idle TTS worker")
	}
	select {
	case <-workerExited:
	default:
		t.Fatal("handler completed before the idle TTS worker exited")
	}
}

func TestAppXinzhiliVoiceTurnWaitsForIdleTTSWorkerAfterRequestCancellation(t *testing.T) {
	workerStarted := make(chan struct{})
	workerExited := make(chan struct{})
	releaseGeneration := make(chan struct{})
	defer close(releaseGeneration)
	ctx, cancel := context.WithCancel(context.Background())
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, _ rag.StreamEmitter) (string, error) {
		<-workerStarted
		cancel()
		<-releaseGeneration
		return "", context.Canceled
	})
	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(ctx, auth.UserInfo{ID: 7}))
	writer := httptest.NewRecorder()

	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appXinzhiliVoiceTurnStreamWithRuntimeHooks(writer, req, xinzhiliMultipartMemory, xinzhiliVoiceRuntimeHooks{
			onTTSWorkerStart: func() { close(workerStarted) },
			onTTSWorkerExit:  func() { close(workerExited) },
		})
	}()
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler remained blocked waiting for the idle TTS worker after request cancellation")
	}
	select {
	case <-workerExited:
	default:
		t.Fatal("handler completed before the canceled TTS worker exited")
	}
}

func TestCleanupXinzhiliMultipartFormLogsRemoveFailureWithoutRequestContent(t *testing.T) {
	var logged string
	cleanupXinzhiliMultipartForm(
		&multipart.Form{},
		func(*multipart.Form) error { return errors.New("disk cleanup failed") },
		func(format string, args ...any) { logged = fmt.Sprintf(format, args...) },
	)

	if !strings.Contains(logged, "xinzhili multipart cleanup failed") || !strings.Contains(logged, "disk cleanup failed") {
		t.Fatalf("cleanup warning = %q", logged)
	}
	for _, sensitive := range []string{"audioBase64", "我最近总是着急", "先停一下"} {
		if strings.Contains(logged, sensitive) {
			t.Fatalf("cleanup warning leaked request content %q: %s", sensitive, logged)
		}
	}
}

func TestAppXinzhiliVoiceTurnAcceptsTenMiBAudio(t *testing.T) {
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.xinzhiliTranscribe = func(_ context.Context, audio []byte, _ string) (string, error) {
		if len(audio) != xinzhiliMaxAudioBytes {
			t.Fatalf("audio bytes = %d, want %d", len(audio), xinzhiliMaxAudioBytes)
		}
		return "我最近总是着急", nil
	}
	req := newXinzhiliMultipartRequest(t, bytes.Repeat([]byte("a"), xinzhiliMaxAudioBytes), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if body := res.Body.String(); !strings.Contains(body, "event: done") {
		t.Fatalf("10 MiB audio was not accepted: %s", body)
	}
}

func TestAppXinzhiliVoiceTurnCleansSpilledMultipartFilesOnEveryExit(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	tests := []struct {
		name      string
		configure func(*testing.T, *Server, context.CancelFunc)
	}{
		{
			name: "normal completion",
			configure: func(t *testing.T, s *Server, _ context.CancelFunc) {
				s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) {
					assertMultipartResiduePresent(t, tmpDir)
					return "我最近总是着急", nil
				}
			},
		},
		{
			name: "ASR failure",
			configure: func(t *testing.T, s *Server, _ context.CancelFunc) {
				s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) {
					assertMultipartResiduePresent(t, tmpDir)
					return "", context.DeadlineExceeded
				}
			},
		},
		{
			name: "no meaningful transcript",
			configure: func(t *testing.T, s *Server, _ context.CancelFunc) {
				s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) {
					assertMultipartResiduePresent(t, tmpDir)
					return "……", nil
				}
			},
		},
		{
			name: "request canceled",
			configure: func(t *testing.T, s *Server, cancel context.CancelFunc) {
				s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) {
					assertMultipartResiduePresent(t, tmpDir)
					cancel()
					return "我最近总是着急", nil
				}
				s.xinzhiliSession = func(ctx context.Context, _ int64) (chat.Session, error) {
					return chat.Session{}, ctx.Err()
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := newSuccessfulXinzhiliVoiceServer(t)
			tt.configure(t, s, cancel)
			req := newXinzhiliMultipartRequest(t, bytes.Repeat([]byte("a"), 2048), 1300)
			req = req.WithContext(contextWithAppUser(ctx, auth.UserInfo{ID: 7}))
			res := httptest.NewRecorder()

			s.appXinzhiliVoiceTurnStreamWithMultipartMemory(res, req, 1)

			assertNoMultipartResidue(t, tmpDir)
		})
	}
}

func TestAppXinzhiliVoiceTurnStreamsTranscriptTextAudioAndPersists(t *testing.T) {
	var savedQuestion, savedAnswer string
	s := &Server{
		chatTimeout: 30,
		ragGen:      xinzhiliTestGenerator{},
		xinzhiliConfigLoader: func(context.Context) (modelconfig.XinzhiliVoiceConfig, error) {
			return readyXinzhiliVoiceConfig(), nil
		},
		xinzhiliMemberCheck: func(context.Context, int64) error { return nil },
		xinzhiliTranscribe:  func(context.Context, []byte, string) (string, error) { return "我最近总是着急", nil },
		xinzhiliSynthesize: func(_ context.Context, text string) ([]byte, string, error) {
			return []byte("audio:" + text), "audio/mpeg", nil
		},
		xinzhiliSession: func(context.Context, int64) (chat.Session, error) {
			return chat.Session{ID: 91, CardID: 5}, nil
		},
		xinzhiliRetrieveDocs: func(context.Context, string, int) ([]rag.Document, error) {
			return []rag.Document{{ID: "kb-1", Title: "知识", Content: "着急时先觉察身体"}, {ID: "theory-1", Title: "九型理论", Content: "不同型号有不同防御机制"}}, nil
		},
		xinzhiliSavePair: func(_ context.Context, sessionID int64, question, answer string, _ json.RawMessage) (int64, error) {
			if sessionID != 91 {
				t.Fatalf("session id = %d", sessionID)
			}
			savedQuestion, savedAnswer = question, answer
			return 101, nil
		},
	}
	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	body := res.Body.String()
	for _, want := range []string{
		"event: ready",
		`"state":"transcribing"`,
		`event: transcript`,
		`"text":"我最近总是着急"`,
		`"state":"retrieving_knowledge"`,
		`"state":"retrieving_theory"`,
		`event: text_delta`,
		`event: audio`,
		base64.StdEncoding.EncodeToString([]byte("audio:先停一下。")),
		`event: done`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE body missing %q:\n%s", want, body)
		}
	}
	if savedQuestion != "我最近总是着急" || savedAnswer != "先停一下。再感受身体。" {
		t.Fatalf("saved pair = %q / %q", savedQuestion, savedAnswer)
	}
}

func TestAppXinzhiliVoiceTurnDoesNotGeneratePsychologyForBlankTranscript(t *testing.T) {
	generated := false
	store := &xinzhiliEphemeralAudioStoreSpy{fakeAppChatStreamStore: newFakeAppChatStreamStore(), t: t}
	preferences := newFakeAppChatPreferenceStore()
	s := &Server{
		appChat:              store,
		userPreferences:      preferences,
		ragGen:               generatorFunc(func(context.Context, rag.GenerateInput) (string, error) { generated = true; return "不应生成", nil }),
		xinzhiliConfigLoader: func(context.Context) (modelconfig.XinzhiliVoiceConfig, error) { return readyXinzhiliVoiceConfig(), nil },
		xinzhiliMemberCheck:  func(context.Context, int64) error { return nil },
		xinzhiliTranscribe:   func(context.Context, []byte, string) (string, error) { return "……", nil },
		voiceAssetCreate: func(context.Context, uploadasset.CreateInput) (uploadasset.Asset, error) {
			t.Fatal("blank xinzhili transcript must not create an upload asset")
			return uploadasset.Asset{}, nil
		},
	}
	req := newXinzhiliMultipartRequest(t, []byte("silence"), 900)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if generated {
		t.Fatal("blank transcript must not call the conversation model")
	}
	if calls := store.saveCallCount(); calls != 0 {
		t.Fatalf("blank transcript saved %d text pairs, want 0", calls)
	}
	storedPreferences, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedPreferences) != 0 {
		t.Fatalf("blank transcript persisted preferences: %+v", storedPreferences)
	}
	if body := res.Body.String(); !strings.Contains(body, `"code":"speech_not_understood"`) || !strings.Contains(body, "没有听清") {
		t.Fatalf("unexpected SSE body: %s", body)
	}
}

func TestAppXinzhiliVoiceTurnPersistsOnlyTextPairAndPreferences(t *testing.T) {
	store := &xinzhiliEphemeralAudioStoreSpy{fakeAppChatStreamStore: newFakeAppChatStreamStore(), t: t}
	preferences := newFakeAppChatPreferenceStore()
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.appChat = store
	s.userPreferences = preferences
	s.xinzhiliSavePair = nil
	s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) {
		return "以后回答短一点，我最近总是着急", nil
	}
	s.voiceAssetCreate = func(context.Context, uploadasset.CreateInput) (uploadasset.Asset, error) {
		t.Fatal("xinzhili source audio must not create an upload asset")
		return uploadasset.Asset{}, nil
	}
	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if calls := store.saveCallCount(); calls != 1 {
		t.Fatalf("SavePair calls = %d, want 1", calls)
	}
	messages, err := store.ListMessages(context.Background(), 91)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Content != "以后回答短一点，我最近总是着急" || messages[1].Content != "先停一下。再感受身体。" {
		t.Fatalf("saved text pair = %+v", messages)
	}
	storedPreferences, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedPreferences) != 1 || storedPreferences[0].Slot != "length.detail_level" {
		t.Fatalf("saved preferences = %+v", storedPreferences)
	}
}

func readyXinzhiliVoiceConfig() modelconfig.XinzhiliVoiceConfig {
	return modelconfig.XinzhiliVoiceConfig{
		Enabled: true,
		ASR:     modelconfig.SpeechModelConfig{Provider: modelconfig.ProviderOpenAICompatible, APIBase: "https://speech.example/v1", APIKey: "asr", Model: "whisper-1"},
		TTS:     modelconfig.SpeechModelConfig{Provider: modelconfig.ProviderOpenAICompatible, APIBase: "https://speech.example/v1", APIKey: "tts", Model: "tts-1", Voice: "nova"},
	}
}

func newSuccessfulXinzhiliVoiceServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		chatTimeout:          30,
		ragGen:               xinzhiliTestGenerator{},
		xinzhiliConfigLoader: func(context.Context) (modelconfig.XinzhiliVoiceConfig, error) { return readyXinzhiliVoiceConfig(), nil },
		xinzhiliMemberCheck:  func(context.Context, int64) error { return nil },
		xinzhiliTranscribe:   func(context.Context, []byte, string) (string, error) { return "我最近总是着急", nil },
		xinzhiliSynthesize: func(_ context.Context, text string) ([]byte, string, error) {
			return []byte("audio:" + text), "audio/mpeg", nil
		},
		xinzhiliSession: func(context.Context, int64) (chat.Session, error) {
			return chat.Session{ID: 91, CardID: 5}, nil
		},
		xinzhiliRetrieveDocs: func(context.Context, string, int) ([]rag.Document, error) { return nil, nil },
		xinzhiliSavePair:     func(context.Context, int64, string, string, json.RawMessage) (int64, error) { return 101, nil },
	}
}

func newXinzhiliMultipartRequest(t *testing.T, audio []byte, durationMs int) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "turn.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(audio); err != nil {
		t.Fatal(err)
	}
	_ = writer.WriteField("durationMs", strconv.Itoa(durationMs))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/turns/stream", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func assertNoMultipartResidue(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "multipart-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("multipart temporary files remain: %v", matches)
	}
}

func assertMultipartResiduePresent(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "multipart-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected multipart parser to spill the audio file to disk")
	}
}

type xinzhiliTestGenerator struct{}

func (xinzhiliTestGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "先停一下。再感受身体。", nil
}
func (xinzhiliTestGenerator) GenerateStream(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if err := emit("先停一下。"); err != nil {
		return "", err
	}
	if err := emit("再感受身体。"); err != nil {
		return "", err
	}
	return "先停一下。再感受身体。", nil
}

type generatorFunc func(context.Context, rag.GenerateInput) (string, error)

func (f generatorFunc) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	return f(ctx, input)
}

type xinzhiliStreamingGeneratorFunc func(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error)

func (f xinzhiliStreamingGeneratorFunc) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "", errors.New("unexpected non-streaming generation")
}

func (f xinzhiliStreamingGeneratorFunc) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	return f(ctx, input, emit)
}

type failingXinzhiliSSEWriter struct {
	header    http.Header
	failEvent string
}

func (w *failingXinzhiliSSEWriter) Header() http.Header { return w.header }
func (w *failingXinzhiliSSEWriter) WriteHeader(int)     {}
func (w *failingXinzhiliSSEWriter) Flush()              {}
func (w *failingXinzhiliSSEWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "event: "+w.failEvent+"\n") {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

type xinzhiliEphemeralAudioStoreSpy struct {
	*fakeAppChatStreamStore
	t *testing.T
}

func (s *xinzhiliEphemeralAudioStoreSpy) SaveVoicePair(context.Context, int64, int64, int, string, string, json.RawMessage) (int64, int64, error) {
	s.t.Fatal("xinzhili source audio must not be stored as a voice pair")
	return 0, 0, nil
}

func (s *xinzhiliEphemeralAudioStoreSpy) SaveVoiceMessage(context.Context, int64, int64, int, string) (int64, error) {
	s.t.Fatal("xinzhili source audio must not be stored as a voice message")
	return 0, nil
}

func (s *xinzhiliEphemeralAudioStoreSpy) GetVoiceAudioAssetID(context.Context, int64, int64) (int64, error) {
	s.t.Fatal("xinzhili flow must not read a stored voice asset")
	return 0, nil
}

func (s *xinzhiliEphemeralAudioStoreSpy) GetVoiceTranscript(context.Context, int64, int64) (string, error) {
	s.t.Fatal("xinzhili flow must not read a stored voice transcript")
	return "", nil
}
