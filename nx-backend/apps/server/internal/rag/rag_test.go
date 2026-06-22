package rag

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestAskRetrievesRelevantKnowledge(t *testing.T) {
	service := NewService([]Document{
		{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则、秩序和高标准，成长建议是允许不完美。", Tags: []string{"完美型", "原则"}},
		{ID: "course-basic", Title: "九型基础课", Content: "九型基础课适合想系统理解九种性格模式的人。", Tags: []string{"课程"}},
	})

	result, err := service.Ask(context.Background(), AskInput{
		Question: "我是完美型，怎么成长？",
		UserProfile: UserProfile{
			Nickname: "小九",
			MainType: 1,
		},
	})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if !strings.Contains(result.Answer, "小九") || !strings.Contains(result.Answer, "完美型") {
		t.Fatalf("expected personalized answer, got %q", result.Answer)
	}
	if len(result.Sources) == 0 || result.Sources[0].ID != "type-1" {
		t.Fatalf("expected type-1 as top source, got %+v", result.Sources)
	}
}

func TestAskRejectsEmptyQuestion(t *testing.T) {
	_, err := NewService(nil).Ask(context.Background(), AskInput{Question: "   "})
	if err == nil {
		t.Fatal("expected error for empty question")
	}
}

func TestAskMatchesChineseKeywordsInsideLongQuestion(t *testing.T) {
	service := NewService([]Document{
		{ID: "kb-enterprise", Title: "企业沟通课", Content: "企业沟通课适合团队冲突复盘和管理者沟通训练。", Tags: []string{"企业", "沟通"}},
	})

	result, err := service.Ask(context.Background(), AskInput{Question: "企业沟通课适合什么场景？"})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if len(result.Sources) == 0 || result.Sources[0].ID != "kb-enterprise" {
		t.Fatalf("expected Chinese keyword match, got %+v", result.Sources)
	}
}

func TestAskUsesGeneratorWithRetrievedContext(t *testing.T) {
	generator := &fakeGenerator{answer: "这是模型生成的回答"}
	service := NewService([]Document{
		{ID: "type-5", Title: "5号 观察型", Content: "观察型重视知识、边界和独处空间。", Tags: []string{"观察型"}},
	}, WithGenerator(generator))

	result, err := service.Ask(context.Background(), AskInput{
		Question:    "观察型怎么沟通？",
		UserProfile: UserProfile{Nickname: "阿九", MainType: 5},
		History: []Message{
			{Role: "user", Content: "我刚测完"},
			{Role: "assistant", Content: "你可以问具体场景"},
		},
	})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if result.Answer != "这是模型生成的回答" {
		t.Fatalf("expected generator answer, got %q", result.Answer)
	}
	if generator.input.Question != "观察型怎么沟通？" || len(generator.input.Sources) != 1 {
		t.Fatalf("generator did not receive retrieved context: %+v", generator.input)
	}
	if len(generator.input.History) != 2 {
		t.Fatalf("expected history to be passed, got %+v", generator.input.History)
	}
}

func TestAskLimitsGeneratorHistory(t *testing.T) {
	generator := &fakeGenerator{answer: "模型回答"}
	service := NewService([]Document{
		{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则。", Tags: []string{"完美型"}},
	}, WithGenerator(generator))

	history := []Message{
		{Role: "system", Content: "不应该传给模型"},
		{Role: "user", Content: "旧问题"},
		{Role: "assistant", Content: "旧回答"},
		{Role: "user", Content: "问题2"},
		{Role: "assistant", Content: "回答2"},
		{Role: "user", Content: "问题3"},
		{Role: "assistant", Content: "回答3"},
		{Role: "user", Content: strings.Repeat("很长", 160)},
	}

	if _, err := service.Ask(context.Background(), AskInput{Question: "完美型怎么成长？", History: history}); err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}

	if len(generator.input.History) != 6 {
		t.Fatalf("expected 6 recent history messages, got %+v", generator.input.History)
	}
	last := generator.input.History[len(generator.input.History)-1]
	if len([]rune(last.Content)) != 223 || !strings.HasSuffix(last.Content, "...") {
		t.Fatalf("expected long history to be trimmed, got len=%d content=%q", len([]rune(last.Content)), last.Content)
	}
}

func TestAskFallsBackWhenGeneratorFails(t *testing.T) {
	service := NewService([]Document{
		{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则。", Tags: []string{"完美型"}},
	}, WithGenerator(&fakeGenerator{err: errors.New("llm unavailable")}))

	result, err := service.Ask(context.Background(), AskInput{
		Question:    "完美型怎么成长？",
		UserProfile: UserProfile{Nickname: "小九", MainType: 1},
	})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if !strings.Contains(result.Answer, "我先按你问到的重点检索了九型资料") {
		t.Fatalf("expected fallback retrieval answer, got %q", result.Answer)
	}
}

type fakeGenerator struct {
	answer string
	err    error
	input  GenerateInput
}

func (f *fakeGenerator) Generate(_ context.Context, input GenerateInput) (string, error) {
	f.input = input
	if f.err != nil {
		return "", f.err
	}
	return f.answer, nil
}
