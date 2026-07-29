package voice

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
func TestBailianCloneVoicePostsSampleDataAndReturnsFinalVoiceID(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "Bearer dashscope-key" {
			t.Fatalf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["model"] != "qwen-voice-enrollment" {
			t.Fatalf("model = %v, want qwen-voice-enrollment", payload["model"])
		}
		input := payload["input"].(map[string]any)
		if input["action"] != "create" || input["target_model"] != "qwen3-tts-vc-realtime-2026-01-15" || input["preferred_name"] != "teacher-voice" {
			t.Fatalf("unexpected input payload: %+v", input)
		}
		audio := input["audio"].(map[string]any)
		data := audio["data"].(string)
		if !strings.HasPrefix(data, "data:audio/wav;base64,") {
			t.Fatalf("audio data URI = %q", data)
		}
		encoded := strings.TrimPrefix(data, "data:audio/wav;base64,")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || string(decoded) != "wav-bytes" {
			t.Fatalf("audio data decode = %q err=%v", string(decoded), err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"voice": "aliyun-final-voice",
			},
		})
	}))
	defer upstream.Close()

	client := NewBailianClient(BailianConfig{
		APIBase:     upstream.URL,
		APIKey:      "dashscope-key",
		TargetModel: "qwen3-tts-vc-realtime-2026-01-15",
	})
	client.client = upstream.Client()

	got, err := client.CloneVoice(context.Background(), BailianCloneInput{
		ContentType: "audio/wav",
		Data:        []byte("wav-bytes"),
		Filename:    "sample.wav",
		VoiceID:     "teacher-voice",
	})

	if err != nil {
		t.Fatalf("CloneVoice returned error: %v", err)
	}
	if got != "aliyun-final-voice" {
		t.Fatalf("final voice id = %q, want aliyun-final-voice", got)
	}
	if gotPath != "/api/v1/services/audio/tts/customization" {
		t.Fatalf("clone path = %q", gotPath)
	}
}

func TestBailianCloneVoiceUsesMiniMaxAudioURLBranch(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "Bearer dashscope-key" {
			t.Fatalf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"voice_id": "minimax-final-voice"},
		})
	}))
	defer upstream.Close()

	client := NewBailianClient(BailianConfig{
		APIBase:     upstream.URL + "/api/v1",
		APIKey:      "dashscope-key",
		TargetModel: "MiniMax/speech-2.8-turbo",
	})
	client.client = upstream.Client()

	got, err := client.CloneVoice(context.Background(), BailianCloneInput{
		AudioURL: "https://cdn.example.com/voice/sample.mp3",
		VoiceID:  "teacher-voice",
	})

	if err != nil {
		t.Fatalf("CloneVoice returned error: %v", err)
	}
	if got != "minimax-final-voice" {
		t.Fatalf("final voice id = %q, want minimax-final-voice", got)
	}
	if gotPath != "/api/v1/services/aigc/multimodal-generation/generation" {
		t.Fatalf("clone path = %q", gotPath)
	}
	if gotBody["model"] != "MiniMax/speech-2.8-turbo" {
		t.Fatalf("model = %v", gotBody["model"])
	}
	input := gotBody["input"].(map[string]any)
	if input["action"] != "voice_clone" || input["voice_id"] != "teacher-voice" || input["audio_url"] != "https://cdn.example.com/voice/sample.mp3" {
		t.Fatalf("unexpected input payload: %+v", input)
	}
}

func TestBailianMiniMaxCloneRequiresPublicObjectURL(t *testing.T) {
	client := NewBailianClient(BailianConfig{
		APIKey:      "dashscope-key",
		TargetModel: "MiniMax/speech-2.8-turbo",
	})

	_, err := client.CloneVoice(context.Background(), BailianCloneInput{
		ContentType: "audio/wav",
		Data:        []byte("private-audio"),
		VoiceID:     "teacher-voice",
	})

	if err == nil || !strings.Contains(err.Error(), "公网 URL") {
		t.Fatalf("err=%v, want public URL guidance", err)
	}
}

func TestBailianHostedMiniMaxTextToAudio(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"audio": hex.EncodeToString([]byte("mp3-bytes"))},
		})
	}))
	defer upstream.Close()

	client := NewBailianClient(BailianConfig{
		APIBase:     upstream.URL + "/api/v1",
		APIKey:      "dashscope-key",
		TargetModel: "MiniMax/speech-2.8-turbo",
	})
	client.client = upstream.Client()

	audio, contentType, err := client.TextToAudio(
		context.Background(),
		"MiniMax/speech-2.8-turbo",
		"teacher-voice",
		"你好",
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/services/aigc/multimodal-generation/generation" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotBody["model"] != "MiniMax/speech-2.8-turbo" {
		t.Fatalf("model=%v", gotBody["model"])
	}
	input := gotBody["input"].(map[string]any)
	voiceSetting := input["voice_setting"].(map[string]any)
	if input["text"] != "你好" || voiceSetting["voice_id"] != "teacher-voice" {
		t.Fatalf("input=%#v", input)
	}
	if string(audio) != "mp3-bytes" || contentType != "audio/mpeg" {
		t.Fatalf("audio=%q contentType=%q", audio, contentType)
	}
}

func TestBailianTextToAudioUsesHostedMiniMaxGeneration(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Header.Get("Authorization") != "Bearer dashscope-key" {
			t.Fatalf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"audio": hexMP3ForVoiceTest()},
		})
	}))
	defer upstream.Close()

	client := NewBailianClient(BailianConfig{APIBase: upstream.URL + "/api/v1", APIKey: "dashscope-key", TargetModel: "MiniMax/speech-2.8-turbo"})
	client.client = upstream.Client()

	audio, mime, err := client.TextToAudio(context.Background(), "", "teacher-voice", "你好")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/services/aigc/multimodal-generation/generation" {
		t.Fatalf("path=%q", gotPath)
	}
	if gotBody["model"] != "MiniMax/speech-2.8-turbo" {
		t.Fatalf("model=%v", gotBody["model"])
	}
	input := gotBody["input"].(map[string]any)
	if input["text"] != "你好" {
		t.Fatalf("input=%#v", input)
	}
	voiceSetting := input["voice_setting"].(map[string]any)
	if voiceSetting["voice_id"] != "teacher-voice" {
		t.Fatalf("voice_setting=%#v", voiceSetting)
	}
	if len(audio) == 0 || mime != "audio/mpeg" {
		t.Fatalf("audio len=%d mime=%q", len(audio), mime)
	}
}

func hexMP3ForVoiceTest() string {
	return "fffb9064" + strings.Repeat("00", 413)
}

func TestProfileProviderDefaultsToMiniMaxButKeepsBailianAliases(t *testing.T) {
	if got := normalizeProfileProvider(""); got != "minimax" {
		t.Fatalf("empty provider = %q, want minimax", got)
	}
	if got := normalizeProfileProvider("dashscope"); got != "bailian" {
		t.Fatalf("dashscope provider = %q, want bailian", got)
	}
	if got := normalizeProfileProvider(" MiniMax "); got != "minimax" {
		t.Fatalf("MiniMax provider = %q, want minimax", got)
	}
}

func TestNormalizeStoreProviderRecognizesBailianRuntime(t *testing.T) {
	if got := normalizeStoreProvider(config.MiniMaxConfig{Provider: "bailian"}); got != ProviderBailian {
		t.Fatalf("provider=%q", got)
	}
	if got := normalizeStoreProvider(config.MiniMaxConfig{
		Provider: "openai-compatible",
		APIBase:  "https://dashscope.aliyuncs.com/api/v1",
		Model:    "MiniMax/speech-2.8-turbo",
	}); got != ProviderBailian {
		t.Fatalf("legacy provider=%q", got)
	}
	if got := normalizeStoreProvider(config.MiniMaxConfig{}); got != ProviderMiniMax {
		t.Fatalf("default provider=%q", got)
	}
}

func TestNewStoreWithBailianKeepsMiniMaxAndBailianCredentialsSeparate(t *testing.T) {
	store := NewStoreWithBailian(nil, nil,
		config.MiniMaxConfig{
			APIBase: "https://minimax.example.com",
			APIKey:  "minimax-key",
			GroupID: "minimax-group",
		},
		BailianConfig{
			APIBase:     "https://dashscope.example.com/api/v1",
			APIKey:      "bailian-key",
			TargetModel: "MiniMax/speech-2.8-turbo",
		},
	)

	if store.client.apiBase != "https://minimax.example.com" || store.client.apiKey != "minimax-key" || store.client.groupID != "minimax-group" {
		t.Fatalf("MiniMax client credentials = %+v, want dedicated MiniMax config", store.client)
	}
	if store.bailian.apiBase != "https://dashscope.example.com/api/v1" || store.bailian.apiKey != "bailian-key" || store.bailian.targetModel != "MiniMax/speech-2.8-turbo" {
		t.Fatalf("Bailian client credentials = %+v, want dedicated Bailian config", store.bailian)
	}
}

func TestConfigureBailianCopyIsSafeDuringBailianSynthesis(t *testing.T) {
	store := NewStore(nil, nil, config.MiniMaxConfig{})
	const iterations = 500
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for i := 0; i < iterations; i++ {
			store.ConfigureBailianCopy(BailianConfig{})
		}
	}()
	go func() {
		defer group.Done()
		for i := 0; i < iterations; i++ {
			_, _, _ = store.textToAudio(context.Background(), ProviderBailian, "", "voice", "text")
		}
	}()
	group.Wait()
}

func TestConfigureMiniMaxIsSafeDuringSynthesis(t *testing.T) {
	store := NewStore(nil, nil, config.MiniMaxConfig{})
	const iterations = 500
	var group sync.WaitGroup
	group.Add(2)
	go func() {
		defer group.Done()
		for i := 0; i < iterations; i++ {
			store.ConfigureMiniMax(config.MiniMaxConfig{})
		}
	}()
	go func() {
		defer group.Done()
		for i := 0; i < iterations; i++ {
			_, _, _ = store.TextToAudio(context.Background(), "", "voice", "text")
		}
	}()
	group.Wait()
}

func TestDefaultSynthesisModelFollowsVoiceProvider(t *testing.T) {
	if got := defaultSynthesisModel(ProviderBailian); got != "MiniMax/speech-2.8-turbo" {
		t.Fatalf("bailian model=%q", got)
	}
	if got := defaultSynthesisModel(ProviderMiniMax); got != "speech-02-hd" {
		t.Fatalf("minimax model=%q", got)
	}
}

func TestAppendCloneVoiceOptionsIncludesBailianReadyProfiles(t *testing.T) {
	options := appendCloneVoiceOptions(nil, []Profile{
		{ID: "7", Name: "韩老师", Provider: "bailian", Status: "ready", VoiceID: "aliyun-voice-7"},
	})

	if len(options) != 1 {
		t.Fatalf("options len = %d, want 1", len(options))
	}
	got := options[0]
	if got.ID != "clone:7" || got.Provider != "bailian" || got.Source != "clone" || got.VoiceID != "aliyun-voice-7" || got.VoiceName != "韩老师" {
		t.Fatalf("unexpected clone option: %+v", got)
	}
	if got.Label != "韩老师（克隆）" {
		t.Fatalf("label = %q", got.Label)
	}
}
