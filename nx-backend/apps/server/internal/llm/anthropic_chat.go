package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

const (
	anthropicChatDefaultAPIBase = "https://api.anthropic.com/v1"
	anthropicChatDefaultModel   = "claude-3-5-haiku-latest"
	anthropicChatVersion        = "2023-06-01"
	anthropicChatMaxBodyBytes   = 2 * 1024 * 1024
	anthropicChatMaxTokens      = 360
)

var errAnthropicStreamTerminal = errors.New("anthropic stream terminal event")

// AnthropicChatGenerator speaks the Anthropic Messages protocol directly.
// It intentionally does not translate requests or stream events through
// OpenAI-compatible structures.
type AnthropicChatGenerator struct {
	apiBase      string
	apiKey       string
	client       *http.Client
	model        string
	systemPrompt string
}

type anthropicChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicChatRequest struct {
	Model       string                 `json:"model"`
	System      string                 `json:"system,omitempty"`
	Messages    []anthropicChatMessage `json:"messages"`
	MaxTokens   int                    `json:"max_tokens"`
	Temperature float64                `json:"temperature"`
	Stream      bool                   `json:"stream,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicError struct {
	Type    string `json:"type,omitempty"`
	Message string `json:"message"`
}

type anthropicChatResponse struct {
	Type    string                  `json:"type"`
	Content []anthropicContentBlock `json:"content"`
	Error   *anthropicError         `json:"error,omitempty"`
}

func newAnthropicChatGenerator(cfg ChatGeneratorConfig, client *http.Client) *AnthropicChatGenerator {
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if apiBase == "" {
		apiBase = anthropicChatDefaultAPIBase
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = anthropicChatDefaultModel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &AnthropicChatGenerator{
		apiBase:      apiBase,
		apiKey:       strings.TrimSpace(cfg.APIKey),
		client:       client,
		model:        model,
		systemPrompt: strings.TrimSpace(cfg.SystemPrompt),
	}
}

func (g *AnthropicChatGenerator) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	return g.complete(ctx, anthropicChatRequest{
		Model:       g.model,
		System:      resolveCompatibleChatSystemPrompt(g.systemPrompt),
		Messages:    g.chatMessages(input),
		MaxTokens:   anthropicChatMaxTokens,
		Temperature: 0.55,
	})
}

func (g *AnthropicChatGenerator) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if err := g.requireAPIKey(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(anthropicChatRequest{
		Model:       g.model,
		System:      resolveCompatibleChatSystemPrompt(g.systemPrompt),
		Messages:    g.chatMessages(input),
		MaxTokens:   anthropicChatMaxTokens,
		Temperature: 0.55,
		Stream:      true,
	})
	if err != nil {
		return "", err
	}
	req, err := g.newRequest(ctx, payload)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/event-stream")

	streamClient := *g.client
	streamClient.Timeout = 0
	resp, err := streamClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", anthropicStatusError(resp)
	}

	var answer strings.Builder
	terminal := false
	err = readSSE(ctx, resp.Body, func(event sseEvent) error {
		data := strings.TrimSpace(event.Data)
		if data == "" {
			return nil
		}
		var payload struct {
			Type  string `json:"type"`
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text,omitempty"`
			} `json:"delta"`
			Error *anthropicError `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return fmt.Errorf("Anthropic 流响应解析失败: %w", err)
		}
		eventType := strings.TrimSpace(payload.Type)
		if eventType == "" {
			eventType = strings.TrimSpace(event.Event)
		}
		switch eventType {
		case "error":
			return anthropicResponseError(payload.Error)
		case "content_block_delta":
			if payload.Delta.Type != "text_delta" || payload.Delta.Text == "" {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if emit != nil {
				if err := emit(payload.Delta.Text); err != nil {
					return err
				}
			}
			answer.WriteString(payload.Delta.Text)
		case "message_stop":
			terminal = true
			return errAnthropicStreamTerminal
		case "message_start", "content_block_start", "content_block_stop", "message_delta", "ping":
			return nil
		default:
			// Ignore unknown event types for forward compatibility. Only native
			// text_delta events are user-visible.
			return nil
		}
		return nil
	})
	if err != nil && !errors.Is(err, errAnthropicStreamTerminal) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", err
	}
	if !terminal {
		return "", fmt.Errorf("Anthropic 流响应缺少 message_stop 终止事件")
	}
	result := strings.TrimSpace(answer.String())
	if result == "" {
		return "", fmt.Errorf("Anthropic 未返回文本回答")
	}
	return result, nil
}

func (g *AnthropicChatGenerator) SummarizeConversation(ctx context.Context, previousSummary string, messages []rag.Message) (string, error) {
	if len(messages) == 0 {
		return strings.TrimSpace(previousSummary), nil
	}
	var prompt strings.Builder
	prompt.WriteString("已有会话摘要：\n")
	previousSummary = strings.TrimSpace(previousSummary)
	if previousSummary == "" {
		prompt.WriteString("暂无。\n")
	} else {
		prompt.WriteString(trimRunes(previousSummary, 1200) + "\n")
	}
	prompt.WriteString("新增对话：\n")
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		prompt.WriteString(role + ": " + trimRunes(content, 220) + "\n")
	}
	prompt.WriteString("请输出合并后的会话摘要，只输出摘要正文。")

	result, err := g.complete(ctx, anthropicChatRequest{
		Model:  g.model,
		System: "你负责压缩会话摘要。必须保留参与人物及关系、已确认事实和事件、用户诉求与边界、关键建议与反馈、尚未解决的问题；删除寒暄、重复表达和无关细节。摘要应准确、简洁，不得添加对话中不存在的信息。",
		Messages: []anthropicChatMessage{
			{Role: "user", Content: prompt.String()},
		},
		MaxTokens:   700,
		Temperature: 0.2,
	})
	if err != nil {
		return "", err
	}
	return trimRunes(result, 1200), nil
}

func (g *AnthropicChatGenerator) PolishPrompt(ctx context.Context, draft, kind string) (string, error) {
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return "", fmt.Errorf("请先填写提示词方向或草稿")
	}
	return g.complete(ctx, anthropicChatRequest{
		Model:  g.model,
		System: polishSystemPrompt(kind),
		Messages: []anthropicChatMessage{
			{Role: "user", Content: "请润色以下" + polishKindLabel(kind) + "提示词方向，只输出润色后的提示词正文：\n" + draft},
		},
		MaxTokens:   600,
		Temperature: 0.7,
	})
}

func (g *AnthropicChatGenerator) CompleteJSON(ctx context.Context, system, user string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	return g.complete(ctx, anthropicChatRequest{
		Model:  g.model,
		System: system,
		Messages: []anthropicChatMessage{
			{Role: "user", Content: user},
		},
		MaxTokens:   maxTokens,
		Temperature: 0.2,
	})
}

func (g *AnthropicChatGenerator) Ping(ctx context.Context) PingResult {
	result := PingResult{APIBase: g.apiBase, Model: g.model}
	if err := g.requireAPIKey(); err != nil {
		result.Message = err.Error()
		return result
	}
	start := time.Now()
	_, err := g.complete(ctx, anthropicChatRequest{
		Model:       g.model,
		Messages:    []anthropicChatMessage{{Role: "user", Content: "ping"}},
		MaxTokens:   1,
		Temperature: 0.01,
	})
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.OK = true
	result.Message = fmt.Sprintf("连通正常，对话模型 %s 已响应", g.model)
	return result
}

func (g *AnthropicChatGenerator) complete(ctx context.Context, body anthropicChatRequest) (string, error) {
	if err := g.requireAPIKey(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := g.newRequest(ctx, payload)
	if err != nil {
		return "", err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", anthropicStatusError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, anthropicChatMaxBodyBytes))
	if err != nil {
		return "", err
	}
	var result anthropicChatResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("Anthropic 响应解析失败: %w", err)
	}
	if result.Type == "error" || result.Error != nil {
		return "", anthropicResponseError(result.Error)
	}
	var answer strings.Builder
	for _, block := range result.Content {
		if block.Type == "text" {
			answer.WriteString(block.Text)
		}
	}
	content := strings.TrimSpace(answer.String())
	if content == "" {
		return "", fmt.Errorf("Anthropic 未返回文本回答")
	}
	return content, nil
}

func (g *AnthropicChatGenerator) newRequest(ctx context.Context, payload []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiBase+"/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", g.apiKey)
	req.Header.Set("anthropic-version", anthropicChatVersion)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (g *AnthropicChatGenerator) requireAPIKey() error {
	if g.apiKey == "" {
		return fmt.Errorf("请先配置对话模型 API Key")
	}
	return nil
}

func (g *AnthropicChatGenerator) chatMessages(input rag.GenerateInput) []anthropicChatMessage {
	messages := make([]anthropicChatMessage, 0, len(input.History)+2)
	appendMessage := func(role, content string) {
		if len(messages) > 0 && messages[len(messages)-1].Role == role {
			messages[len(messages)-1].Content += "\n\n" + content
			return
		}
		messages = append(messages, anthropicChatMessage{Role: role, Content: content})
	}
	if preferences := buildCompatibleChatPreferenceMessage(input.UserPreferences); preferences != "" {
		appendMessage("user", preferences)
	}
	for _, message := range input.History {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		appendMessage(role, content)
	}
	appendMessage("user", buildCompatibleChatUserMessage(input))
	return messages
}

func anthropicStatusError(resp *http.Response) error {
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, anthropicChatMaxBodyBytes))
	if readErr != nil {
		return fmt.Errorf("Anthropic 请求失败(%d): %v", resp.StatusCode, readErr)
	}
	var payload anthropicChatResponse
	if json.Unmarshal(raw, &payload) == nil && payload.Error != nil && strings.TrimSpace(payload.Error.Message) != "" {
		return fmt.Errorf("Anthropic 请求失败(%d): %s", resp.StatusCode, strings.TrimSpace(payload.Error.Message))
	}
	return fmt.Errorf("Anthropic 请求失败(%d): %s", resp.StatusCode, compact(raw))
}

func anthropicResponseError(payload *anthropicError) error {
	message := "未知错误"
	if payload != nil && strings.TrimSpace(payload.Message) != "" {
		message = strings.TrimSpace(payload.Message)
	}
	return fmt.Errorf("Anthropic 返回错误: %s", message)
}

var _ ChatGenerator = (*AnthropicChatGenerator)(nil)
