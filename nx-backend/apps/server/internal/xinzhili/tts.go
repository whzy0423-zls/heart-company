package xinzhili

import (
	"bytes"
	"context"
	"crypto/sha256"
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
)

// AudioSegment is one complete, independently playable MP3 sentence. Audio is
// never split at the byte level and is never persisted by this package.
type AudioSegment struct {
	Seq    uint32
	Text   string
	Audio  []byte
	MIME   string
	SHA256 [32]byte
}

// TTSProvider is deliberately stateless. Implementations return one complete
// audio file for one short sentence.
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
	default:
		return nil, errors.New("不支持的 TTS provider")
	}
}

type miniMaxTTSAdapter struct{ client MiniMaxTextToAudio }

func (p miniMaxTTSAdapter) Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error) {
	audio, mimeType, err := p.client.TextToAudioLimited(ctx, cfg.Model, cfg.Voice, text, maxTTSSegmentBytes)
	if err != nil {
		return nil, "", err
	}
	if len(audio) > maxTTSSegmentBytes {
		return nil, "", errors.New("MiniMax 音频超过 1MiB")
	}
	return audio, mimeType, nil
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
			if isStrongSentenceEnd(runes[i]) {
				cut = i + 1
				break
			}
		}
		if cut == 0 && len(runes) > maxTTSSentenceRunes {
			for i := limit - 1; i >= 0; i-- {
				if isWeakSentenceEnd(runes[i]) {
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

func isStrongSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '!', '?', ';', '；', '\n':
		return true
	case '.':
		return true
	default:
		return false
	}
}

func isWeakSentenceEnd(r rune) bool {
	if isStrongSentenceEnd(r) {
		return true
	}
	switch r {
	case '，', ',', '、', ':', '：':
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
	provider    TTSProvider
	concurrency int
}

func NewSynthesizer(provider TTSProvider, concurrency int) *Synthesizer {
	if concurrency <= 0 {
		concurrency = defaultTTSWorkers
	} else if concurrency > defaultTTSWorkers {
		concurrency = defaultTTSWorkers
	}
	return &Synthesizer{provider: provider, concurrency: concurrency}
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
	results := make(chan synthesisResult, s.concurrency)
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
					result := s.synthesizeOne(ctx, cfg, job)
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
	go func() {
		workers.Wait()
		close(results)
	}()

	pending := make(map[uint32]AudioSegment, workerCount)
	var next uint32
	var totalBytes int
	for result := range results {
		if result.err != nil {
			cancel()
			for range results {
			}
			if ctx.Err() != nil && errors.Is(result.err, context.Canceled) {
				return ctx.Err()
			}
			return result.err
		}
		if ctx.Err() != nil {
			cancel()
			for range results {
			}
			return ctx.Err()
		}
		pending[result.segment.Seq] = result.segment
		for {
			segment, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			totalBytes += len(segment.Audio)
			if totalBytes > maxTTSTurnBytes {
				cancel()
				for range results {
				}
				return errors.New("TTS 单轮音频超过 10MiB")
			}
			if err := emit(segment); err != nil {
				cancel()
				for range results {
				}
				return err
			}
			next++
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *Synthesizer) synthesizeOne(ctx context.Context, cfg TTSConfig, job synthesisJob) synthesisResult {
	audio, contentType, err := s.provider.Synthesize(ctx, cfg, job.text)
	if err != nil {
		if ctx.Err() != nil {
			return synthesisResult{err: ctx.Err()}
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
		Seq: job.seq, Text: job.text, Audio: audio, MIME: mimeType, SHA256: sha256.Sum256(audio),
	}}
}

func segmentError(seq uint32, reason string) error {
	return fmt.Errorf("TTS segment %d failed: %s", seq, reason)
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
	frameLength, ok := mpegLayerIIIFrameLength(audio[offset:])
	return ok && frameLength <= len(audio)-offset
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
