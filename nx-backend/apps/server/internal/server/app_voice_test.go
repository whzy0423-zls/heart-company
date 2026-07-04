package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
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
