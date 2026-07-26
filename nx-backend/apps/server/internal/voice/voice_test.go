package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
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

func TestTextToAudioLimitedRejectsOversizedInlineAudioBeforeReturning(t *testing.T) {
	for _, encoding := range []string{"base64", "hex"} {
		t.Run(encoding, func(t *testing.T) {
			raw := []byte(strings.Repeat("\xff", 33))
			value := base64.StdEncoding.EncodeToString(raw)
			if encoding == "hex" {
				value = strings.Repeat("00", len(raw))
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"audio": value}})
			}))
			defer upstream.Close()
			client := NewMiniMaxClient(config.MiniMaxConfig{APIBase: upstream.URL, APIKey: "test-key"})
			client.client = upstream.Client()
			_, _, err := client.TextToAudioLimited(context.Background(), "model", "voice", "text", 32)
			if err == nil || !strings.Contains(err.Error(), "超过") {
				t.Fatalf("err=%v", err)
			}
			if strings.Contains(err.Error(), value) {
				t.Fatalf("error leaked response body: %v", err)
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestDownloadLimitedStopsAtMaxBytes(t *testing.T) {
	client := NewMiniMaxClient(config.MiniMaxConfig{APIKey: "test-key"})
	client.client = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", 33))),
		}, nil
	})}
	_, _, err := client.downloadLimited(context.Background(), "https://example.com/audio.mp3", 32)
	if err == nil || !strings.Contains(err.Error(), "超过") {
		t.Fatalf("err=%v", err)
	}
}

func TestTextToAudioLimitedBoundsJSONResponseWithoutLeakingBody(t *testing.T) {
	secret := strings.Repeat("secret-response-", 10_000)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"padding": secret, "data": map[string]any{"audio": "AA=="}})
	}))
	defer upstream.Close()
	client := NewMiniMaxClient(config.MiniMaxConfig{APIBase: upstream.URL, APIKey: "test-key"})
	client.client = upstream.Client()
	_, _, err := client.TextToAudioLimited(context.Background(), "model", "voice", "text", 32)
	if err == nil || !strings.Contains(err.Error(), "响应过大") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "secret-response") {
		t.Fatalf("error leaked response body: %v", err)
	}
}

func TestTextToAudioLimitedStatusErrorDoesNotLeakResponseBody(t *testing.T) {
	secret := "provider-secret-response-body"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(secret))
	}))
	defer upstream.Close()
	client := NewMiniMaxClient(config.MiniMaxConfig{APIBase: upstream.URL, APIKey: "test-key"})
	client.client = upstream.Client()
	_, _, err := client.TextToAudioLimited(context.Background(), "model", "voice", "text", 32)
	if err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked response body: %v", err)
	}
}
