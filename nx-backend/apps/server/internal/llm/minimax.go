package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/netguard"
)

type MiniMaxGenerator struct {
	apiBase string
	apiKey  string
	client  *http.Client
	groupID string
	model   string
}

type VideoAnalysisResult struct {
	Assets         []string `json:"assets"`
	AudioSummary   string   `json:"audioSummary"`
	Characters     []string `json:"characters"`
	HasSpeech      bool     `json:"hasSpeech"`
	RawResult      string   `json:"rawResult"`
	Scenes         []string `json:"scenes"`
	SeedancePrompt string   `json:"seedancePrompt"`
	SpeechKeywords []string `json:"speechKeywords"`
	SpeechOutline  []string `json:"speechOutline"`
	SpeechTopics   []string `json:"speechTopics"`
}

type StoryboardRawResultError struct {
	Message   string
	RawResult string
}

func (e *StoryboardRawResultError) Error() string {
	return e.Message
}

func NewStoryboardRawResultError(message string, rawResult string) error {
	return &StoryboardRawResultError{Message: message, RawResult: rawResult}
}

func StoryboardRawResultFromError(err error) string {
	var rawErr *StoryboardRawResultError
	if errors.As(err, &rawErr) {
		return strings.TrimSpace(rawErr.RawResult)
	}
	return ""
}

type VideoStoryboardInput struct {
	AnalysisID     string   `json:"analysisId"`
	Assets         []string `json:"assets"`
	AudioSummary   string   `json:"audioSummary"`
	Characters     []string `json:"characters"`
	Scenes         []string `json:"scenes"`
	SeedancePrompt string   `json:"seedancePrompt"`
	SpeechKeywords []string `json:"speechKeywords"`
	SpeechOutline  []string `json:"speechOutline"`
	SpeechTopics   []string `json:"speechTopics"`
	Theme          string   `json:"theme"`
	VideoName      string   `json:"videoName"`
}

type VideoStoryboardResult struct {
	GlobalPrompt string                `json:"globalPrompt"`
	RawResult    string                `json:"rawResult"`
	Shots        []VideoStoryboardShot `json:"shots"`
	StyleGuide   []string              `json:"styleGuide"`
	Title        string                `json:"title"`
}

type VideoStoryboardShot struct {
	Action         string   `json:"action"`
	Assets         []string `json:"assets"`
	Audio          string   `json:"audio"`
	Camera         string   `json:"camera"`
	Characters     []string `json:"characters"`
	Composition    string   `json:"composition"`
	Dialogue       string   `json:"dialogue"`
	Duration       float64  `json:"duration"`
	Index          int      `json:"index"`
	Lighting       string   `json:"lighting"`
	Scene          string   `json:"scene"`
	SeedancePrompt string   `json:"seedancePrompt"`
	Title          string   `json:"title"`
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
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "abab6.5s-chat"
	}
	return &MiniMaxGenerator{
		apiBase: apiBase,
		apiKey:  strings.TrimSpace(cfg.APIKey),
		client: &http.Client{
			Timeout:   timeout,
			Transport: netguard.NewGuardedTransport(),
		},
		groupID: strings.TrimSpace(cfg.GroupID),
		model:   model,
	}
}

// AnalyzeVideo 根据公开视频地址输出结构化视频分析和 seedance2.0 参考提示词。
func (g *MiniMaxGenerator) AnalyzeVideo(ctx context.Context, videoURL, videoName string) (VideoAnalysisResult, error) {
	if g.apiKey == "" {
		return VideoAnalysisResult{}, fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		return VideoAnalysisResult{}, fmt.Errorf("请先上传视频")
	}
	if strings.TrimSpace(videoName) == "" {
		videoName = "参考视频"
	}

	userText := fmt.Sprintf("请直接读取随消息附带的 video_url，分析这个参考视频，并按要求返回 JSON。\n视频名称：%s", videoName)
	messages := []map[string]any{
		{"role": "system", "content": videoAnalysisSystemPrompt()},
		{"role": "user", "content": fmt.Sprintf("%s\n视频地址：%s", userText, videoURL)},
	}
	body := map[string]any{
		"model":       g.model,
		"temperature": 0.25,
		"messages":    messages,
	}
	endpoint := g.endpoint("/v1/text/chatcompletion_v2")
	if g.useOpenAICompatibleAnalysis() {
		messages[1]["content"] = []map[string]any{
			{
				"type": "text",
				"text": userText,
			},
			{
				"type": "video_url",
				"video_url": map[string]any{
					"url": videoURL,
				},
			},
		}
		if g.isMiniMaxM3() {
			body["max_completion_tokens"] = 1200
			body["thinking"] = map[string]string{"type": "disabled"}
		} else {
			body["max_tokens"] = 1200
		}
		endpoint = g.endpoint("/v1/chat/completions")
	} else {
		body["tokens_to_generate"] = 1200
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return VideoAnalysisResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return VideoAnalysisResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型请求失败(%d): %s", resp.StatusCode, compact(raw))
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return VideoAnalysisResult{}, err
	}
	if err := baseRespError(result); err != nil {
		return VideoAnalysisResult{}, err
	}
	answer := findString(result,
		"choices.0.message.content",
		"choices.0.text",
		"reply",
		"data.reply",
		"data.choices.0.message.content",
	)
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型未返回视频分析结果")
	}
	parsed, err := parseVideoAnalysis(answer)
	if err != nil {
		return VideoAnalysisResult{}, err
	}
	parsed.RawResult = answer
	if strings.TrimSpace(parsed.SeedancePrompt) == "" {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型未返回 seedance2.0 参考提示词")
	}
	return parsed, nil
}

func (g *MiniMaxGenerator) GenerateVideoStoryboard(ctx context.Context, input VideoStoryboardInput) (VideoStoryboardResult, error) {
	if g.apiKey == "" {
		return VideoStoryboardResult{}, fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}
	input.Theme = strings.TrimSpace(input.Theme)
	if input.Theme == "" {
		return VideoStoryboardResult{}, fmt.Errorf("请输入分镜主题")
	}
	userText := buildVideoStoryboardUserPrompt(input)
	messages := []map[string]any{
		{"role": "system", "content": videoStoryboardSystemPrompt()},
		{"role": "user", "content": userText},
	}
	body := map[string]any{
		"model":       g.model,
		"temperature": 0.35,
		"messages":    messages,
	}
	endpoint := g.endpoint("/v1/text/chatcompletion_v2")
	if g.useOpenAICompatibleAnalysis() {
		if g.isMiniMaxM3() {
			body["max_completion_tokens"] = 1800
			body["thinking"] = map[string]string{"type": "disabled"}
		} else {
			body["max_tokens"] = 1800
		}
		endpoint = g.endpoint("/v1/chat/completions")
	} else {
		body["tokens_to_generate"] = 1800
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return VideoStoryboardResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return VideoStoryboardResult{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return VideoStoryboardResult{}, NewStoryboardRawResultError(
			fmt.Sprintf("分镜设计模型请求失败(%d): %s", resp.StatusCode, compact(raw)),
			string(raw),
		)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return VideoStoryboardResult{}, NewStoryboardRawResultError(err.Error(), string(raw))
	}
	if err := baseRespError(result); err != nil {
		return VideoStoryboardResult{}, NewStoryboardRawResultError(err.Error(), string(raw))
	}
	answer := findString(result,
		"choices.0.message.content",
		"choices.0.text",
		"reply",
		"data.reply",
		"data.choices.0.message.content",
	)
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return VideoStoryboardResult{}, NewStoryboardRawResultError("分镜设计模型未返回结果", string(raw))
	}
	parsed, err := parseVideoStoryboardDesign(answer)
	if err != nil {
		return VideoStoryboardResult{}, NewStoryboardRawResultError(err.Error(), answer)
	}
	parsed.RawResult = answer
	if len(parsed.Shots) == 0 {
		return VideoStoryboardResult{}, NewStoryboardRawResultError(
			fmt.Sprintf("分镜设计模型未返回分镜明细，返回片段：%s", previewText(answer)),
			answer,
		)
	}
	return parsed, nil
}

func (g *MiniMaxGenerator) useOpenAICompatibleAnalysis() bool {
	model := strings.ToLower(strings.TrimSpace(g.model))
	base := strings.ToLower(strings.TrimSpace(g.apiBase))
	if strings.Contains(model, "minimax-m3") {
		return true
	}
	if strings.Contains(base, "api.minimaxi.com") || strings.HasPrefix(model, "abab") {
		return false
	}
	return strings.HasPrefix(model, "gpt-") ||
		strings.HasPrefix(model, "o") ||
		strings.Contains(base, "coding-play") ||
		strings.Contains(base, "openai")
}

func (g *MiniMaxGenerator) isMiniMaxM3() bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(g.model)), "minimax-m3")
}

func buildVideoStoryboardUserPrompt(input VideoStoryboardInput) string {
	payload := map[string]any{
		"assets":         cleanStringList(input.Assets),
		"audioSummary":   strings.TrimSpace(input.AudioSummary),
		"characters":     cleanStringList(input.Characters),
		"scenes":         cleanStringList(input.Scenes),
		"seedancePrompt": strings.TrimSpace(input.SeedancePrompt),
		"speechKeywords": cleanStringList(input.SpeechKeywords),
		"speechOutline":  cleanStringList(input.SpeechOutline),
		"speechTopics":   cleanStringList(input.SpeechTopics),
		"theme":          strings.TrimSpace(input.Theme),
		"videoName":      strings.TrimSpace(input.VideoName),
	}
	raw, _ := json.Marshal(payload)
	return "请基于以下视频解析结果和主题，设计一套适合 Seedance 2.0 的可编辑分镜方案。只返回 JSON。\n" + string(raw)
}

func videoStoryboardSystemPrompt() string {
	return `你是一名 Seedance 2.0 分镜导演和提示词工程师。你会根据参考视频解析结果和用户给定主题，设计可执行、可编辑、便于复制到视频生成模型的分镜方案。
请只返回 JSON，不要 Markdown，不要解释。JSON 字段必须为：
{
  "title": "分镜方案标题",
  "styleGuide": ["全片统一风格、镜头语言、光影、色彩、质感等，3-8项"],
  "globalPrompt": "整套视频的全局 Seedance 2.0 参考提示词",
  "shots": [
    {
      "index": 1,
      "title": "镜头标题",
      "duration": 3,
      "scene": "场景/环境/时段",
      "characters": ["人物/主体"],
      "assets": ["可复用资产/道具/服装/声音/风格"],
      "action": "主体动作和情绪",
      "camera": "镜头运动、景别、机位",
      "composition": "构图和画面重点",
      "lighting": "光影、色彩、质感",
      "audio": "音乐/环境声/旁白方向",
      "dialogue": "可选台词或旁白，没有则为空字符串",
      "seedancePrompt": "单镜头中文 Seedance 2.0 提示词，包含主体、场景、动作、镜头、光影、质感、节奏"
    }
  ]
}
分镜数量控制在 4-8 个。必须贴合用户主题，同时继承参考视频解析出的场景、人物、资产、语音主题和风格。不要编造无法从解析中支持的具体品牌、人物身份或台词。`
}

func videoAnalysisSystemPrompt() string {
	return `你是一名资深视频解析与 Seedance 2.0 提示词工程师。你会根据用户给出的视频地址和名称，尽可能分析视频内容，提取适合复刻或二创的结构化信息。
请只返回 JSON，不要 Markdown，不要解释。JSON 字段必须为：
{
  "scenes": ["场景/环境/时段/光线等，3-8项"],
  "characters": ["人物/主体/外观/状态/动作等，2-8项"],
  "assets": ["可复用资产，例如道具、服装、镜头、画面风格、声音等，3-10项"],
  "hasSpeech": true,
  "audioSummary": "如果视频中有人声、旁白、对白或可理解的语音内容，用1-3句话概括语音内容主题；如果没有可识别语音则为空字符串",
  "speechTopics": ["语音/旁白/对白中的主题，0-8项"],
  "speechKeywords": ["语音内容关键词，0-12项"],
  "speechOutline": ["按顺序提炼语音内容大纲，0-8项"],
  "seedancePrompt": "一段中文 seedance2.0 视频生成参考提示词，包含主体、场景、动作、镜头运动、景别、光影氛围、质感风格、节奏和时长感"
}
请尽量同时分析视频中的人声、旁白、对白和背景声音。如果无法确认语音内容，hasSpeech 设为 false，语音相关数组返回空数组，不要编造具体台词或主题。
如果无法真正读取视频，也要根据可用信息保守输出，并在提示词中避免编造具体不可确认细节。`
}

func parseVideoAnalysis(answer string) (VideoAnalysisResult, error) {
	answer = strings.TrimSpace(stripThinkBlocks(answer))
	jsonText, ok := firstJSONObject(answer)
	if !ok {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型未返回有效 JSON，请重试或换一个较短、可公开读取的视频。返回片段：%s", previewText(answer))
	}
	fields, err := looseJSONObject(jsonText)
	if err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型未返回有效 JSON，请重试或换一个较短、可公开读取的视频。返回片段：%s", previewText(answer))
	}
	var result VideoAnalysisResult
	if result.Scenes, err = looseStringListField(fields, "scenes"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 scenes 格式不正确：%w", err)
	}
	if result.Characters, err = looseStringListField(fields, "characters"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 characters 格式不正确：%w", err)
	}
	if result.Assets, err = looseStringListField(fields, "assets"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 assets 格式不正确：%w", err)
	}
	if result.HasSpeech, err = looseBoolField(fields, "hasSpeech"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 hasSpeech 格式不正确：%w", err)
	}
	if result.AudioSummary, err = looseStringField(fields, "audioSummary"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 audioSummary 格式不正确：%w", err)
	}
	if result.SpeechTopics, err = looseStringListField(fields, "speechTopics"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 speechTopics 格式不正确：%w", err)
	}
	if result.SpeechKeywords, err = looseStringListField(fields, "speechKeywords"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 speechKeywords 格式不正确：%w", err)
	}
	if result.SpeechOutline, err = looseStringListField(fields, "speechOutline"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 speechOutline 格式不正确：%w", err)
	}
	if result.SeedancePrompt, err = looseStringField(fields, "seedancePrompt"); err != nil {
		return VideoAnalysisResult{}, fmt.Errorf("视频分析模型字段 seedancePrompt 格式不正确：%w", err)
	}
	result.Scenes = cleanStringList(result.Scenes)
	result.Characters = cleanStringList(result.Characters)
	result.Assets = cleanStringList(result.Assets)
	result.AudioSummary = strings.TrimSpace(result.AudioSummary)
	result.SpeechTopics = cleanStringList(result.SpeechTopics)
	result.SpeechKeywords = cleanStringList(result.SpeechKeywords)
	result.SpeechOutline = cleanStringList(result.SpeechOutline)
	result.SeedancePrompt = strings.TrimSpace(result.SeedancePrompt)
	return result, nil
}

func parseVideoStoryboardDesign(answer string) (VideoStoryboardResult, error) {
	answer = strings.TrimSpace(stripThinkBlocks(answer))
	jsonText, ok := firstJSONObject(answer)
	if !ok {
		return VideoStoryboardResult{}, fmt.Errorf("分镜设计模型未返回有效 JSON，请重试或调整主题。返回片段：%s", previewText(answer))
	}
	fields, err := looseJSONObject(jsonText)
	if err != nil {
		return VideoStoryboardResult{}, fmt.Errorf("分镜设计模型未返回有效 JSON，请重试或调整主题。返回片段：%s", previewText(answer))
	}
	var result VideoStoryboardResult
	if result.Title, err = looseStringField(fields, "title"); err != nil {
		return VideoStoryboardResult{}, fmt.Errorf("分镜设计模型字段 title 格式不正确：%w", err)
	}
	if result.StyleGuide, err = looseStringListField(fields, "styleGuide"); err != nil {
		return VideoStoryboardResult{}, fmt.Errorf("分镜设计模型字段 styleGuide 格式不正确：%w", err)
	}
	if result.GlobalPrompt, err = looseStringField(fields, "globalPrompt"); err != nil {
		return VideoStoryboardResult{}, fmt.Errorf("分镜设计模型字段 globalPrompt 格式不正确：%w", err)
	}
	if result.Shots, err = looseStoryboardShotsField(fields, "shots"); err != nil {
		return VideoStoryboardResult{}, fmt.Errorf("分镜设计模型字段 shots 格式不正确：%w", err)
	}
	result.Title = strings.TrimSpace(result.Title)
	result.GlobalPrompt = strings.TrimSpace(result.GlobalPrompt)
	result.StyleGuide = cleanStringList(result.StyleGuide)
	result.Shots = cleanStoryboardShots(result.Shots)
	return result, nil
}

func cleanStoryboardShots(values []VideoStoryboardShot) []VideoStoryboardShot {
	out := []VideoStoryboardShot{}
	for index, shot := range values {
		shot.Title = strings.TrimSpace(shot.Title)
		shot.Scene = strings.TrimSpace(shot.Scene)
		shot.Action = strings.TrimSpace(shot.Action)
		shot.Camera = strings.TrimSpace(shot.Camera)
		shot.Composition = strings.TrimSpace(shot.Composition)
		shot.Lighting = strings.TrimSpace(shot.Lighting)
		shot.Audio = strings.TrimSpace(shot.Audio)
		shot.Dialogue = strings.TrimSpace(shot.Dialogue)
		shot.SeedancePrompt = strings.TrimSpace(shot.SeedancePrompt)
		shot.Characters = cleanStringList(shot.Characters)
		shot.Assets = cleanStringList(shot.Assets)
		if shot.Index <= 0 {
			shot.Index = index + 1
		}
		if shot.Duration < 0 {
			shot.Duration = 0
		}
		if shot.Title == "" && shot.Scene == "" && shot.Action == "" && shot.SeedancePrompt == "" {
			continue
		}
		out = append(out, shot)
	}
	return out
}

func looseJSONObject(value string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = map[string]json.RawMessage{}
	}
	return fields, nil
}

func looseStringField(fields map[string]json.RawMessage, name string) (string, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return "", nil
	}
	value, err := decodeLooseValue(raw)
	if err != nil {
		return "", err
	}
	return looseStringValue(value)
}

func looseStringListField(fields map[string]json.RawMessage, name string) ([]string, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	value, err := decodeLooseValue(raw)
	if err != nil {
		return nil, err
	}
	return looseStringListValue(value)
}

func looseBoolField(fields map[string]json.RawMessage, name string) (bool, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return false, nil
	}
	value, err := decodeLooseValue(raw)
	if err != nil {
		return false, err
	}
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "", "false", "0", "no", "n", "否", "无":
			return false, nil
		case "true", "1", "yes", "y", "是", "有":
			return true, nil
		default:
			return false, fmt.Errorf("不能把 %q 转为布尔值", v)
		}
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return false, err
		}
		return n != 0, nil
	case nil:
		return false, nil
	default:
		return false, fmt.Errorf("期望布尔值或字符串，实际是 %T", value)
	}
}

func looseStoryboardShotsField(fields map[string]json.RawMessage, name string) ([]VideoStoryboardShot, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	shotsRaw, err := looseRawArray(raw)
	if err != nil {
		return nil, err
	}
	shots := make([]VideoStoryboardShot, 0, len(shotsRaw))
	for i, rawShot := range shotsRaw {
		shotFields, err := looseJSONObject(string(rawShot))
		if err != nil {
			return nil, fmt.Errorf("第 %d 个镜头不是 JSON 对象", i+1)
		}
		shot, err := looseStoryboardShot(shotFields)
		if err != nil {
			return nil, fmt.Errorf("第 %d 个镜头：%w", i+1, err)
		}
		shots = append(shots, shot)
	}
	return shots, nil
}

func looseStoryboardShot(fields map[string]json.RawMessage) (VideoStoryboardShot, error) {
	var shot VideoStoryboardShot
	var err error
	if shot.Index, err = looseIntField(fields, "index"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("index 格式不正确：%w", err)
	}
	if shot.Title, err = looseStringField(fields, "title"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("title 格式不正确：%w", err)
	}
	if shot.Duration, err = looseDurationField(fields, "duration"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("duration 格式不正确：%w", err)
	}
	if shot.Scene, err = looseStringField(fields, "scene"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("scene 格式不正确：%w", err)
	}
	if shot.Characters, err = looseStringListField(fields, "characters"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("characters 格式不正确：%w", err)
	}
	if shot.Assets, err = looseStringListField(fields, "assets"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("assets 格式不正确：%w", err)
	}
	if shot.Action, err = looseStringField(fields, "action"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("action 格式不正确：%w", err)
	}
	if shot.Camera, err = looseStringField(fields, "camera"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("camera 格式不正确：%w", err)
	}
	if shot.Composition, err = looseStringField(fields, "composition"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("composition 格式不正确：%w", err)
	}
	if shot.Lighting, err = looseStringField(fields, "lighting"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("lighting 格式不正确：%w", err)
	}
	if shot.Audio, err = looseStringField(fields, "audio"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("audio 格式不正确：%w", err)
	}
	if shot.Dialogue, err = looseStringField(fields, "dialogue"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("dialogue 格式不正确：%w", err)
	}
	if shot.SeedancePrompt, err = looseStringField(fields, "seedancePrompt"); err != nil {
		return VideoStoryboardShot{}, fmt.Errorf("seedancePrompt 格式不正确：%w", err)
	}
	return shot, nil
}

func looseIntField(fields map[string]json.RawMessage, name string) (int, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return 0, nil
	}
	value, err := decodeLooseValue(raw)
	if err != nil {
		return 0, err
	}
	switch v := value.(type) {
	case json.Number:
		i, err := strconv.Atoi(v.String())
		if err == nil {
			return i, nil
		}
		f, err := v.Float64()
		return int(f), err
	case string:
		n, ok := firstFloatInString(v)
		if !ok {
			return 0, fmt.Errorf("不能把 %q 转为整数", v)
		}
		return int(n), nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("期望数字或字符串，实际是 %T", value)
	}
}

func looseDurationField(fields map[string]json.RawMessage, name string) (float64, error) {
	raw, ok := fields[name]
	if !ok || isJSONNull(raw) {
		return 0, nil
	}
	value, err := decodeLooseValue(raw)
	if err != nil {
		return 0, err
	}
	switch v := value.(type) {
	case json.Number:
		return v.Float64()
	case string:
		n, ok := firstFloatInString(v)
		if !ok {
			return 0, fmt.Errorf("不能把 %q 转为时长", v)
		}
		return n, nil
	case nil:
		return 0, nil
	default:
		return 0, fmt.Errorf("期望数字或字符串，实际是 %T", value)
	}
}

func looseRawArray(raw json.RawMessage) ([]json.RawMessage, error) {
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err == nil {
		return values, nil
	}
	text, err := looseStringField(map[string]json.RawMessage{"value": raw}, "value")
	if err != nil || strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("期望数组")
	}
	if err := json.Unmarshal([]byte(text), &values); err != nil {
		return nil, fmt.Errorf("期望数组")
	}
	return values, nil
}

func looseStringListValue(value any) ([]string, error) {
	switch v := value.(type) {
	case []any:
		out := []string{}
		for _, item := range v {
			text, err := looseStringValue(item)
			if err != nil {
				return nil, err
			}
			out = append(out, text)
		}
		return cleanStringList(out), nil
	case string:
		return splitLooseList(v), nil
	case json.Number, bool:
		text, err := looseStringValue(v)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return []string{text}, nil
	case nil:
		return nil, nil
	default:
		return nil, fmt.Errorf("期望数组或字符串，实际是 %T", value)
	}
}

func looseStringValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v), nil
	case json.Number:
		return strings.TrimSpace(v.String()), nil
	case bool:
		return strconv.FormatBool(v), nil
	case nil:
		return "", nil
	default:
		return "", fmt.Errorf("期望字符串，实际是 %T", value)
	}
}

func decodeLooseValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

func splitLooseList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if strings.HasPrefix(value, "[") {
		var nested []json.RawMessage
		if err := json.Unmarshal([]byte(value), &nested); err == nil {
			out := []string{}
			for _, raw := range nested {
				text, err := looseStringField(map[string]json.RawMessage{"value": raw}, "value")
				if err == nil {
					out = append(out, text)
				}
			}
			return cleanStringList(out)
		}
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case '\n', '\r', ',', '，', '、', ';', '；':
			return true
		default:
			return false
		}
	})
	return cleanStringList(parts)
}

func firstFloatInString(value string) (float64, bool) {
	start := -1
	end := -1
	for i, r := range value {
		if (r >= '0' && r <= '9') || r == '.' {
			if start < 0 {
				start = i
			}
			end = i + len(string(r))
			continue
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0, false
	}
	n, err := strconv.ParseFloat(value[start:end], 64)
	return n, err == nil
}

func stripThinkBlocks(value string) string {
	for {
		lower := strings.ToLower(value)
		start := strings.Index(lower, "<think>")
		if start < 0 {
			return value
		}
		end := strings.Index(lower[start:], "</think>")
		if end < 0 {
			return strings.TrimSpace(value[:start])
		}
		end += start + len("</think>")
		value = value[:start] + value[end:]
	}
}

func firstJSONObject(value string) (string, bool) {
	for start := 0; start < len(value); start++ {
		if value[start] != '{' {
			continue
		}
		if candidate, ok := balancedJSONObject(value[start:]); ok && json.Valid([]byte(candidate)) {
			return candidate, true
		}
	}
	return "", false
}

func balancedJSONObject(value string) (string, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch ch {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return value[:i+1], true
			}
			if depth < 0 {
				return "", false
			}
		}
	}
	return "", false
}

func previewText(value string) string {
	value = compact([]byte(value))
	if value == "" {
		return "空响应"
	}
	const maxPreview = 180
	if len(value) <= maxPreview {
		return value
	}
	return value[:maxPreview] + "..."
}

func cleanStringList(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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

func trimRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
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
