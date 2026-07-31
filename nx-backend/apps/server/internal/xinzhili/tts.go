package xinzhili

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	appconfig "nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/netguard"
	"nine-xing/nx-backend/apps/server/internal/voice"
)

const (
	maxTTSSentenceRunes = 42
	maxTTSSegmentBytes  = 1 << 20
	maxTTSTurnBytes     = 10 << 20
	defaultTTSWorkers   = 2
	defaultTTSTimeout   = 15 * time.Second
	bailianTTSPath      = "/api/v1/services/aigc/multimodal-generation/generation"
)

var (
	ErrTTSTimeout      = errors.New("TTS 短句合成超时")
	ErrTTSTurnTooLarge = errors.New("TTS 单轮音频超过 10MiB")
)

// AudioSegment is one complete, independently playable MP3 sentence. Audio is
// never split at the byte level and is never persisted by this package.
type AudioSegment struct {
	TurnKey    uint64   `json:"-"`
	Seq        uint32   `json:"seq"`
	Audio      []byte   `json:"audio"`
	MIME       string   `json:"mime"`
	SHA256     [32]byte `json:"sha256"`
	ByteLength int      `json:"byteLength"`

	deliveryText string
}

// DeliveryText is available only to in-process orchestration. It is not an
// exported data field and therefore cannot be serialized by encoding/json.
func (s AudioSegment) DeliveryText() string { return s.deliveryText }

// TTSProvider is deliberately stateless. Implementations return one complete
// audio file for one short sentence. Providers must honor ctx cancellation and
// deadlines; Go cannot forcibly terminate an implementation that violates
// this contract.
type TTSProvider interface {
	Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error)
}

// MiniMaxTextToAudio is the narrow part of voice.MiniMaxClient reused here. It
// intentionally excludes the legacy Store, uploads and generated assets.
type MiniMaxTextToAudio interface {
	TextToAudioLimited(ctx context.Context, model string, voiceID string, text string, maxBytes int64) ([]byte, string, error)
}

type TTSProviderFactory struct {
	HTTPClient *http.Client
	MiniMax    MiniMaxTextToAudio
}

type dynamicTTSProvider struct {
	factory  TTSProviderFactory
	mu       sync.Mutex
	config   TTSConfig
	provider TTSProvider
}

// Dynamic resolves and caches the concrete provider from the configuration
// supplied by each turn. This lets an already-connected realtime session use
// the latest provider, endpoint, model and voice saved in the admin console.
func (f TTSProviderFactory) Dynamic() TTSProvider {
	return &dynamicTTSProvider{factory: f}
}

func (p *dynamicTTSProvider) Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error) {
	p.mu.Lock()
	provider := p.provider
	if provider == nil || p.config != cfg {
		var err error
		provider, err = p.factory.New(cfg)
		if err != nil {
			p.mu.Unlock()
			return nil, "", err
		}
		p.config = cfg
		p.provider = provider
	}
	p.mu.Unlock()
	return provider.Synthesize(ctx, cfg, text)
}

func (f TTSProviderFactory) New(cfg TTSConfig) (TTSProvider, error) {
	switch strings.TrimSpace(cfg.Provider) {
	case TTSProviderOpenAICompatible:
		client := f.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second, Transport: netguard.NewGuardedTransport()}
		}
		endpoint, err := openAISpeechEndpoint(cfg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("TTS endpoint 无效")
		}
		return &openAICompatibleTTS{client: client, endpoint: endpoint}, nil
	case TTSProviderMiniMax:
		client := f.MiniMax
		if client == nil {
			client = voice.NewMiniMaxClient(appconfig.MiniMaxConfig{
				APIBase: cfg.Endpoint,
				APIKey:  cfg.APIKey,
				GroupID: cfg.GroupID,
				Model:   cfg.Model,
			})
		}
		return miniMaxTTSAdapter{client: client}, nil
	case TTSProviderBailian:
		client := f.HTTPClient
		if client == nil {
			client = &http.Client{Timeout: 30 * time.Second, Transport: netguard.NewGuardedTransport()}
		}
		endpoint, err := bailianTTSEndpoint(cfg.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("TTS endpoint 无效")
		}
		return &bailianHostedMiniMaxTTS{client: client, endpoint: endpoint}, nil
	default:
		return nil, errors.New("不支持的 TTS provider")
	}
}

type miniMaxTTSAdapter struct{ client MiniMaxTextToAudio }

func (p miniMaxTTSAdapter) Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error) {
	audio, mimeType, err := p.client.TextToAudioLimited(ctx, cfg.Model, cfg.Voice, text, maxTTSSegmentBytes)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, "", ErrTTSTimeout
		}
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", err
	}
	if len(audio) > maxTTSSegmentBytes {
		return nil, "", errors.New("MiniMax 音频超过 1MiB")
	}
	return audio, mimeType, nil
}

type bailianHostedMiniMaxTTS struct {
	client   *http.Client
	endpoint string
}

func (p *bailianHostedMiniMaxTTS) Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error) {
	format := strings.TrimSpace(cfg.Format)
	if format == "" {
		format = "mp3"
	}
	input := map[string]any{"text": text}
	if isBailianHostedMiniMaxTTSModel(cfg.Model) {
		input["voice_setting"] = map[string]any{
			"voice_id": cfg.Voice,
			"speed":    1,
			"vol":      1,
			"pitch":    0,
		}
		input["audio_setting"] = map[string]any{
			"format":      format,
			"sample_rate": 32000,
			"bitrate":     128000,
			"channel":     1,
		}
	} else if isBailianQwenVoiceCloneTTSModel(cfg.Model) {
		input["voice"] = cfg.Voice
	} else {
		return nil, "", errors.New("阿里百炼 TTS 模型不支持")
	}
	payload, err := json.Marshal(map[string]any{"model": cfg.Model, "input": input})
	if err != nil {
		return nil, "", errors.New("TTS 请求编码失败")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", errors.New("TTS 请求创建失败")
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, "", ErrTTSTimeout
		}
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", errors.New("TTS 请求失败")
	}
	defer resp.Body.Close()
	// Hex may be 2x and Base64 about 4/3x the decoded MP3 size.
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*maxTTSSegmentBytes+64*1024))
	if readErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(readErr, context.DeadlineExceeded) {
			return nil, "", ErrTTSTimeout
		}
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", errors.New("TTS 响应读取失败")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("TTS 请求失败（状态码 %d）", resp.StatusCode)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, "", errors.New("TTS 响应解析失败")
	}
	if err := bailianResponseError(result); err != nil {
		return nil, "", err
	}
	audioRef := firstNestedString(result,
		"data.audio", "data.hex", "data.base64", "data.audio_url", "data.url",
		"output.audio", "output.audio.data", "output.audio.url", "output.hex", "output.base64", "output.audio_url", "output.url",
		"output.data.audio", "output.data.hex", "output.data.base64", "output.data.audio_url", "output.data.url",
		"audio", "hex", "base64", "audio_url", "url",
	)
	if audioRef == "" {
		return nil, "", errors.New("TTS 返回空音频")
	}
	audio, err := p.decodeOrFetchAudio(ctx, audioRef)
	if err != nil {
		return nil, "", err
	}
	audio, _, err = voice.NormalizeTTSMP3(ctx, audio, "", maxTTSSegmentBytes)
	if err != nil {
		return nil, "", err
	}
	if len(audio) == 0 {
		return nil, "", errors.New("TTS 返回空音频")
	}
	if len(audio) > maxTTSSegmentBytes {
		return nil, "", errors.New("TTS 单片音频超过 1MiB")
	}
	if !validMP3(audio) {
		return nil, "", errors.New("TTS 返回的音频格式无效")
	}
	return audio, "audio/mpeg", nil
}

func isBailianHostedMiniMaxTTSModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "minimax/")
}

func isBailianQwenVoiceCloneTTSModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "qwen3-tts-vc-")
}

func (p *bailianHostedMiniMaxTTS) decodeOrFetchAudio(ctx context.Context, raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, errors.New("TTS 返回空音频")
	}
	if strings.HasPrefix(strings.ToLower(value), "http://") || strings.HasPrefix(strings.ToLower(value), "https://") {
		if !netguard.IsPublicHTTPURL(value) {
			return nil, errors.New("TTS 返回了不安全的音频 URL")
		}
		return p.fetchAudioURL(ctx, value)
	}
	if decoded, ok := decodeDataURLBase64(value); ok {
		return decoded, nil
	}
	compact := strings.Join(strings.Fields(value), "")
	if isHexString(compact) {
		decoded, err := hex.DecodeString(compact)
		if err != nil {
			return nil, errors.New("TTS hex 音频解析失败")
		}
		return decoded, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		return nil, errors.New("TTS 音频解析失败")
	}
	return decoded, nil
}

func (p *bailianHostedMiniMaxTTS) fetchAudioURL(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("TTS 音频下载请求创建失败")
	}
	req.Header.Set("Accept", "audio/mpeg, audio/mp3, audio/wav, audio/x-wav")
	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTTSTimeout
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("TTS 音频下载失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("TTS 音频下载失败（状态码 %d）", resp.StatusCode)
	}
	if err := validateBailianDownloadedAudioMIME(resp.Header.Get("Content-Type")); err != nil {
		return nil, err
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, 4*maxTTSSegmentBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, ErrTTSTimeout
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("TTS 音频读取失败")
	}
	return audio, nil
}

func validateBailianDownloadedAudioMIME(raw string) error {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("TTS Content-Type 无效")
	}
	switch strings.ToLower(mediaType) {
	case "application/octet-stream", "audio/mpeg", "audio/mp3", "audio/wav", "audio/x-wav":
		return nil
	default:
		return errors.New("TTS Content-Type 必须为音频格式")
	}
}

type openAICompatibleTTS struct {
	client   *http.Client
	endpoint string
}

func (p *openAICompatibleTTS) Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error) {
	payload, err := json.Marshal(map[string]string{
		"model":           cfg.Model,
		"voice":           cfg.Voice,
		"input":           text,
		"response_format": "mp3",
	})
	if err != nil {
		return nil, "", errors.New("TTS 请求编码失败")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, "", errors.New("TTS 请求创建失败")
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg, audio/mp3")

	resp, err := p.client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, "", ErrTTSTimeout
		}
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", errors.New("TTS 请求失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("TTS 请求失败（状态码 %d）", resp.StatusCode)
	}
	normalizedMIME, err := normalizeMP3MIME(resp.Header.Get("Content-Type"))
	if err != nil {
		return nil, "", err
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, maxTTSSegmentBytes+1))
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, "", ErrTTSTimeout
		}
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		return nil, "", errors.New("TTS 音频读取失败")
	}
	if len(audio) == 0 {
		return nil, "", errors.New("TTS 返回空音频")
	}
	if len(audio) > maxTTSSegmentBytes {
		return nil, "", errors.New("TTS 单片音频超过 1MiB")
	}
	if !validMP3(audio) {
		return nil, "", errors.New("TTS 返回的音频格式无效")
	}
	return audio, normalizedMIME, nil
}

func bailianTTSEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid endpoint")
	}
	pathValue := strings.TrimRight(u.Path, "/")
	if strings.Contains(pathValue, "/api/v1/services/") {
		pathValue = pathValue[:strings.Index(pathValue, "/api/v1/services/")]
	}
	if strings.HasSuffix(pathValue, "/api/v1") {
		u.Path = pathValue + strings.TrimPrefix(bailianTTSPath, "/api/v1")
	} else {
		u.Path = strings.TrimRight(pathValue, "/") + bailianTTSPath
	}
	return u.String(), nil
}

func bailianResponseError(payload map[string]any) error {
	code := firstNestedString(payload, "code", "Code", "output.base_resp.status_code", "data.base_resp.status_code")
	if code == "" || strings.EqualFold(code, "Success") || code == "0" {
		return nil
	}
	message := firstNestedString(payload, "message", "Message", "output.base_resp.status_msg", "data.base_resp.status_msg")
	if message == "" {
		message = "阿里百炼 TTS 调用失败"
	}
	return fmt.Errorf("%s: %s", code, message)
}

func firstNestedString(payload map[string]any, paths ...string) string {
	for _, rawPath := range paths {
		var current any = payload
		for _, part := range strings.Split(rawPath, ".") {
			object, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = object[part]
		}
		if value, ok := current.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
		switch value := current.(type) {
		case float64:
			if value != 0 {
				return fmt.Sprintf("%.0f", value)
			}
		case json.Number:
			return value.String()
		}
	}
	return ""
}

func decodeDataURLBase64(value string) ([]byte, bool) {
	lower := strings.ToLower(value)
	comma := strings.Index(value, ",")
	if !strings.HasPrefix(lower, "data:audio/") || comma < 0 || !strings.Contains(lower[:comma], ";base64") {
		return nil, false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value[comma+1:]))
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func isHexString(value string) bool {
	if value == "" || len(value)%2 != 0 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}

func openAISpeechEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid endpoint")
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/audio/speech") {
		path += "/audio/speech"
	}
	if path == "" {
		path = "/audio/speech"
	}
	u.Path = path
	return u.String(), nil
}

// SplitSentences prefers sentence-ending punctuation. When a sentence exceeds
// 42 Unicode code points, the last punctuation within the window is used;
// otherwise it is hard-cut on a rune boundary.
func SplitSentences(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	result := make([]string, 0, len(runes)/maxTTSSentenceRunes+1)
	for len(runes) > 0 {
		limit := min(len(runes), maxTTSSentenceRunes)
		cut := 0
		for i := 0; i < limit; i++ {
			if isStrongSentenceEndAt(runes, i) {
				cut = i + 1
				for cut < limit && isClosingQuote(runes[cut]) {
					cut++
				}
				break
			}
		}
		if cut == 0 && len(runes) > maxTTSSentenceRunes {
			for i := limit - 1; i >= 0; i-- {
				if isWeakSentenceEndAt(runes, i) {
					cut = i + 1
					break
				}
			}
		}
		if cut == 0 {
			cut = limit
		}
		segment := strings.TrimSpace(string(runes[:cut]))
		if segment != "" {
			result = append(result, segment)
		}
		runes = trimLeftSpaceRunes(runes[cut:])
	}
	return result
}

func isStrongSentenceEndAt(value []rune, index int) bool {
	r := value[index]
	switch r {
	case '。', '！', '？', '!', '?', ';', '；', '\n':
		return true
	case '…':
		return index+1 >= len(value) || value[index+1] != '…'
	case '.':
		if index+1 < len(value) && value[index+1] == '.' {
			return false
		}
		if index > 0 && index+1 < len(value) && isASCIIAlphaNumeric(value[index-1]) && isASCIIAlphaNumeric(value[index+1]) {
			return false
		}
		if isInitialismBeforePeriod(value, index) && nextNonSpaceIsAlphaNumeric(value, index+1) {
			return false
		}
		return true
	default:
		return false
	}
}

func isWeakSentenceEndAt(value []rune, index int) bool {
	r := value[index]
	if isStrongSentenceEndAt(value, index) {
		return true
	}
	switch r {
	case '，', ',', '、', ':', '：':
		return true
	default:
		return false
	}
}

func isASCIIAlphaNumeric(r rune) bool {
	return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
}

func isInitialismBeforePeriod(value []rune, period int) bool {
	start := period - 1
	for start >= 0 && (isASCIIAlphaNumeric(value[start]) || value[start] == '.') {
		start--
	}
	token := value[start+1 : period]
	if len(token) < 3 {
		return false
	}
	parts := strings.Split(string(token), ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if len([]rune(part)) != 1 || !isASCIIAlphaNumeric([]rune(part)[0]) {
			return false
		}
	}
	return true
}

func nextNonSpaceIsAlphaNumeric(value []rune, start int) bool {
	for start < len(value) && unicode.IsSpace(value[start]) {
		start++
	}
	return start < len(value) && isASCIIAlphaNumeric(value[start])
}

func isClosingQuote(r rune) bool {
	switch r {
	case '”', '’', '"', '\'', '」', '』', '》', '）', ')', '】', ']':
		return true
	default:
		return false
	}
}

func trimLeftSpaceRunes(value []rune) []rune {
	for len(value) > 0 && unicode.IsSpace(value[0]) {
		value = value[1:]
	}
	return value
}

// Synthesizer runs a small bounded worker pool while preserving sentence
// order. The callback supplies natural backpressure and makes cancellation
// explicit, avoiding an unread output channel that can leak goroutines.
type Synthesizer struct {
	provider       TTSProvider
	concurrency    int
	segmentTimeout time.Duration
}

type SynthesizerOption func(*Synthesizer)

func WithTTSSegmentTimeout(timeout time.Duration) SynthesizerOption {
	return func(s *Synthesizer) {
		if timeout > 0 {
			s.segmentTimeout = timeout
		}
	}
}

func NewSynthesizer(provider TTSProvider, concurrency int, options ...SynthesizerOption) *Synthesizer {
	if concurrency <= 0 {
		concurrency = defaultTTSWorkers
	} else if concurrency > defaultTTSWorkers {
		concurrency = defaultTTSWorkers
	}
	synthesizer := &Synthesizer{
		provider:       provider,
		concurrency:    concurrency,
		segmentTimeout: defaultTTSTimeout,
	}
	for _, option := range options {
		if option != nil {
			option(synthesizer)
		}
	}
	return synthesizer
}

type synthesisJob struct {
	seq  uint32
	text string
}

type synthesisResult struct {
	segment AudioSegment
	err     error
}

func (s *Synthesizer) Synthesize(ctx context.Context, cfg TTSConfig, text string, emit func(AudioSegment) error) error {
	if s == nil || s.provider == nil {
		return errors.New("TTS provider 未配置")
	}
	if emit == nil {
		return errors.New("TTS 输出回调未配置")
	}
	sentences := SplitSentences(text)
	if len(sentences) == 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan synthesisJob)
	results := make(chan synthesisResult)
	var workers sync.WaitGroup
	workerCount := min(s.concurrency, len(sentences))
	workers.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, ok := <-jobs:
					if !ok || ctx.Err() != nil {
						return
					}
					segmentCtx, segmentCancel := context.WithTimeout(ctx, s.segmentTimeout)
					result := s.synthesizeOne(segmentCtx, cfg, job)
					segmentCancel()
					select {
					case results <- result:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for seq, sentence := range sentences {
			select {
			case jobs <- synthesisJob{seq: uint32(seq), text: sentence}:
			case <-ctx.Done():
				return
			}
		}
	}()
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(results)
		close(workersDone)
	}()
	stopAndWait := func(err error) error {
		cancel()
		<-workersDone
		return err
	}

	pending := make(map[uint32]AudioSegment, workerCount)
	var next uint32
	var deliveredBytes int
	var pendingBytes int
	for result := range results {
		if result.err != nil {
			return stopAndWait(result.err)
		}
		if ctx.Err() != nil {
			return stopAndWait(ctx.Err())
		}
		segmentBytes := len(result.segment.Audio)
		if deliveredBytes+pendingBytes+segmentBytes > maxTTSTurnBytes {
			return stopAndWait(ErrTTSTurnTooLarge)
		}
		pending[result.segment.Seq] = result.segment
		pendingBytes += segmentBytes
		for {
			segment, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			pendingBytes -= len(segment.Audio)
			deliveredBytes += len(segment.Audio)
			if err := emit(segment); err != nil {
				return stopAndWait(err)
			}
			next++
		}
	}
	<-workersDone
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Synthesizer) synthesizeOne(ctx context.Context, cfg TTSConfig, job synthesisJob) synthesisResult {
	audio, contentType, err := s.provider.Synthesize(ctx, cfg, job.text)
	if err != nil {
		if errors.Is(err, ErrTTSTimeout) || errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return synthesisResult{err: segmentCauseError(job.seq, ErrTTSTimeout)}
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return synthesisResult{err: ctx.Err()}
		}
		if errors.Is(err, context.Canceled) {
			return synthesisResult{err: context.Canceled}
		}
		return synthesisResult{err: segmentError(job.seq, "供应商调用失败")}
	}
	if len(audio) == 0 {
		return synthesisResult{err: segmentError(job.seq, "返回空音频")}
	}
	if len(audio) > maxTTSSegmentBytes {
		return synthesisResult{err: segmentError(job.seq, "单片音频超过 1MiB")}
	}
	mimeType, err := normalizeMP3MIME(contentType)
	if err != nil {
		return synthesisResult{err: segmentError(job.seq, "音频类型无效")}
	}
	if !validMP3(audio) {
		return synthesisResult{err: segmentError(job.seq, "MP3 数据无效")}
	}
	return synthesisResult{segment: AudioSegment{
		Seq: job.seq, Audio: audio, MIME: mimeType, SHA256: sha256.Sum256(audio), ByteLength: len(audio), deliveryText: job.text,
	}}
}

func segmentError(seq uint32, reason string) error {
	return fmt.Errorf("TTS segment %d failed: %s", seq, reason)
}

func segmentCauseError(seq uint32, cause error) error {
	return fmt.Errorf("TTS segment %d failed: %w", seq, cause)
}

func normalizeMP3MIME(raw string) (string, error) {
	mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("TTS Content-Type 无效")
	}
	switch strings.ToLower(mediaType) {
	case "audio/mpeg", "audio/mp3":
		return "audio/mpeg", nil
	default:
		return "", errors.New("TTS Content-Type 必须为 audio/mpeg")
	}
}

func validMP3(audio []byte) bool {
	offset := 0
	if len(audio) >= 3 && bytes.Equal(audio[:3], []byte("ID3")) {
		var ok bool
		offset, ok = id3v2End(audio)
		if !ok {
			return false
		}
	}
	frames := 0
	for offset < len(audio) {
		if len(audio)-offset == 128 && bytes.Equal(audio[offset:offset+3], []byte("TAG")) {
			offset += 128
			break
		}
		frameLength, ok := mpegLayerIIIFrameLength(audio[offset:])
		if !ok || frameLength > len(audio)-offset {
			return false
		}
		offset += frameLength
		frames++
	}
	return frames > 0 && offset == len(audio)
}

func id3v2End(audio []byte) (int, bool) {
	if len(audio) < 10 || audio[3] < 2 || audio[3] > 4 || audio[4] == 0xff {
		return 0, false
	}
	for _, value := range audio[6:10] {
		if value&0x80 != 0 {
			return 0, false
		}
	}
	size := int(audio[6])<<21 | int(audio[7])<<14 | int(audio[8])<<7 | int(audio[9])
	end := 10 + size
	if audio[3] == 4 && audio[5]&0x10 != 0 {
		end += 10
	}
	if end < 10 || end > len(audio) {
		return 0, false
	}
	return end, true
}

func mpegLayerIIIFrameLength(frame []byte) (int, bool) {
	if len(frame) < 4 || frame[0] != 0xff || frame[1]&0xe0 != 0xe0 {
		return 0, false
	}
	versionBits := (frame[1] >> 3) & 0x03
	if versionBits == 0x01 || (frame[1]>>1)&0x03 != 0x01 {
		return 0, false
	}
	bitrateIndex := (frame[2] >> 4) & 0x0f
	sampleRateIndex := (frame[2] >> 2) & 0x03
	if bitrateIndex == 0 || bitrateIndex == 0x0f || sampleRateIndex == 0x03 {
		return 0, false
	}

	mpeg1Bitrates := [...]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320}
	mpeg2Bitrates := [...]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160}
	mpeg1SampleRates := [...]int{44100, 48000, 32000}
	bitrateKbps := 0
	sampleRate := mpeg1SampleRates[sampleRateIndex]
	coefficient := 144
	switch versionBits {
	case 0x03: // MPEG-1
		bitrateKbps = mpeg1Bitrates[bitrateIndex]
	case 0x02: // MPEG-2
		bitrateKbps = mpeg2Bitrates[bitrateIndex]
		sampleRate /= 2
		coefficient = 72
	case 0x00: // MPEG-2.5
		bitrateKbps = mpeg2Bitrates[bitrateIndex]
		sampleRate /= 4
		coefficient = 72
	default:
		return 0, false
	}
	padding := int((frame[2] >> 1) & 0x01)
	frameLength := coefficient*bitrateKbps*1000/sampleRate + padding
	return frameLength, frameLength >= 4
}
