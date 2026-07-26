package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

func TestCompatibleSpeechClientTranscribesMultipartAudio(t *testing.T) {
	var gotAuthorization, gotModel, gotLanguage, gotFilename string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/transcriptions" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		gotAuthorization = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		gotModel = r.FormValue("model")
		gotLanguage = r.FormValue("language")
		file, header, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		gotFilename = header.Filename
		body, _ := io.ReadAll(file)
		if string(body) != "wav-data" {
			t.Fatalf("audio = %q", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"text": "你好"})
	}))
	defer server.Close()

	client := NewCompatibleSpeechClient(modelconfig.XinzhiliVoiceConfig{ASR: modelconfig.SpeechModelConfig{
		APIBase: server.URL + "/v1", APIKey: "secret", Model: "whisper-1", Language: "zh", TimeoutSeconds: 3,
	}})
	text, err := client.Transcribe(context.Background(), []byte("wav-data"), "turn.wav")
	if err != nil {
		t.Fatal(err)
	}
	if text != "你好" || gotAuthorization != "Bearer secret" || gotModel != "whisper-1" || gotLanguage != "zh" || gotFilename != "turn.wav" {
		t.Fatalf("unexpected request/result: text=%q auth=%q model=%q language=%q filename=%q", text, gotAuthorization, gotModel, gotLanguage, gotFilename)
	}
}

func TestCompatibleSpeechClientSynthesizesOpenAICompatibleAudio(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/audio/speech" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mp3-data"))
	}))
	defer server.Close()

	client := NewCompatibleSpeechClient(modelconfig.XinzhiliVoiceConfig{TTS: modelconfig.SpeechModelConfig{
		APIBase: server.URL + "/v1/", APIKey: "secret", Model: "tts-1", Voice: "nova", Speed: 1.1, ResponseFormat: "mp3", TimeoutSeconds: 3,
	}})
	audio, contentType, err := client.Synthesize(context.Background(), "慢慢来")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != "mp3-data" || contentType != "audio/mpeg" {
		t.Fatalf("audio=%q contentType=%q", audio, contentType)
	}
	encoded, _ := json.Marshal(request)
	for _, want := range []string{`"model":"tts-1"`, `"voice":"nova"`, `"input":"慢慢来"`, `"response_format":"mp3"`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("request %s missing %s", encoded, want)
		}
	}
}
