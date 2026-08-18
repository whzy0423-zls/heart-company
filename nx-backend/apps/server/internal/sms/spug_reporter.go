package sms

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type SpugReporterOptions struct {
	APIBase    string
	Token      string
	Path       string
	Channel    string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type SpugReporter struct {
	endpoint string
	channel  string
	timeout  time.Duration
	client   *http.Client
}

func NewSpugReporter(opts SpugReporterOptions) (*SpugReporter, error) {
	base := strings.TrimRight(strings.TrimSpace(opts.APIBase), "/")
	if base == "" {
		base = "https://push.spug.cc"
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("sms: invalid spug report api base")
	}
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		return nil, fmt.Errorf("sms: spug report token is required")
	}
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		path = "/send"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		path = "/send"
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	return &SpugReporter{
		endpoint: base + path + "/" + url.PathEscape(token),
		channel:  strings.TrimSpace(opts.Channel),
		timeout:  timeout,
		client:   client,
	}, nil
}

func (s *SpugReporter) Report(ctx context.Context, title, content, messageType, channel string) error {
	if s == nil {
		return nil
	}
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	if strings.TrimSpace(messageType) == "" {
		messageType = "text"
	}
	if strings.TrimSpace(channel) == "" {
		channel = s.channel
	}
	payload := struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Type    string `json:"type"`
		Channel string `json:"channel,omitempty"`
	}{
		Title: strings.TrimSpace(title), Content: strings.TrimSpace(content),
		Type: messageType, Channel: strings.TrimSpace(channel),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("sms: marshal spug report: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("sms: create spug report request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("sms: spug report failed: %w", err)
	}
	defer resp.Body.Close()
	responseBody := readProviderBody(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if responseBody == "" {
			responseBody = resp.Status
		}
		return fmt.Errorf("sms: spug report rejected: status=%d body=%s", resp.StatusCode, responseBody)
	}
	if responseBody == "" {
		return nil
	}
	var result struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return fmt.Errorf("sms: invalid spug report response: %w", err)
	}
	if result.Code != 0 && result.Code != http.StatusOK {
		msg := strings.TrimSpace(result.Msg)
		if msg == "" {
			msg = strings.TrimSpace(result.Message)
		}
		return fmt.Errorf("sms: spug report rejected: code=%d msg=%s", result.Code, msg)
	}
	return nil
}
