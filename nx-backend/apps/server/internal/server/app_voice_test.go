package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

func TestRecognizeSpeechRequiresASRConfig(t *testing.T) {
	s := &Server{}

	_, err := s.recognizeSpeech(context.Background(), []byte("audio"), "voice.wav")
	if err == nil {
		t.Fatal("expected missing ASR config error")
	}
	if !strings.Contains(err.Error(), "ASR_API_BASE") || !strings.Contains(err.Error(), "ASR_API_KEY") {
		t.Fatalf("expected actionable config error, got %v", err)
	}
	if !strings.Contains(err.Error(), "模型配置") || !strings.Contains(err.Error(), "芯之力语音配置") {
		t.Fatalf("expected admin configuration path, got %v", err)
	}
}

func TestRecognizeSpeechPrefersStoredXinzhiliVoiceASRConfig(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer stored-key" {
			t.Errorf("expected stored authorization, got %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("parse multipart form: %v", err)
		}
		if got := r.FormValue("model"); got != "stored-model" {
			t.Errorf("expected stored model, got %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "后台配置已生效"})
	}))
	defer upstream.Close()

	stored := modelconfig.Config{XinzhiliVoice: modelconfig.XinzhiliVoiceConfig{
		ASR: modelconfig.SpeechModelConfig{
			Provider:       modelconfig.ProviderOpenAICompatible,
			APIBase:        upstream.URL,
			APIKey:         "stored-key",
			Model:          "stored-model",
			TimeoutSeconds: 17,
		},
	}}
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	db := openModelConfigViewTestDB(t, string(raw))

	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		if timeout != 17*time.Second {
			t.Errorf("expected stored timeout 17s, got %s", timeout)
		}
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	s := &Server{
		db: db,
		env: config.Env{ASR: config.ASRConfig{
			APIBase:        upstream.URL,
			APIKey:         "env-key",
			Model:          "env-model",
			TimeoutSeconds: 3,
		}},
	}

	text, err := s.recognizeSpeech(context.Background(), []byte("audio-bytes"), "recording.wav")
	if err != nil {
		t.Fatalf("recognize speech: %v", err)
	}
	if text != "后台配置已生效" {
		t.Fatalf("unexpected transcription text: %q", text)
	}
}

func TestResolveAppChatASRConfigFallsBackToEnvironment(t *testing.T) {
	envConfig := config.ASRConfig{
		APIBase:        "https://env-asr.example.com",
		APIKey:         "env-key",
		Model:          "env-model",
		TimeoutSeconds: 23,
	}
	tests := []struct {
		name   string
		stored modelconfig.SpeechModelConfig
		found  bool
		err    error
	}{
		{name: "missing", found: false},
		{name: "read failure", found: false, err: errors.New("database unavailable")},
		{
			name: "incomplete",
			stored: modelconfig.SpeechModelConfig{
				Provider: modelconfig.ProviderOpenAICompatible,
				APIBase:  "https://stored-asr.example.com",
				APIKey:   "stored-key",
			},
			found: true,
		},
		{
			name: "unsupported provider",
			stored: modelconfig.SpeechModelConfig{
				Provider: "minimax",
				APIBase:  "https://stored-asr.example.com",
				APIKey:   "stored-key",
				Model:    "stored-model",
			},
			found: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				env: config.Env{ASR: envConfig},
				appChatASRConfigLoader: func(context.Context) (modelconfig.SpeechModelConfig, bool, error) {
					return tt.stored, tt.found, tt.err
				},
			}

			if got := s.resolveAppChatASRConfig(context.Background()); got != envConfig {
				t.Fatalf("expected environment fallback %+v, got %+v", envConfig, got)
			}
		})
	}
}

func TestAppVoiceRecognizeRejectsNonAudioUpload(t *testing.T) {
	body, contentType := multipartBody(t, "audio", "notes.txt", "text/plain", "not audio")
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/api/app/voice/recognize", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 42}))
	res := httptest.NewRecorder()

	s.appVoiceRecognize(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected non-audio upload to return 400, got %d body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "音频") {
		t.Fatalf("expected actionable audio validation error, got %s", res.Body.String())
	}
}

func TestRecognizeSpeechRejectsLocalAPIBaseBeforeDial(t *testing.T) {
	sawRequest := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sawRequest = true
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "local transcription"})
	}))
	defer upstream.Close()

	s := &Server{env: config.Env{ASR: config.ASRConfig{
		APIBase:        upstream.URL,
		APIKey:         "test-key",
		Model:          "whisper-1",
		TimeoutSeconds: 3,
	}}}

	_, err := s.recognizeSpeech(context.Background(), []byte("audio-bytes"), "recording.wav")
	if err == nil {
		t.Fatal("expected local ASR API base to be rejected")
	}
	if !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("expected private/local address error, got %v", err)
	}
	if sawRequest {
		t.Fatal("local ASR API base must be rejected before sending the request")
	}
}

func TestRecognizeSpeechPostsOpenAICompatibleTranscription(t *testing.T) {
	var sawRequest bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawRequest = true
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("expected transcription path, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected authorization header: %q", got)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart form: %v", err)
		}
		if got := r.FormValue("model"); got != "whisper-1" {
			t.Fatalf("expected model whisper-1, got %q", got)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("expected file field: %v", err)
		}
		defer file.Close()
		if header.Filename != "recording.wav" {
			t.Fatalf("expected original filename, got %q", header.Filename)
		}
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "audio-bytes" {
			t.Fatalf("unexpected audio payload: %q", string(data))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"text": "今天状态很好"})
	}))
	defer upstream.Close()
	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	s := &Server{env: config.Env{ASR: config.ASRConfig{
		APIBase:        upstream.URL,
		APIKey:         "test-key",
		Model:          "whisper-1",
		TimeoutSeconds: 3,
	}}}

	text, err := s.recognizeSpeech(context.Background(), []byte("audio-bytes"), "recording.wav")
	if err != nil {
		t.Fatalf("recognize speech: %v", err)
	}
	if text != "今天状态很好" {
		t.Fatalf("unexpected transcription text: %q", text)
	}
	if !sawRequest {
		t.Fatal("expected upstream request")
	}
}
