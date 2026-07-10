package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ScriptResult struct {
	Content   string `json:"content"`
	Summary   string `json:"summary"`
	Style     string `json:"style"`
	RawResult string `json:"rawResult"`
}

func (g *MiniMaxGenerator) PolishVideoProjectScript(ctx context.Context, draft string) (ScriptResult, error) {
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return ScriptResult{}, fmt.Errorf("请先填写一句创意或剧本内容")
	}
	if g.apiKey == "" {
		return ScriptResult{}, fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}

	body := map[string]any{
		"model":              g.model,
		"temperature":        0.55,
		"tokens_to_generate": 2400,
		"messages": []map[string]string{
			{"role": "system", "content": videoProjectScriptSystemPrompt()},
			{"role": "user", "content": "请把下面的一句话创意或剧本草稿整理为可拍摄、可继续拆解的完整视频剧本。只返回约定的 JSON。\n\n原始内容：\n" + draft},
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("/v1/text/chatcompletion_v2"), bytes.NewReader(payload))
	if err != nil {
		return ScriptResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return ScriptResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ScriptResult{}, fmt.Errorf("剧本创作模型请求失败(%d): %s", resp.StatusCode, compact(raw))
	}
	var response map[string]any
	if err := json.Unmarshal(raw, &response); err != nil {
		return ScriptResult{}, fmt.Errorf("剧本创作模型响应解析失败: %w", err)
	}
	if err := baseRespError(response); err != nil {
		return ScriptResult{}, err
	}
	answer := strings.TrimSpace(findString(response,
		"choices.0.message.content",
		"choices.0.text",
		"reply",
		"data.reply",
		"data.choices.0.message.content",
	))
	if answer == "" {
		return ScriptResult{}, fmt.Errorf("剧本创作模型未返回内容")
	}

	result := parseVideoProjectScript(answer)
	result.RawResult = answer
	if result.Content == "" {
		return ScriptResult{}, fmt.Errorf("剧本创作模型未返回可编辑的剧本正文")
	}
	return result, nil
}

func parseVideoProjectScript(answer string) ScriptResult {
	cleaned := strings.TrimSpace(stripThinkBlocks(answer))
	if jsonText, ok := firstJSONObject(cleaned); ok {
		if fields, err := looseJSONObject(jsonText); err == nil {
			content, contentErr := looseStringField(fields, "content")
			summary, summaryErr := looseStringField(fields, "summary")
			style, styleErr := looseStringField(fields, "style")
			if contentErr == nil && summaryErr == nil && styleErr == nil && strings.TrimSpace(content) != "" {
				return ScriptResult{
					Content: strings.TrimSpace(content),
					Summary: strings.TrimSpace(summary),
					Style:   strings.TrimSpace(style),
				}
			}
		}
	}
	return ScriptResult{Content: strings.TrimSpace(stripMarkdownFence(cleaned))}
}

func stripMarkdownFence(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "```") {
		return value
	}
	firstLineEnd := strings.IndexByte(value, '\n')
	if firstLineEnd < 0 {
		return strings.Trim(value, "`")
	}
	value = value[firstLineEnd+1:]
	if end := strings.LastIndex(value, "```"); end >= 0 {
		value = value[:end]
	}
	return strings.TrimSpace(value)
}

func videoProjectScriptSystemPrompt() string {
	return `你是一名面向新手创作者的视频编剧，也熟悉 Seedance 2.0 的镜头生成特点。你的任务是把一句创意、故事梗概或已有草稿整理成可拍摄、可拆解人物/场景/物品/服饰/风格、可继续设计分镜的剧本。
要求：
1. 保留用户的核心主题和已有设定，不擅自改变人物关系或结局；信息不足时做克制、易理解的补全。
2. 使用清晰场次，每场写明时间、地点、出场人物、可见动作；台词单独成行，旁白明确标注。
3. 控制单镜头可表现的动作复杂度，避免抽象空话，保证后续能拆成 Seedance 2.0 的 5/10/15 秒镜头。
4. 对人物外观、关键服饰、场景和道具保持前后一致，不在正文中使用图片1、视频1等素材编号。
5. 语言自然，第一次做视频的人也能直接看懂和修改。
只返回 JSON，不要 Markdown，不要解释，格式为：
{"content":"完整剧本正文","summary":"故事摘要与创作说明","style":"建议的整体视觉风格"}`
}
