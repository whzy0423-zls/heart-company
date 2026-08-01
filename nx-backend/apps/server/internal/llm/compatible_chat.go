package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/netguard"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

const (
	chatProviderOpenAI    = "openai-compatible"
	chatProviderAnthropic = "anthropic-compatible"
	chatStreamMaxBytes    = 1024 * 1024
)

type CompatibleChatGenerator struct {
	provider     string
	apiBase      string
	apiKey       string
	model        string
	systemPrompt string
	client       *http.Client
}

func NewCompatibleChatGenerator(cfg config.MiniMaxConfig) *CompatibleChatGenerator {
	provider := normalizeChatProvider(cfg.Provider)
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://coding-play.codes"
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	return &CompatibleChatGenerator{
		provider:     provider,
		apiBase:      apiBase,
		apiKey:       strings.TrimSpace(cfg.APIKey),
		model:        strings.TrimSpace(cfg.Model),
		systemPrompt: strings.TrimSpace(cfg.SystemPrompt),
		client: &http.Client{
			Timeout:   timeout,
			Transport: netguard.NewGuardedTransport(),
		},
	}
}

func normalizeChatProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "anthropic", chatProviderAnthropic:
		return chatProviderAnthropic
	default:
		return chatProviderOpenAI
	}
}

func (g *CompatibleChatGenerator) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	return g.generateText(ctx, g.resolveSystemPrompt(), buildUserPrompt(input), chatTokenBudgetForTier(input.Question, input.Tier), 0.55)
}

func (g *CompatibleChatGenerator) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if err := g.validate(); err != nil {
		return "", err
	}
	body := g.requestBody(g.resolveSystemPrompt(), buildUserPrompt(input), chatTokenBudgetForTier(input.Question, input.Tier), 0.55, true)
	req, err := g.newRequest(ctx, body, true)
	if err != nil {
		return "", err
	}
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		return "", fmt.Errorf("%s流式请求失败(%d): %s", g.providerLabel(), resp.StatusCode, compact(raw))
	}
	answer, err := consumeCompatibleChatStream(ctx, resp.Body, g.provider, emit)
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", fmt.Errorf("%s未返回文本回答", g.providerLabel())
	}
	return answer, nil
}

func (g *CompatibleChatGenerator) SummarizeConversation(ctx context.Context, previousSummary string, messages []rag.Message) (string, error) {
	if len(messages) == 0 {
		return strings.TrimSpace(previousSummary), nil
	}
	var prompt strings.Builder
	prompt.WriteString("已有会话摘要：\n")
	if previousSummary = strings.TrimSpace(previousSummary); previousSummary == "" {
		prompt.WriteString("暂无。\n")
	} else {
		prompt.WriteString(trimRunes(previousSummary, 1200) + "\n")
	}
	prompt.WriteString("新增对话：\n")
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.Content)
		if (role != "user" && role != "assistant") || content == "" {
			continue
		}
		prompt.WriteString(role + ": " + trimRunes(content, 220) + "\n")
	}
	prompt.WriteString("请输出合并后的会话摘要，只输出摘要正文。")
	summary, err := g.generateText(ctx,
		"你负责压缩会话摘要。必须保留参与人物及关系、已确认事实和事件、用户诉求与边界、关键建议与反馈、尚未解决的问题；删除寒暄、重复表达和无关细节。摘要应准确、简洁，不得添加对话中不存在的信息。",
		prompt.String(), 700, 0.2,
	)
	if err != nil {
		return "", err
	}
	return trimRunes(strings.TrimSpace(summary), 1200), nil
}

func (g *CompatibleChatGenerator) Ping(ctx context.Context) PingResult {
	result := PingResult{APIBase: g.apiBase, Model: g.model}
	if err := g.validate(); err != nil {
		result.Message = err.Error()
		return result
	}
	start := time.Now()
	_, err := g.generateText(ctx, "", "ping", 1, 0.01)
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.OK = true
	result.Message = fmt.Sprintf("连通正常，%s %s 已响应", g.providerLabel(), g.model)
	return result
}

func (g *CompatibleChatGenerator) generateText(ctx context.Context, systemPrompt, userPrompt string, maxTokens int, temperature float64) (string, error) {
	if err := g.validate(); err != nil {
		return "", err
	}
	body := g.requestBody(systemPrompt, userPrompt, maxTokens, temperature, false)
	req, err := g.newRequest(ctx, body, false)
	if err != nil {
		return "", err
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("%s请求失败(%d): %s", g.providerLabel(), resp.StatusCode, compact(raw))
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("%s响应解析失败: %w", g.providerLabel(), err)
	}
	if err := compatiblePayloadError(payload); err != nil {
		return "", fmt.Errorf("%s返回错误: %w", g.providerLabel(), err)
	}
	answer := ""
	if g.provider == chatProviderAnthropic {
		answer = findString(payload, "content.0.text")
	} else {
		answer = findString(payload, "choices.0.message.content", "choices.0.text")
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", fmt.Errorf("%s未返回文本回答", g.providerLabel())
	}
	return answer, nil
}

func (g *CompatibleChatGenerator) validate() error {
	if g.apiKey == "" {
		return fmt.Errorf("请先配置会话模型 API Key")
	}
	if g.model == "" {
		return fmt.Errorf("请先配置会话模型 Model")
	}
	return nil
}

func (g *CompatibleChatGenerator) resolveSystemPrompt() string {
	return appendChatCustomSystemPrompt(defaultSystemPrompt, g.systemPrompt)
}

func (g *CompatibleChatGenerator) providerLabel() string {
	if g.provider == chatProviderAnthropic {
		return "Anthropic 兼容模型"
	}
	return "OpenAI 兼容模型"
}

func (g *CompatibleChatGenerator) endpoint() string {
	path := "/v1/chat/completions"
	if g.provider == chatProviderAnthropic {
		path = "/v1/messages"
	}
	if strings.HasSuffix(g.apiBase, "/v1") {
		return g.apiBase + strings.TrimPrefix(path, "/v1")
	}
	return g.apiBase + path
}

func (g *CompatibleChatGenerator) requestBody(systemPrompt, userPrompt string, maxTokens int, temperature float64, stream bool) map[string]any {
	if maxTokens <= 0 {
		maxTokens = 220
	}
	if g.provider == chatProviderAnthropic {
		body := map[string]any{
			"model":      g.model,
			"max_tokens": maxTokens,
			"messages": []map[string]string{
				{"role": "user", "content": userPrompt},
			},
		}
		if strings.TrimSpace(systemPrompt) != "" {
			body["system"] = systemPrompt
		}
		if temperature > 0 {
			body["temperature"] = temperature
		}
		if stream {
			body["stream"] = true
		}
		return body
	}
	body := map[string]any{
		"model": g.model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	if usesMaxCompletionTokens(g.model) {
		body["max_completion_tokens"] = maxTokens
	} else {
		body["max_tokens"] = maxTokens
	}
	if temperature > 0 && !usesMaxCompletionTokens(g.model) {
		body["temperature"] = temperature
	}
	if stream {
		body["stream"] = true
	}
	return body
}

func usesMaxCompletionTokens(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gpt-5") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4")
}

func (g *CompatibleChatGenerator) newRequest(ctx context.Context, body map[string]any, stream bool) (*http.Request, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	}
	if g.provider == chatProviderAnthropic {
		req.Header.Set("x-api-key", g.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+g.apiKey)
	}
	return req, nil
}

func consumeCompatibleChatStream(ctx context.Context, body io.Reader, provider string, emit rag.StreamEmitter) (string, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 32*1024), chatStreamMaxBytes)
	var answer strings.Builder
	var eventType string
	var data strings.Builder
	dispatch := func() (bool, error) {
		raw := strings.TrimSpace(data.String())
		data.Reset()
		currentEvent := eventType
		eventType = ""
		if raw == "" {
			return false, nil
		}
		if raw == "[DONE]" || currentEvent == "message_stop" {
			return true, nil
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return false, fmt.Errorf("会话模型流响应解析失败: %w", err)
		}
		if err := compatiblePayloadError(payload); err != nil {
			return false, err
		}
		if findString(payload, "type") == "message_stop" {
			return true, nil
		}
		delta := ""
		if normalizeChatProvider(provider) == chatProviderAnthropic {
			delta = findString(payload, "delta.text")
		} else {
			delta = findString(payload, "choices.0.delta.content", "choices.0.delta.text")
		}
		if delta == "" {
			return false, nil
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
		if emit != nil {
			if err := emit(delta); err != nil {
				return false, err
			}
		}
		answer.WriteString(delta)
		return false, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			done, err := dispatch()
			if err != nil {
				return "", err
			}
			if done {
				return answer.String(), nil
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			eventType = value
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(value)
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return "", fmt.Errorf("会话模型流事件过大")
		}
		return "", err
	}
	_, err := dispatch()
	return answer.String(), err
}

func compatiblePayloadError(payload map[string]any) error {
	errorValue, exists := payload["error"]
	if !exists || errorValue == nil {
		return nil
	}
	if message := findString(payload, "error.message", "error.type", "message"); strings.TrimSpace(message) != "" {
		return errors.New(strings.TrimSpace(message))
	}
	return fmt.Errorf("会话模型返回错误")
}
