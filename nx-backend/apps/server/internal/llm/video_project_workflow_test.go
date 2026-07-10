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
