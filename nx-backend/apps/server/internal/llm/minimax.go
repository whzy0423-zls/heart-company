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
	"net/url"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/netguard"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

const miniMaxMaxStreamEventBytes = 1024 * 1024

type MiniMaxGenerator struct {
	apiBase      string
	apiKey       string
	client       *http.Client
	groupID      string
	model        string
	systemPrompt string
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
		groupID:      strings.TrimSpace(cfg.GroupID),
		model:        model,
		systemPrompt: strings.TrimSpace(cfg.SystemPrompt),
	}
}

func (g *MiniMaxGenerator) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}

	body := map[string]any{
		"model":              g.model,
		"temperature":        0.55,
		"tokens_to_generate": chatTokenBudgetForTier(input.Question, input.Tier),
		"messages": []map[string]string{
			{"role": "system", "content": resolveRuntimeSystemPrompt(g.resolveSystemPrompt(), input)},
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

// GenerateStream requests MiniMax's SSE response and forwards each model delta
// as soon as it arrives. It intentionally does not buffer the successful
// response body so callers can flush deltas to their clients in real time.
func (g *MiniMaxGenerator) GenerateStream(ctx context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}

	body := map[string]any{
		"model":              g.model,
		"temperature":        0.55,
		"tokens_to_generate": chatTokenBudgetForTier(input.Question, input.Tier),
		"stream":             true,
		"messages": []map[string]string{
			{"role": "system", "content": resolveRuntimeSystemPrompt(g.resolveSystemPrompt(), input)},
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		return "", fmt.Errorf("MiniMax 请求失败(%d): %s", resp.StatusCode, compact(raw))
	}

	answer, err := consumeMiniMaxStream(ctx, resp.Body, emit)
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", fmt.Errorf("MiniMax 未返回文本回答")
	}
	return answer, nil
}

func consumeMiniMaxStream(ctx context.Context, body io.Reader, emit rag.StreamEmitter) (string, error) {
	reader := bufio.NewReaderSize(body, 32*1024)
	var answer strings.Builder
	var eventData strings.Builder
	eventType := ""
	eventBytes := 0

	dispatch := func() (bool, error) {
		data := strings.TrimSpace(eventData.String())
		defer func() {
			eventType = ""
			eventData.Reset()
			eventBytes = 0
		}()
		if data == "" {
			return false, nil
		}
		if data == "[DONE]" {
			return true, nil
		}

		if eventType == "error" {
			var payload map[string]any
			if err := json.Unmarshal([]byte(data), &payload); err == nil {
				return false, miniMaxStreamEventError(payload, data)
			}
			return false, fmt.Errorf("MiniMax 返回错误: %s", compact([]byte(data)))
		}

		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return false, fmt.Errorf("MiniMax 流响应解析失败: %w", err)
		}
		if err := miniMaxStreamPayloadError(payload); err != nil {
			return false, err
		}

		delta := findString(payload,
			"choices.0.delta.content",
			"choices.0.delta.text",
			"choices.0.delta",
			"delta.content",
			"delta.text",
			"delta",
			"data.choices.0.delta.content",
			"data.choices.0.delta",
		)
		if delta == "" {
			snapshot := findString(payload,
				"choices.0.messages.0.text",
				"choices.0.messages.0.content",
				"choices.0.message.content",
				"choices.0.message.text",
				"choices.0.text",
				"reply",
				"data.reply",
				"data.choices.0.messages.0.text",
				"data.choices.0.message.content",
			)
			accumulated := answer.String()
			switch {
			case snapshot == "":
				return false, nil
			case strings.HasPrefix(snapshot, accumulated):
				delta = strings.TrimPrefix(snapshot, accumulated)
			case strings.HasPrefix(accumulated, snapshot):
				return false, nil
			default:
				delta = snapshot
			}
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

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		line, readErr := readMiniMaxStreamLine(reader, miniMaxMaxStreamEventBytes-eventBytes+2)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			return "", readErr
		}
		if len(line) > 0 {
			line = bytes.TrimSuffix(line, []byte("\n"))
			line = bytes.TrimSuffix(line, []byte("\r"))
			if len(line) == 0 {
				done, err := dispatch()
				if err != nil {
					return "", err
				}
				if done {
					return answer.String(), nil
				}
			} else {
				eventBytes += len(line) + 1
				if eventBytes > miniMaxMaxStreamEventBytes {
					return "", fmt.Errorf("MiniMax 流事件过大（上限 %d 字节）", miniMaxMaxStreamEventBytes)
				}
				if line[0] == ':' {
					continue
				}
				field, value, found := bytes.Cut(line, []byte(":"))
				if !found {
					value = nil
				}
				value = bytes.TrimPrefix(value, []byte(" "))
				switch string(field) {
				case "event":
					eventType = string(value)
				case "data":
					if eventData.Len() > 0 {
						eventData.WriteByte('\n')
					}
					eventData.Write(value)
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			done, err := dispatch()
			if err != nil {
				return "", err
			}
			_ = done
			return answer.String(), nil
		}
	}
}

func readMiniMaxStreamLine(reader *bufio.Reader, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, fmt.Errorf("MiniMax 流事件过大（上限 %d 字节）", miniMaxMaxStreamEventBytes)
	}
	line := make([]byte, 0, min(maxBytes, 32*1024))
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > maxBytes {
			return nil, fmt.Errorf("MiniMax 流事件过大（上限 %d 字节）", miniMaxMaxStreamEventBytes)
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}

func miniMaxStreamPayloadError(payload map[string]any) error {
	if err := baseRespError(payload); err != nil {
		return err
	}
	errorValue, ok := payload["error"]
	if !ok || errorValue == nil {
		return nil
	}
	switch value := errorValue.(type) {
	case string:
		if strings.TrimSpace(value) != "" {
			return fmt.Errorf("MiniMax 返回错误: %s", strings.TrimSpace(value))
		}
	case map[string]any:
		message := findString(value, "message", "msg", "status_msg")
		if strings.TrimSpace(message) != "" {
			return fmt.Errorf("MiniMax 返回错误: %s", strings.TrimSpace(message))
		}
	}
	return fmt.Errorf("MiniMax 返回错误")
}

func miniMaxStreamEventError(payload map[string]any, raw string) error {
	if err := miniMaxStreamPayloadError(payload); err != nil {
		return err
	}
	message := findString(payload, "message", "msg", "status_msg")
	if strings.TrimSpace(message) == "" {
		message = compact([]byte(raw))
	}
	if message == "" {
		message = "MiniMax 流返回错误事件"
	}
	return fmt.Errorf("MiniMax 返回错误: %s", message)
}

func (g *MiniMaxGenerator) SummarizeConversation(ctx context.Context, previousSummary string, messages []rag.Message) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}
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

	body := map[string]any{
		"model":              g.model,
		"temperature":        0.2,
		"tokens_to_generate": 700,
		"messages": []map[string]string{
			{"role": "system", "content": "你负责压缩会话摘要。必须保留参与人物及关系、已确认事实和事件、用户诉求与边界、关键建议与反馈、尚未解决的问题；删除寒暄、重复表达和无关细节。摘要应准确、简洁，不得添加对话中不存在的信息。"},
			{"role": "user", "content": prompt.String()},
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
		return "", fmt.Errorf("会话摘要生成失败(%d): %s", resp.StatusCode, compact(raw))
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if err := baseRespError(result); err != nil {
		return "", err
	}
	summary := strings.TrimSpace(findString(result,
		"choices.0.message.content",
		"choices.0.text",
		"reply",
		"data.reply",
		"data.choices.0.message.content",
	))
	if summary == "" {
		return "", fmt.Errorf("会话摘要未返回内容")
	}
	return trimRunes(summary, 1200), nil
}

// PolishPrompt 把用户给出的方向或草稿润色成一段高质量的文生图/文生视频提示词。
// kind 取值："image"（文生图）或 "video"（文生视频），用于切换润色侧重点。
// 复用对话模型（MiniMax），但使用独立的系统提示词，与成长教练人设解耦。
func (g *MiniMaxGenerator) PolishPrompt(ctx context.Context, draft, kind string) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("请先配置 MINIMAX_API_KEY")
	}
	draft = strings.TrimSpace(draft)
	if draft == "" {
		return "", fmt.Errorf("请先填写提示词方向或草稿")
	}

	body := map[string]any{
		"model":              g.model,
		"temperature":        0.7,
		"tokens_to_generate": 600,
		"messages": []map[string]string{
			{"role": "system", "content": polishSystemPrompt(kind)},
			{"role": "user", "content": "请润色以下" + polishKindLabel(kind) + "提示词方向，只输出润色后的提示词正文：\n" + draft},
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
		return "", fmt.Errorf("MiniMax 未返回润色结果")
	}
	return strings.TrimSpace(answer), nil
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

// resolveSystemPrompt 返回最终系统提示词：内置规则始终生效，后台配置仅作为补充设定。
func (g *MiniMaxGenerator) resolveSystemPrompt() string {
	return appendChatCustomSystemPrompt(defaultSystemPrompt, g.systemPrompt)
}

// defaultSystemPrompt 内置默认提示词。
// 与旧版的区别：不再"只基于检索资料"，资料不足时允许结合九型人格常识温和作答，
// 这样检索未命中也能给出有帮助的回答，而不是退回固定兜底。
const defaultSystemPrompt = "你是九型人格成长陪伴里的成长教练，像一个懂用户的朋友，自然、有温度、少说教。默认使用中文回答，不要夹杂英文；数字和九型编号用中文自然表达，便于语音播报。请优先结合给定的高相关检索资料和用户档案回答；当资料不足或没有资料时，也可以基于稳妥的通用知识继续作答，不要生硬拒绝。菜谱、生活知识等普通问题应直接使用通用知识回答，不要强行关联九型人格，也不要默认让用户自行上网搜索；只有实时价格、营业状态等确实需要外部核实的信息，才提醒用户核实。不做医疗或心理诊断。回答要具体、先给重点、适合手机阅读：未指定模式时，普通问题通常只回答 1-3 句，完整分句分别换行，重点、建议、步骤等需要区分时使用简短中文标签；只有用户明确要求展开、详细说明或完整分析时才使用简短段落展开；请求里有明确对话模式时，以该模式规则为准。不主动使用亲爱的、宝贝等亲昵称呼。不要主动描述运行载体、页面、后台服务、接口链路、模型配置、实现细节或内部链路；只直接回答用户问题。用户要求纠正时立即按新要求重答，不要解释为什么要纠正，也不要维护之前的回答。不要机械复述用户的话，不要固定总结，不要固定给建议；只有确有必要时，最多追问一个真正有用的问题。"

const fixedConciseReplyInstruction = "基础问答模式：请直接回答当前问题；有高相关检索资料时可参考，没有时使用通用知识。通常用 1～3 句话直接回答，不要长篇展开，不使用“亲爱的”等亲昵称呼。"

const deepReplyInstruction = "深度分析模式：请按“核心判断、原因与模式、影响、具体行动”的顺序深入展开。结合高相关资料与人物画像，但不要为了套用九型而偏离用户问题；内容要清晰、具体、适合手机阅读。"

const companionReplyInstruction = "专业陪伴模式：先回应用户的情绪和处境，再温和梳理正在发生的事情与可尝试的下一步。不要说教，不要急于贴九型标签；只有确有帮助时，最多追问一个问题。"

const allTypesReplyInstruction = "请完整回答当前问题，按1号到9号的顺序逐一回答，不能遗漏或只围绕用户自己的主型。每个型号都要紧扣用户当前主题给出特点和具体应用；如果问题涉及孩子，每个型号必须包含：孩子的典型特点、家长如何理解和沟通、一个具体应用方法。检索资料只作参考，不能因为检索资料不完整而遗漏任何型号；缺少的部分可基于稳妥的九型人格通用常识补充。使用清晰、适合手机阅读的编号列表，不使用“亲爱的”等亲昵称呼。"

func chatTokenBudget(question string) int {
	question = strings.TrimSpace(question)
	if requestsAllEnneagramTypes(question) {
		return 1200
	}
	for _, marker := range []string{"详细", "展开", "完整分析", "深入分析", "逐步说明"} {
		if strings.Contains(question, marker) {
			return 420
		}
	}
	return 220
}

func chatTokenBudgetForTier(question, tier string) int {
	if requestsAllEnneagramTypes(question) {
		return 1200
	}
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "basic":
		return 220
	case "companion":
		return 500
	case "deep":
		return 700
	default:
		return chatTokenBudget(question)
	}
}

func requestsAllEnneagramTypes(question string) bool {
	normalized := strings.NewReplacer(
		" ", "",
		"－", "-",
		"—", "-",
		"–", "-",
		"～", "-",
		"~", "-",
	).Replace(strings.TrimSpace(question))
	for _, marker := range []string{
		"1-9",
		"1到9",
		"1至9",
		"九种类型",
		"九个类型",
		"所有型号",
		"全部型号",
		"每个型号",
		"逐个型号",
		"所有类型",
		"全部类型",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
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
	card := input.ConversationCard
	if card.Name != "" || card.Relation != "" || card.MainType > 0 || card.WingType > 0 || card.Profile != "" {
		b.WriteString("当前关注对象：")
		if card.Name != "" {
			b.WriteString("称呼=" + strings.TrimSpace(card.Name) + "；")
		}
		if card.Relation != "" {
			b.WriteString("与用户关系=" + strings.TrimSpace(card.Relation) + "；")
		}
		if card.MainType > 0 {
			b.WriteString(fmt.Sprintf("主型=%d号；", card.MainType))
		}
		if card.WingType > 0 {
			b.WriteString(fmt.Sprintf("翼型=%d号；", card.WingType))
		}
		if profile := strings.TrimSpace(card.Profile); profile != "" {
			b.WriteString("画像=" + trimRunes(profile, 800) + "；")
		}
		b.WriteString("\n")
		if strings.EqualFold(strings.TrimSpace(card.CardType), "secondary") {
			b.WriteString("当前关注对象是用户正在咨询的 TA，不要把当前关注对象当成正在输入的用户本人，也不要冒充当前关注对象；请围绕用户与 TA 的关系提供分析和建议。\n")
		}
	}
	if len(input.UserProfile.Memories) > 0 {
		b.WriteString("近期记忆：\n")
		for i, memory := range input.UserProfile.Memories {
			memory = strings.TrimSpace(memory)
			if memory == "" {
				continue
			}
			if i >= 6 {
				break
			}
			b.WriteString(fmt.Sprintf("%d. %s\n", i+1, trimRunes(memory, 160)))
		}
	}
	if len(input.UserPreferences) > 0 {
		b.WriteString("已保存的交流偏好：\n")
		for _, preference := range input.UserPreferences {
			preference = strings.TrimSpace(preference)
			if preference == "" {
				continue
			}
			b.WriteString("- " + trimRunes(preference, 160) + "\n")
		}
	}
	if summary := strings.TrimSpace(input.ConversationSummary); summary != "" {
		b.WriteString("会话前情：\n")
		b.WriteString(trimRunes(summary, 1200) + "\n")
	}
	if len(input.History) > 0 {
		b.WriteString("最近对话：\n")
		for _, item := range input.History {
			b.WriteString(item.Role + ": " + item.Content + "\n")
		}
	}
	b.WriteString("用户问题：" + input.Question + "\n")
	if len(input.CurrentDirectives) > 0 {
		b.WriteString("当前消息优先规则（与旧偏好或历史冲突时，以当前规则为准；不能要求改用英文，默认中文规则仍然生效，只有当前用户明确要求英文或翻译时才输出必要英文）：\n")
		for _, directive := range input.CurrentDirectives {
			directive = strings.TrimSpace(directive)
			if directive == "" {
				continue
			}
			b.WriteString("- " + trimRunes(directive, 160) + "\n")
		}
	}
	b.WriteString("检索资料：\n")
	if len(input.Sources) == 0 {
		b.WriteString("暂无高相关资料。\n")
	} else {
		for i, source := range input.Sources {
			b.WriteString(fmt.Sprintf("%d. %s：%s\n", i+1, source.Title, source.Snippet))
		}
	}
	if requestsAllEnneagramTypes(input.Question) {
		b.WriteString(allTypesReplyInstruction)
	} else {
		switch strings.ToLower(strings.TrimSpace(input.Tier)) {
		case "deep":
			b.WriteString(deepReplyInstruction)
		case "companion":
			b.WriteString(companionReplyInstruction)
		default:
			b.WriteString(fixedConciseReplyInstruction)
		}
	}
	return b.String()
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
