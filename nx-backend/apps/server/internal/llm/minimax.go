package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

type MiniMaxGenerator struct {
	apiBase string
	apiKey  string
	client  *http.Client
	groupID string
	model   string
}

func NewMiniMaxGenerator(cfg config.MiniMaxConfig) *MiniMaxGenerator {
	apiBase := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/")
	if apiBase == "" {
		apiBase = "https://api.minimaxi.com"
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	return &MiniMaxGenerator{
		apiBase: apiBase,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client:  &http.Client{Timeout: timeout},
		groupID: strings.TrimSpace(cfg.GroupID),
		model:   "abab6.5s-chat",
	}
}

func (g *MiniMaxGenerator) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}

	body := map[string]any{
		"model":              g.model,
		"temperature":        0.55,
		"tokens_to_generate": 520,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt()},
			{"role": "user", "content": buildUserPrompt(input)},
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("/v1/text/chatcompletion_v2"), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("MiniMax 请求失败(%d): %s", resp.StatusCode, compact(raw))
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if err := baseRespError(result); err != nil {
		return "", err
	}
	answer := findString(result,
		"choices.0.message.content",
		"choices.0.text",
		"reply",
		"data.reply",
		"data.choices.0.message.content",
	)
	if strings.TrimSpace(answer) == "" {
		return "", fmt.Errorf("MiniMax 未返回文本回答")
	}
	return strings.TrimSpace(answer), nil
}

func (g *MiniMaxGenerator) endpoint(path string) string {
	endpoint := g.apiBase + path
	if g.groupID == "" {
		return endpoint
	}
	sep := "?"
	if strings.Contains(endpoint, "?") {
		sep = "&"
	}
	return endpoint + sep + "GroupId=" + url.QueryEscape(g.groupID)
}

func systemPrompt() string {
	return "你是九型芯之力小程序里的九型人格成长教练。只基于给定检索资料和用户档案回答；不做医疗诊断；回答要温和、具体、适合手机阅读；如果资料不足，说明可以继续补充问题。"
}

func buildUserPrompt(input rag.GenerateInput) string {
	var b strings.Builder
	if input.UserProfile.Nickname != "" || input.UserProfile.MainType > 0 {
		b.WriteString("用户档案：")
		if input.UserProfile.Nickname != "" {
			b.WriteString("昵称=" + input.UserProfile.Nickname + "；")
		}
		if input.UserProfile.MainType > 0 {
			b.WriteString(fmt.Sprintf("最近主型=%d号；", input.UserProfile.MainType))
		}
		b.WriteString("\n")
	}
	if len(input.History) > 0 {
		b.WriteString("最近对话：\n")
		for _, item := range input.History {
			b.WriteString(item.Role + ": " + item.Content + "\n")
		}
	}
	b.WriteString("用户问题：" + input.Question + "\n")
	b.WriteString("检索资料：\n")
	if len(input.Sources) == 0 {
		b.WriteString("暂无高相关资料。\n")
	} else {
		for i, source := range input.Sources {
			b.WriteString(fmt.Sprintf("%d. %s：%s\n", i+1, source.Title, source.Snippet))
		}
	}
	b.WriteString("请结合检索资料给出 2-4 段回答，最后给一个可执行的小建议。")
	return b.String()
}

func baseRespError(payload map[string]any) error {
	base, ok := payload["base_resp"].(map[string]any)
	if !ok {
		return nil
	}
	code, _ := base["status_code"].(float64)
	if code == 0 {
		return nil
	}
	message, _ := base["status_msg"].(string)
	if message == "" {
		message = "MiniMax 返回错误"
	}
	return fmt.Errorf(message)
}

func findString(payload any, paths ...string) string {
	for _, path := range paths {
		if value := findPath(payload, strings.Split(path, ".")); value != "" {
			return value
		}
	}
	return ""
}

func findPath(value any, parts []string) string {
	if len(parts) == 0 {
		text, _ := value.(string)
		return text
	}
	switch current := value.(type) {
	case map[string]any:
		return findPath(current[parts[0]], parts[1:])
	case []any:
		index := 0
		if parts[0] != "0" {
			return ""
		}
		if len(current) <= index {
			return ""
		}
		return findPath(current[index], parts[1:])
	default:
		return ""
	}
}

func compact(raw []byte) string {
	return strings.TrimSpace(strings.Join(strings.Fields(string(raw)), " "))
}
