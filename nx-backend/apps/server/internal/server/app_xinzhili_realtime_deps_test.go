package server

import (
	"context"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
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

func TestServerXinzhiliLayeredKnowledgeUsesCurrentConversationCard(t *testing.T) {
	resolver := &layeredKnowledgeResolver{mainType: 4, revision: 12}
	searcher := newLayeredKnowledgeSearcher()
	server := &Server{appKnowledge: appknowledge.NewCoordinator(resolver, searcher, searcher)}

	result, err := (serverXinzhiliLayeredKnowledge{server: server}).Retrieve(context.Background(), 7, 91, 55, layeredKnowledgeQuestion)
	if err != nil {
		t.Fatal(err)
	}
	if resolver.calls != 1 || resolver.lastUserID != 7 || resolver.lastSessionID != 91 || resolver.lastCardID != 55 {
		t.Fatalf("resolution calls=%d input=%d/%d/%d", resolver.calls, resolver.lastUserID, resolver.lastSessionID, resolver.lastCardID)
	}
	if result.Trace == nil || result.Trace.CardID != 55 || result.Trace.EnneagramType == nil || *result.Trace.EnneagramType != 4 || result.Trace.CardRevision != 12 {
		t.Fatalf("realtime trace = %+v", result.Trace)
	}
	if len(result.Documents) != 3 {
		t.Fatalf("realtime documents = %+v", result.Documents)
	}
}
