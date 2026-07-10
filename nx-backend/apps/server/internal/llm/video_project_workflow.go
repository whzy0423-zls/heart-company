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

type ProjectAssetSummary struct {
	Key               string `json:"key"`
	Type              string `json:"type"`
	Name              string `json:"name"`
	Description       string `json:"description"`
	VisualPrompt      string `json:"visualPrompt"`
	ReferenceImageURL string `json:"referenceImageUrl"`
	Required          bool   `json:"required"`
}

type ProjectStoryboardInput struct {
	Script            string                `json:"script"`
	ScriptRevision    int                   `json:"sourceScriptRevision"`
	BreakdownID       string                `json:"sourceBreakdownId"`
	AssetRevision     int                   `json:"sourceAssetRevision"`
	CapabilityVersion string                `json:"sourceCapabilityVersion"`
	Model             string                `json:"model"`
	AspectRatio       string                `json:"aspectRatio"`
	AllowedDurations  []int                 `json:"allowedDurations"`
	TaskModes         []string              `json:"taskModes"`
	ReferenceRoles    []string              `json:"referenceRoles"`
	Assets            []ProjectAssetSummary `json:"confirmedAssets"`
}

type ProjectStoryboardReference struct {
	AssetKey  string `json:"assetKey"`
	Role      string `json:"role"`
	SortOrder int    `json:"sortOrder"`
	UsageNote string `json:"usageNote"`
}

type ProjectStoryboardShot struct {
	SourceKey     string                       `json:"sourceKey"`
	Name          string                       `json:"name"`
	Enabled       bool                         `json:"enabled"`
	Duration      int                          `json:"duration"`
	SceneKey      string                       `json:"sceneKey"`
	CharacterKeys []string                     `json:"characterKeys"`
	AssetKeys     []string                     `json:"assetKeys"`
	Action        string                       `json:"action"`
	Camera        string                       `json:"camera"`
	Composition   string                       `json:"composition"`
	Lighting      string                       `json:"lighting"`
	Audio         string                       `json:"audio"`
	Dialogue      string                       `json:"dialogue"`
	TaskMode      string                       `json:"taskMode"`
	References    []ProjectStoryboardReference `json:"references"`
}

type ProjectStoryboardResult struct {
	Shots []ProjectStoryboardShot `json:"shots"`
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

func (g *MiniMaxGenerator) DesignVideoProjectStoryboard(ctx context.Context, input ProjectStoryboardInput) (ProjectStoryboardResult, string, error) {
	input.Script = strings.TrimSpace(input.Script)
	if input.Script == "" {
		return ProjectStoryboardResult{}, "", fmt.Errorf("请先填写并确认剧本内容")
	}
	if strings.TrimSpace(input.BreakdownID) == "" {
		return ProjectStoryboardResult{}, "", fmt.Errorf("请先确认剧本拆解结果")
	}
	if strings.TrimSpace(input.CapabilityVersion) == "" {
		return ProjectStoryboardResult{}, "", fmt.Errorf("缺少当前 Seedance 2.0 能力版本，请刷新后重试")
	}
	if g.apiKey == "" {
		return ProjectStoryboardResult{}, "", fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}

	input.Assets = normalizeProjectAssetSummaries(input.Assets)
	userPayload, _ := json.Marshal(input)
	body := map[string]any{
		"model":              g.model,
		"temperature":        0.3,
		"tokens_to_generate": 4200,
		"messages": []map[string]string{
			{"role": "system", "content": videoProjectStoryboardSystemPrompt()},
			{"role": "user", "content": "请根据以下已确认剧本、资产和当前能力设计可编辑分镜。只返回约定的 JSON。\n" + string(userPayload)},
		},
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("/v1/text/chatcompletion_v2"), bytes.NewReader(payload))
	if err != nil {
		return ProjectStoryboardResult{}, "", err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return ProjectStoryboardResult{}, "", err
	}
	defer resp.Body.Close()
	rawResponse, _ := io.ReadAll(io.LimitReader(resp.Body, 6*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ProjectStoryboardResult{}, string(rawResponse), fmt.Errorf("AI 分镜模型请求失败(%d): %s", resp.StatusCode, compact(rawResponse))
	}
	var response map[string]any
	if err := json.Unmarshal(rawResponse, &response); err != nil {
		return ProjectStoryboardResult{}, string(rawResponse), fmt.Errorf("AI 分镜模型响应解析失败: %w", err)
	}
	if err := baseRespError(response); err != nil {
		return ProjectStoryboardResult{}, string(rawResponse), err
	}
	answer := strings.TrimSpace(findString(response,
		"choices.0.message.content",
		"choices.0.text",
		"reply",
		"data.reply",
		"data.choices.0.message.content",
	))
	if answer == "" {
		return ProjectStoryboardResult{}, string(rawResponse), fmt.Errorf("AI 分镜模型未返回内容")
	}
	result, err := parseProjectStoryboard(answer)
	if err != nil {
		return ProjectStoryboardResult{}, answer, err
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

func parseProjectStoryboard(answer string) (ProjectStoryboardResult, error) {
	cleaned := strings.TrimSpace(stripThinkBlocks(answer))
	jsonText, ok := firstJSONObject(cleaned)
	if !ok {
		return ProjectStoryboardResult{}, fmt.Errorf("AI 分镜模型未返回有效 JSON，请重新生成。返回片段：%s", previewText(cleaned))
	}
	fields, err := looseJSONObject(jsonText)
	if err != nil {
		return ProjectStoryboardResult{}, fmt.Errorf("AI 分镜模型未返回有效 JSON，请重新生成。返回片段：%s", previewText(cleaned))
	}
	shots, err := looseProjectStoryboardShotsField(fields, "shots")
	if err != nil {
		return ProjectStoryboardResult{}, fmt.Errorf("AI 分镜模型字段 shots 格式不正确：%w", err)
	}
	shots = normalizeProjectStoryboardShots(shots)
	if len(shots) == 0 {
		return ProjectStoryboardResult{}, fmt.Errorf("AI 分镜模型没有返回可编辑镜头，请重新生成")
	}
	return ProjectStoryboardResult{Shots: shots}, nil
}

func looseProjectStoryboardShotsField(fields map[string]json.RawMessage, name string) ([]ProjectStoryboardShot, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return []ProjectStoryboardShot{}, nil
	}
	shotsRaw, err := looseRawArray(raw)
	if err != nil {
		return nil, err
	}
	shots := make([]ProjectStoryboardShot, 0, len(shotsRaw))
	for index, rawShot := range shotsRaw {
		shotFields, err := looseJSONObject(string(rawShot))
		if err != nil {
			return nil, fmt.Errorf("第 %d 个镜头不是 JSON 对象", index+1)
		}
		shot, err := looseProjectStoryboardShot(shotFields)
		if err != nil {
			return nil, fmt.Errorf("第 %d 个镜头：%w", index+1, err)
		}
		shots = append(shots, shot)
	}
	return shots, nil
}

func looseProjectStoryboardShot(fields map[string]json.RawMessage) (ProjectStoryboardShot, error) {
	var shot ProjectStoryboardShot
	var err error
	if shot.SourceKey, err = looseStringField(fields, "sourceKey"); err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("sourceKey 格式不正确：%w", err)
	}
	if shot.Name, err = looseStringField(fields, "name"); err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("name 格式不正确：%w", err)
	}
	shot.Enabled, err = looseBoolFieldDefault(fields, "enabled", true)
	if err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("enabled 格式不正确：%w", err)
	}
	if shot.Duration, err = looseIntField(fields, "duration"); err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("duration 格式不正确：%w", err)
	}
	if shot.SceneKey, err = looseStringField(fields, "sceneKey"); err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("sceneKey 格式不正确：%w", err)
	}
	if shot.CharacterKeys, err = looseStringListField(fields, "characterKeys"); err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("characterKeys 格式不正确：%w", err)
	}
	if shot.AssetKeys, err = looseStringListField(fields, "assetKeys"); err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("assetKeys 格式不正确：%w", err)
	}
	for fieldName, target := range map[string]*string{
		"action": &shot.Action, "camera": &shot.Camera, "composition": &shot.Composition,
		"lighting": &shot.Lighting, "audio": &shot.Audio, "dialogue": &shot.Dialogue,
		"taskMode": &shot.TaskMode,
	} {
		if *target, err = looseStringField(fields, fieldName); err != nil {
			return ProjectStoryboardShot{}, fmt.Errorf("%s 格式不正确：%w", fieldName, err)
		}
	}
	if shot.References, err = looseProjectStoryboardReferencesField(fields, "references"); err != nil {
		return ProjectStoryboardShot{}, fmt.Errorf("references 格式不正确：%w", err)
	}
	return shot, nil
}

func looseBoolFieldDefault(fields map[string]json.RawMessage, name string, defaultValue bool) (bool, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return defaultValue, nil
	}
	return looseBoolField(fields, name)
}

func looseProjectStoryboardReferencesField(fields map[string]json.RawMessage, name string) ([]ProjectStoryboardReference, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return []ProjectStoryboardReference{}, nil
	}
	referencesRaw, err := looseRawArray(raw)
	if err != nil {
		return nil, err
	}
	references := make([]ProjectStoryboardReference, 0, len(referencesRaw))
	for index, rawReference := range referencesRaw {
		referenceFields, err := looseJSONObject(string(rawReference))
		if err != nil {
			return nil, fmt.Errorf("第 %d 项不是 JSON 对象", index+1)
		}
		var reference ProjectStoryboardReference
		if reference.AssetKey, err = looseStringField(referenceFields, "assetKey"); err != nil {
			return nil, fmt.Errorf("第 %d 项 assetKey 格式不正确：%w", index+1, err)
		}
		if reference.Role, err = looseStringField(referenceFields, "role"); err != nil {
			return nil, fmt.Errorf("第 %d 项 role 格式不正确：%w", index+1, err)
		}
		if reference.SortOrder, err = looseIntField(referenceFields, "sortOrder"); err != nil {
			return nil, fmt.Errorf("第 %d 项 sortOrder 格式不正确：%w", index+1, err)
		}
		if reference.UsageNote, err = looseStringField(referenceFields, "usageNote"); err != nil {
			return nil, fmt.Errorf("第 %d 项 usageNote 格式不正确：%w", index+1, err)
		}
		references = append(references, reference)
	}
	return references, nil
}

func normalizeProjectStoryboardShots(shots []ProjectStoryboardShot) []ProjectStoryboardShot {
	result := make([]ProjectStoryboardShot, 0, len(shots))
	usedKeys := map[string]bool{}
	for index, shot := range shots {
		shot.SourceKey = strings.TrimSpace(shot.SourceKey)
		shot.Name = strings.TrimSpace(shot.Name)
		shot.SceneKey = strings.TrimSpace(shot.SceneKey)
		shot.CharacterKeys = cleanStringList(shot.CharacterKeys)
		shot.AssetKeys = cleanStringList(shot.AssetKeys)
		shot.Action = strings.TrimSpace(shot.Action)
		shot.Camera = strings.TrimSpace(shot.Camera)
		shot.Composition = strings.TrimSpace(shot.Composition)
		shot.Lighting = strings.TrimSpace(shot.Lighting)
		shot.Audio = strings.TrimSpace(shot.Audio)
		shot.Dialogue = strings.TrimSpace(shot.Dialogue)
		shot.TaskMode = strings.ToLower(strings.TrimSpace(shot.TaskMode))
		if shot.TaskMode == "" {
			shot.TaskMode = "reference"
		}
		if shot.Name == "" {
			shot.Name = fmt.Sprintf("镜头 %d", index+1)
		}
		if shot.Duration < 0 {
			shot.Duration = 0
		}
		if !validWorkflowKey(shot.SourceKey) || usedKeys[shot.SourceKey] {
			shot.SourceKey = stableWorkflowKey("shot", index, shot.Name, shot.SceneKey, strings.Join(shot.CharacterKeys, ","), strings.Join(shot.AssetKeys, ","), shot.Action, shot.Camera)
		}
		for usedKeys[shot.SourceKey] {
			shot.SourceKey += "x"
		}
		usedKeys[shot.SourceKey] = true
		shot.References = normalizeProjectStoryboardReferences(shot.References)
		result = append(result, shot)
	}
	return result
}

func normalizeProjectStoryboardReferences(references []ProjectStoryboardReference) []ProjectStoryboardReference {
	result := make([]ProjectStoryboardReference, 0, len(references))
	for index, reference := range references {
		reference.AssetKey = strings.TrimSpace(reference.AssetKey)
		reference.Role = strings.ToLower(strings.TrimSpace(reference.Role))
		reference.UsageNote = strings.TrimSpace(reference.UsageNote)
		if reference.AssetKey == "" {
			continue
		}
		if reference.Role == "" {
			reference.Role = "reference_image"
		}
		if reference.SortOrder <= 0 {
			reference.SortOrder = index + 1
		}
		result = append(result, reference)
	}
	return result
}

func normalizeProjectAssetSummaries(assets []ProjectAssetSummary) []ProjectAssetSummary {
	result := make([]ProjectAssetSummary, 0, len(assets))
	for _, asset := range assets {
		asset.Key = strings.TrimSpace(asset.Key)
		asset.Type = strings.TrimSpace(asset.Type)
		asset.Name = strings.TrimSpace(asset.Name)
		asset.Description = strings.TrimSpace(asset.Description)
		asset.VisualPrompt = strings.TrimSpace(asset.VisualPrompt)
		asset.ReferenceImageURL = strings.TrimSpace(asset.ReferenceImageURL)
		if asset.Key == "" || asset.Name == "" {
			continue
		}
		result = append(result, asset)
	}
	return result
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

func videoProjectStoryboardSystemPrompt() string {
	return `你是一名 Seedance 2.0 分镜导演，任务是把已确认剧本和资产设计成新手可以逐镜修改、直接进入提示词编译的结构化分镜。
要求：
1. 每个镜头只安排模型在给定 allowedDurations 内能完整表现的动作；duration 必须选择 allowedDurations 中的值，画面比例、模型和能力以输入为准。
2. sceneKey、characterKeys、assetKeys 和 references.assetKey 只能引用 confirmedAssets 里的稳定 key，不使用资产名称代替 key，不虚构不存在的参考素材。
3. action 写主体可见动作；camera 只写一种主要运镜；composition、lighting、audio、dialogue 分开填写。台词写“人物：台词”，没有则为空字符串。
4. taskMode 只能从 taskModes 选择；reference 的 role 只能从 referenceRoles 选择。普通人物、场景、物品和服饰参考图使用 reference_image，素材顺序按重要性从 1 开始。
5. sourceKey 必须唯一稳定；enabled 默认 true。内容要直白、可拍摄，不写“图片1/视频1/音频1”，素材编号由后续编译器统一生成。
6. 不承诺当前能力未声明的分辨率、编辑、延长、首尾帧或音画能力。
只返回 JSON，不要 Markdown，不要解释：
{"shots":[{"sourceKey":"shot-01","name":"镜头名称","enabled":true,"duration":5,"sceneKey":"scene-a","characterKeys":["character-a"],"assetKeys":["prop-a"],"action":"可见动作","camera":"景别和运镜","composition":"构图","lighting":"光线和色彩","audio":"环境声/音乐/音效","dialogue":"人物：台词","taskMode":"reference","references":[{"assetKey":"character-a","role":"reference_image","sortOrder":1,"usageNote":"保持人物一致"}]}]}`
}
