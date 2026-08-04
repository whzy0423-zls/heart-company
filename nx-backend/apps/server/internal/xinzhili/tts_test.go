package xinzhili

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"中英文标点", " 你好，世界。 How are you? 我很好！ ", []string{"你好，世界。", "How are you?", "我很好！"}},
		{"无标点硬切且不拆UTF8", strings.Repeat("你", maxTTSSentenceRunes+1), []string{strings.Repeat("你", maxTTSSentenceRunes), "你"}},
		{"长中文不在旧四十二字位置硬切", strings.Repeat("甲", 50) + "收束。", []string{strings.Repeat("甲", 50) + "收束。"}},
		{"超长优先弱标点", strings.Repeat("甲", 50) + "，" + strings.Repeat("乙", 30), []string{strings.Repeat("甲", 50) + "，", strings.Repeat("乙", 30)}},
		{"小数域名缩写不误切", "版本3.14发布。Visit example.com now. U.S.A. test.", []string{"版本3.14发布。", "Visit example.com now.", "U.S.A. test."}},
		{"引号跟随句末标点", "他说：“你好。”她答：“嗯……”", []string{"他说：“你好。”", "她答：“嗯……”"}},
		{"英文引号和省略号", `He said "Hi..." Then left.`, []string{`He said "Hi..."`, "Then left."}},
		{"空文本", " \n\t ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitSentences(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
			for _, segment := range got {
				if n := len([]rune(segment)); n > maxTTSSentenceRunes {
					t.Fatalf("%d runes in %q", n, segment)
				}
			}
		})
	}
}

func TestBailianHostedMiniMaxTTSRequestParsesHexAudio(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"audio": hex.EncodeToString(testMP3())},
		})
	}))
	defer server.Close()

	cfg := TTSConfig{Provider: TTSProviderBailian, Endpoint: server.URL + "/api/v1", APIKey: "dashscope-key", Model: "MiniMax/speech-2.8-turbo", Voice: "teacher-voice", Format: "mp3"}
	provider, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	audio, mime, err := provider.Synthesize(context.Background(), cfg, "OK，我是7号，不需要 worry")
	if err != nil {
		t.Fatal(err)
	}

	if gotPath != "/api/v1/services/aigc/multimodal-generation/generation" || gotAuth != "Bearer dashscope-key" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	if gotBody["model"] != "MiniMax/speech-2.8-turbo" {
		t.Fatalf("model=%v", gotBody["model"])
	}
	input := gotBody["input"].(map[string]any)
	if input["text"] != "好，我是七号，不需要担心" {
		t.Fatalf("input text=%v", input["text"])
	}
	voiceSetting := input["voice_setting"].(map[string]any)
	if voiceSetting["voice_id"] != "teacher-voice" || voiceSetting["speed"].(float64) != 1 || voiceSetting["vol"].(float64) != 1 || voiceSetting["pitch"].(float64) != 0 {
		t.Fatalf("voice_setting=%#v", voiceSetting)
	}
	audioSetting := input["audio_setting"].(map[string]any)
	if audioSetting["format"] != "mp3" || audioSetting["sample_rate"].(float64) != 32000 || audioSetting["bitrate"].(float64) != 128000 || audioSetting["channel"].(float64) != 1 {
		t.Fatalf("audio_setting=%#v", audioSetting)
	}
	if string(audio) != string(testMP3()) || mime != "audio/mpeg" {
		t.Fatalf("audio=%x mime=%q", audio, mime)
	}
}

func TestBailianHostedMiniMaxTTSParsesOutputAudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"audio": hex.EncodeToString(testMP3())},
		})
	}))
	defer server.Close()

	cfg := TTSConfig{Provider: TTSProviderBailian, Endpoint: server.URL + "/api/v1/services/aigc/multimodal-generation/generation", APIKey: "key", Model: "MiniMax/speech-2.8-turbo", Voice: "voice"}
	provider, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	audio, mime, err := provider.Synthesize(context.Background(), cfg, "短句")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != string(testMP3()) || mime != "audio/mpeg" {
		t.Fatalf("audio/mime mismatch")
	}
}

func TestBailianQwenTTSUsesClonedVoicePayload(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"audio": hex.EncodeToString(testMP3())},
		})
	}))
	defer server.Close()

	cfg := TTSConfig{
		Provider: TTSProviderBailian,
		Endpoint: server.URL + "/api/v1",
		APIKey:   "dashscope-key",
		Model:    "qwen3-tts-vc-2026-01-22",
		Voice:    "qwen-cloned-voice",
		Format:   "mp3",
	}
	provider, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	audio, mime, err := provider.Synthesize(context.Background(), cfg, "OK，我是7号，不需要 worry")
	if err != nil {
		t.Fatal(err)
	}
	if gotBody["model"] != cfg.Model {
		t.Fatalf("model=%v", gotBody["model"])
	}
	input := gotBody["input"].(map[string]any)
	if input["text"] != "好，我是七号，不需要担心" || input["voice"] != "qwen-cloned-voice" {
		t.Fatalf("input=%#v", input)
	}
	if _, exists := input["voice_setting"]; exists {
		t.Fatalf("Qwen payload must not contain MiniMax voice_setting: %#v", input)
	}
	if string(audio) != string(testMP3()) || mime != "audio/mpeg" {
		t.Fatalf("audio/mime mismatch")
	}
}

func TestBailianQwenTTSParsesOfficialNestedAudioData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{
				"audio": map[string]any{
					"data": base64.StdEncoding.EncodeToString(testMP3()),
				},
			},
		})
	}))
	defer server.Close()

	cfg := TTSConfig{Provider: TTSProviderBailian, Endpoint: server.URL, APIKey: "key", Model: "qwen3-tts-vc-2026-01-22", Voice: "voice", Format: "mp3"}
	provider, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	audio, mime, err := provider.Synthesize(context.Background(), cfg, "短句")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != string(testMP3()) || mime != "audio/mpeg" {
		t.Fatalf("audio/mime mismatch")
	}
}

func TestBailianFetchAudioURLAcceptsOfficialWAVForMP3Normalization(t *testing.T) {
	wav := []byte("RIFF\x04\x00\x00\x00WAVE")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	}))
	defer server.Close()

	provider := &bailianHostedMiniMaxTTS{client: server.Client()}
	got, err := provider.fetchAudioURL(context.Background(), server.URL+"/qwen.wav")
	if err != nil {
		t.Fatalf("fetchAudioURL returned error: %v", err)
	}
	if !bytes.Equal(got, wav) {
		t.Fatalf("downloaded WAV = %q", got)
	}
}

func TestBailianHostedMiniMaxTTSParsesBase64AndRejectsPrivateURL(t *testing.T) {
	t.Run("base64", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"audio": base64.StdEncoding.EncodeToString(testMP3())},
			})
		}))
		defer server.Close()
		cfg := TTSConfig{Provider: TTSProviderBailian, Endpoint: server.URL, APIKey: "key", Model: "MiniMax/speech-2.8-turbo", Voice: "voice"}
		provider, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		audio, mime, err := provider.Synthesize(context.Background(), cfg, "短句")
		if err != nil {
			t.Fatal(err)
		}
		if string(audio) != string(testMP3()) || mime != "audio/mpeg" {
			t.Fatalf("audio/mime mismatch")
		}
	})

	t.Run("private url", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"audio_url": "http://127.0.0.1/audio.mp3"},
			})
		}))
		defer server.Close()
		cfg := TTSConfig{Provider: TTSProviderBailian, Endpoint: server.URL, APIKey: "key", Model: "MiniMax/speech-2.8-turbo", Voice: "voice"}
		provider, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		_, _, err = provider.Synthesize(context.Background(), cfg, "短句")
		if err == nil || !strings.Contains(err.Error(), "不安全") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestOpenAICompatibleTTSRequest(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(testMP3())
	}))
	defer server.Close()
	cfg := TTSConfig{Provider: TTSProviderOpenAICompatible, Endpoint: server.URL + "/proxy/v1", APIKey: "secret-key", Model: "tts-model", Voice: "warm"}
	provider, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	audio, mime, err := provider.Synthesize(context.Background(), cfg, "OK，我是7号，不需要 worry")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/proxy/v1/audio/speech" || gotAuth != "Bearer secret-key" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	want := map[string]any{"model": "tts-model", "voice": "warm", "input": "好，我是七号，不需要担心", "response_format": "mp3"}
	if !reflect.DeepEqual(gotBody, want) {
		t.Fatalf("body=%#v want=%#v", gotBody, want)
	}
	if string(audio) != string(testMP3()) || mime != "audio/mpeg" {
		t.Fatalf("audio=%x mime=%q", audio, mime)
	}
}

func TestOpenAICompatibleTTSAcceptsExactPathAndAudioMP3(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Header().Set("Content-Type", "audio/mp3; charset=binary")
		_, _ = w.Write(append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}, testMP3()...))
	}))
	defer server.Close()
	cfg := TTSConfig{Provider: TTSProviderOpenAICompatible, Endpoint: server.URL + "/v1/audio/speech", APIKey: "key", Model: "m", Voice: "v"}
	p, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, mime, err := p.Synthesize(context.Background(), cfg, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/audio/speech" || mime != "audio/mpeg" {
		t.Fatalf("path=%q mime=%q", path, mime)
	}
}

func TestOpenAICompatibleTTSRejectsUnsafeResponses(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        []byte
	}{
		{"状态码", http.StatusBadGateway, "text/plain", []byte("secret-key sensitive provider response")},
		{"错误Content-Type", http.StatusOK, "text/html", []byte("<html>not audio</html>")},
		{"空响应", http.StatusOK, "audio/mpeg", nil},
		{"超过响应上限", http.StatusOK, "audio/mpeg", append(testMP3(), make([]byte, maxTTSSegmentBytes)...)},
		{"无效MP3", http.StatusOK, "audio/mpeg", []byte("random bytes")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = w.Write(tt.body)
			}))
			defer server.Close()
			cfg := TTSConfig{Provider: TTSProviderOpenAICompatible, Endpoint: server.URL, APIKey: "secret-key", Model: "m", Voice: "v"}
			p, err := (TTSProviderFactory{HTTPClient: server.Client()}).New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			_, _, err = p.Synthesize(context.Background(), cfg, "完整的秘密输入文本")
			if err == nil {
				t.Fatal("expected error")
			}
			for _, secret := range []string{"secret-key", "完整的秘密输入文本", "sensitive provider response"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("leak: %v", err)
				}
			}
		})
	}
}

func TestOpenAICompatibleTTSTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(testMP3())
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Millisecond
	cfg := TTSConfig{Provider: TTSProviderOpenAICompatible, Endpoint: server.URL, APIKey: "key", Model: "m", Voice: "v"}
	p, err := (TTSProviderFactory{HTTPClient: client}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = p.Synthesize(context.Background(), cfg, "hello"); !errors.Is(err, ErrTTSTimeout) {
		t.Fatalf("err=%v", err)
	}
}

func TestOpenAICompatibleTTSBodyReadTimeoutIsStable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write(testMP3())
	}))
	defer server.Close()
	client := server.Client()
	client.Timeout = 10 * time.Millisecond
	cfg := TTSConfig{Provider: TTSProviderOpenAICompatible, Endpoint: server.URL, APIKey: "key", Model: "m", Voice: "v"}
	provider, err := (TTSProviderFactory{HTTPClient: client}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = provider.Synthesize(context.Background(), cfg, "hello")
	if !errors.Is(err, ErrTTSTimeout) {
		t.Fatalf("err=%v", err)
	}
}

type fakeMiniMaxTTS struct {
	model, voice, text string
	maxBytes           int64
}

func (f *fakeMiniMaxTTS) TextToAudioLimited(_ context.Context, model, voice, text string, maxBytes int64) ([]byte, string, error) {
	f.model, f.voice, f.text = model, voice, text
	f.maxBytes = maxBytes
	return testMP3(), "audio/mpeg", nil
}

func TestMiniMaxTTSAdapterUsesTextToAudioOnly(t *testing.T) {
	client := &fakeMiniMaxTTS{}
	cfg := TTSConfig{Provider: TTSProviderMiniMax, Model: "speech-02", Voice: "voice-1"}
	p, err := (TTSProviderFactory{MiniMax: client}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, mime, err := p.Synthesize(context.Background(), cfg, "OK，我是7号，不需要 worry")
	if err != nil {
		t.Fatal(err)
	}
	if client.model != "speech-02" || client.voice != "voice-1" || client.text != "好，我是七号，不需要担心" || client.maxBytes != maxTTSSegmentBytes || mime != "audio/mpeg" {
		t.Fatalf("client=%#v mime=%q", client, mime)
	}
}

func TestDynamicTTSProviderResolvesProviderFromEachTurnConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != bailianTTSPath {
			t.Fatalf("path=%q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": map[string]any{"audio": hex.EncodeToString(testMP3())},
		})
	}))
	defer server.Close()

	minimax := &fakeMiniMaxTTS{}
	provider := (TTSProviderFactory{HTTPClient: server.Client(), MiniMax: minimax}).Dynamic()
	miniCfg := TTSConfig{
		Provider: TTSProviderMiniMax,
		Endpoint: server.URL,
		APIKey:   "minimax-key",
		GroupID:  "group-1",
		Model:    "speech-02-hd",
		Voice:    "legacy-voice",
		Format:   "mp3",
	}
	if _, _, err := provider.Synthesize(context.Background(), miniCfg, "旧配置"); err != nil {
		t.Fatal(err)
	}
	if minimax.voice != "legacy-voice" {
		t.Fatalf("MiniMax voice=%q", minimax.voice)
	}

	bailianCfg := TTSConfig{
		Provider: TTSProviderBailian,
		Endpoint: server.URL,
		APIKey:   "bailian-key",
		Model:    "qwen3-tts-vc-2026-01-22",
		Voice:    "qwen-cloned-voice",
		Format:   "mp3",
	}
	audio, mimeType, err := provider.Synthesize(context.Background(), bailianCfg, "新配置")
	if err != nil {
		t.Fatal(err)
	}
	if string(audio) != string(testMP3()) || mimeType != "audio/mpeg" {
		t.Fatalf("audio=%x mime=%q", audio, mimeType)
	}
}

func TestMiniMaxTTSAdapterRejectsOversizedAudioAtProviderBoundary(t *testing.T) {
	client := miniMaxLimitedFunc(func(context.Context, string, string, string, int64) ([]byte, string, error) {
		return make([]byte, maxTTSSegmentBytes+1), "audio/mpeg", nil
	})
	cfg := TTSConfig{Provider: TTSProviderMiniMax, Model: "model", Voice: "voice"}
	provider, err := (TTSProviderFactory{MiniMax: client}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = provider.Synthesize(context.Background(), cfg, "短句")
	if err == nil || !strings.Contains(err.Error(), "超过") {
		t.Fatalf("err=%v", err)
	}
}

func TestMiniMaxTTSAdapterMapsContextDeadlineToStableTimeout(t *testing.T) {
	client := miniMaxLimitedFunc(func(ctx context.Context, _, _, _ string, _ int64) ([]byte, string, error) {
		<-ctx.Done()
		return nil, "", ctx.Err()
	})
	cfg := TTSConfig{Provider: TTSProviderMiniMax, Model: "model", Voice: "voice"}
	provider, err := (TTSProviderFactory{MiniMax: client}).New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, _, err = provider.Synthesize(ctx, cfg, "短句")
	if !errors.Is(err, ErrTTSTimeout) {
		t.Fatalf("err=%v", err)
	}
}

type miniMaxLimitedFunc func(context.Context, string, string, string, int64) ([]byte, string, error)

func (f miniMaxLimitedFunc) TextToAudioLimited(ctx context.Context, model, voice, text string, maxBytes int64) ([]byte, string, error) {
	return f(ctx, model, voice, text, maxBytes)
}

type delayedTTSProvider struct {
	delays  map[string]time.Duration
	failOn  string
	sizeFor map[string]int
	started atomic.Int32
	done    atomic.Int32
}

func (f *delayedTTSProvider) Synthesize(ctx context.Context, _ TTSConfig, text string) ([]byte, string, error) {
	f.started.Add(1)
	defer f.done.Add(1)
	select {
	case <-time.After(f.delays[text]):
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	if text == f.failOn {
		return nil, "", errors.New("provider full secret response")
	}
	if size := f.sizeFor[text]; size > 0 {
		frame := testMP3()
		return bytes.Repeat(frame, (size+len(frame)-1)/len(frame)), "audio/mpeg", nil
	}
	return testMP3(), "audio/mpeg", nil
}

func TestSynthesizerOrderedAndStreamsFirstCompletedPrefix(t *testing.T) {
	p := &delayedTTSProvider{delays: map[string]time.Duration{"一。": 30 * time.Millisecond, "二。": 5 * time.Millisecond, "三。": 200 * time.Millisecond}}
	start := time.Now()
	var got []uint32
	err := NewSynthesizer(p, 2).Synthesize(context.Background(), TTSConfig{}, "一。二。三。", func(s AudioSegment) error {
		got = append(got, s.Seq)
		if len(got) == 1 && time.Since(start) >= 150*time.Millisecond {
			t.Fatalf("first waited %v", time.Since(start))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []uint32{0, 1, 2}) {
		t.Fatalf("seq=%v", got)
	}
}

func TestSynthesizerSingleSegmentInputKeepsRealtimeChunkTogether(t *testing.T) {
	var calls []string
	provider := ttsProviderFunc(func(_ context.Context, _ TTSConfig, text string) ([]byte, string, error) {
		calls = append(calls, text)
		return testMP3(), "audio/mpeg", nil
	})
	var got []AudioSegment

	err := NewSynthesizer(provider, 1, WithSingleSegmentTTSInput()).Synthesize(
		context.Background(), TTSConfig{}, "好。后面继续补足一段自然长度。", func(s AudioSegment) error {
			got = append(got, s)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"好。后面继续补足一段自然长度。"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("provider calls=%q want %q", calls, want)
	}
	if len(got) != 1 || got[0].DeliveryText() != "好。后面继续补足一段自然长度。" {
		t.Fatalf("segments=%+v", got)
	}
}

func TestSynthesizerCancellationStopsQueuedWorkAndLateOutput(t *testing.T) {
	p := &delayedTTSProvider{delays: map[string]time.Duration{"一。": time.Second, "二。": time.Second, "三。": time.Second, "四。": time.Second}}
	ctx, cancel := context.WithCancel(context.Background())
	var emitted atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- NewSynthesizer(p, 2).Synthesize(ctx, TTSConfig{}, "一。二。三。四。", func(AudioSegment) error { emitted.Add(1); return nil })
	}()
	deadline := time.Now().Add(time.Second)
	for p.started.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("goroutine leak")
	}
	if p.started.Load() != 2 || p.done.Load() != 2 || emitted.Load() != 0 {
		t.Fatalf("started=%d done=%d emitted=%d", p.started.Load(), p.done.Load(), emitted.Load())
	}
}

func TestSynthesizerSegmentTimeoutCancelsCompliantProviderAndExits(t *testing.T) {
	var exited atomic.Bool
	provider := ttsProviderFunc(func(ctx context.Context, _ TTSConfig, _ string) ([]byte, string, error) {
		<-ctx.Done()
		exited.Store(true)
		return nil, "", ctx.Err()
	})
	start := time.Now()
	err := NewSynthesizer(provider, 1, WithTTSSegmentTimeout(20*time.Millisecond)).Synthesize(
		context.Background(), TTSConfig{}, "需要超时。", func(AudioSegment) error { return nil },
	)
	if !errors.Is(err, ErrTTSTimeout) {
		t.Fatalf("err=%v", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Fatalf("timeout return took %v", elapsed)
	}
	if !exited.Load() {
		t.Fatal("provider goroutine did not exit")
	}
}

func TestSynthesizerDoesNotTurnProviderCancellationIntoEmptySegment(t *testing.T) {
	provider := ttsProviderFunc(func(context.Context, TTSConfig, string) ([]byte, string, error) {
		return nil, "", context.Canceled
	})
	err := NewSynthesizer(provider, 1).Synthesize(context.Background(), TTSConfig{}, "取消。", func(AudioSegment) error {
		t.Fatal("must not emit an empty segment")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}

func TestSynthesizerEmitFailureCancelsBlockedWorkerWithoutDrain(t *testing.T) {
	emitErr := errors.New("emit stopped")
	var blockedExited atomic.Bool
	secondStarted := make(chan struct{})
	provider := ttsProviderFunc(func(ctx context.Context, _ TTSConfig, text string) ([]byte, string, error) {
		if text == "一。" {
			select {
			case <-secondStarted:
				return testMP3(), "audio/mpeg", nil
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
		}
		close(secondStarted)
		<-ctx.Done()
		blockedExited.Store(true)
		return nil, "", ctx.Err()
	})
	start := time.Now()
	err := NewSynthesizer(provider, 2, WithTTSSegmentTimeout(time.Second)).Synthesize(
		context.Background(), TTSConfig{}, "一。二。", func(AudioSegment) error { return emitErr },
	)
	if !errors.Is(err, emitErr) {
		t.Fatalf("err=%v", err)
	}
	if time.Since(start) > 300*time.Millisecond || !blockedExited.Load() {
		t.Fatalf("elapsed=%v blockedExited=%v", time.Since(start), blockedExited.Load())
	}
}

func TestSynthesizerDropsLateProviderResultAndWaitsForWorkerExit(t *testing.T) {
	var finished atomic.Bool
	provider := ttsProviderFunc(func(context.Context, TTSConfig, string) ([]byte, string, error) {
		time.Sleep(30 * time.Millisecond) // 模拟不及时响应 ctx 的第三方 SDK。
		finished.Store(true)
		return testMP3(), "audio/mpeg", nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	var emitted atomic.Int32
	done := make(chan error, 1)
	go func() {
		done <- NewSynthesizer(provider, 1).Synthesize(ctx, TTSConfig{}, "一。", func(AudioSegment) error {
			emitted.Add(1)
			return nil
		})
	}()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("goroutine leak")
	}
	if !finished.Load() || emitted.Load() != 0 {
		t.Fatalf("finished=%v emitted=%d", finished.Load(), emitted.Load())
	}
}

func TestSynthesizerHonorsConcurrencyLimit(t *testing.T) {
	var active atomic.Int32
	var peak atomic.Int32
	provider := ttsProviderFunc(func(ctx context.Context, _ TTSConfig, _ string) ([]byte, string, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if current <= old || peak.CompareAndSwap(old, current) {
				break
			}
		}
		select {
		case <-time.After(5 * time.Millisecond):
			return testMP3(), "audio/mpeg", nil
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	})
	err := NewSynthesizer(provider, 2).Synthesize(context.Background(), TTSConfig{}, "一。二。三。四。五。", func(AudioSegment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 2 {
		t.Fatalf("peak concurrency=%d", peak.Load())
	}
}

func TestNewSynthesizerClampsConcurrencyToTwo(t *testing.T) {
	provider := ttsProviderFunc(func(context.Context, TTSConfig, string) ([]byte, string, error) {
		return testMP3(), "audio/mpeg", nil
	})
	tests := []struct {
		input int
		want  int
	}{
		{input: -1, want: 2},
		{input: 0, want: 2},
		{input: 1, want: 1},
		{input: 2, want: 2},
		{input: 8, want: 2},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("input_%d", tt.input), func(t *testing.T) {
			if got := NewSynthesizer(provider, tt.input).concurrency; got != tt.want {
				t.Fatalf("concurrency=%d want %d", got, tt.want)
			}
		})
	}
}

func TestSynthesizerRequestedConcurrencyEightStillPeaksAtTwo(t *testing.T) {
	var active atomic.Int32
	var peak atomic.Int32
	provider := ttsProviderFunc(func(ctx context.Context, _ TTSConfig, _ string) ([]byte, string, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if current <= old || peak.CompareAndSwap(old, current) {
				break
			}
		}
		select {
		case <-time.After(10 * time.Millisecond):
			return testMP3(), "audio/mpeg", nil
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
	})
	err := NewSynthesizer(provider, 8).Synthesize(context.Background(), TTSConfig{}, "一。二。三。四。五。六。七。八。", func(AudioSegment) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 2 {
		t.Fatalf("peak concurrency=%d want 2", peak.Load())
	}
}

func TestSynthesizerLimits(t *testing.T) {
	t.Run("单片", func(t *testing.T) {
		p := &delayedTTSProvider{delays: map[string]time.Duration{}, sizeFor: map[string]int{"一。": maxTTSSegmentBytes}}
		err := NewSynthesizer(p, 2).Synthesize(context.Background(), TTSConfig{}, "一。二。", func(AudioSegment) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "segment 0") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("单轮", func(t *testing.T) {
		var texts []string
		delays := map[string]time.Duration{}
		sizes := map[string]int{}
		for i := 0; i < 11; i++ {
			s := fmt.Sprintf("第%02d句。", i)
			texts = append(texts, s)
			delays[s] = 0
			sizes[s] = maxTTSSegmentBytes - len(testMP3())
		}
		p := &delayedTTSProvider{delays: delays, sizeFor: sizes}
		err := NewSynthesizer(p, 2).Synthesize(context.Background(), TTSConfig{}, strings.Join(texts, ""), func(AudioSegment) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "单轮音频超过") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("pending累计超过单轮上限立即取消", func(t *testing.T) {
		largeMP3 := testMP3NearLimit()
		var firstExited atomic.Bool
		provider := ttsProviderFunc(func(ctx context.Context, _ TTSConfig, text string) ([]byte, string, error) {
			if text == "零。" {
				<-ctx.Done()
				firstExited.Store(true)
				return nil, "", ctx.Err()
			}
			return largeMP3, "audio/mpeg", nil
		})
		start := time.Now()
		err := NewSynthesizer(provider, 2, WithTTSSegmentTimeout(time.Second)).Synthesize(
			context.Background(), TTSConfig{}, "零。一。二。三。四。五。六。七。八。九。十。十一。", func(AudioSegment) error {
				t.Fatal("blocked sequence zero means no segment may be emitted")
				return nil
			},
		)
		if !errors.Is(err, ErrTTSTurnTooLarge) {
			t.Fatalf("err=%v", err)
		}
		if time.Since(start) > 500*time.Millisecond || !firstExited.Load() {
			t.Fatalf("elapsed=%v firstExited=%v", time.Since(start), firstExited.Load())
		}
	})
}

func TestSynthesizerRetriesTransientProviderFailureOnce(t *testing.T) {
	var calls atomic.Int32
	provider := ttsProviderFunc(func(context.Context, TTSConfig, string) ([]byte, string, error) {
		if calls.Add(1) == 1 {
			return nil, "", errors.New("temporary upstream failure")
		}
		return testMP3(), "audio/mpeg", nil
	})
	var emitted int
	err := NewSynthesizer(provider, 1).Synthesize(context.Background(), TTSConfig{}, "你好。", func(AudioSegment) error {
		emitted++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("calls=%d want=2", got)
	}
	if emitted != 1 {
		t.Fatalf("emitted=%d want=1", emitted)
	}
}

func TestSynthesizerRetriesProviderReportedTTSTimeoutOnceWhenSegmentStillAlive(t *testing.T) {
	var calls atomic.Int32
	provider := ttsProviderFunc(func(ctx context.Context, _ TTSConfig, _ string) ([]byte, string, error) {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if calls.Add(1) == 1 {
			return nil, "", ErrTTSTimeout
		}
		return testMP3(), "audio/mpeg", nil
	})
	var emitted int
	err := NewSynthesizer(provider, 1, WithTTSSegmentTimeout(time.Second)).Synthesize(context.Background(), TTSConfig{}, "青泥何盘盘。", func(AudioSegment) error {
		emitted++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || emitted != 1 {
		t.Fatalf("calls=%d emitted=%d, want one retry and one segment", calls.Load(), emitted)
	}
}

type ttsProviderFunc func(context.Context, TTSConfig, string) ([]byte, string, error)

func (f ttsProviderFunc) Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error) {
	return f(ctx, cfg, text)
}

func TestSynthesizerValidatesAudioAndSanitizesErrors(t *testing.T) {
	t.Run("无效MP3", func(t *testing.T) {
		p := ttsProviderFunc(func(context.Context, TTSConfig, string) ([]byte, string, error) {
			return []byte("not mp3"), "audio/mpeg", nil
		})
		err := NewSynthesizer(p, 1).Synthesize(context.Background(), TTSConfig{}, "秘密全文。", func(AudioSegment) error { return nil })
		if err == nil || !strings.Contains(err.Error(), "segment 0") || strings.Contains(err.Error(), "秘密全文") {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("失败后停止", func(t *testing.T) {
		p := &delayedTTSProvider{delays: map[string]time.Duration{"一。": 0, "秘密全文。": 5 * time.Millisecond, "三。": time.Second}, failOn: "秘密全文。"}
		var got []uint32
		err := NewSynthesizer(p, 2).Synthesize(context.Background(), TTSConfig{}, "一。秘密全文。三。", func(s AudioSegment) error { got = append(got, s.Seq); return nil })
		if err == nil || !strings.Contains(err.Error(), "segment 1") || strings.Contains(err.Error(), "秘密全文") || strings.Contains(err.Error(), "full secret") {
			t.Fatalf("err=%v", err)
		}
		if !reflect.DeepEqual(got, []uint32{0}) {
			t.Fatalf("got=%v", got)
		}
	})
}

func TestSynthesizerMetadata(t *testing.T) {
	p := ttsProviderFunc(func(context.Context, TTSConfig, string) ([]byte, string, error) { return testMP3(), "audio/mp3", nil })
	err := NewSynthesizer(p, 1).Synthesize(context.Background(), TTSConfig{}, "你好。", func(s AudioSegment) error {
		if s.Seq != 0 || s.DeliveryText() != "你好。" || s.MIME != "audio/mpeg" || len(s.Audio) == 0 || s.ByteLength != len(s.Audio) || s.SHA256 == ([32]byte{}) {
			t.Fatalf("segment=%#v", s)
		}
		encoded, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(encoded, []byte("你好")) || bytes.Contains(encoded, []byte(`"Text"`)) || bytes.Contains(encoded, []byte("deliveryText")) {
			t.Fatalf("serialized segment leaks delivery text: %s", encoded)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidMP3RequiresACompleteMPEGLayerIIIFrame(t *testing.T) {
	fullFrame := testMP3()
	tests := []struct {
		name  string
		audio []byte
		want  bool
	}{
		{name: "完整MPEG1 Layer III帧", audio: fullFrame, want: true},
		{name: "合法ID3后有完整帧", audio: append([]byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}, fullFrame...), want: true},
		{name: "六字节伪帧", audio: []byte{0xff, 0xfb, 0x90, 0x64, 0, 0}, want: false},
		{name: "仅ID3标签", audio: []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}, want: false},
		{name: "截断MPEG帧", audio: fullFrame[:len(fullFrame)-1], want: false},
		{name: "合法首帧加截断二帧", audio: append(append([]byte{}, fullFrame...), fullFrame[:len(fullFrame)-1]...), want: false},
		{name: "合法帧加HTML垃圾", audio: append(append([]byte{}, fullFrame...), []byte("<html>bad</html>")...), want: false},
		{name: "多个完整合法帧", audio: append(append([]byte{}, fullFrame...), fullFrame...), want: true},
		{name: "ID3声明越界", audio: []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 1, 0}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validMP3(tt.audio); got != tt.want {
				t.Fatalf("validMP3()=%v want %v (len=%d)", got, tt.want, len(tt.audio))
			}
		})
	}
}

func TestNormalizeMP3ForStreamingDropsEmbeddedID3Tags(t *testing.T) {
	fullFrame := testMP3()
	emptyID3 := []byte{'I', 'D', '3', 4, 0, 0, 0, 0, 0, 0}
	audio := append(append(append(append([]byte{}, emptyID3...), fullFrame...), emptyID3...), fullFrame...)

	cleaned := normalizeMP3ForStreaming(audio)
	if bytes.Count(cleaned, []byte("ID3")) != 1 {
		t.Fatalf("cleaned should keep only leading ID3, got %d ID3 markers", bytes.Count(cleaned, []byte("ID3")))
	}
	if !validMP3(cleaned) {
		t.Fatalf("cleaned audio should be a valid streaming mp3")
	}
	want := append(append(append([]byte{}, emptyID3...), fullFrame...), fullFrame...)
	if !bytes.Equal(cleaned, want) {
		t.Fatalf("cleaned length=%d want=%d", len(cleaned), len(want))
	}
}

func testMP3() []byte {
	// MPEG-1 Layer III, 128 kbps, 44.1 kHz, no padding => 417 bytes.
	frame := make([]byte, 417)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0x64})
	return frame
}

func testMP3NearLimit() []byte {
	frame := testMP3()
	return bytes.Repeat(frame, maxTTSSegmentBytes/len(frame))
}
