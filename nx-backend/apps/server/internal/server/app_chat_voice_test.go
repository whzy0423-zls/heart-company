package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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

type fakeVoiceChatStore struct {
	*fakeAppChatStreamStore
	transcript           string
	audioAssetID         int64
	ownedAssetID         int64
	audioLookupUserID    int64
	audioLookupMessageID int64
}

func (s *fakeVoiceChatStore) SaveVoicePair(_ context.Context, _ int64, audioAssetID int64, _ int, transcript, _ string, _ json.RawMessage) (int64, int64, error) {
	s.audioAssetID = audioAssetID
	s.transcript = transcript
	return 11, 12, nil
}

func (s *fakeVoiceChatStore) GetVoiceAudioAssetID(_ context.Context, appUserID, messageID int64) (int64, error) {
	s.audioLookupUserID = appUserID
	s.audioLookupMessageID = messageID
	if s.ownedAssetID == 0 {
		return 0, chat.ErrNotFound
	}
	return s.ownedAssetID, nil
}

type voiceChatGenerator struct {
	answer   string
	question string
}

func (g *voiceChatGenerator) Generate(_ context.Context, input rag.GenerateInput) (string, error) {
	g.question = input.Question
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
