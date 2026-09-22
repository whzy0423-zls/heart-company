package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/voicebroadcastconfig"
)

type memoryVoiceBroadcastStore struct {
	cfg       voicebroadcastconfig.Config
	found     bool
	readErr   error
	updateErr error
	healthErr error
}

func (s *memoryVoiceBroadcastStore) Read(context.Context) (voicebroadcastconfig.Config, bool, error) {
	if s.readErr != nil {
		return voicebroadcastconfig.Config{}, false, s.readErr
	}
	if !s.found {
		return voicebroadcastconfig.DefaultConfig(), false, nil
	}
	return s.cfg, true, nil
}

func (s *memoryVoiceBroadcastStore) Update(_ context.Context, input voicebroadcastconfig.UpdateInput, expectedVersion int64) (voicebroadcastconfig.Config, error) {
	if s.updateErr != nil {
		return voicebroadcastconfig.Config{}, s.updateErr
	}
	if (!s.found && expectedVersion != 0) || (s.found && expectedVersion != s.cfg.Version) {
		return voicebroadcastconfig.Config{}, voicebroadcastconfig.ErrConflict
	}
	next := s.cfg
	if !s.found {
		next = voicebroadcastconfig.DefaultConfig()
		next.Version = 1
	} else {
		next.Version++
	}
	if input.Provider != "" {
		next.Provider = input.Provider
	}
	if input.Region != "" {
		next.Region = input.Region
	}
	if input.WorkspaceID != "" {
		next.WorkspaceID = input.WorkspaceID
	}
	if input.Model != "" {
		next.Model = input.Model
	}
	if input.CurrentVoice != "" {
		next.CurrentVoice = input.CurrentVoice
	}
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
	}
	s.cfg, s.found = next, true
	return next, nil
}

func (s *memoryVoiceBroadcastStore) RecordHealth(_ context.Context, expectedVersion int64, health voicebroadcastconfig.Health) (voicebroadcastconfig.Config, error) {
	if s.healthErr != nil {
		return voicebroadcastconfig.Config{}, s.healthErr
	}
	if !s.found {
		if expectedVersion != 0 {
			return voicebroadcastconfig.Config{}, voicebroadcastconfig.ErrConflict
		}
	} else if expectedVersion != s.cfg.Version {
		return voicebroadcastconfig.Config{}, voicebroadcastconfig.ErrConflict
	}
	s.cfg.Version++
	s.cfg.Health = health
	s.found = true
	return s.cfg, nil
}

func TestAdminVoiceBroadcastGETReturnsSafeDefaultView(t *testing.T) {
	s := &Server{voiceBroadcastConfig: &memoryVoiceBroadcastStore{}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/app/voice-broadcast-config", nil)
	s.voiceBroadcastConfigHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var envelope struct {
		Data voicebroadcastconfig.View `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.Enabled || envelope.Data.Version != 0 || envelope.Data.DefaultVoice != voicebroadcastconfig.DefaultFemaleVoice {
		t.Fatalf("view=%+v", envelope.Data)
	}
	if strings.Contains(rr.Body.String(), "apiKey\":\"sk-") {
		t.Fatal("GET leaked API key")
	}
}

func TestAdminVoiceBroadcastPUTUsesExpectedVersionAndMasksSecret(t *testing.T) {
	store := &memoryVoiceBroadcastStore{}
	s := &Server{voiceBroadcastConfig: store}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/app/voice-broadcast-config", strings.NewReader(`{"expectedVersion":0,"enabled":true,"provider":"aliyun-bailian","region":"cn-beijing","workspaceId":"ws-1","model":"qwen3-tts-instruct-flash","currentVoice":"Cherry","apiKey":"sk-secret"}`))
	s.voiceBroadcastConfigHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk-secret") {
		t.Fatal("PUT response leaked API key")
	}
	conflict := httptest.NewRecorder()
	conflictReq := httptest.NewRequest(http.MethodPut, "/api/app/voice-broadcast-config", strings.NewReader(`{"expectedVersion":0,"enabled":false}`))
	s.voiceBroadcastConfigHandler(conflict, conflictReq)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestAdminVoiceBroadcastPUTRejectsUnknownFields(t *testing.T) {
	s := &Server{voiceBroadcastConfig: &memoryVoiceBroadcastStore{}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/app/voice-broadcast-config", strings.NewReader(`{"expectedVersion":0,"unexpected":true}`))
	s.voiceBroadcastConfigHandler(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminVoiceBroadcastGETHidesStorageErrors(t *testing.T) {
	s := &Server{voiceBroadcastConfig: &memoryVoiceBroadcastStore{readErr: errors.New("db leaked sk-secret")}}
	rr := httptest.NewRecorder()
	s.voiceBroadcastConfigHandler(rr, httptest.NewRequest(http.MethodGet, "/api/app/voice-broadcast-config", nil))
	if rr.Code != http.StatusInternalServerError || strings.Contains(rr.Body.String(), "sk-secret") {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminVoiceBroadcastTestUsesInjectedProbeAndRecordsHealth(t *testing.T) {
	store := &memoryVoiceBroadcastStore{cfg: voicebroadcastconfig.Config{
		Version:      3,
		Provider:     voicebroadcastconfig.DefaultProvider,
		Region:       voicebroadcastconfig.DefaultRegion,
		Model:        voicebroadcastconfig.DefaultModel,
		CurrentVoice: voicebroadcastconfig.DefaultFemaleVoice,
	}, found: true}
	var got voicebroadcastconfig.Config
	s := &Server{
		voiceBroadcastConfig: store,
		voiceBroadcastProbe: func(_ context.Context, cfg voicebroadcastconfig.Config, text string) voiceBroadcastProbeResult {
			got = cfg
			if text != "自定义测试" {
				t.Fatalf("probe text=%q", text)
			}
			return voiceBroadcastProbeResult{OK: true, LatencyMS: 42, Message: "provider sk-test ok"}
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/app/voice-broadcast-config/test", strings.NewReader(`{"expectedVersion":3,"apiKey":"sk-test","currentVoice":"Serena","text":"自定义测试"}`))
	s.voiceBroadcastConfigTestHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got.APIKey != "sk-test" || got.CurrentVoice != "Serena" {
		t.Fatalf("probe config=%+v", got)
	}
	if store.cfg.Health.Status != "ok" || !store.cfg.Health.OK || store.cfg.Health.LatencyMS != 42 {
		t.Fatalf("health=%+v", store.cfg.Health)
	}
	if strings.Contains(store.cfg.Health.Message, "sk-test") || !strings.Contains(store.cfg.Health.Message, "[redacted]") {
		t.Fatalf("health message was not redacted: %q", store.cfg.Health.Message)
	}
	if store.cfg.Version != 4 {
		t.Fatalf("version=%d want 4", store.cfg.Version)
	}
}

func TestAdminVoiceBroadcastTestReturnsConflictWithoutLeakingProbeData(t *testing.T) {
	store := &memoryVoiceBroadcastStore{cfg: voicebroadcastconfig.Config{Version: 4}, found: true, healthErr: voicebroadcastconfig.ErrConflict}
	s := &Server{
		voiceBroadcastConfig: store,
		voiceBroadcastProbe: func(context.Context, voicebroadcastconfig.Config, string) voiceBroadcastProbeResult {
			return voiceBroadcastProbeResult{Message: "provider body sk-secret"}
		},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/app/voice-broadcast-config/test", strings.NewReader(`{"expectedVersion":4,"apiKey":"sk-secret"}`))
	s.voiceBroadcastConfigTestHandler(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "sk-secret") {
		t.Fatal("conflict response leaked probe data")
	}
}
