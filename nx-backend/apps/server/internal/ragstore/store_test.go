package ragstore

import "testing"

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
