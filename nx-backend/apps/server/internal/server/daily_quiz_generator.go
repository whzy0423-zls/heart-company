package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/netguard"
	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

type serverDailyQuizQuestionGenerator struct {
	server                  *Server
	readModelConfig         func(context.Context, any) (modelconfig.Config, bool, error)
	newStructuredJSONClient func(modelconfig.CompatibleModelConfig) dailyQuizStructuredJSONClient
}

type dailyQuizStructuredJSONClient interface {
	GenerateJSON(context.Context, llm.StructuredJSONRequest) (llm.StructuredJSONResult, error)
}

func (g serverDailyQuizQuestionGenerator) GenerateDailyQuizQuestions(ctx context.Context, input profilecalibration.DailyQuizGenerationInput) (profilecalibration.DailyQuizGenerationResult, error) {
	if g.server == nil {
		return profilecalibration.DailyQuizGenerationResult{}, fmt.Errorf("daily quiz generator unavailable")
	}
	readConfig := g.readModelConfig
	if readConfig == nil {
		if g.server.db == nil {
			return profilecalibration.DailyQuizGenerationResult{}, fmt.Errorf("daily quiz generator unavailable")
		}
		readConfig = func(ctx context.Context, _ any) (modelconfig.Config, bool, error) {
			return modelconfig.ReadStore(ctx, g.server.db)
		}
	}
	stored, _, err := readConfig(ctx, g.server.db)
	if err != nil {
		return profilecalibration.DailyQuizGenerationResult{}, err
	}
	adminBase := stored.ApplyAdmin(g.server.env.MiniMax)
	cfg := modelconfig.Config{Admin: adminBase, DailyQuiz: stored.DailyQuiz}.ApplyDailyQuiz()
	if strings.TrimSpace(cfg.APIKey) == "" {
		return profilecalibration.DailyQuizGenerationResult{}, fmt.Errorf("请先在后台配置管理端大模型密钥")
	}
	if input.Count <= 0 {
		input.Count = profilecalibration.DailyQuestionCount
	}
	prompt := g.buildDailyQuizPrompt(ctx, input)
	content := ""
	rawResponse := ""
	if g.newStructuredJSONClient != nil {
		result, err := g.newStructuredJSONClient(cfg).GenerateJSON(ctx, llm.StructuredJSONRequest{
			SystemPrompt: dailyQuizSystemPrompt(),
			UserPrompt:   prompt,
			MaxTokens:    2200,
			Temperature:  0.35,
		})
		if err != nil {
			return profilecalibration.DailyQuizGenerationResult{}, err
		}
		content = result.Content
		rawResponse = result.RawResponse
		if cfg.Provider == "" {
			cfg.Provider = result.Provider
		}
		if cfg.Model == "" {
			cfg.Model = result.Model
		}
	} else {
		content, err = callAdminModelJSON(ctx, cfg, dailyQuizSystemPrompt(), prompt)
		if err != nil {
			return profilecalibration.DailyQuizGenerationResult{}, err
		}
		rawResponse = content
	}
	questions, err := parseDailyQuizQuestions(content)
	if err != nil {
		return profilecalibration.DailyQuizGenerationResult{}, err
	}
	if len(questions) < input.Count {
		return profilecalibration.DailyQuizGenerationResult{}, fmt.Errorf("模型仅返回 %d 道题，少于要求的 %d 道", len(questions), input.Count)
	}
	return profilecalibration.DailyQuizGenerationResult{
		Questions:     questions[:input.Count],
		Prompt:        prompt,
		RawResponse:   rawResponse,
		ModelProvider: strings.ToLower(strings.TrimSpace(cfg.Provider)),
		ModelName:     cfg.Model,
		Source:        "ai",
	}, nil
}

func (g serverDailyQuizQuestionGenerator) buildDailyQuizPrompt(ctx context.Context, input profilecalibration.DailyQuizGenerationInput) string {
	date := strings.TrimSpace(input.Date)
	if date == "" {
		date = appCalibrationBusinessDate(time.Now())
	}
	var b strings.Builder
	b.WriteString("请生成用于 App 每日画像校准的九型人格题目。\n")
	b.WriteString("日期：" + date + "\n")
	b.WriteString(fmt.Sprintf("题目数量：%d\n", input.Count))
	if input.SlotNo > 0 {
		b.WriteString(fmt.Sprintf("这是替换第 %d 题，只生成这一题。\n", input.SlotNo))
	}
	if input.ReplaceReason != "" {
		b.WriteString("管理员换题原因：" + input.ReplaceReason + "\n")
	}
	b.WriteString("要求：围绕九型人格、能量状态、画像校准、日常压力/关系/行动模式；不要生成医疗诊断、玄学断语或偏离主题内容。\n")
	b.WriteString("每题 4 个选项，选项必须带 typeWeights，key 为 \"1\" 到 \"9\"，value 为 1-3 的整数。\n")
	b.WriteString("只输出 JSON，格式：{\"questions\":[{\"body\":\"题干\",\"dimension\":\"security|relationship|action|emotion|boundary|growth\",\"options\":[{\"id\":\"a\",\"label\":\"A\",\"text\":\"选项\",\"typeWeights\":{\"1\":2}}]}]}。\n")

	docs := g.dailyQuizKnowledge(ctx)
	if len(docs) > 0 {
		b.WriteString("公共知识库参考摘要：\n")
		for i, doc := range docs {
			if i >= 8 {
				break
			}
			b.WriteString(fmt.Sprintf("%d. %s：%s\n", i+1, doc.Title, trimServerRunes(doc.Content, 260)))
		}
	}
	return b.String()
}

func (g serverDailyQuizQuestionGenerator) dailyQuizKnowledge(ctx context.Context) []rag.Document {
	if g.server == nil || g.server.ragDocs == nil {
		return nil
	}
	docs, err := g.server.ragDocs.EnabledDocuments(ctx)
	if err != nil {
		return nil
	}
	out := make([]rag.Document, 0, 8)
	for _, doc := range docs {
		text := strings.ToLower(doc.Title + " " + doc.Content + " " + strings.Join(doc.Tags, " "))
		if strings.Contains(text, "九型") || strings.Contains(text, "人格") || strings.Contains(text, "能量") || strings.Contains(text, "画像") || strings.Contains(text, "成长") {
			out = append(out, doc)
			if len(out) >= 8 {
				break
			}
		}
	}
	if len(out) == 0 && len(docs) > 0 {
		limit := len(docs)
		if limit > 5 {
			limit = 5
		}
		out = append(out, docs[:limit]...)
	}
	return out
}

func dailyQuizSystemPrompt() string {
	return "你是九型人格画像校准题库专家。你只生成中文 App 题目，题目短、具体、适合手机端，每个选项能反映不同九型倾向。你必须只输出合法 JSON。"
}

func callAdminModelJSON(ctx context.Context, cfg modelconfig.AdminModelConfig, systemPrompt, userPrompt string) (string, error) {
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	if provider == "" {
		provider = "minimax"
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	client := &http.Client{Timeout: timeout, Transport: netguard.NewGuardedTransport()}
	switch provider {
	case "anthropic", "anthropic-compatible":
		return callAnthropicJSON(ctx, client, cfg, systemPrompt, userPrompt)
	case "openai", "openai-compatible", "newapi":
		return callOpenAICompatibleJSON(ctx, client, cfg, systemPrompt, userPrompt)
	default:
		return callMiniMaxJSON(ctx, client, cfg, systemPrompt, userPrompt)
	}
}

func callOpenAICompatibleJSON(ctx context.Context, client *http.Client, cfg modelconfig.AdminModelConfig, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model":       cfg.Model,
		"temperature": 0.35,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	raw, err := doJSONRequest(ctx, client, http.MethodPost, adminModelEndpoint(cfg.APIBase, "/v1/chat/completions"), "Bearer "+cfg.APIKey, body, nil)
	if err != nil {
		return "", err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return "", err
	}
	text := findServerString(result, "choices.0.message.content", "choices.0.text")
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("OpenAI 兼容模型未返回文本")
	}
	return strings.TrimSpace(text), nil
}

func callAnthropicJSON(ctx context.Context, client *http.Client, cfg modelconfig.AdminModelConfig, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model":       cfg.Model,
		"max_tokens":  2600,
		"temperature": 0.35,
		"system":      systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}
	headers := map[string]string{"x-api-key": cfg.APIKey, "anthropic-version": "2023-06-01"}
	raw, err := doJSONRequest(ctx, client, http.MethodPost, adminModelEndpoint(cfg.APIBase, "/v1/messages"), "", body, headers)
	if err != nil {
		return "", err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return "", err
	}
	text := findAnthropicText(result)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("Anthropic 模型未返回文本")
	}
	return strings.TrimSpace(text), nil
}

func callMiniMaxJSON(ctx context.Context, client *http.Client, cfg modelconfig.AdminModelConfig, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model":              cfg.Model,
		"temperature":        0.35,
		"tokens_to_generate": 1800,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	endpoint := adminModelEndpoint(cfg.APIBase, "/v1/text/chatcompletion_v2")
	if strings.TrimSpace(cfg.GroupID) != "" {
		sep := "?"
		if strings.Contains(endpoint, "?") {
			sep = "&"
		}
		endpoint += sep + "GroupId=" + url.QueryEscape(strings.TrimSpace(cfg.GroupID))
	}
	raw, err := doJSONRequest(ctx, client, http.MethodPost, endpoint, "Bearer "+cfg.APIKey, body, nil)
	if err != nil {
		return "", err
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return "", err
	}
	text := findServerString(result, "choices.0.message.content", "choices.0.text", "reply", "data.reply", "data.choices.0.message.content")
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("MiniMax 模型未返回文本")
	}
	return strings.TrimSpace(text), nil
}

func doJSONRequest(ctx context.Context, client *http.Client, method, endpoint, authorization string, body any, headers map[string]string) (string, error) {
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 3*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("管理端大模型请求失败(%d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return string(raw), nil
}

func adminModelEndpoint(apiBase, path string) string {
	base := strings.TrimRight(strings.TrimSpace(apiBase), "/")
	if base == "" {
		base = "https://api.minimaxi.com"
	}
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1/") {
		return base + strings.TrimPrefix(path, "/v1")
	}
	return base + path
}

func parseDailyQuizQuestions(raw string) ([]profilecalibration.GeneratedDailyQuizQuestion, error) {
	jsonText := extractFirstJSONObject(raw)
	if jsonText == "" {
		return nil, fmt.Errorf("模型未返回有效 JSON")
	}
	var payload struct {
		Questions []profilecalibration.GeneratedDailyQuizQuestion `json:"questions"`
	}
	if err := json.Unmarshal([]byte(jsonText), &payload); err != nil {
		return nil, err
	}
	return payload.Questions, nil
}

func extractFirstJSONObject(text string) string {
	text = strings.TrimSpace(text)
	start := strings.Index(text, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escaped := false
	for index, r := range text[start:] {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return text[start : start+index+1]
			}
		}
	}
	return ""
}

func findServerString(payload any, paths ...string) string {
	for _, path := range paths {
		if value := findServerPath(payload, strings.Split(path, ".")); value != "" {
			return value
		}
	}
	return ""
}

func findServerPath(value any, parts []string) string {
	if len(parts) == 0 {
		text, _ := value.(string)
		return text
	}
	switch current := value.(type) {
	case map[string]any:
		return findServerPath(current[parts[0]], parts[1:])
	case []any:
		index, err := strconv.Atoi(parts[0])
		if err != nil || index < 0 || index >= len(current) {
			return ""
		}
		return findServerPath(current[index], parts[1:])
	default:
		return ""
	}
}

func findAnthropicText(payload map[string]any) string {
	content, _ := payload["content"].([]any)
	parts := make([]string, 0, len(content))
	for _, item := range content {
		obj, _ := item.(map[string]any)
		if text, _ := obj["text"].(string); strings.TrimSpace(text) != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "\n")
}

func trimServerRunes(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if limit <= 0 || len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit])
}
