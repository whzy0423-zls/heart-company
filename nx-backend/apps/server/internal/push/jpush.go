package push

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
)

const jpushAPIURL = "https://api.jpush.cn/v3/push"

type BatchPusher struct {
	batchSize int
	inner     Pusher
}

func NewBatchPusher(inner Pusher, batchSize int) Pusher {
	if batchSize <= 0 {
		batchSize = 1000
	}
	return BatchPusher{batchSize: batchSize, inner: inner}
}

func (p BatchPusher) Push(ctx context.Context, registrationIDs []string, msg Message) (PushResult, error) {
	if len(registrationIDs) == 0 {
		return PushResult{}, nil
	}
	if p.inner == nil {
		return PushResult{}, errors.New("push sender is not configured")
	}
	total := 0
	msgIDs := make([]string, 0, (len(registrationIDs)+p.batchSize-1)/p.batchSize)
	for start := 0; start < len(registrationIDs); start += p.batchSize {
		end := start + p.batchSize
		if end > len(registrationIDs) {
			end = len(registrationIDs)
		}
		result, err := p.inner.Push(ctx, registrationIDs[start:end], msg)
		if err != nil {
			return PushResult{MsgID: strings.Join(msgIDs, ","), Sent: total}, err
		}
		total += result.Sent
		if result.MsgID != "" {
			msgIDs = append(msgIDs, result.MsgID)
		}
	}
	return PushResult{MsgID: strings.Join(msgIDs, ","), Sent: total}, nil
}

// JPushClient 极光推送 REST API v3 客户端。
type JPushClient struct {
	appKey       string
	masterSecret string
	httpClient   *http.Client
}

func NewJPushClient(appKey, masterSecret string) *JPushClient {
	return &JPushClient{
		appKey:       appKey,
		masterSecret: masterSecret,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *JPushClient) Push(ctx context.Context, registrationIDs []string, msg Message) (PushResult, error) {
	if len(registrationIDs) == 0 {
		return PushResult{}, nil
	}

	payload := jpushPayload{
		Platform: "all",
		Audience: jpushAudience{RegistrationID: registrationIDs},
		Notification: &jpushNotification{
			Android: &jpushAndroid{
				Alert: msg.Content,
				Title: msg.Title,
			},
			IOS: &jpushIOS{
				Alert: msg.Content,
				Sound: "default",
			},
		},
	}
	if msg.DeepLink != "" {
		extras := map[string]string{"deep_link": msg.DeepLink}
		payload.Notification.Android.Extras = extras
		payload.Notification.IOS.Extras = extras
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PushResult{}, fmt.Errorf("jpush: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, jpushAPIURL, bytes.NewReader(body))
	if err != nil {
		return PushResult{}, fmt.Errorf("jpush: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+c.basicAuth())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return PushResult{}, fmt.Errorf("jpush: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return PushResult{}, fmt.Errorf("jpush: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result jpushResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return PushResult{}, fmt.Errorf("jpush: decode response: %w", err)
	}

	return PushResult{
		MsgID: fmt.Sprintf("%d", result.MsgID),
		Sent:  len(registrationIDs),
	}, nil
}

func (c *JPushClient) basicAuth() string {
	return base64.StdEncoding.EncodeToString([]byte(c.appKey + ":" + c.masterSecret))
}

// NoopPusher 空推送实现，AppKey 未配置时使用。
type NoopPusher struct{}

func (NoopPusher) Push(_ context.Context, registrationIDs []string, msg Message) (PushResult, error) {
	log.Printf("[push/noop] title=%q content=%q targets=%d", msg.Title, msg.Content, len(registrationIDs))
	return PushResult{MsgID: "noop", Sent: len(registrationIDs)}, nil
}

type DisabledPusher struct{}

func (DisabledPusher) Push(context.Context, []string, Message) (PushResult, error) {
	return PushResult{}, errors.New("push sender is not configured")
}

// NewPusher 根据配置返回真实或 noop 推送客户端。
func NewPusher(appEnv, appKey, masterSecret string) Pusher {
	if appKey == "" || masterSecret == "" {
		if config.IsProduction(appEnv) {
			log.Println("[push] JPush AppKey 未配置，生产环境推送将失败关闭")
			return DisabledPusher{}
		}
		log.Println("[push] JPush AppKey 未配置，非生产环境推送将仅写日志")
		return NoopPusher{}
	}
	return NewBatchPusher(NewJPushClient(appKey, masterSecret), 1000)
}

// --- JPush API 请求/响应结构 ---

type jpushPayload struct {
	Platform     string             `json:"platform"`
	Audience     jpushAudience      `json:"audience"`
	Notification *jpushNotification `json:"notification,omitempty"`
}

type jpushAudience struct {
	RegistrationID []string `json:"registration_id"`
}

type jpushNotification struct {
	Android *jpushAndroid `json:"android,omitempty"`
	IOS     *jpushIOS     `json:"ios,omitempty"`
}

type jpushAndroid struct {
	Alert  string            `json:"alert"`
	Title  string            `json:"title"`
	Extras map[string]string `json:"extras,omitempty"`
}

type jpushIOS struct {
	Alert  string            `json:"alert"`
	Sound  string            `json:"sound,omitempty"`
	Extras map[string]string `json:"extras,omitempty"`
}

type jpushResponse struct {
	MsgID  int64 `json:"msg_id"`
	SendNo int64 `json:"sendno"`
}
