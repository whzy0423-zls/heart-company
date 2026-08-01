package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

type fakeXinzhiliVoiceConfigStore struct {
	active          xinzhili.VoiceConfig
	activeFound     bool
	draft           xinzhili.VoiceConfig
	draftFound      bool
	saved           xinzhili.TTSConfig
	expectedVersion int64
}

func (f *fakeXinzhiliVoiceConfigStore) ReadActive(context.Context) (xinzhili.VoiceConfig, bool, error) {
	return f.active, f.activeFound, nil
}
func (f *fakeXinzhiliVoiceConfigStore) ReadDraft(context.Context) (xinzhili.VoiceConfig, bool, error) {
	return f.draft, f.draftFound, nil
}
func (f *fakeXinzhiliVoiceConfigStore) SaveDraft(_ context.Context, cfg xinzhili.TTSConfig, expectedVersion int64) (xinzhili.VoiceConfig, error) {
	f.saved, f.expectedVersion = cfg, expectedVersion
	return xinzhili.VoiceConfig{Version: expectedVersion + 1, Status: xinzhili.VoiceConfigStatusDraft, TTS: cfg, APIKeySet: cfg.APIKey != "", APIKeySuffix: "4321"}, nil
}
func (f *fakeXinzhiliVoiceConfigStore) Activate(context.Context, int64) (xinzhili.VoiceConfig, error) {
	return xinzhili.VoiceConfig{}, nil
}
func (f *fakeXinzhiliVoiceConfigStore) Deactivate(context.Context, int64) error { return nil }
func (f *fakeXinzhiliVoiceConfigStore) Restore(context.Context, int64, int64) (xinzhili.VoiceConfig, error) {
	return xinzhili.VoiceConfig{}, nil
}
func (f *fakeXinzhiliVoiceConfigStore) ScheduleRemoteDelete(context.Context, int64, string, string) (xinzhili.VoiceCleanupJob, error) {
	return xinzhili.VoiceCleanupJob{}, nil
}

func TestXinzhiliVoiceConfigGETMasksSecrets(t *testing.T) {
	store := &fakeXinzhiliVoiceConfigStore{
		activeFound: true,
		active: xinzhili.VoiceConfig{Version: 3, Status: xinzhili.VoiceConfigStatusActive, APIKeySet: true, APIKeySuffix: "abcd", TTS: xinzhili.TTSConfig{
			Provider: xinzhili.TTSProviderAliyunCosyVoice, Endpoint: "wss://dashscope.aliyuncs.com/api-ws/v1/inference", APIKey: "secret-abcd", GroupID: "workspace", Model: "cosyvoice-v3.5-plus", Voice: "voice-a", Format: "mp3",
		}},
	}
	s := &Server{xinzhiliVoiceConfig: store}
	res := httptest.NewRecorder()
	s.xinzhiliVoiceConfigHandler(res, httptest.NewRequest(http.MethodGet, "/api/xinzhili-voice-config", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if strings.Contains(res.Body.String(), "secret-abcd") || !strings.Contains(res.Body.String(), "\"apiKeySet\":true") || !strings.Contains(res.Body.String(), "\"apiKeySuffix\":\"abcd\"") {
		t.Fatalf("response did not mask secret correctly: %s", res.Body.String())
	}
}

func TestXinzhiliVoiceConfigPUTSavesDraftWithExpectedVersion(t *testing.T) {
	store := &fakeXinzhiliVoiceConfigStore{}
	s := &Server{xinzhiliVoiceConfig: store}
	body := `{"expectedVersion":7,"tts":{"provider":"aliyun-cosyvoice","endpoint":"wss://dashscope.aliyuncs.com/api-ws/v1/inference","apiKey":"dashscope-key-4321","groupId":"workspace","model":"cosyvoice-v3.5-plus","voice":"voice-b","format":"mp3"}}`
	res := httptest.NewRecorder()
	s.xinzhiliVoiceConfigHandler(res, httptest.NewRequest(http.MethodPut, "/api/xinzhili-voice-config", strings.NewReader(body)))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	if store.expectedVersion != 7 || store.saved.APIKey != "dashscope-key-4321" || store.saved.GroupID != "workspace" {
		t.Fatalf("save not called correctly: expected=%d saved=%+v", store.expectedVersion, store.saved)
	}
	var decoded struct {
		Data xinzhiliVoiceConfigView `json:"data"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Data.Draft.TTS.APIKey != "" || !decoded.Data.Draft.TTS.APIKeySet {
		t.Fatalf("saved response should be masked: %+v", decoded.Data.Draft.TTS)
	}
}
