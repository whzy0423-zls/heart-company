package server

import (
	"context"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/profilecalibration"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/ragstore"
)

func TestServerDailyQuizQuestionGeneratorUsesKnowledgeContextAndModelConfig(t *testing.T) {
	store := &fakeDailyQuizRAGStore{docs: []rag.Document{{
		ID: "kb-1", Title: "九型核心动机", Content: "1号重视正确，2号重视被需要，3号重视成就。", Tags: []string{"九型人格", "画像校准"},
	}}}
	fakeClient := &fakeDailyQuizStructuredJSONClient{content: `{"questions":[{"body":"当计划被打乱时，你最自然的反应更接近哪一种？","dimension":"enneagram_profile_calibration","typeWeights":{"1":1},"options":[{"id":"A","label":"A","text":"先找出哪里不够正确","typeWeights":{"1":2}},{"id":"B","label":"B","text":"确认大家是否还需要我","typeWeights":{"2":2}},{"id":"C","label":"C","text":"快速调整为可展示成果","typeWeights":{"3":2}}]},{"body":"面对新的合作，你最在意什么？","dimension":"enneagram_profile_calibration","options":[{"id":"A","label":"A","text":"规则是否清晰","typeWeights":{"1":1}},{"id":"B","label":"B","text":"关系是否靠近","typeWeights":{"2":1}},{"id":"C","label":"C","text":"目标是否可衡量","typeWeights":{"3":1}}]}]}`}
	var capturedConfig modelconfig.CompatibleModelConfig
	gen := serverDailyQuizQuestionGenerator{
		server: &Server{ragDocs: store},
		readModelConfig: func(context.Context, any) (modelconfig.Config, bool, error) {
			return modelconfig.Config{DailyQuiz: modelconfig.CompatibleModelConfig{Provider: "openai-compatible", APIBase: "https://model.example.com/v1", APIKey: "secret", Model: "gpt-5.5-mini", TimeoutSeconds: 44}}, true, nil
		},
		newStructuredJSONClient: func(cfg modelconfig.CompatibleModelConfig) dailyQuizStructuredJSONClient {
			capturedConfig = cfg
			return fakeClient
		},
	}

	result, err := gen.GenerateDailyQuizQuestions(context.Background(), profilecalibration.DailyQuizGenerationInput{Date: "2026-07-09", Count: 2})
	if err != nil {
		t.Fatalf("GenerateDailyQuizQuestions returned error: %v", err)
	}

	if !store.enabledCalled {
		t.Fatal("expected generator to load enabled knowledge documents")
	}
	if capturedConfig.APIKey != "secret" || capturedConfig.Model != "gpt-5.5-mini" || capturedConfig.TimeoutSeconds != 44 {
		t.Fatalf("expected configured model passed to client without echoing elsewhere, got %+v", capturedConfig)
	}
	if !strings.Contains(fakeClient.request.UserPrompt, "九型核心动机") || !strings.Contains(fakeClient.request.UserPrompt, "2026-07-09") {
		t.Fatalf("expected prompt to include knowledge context and date, got %s", fakeClient.request.UserPrompt)
	}
	if len(result.Questions) != 2 {
		t.Fatalf("expected 2 generated questions, got %d", len(result.Questions))
	}
	if result.ModelProvider != "openai-compatible" || result.ModelName != "gpt-5.5-mini" || result.Source != "ai" {
		t.Fatalf("unexpected generation metadata: %+v", result)
	}
	if strings.Contains(result.Prompt, "secret") || strings.Contains(result.RawResponse, "secret") {
		t.Fatalf("generation result must not echo API key: %+v", result)
	}
}

type fakeDailyQuizStructuredJSONClient struct {
	request llm.StructuredJSONRequest
	content string
}

func (f *fakeDailyQuizStructuredJSONClient) GenerateJSON(_ context.Context, req llm.StructuredJSONRequest) (llm.StructuredJSONResult, error) {
	f.request = req
	return llm.StructuredJSONResult{Provider: "openai-compatible", Model: "gpt-5.5-mini", Content: f.content, RawResponse: `{"choices":[{"message":{"content":"ok"}}]}`}, nil
}

type fakeDailyQuizRAGStore struct {
	enabledCalled bool
	docs          []rag.Document
}

func (f *fakeDailyQuizRAGStore) EnabledDocuments(context.Context) ([]rag.Document, error) {
	f.enabledCalled = true
	return f.docs, nil
}

func (f *fakeDailyQuizRAGStore) DeleteDocument(context.Context, string) (bool, error) {
	return false, nil
}
func (f *fakeDailyQuizRAGStore) ListDocuments(context.Context, map[string]string) (ragstore.PageResult[ragstore.Document], error) {
	return ragstore.PageResult[ragstore.Document]{}, nil
}
func (f *fakeDailyQuizRAGStore) SaveDocument(context.Context, ragstore.Document) (ragstore.Document, error) {
	return ragstore.Document{}, nil
}
