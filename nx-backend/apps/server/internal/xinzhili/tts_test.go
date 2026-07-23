package xinzhili

import (
	"context"
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
		{"无标点硬切且不拆UTF8", strings.Repeat("你", 43), []string{strings.Repeat("你", 42), "你"}},
		{"超长优先弱标点", strings.Repeat("甲", 30) + "，" + strings.Repeat("乙", 20), []string{strings.Repeat("甲", 30) + "，", strings.Repeat("乙", 20)}},
		{"空文本", " \n\t ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitSentences(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
			for _, segment := range got {
				if n := len([]rune(segment)); n > 42 {
					t.Fatalf("%d runes in %q", n, segment)
				}
			}
		})
	}
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
	audio, mime, err := provider.Synthesize(context.Background(), cfg, "你好")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/proxy/v1/audio/speech" || gotAuth != "Bearer secret-key" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	want := map[string]any{"model": "tts-model", "voice": "warm", "input": "你好", "response_format": "mp3"}
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
	if _, _, err = p.Synthesize(context.Background(), cfg, "hello"); err == nil {
		t.Fatal("expected timeout")
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
	_, mime, err := p.Synthesize(context.Background(), cfg, "短句")
	if err != nil {
		t.Fatal(err)
	}
	if client.model != "speech-02" || client.voice != "voice-1" || client.text != "短句" || client.maxBytes != maxTTSSegmentBytes || mime != "audio/mpeg" {
		t.Fatalf("client=%#v mime=%q", client, mime)
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
		return append(testMP3(), make([]byte, size)...), "audio/mpeg", nil
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
		if s.Seq != 0 || s.Text != "你好。" || s.MIME != "audio/mpeg" || len(s.Audio) == 0 || s.SHA256 == ([32]byte{}) {
			t.Fatalf("segment=%#v", s)
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

func testMP3() []byte {
	// MPEG-1 Layer III, 128 kbps, 44.1 kHz, no padding => 417 bytes.
	frame := make([]byte, 417)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0x64})
	return frame
}
