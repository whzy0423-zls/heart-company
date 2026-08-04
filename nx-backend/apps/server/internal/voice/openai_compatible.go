package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

type CompatibleSpeechClient struct {
	config modelconfig.XinzhiliVoiceConfig
	client *http.Client
}

func NewCompatibleSpeechClient(cfg modelconfig.XinzhiliVoiceConfig) *CompatibleSpeechClient {
	return &CompatibleSpeechClient{config: cfg, client: &http.Client{}}
}

func (c *CompatibleSpeechClient) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	cfg := c.config.ASR
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(audio); err != nil {
		return "", err
	}
	_ = writer.WriteField("model", strings.TrimSpace(cfg.Model))
	if language := strings.TrimSpace(cfg.Language); language != "" {
		_ = writer.WriteField("language", language)
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	requestCtx, cancel := withSpeechTimeout(ctx, cfg.TimeoutSeconds, 30)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, speechEndpoint(cfg.APIBase, "audio/transcriptions"), &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", speechHTTPError(resp)
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return "", err
	}
	return strings.TrimSpace(payload.Text), nil
}

func (c *CompatibleSpeechClient) Synthesize(ctx context.Context, text string) ([]byte, string, error) {
	cfg := c.config.TTS
	payload := map[string]any{
		"model":           strings.TrimSpace(cfg.Model),
		"voice":           strings.TrimSpace(cfg.Voice),
		"input":           NormalizeChineseTTSInput(text),
		"response_format": defaultString(strings.ToLower(strings.TrimSpace(cfg.ResponseFormat)), "mp3"),
		"speed":           defaultSpeed(cfg.Speed),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	requestCtx, cancel := withSpeechTimeout(ctx, cfg.TimeoutSeconds, 45)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, speechEndpoint(cfg.APIBase, "audio/speech"), bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", speechHTTPError(resp)
	}
	audio, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, "", err
	}
	if len(audio) == 0 {
		return nil, "", fmt.Errorf("speech service returned empty audio")
	}
	contentType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if contentType == "" {
		contentType = contentTypeForFormat(cfg.ResponseFormat)
	}
	return audio, contentType, nil
}

func speechEndpoint(apiBase, suffix string) string {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	return base + "/" + path.Clean("/" + suffix)[1:]
}

func withSpeechTimeout(ctx context.Context, seconds, fallback int) (context.Context, context.CancelFunc) {
	if seconds <= 0 {
		seconds = fallback
	}
	return context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
}

func speechHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = resp.Status
	}
	return fmt.Errorf("speech service returned %d: %s", resp.StatusCode, message)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func defaultSpeed(value float64) float64 {
	if value <= 0 {
		return 1
	}
	return value
}

func contentTypeForFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "wav":
		return "audio/wav"
	case "opus":
		return "audio/opus"
	case "aac":
		return "audio/aac"
	default:
		return "audio/mpeg"
	}
}
