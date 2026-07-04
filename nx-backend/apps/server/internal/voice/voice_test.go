package voice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestTextToAudioRejectsLocalAudioURL(t *testing.T) {
	downloadCalled := false
	audio := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		downloadCalled = true
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("local-audio"))
	}))
	defer audio.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/t2a_v2" {
			t.Fatalf("expected MiniMax t2a path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"audio": audio.URL + "/voice.mp3",
			},
		})
	}))
	defer upstream.Close()

	client := NewMiniMaxClient(config.MiniMaxConfig{
		APIBase: upstream.URL,
		APIKey:  "test-key",
	})
	client.client = upstream.Client()

	_, _, err := client.TextToAudio(context.Background(), "speech-02-hd", "voice-id", "hello")
	if err == nil {
		t.Fatal("expected local audio URL to be rejected")
	}
	if !strings.Contains(err.Error(), "不安全") {
		t.Fatalf("expected unsafe URL error, got %v", err)
	}
	if downloadCalled {
		t.Fatal("local audio URL must be rejected before making a download request")
	}
}
