package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

func TestVoiceChatRejectsInvalidDurationBeforeASR(t *testing.T) {
	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	s := newVoiceChatTestServer(store, nil)
	body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", "200")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "0.8") {
		t.Fatalf("expected duration validation, got %d %s", response.Code, response.Body.String())
	}
}

func TestVoiceChatReturnsUnavailableWhenASRIsNotConfigured(t *testing.T) {
	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	s := newVoiceChatTestServer(store, nil)
	body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", "1200")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "ASR_API_KEY") {
		t.Fatalf("expected actionable ASR unavailable response, got %d %s", response.Code, response.Body.String())
	}
}

func TestVoiceChatPersistsAudioAndHidesTranscriptFromResponse(t *testing.T) {
	const hiddenTranscript = "孩子最近不愿意沟通"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": hiddenTranscript})
	}))
	defer upstream.Close()
	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	generator := &voiceChatGenerator{answer: "可以先从倾听开始"}
	s := newVoiceChatTestServer(store, generator)
	s.env.ASR = config.ASRConfig{APIBase: upstream.URL, APIKey: "test-key", Model: "whisper-1", TimeoutSeconds: 3}
	s.voiceAssetCreate = func(_ context.Context, input uploadasset.CreateInput) (uploadasset.Asset, error) {
		if string(input.Data) != "audio" || input.ContentType != "audio/aac" {
			t.Fatalf("unexpected persisted audio: %+v", input)
		}
		return uploadasset.Asset{ID: 88}, nil
	}
	body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", "3200")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("voice chat failed: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), hiddenTranscript) || strings.Contains(response.Body.String(), "transcript") || strings.Contains(response.Body.String(), "audioAssetId") {
		t.Fatalf("response leaked hidden voice metadata: %s", response.Body.String())
	}
	for _, expected := range []string{`"messageType":"voice"`, `"audioDurationMs":3200`, `"audioUrl":"/api/app/chat/messages/11/audio"`, "可以先从倾听开始"} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("response missing %s: %s", expected, response.Body.String())
		}
	}
	if store.transcript != hiddenTranscript || generator.question != hiddenTranscript || store.audioAssetID != 88 {
		t.Fatalf("hidden transcript flow mismatch: store=%q generator=%q asset=%d", store.transcript, generator.question, store.audioAssetID)
	}
}

func TestVoiceChatCleansProductImplementationMetaBeforePersistingAndReturning(t *testing.T) {
	const transcript = "推荐三道菜"
	store, response := runVoiceChatAnswerTest(t, transcript, "针对当前 App 端，页面实现方案要先统一。推荐：番茄炒蛋、青椒肉丝、可乐鸡翅。")

	if response.Code != http.StatusOK {
		t.Fatalf("voice chat failed: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data voiceChatResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	want := "推荐：番茄炒蛋、青椒肉丝、可乐鸡翅。"
	if payload.Data.Answer.Answer != want {
		t.Fatalf("HTTP answer = %q, want %q", payload.Data.Answer.Answer, want)
	}
	if store.assistantAnswer != want {
		t.Fatalf("persisted assistant answer = %q, want %q", store.assistantAnswer, want)
	}
	if payload.Data.UserMessage.Content != "" {
		t.Fatalf("response leaked ASR transcript through user message: %q", payload.Data.UserMessage.Content)
	}
	if strings.Contains(response.Body.String(), transcript) || strings.Contains(response.Body.String(), "App 端") {
		t.Fatalf("response leaked transcript or product meta: %s", response.Body.String())
	}
}

func TestVoiceChatUsesNeutralFallbackWhenAnswerIsOnlyProductImplementationMeta(t *testing.T) {
	store, response := runVoiceChatAnswerTest(t, "推荐三道菜", "App 端需要先处理页面状态。基础框架建议统一走后台接口。")

	if response.Code != http.StatusOK {
		t.Fatalf("voice chat failed: %d %s", response.Code, response.Body.String())
	}
	const want = "请再具体说一点，我会直接回答。"
	if store.assistantAnswer != want || !strings.Contains(response.Body.String(), want) {
		t.Fatalf("neutral fallback mismatch: persisted=%q response=%s", store.assistantAnswer, response.Body.String())
	}
}

func TestVoiceChatPreservesImplementationAnswerForExplicitTechnicalQuestion(t *testing.T) {
	const answer = "服务器可以通过容器部署，并由接口暴露健康检查。"
	store, response := runVoiceChatAnswerTest(t, "服务器怎么部署？", answer)

	if response.Code != http.StatusOK {
		t.Fatalf("voice chat failed: %d %s", response.Code, response.Body.String())
	}
	if store.assistantAnswer != answer || !strings.Contains(response.Body.String(), answer) {
		t.Fatalf("technical answer was altered: persisted=%q response=%s", store.assistantAnswer, response.Body.String())
	}
}

func TestVoiceChatModelIdentityPersistsFixedReplyWithoutGeneration(t *testing.T) {
	const identityQuestion = "你用的是什么模型？"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": identityQuestion})
	}))
	defer upstream.Close()
	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	generator := &voiceChatGenerator{answer: "我是某个模型，通过 Codex CLI 提供帮助。"}
	s := newVoiceChatTestServer(store, generator)
	s.env.ASR = config.ASRConfig{APIBase: upstream.URL, APIKey: "test-key", Model: "whisper-1", TimeoutSeconds: 3}
	s.voiceAssetCreate = func(_ context.Context, _ uploadasset.CreateInput) (uploadasset.Asset, error) {
		return uploadasset.Asset{ID: 88}, nil
	}
	body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", "2100")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("voice chat failed: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Answer rag.Answer `json:"answer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Answer.Answer != rag.ModelIdentityReply {
		t.Fatalf("voice identity answer = %q, want %q", payload.Data.Answer.Answer, rag.ModelIdentityReply)
	}
	if len(payload.Data.Answer.Sources) != 0 || len(payload.Data.Answer.Suggestions) != 0 {
		t.Fatalf("voice identity metadata must be empty: %+v", payload.Data.Answer)
	}
	if store.transcript != identityQuestion || store.assistantAnswer != rag.ModelIdentityReply {
		t.Fatalf("persisted voice pair mismatch: transcript=%q answer=%q", store.transcript, store.assistantAnswer)
	}
	if string(store.assistantSources) != "[]" {
		t.Fatalf("persisted identity sources = %s, want []", store.assistantSources)
	}
	if generator.calls != 0 {
		t.Fatalf("voice identity called generator %d times", generator.calls)
	}
}

func TestVoiceChatPassesSecondaryCardContextToGenerator(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "她最近为什么总是迎合别人？"})
	}))
	defer upstream.Close()
	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	generator := &voiceChatGenerator{answer: "可以先确认她真正想要什么"}
	s := newVoiceChatTestServer(store, generator)
	s.env.ASR = config.ASRConfig{APIBase: upstream.URL, APIKey: "test-key", Model: "whisper-1", TimeoutSeconds: 3}
	s.appChatProfilesForCardOverride = func(_ context.Context, userID, cardID int64) (rag.UserProfile, rag.ConversationCard) {
		if userID != 7 || cardID != 0 {
			t.Fatalf("unexpected profile lookup: user=%d card=%d", userID, cardID)
		}
		return rag.UserProfile{Nickname: "小王", MainType: 6}, rag.ConversationCard{
			CardType: "secondary",
			Name:     "妈妈",
			Relation: "家人",
			MainType: 2,
			WingType: 1,
			Profile:  `{"primaryMotivation":"希望被需要"}`,
		}
	}
	s.voiceAssetCreate = func(_ context.Context, input uploadasset.CreateInput) (uploadasset.Asset, error) {
		return uploadasset.Asset{ID: 88}, nil
	}
	body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", "3200")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("voice chat failed: %d %s", response.Code, response.Body.String())
	}
	card := generator.input.ConversationCard
	if card.CardType != "secondary" || card.Name != "妈妈" || card.Relation != "家人" || card.MainType != 2 || card.WingType != 1 || !strings.Contains(card.Profile, "希望被需要") {
		t.Fatalf("secondary conversation card missing from voice generation: %+v", card)
	}
	if generator.input.Tier != "basic" {
		t.Fatalf("voice conversation tier = %q, want basic", generator.input.Tier)
	}
}

func TestVoiceChatPersistsPunctuationOnlySilentAudioWithoutAI(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "."})
	}))
	defer upstream.Close()
	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	generator := &voiceChatGenerator{answer: "不应该生成回答"}
	assetCreateCalls := 0
	s := newVoiceChatTestServer(store, generator)
	s.env.ASR = config.ASRConfig{APIBase: upstream.URL, APIKey: "test-key", Model: "whisper-1", TimeoutSeconds: 3}
	s.voiceAssetCreate = func(_ context.Context, _ uploadasset.CreateInput) (uploadasset.Asset, error) {
		assetCreateCalls++
		return uploadasset.Asset{ID: 88}, nil
	}
	body, contentType := voiceChatMultipartBody(t, "silent.aac", "audio/aac", "audio", "2500")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("expected silent voice success, got %d %s", response.Code, response.Body.String())
	}
	for _, expected := range []string{
		`"messageType":"voice"`,
		`"audioUrl":"/api/app/chat/messages/11/audio"`,
		`"messageId":0`,
		"我没有听清你说了什么",
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("silent voice response missing %q: %s", expected, response.Body.String())
		}
	}
	if generator.calls != 0 || assetCreateCalls != 1 || store.audioAssetID != 88 || store.silentVoiceSaves != 1 {
		t.Fatalf("silent voice reached downstream: generator=%d assets=%d storedAsset=%d", generator.calls, assetCreateCalls, store.audioAssetID)
	}
}

func TestVoiceAudioRequiresOwnershipAndReturnsBytes(t *testing.T) {
	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore(), ownedAssetID: 88}
	s := newVoiceChatTestServer(store, nil)
	s.voiceAssetFind = func(_ context.Context, id int64) (uploadasset.Asset, error) {
		if id != 88 {
			t.Fatalf("asset id = %d", id)
		}
		return uploadasset.Asset{ID: id, ContentType: "audio/aac", Data: []byte("saved-audio")}, nil
	}
	req := httptest.NewRequest(http.MethodGet, "/api/app/chat/messages/11/audio", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "audio/aac" || response.Body.String() != "saved-audio" {
		t.Fatalf("unexpected audio response: %d %s %q", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	if store.audioLookupUserID != 7 || store.audioLookupMessageID != 11 {
		t.Fatalf("ownership lookup mismatch: user=%d message=%d", store.audioLookupUserID, store.audioLookupMessageID)
	}
}

func TestVoiceTranscriptReturnsStoredText(t *testing.T) {
	store := &fakeVoiceChatStore{
		fakeAppChatStreamStore: newFakeAppChatStreamStore(),
		voiceTranscript:        "孩子最近不愿意沟通",
	}
	s := newVoiceChatTestServer(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/app/chat/messages/11/transcript", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected transcript response: %d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Text string `json:"text"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Data.Text != "孩子最近不愿意沟通" {
		t.Fatalf("transcript text = %q", payload.Data.Text)
	}
	if store.transcriptLookupUserID != 7 || store.transcriptLookupMessageID != 11 {
		t.Fatalf("ownership lookup mismatch: user=%d message=%d", store.transcriptLookupUserID, store.transcriptLookupMessageID)
	}
}

func TestVoiceTranscriptReturnsEmptyText(t *testing.T) {
	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	s := newVoiceChatTestServer(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/app/chat/messages/11/transcript", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"text":""`) {
		t.Fatalf("unexpected empty transcript response: %d %s", response.Code, response.Body.String())
	}
}

func TestVoiceTranscriptMapsNotFoundTo404(t *testing.T) {
	store := &fakeVoiceChatStore{
		fakeAppChatStreamStore: newFakeAppChatStreamStore(),
		voiceTranscriptErr:     chat.ErrNotFound,
	}
	s := newVoiceChatTestServer(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/app/chat/messages/11/transcript", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d %s", response.Code, response.Body.String())
	}
}

func TestVoiceTranscriptMapsStoreFailureTo500(t *testing.T) {
	store := &fakeVoiceChatStore{
		fakeAppChatStreamStore: newFakeAppChatStreamStore(),
		voiceTranscriptErr:     errors.New("database unavailable"),
	}
	s := newVoiceChatTestServer(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/app/chat/messages/11/transcript", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d %s", response.Code, response.Body.String())
	}
}

func TestVoiceTranscriptRejectsInvalidMessageID(t *testing.T) {
	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	s := newVoiceChatTestServer(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/app/chat/messages/not-a-number/transcript", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d %s", response.Code, response.Body.String())
	}
}

func TestVoiceTranscriptRouteRejectsUnsupportedMethod(t *testing.T) {
	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	s := newVoiceChatTestServer(store, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/messages/11/transcript", nil)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatMessageRouter(response, req)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d %s", response.Code, response.Body.String())
	}
	if store.transcriptLookupCalls != 0 {
		t.Fatalf("transcript store called %d times", store.transcriptLookupCalls)
	}
}

type fakeVoiceChatStore struct {
	*fakeAppChatStreamStore
	transcript                string
	audioAssetID              int64
	ownedAssetID              int64
	audioLookupUserID         int64
	audioLookupMessageID      int64
	voiceTranscript           string
	voiceTranscriptErr        error
	transcriptLookupUserID    int64
	transcriptLookupMessageID int64
	transcriptLookupCalls     int
	silentVoiceSaves          int
	assistantAnswer           string
	assistantSources          json.RawMessage
}

func (s *fakeVoiceChatStore) SaveVoicePair(_ context.Context, _ int64, audioAssetID int64, _ int, transcript, answer string, sources json.RawMessage) (int64, int64, error) {
	s.audioAssetID = audioAssetID
	s.transcript = transcript
	s.assistantAnswer = answer
	s.assistantSources = append(json.RawMessage(nil), sources...)
	return 11, 12, nil
}

func (s *fakeVoiceChatStore) SaveVoiceMessage(_ context.Context, _ int64, audioAssetID int64, _ int, transcript string) (int64, error) {
	s.audioAssetID = audioAssetID
	s.transcript = transcript
	s.silentVoiceSaves++
	return 11, nil
}

func (s *fakeVoiceChatStore) GetVoiceAudioAssetID(_ context.Context, appUserID, messageID int64) (int64, error) {
	s.audioLookupUserID = appUserID
	s.audioLookupMessageID = messageID
	if s.ownedAssetID == 0 {
		return 0, chat.ErrNotFound
	}
	return s.ownedAssetID, nil
}

func (s *fakeVoiceChatStore) GetVoiceTranscript(_ context.Context, appUserID, messageID int64) (string, error) {
	s.transcriptLookupCalls++
	s.transcriptLookupUserID = appUserID
	s.transcriptLookupMessageID = messageID
	return s.voiceTranscript, s.voiceTranscriptErr
}

type voiceChatGenerator struct {
	answer   string
	question string
	calls    int
	input    rag.GenerateInput
}

func (g *voiceChatGenerator) Generate(_ context.Context, input rag.GenerateInput) (string, error) {
	g.calls++
	g.question = input.Question
	g.input = input
	return g.answer, nil
}

func newVoiceChatTestServer(store appChatStore, generator rag.Generator) *Server {
	return &Server{
		env:         config.Env{},
		appChat:     store,
		ragGen:      generator,
		ragDocs:     &emptyAppChatRAGStore{},
		chatTimeout: 5 * time.Second,
		db:          (*sql.DB)(nil),
	}
}

func runVoiceChatAnswerTest(t *testing.T, transcript, generatedAnswer string) (*fakeVoiceChatStore, *httptest.ResponseRecorder) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": transcript})
	}))
	t.Cleanup(upstream.Close)
	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	s := newVoiceChatTestServer(store, &voiceChatGenerator{answer: generatedAnswer})
	s.env.ASR = config.ASRConfig{APIBase: upstream.URL, APIKey: "test-key", Model: "whisper-1", TimeoutSeconds: 3}
	s.voiceAssetCreate = func(_ context.Context, _ uploadasset.CreateInput) (uploadasset.Asset, error) {
		return uploadasset.Asset{ID: 88}, nil
	}
	body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", "2200")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)
	return store, response
}

func voiceChatMultipartBody(t *testing.T, filename, contentType, content, duration string) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {`form-data; name="audio"; filename="` + filename + `"`},
		"Content-Type":        {contentType},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("durationMs", duration); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}
