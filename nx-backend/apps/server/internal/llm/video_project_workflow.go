package llm

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

type ScriptResult struct {
	Content   string `json:"content"`
	Summary   string `json:"summary"`
	Style     string `json:"style"`
	RawResult string `json:"rawResult"`
}

type BreakdownItem struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	VisualPrompt string `json:"visualPrompt"`
	UsageNote    string `json:"usageNote"`
	Required     bool   `json:"required"`
	Decision     string `json:"decision"`
}

type ProjectStoryBeat struct {
	Key         string   `json:"key"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	SceneKeys   []string `json:"sceneKeys"`
	AssetKeys   []string `json:"assetKeys"`
}

type ProjectBreakdownResult struct {
	Characters []BreakdownItem    `json:"characters"`
	Scenes     []BreakdownItem    `json:"scenes"`
	Props      []BreakdownItem    `json:"props"`
	Outfits    []BreakdownItem    `json:"outfits"`
	Styles     []BreakdownItem    `json:"styles"`
	StoryBeats []ProjectStoryBeat `json:"storyBeats"`
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

func (g *MiniMaxGenerator) BreakdownVideoProjectScript(ctx context.Context, script string) (ProjectBreakdownResult, string, error) {
	script = strings.TrimSpace(script)
	if script == "" {
		return ProjectBreakdownResult{}, "", fmt.Errorf("请先填写并确认剧本内容")
	}
	if g.apiKey == "" {
		return ProjectBreakdownResult{}, "", fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}

	body := map[string]any{
		"model":              g.model,
		"temperature":        0.25,
		"tokens_to_generate": 3200,
		"messages": []map[string]string{
			{"role": "system", "content": videoProjectBreakdownSystemPrompt()},
			{"role": "user", "content": "请拆解以下已确认剧本，只返回约定的 JSON。\n\n剧本正文：\n" + script},
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("/v1/text/chatcompletion_v2"), bytes.NewReader(payload))
	if err != nil {
		return ProjectBreakdownResult{}, "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return ProjectBreakdownResult{}, "", err
	}
	defer resp.Body.Close()
	rawResponse, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProjectBreakdownResult{}, string(rawResponse), fmt.Errorf("剧本拆解模型请求失败(%d): %s", resp.StatusCode, compact(rawResponse))
	}
	var response map[string]any
	if err := json.Unmarshal(rawResponse, &response); err != nil {
		return ProjectBreakdownResult{}, string(rawResponse), fmt.Errorf("剧本拆解模型响应解析失败: %w", err)
	}
	if err := baseRespError(response); err != nil {
		return ProjectBreakdownResult{}, string(rawResponse), err
	}
	answer := strings.TrimSpace(findString(response,
		"choices.0.message.content",
		"choices.0.text",
		"reply",
		"data.reply",
		"data.choices.0.message.content",
	))
	if answer == "" {
		return ProjectBreakdownResult{}, string(rawResponse), fmt.Errorf("剧本拆解模型未返回内容")
	}
	result, err := parseProjectBreakdown(answer)
	if err != nil {
		return ProjectBreakdownResult{}, answer, err
	}
	return result, answer, nil
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

func parseProjectBreakdown(answer string) (ProjectBreakdownResult, error) {
	cleaned := strings.TrimSpace(stripThinkBlocks(answer))
	jsonText, ok := firstJSONObject(cleaned)
	if !ok {
		return ProjectBreakdownResult{}, fmt.Errorf("剧本拆解模型未返回有效 JSON，请重新生成。返回片段：%s", previewText(cleaned))
	}
	fields, err := looseJSONObject(jsonText)
	if err != nil {
		return ProjectBreakdownResult{}, fmt.Errorf("剧本拆解模型未返回有效 JSON，请重新生成。返回片段：%s", previewText(cleaned))
	}

	result := ProjectBreakdownResult{
		Characters: []BreakdownItem{},
		Scenes:     []BreakdownItem{},
		Props:      []BreakdownItem{},
		Outfits:    []BreakdownItem{},
		Styles:     []BreakdownItem{},
		StoryBeats: []ProjectStoryBeat{},
	}
	usedKeys := map[string]bool{}
	for _, category := range []struct {
		name   string
		prefix string
		target *[]BreakdownItem
	}{
		{name: "characters", prefix: "character", target: &result.Characters},
		{name: "scenes", prefix: "scene", target: &result.Scenes},
		{name: "props", prefix: "prop", target: &result.Props},
		{name: "outfits", prefix: "outfit", target: &result.Outfits},
		{name: "styles", prefix: "style", target: &result.Styles},
	} {
		items, parseErr := looseBreakdownItemsField(fields, category.name)
		if parseErr != nil {
			return ProjectBreakdownResult{}, fmt.Errorf("剧本拆解模型字段 %s 格式不正确：%w", category.name, parseErr)
		}
		*category.target = normalizeBreakdownItems(category.prefix, items, usedKeys)
	}
	result.StoryBeats, err = looseProjectStoryBeatsField(fields, "storyBeats")
	if err != nil {
		return ProjectBreakdownResult{}, fmt.Errorf("剧本拆解模型字段 storyBeats 格式不正确：%w", err)
	}
	result.StoryBeats = normalizeProjectStoryBeats(result.StoryBeats, usedKeys)

	if len(result.Characters)+len(result.Scenes)+len(result.Props)+len(result.Outfits)+len(result.Styles) == 0 {
		return ProjectBreakdownResult{}, fmt.Errorf("剧本拆解模型没有识别出人物、场景、物品、服饰或风格，请重新生成")
	}
	return result, nil
}

func looseBreakdownItemsField(fields map[string]json.RawMessage, name string) ([]BreakdownItem, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return []BreakdownItem{}, nil
	}
	itemsRaw, err := looseRawArray(raw)
	if err != nil {
		return nil, err
	}
	items := make([]BreakdownItem, 0, len(itemsRaw))
	for index, rawItem := range itemsRaw {
		itemFields, err := looseJSONObject(string(rawItem))
		if err != nil {
			return nil, fmt.Errorf("第 %d 项不是 JSON 对象", index+1)
		}
		item, err := looseBreakdownItem(itemFields)
		if err != nil {
			return nil, fmt.Errorf("第 %d 项：%w", index+1, err)
		}
		items = append(items, item)
	}
	return items, nil
}

func looseBreakdownItem(fields map[string]json.RawMessage) (BreakdownItem, error) {
	var item BreakdownItem
	var err error
	if item.Key, err = looseStringField(fields, "key"); err != nil {
		return BreakdownItem{}, fmt.Errorf("key 格式不正确：%w", err)
	}
	if item.Name, err = looseStringField(fields, "name"); err != nil {
		return BreakdownItem{}, fmt.Errorf("name 格式不正确：%w", err)
	}
	if item.Description, err = looseStringField(fields, "description"); err != nil {
		return BreakdownItem{}, fmt.Errorf("description 格式不正确：%w", err)
	}
	if item.VisualPrompt, err = looseStringField(fields, "visualPrompt"); err != nil {
		return BreakdownItem{}, fmt.Errorf("visualPrompt 格式不正确：%w", err)
	}
	if item.UsageNote, err = looseStringField(fields, "usageNote"); err != nil {
		return BreakdownItem{}, fmt.Errorf("usageNote 格式不正确：%w", err)
	}
	if item.Required, err = looseBoolField(fields, "required"); err != nil {
		return BreakdownItem{}, fmt.Errorf("required 格式不正确：%w", err)
	}
	if item.Decision, err = looseStringField(fields, "decision"); err != nil {
		return BreakdownItem{}, fmt.Errorf("decision 格式不正确：%w", err)
	}
	return item, nil
}

func normalizeBreakdownItems(category string, items []BreakdownItem, usedKeys map[string]bool) []BreakdownItem {
	result := make([]BreakdownItem, 0, len(items))
	for index, item := range items {
		item.Key = strings.TrimSpace(item.Key)
		item.Name = strings.TrimSpace(item.Name)
		item.Description = strings.TrimSpace(item.Description)
		item.VisualPrompt = strings.TrimSpace(item.VisualPrompt)
		item.UsageNote = strings.TrimSpace(item.UsageNote)
		if item.Name == "" && item.Description == "" && item.VisualPrompt == "" && item.UsageNote == "" {
			continue
		}
		if item.Name == "" {
			item.Name = fmt.Sprintf("未命名%s %d", breakdownCategoryLabel(category), index+1)
		}
		if !validWorkflowKey(item.Key) || usedKeys[item.Key] {
			item.Key = stableWorkflowKey(category, index, item.Name, item.Description, item.VisualPrompt, item.UsageNote)
		}
		for usedKeys[item.Key] {
			item.Key += "x"
		}
		usedKeys[item.Key] = true
		switch item.Decision {
		case "confirmed", "ignored":
		default:
			item.Decision = "pending"
		}
		result = append(result, item)
	}
	return result
}

func looseProjectStoryBeatsField(fields map[string]json.RawMessage, name string) ([]ProjectStoryBeat, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return []ProjectStoryBeat{}, nil
	}
	beatsRaw, err := looseRawArray(raw)
	if err != nil {
		return nil, err
	}
	beats := make([]ProjectStoryBeat, 0, len(beatsRaw))
	for index, rawBeat := range beatsRaw {
		beatFields, err := looseJSONObject(string(rawBeat))
		if err != nil {
			return nil, fmt.Errorf("第 %d 项不是 JSON 对象", index+1)
		}
		var beat ProjectStoryBeat
		if beat.Key, err = looseStringField(beatFields, "key"); err != nil {
			return nil, fmt.Errorf("第 %d 项 key 格式不正确：%w", index+1, err)
		}
		if beat.Title, err = looseStringField(beatFields, "title"); err != nil {
			return nil, fmt.Errorf("第 %d 项 title 格式不正确：%w", index+1, err)
		}
		if beat.Description, err = looseStringField(beatFields, "description"); err != nil {
			return nil, fmt.Errorf("第 %d 项 description 格式不正确：%w", index+1, err)
		}
		if beat.SceneKeys, err = looseStringListField(beatFields, "sceneKeys"); err != nil {
			return nil, fmt.Errorf("第 %d 项 sceneKeys 格式不正确：%w", index+1, err)
		}
		if beat.AssetKeys, err = looseStringListField(beatFields, "assetKeys"); err != nil {
			return nil, fmt.Errorf("第 %d 项 assetKeys 格式不正确：%w", index+1, err)
		}
		beats = append(beats, beat)
	}
	return beats, nil
}

func normalizeProjectStoryBeats(beats []ProjectStoryBeat, usedKeys map[string]bool) []ProjectStoryBeat {
	result := make([]ProjectStoryBeat, 0, len(beats))
	for index, beat := range beats {
		beat.Key = strings.TrimSpace(beat.Key)
		beat.Title = strings.TrimSpace(beat.Title)
		beat.Description = strings.TrimSpace(beat.Description)
		beat.SceneKeys = cleanStringList(beat.SceneKeys)
		beat.AssetKeys = cleanStringList(beat.AssetKeys)
		if beat.Title == "" && beat.Description == "" {
			continue
		}
		if beat.Title == "" {
			beat.Title = fmt.Sprintf("故事节点 %d", index+1)
		}
		if !validWorkflowKey(beat.Key) || usedKeys[beat.Key] {
			beat.Key = stableWorkflowKey("beat", index, beat.Title, beat.Description, strings.Join(beat.SceneKeys, ","), strings.Join(beat.AssetKeys, ","))
		}
		for usedKeys[beat.Key] {
			beat.Key += "x"
		}
		usedKeys[beat.Key] = true
		result = append(result, beat)
	}
	return result
}

var workflowKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func validWorkflowKey(value string) bool {
	return workflowKeyPattern.MatchString(value)
}

func stableWorkflowKey(category string, index int, values ...string) string {
	normalized := make([]string, 0, len(values)+2)
	normalized = append(normalized, category, fmt.Sprint(index+1))
	for _, value := range values {
		normalized = append(normalized, strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	}
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\x00")))
	return fmt.Sprintf("%s:%02d:%x", category, index+1, sum[:6])
}

func breakdownCategoryLabel(category string) string {
	switch category {
	case "character":
		return "人物"
	case "scene":
		return "场景"
	case "prop":
		return "物品"
	case "outfit":
		return "服饰"
	case "style":
		return "风格"
	default:
		return "项目"
	}
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

func videoProjectBreakdownSystemPrompt() string {
	return `你是一名影视制片拆解师，也熟悉 Seedance 2.0 的参考素材工作方式。请把已确认剧本拆成人物、场景、关键物品、服饰和整体风格五类可复用资产，并整理故事节点。
要求：
1. 只列画面中实际出现或维持一致性所必需的内容；同名但不同年龄、造型或身份的对象必须分成独立项目，不能合并。
2. description 用中文说明设定；visualPrompt 写成适合生成标准参考图的完整视觉描述，固定年龄、外貌、材质、颜色、环境或造型特征；usageNote 说明在哪些场次使用。
3. required 表示缺少该参考素材是否会明显影响镜头；decision 一律输出 pending，等待用户确认。
4. key 使用简短、唯一、稳定的英文标识；storyBeats 中优先引用对应 key。不要使用可变名称暗示数据库主键。
5. 结果面向第一次制作视频的用户，名称和描述必须直白清楚。
只返回 JSON，不要 Markdown，不要解释。所有数组无内容时返回 []：
{
  "characters":[{"key":"character-a","name":"名称","description":"设定","visualPrompt":"参考图提示词","usageNote":"使用说明","required":true,"decision":"pending"}],
  "scenes":[{"key":"scene-a","name":"名称","description":"设定","visualPrompt":"参考图提示词","usageNote":"使用说明","required":true,"decision":"pending"}],
  "props":[{"key":"prop-a","name":"名称","description":"设定","visualPrompt":"参考图提示词","usageNote":"使用说明","required":true,"decision":"pending"}],
  "outfits":[{"key":"outfit-a","name":"名称","description":"设定","visualPrompt":"参考图提示词","usageNote":"使用说明","required":true,"decision":"pending"}],
  "styles":[{"key":"style-a","name":"名称","description":"设定","visualPrompt":"参考图提示词","usageNote":"使用说明","required":true,"decision":"pending"}],
  "storyBeats":[{"key":"beat-a","title":"故事节点","description":"发生的事件","sceneKeys":["scene-a"],"assetKeys":["character-a","prop-a"]}]
}`
}
