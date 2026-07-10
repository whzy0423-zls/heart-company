package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestPolishVideoProjectScriptParsesFencedJSONAndUsesDedicatedPrompt(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/text/chatcompletion_v2" {
			t.Fatalf("expected existing conversation intermediary endpoint, got %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": `<think>先整理三幕结构。</think>
` + "```json" + `
{"content":"第一场：雨夜车站。阿宁找到旧相机。","summary":"阿宁借旧相机寻找记忆。","style":"温暖写实、轻悬疑"}
` + "```"},
			}},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	result, err := generator.PolishVideoProjectScript(context.Background(), "一个女孩在车站找到旧相机")
	if err != nil {
		t.Fatalf("PolishVideoProjectScript returned error: %v", err)
	}
	if result.Content != "第一场：雨夜车站。阿宁找到旧相机。" || result.Summary != "阿宁借旧相机寻找记忆。" || result.Style != "温暖写实、轻悬疑" {
		t.Fatalf("unexpected parsed script result: %+v", result)
	}
	if !strings.Contains(result.RawResult, "<think>") || !strings.Contains(result.RawResult, `"content"`) {
		t.Fatalf("expected original model answer to be preserved, got %q", result.RawResult)
	}

	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected system and user messages, got %+v", requestBody["messages"])
	}
	systemMessage, _ := messages[0].(map[string]any)
	systemPrompt, _ := systemMessage["content"].(string)
	if !strings.Contains(systemPrompt, "视频编剧") || !strings.Contains(systemPrompt, "Seedance 2.0") {
		t.Fatalf("expected dedicated video script prompt, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "九型人格成长陪伴") {
		t.Fatalf("video script request must not reuse the growth-coach persona: %q", systemPrompt)
	}
	userMessage, _ := messages[1].(map[string]any)
	if !strings.Contains(userMessage["content"].(string), "一个女孩在车站找到旧相机") {
		t.Fatalf("expected draft in user prompt, got %+v", userMessage)
	}
}

func TestPolishVideoProjectScriptFallsBackToPlainTextAfterThinkBlock(t *testing.T) {
	rawAnswer := "<think>用户给的是一句创意，我需要补成可拍摄剧本。</think>\n第一场：清晨厨房。\n妈妈把早餐放到桌上。"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": rawAnswer},
			}},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	result, err := generator.PolishVideoProjectScript(context.Background(), "妈妈准备早餐")
	if err != nil {
		t.Fatalf("PolishVideoProjectScript returned error: %v", err)
	}
	if result.Content != "第一场：清晨厨房。\n妈妈把早餐放到桌上。" {
		t.Fatalf("expected plain text fallback without thinking, got %q", result.Content)
	}
	if result.Summary != "" || result.Style != "" {
		t.Fatalf("plain text fallback should leave optional fields empty, got %+v", result)
	}
	if result.RawResult != rawAnswer {
		t.Fatalf("expected unmodified raw answer, got %q", result.RawResult)
	}
}

func TestPolishVideoProjectScriptRejectsEmptyDraft(t *testing.T) {
	generator := NewMiniMaxGenerator(config.MiniMaxConfig{APIKey: "test-key"})
	_, err := generator.PolishVideoProjectScript(context.Background(), "  ")
	if err == nil || !strings.Contains(err.Error(), "剧本") {
		t.Fatalf("expected helpful empty script error, got %v", err)
	}
}

func TestProjectBreakdownParsesTypeDriftAndNormalizesStableDistinctKeys(t *testing.T) {
	answer := `<think>先识别人物、场景、物品、服饰和风格。</think>
` + "```json" + `
{
  "characters": "[{\"key\":\"hero\",\"name\":\"阿宁\",\"description\":\"成年阿宁\",\"visualPrompt\":\"二十五岁短发女性\",\"required\":\"true\"},{\"key\":\"hero\",\"name\":\"阿宁\",\"description\":\"童年阿宁\"}]",
  "scenes": [{"name":"雨夜车站"}],
  "props": [{"name":"旧相机","usageNote":"推动剧情"}],
  "outfits": null,
  "styles": [{"name":"温暖写实","required":true}],
  "storyBeats": [{"title":"发现相机","description":"阿宁捡到相机","sceneKeys":"雨夜车站","assetKeys":"旧相机、阿宁"}]
}
` + "```"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": answer},
			}},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	first, raw, err := generator.BreakdownVideoProjectScript(context.Background(), "第一场：阿宁在雨夜车站捡到旧相机。")
	if err != nil {
		t.Fatalf("BreakdownVideoProjectScript returned error: %v", err)
	}
	second, _, err := generator.BreakdownVideoProjectScript(context.Background(), "第一场：阿宁在雨夜车站捡到旧相机。")
	if err != nil {
		t.Fatalf("second BreakdownVideoProjectScript returned error: %v", err)
	}
	if raw != answer {
		t.Fatalf("expected raw model output to be preserved, got %q", raw)
	}
	if len(first.Characters) != 2 || first.Characters[0].Name != "阿宁" || first.Characters[1].Name != "阿宁" {
		t.Fatalf("duplicate names must stay as two items, got %+v", first.Characters)
	}
	if first.Characters[0].Key == "" || first.Characters[1].Key == "" || first.Characters[0].Key == first.Characters[1].Key {
		t.Fatalf("expected stable distinct character keys, got %+v", first.Characters)
	}
	if first.Characters[0].Key != second.Characters[0].Key || first.Characters[1].Key != second.Characters[1].Key {
		t.Fatalf("repeated parsing must return identical keys, first=%+v second=%+v", first.Characters, second.Characters)
	}
	if !first.Characters[0].Required || first.Characters[0].Decision != "pending" || first.Characters[1].Decision != "pending" {
		t.Fatalf("expected required coercion and pending defaults, got %+v", first.Characters)
	}
	if len(first.Scenes) != 1 || first.Scenes[0].Description != "" || first.Scenes[0].Decision != "pending" {
		t.Fatalf("expected missing optional scene fields to be accepted, got %+v", first.Scenes)
	}
	if first.Outfits == nil || len(first.Outfits) != 0 {
		t.Fatalf("expected omitted outfits to normalize to an empty array, got %#v", first.Outfits)
	}
	if len(first.StoryBeats) != 1 || len(first.StoryBeats[0].SceneKeys) != 1 || len(first.StoryBeats[0].AssetKeys) != 2 {
		t.Fatalf("expected story beat string lists to normalize, got %+v", first.StoryBeats)
	}
}

func TestProjectBreakdownReturnsRawOutputWhenParsingFails(t *testing.T) {
	answer := "我识别到一个人物和一个场景，但这次没有按 JSON 输出。"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message": map[string]any{"content": answer},
			}},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	_, raw, err := generator.BreakdownVideoProjectScript(context.Background(), "阿宁走进车站。")
	if err == nil || !strings.Contains(err.Error(), "有效 JSON") {
		t.Fatalf("expected helpful JSON parse error, got %v", err)
	}
	if raw != answer {
		t.Fatalf("expected failed model output to be recoverable, got %q", raw)
	}
}
