package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/netguard"
)

type StructuredJSONConfig struct {
	Provider string
	APIBase  string
	APIKey   string
	Model    string
	Timeout  time.Duration
	Client   *http.Client
}

type StructuredJSONRequest struct {
	SystemPrompt string
	UserPrompt   string
	MaxTokens    int
	Temperature  float64
}

type StructuredJSONResult struct {
	Provider    string
	Model       string
	Content     string
	RawResponse string
}

type StructuredJSONClient struct {
	cfg    StructuredJSONConfig
	client *http.Client
}

func NewStructuredJSONClient(cfg StructuredJSONConfig) *StructuredJSONClient {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.APIBase = strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout, Transport: netguard.NewGuardedTransport()}
	}
	return &StructuredJSONClient{cfg: cfg, client: client}
}

func (c *StructuredJSONClient) GenerateJSON(ctx context.Context, req StructuredJSONRequest) (StructuredJSONResult, error) {
	provider := c.cfg.Provider
	if provider == "" {
		provider = "openai-compatible"
	}
	switch provider {
	case "openai", "openai-compatible", "newapi":
		return c.generateOpenAICompatible(ctx, req)
	case "anthropic", "anthropic-compatible":
		return StructuredJSONResult{}, fmt.Errorf("provider %s 暂未实现", provider)
	default:
		return StructuredJSONResult{}, fmt.Errorf("unsupported structured json provider %s", provider)
	}
}

func (c *StructuredJSONClient) generateOpenAICompatible(ctx context.Context, req StructuredJSONRequest) (StructuredJSONResult, error) {
	if c.cfg.APIKey == "" {
		return StructuredJSONResult{}, fmt.Errorf("请先配置管理端大模型 API Key")
	}
	apiBase := c.cfg.APIBase
	if apiBase == "" {
		apiBase = "https://api.openai.com"
	}
	model := c.cfg.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	temperature := req.Temperature
	if temperature <= 0 {
		temperature = 0.2
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	body := map[string]any{
		"model":       model,
		"temperature": temperature,
		"max_tokens":  maxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	payload, _ := json.Marshal(body)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+"/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return StructuredJSONResult{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return StructuredJSONResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return StructuredJSONResult{}, fmt.Errorf("OpenAI 兼容模型请求失败(%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return StructuredJSONResult{}, err
	}
	content := findString(result, "choices.0.message.content", "choices.0.text")
	if strings.TrimSpace(content) == "" {
		return StructuredJSONResult{}, fmt.Errorf("OpenAI 兼容模型未返回 JSON 内容")
	}
	return StructuredJSONResult{
		Provider:    c.cfg.Provider,
		Model:       model,
		Content:     strings.TrimSpace(content),
		RawResponse: string(raw),
	}, nil
}
