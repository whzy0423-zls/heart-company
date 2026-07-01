package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestMiniMaxGeneratorSendsRAGContext(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header: %s", r.Header.Get("Authorization"))
		}
		if !strings.Contains(r.URL.RawQuery, "GroupId=test-group") {
			t.Fatalf("missing group id in query: %s", r.URL.RawQuery)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": "模型回答"},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewMiniMaxGenerator(config.MiniMaxConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		GroupID: "test-group",
	})
	answer, err := generator.Generate(context.Background(), rag.GenerateInput{
		Question: "完美型怎么成长？",
		UserProfile: rag.UserProfile{
			Nickname: "小九",
			MainType: 1,
		},
		Sources: []rag.Source{{Title: "1号 完美型", Snippet: "允许不完美"}},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if answer != "模型回答" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	messages, _ := requestBody["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("expected messages in request: %+v", requestBody)
	}
	body, _ := json.Marshal(requestBody)
	if !strings.Contains(string(body), "允许不完美") || !strings.Contains(string(body), "完美型怎么成长") {
		t.Fatalf("request did not include rag context/question: %s", string(body))
	}
}

func TestMiniMaxGeneratorRequiresAPIKey(t *testing.T) {
	_, err := NewMiniMaxGenerator(config.MiniMaxConfig{}).Generate(context.Background(), rag.GenerateInput{Question: "hi"})
	if err == nil {
		t.Fatal("expected missing api key error")
	}
}

func TestMiniMaxGeneratorUsesConfiguredTimeout(t *testing.T) {
	generator := NewMiniMaxGenerator(config.MiniMaxConfig{TimeoutSeconds: 12})
	if generator.client.Timeout.String() != "12s" {
		t.Fatalf("expected configured timeout, got %s", generator.client.Timeout)
	}

	defaultGenerator := NewMiniMaxGenerator(config.MiniMaxConfig{})
	if defaultGenerator.client.Timeout.String() != "25s" {
		t.Fatalf("expected 25s default timeout, got %s", defaultGenerator.client.Timeout)
	}
}

func TestAnalyzeVideoUsesOpenAICompatibleEndpointForProxyBase(t *testing.T) {
	var gotPath string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"scenes":["室内"],"characters":["女性"],"assets":["近景"],"seedancePrompt":"室内女性近景，柔和光线"}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewMiniMaxGenerator(config.MiniMaxConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		Model:   "gpt-5.5",
	})
	result, err := generator.AnalyzeVideo(context.Background(), "https://example.com/video.mp4", "demo.mp4")
	if err != nil {
		t.Fatalf("AnalyzeVideo returned error: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected OpenAI-compatible endpoint, got %s", gotPath)
	}
	if requestBody["max_tokens"] == nil {
		t.Fatalf("expected max_tokens in OpenAI-compatible request: %+v", requestBody)
	}
	if result.SeedancePrompt == "" {
		t.Fatalf("expected parsed seedance prompt: %+v", result)
	}
}

func TestAnalyzeVideoSendsVideoURLContentForMiniMaxM3(t *testing.T) {
	var gotPath string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"scenes":["室外"],"characters":["少年"],"assets":["航拍"],"seedancePrompt":"室外少年奔跑，航拍镜头"}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewMiniMaxGenerator(config.MiniMaxConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		GroupID: "voice-group",
		Model:   "MiniMax-M3",
	})
	result, err := generator.AnalyzeVideo(context.Background(), "https://example.com/video.mp4", "demo.mp4")
	if err != nil {
		t.Fatalf("AnalyzeVideo returned error: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected MiniMax-M3 to use chat completions endpoint, got %s", gotPath)
	}
	if requestBody["model"] != "MiniMax-M3" {
		t.Fatalf("expected MiniMax-M3 model in request, got %+v", requestBody["model"])
	}
	if requestBody["max_completion_tokens"] != float64(1200) {
		t.Fatalf("expected max_completion_tokens for MiniMax-M3 request, got %+v", requestBody)
	}
	if _, ok := requestBody["max_tokens"]; ok {
		t.Fatalf("expected MiniMax-M3 request to avoid deprecated max_tokens, got %+v", requestBody)
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("expected MiniMax-M3 thinking to be disabled for JSON analysis, got %+v", requestBody["thinking"])
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected two messages, got %+v", requestBody["messages"])
	}
	userMessage, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("expected user message object, got %+v", messages[1])
	}
	content, ok := userMessage["content"].([]any)
	if !ok || len(content) < 2 {
		t.Fatalf("expected multimodal user content, got %+v", userMessage["content"])
	}
	var foundVideoURL bool
	for _, part := range content {
		partMap, ok := part.(map[string]any)
		if !ok || partMap["type"] != "video_url" {
			continue
		}
		videoURL, _ := partMap["video_url"].(map[string]any)
		if videoURL["url"] == "https://example.com/video.mp4" {
			foundVideoURL = true
		}
	}
	if !foundVideoURL {
		body, _ := json.Marshal(requestBody)
		t.Fatalf("expected request to contain video_url content part, got %s", string(body))
	}
	if result.SeedancePrompt == "" {
		t.Fatalf("expected parsed seedance prompt: %+v", result)
	}
}

func TestAnalyzeVideoReturnsHelpfulErrorForNonJSONModelAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": "<html><body>403 Forbidden</body></html>"},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewMiniMaxGenerator(config.MiniMaxConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		Model:   "MiniMax-M3",
	})
	_, err := generator.AnalyzeVideo(context.Background(), "https://example.com/video.mp4", "demo.mp4")
	if err == nil {
		t.Fatal("expected AnalyzeVideo to reject non-JSON model answer")
	}
	if !strings.Contains(err.Error(), "视频分析模型未返回有效 JSON") {
		t.Fatalf("expected helpful non-JSON error, got %v", err)
	}
	if strings.Contains(err.Error(), "invalid character '<'") {
		t.Fatalf("expected implementation detail to be hidden, got %v", err)
	}
}

func TestParseVideoAnalysisIgnoresMiniMaxThinkingBlock(t *testing.T) {
	result, err := parseVideoAnalysis(`<think>
我需要按 JSON 输出，草稿结构类似 {"scenes": []}。
</think>
` + "```json" + `
{
  "scenes": ["室外街道"],
  "characters": ["行人"],
  "assets": ["手持镜头"],
  "seedancePrompt": "室外街道里行人经过，手持跟拍，真实自然光"
}
` + "```")
	if err != nil {
		t.Fatalf("parseVideoAnalysis returned error: %v", err)
	}
	if result.SeedancePrompt == "" || len(result.Scenes) != 1 {
		t.Fatalf("expected parsed JSON after thinking block, got %+v", result)
	}
}

func TestParseVideoAnalysisIncludesSpeechInsights(t *testing.T) {
	result, err := parseVideoAnalysis(`{
  "scenes": ["访谈室"],
  "characters": ["主持人"],
  "assets": ["固定机位"],
  "hasSpeech": true,
  "audioSummary": "视频中主要讨论自我认知和行动计划。",
  "speechTopics": ["自我认知", "行动计划"],
  "speechKeywords": ["目标", "复盘"],
  "speechOutline": ["介绍问题背景", "提出行动建议"],
  "seedancePrompt": "访谈室里主持人讲述自我认知主题，固定机位，柔和光线"
}`)
	if err != nil {
		t.Fatalf("parseVideoAnalysis returned error: %v", err)
	}
	if !result.HasSpeech {
		t.Fatal("expected hasSpeech to be true")
	}
	if result.AudioSummary != "视频中主要讨论自我认知和行动计划。" {
		t.Fatalf("unexpected audio summary: %q", result.AudioSummary)
	}
	if len(result.SpeechTopics) != 2 || result.SpeechTopics[0] != "自我认知" {
		t.Fatalf("unexpected speech topics: %+v", result.SpeechTopics)
	}
	if len(result.SpeechKeywords) != 2 || result.SpeechKeywords[1] != "复盘" {
		t.Fatalf("unexpected speech keywords: %+v", result.SpeechKeywords)
	}
	if len(result.SpeechOutline) != 2 || result.SpeechOutline[0] != "介绍问题背景" {
		t.Fatalf("unexpected speech outline: %+v", result.SpeechOutline)
	}
}

func TestParseVideoAnalysisCoercesCommonLLMTypeDrift(t *testing.T) {
	result, err := parseVideoAnalysis(`{
  "scenes": "室内窗边\n书桌特写",
  "characters": "女性主角、旁白",
  "assets": "窗光",
  "hasSpeech": "true",
  "audioSummary": "一段关于自我接纳的旁白。",
  "speechTopics": "自我接纳, 行动计划",
  "speechKeywords": "觉察",
  "speechOutline": "提出困惑\n给出建议",
  "seedancePrompt": "室内窗边女性沉思，柔和窗光，近景推入"
}`)
	if err != nil {
		t.Fatalf("parseVideoAnalysis returned error: %v", err)
	}
	if !result.HasSpeech {
		t.Fatal("expected string true to parse as bool")
	}
	if len(result.Scenes) != 2 || result.Scenes[0] != "室内窗边" {
		t.Fatalf("expected string scenes to become list, got %+v", result.Scenes)
	}
	if len(result.Characters) != 2 || result.Characters[1] != "旁白" {
		t.Fatalf("expected comma-separated characters to become list, got %+v", result.Characters)
	}
	if len(result.SpeechTopics) != 2 || result.SpeechTopics[1] != "行动计划" {
		t.Fatalf("expected string speech topics to become list, got %+v", result.SpeechTopics)
	}
}

func TestParseVideoStoryboardDesignIgnoresThinkingBlock(t *testing.T) {
	result, err := parseVideoStoryboardDesign(`<think>
先根据视频解析和主题规划三段式节奏。
</think>
` + "```json" + `
{
  "title": "九型课程开场分镜",
  "styleGuide": ["温暖自然光", "真实纪实质感"],
  "globalPrompt": "围绕自我认知主题的 Seedance 2.0 短片，节奏舒缓",
  "shots": [
    {
      "index": 1,
      "duration": 3,
      "scene": "清晨教室",
      "characters": ["讲师"],
      "assets": ["白板", "柔和窗光"],
      "action": "讲师看向镜头微笑",
      "camera": "中景缓慢推进",
      "lighting": "暖色自然光",
      "audio": "轻柔环境音乐",
      "seedancePrompt": "清晨教室中讲师看向镜头微笑，中景缓慢推进，暖色自然光，真实纪实质感"
    }
  ]
}
` + "```")
	if err != nil {
		t.Fatalf("parseVideoStoryboardDesign returned error: %v", err)
	}
	if result.Title != "九型课程开场分镜" || len(result.Shots) != 1 {
		t.Fatalf("unexpected storyboard result: %+v", result)
	}
	if result.Shots[0].SeedancePrompt == "" || result.Shots[0].Characters[0] != "讲师" {
		t.Fatalf("expected parsed shot prompt and characters, got %+v", result.Shots[0])
	}
}

func TestParseVideoStoryboardDesignCoercesCommonLLMTypeDrift(t *testing.T) {
	result, err := parseVideoStoryboardDesign(`{
  "title": "疗愈主题分镜",
  "styleGuide": "柔和光\n真实纪实",
  "globalPrompt": "疗愈短片",
  "shots": [
    {
      "index": "1",
      "title": "开场",
      "duration": "3秒",
      "scene": "室内",
      "characters": "女性、书本",
      "assets": "窗光, 绿植",
      "action": "低头翻书",
      "camera": "近景推入",
      "composition": "人物在画面右侧",
      "lighting": "柔和光",
      "audio": "轻音乐",
      "dialogue": false,
      "seedancePrompt": "室内女性低头翻书，近景推入，柔和光"
    },
    {
      "index": 2,
      "duration": "4s",
      "scene": "书桌",
      "characters": ["女性"],
      "assets": ["书本"],
      "action": "抬头微笑",
      "camera": "固定中景",
      "lighting": "自然光",
      "audio": true,
      "dialogue": "今天开始接纳自己",
      "seedancePrompt": "书桌旁女性抬头微笑，固定中景，自然光"
    }
  ]
}`)
	if err != nil {
		t.Fatalf("parseVideoStoryboardDesign returned error: %v", err)
	}
	if len(result.StyleGuide) != 2 || result.StyleGuide[1] != "真实纪实" {
		t.Fatalf("expected string styleGuide to become list, got %+v", result.StyleGuide)
	}
	if len(result.Shots) != 2 {
		t.Fatalf("expected two shots, got %+v", result.Shots)
	}
	if result.Shots[0].Duration != 3 || result.Shots[1].Duration != 4 {
		t.Fatalf("expected string durations to parse, got %+v and %+v", result.Shots[0].Duration, result.Shots[1].Duration)
	}
	if len(result.Shots[0].Characters) != 2 || result.Shots[0].Characters[1] != "书本" {
		t.Fatalf("expected string characters to become list, got %+v", result.Shots[0].Characters)
	}
	if result.Shots[0].Dialogue != "false" || result.Shots[1].Audio != "true" {
		t.Fatalf("expected bool string fields to be preserved textually, got shot1 dialogue=%q shot2 audio=%q", result.Shots[0].Dialogue, result.Shots[1].Audio)
	}
}

func TestGenerateVideoStoryboardUsesOpenAICompatibleJSONRequest(t *testing.T) {
	var gotPath string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"title":"疗愈主题分镜","styleGuide":["柔和光"],"globalPrompt":"疗愈短片","shots":[{"index":1,"duration":4,"scene":"室内","characters":["女性"],"assets":["窗光"],"action":"低头翻书","camera":"近景推入","lighting":"柔和光","audio":"轻音乐","seedancePrompt":"室内女性低头翻书，近景推入，柔和光"}]}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewMiniMaxGenerator(config.MiniMaxConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
		Model:   "MiniMax-M3",
	})
	result, err := generator.GenerateVideoStoryboard(context.Background(), VideoStoryboardInput{
		Assets:         []string{"窗光"},
		Characters:     []string{"女性"},
		Scenes:         []string{"室内"},
		SeedancePrompt: "室内女性近景，柔和光线",
		Theme:          "疗愈感品牌宣传",
		VideoName:      "demo.mp4",
	})
	if err != nil {
		t.Fatalf("GenerateVideoStoryboard returned error: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected OpenAI-compatible endpoint, got %s", gotPath)
	}
	if requestBody["max_completion_tokens"] != float64(1800) {
		t.Fatalf("expected max_completion_tokens for storyboard request, got %+v", requestBody)
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("expected thinking disabled, got %+v", requestBody["thinking"])
	}
	if result.Title == "" || len(result.Shots) != 1 {
		t.Fatalf("expected parsed storyboard result, got %+v", result)
	}
}
