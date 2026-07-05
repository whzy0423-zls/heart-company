package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
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

func TestMiniappRAGDocumentsLoadsSiteAndKnowledgeStore(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "site-config.json")
	if err := os.WriteFile(configPath, []byte(miniappRAGTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	server := &Server{
		env:      config.Env{SiteConfig: configPath},
		ragCache: newMiniappRAGCache(time.Minute),
		ragDocs: &fakeRAGDocumentStore{
			enabledDocs: []rag.Document{{ID: "kb-8", Title: "后台知识库", Content: "公共知识库内容"}},
		},
	}

	docs, err := server.miniappRAGDocuments(context.Background())
	if err != nil {
		t.Fatalf("miniappRAGDocuments returned error: %v", err)
	}
	foundSite := false
	foundKnowledge := false
	for _, doc := range docs {
		switch doc.ID {
		case "type-1":
			foundSite = true
		case "kb-8":
			foundKnowledge = true
		}
	}
	if !foundSite || !foundKnowledge {
		t.Fatalf("expected site and public knowledge documents, foundSite=%v foundKnowledge=%v docs=%+v", foundSite, foundKnowledge, docs)
	}
}

const miniappRAGTestConfig = `{
  "site": {
    "brandName": "九型星球",
    "logo": "/logo.png"
  },
  "navigation": {
    "main": [{"label": "首页", "to": "/"}]
  },
  "home": {},
  "types": [
    {
      "id": "1",
      "name": "完美型",
      "avatar": "/type-1.png",
      "description": "完美型重视原则。",
      "keywords": "原则 自律 改进"
    }
  ]
}`

func TestMiniappKnowledgeDocumentsAreSearchable(t *testing.T) {
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
