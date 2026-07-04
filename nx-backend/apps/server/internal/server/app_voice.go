package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/netguard"
)

var errASRNotConfigured = errors.New("语音识别未配置 ASR_API_BASE/ASR_API_KEY")

var newASRHTTPClient = func(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: netguard.NewGuardedTransport()}
}

// appVoiceRecognize 接收音频文件并调用语音识别服务
// POST /api/app/voice/recognize
func (s *Server) appVoiceRecognize(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	_ = userInfo.ID // 用户 ID，可用于日志记录或配额控制

	// 解析 multipart form，并硬限制请求体大小，避免大文件落盘。
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	err := r.ParseMultipartForm(10 << 20) // 10MB 限制
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "文件过大或格式错误")
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "未找到音频文件")
		return
	}
	defer file.Close()

	// 读取文件内容
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "读取文件失败")
		return
	}

	text, err := s.recognizeSpeech(r.Context(), fileBytes, header.Filename)
	if err != nil {
		if errors.Is(err, errASRNotConfigured) {
			httpx.Fail(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, fmt.Sprintf("识别失败: %v", err))
		return
	}

	httpx.OK(w, map[string]interface{}{
		"text": text,
	})
}

// recognizeSpeech 调用 OpenAI 兼容 ASR 网关，将音频转写为文本。
func (s *Server) recognizeSpeech(ctx context.Context, audioData []byte, filename string) (string, error) {
	cfg := s.env.ASR
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiBase == "" || apiKey == "" {
		return "", errASRNotConfigured
	}
	if len(audioData) == 0 {
		return "", fmt.Errorf("音频文件不能为空")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "whisper-1"
	}

	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", safeASRFilename(filename))
	if err != nil {
		return "", err
	}
	_, err = io.Copy(part, bytes.NewReader(audioData))
	if err != nil {
		return "", err
	}
	if err := writer.WriteField("model", model); err != nil {
		return "", err
	}
	err = writer.Close()
	if err != nil {
		return "", err
	}

	endpoint, err := buildASREndpoint(apiBase)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := newASRHTTPClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ASR 服务返回 HTTP %d: %s", resp.StatusCode, compactASRBody(raw))
	}
	text, err := parseASRText(raw)
	if err != nil {
		return "", err
	}
	return text, nil
}

func safeASRFilename(filename string) string {
	filename = strings.TrimSpace(filepath.Base(filename))
	if filename == "" || filename == "." || filename == string(filepath.Separator) {
		return "audio.wav"
	}
	return filename
}

func buildASREndpoint(apiBase string) (string, error) {
	apiBase = strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if apiBase == "" {
		return "", errASRNotConfigured
	}
	if strings.HasSuffix(apiBase, "/v1/audio/transcriptions") || strings.HasSuffix(apiBase, "/audio/transcriptions") {
		if _, err := url.ParseRequestURI(apiBase); err != nil {
			return "", fmt.Errorf("ASR_API_BASE 无效: %w", err)
		}
		return apiBase, nil
	}
	if strings.HasSuffix(apiBase, "/v1") {
		apiBase += "/audio/transcriptions"
	} else {
		apiBase += "/v1/audio/transcriptions"
	}
	if _, err := url.ParseRequestURI(apiBase); err != nil {
		return "", fmt.Errorf("ASR_API_BASE 无效: %w", err)
	}
	return apiBase, nil
}

func parseASRText(raw []byte) (string, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("解析 ASR 响应失败: %w", err)
	}
	text := strings.TrimSpace(findASRText(payload))
	if text == "" {
		return "", fmt.Errorf("ASR 响应未返回 text")
	}
	return text, nil
}

func findASRText(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"text", "transcript", "transcription", "result"} {
			if text, ok := typed[key].(string); ok && strings.TrimSpace(text) != "" {
				return text
			}
		}
		for _, key := range []string{"data", "result", "output"} {
			if nested, ok := typed[key]; ok {
				if text := findASRText(nested); text != "" {
					return text
				}
			}
		}
	case []any:
		for _, item := range typed {
			if text := findASRText(item); text != "" {
				return text
			}
		}
	}
	return ""
}

func compactASRBody(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "<empty>"
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 500 {
		return text[:500] + "..."
	}
	return text
}
