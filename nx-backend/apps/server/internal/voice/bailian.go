package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/netguard"
)

const (
	ProviderBailian = "bailian"
	ProviderMiniMax = "minimax"

	defaultBailianAPIBase     = "https://dashscope.aliyuncs.com"
	defaultBailianEnrollModel = "qwen-voice-enrollment"
	defaultBailianTargetModel = "qwen3-tts-vc-2026-01-22"
	bailianCustomizationPath  = "/api/v1/services/audio/tts/customization"
	bailianGenerationPath     = "/api/v1/services/aigc/multimodal-generation/generation"
)

type BailianConfig struct {
	APIBase     string
	APIKey      string
	TargetModel string
}

type BailianCloneInput struct {
	AudioURL    string
	ContentType string
	Data        []byte
	Filename    string
	TargetModel string
	VoiceID     string
}

type BailianClient struct {
	apiBase     string
	apiKey      string
	client      *http.Client
	targetModel string
}

func NewBailianClient(cfg BailianConfig) *BailianClient {
	apiBase := normalizeBailianAPIBase(cfg.APIBase)
	targetModel := strings.TrimSpace(cfg.TargetModel)
	if targetModel == "" || looksLikeLegacyMiniMaxModel(targetModel) {
		targetModel = defaultBailianTargetModel
	}
	return &BailianClient{
		apiBase:     apiBase,
		apiKey:      strings.TrimSpace(cfg.APIKey),
		targetModel: targetModel,
		client: &http.Client{
			Timeout:   120 * time.Second,
			Transport: netguard.NewGuardedTransport(),
		},
	}
}

func (c *BailianClient) CloneVoice(ctx context.Context, input BailianCloneInput) (string, error) {
	if c == nil || c.apiKey == "" {
		return "", errors.New("请先配置阿里百炼 API Key")
	}
	preferredName := normalizedBailianPreferredName(input.VoiceID)
	if preferredName == "" {
		preferredName = "nx_voice_" + randomID(10)
	}
	targetModel := strings.TrimSpace(input.TargetModel)
	if targetModel == "" {
		targetModel = c.targetModel
	}
	if isBailianHostedMiniMaxModel(targetModel) {
		return c.cloneMiniMaxVoice(ctx, targetModel, preferredName, input.AudioURL)
	}
	return c.cloneQwenVoice(ctx, targetModel, preferredName, input)
}

func (c *BailianClient) cloneMiniMaxVoice(ctx context.Context, model string, voiceID string, audioURL string) (string, error) {
	audioURL = strings.TrimSpace(audioURL)
	if !netguard.IsPublicHTTPURL(audioURL) {
		return "", errors.New("百炼托管 MiniMax 声音复刻需要已上传到公网可访问文件桶的音频公网 URL")
	}
	payload, _ := json.Marshal(map[string]any{
		"model": model,
		"input": map[string]any{
			"action":    "voice_clone",
			"voice_id":  voiceID,
			"audio_url": audioURL,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(bailianGenerationPath), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	result, err := c.doJSON(req)
	if err != nil {
		return "", err
	}
	finalVoiceID := findString(result, "data.voice_id", "output.voice_id", "voice_id")
	if finalVoiceID == "" {
		return voiceID, nil
	}
	return finalVoiceID, nil
}

func (c *BailianClient) cloneQwenVoice(ctx context.Context, targetModel string, preferredName string, input BailianCloneInput) (string, error) {
	if len(input.Data) == 0 {
		return "", errors.New("阿里百炼声音复刻需要音频样本")
	}
	contentType := strings.TrimSpace(input.ContentType)
	if contentType == "" {
		contentType = "audio/mpeg"
	}
	audioData := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(input.Data)
	payload, _ := json.Marshal(map[string]any{
		"model": defaultBailianEnrollModel,
		"input": map[string]any{
			"action":         "create",
			"target_model":   targetModel,
			"preferred_name": preferredName,
			"audio": map[string]any{
				"data": audioData,
			},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(bailianCustomizationPath), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	result, err := c.doJSON(req)
	if err != nil {
		return "", err
	}
	voiceID := findString(result, "output.voice", "output.voice_id", "voice", "voice_id")
	if voiceID == "" {
		return "", errors.New("阿里百炼未返回最终音色 ID")
	}
	return voiceID, nil
}

func (c *BailianClient) TextToAudio(ctx context.Context, model string, voiceID string, text string) ([]byte, string, error) {
	if c == nil || c.apiKey == "" {
		return nil, "", errors.New("请先配置阿里百炼 API Key")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = c.targetModel
	}
	input := map[string]any{"text": text}
	if isBailianHostedMiniMaxModel(model) {
		input["voice_setting"] = map[string]any{
			"voice_id": strings.TrimSpace(voiceID),
			"speed":    1,
			"vol":      1,
			"pitch":    0,
		}
		input["audio_setting"] = map[string]any{
			"sample_rate": 32000,
			"bitrate":     128000,
			"format":      "mp3",
			"channel":     1,
		}
	} else if isBailianQwenVoiceCloneModel(model) {
		input["voice"] = strings.TrimSpace(voiceID)
	} else {
		return nil, "", fmt.Errorf("当前百炼声音测试不支持模型 %s", model)
	}
	payload, _ := json.Marshal(map[string]any{"model": model, "input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(bailianGenerationPath), bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	result, err := c.doJSON(req)
	if err != nil {
		return nil, "", err
	}
	audioValue := findString(result,
		"data.audio", "data.audio_url", "data.url",
		"output.audio", "output.audio.data", "output.audio.url", "output.audio_url", "output.url",
		"output.data.audio", "output.data.audio_url", "output.data.url",
		"audio", "audio_url", "url",
	)
	if audioValue == "" {
		return nil, "", errors.New("阿里百炼未返回音频数据")
	}
	audio, contentType, err := c.decodeOrDownloadAudio(ctx, audioValue)
	if err != nil {
		return nil, "", err
	}
	return NormalizeTTSMP3(ctx, audio, contentType, 20*1024*1024)
}

// NormalizeTTSMP3 keeps the existing WebSocket/audio-asset MP3 contract while
// accepting Qwen's documented WAV result. MiniMax and already-MP3 responses
// pass through unchanged.
func NormalizeTTSMP3(ctx context.Context, audio []byte, contentType string, maxBytes int64) ([]byte, string, error) {
	if len(audio) == 0 {
		return nil, "", errors.New("阿里百炼未返回音频数据")
	}
	if !looksLikeWAV(audio) {
		return audio, contentType, nil
	}
	command := exec.CommandContext(ctx,
		"ffmpeg", "-hide_banner", "-loglevel", "error",
		"-i", "pipe:0", "-vn", "-codec:a", "libmp3lame", "-b:a", "128k",
		"-f", "mp3", "pipe:1",
	)
	command.Stdin = bytes.NewReader(audio)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, "", ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, "", errors.New("Qwen 音频转换需要服务端安装 ffmpeg")
		}
		return nil, "", errors.New("Qwen 音频转换失败")
	}
	if stdout.Len() == 0 || (maxBytes > 0 && int64(stdout.Len()) > maxBytes) {
		return nil, "", errors.New("Qwen 音频转换结果大小无效")
	}
	return stdout.Bytes(), "audio/mpeg", nil
}

func looksLikeWAV(audio []byte) bool {
	return len(audio) >= 12 && string(audio[:4]) == "RIFF" && string(audio[8:12]) == "WAVE"
}

func (c *BailianClient) decodeOrDownloadAudio(ctx context.Context, raw string) ([]byte, string, error) {
	value := strings.TrimSpace(raw)
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if !netguard.IsPublicHTTPURL(value) {
			return nil, "", errors.New("阿里百炼返回了不安全的音频地址")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, value, nil)
		if err != nil {
			return nil, "", err
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, "", fmt.Errorf("下载阿里百炼音频失败(%d)", resp.StatusCode)
		}
		audio, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024+1))
		if err != nil {
			return nil, "", err
		}
		if len(audio) == 0 || len(audio) > 20*1024*1024 {
			return nil, "", errors.New("阿里百炼音频大小无效")
		}
		contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
		if contentType == "" || contentType == "application/octet-stream" || contentType == "audio/mp3" {
			contentType = "audio/mpeg"
		}
		return audio, contentType, nil
	}
	if comma := strings.Index(value, ","); strings.HasPrefix(lower, "data:audio/") && comma > 0 && strings.Contains(lower[:comma], ";base64") {
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value[comma+1:]))
		return decoded, "audio/mpeg", err
	}
	compact := strings.Join(strings.Fields(value), "")
	if len(compact)%2 == 0 {
		if decoded, err := hex.DecodeString(compact); err == nil && len(decoded) > 0 {
			return decoded, "audio/mpeg", nil
		}
	}
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil || len(decoded) == 0 {
		return nil, "", errors.New("阿里百炼音频格式无法识别")
	}
	return decoded, "audio/mpeg", nil
}

func (c *BailianClient) endpoint(endpointPath string) string {
	base := strings.TrimRight(c.apiBase, "/")
	if strings.HasSuffix(base, endpointPath) {
		return base
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimRight(defaultBailianAPIBase, "/") + endpointPath
	}
	if strings.Contains(u.Path, "/compatible-mode/") {
		u.Path = ""
	}
	if strings.Contains(u.Path, "/api/v1/services/") {
		u.Path = u.Path[:strings.Index(u.Path, "/api/v1/services/")]
	}
	basePath := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(basePath, "/api/v1") && strings.HasPrefix(endpointPath, "/api/v1/") {
		u.Path = basePath + strings.TrimPrefix(endpointPath, "/api/v1")
	} else {
		u.Path = path.Join(basePath, endpointPath)
	}
	return u.String()
}

func (c *BailianClient) doJSON(req *http.Request) (map[string]any, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	var payload map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &payload)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("阿里百炼请求失败(%d): %s", resp.StatusCode, compactBody(raw))
	}
	if err := bailianBaseError(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func bailianBaseError(payload map[string]any) error {
	code := findString(payload, "code", "Code", "output.base_resp.status_code", "data.base_resp.status_code")
	if code == "" || strings.EqualFold(code, "Success") || code == "0" {
		return nil
	}
	message := findString(payload, "message", "Message", "output.base_resp.status_msg", "data.base_resp.status_msg")
	if message == "" {
		message = "阿里百炼调用失败"
	}
	return fmt.Errorf("%s: %s", code, message)
}

func normalizeBailianAPIBase(raw string) string {
	value := strings.TrimRight(strings.TrimSpace(raw), "/")
	if value == "" || strings.Contains(value, "api.minimaxi.com") {
		return defaultBailianAPIBase
	}
	return value
}

func normalizeProfileProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "":
		return ProviderMiniMax
	case "aliyun", "aliyun-bailian", "dashscope":
		return ProviderBailian
	case ProviderMiniMax:
		return ProviderMiniMax
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

var bailianPreferredNameInvalid = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func normalizedBailianPreferredName(raw string) string {
	value := strings.Trim(bailianPreferredNameInvalid.ReplaceAllString(strings.TrimSpace(raw), "_"), "_-")
	if value == "" {
		return ""
	}
	if len(value) > 40 {
		return value[:40]
	}
	return value
}

func looksLikeLegacyMiniMaxModel(model string) bool {
	value := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(value, "speech-") || strings.HasPrefix(value, "abab")
}

func isBailianHostedMiniMaxModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "minimax/")
}

func isBailianQwenVoiceCloneModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "qwen3-tts-vc-")
}

func appendCloneVoiceOptions(options []VoiceOption, profiles []Profile) []VoiceOption {
	for _, profile := range profiles {
		if strings.TrimSpace(profile.Status) != "ready" || strings.TrimSpace(profile.VoiceID) == "" {
			continue
		}
		options = append(options, VoiceOption{
			ID:        "clone:" + profile.ID,
			Label:     profile.Name + "（克隆）",
			Model:     profile.Model,
			Provider:  strings.TrimSpace(profile.Provider),
			Source:    "clone",
			VoiceID:   profile.VoiceID,
			VoiceName: profile.Name,
		})
	}
	return options
}
