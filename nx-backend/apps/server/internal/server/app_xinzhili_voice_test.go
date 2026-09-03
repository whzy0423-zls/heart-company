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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
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
	if savedQuestion != "我最近总是着急" || savedAnswer != "先停一下。\n再感受身体。" {
		t.Fatalf("saved pair = %q / %q", savedQuestion, savedAnswer)
	}
}

func TestAppXinzhiliVoiceTurnAddsNaturalVoiceResponseDirective(t *testing.T) {
	var captured rag.GenerateInput
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		captured = input
		if err := emit("我在听，我们慢慢说。"); err != nil {
			return "", err
		}
		return "我在听，我们慢慢说。", nil
	})

	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()
	s.appXinzhiliVoiceTurnStream(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("voice turn failed: %d %s", res.Code, res.Body.String())
	}
	found := false
	for _, directive := range captured.CurrentDirectives {
		if directive == xinzhili.DefaultVoiceResponseDirective {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("current directives=%q want natural voice directive", captured.CurrentDirectives)
	}
}

func TestServerRegistersAppXinzhiliLegacyTurnStreamRoute(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `s.mux.HandleFunc("/api/app/xinzhili/turns/stream"`) {
		t.Fatal("app xinzhili legacy turn stream route is not registered")
	}
}

func TestMergeXinzhiliRAGDocumentsKeepsKnowledgeBeforeTheoryAndDedupes(t *testing.T) {
	docs := mergeXinzhiliRAGDocuments(
		[]rag.Document{{ID: "kb-1", Title: "知识", Content: "知识内容"}, {ID: "shared", Title: "重复", Content: "知识版本"}},
		[]rag.Document{{ID: "shared", Title: "重复", Content: "理论版本"}, {ID: "theory:1", Title: "理论", Content: "理论内容"}},
	)
	if len(docs) != 3 || docs[0].ID != "kb-1" || docs[1].ID != "shared" || docs[2].ID != "theory:1" {
		t.Fatalf("merged docs = %+v", docs)
	}
}

func TestAppXinzhiliVoiceTurnAddsTheoryDocumentsToModelInput(t *testing.T) {
	var captured rag.GenerateInput
	var savedSources json.RawMessage
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) { return "什么是九型", nil }
	s.xinzhiliRetrieveDocs = func(_ context.Context, question string, topK int) ([]rag.Document, error) {
		if question != "什么是九型" || topK != 8 {
			t.Fatalf("knowledge retrieval = %q/%d, want transcript/8", question, topK)
		}
		return []rag.Document{{ID: "kb-nine-types", Title: "知识库：九型基础", Content: "九型人格用于理解九种性格模式与核心动机。", Tags: []string{"九型", "基础"}}}, nil
	}
	s.xinzhiliRetrieveTheoryDocs = func(_ context.Context, question string, topK int, minScore float64) ([]rag.Document, error) {
		if question != "什么是九型" || topK != 6 || minScore != 0.2 {
			t.Fatalf("theory retrieval = %q/%d/%v, want transcript/6/0.2", question, topK, minScore)
		}
		return []rag.Document{{ID: "theory:11", Title: "理论库：九型是观察地图", Content: "九型是观察地图，不是身份标签。", Tags: []string{"九型", "理论"}}}, nil
	}
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		captured = input
		if err := emit("九型是理解性格模式的地图。"); err != nil {
			return "", err
		}
		return "九型是理解性格模式的地图。", nil
	})
	s.xinzhiliSavePair = func(_ context.Context, _ int64, _, _ string, sources json.RawMessage) (int64, error) {
		savedSources = append(json.RawMessage(nil), sources...)
		return 101, nil
	}

	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	body := res.Body.String()
	if !strings.Contains(body, `event: done`) {
		t.Fatalf("turn did not complete: status=%d body=%s", res.Code, body)
	}
	if len(captured.Sources) < 2 {
		t.Fatalf("model sources = %+v, want knowledge plus theory", captured.Sources)
	}
	if captured.Sources[0].ID != "kb-nine-types" {
		t.Fatalf("first source = %+v, want knowledge document first", captured.Sources)
	}
	if !containsRAGSourceID(captured.Sources, "theory:11") {
		t.Fatalf("model sources missing theory document: %+v", captured.Sources)
	}
	if !strings.Contains(string(savedSources), "kb-nine-types") || !strings.Contains(string(savedSources), "theory:11") {
		t.Fatalf("persisted sources = %s, want knowledge and theory", savedSources)
	}
}

func TestAppXinzhiliVoiceTurnUsesLayeredKnowledgeForPrimaryCard(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	resolver := &layeredKnowledgeResolver{mainType: 5, revision: 7}
	searcher := newLayeredKnowledgeSearcher()
	generator := &layeredKnowledgeGenerator{}
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.appChat = store
	s.xinzhiliSavePair = nil
	s.xinzhiliTranscribe = func(context.Context, []byte, string) (string, error) { return layeredKnowledgeQuestion, nil }
	s.appKnowledge = appknowledge.NewCoordinator(resolver, searcher, searcher)
	s.ragGen = generator
	s.appChatProfilesForCardOverride = func(_ context.Context, _, cardID int64) (rag.UserProfile, rag.ConversationCard) {
		return rag.UserProfile{MainType: 9}, rag.ConversationCard{MainType: 5, Name: fmt.Sprintf("primary-%d", cardID)}
	}

	request := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	request = request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()
	s.appXinzhiliVoiceTurnStream(response, request)

	if !strings.Contains(response.Body.String(), "event: done") || strings.Contains(response.Body.String(), `"code":"save_failed"`) {
		t.Fatalf("xinzhili layered turn failed: %s", response.Body.String())
	}
	if resolver.calls != 1 || resolver.lastSessionID != 91 || resolver.lastCardID != 5 {
		t.Fatalf("xinzhili resolution calls=%d session/card=%d/%d", resolver.calls, resolver.lastSessionID, resolver.lastCardID)
	}
	trace := store.singleTrace(t)
	if trace.EnneagramType == nil || *trace.EnneagramType != 5 || trace.CardRevision != 7 {
		t.Fatalf("xinzhili trace = %+v", trace)
	}
	assertLayeredTrace(t, trace.LayerHits, "type-5")
	assertSourceIDs(t, generator.lastSources(), "public", "theory", "type-5")
}

func TestAppXinzhiliVoiceTurnFiltersRestrictedTermsBeforeTextAudioAndPersistence(t *testing.T) {
	var savedAnswer string
	var synthesized []string
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		chunks := []string{"当前通过 C o d e x C L I 运行。", "你可以继续描述困扰。", "再慢慢说清楚。"}
		for _, chunk := range chunks {
			if err := emit(chunk); err != nil {
				return "", err
			}
		}
		return strings.Join(chunks, ""), nil
	})
	s.xinzhiliSynthesize = func(_ context.Context, text string) ([]byte, string, error) {
		synthesized = append(synthesized, text)
		return []byte("audio:" + text), "audio/mpeg", nil
	}
	s.xinzhiliSavePair = func(_ context.Context, _ int64, _, answer string, _ json.RawMessage) (int64, error) {
		savedAnswer = answer
		return 101, nil
	}

	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()
	s.appXinzhiliVoiceTurnStream(res, req)

	body := res.Body.String()
	for _, forbidden := range []string{"Codex", "C o d e x", "CLI", "C L I"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("voice stream leaked %q: %s", forbidden, body)
		}
		for _, text := range synthesized {
			if strings.Contains(text, forbidden) {
				t.Fatalf("tts leaked %q: %#v", forbidden, synthesized)
			}
		}
	}
	want := "你可以继续描述困扰。\n再慢慢说清楚。"
	if savedAnswer != want {
		t.Fatalf("saved answer = %q, want %q", savedAnswer, want)
	}
	if !strings.Contains(body, `"answer":"你可以继续描述困扰。\n再慢慢说清楚。"`) {
		t.Fatalf("done event was not cleaned and formatted: %s", body)
	}
}

func TestAppXinzhiliVoiceTurnBatchesTinySpeechFragmentsBeforeTTS(t *testing.T) {
	var synthesized []string
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		for _, delta := range []string{"嗯。", "好。", "我们先慢慢来。"} {
			if err := emit(delta); err != nil {
				return "", err
			}
		}
		return "嗯。好。我们先慢慢来。", nil
	})
	s.xinzhiliSynthesize = func(_ context.Context, text string) ([]byte, string, error) {
		synthesized = append(synthesized, text)
		return []byte("audio:" + text), "audio/mpeg", nil
	}

	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("voice turn failed: %d %s", res.Code, res.Body.String())
	}
	want := []string{"嗯。好。我们先慢慢来。"}
	if len(synthesized) != len(want) || synthesized[0] != want[0] {
		t.Fatalf("synthesized chunks = %#v, want %#v", synthesized, want)
	}
	if !strings.Contains(res.Body.String(), base64.StdEncoding.EncodeToString([]byte("audio:"+want[0]))) {
		t.Fatalf("SSE body missing batched audio:\n%s", res.Body.String())
	}
}

func TestAppXinzhiliVoiceTurnStrictChineseOutputRewritesEnglishDriftAndDigits(t *testing.T) {
	var savedAnswer string
	var synthesizedMu sync.Mutex
	var synthesized []string
	answer := "好的。OK，我们做1 2 3次。不要 worry。"
	want := "好的。\n好，我们做一二三次。\n不要担心。"
	s := newSuccessfulXinzhiliVoiceServer(t)
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		if err := emit(answer); err != nil {
			return "", err
		}
		return answer, nil
	})
	s.xinzhiliSynthesize = func(_ context.Context, text string) ([]byte, string, error) {
		synthesizedMu.Lock()
		synthesized = append(synthesized, text)
		synthesizedMu.Unlock()
		assertNoXinzhiliVoiceASCII(t, text)
		return []byte("audio:" + text), "audio/mpeg", nil
	}
	s.xinzhiliSavePair = func(_ context.Context, _ int64, _, answer string, _ json.RawMessage) (int64, error) {
		savedAnswer = answer
		return 101, nil
	}

	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("voice turn failed: %d %s", res.Code, res.Body.String())
	}
	if savedAnswer != want {
		t.Fatalf("saved answer = %q, want %q", savedAnswer, want)
	}
	synthesizedMu.Lock()
	synthesizedCount := len(synthesized)
	synthesizedMu.Unlock()
	if synthesizedCount == 0 {
		t.Fatal("expected at least one synthesized chunk")
	}
	body := res.Body.String()
	for _, forbidden := range []string{"OK", "worry"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("SSE body still contains English drift %q:\n%s", forbidden, body)
		}
	}
	for _, required := range []string{
		`"content":"好的。"`,
		`"content":"\n好，我们做一二三次。"`,
		`"content":"\n不要担心。"`,
		`"answer":"好的。\n好，我们做一二三次。\n不要担心。"`,
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("SSE body missing normalized %s:\n%s", required, body)
		}
	}
}

func assertNoXinzhiliVoiceASCII(t *testing.T, text string) {
	t.Helper()
	for _, r := range text {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			t.Fatalf("text still contains ASCII rune %q in %q", r, text)
		}
	}
}

func TestShouldNormalizeXinzhiliVoiceOutputToChinese(t *testing.T) {
	tests := []struct {
		name       string
		transcript string
		want       bool
	}{
		{name: "default Chinese with numeric tokens", transcript: "我们先做1 2 3次呼吸", want: true},
		{name: "negative English mention still enforces Chinese", transcript: "不要英文，中文回答", want: true},
		{name: "explicit English word request", transcript: "请用英文说 Apple 这个单词", want: false},
		{name: "translation to English request", transcript: "把这句话翻译成英文", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldNormalizeXinzhiliVoiceOutputToChinese(tt.transcript); got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeXinzhiliVoiceTranscriptLocksNineTypeHomophones(t *testing.T) {
	tests := []struct {
		transcript string
		want       string
	}{
		{transcript: "什么是九型人格", want: "什么是九型人格"},
		{transcript: "什么是九形人格", want: "什么是九型人格"},
		{transcript: "什么是九星人格", want: "什么是九型人格"},
		{transcript: "我想了解就行人格", want: "我想了解九型人格"},
		{transcript: "什么是就行", want: "什么是九型"},
		{transcript: "九行测试怎么做", want: "九型测试怎么做"},
		{transcript: "这样就行了", want: "这样就行了"},
	}
	for _, tt := range tests {
		if got := normalizeXinzhiliVoiceTranscript(tt.transcript); got != tt.want {
			t.Fatalf("normalizeXinzhiliVoiceTranscript(%q) = %q, want %q", tt.transcript, got, tt.want)
		}
	}
}

func TestAppXinzhiliVoiceTurnSynthesizesParallelTTSAndStreamsAudioInOrder(t *testing.T) {
	firstChunk := "第一段需要稳定输出。"
	secondChunk := "第二段也要稳定输出。"
	started := make(chan string, 2)
	releaseFirst := make(chan struct{})
	releaseFirstClosed := false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer func() {
		if !releaseFirstClosed {
			close(releaseFirst)
		}
	}()

	s := newSuccessfulXinzhiliVoiceServer(t)
	s.ragGen = xinzhiliStreamingGeneratorFunc(func(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
		for _, delta := range []string{firstChunk, secondChunk} {
			if err := emit(delta); err != nil {
				return "", err
			}
		}
		return firstChunk + secondChunk, nil
	})
	s.xinzhiliSynthesize = func(ctx context.Context, text string) ([]byte, string, error) {
		started <- text
		if text == firstChunk {
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
		}
		return []byte("audio:" + text), "audio/mpeg", nil
	}

	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(ctx, auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		s.appXinzhiliVoiceTurnStream(res, req)
	}()

	startedChunks := []string{
		waitForXinzhiliTTSStart(t, started, time.Second),
		waitForXinzhiliTTSStart(t, started, 250*time.Millisecond),
	}
	if !xinzhiliStartedChunk(startedChunks, firstChunk) || !xinzhiliStartedChunk(startedChunks, secondChunk) {
		cancel()
		t.Fatalf("TTS chunks did not both start while first chunk was still synthesizing: got %#v", startedChunks)
	}
	close(releaseFirst)
	releaseFirstClosed = true

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("handler did not finish after parallel TTS was released")
	}

	body := res.Body.String()
	firstAudio := base64.StdEncoding.EncodeToString([]byte("audio:" + firstChunk))
	secondAudio := base64.StdEncoding.EncodeToString([]byte("audio:" + secondChunk))
	firstPos := strings.Index(body, firstAudio)
	secondPos := strings.Index(body, secondAudio)
	if firstPos < 0 || secondPos < 0 {
		t.Fatalf("SSE body missing audio chunks first=%d second=%d:\n%s", firstPos, secondPos, body)
	}
	if firstPos > secondPos {
		t.Fatalf("audio streamed out of order: first at %d, second at %d\n%s", firstPos, secondPos, body)
	}
}

func waitForXinzhiliTTSStart(t *testing.T, started <-chan string, timeout time.Duration) string {
	t.Helper()
	select {
	case text := <-started:
		return text
	case <-time.After(timeout):
		return ""
	}
}

func xinzhiliStartedChunk(started []string, want string) bool {
	for _, got := range started {
		if got == want {
			return true
		}
	}
	return false
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
	preferenceApplyCalls := 0
	preferences.onApply = func() { preferenceApplyCalls++ }
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
	if len(messages) != 2 || messages[0].Content != "以后回答短一点，我最近总是着急" || messages[1].Content != "先停一下。\n再感受身体。" {
		t.Fatalf("saved text pair = %+v", messages)
	}
	for _, message := range messages {
		if message.MessageType == "voice" || message.AudioAssetID != 0 || message.AudioDurationMs != 0 || message.AudioURL != "" || message.Transcript != "" {
			t.Fatalf("xinzhili message contains voice-only storage fields: %+v", message)
		}
	}
	if preferenceApplyCalls != 1 {
		t.Fatalf("persistAppChatPreferences Apply calls = %d, want 1", preferenceApplyCalls)
	}
	storedPreferences, err := preferences.List(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedPreferences) != 1 || storedPreferences[0].Slot != "length.detail_level" {
		t.Fatalf("saved preferences = %+v", storedPreferences)
	}

	var ordinaryInputs []rag.GenerateInput
	ordinaryGenerator := &controlledAppChatStreamingGenerator{
		generateStream: func(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
			ordinaryInputs = append(ordinaryInputs, input)
			if err := emit("普通聊天回答"); err != nil {
				return "", err
			}
			return "普通聊天回答", nil
		},
	}
	ordinary := newAppChatStreamServer(store, ordinaryGenerator)
	ordinary.userPreferences = preferences
	performAppChatPreferenceRequest(t, ordinary, 7, 42, "换到普通聊天继续")
	performAppChatPreferenceRequest(t, ordinary, 8, 43, "另一个用户的问题")

	if len(ordinaryInputs) != 2 {
		t.Fatalf("ordinary chat model inputs = %d, want 2", len(ordinaryInputs))
	}
	if strings.Join(ordinaryInputs[0].UserPreferences, "|") != "回答简短，避免长篇大论" {
		t.Fatalf("ordinary chat did not load xinzhili preference: %+v", ordinaryInputs[0])
	}
	if len(ordinaryInputs[1].UserPreferences) != 0 {
		t.Fatalf("xinzhili preference leaked to another user: %+v", ordinaryInputs[1])
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

func containsRAGSourceID(sources []rag.Source, id string) bool {
	for _, source := range sources {
		if source.ID == id {
			return true
		}
	}
	return false
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
