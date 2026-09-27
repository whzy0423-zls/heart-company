package rag

import (
	"strings"
	"testing"
)

func TestBuildRetrievalQueryKeepsStandaloneQuestionFocused(t *testing.T) {
	history := []Message{{Role: "user", Content: "我之前总是在拖延。"}}

	query := BuildRetrievalQuery("伴侣说我太敏感，我应该怎么回应？", history, "旧摘要")

	if query != "伴侣说我太敏感，我应该怎么回应？" {
		t.Fatalf("standalone retrieval query = %q", query)
	}
}

func TestBuildRetrievalQueryAddsRecentUserContextForFollowUp(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "我最近读书总是读完就忘。"},
		{Role: "assistant", Content: "可以试试三句复述法。"},
	}

	query := BuildRetrievalQuery("就按刚才第一个方法，给我一个具体例子。", history, "")

	if !strings.Contains(query, "我最近读书总是读完就忘") || !strings.Contains(query, "就按刚才第一个方法") {
		t.Fatalf("contextual retrieval query = %q", query)
	}
	if strings.Contains(query, "可以试试三句复述法") {
		t.Fatalf("assistant answer must not become retrieval evidence: %q", query)
	}
}

func TestBuildRetrievalQueryUsesSummaryWhenNoRecentUserTurnExists(t *testing.T) {
	query := BuildRetrievalQuery("那我现在该怎么办？", nil, "用户正在讨论伴侣冲突和沟通边界。")

	if !strings.Contains(query, "伴侣冲突和沟通边界") || !strings.Contains(query, "那我现在该怎么办") {
		t.Fatalf("summary-backed retrieval query = %q", query)
	}
}

func TestBuildRetrievalQueryRecognizesCommonChineseFollowUps(t *testing.T) {
	history := []Message{{Role: "user", Content: "我和同事沟通时容易急着证明自己。"}}
	for _, question := range []string{
		"那他呢？",
		"这种情况怎么处理？",
		"能再详细一点吗？",
		"具体应该怎么做？",
	} {
		t.Run(question, func(t *testing.T) {
			query := BuildRetrievalQuery(question, history, "")
			if !strings.Contains(query, history[0].Content) {
				t.Fatalf("follow-up retrieval query=%q", query)
			}
		})
	}
}
