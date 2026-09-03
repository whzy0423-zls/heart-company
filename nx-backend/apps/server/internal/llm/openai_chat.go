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
	openAIChatDefaultAPIBase = "https://api.openai.com/v1"
	openAIChatDefaultModel   = "gpt-4o-mini"
	openAIChatMaxBodyBytes   = 2 * 1024 * 1024
	openAIChatMaxTokens      = 360
)

var errOpenAIStreamTerminal = errors.New("openai stream terminal event")

type OpenAIChatGenerator struct {
	apiBase      string
	apiKey       string
	client       *http.Client
	model        string
	systemPrompt string
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model          string              `json:"model"`
	Messages       []openAIChatMessage `json:"messages"`
	Temperature    float64             `json:"temperature"`
	MaxTokens      int                 `json:"max_tokens"`
	Stream         bool                `json:"stream,omitempty"`
	ResponseFormat map[string]string   `json:"response_format,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices"`
	Error *openAIError `json:"error,omitempty"`
}

type openAIError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

func newOpenAIChatGenerator(cfg ChatGeneratorConfig, client *http.Client) *OpenAIChatGenerator {
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if apiBase == "" {
		apiBase = openAIChatDefaultAPIBase
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = openAIChatDefaultModel
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAIChatGenerator{
		apiBase:      apiBase,
		apiKey:       strings.TrimSpace(cfg.APIKey),
		client:       client,
		model:        model,
		systemPrompt: strings.TrimSpace(cfg.SystemPrompt),
	}
}

func (g *OpenAIChatGenerator) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	return g.complete(ctx, openAIChatRequest{
		Model:       g.model,
		Messages:    g.chatMessages(input),
		Temperature: 0.55,
		MaxTokens:   chatTokenBudgetForTier(input.Question, input.Tier),
	})
}

func (g *OpenAIChatGenerator) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if err := g.requireAPIKey(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(openAIChatRequest{
		Model:       g.model,
		Messages:    g.chatMessages(input),
		Temperature: 0.55,
		MaxTokens:   chatTokenBudgetForTier(input.Question, input.Tier),
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
		return "", openAIStatusError(resp)
	}

	var answer strings.Builder
	terminal := false
	err = readSSE(ctx, resp.Body, func(event sseEvent) error {
		data := strings.TrimSpace(event.Data)
		if data == "" {
			return nil
		}
		if data == "[DONE]" {
			terminal = true
			return errOpenAIStreamTerminal
		}
		if !strings.HasPrefix(data, "{") {
			return fmt.Errorf("OpenAI 流响应解析失败: 事件不是 JSON 对象")
		}

		var payload struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Error *openAIError `json:"error,omitempty"`
		}
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return fmt.Errorf("OpenAI 流响应解析失败: %w", err)
		}
		if payload.Error != nil {
			return openAIResponseError(payload.Error)
		}
		for _, choice := range payload.Choices {
			delta := choice.Delta.Content
			if delta != "" {
				if err := ctx.Err(); err != nil {
					return err
				}
				if emit != nil {
					if err := emit(delta); err != nil {
						return err
					}
				}
				answer.WriteString(delta)
			}
			if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
				terminal = true
			}
		}
		if terminal {
			return errOpenAIStreamTerminal
		}
		return nil
	})
	if err != nil && !errors.Is(err, errOpenAIStreamTerminal) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		return "", err
	}
	if !terminal {
		return "", fmt.Errorf("OpenAI 流响应缺少终止标记")
	}
	result := strings.TrimSpace(answer.String())
	if result == "" {
		return "", fmt.Errorf("OpenAI 未返回文本回答")
	}
	return result, nil
}

func (g *OpenAIChatGenerator) SummarizeConversation(ctx context.Context, previousSummary string, messages []rag.Message) (string, error) {
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

	result, err := g.complete(ctx, openAIChatRequest{
		Model: g.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: "你负责压缩会话摘要。必须保留参与人物及关系、已确认事实和事件、用户诉求与边界、关键建议与反馈、尚未解决的问题；删除寒暄、重复表达和无关细节。摘要应准确、简洁，不得添加对话中不存在的信息。"},
			{Role: "user", Content: prompt.String()},
		},
		Temperature: 0.2,
		MaxTokens:   700,
	})
	if err != nil {
		return "", err
	}
	return trimRunes(result, 1200), nil
}

func (g *OpenAIChatGenerator) PolishPrompt(ctx context.Context, draft, kind string) (string, error) {
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return "", fmt.Errorf("请先填写提示词方向或草稿")
	}
	return g.complete(ctx, openAIChatRequest{
		Model: g.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: polishSystemPrompt(kind)},
			{Role: "user", Content: "请润色以下" + polishKindLabel(kind) + "提示词方向，只输出润色后的提示词正文：\n" + draft},
		},
		Temperature: 0.7,
		MaxTokens:   600,
	})
}

func (g *OpenAIChatGenerator) CompleteJSON(ctx context.Context, system, user string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = 1200
	}
	return g.complete(ctx, openAIChatRequest{
		Model: g.model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature:    0.2,
		MaxTokens:      maxTokens,
		ResponseFormat: map[string]string{"type": "json_object"},
	})
}

func (g *OpenAIChatGenerator) Ping(ctx context.Context) PingResult {
	result := PingResult{APIBase: g.apiBase, Model: g.model}
	if err := g.requireAPIKey(); err != nil {
		result.Message = err.Error()
		return result
	}
	start := time.Now()
	_, err := g.complete(ctx, openAIChatRequest{
		Model:       g.model,
		Messages:    []openAIChatMessage{{Role: "user", Content: "ping"}},
		Temperature: 0.01,
		MaxTokens:   1,
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

func (g *OpenAIChatGenerator) complete(ctx context.Context, body openAIChatRequest) (string, error) {
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
		return "", openAIStatusError(resp)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, openAIChatMaxBodyBytes))
	if err != nil {
		return "", err
	}
	var result openAIChatResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("OpenAI 响应解析失败: %w", err)
	}
	if result.Error != nil {
		return "", openAIResponseError(result.Error)
	}
	for _, choice := range result.Choices {
		if isContentFilterCode(choice.FinishReason) {
			return "", newContentFilterError("openai", choice.FinishReason)
		}
	}
	for _, choice := range result.Choices {
		if content := strings.TrimSpace(choice.Message.Content); content != "" {
			return content, nil
		}
	}
	return "", fmt.Errorf("OpenAI 未返回文本回答")
}

func (g *OpenAIChatGenerator) newRequest(ctx context.Context, payload []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.apiBase+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (g *OpenAIChatGenerator) requireAPIKey() error {
	if g.apiKey == "" {
		return fmt.Errorf("请先配置对话模型 API Key")
	}
	return nil
}

func (g *OpenAIChatGenerator) chatMessages(input rag.GenerateInput) []openAIChatMessage {
	messages := []openAIChatMessage{{Role: "system", Content: resolveRuntimeSystemPrompt(g.resolveSystemPrompt(), input)}}
	if preferences := buildCompatibleChatPreferenceMessage(input.UserPreferences); preferences != "" {
		messages = append(messages, openAIChatMessage{Role: "user", Content: preferences})
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
		messages = append(messages, openAIChatMessage{Role: role, Content: content})
	}
	messages = append(messages, openAIChatMessage{Role: "user", Content: buildCompatibleChatUserMessage(input)})
	return messages
}

func (g *OpenAIChatGenerator) resolveSystemPrompt() string {
	return resolveCompatibleChatSystemPrompt(g.systemPrompt)
}

func openAIStatusError(resp *http.Response) error {
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, openAIChatMaxBodyBytes))
	if readErr != nil {
		return fmt.Errorf("OpenAI 请求失败(%d): %v", resp.StatusCode, readErr)
	}
	var payload openAIChatResponse
	if json.Unmarshal(raw, &payload) == nil && payload.Error != nil {
		if isContentFilterCode(payload.Error.Type) || isContentFilterCode(payload.Error.Code) {
			return newContentFilterError("openai", firstContentFilterCode(payload.Error.Type, payload.Error.Code))
		}
		if message := strings.TrimSpace(payload.Error.Message); message != "" {
			return fmt.Errorf("OpenAI 请求失败(%d): %s", resp.StatusCode, message)
		}
	}
	return fmt.Errorf("OpenAI 请求失败(%d): %s", resp.StatusCode, compact(raw))
}

func openAIResponseError(payload *openAIError) error {
	if payload != nil && (isContentFilterCode(payload.Type) || isContentFilterCode(payload.Code)) {
		return newContentFilterError("openai", firstContentFilterCode(payload.Type, payload.Code))
	}
	message := ""
	if payload != nil {
		message = strings.TrimSpace(payload.Message)
	}
	if message == "" {
		message = "未知错误"
	}
	return fmt.Errorf("OpenAI 返回错误: %s", message)
}

func firstContentFilterCode(values ...any) string {
	for _, value := range values {
		if !isContentFilterCode(value) {
			continue
		}
		if code, ok := value.(string); ok {
			return code
		}
	}
	return "content_filtered"
}

var _ ChatGenerator = (*OpenAIChatGenerator)(nil)
