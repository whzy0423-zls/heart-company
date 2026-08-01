package xinzhili

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const aliyunCosyVoiceMaxJSONBytes = 1 << 20

type aliyunCosyVoiceTTS struct {
	endpoint string
	dialer   *websocket.Dialer
}

type aliyunTTSServerMessage struct {
	Header struct {
		Event        string `json:"event"`
		TaskID       string `json:"task_id"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"header"`
}

func newAliyunCosyVoiceTTS(cfg TTSConfig) (TTSProvider, error) {
	endpoint, err := aliyunCosyVoiceEndpoint(cfg.Endpoint)
	if err != nil {
		return nil, err
	}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	return &aliyunCosyVoiceTTS{endpoint: endpoint, dialer: &dialer}, nil
}

func (p *aliyunCosyVoiceTTS) Synthesize(ctx context.Context, cfg TTSConfig, text string) ([]byte, string, error) {
	if err := validateAliyunCosyVoiceRuntimeConfig(cfg); err != nil {
		return nil, "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, "", errors.New("阿里 CosyVoice TTS 文本不能为空")
	}
	taskID, err := newAliyunTTSTaskID()
	if err != nil {
		return nil, "", fmt.Errorf("生成阿里 CosyVoice 任务 ID: %w", err)
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.APIKey))
	if workspace := strings.TrimSpace(cfg.GroupID); workspace != "" {
		header.Set("X-DashScope-WorkSpace", workspace)
	}
	dialCtx, cancelDial := context.WithTimeout(ctx, 10*time.Second)
	defer cancelDial()
	conn, resp, err := p.dialer.DialContext(dialCtx, p.endpoint, header)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		if errors.Is(dialCtx.Err(), context.DeadlineExceeded) {
			return nil, "", fmt.Errorf("%w: 阿里 CosyVoice WebSocket 握手", ErrTTSTimeout)
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return nil, "", fmt.Errorf("%w: 阿里 CosyVoice WebSocket 握手: %w", ErrTTSTimeout, err)
		}
		return nil, "", fmt.Errorf("连接阿里 CosyVoice TTS 上游: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(maxTTSSegmentBytes + aliyunCosyVoiceMaxJSONBytes)

	if err := writeAliyunTTSJSON(ctx, conn, aliyunTTSRunTaskMessage(taskID, cfg)); err != nil {
		return nil, "", err
	}
	if err := waitAliyunTTSEvent(ctx, conn, taskID, "task-started"); err != nil {
		return nil, "", err
	}
	if err := writeAliyunTTSJSON(ctx, conn, aliyunTTSContinueTaskMessage(taskID, text)); err != nil {
		return nil, "", err
	}
	if err := writeAliyunTTSJSON(ctx, conn, aliyunTTSFinishTaskMessage(taskID)); err != nil {
		return nil, "", err
	}
	audio, err := readAliyunTTSAudio(ctx, conn, taskID)
	if err != nil {
		return nil, "", err
	}
	if len(audio) == 0 {
		return nil, "", errors.New("阿里 CosyVoice TTS 返回空音频")
	}
	if len(audio) > maxTTSSegmentBytes {
		return nil, "", errors.New("阿里 CosyVoice TTS 单片音频超过 1MiB")
	}
	if !validMP3(audio) {
		return nil, "", errors.New("阿里 CosyVoice TTS 返回的音频格式无效")
	}
	return audio, "audio/mpeg", nil
}

func validateAliyunCosyVoiceRuntimeConfig(cfg TTSConfig) error {
	if strings.TrimSpace(cfg.Provider) != TTSProviderAliyunCosyVoice {
		return fmt.Errorf("阿里 CosyVoice TTS provider 必须为 %s", TTSProviderAliyunCosyVoice)
	}
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" || strings.TrimSpace(cfg.Voice) == "" {
		return errors.New("阿里 CosyVoice TTS API Key、模型和音色不能为空")
	}
	if strings.TrimSpace(cfg.Format) != "mp3" {
		return errors.New("阿里 CosyVoice TTS format 必须为 mp3")
	}
	_, err := aliyunCosyVoiceEndpoint(cfg.Endpoint)
	return err
}

func aliyunCosyVoiceEndpoint(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return "", errors.New("阿里 CosyVoice TTS endpoint 无效")
	}
	switch parsed.Scheme {
	case "wss", "ws":
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", errors.New("阿里 CosyVoice TTS endpoint 必须使用 wss、ws 或 https")
	}
	return parsed.String(), nil
}

func aliyunTTSRunTaskMessage(taskID string, cfg TTSConfig) any {
	return map[string]any{
		"header": map[string]any{"action": "run-task", "task_id": taskID, "streaming": "duplex"},
		"payload": map[string]any{
			"task_group": "audio", "task": "tts", "function": "SpeechSynthesizer", "model": strings.TrimSpace(cfg.Model),
			"parameters": map[string]any{"text_type": "PlainText", "voice": strings.TrimSpace(cfg.Voice), "format": "mp3"},
			"input":      map[string]any{},
		},
	}
}

func aliyunTTSContinueTaskMessage(taskID, text string) any {
	return map[string]any{
		"header":  map[string]any{"action": "continue-task", "task_id": taskID, "streaming": "duplex"},
		"payload": map[string]any{"input": map[string]any{"text": text}},
	}
}

func aliyunTTSFinishTaskMessage(taskID string) any {
	return map[string]any{
		"header":  map[string]any{"action": "finish-task", "task_id": taskID, "streaming": "duplex"},
		"payload": map[string]any{"input": map[string]any{}},
	}
}

func writeAliyunTTSJSON(ctx context.Context, conn *websocket.Conn, value any) error {
	if err := setAliyunTTSWriteDeadline(ctx, conn); err != nil {
		return err
	}
	if err := conn.WriteJSON(value); err != nil {
		return mapAliyunTTSIOError(ctx, "写入上游", err)
	}
	return nil
}

func waitAliyunTTSEvent(ctx context.Context, conn *websocket.Conn, taskID, wanted string) error {
	for {
		messageType, data, err := readAliyunTTSMessage(ctx, conn)
		if err != nil {
			return err
		}
		if messageType != websocket.TextMessage {
			continue
		}
		message, err := parseAliyunTTSServerMessage(data)
		if err != nil {
			return err
		}
		if err := validateAliyunTTSServerTask(message, taskID); err != nil {
			return err
		}
		if message.Header.Event == wanted {
			return nil
		}
		if message.Header.Event == "task-failed" {
			return aliyunTTSTaskFailed(message)
		}
	}
}

func readAliyunTTSAudio(ctx context.Context, conn *websocket.Conn, taskID string) ([]byte, error) {
	var audio bytes.Buffer
	for {
		messageType, data, err := readAliyunTTSMessage(ctx, conn)
		if err != nil {
			return nil, err
		}
		switch messageType {
		case websocket.BinaryMessage:
			if audio.Len()+len(data) > maxTTSSegmentBytes {
				return nil, errors.New("阿里 CosyVoice TTS 单片音频超过 1MiB")
			}
			audio.Write(data)
		case websocket.TextMessage:
			message, err := parseAliyunTTSServerMessage(data)
			if err != nil {
				return nil, err
			}
			if err := validateAliyunTTSServerTask(message, taskID); err != nil {
				return nil, err
			}
			switch message.Header.Event {
			case "task-finished":
				return audio.Bytes(), nil
			case "task-failed":
				return nil, aliyunTTSTaskFailed(message)
			}
		}
	}
}

func readAliyunTTSMessage(ctx context.Context, conn *websocket.Conn) (int, []byte, error) {
	if err := setAliyunTTSReadDeadline(ctx, conn); err != nil {
		return 0, nil, err
	}
	messageType, data, err := conn.ReadMessage()
	if err != nil {
		return 0, nil, mapAliyunTTSIOError(ctx, "读取上游", err)
	}
	return messageType, data, nil
}

func parseAliyunTTSServerMessage(data []byte) (aliyunTTSServerMessage, error) {
	var message aliyunTTSServerMessage
	if err := json.Unmarshal(data, &message); err != nil {
		return message, fmt.Errorf("阿里 CosyVoice TTS 协议消息无效: %w", err)
	}
	if message.Header.Event == "" {
		return message, errors.New("阿里 CosyVoice TTS 协议消息缺少 event")
	}
	return message, nil
}

func validateAliyunTTSServerTask(message aliyunTTSServerMessage, taskID string) error {
	if message.Header.TaskID != "" && message.Header.TaskID != taskID {
		return fmt.Errorf("阿里 CosyVoice TTS task_id 不匹配: %s", message.Header.TaskID)
	}
	return nil
}

func aliyunTTSTaskFailed(message aliyunTTSServerMessage) error {
	code := strings.TrimSpace(message.Header.ErrorCode)
	if code == "" {
		code = "task_failed"
	}
	return fmt.Errorf("阿里 CosyVoice TTS 任务失败: %s", code)
}

func setAliyunTTSWriteDeadline(ctx context.Context, conn *websocket.Conn) error {
	return setAliyunTTSDeadline(ctx, func(deadline time.Time) error { return conn.SetWriteDeadline(deadline) })
}

func setAliyunTTSReadDeadline(ctx context.Context, conn *websocket.Conn) error {
	return setAliyunTTSDeadline(ctx, func(deadline time.Time) error { return conn.SetReadDeadline(deadline) })
}

func setAliyunTTSDeadline(ctx context.Context, apply func(time.Time) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline := time.Now().Add(defaultTTSTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := apply(deadline); err != nil {
		return fmt.Errorf("设置阿里 CosyVoice TTS 超时: %w", err)
	}
	return nil
}

func mapAliyunTTSIOError(ctx context.Context, phase string, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %s", ErrTTSTimeout, phase)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("%w: %s: %w", ErrTTSTimeout, phase, err)
	}
	return fmt.Errorf("阿里 CosyVoice TTS %s失败: %w", phase, err)
}

func newAliyunTTSTaskID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	var encoded [36]byte
	hex.Encode(encoded[0:8], raw[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], raw[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], raw[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], raw[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], raw[10:16])
	return string(encoded[:]), nil
}
