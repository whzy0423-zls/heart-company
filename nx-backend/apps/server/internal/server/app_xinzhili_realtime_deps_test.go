package server

import (
	"testing"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestFilterXinzhiliKnowledgeAppliesActualScoreThreshold(t *testing.T) {
	documents := []rag.Document{
		{ID: "knowledge:relationship", Title: "伴侣冲突沟通", Content: "先说感受和需要，再提出请求。"},
		{ID: "knowledge:recipe", Title: "三道家常菜", Content: "番茄炒蛋、红烧豆腐和青椒肉丝的做法。"},
	}

	matches := filterXinzhiliKnowledge("推荐三道家常菜做法", documents, 4, 0.35)
	if len(matches) != 1 || matches[0].ID != "knowledge:recipe" {
		t.Fatalf("recipe query matches=%+v", matches)
	}

	unrelated := filterXinzhiliKnowledge("伴侣争吵后怎么表达需要", documents[1:], 4, 0.35)
	if len(unrelated) != 0 {
		t.Fatalf("low-relevance recipe document was injected: %+v", unrelated)
	}
}
