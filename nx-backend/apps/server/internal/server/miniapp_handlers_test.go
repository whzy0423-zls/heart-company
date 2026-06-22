package server

import (
	"testing"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestMergeMiniappRAGDocumentsIncludesKnowledgeStore(t *testing.T) {
	docs := mergeMiniappRAGDocuments(
		[]rag.Document{{ID: "type-1", Title: "1号", Content: "原则"}},
		[]rag.Document{{ID: "kb-8", Title: "课程答疑", Content: "课程安排"}},
	)
	if len(docs) != 2 {
		t.Fatalf("expected site and knowledge documents, got %+v", docs)
	}
	if docs[0].ID != "type-1" || docs[1].ID != "kb-8" {
		t.Fatalf("unexpected document order: %+v", docs)
	}
}

func TestMergedKnowledgeDocumentsAreSearchable(t *testing.T) {
	service := rag.NewService(mergeMiniappRAGDocuments(
		[]rag.Document{{ID: "type-1", Title: "1号 完美型", Content: "完美型重视原则。", Tags: []string{"完美型"}}},
		[]rag.Document{{ID: "kb-8", Title: "企业沟通课", Content: "企业沟通课适合团队冲突复盘和管理者沟通训练。", Tags: []string{"企业", "沟通"}}},
	))

	answer, err := service.Ask(nil, rag.AskInput{Question: "企业沟通课适合什么场景？"})
	if err != nil {
		t.Fatalf("Ask returned error: %v", err)
	}
	if len(answer.Sources) == 0 || answer.Sources[0].ID != "kb-8" {
		t.Fatalf("expected knowledge document source, got %+v", answer.Sources)
	}
}
