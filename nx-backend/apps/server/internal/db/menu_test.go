package db

import "testing"

func TestDefaultMenusIncludeRAGKnowledgeManagement(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "RAGKnowledge" {
			continue
		}
		found = true
		if menu.Path != "/rag/knowledge" || menu.Component != "/rag/knowledge" {
			t.Fatalf("unexpected RAG knowledge route: %+v", menu)
		}
		if menu.AuthCode != "RAG:Knowledge:Manage" || menu.Title != "知识库管理" {
			t.Fatalf("unexpected RAG knowledge metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu RAGKnowledge")
	}
}
