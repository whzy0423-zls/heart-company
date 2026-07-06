package ragstore

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeDocumentPreparesManualKnowledge(t *testing.T) {
	doc, err := NormalizeDocument(Document{
		Title:   "  关系沟通  ",
		Content: "  亲密关系里先表达感受，再表达请求。  ",
		Tags:    []string{" 沟通 ", "关系", "沟通", ""},
	})
	if err != nil {
		t.Fatalf("NormalizeDocument returned error: %v", err)
	}
	if doc.Title != "关系沟通" || doc.Content != "亲密关系里先表达感受，再表达请求。" {
		t.Fatalf("unexpected normalized document: %+v", doc)
	}
	if doc.Status != StatusEnabled || doc.Source != SourceManual {
		t.Fatalf("expected default enabled/manual, got status=%q source=%q", doc.Status, doc.Source)
	}
	if got := len(doc.Tags); got != 2 || doc.Tags[0] != "沟通" || doc.Tags[1] != "关系" {
		t.Fatalf("expected trimmed unique tags, got %+v", doc.Tags)
	}
}

func TestNormalizeDocumentRejectsEmptyRequiredFields(t *testing.T) {
	if _, err := NormalizeDocument(Document{Title: "  ", Content: "内容"}); err == nil {
		t.Fatal("expected title validation error")
	}
	if _, err := NormalizeDocument(Document{Title: "标题", Content: "  "}); err == nil {
		t.Fatal("expected content validation error")
	}
}

func TestToRAGDocumentsOnlyReturnsEnabledDocuments(t *testing.T) {
	docs := ToRAGDocuments([]Document{
		{ID: "1", Title: "可用知识", Content: "适合检索的内容", Tags: []string{"成长"}, Status: StatusEnabled, Source: SourceManual},
		{ID: "2", Title: "停用知识", Content: "不应该被检索", Tags: []string{"停用"}, Status: StatusDisabled, Source: SourceManual},
		{ID: "3", Title: "", Content: "缺标题", Status: StatusEnabled},
	})
	if len(docs) != 1 {
		t.Fatalf("expected one enabled RAG document, got %+v", docs)
	}
	if docs[0].ID != "kb-1" || docs[0].Title != "可用知识" || docs[0].Tags[0] != "成长" {
		t.Fatalf("unexpected RAG document: %+v", docs[0])
	}
}

func TestEnabledDocumentsLimitCoversSeededKnowledgePack(t *testing.T) {
	if enabledDocumentsLimit < 5000 {
		t.Fatalf("enabledDocumentsLimit=%d is too small for the expanding seeded App knowledge pack", enabledDocumentsLimit)
	}
	source := readSourceFile(t, "store.go")
	if strings.Contains(source, "LIMIT 200") {
		t.Fatal("EnabledDocuments must not keep the old 200-row cap; it would hide later seed entries from App RAG")
	}
	if !strings.Contains(source, "LIMIT $2") {
		t.Fatal("EnabledDocuments should use the configured enabledDocumentsLimit instead of a hard-coded small LIMIT")
	}
}

func TestSaveDocumentUpdateInvalidatesExistingEmbedding(t *testing.T) {
	source := readSourceFile(t, "store.go")
	if !strings.Contains(source, "embedding=NULL") || !strings.Contains(source, "embedding_model=''") || !strings.Contains(source, "embedded_at=NULL") {
		t.Fatal("updating a RAG document should invalidate stale embedding fields")
	}
}

func TestDocsNeedingEmbeddingIncludesStaleUpdatedDocuments(t *testing.T) {
	source := readSourceFile(t, "vector.go")
	if !strings.Contains(source, "embedded_at IS NULL") || !strings.Contains(source, "embedded_at < update_time") {
		t.Fatal("DocsNeedingEmbedding should include documents updated after embedding")
	}
}

func readSourceFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUpdateDocumentSQLSkipsEmbeddingColumnsWhenVectorUnavailable(t *testing.T) {
	query := updateDocumentSQL(false)
	if strings.Contains(query, "embedding") || strings.Contains(query, "embedded_at") {
		t.Fatalf("non-vector update SQL must not reference optional embedding columns: %s", query)
	}
	if !strings.Contains(query, "update_time=now()") {
		t.Fatalf("expected update_time refresh in update SQL: %s", query)
	}
}

func TestUpdateDocumentSQLInvalidatesEmbeddingWhenVectorAvailable(t *testing.T) {
	query := updateDocumentSQL(true)
	for _, want := range []string{"embedding=NULL", "embedding_model=''", "embedded_at=NULL"} {
		if !strings.Contains(query, want) {
			t.Fatalf("vector update SQL should contain %s: %s", want, query)
		}
	}
}
