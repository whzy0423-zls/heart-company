package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/publicknowledge"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

type fakePublicKnowledgeStore struct {
	enabled     bool
	searchCalls int
	statusCalls int
	err         error
	params      publicknowledge.ListParams
}

func (f *fakePublicKnowledgeStore) List(_ context.Context, p publicknowledge.ListParams) (publicknowledge.ListResult, error) {
	f.params = p
	return publicknowledge.ListResult{Items: []publicknowledge.Source{{ID: "fixture:book:1", Enabled: f.enabled}}, Total: 1, Page: p.Page, PageSize: p.PageSize, Categories: []publicknowledge.Category{{Name: "心理学", Count: 1}}}, f.err
}
func (f *fakePublicKnowledgeStore) SetEnabled(_ context.Context, ids []string, enabled bool) (int, error) {
	f.statusCalls++
	if f.err != nil {
		return 0, f.err
	}
	f.enabled = enabled
	return len(ids), nil
}
func (f *fakePublicKnowledgeStore) Preview(context.Context, string) (publicknowledge.PreviewResult, error) {
	return publicknowledge.PreviewResult{Items: []publicknowledge.Chunk{}, Total: 0}, f.err
}
func (f *fakePublicKnowledgeStore) Search(context.Context, string, int) ([]rag.Document, error) {
	f.searchCalls++
	if f.enabled {
		return []rag.Document{{ID: "managed:1", Title: "心理成长", Content: "情绪管理"}}, f.err
	}
	return nil, f.err
}

func TestPublicKnowledgeRoutesRequireManagementPermission(t *testing.T) {
	mux := http.NewServeMux()
	var permissions []string
	registerPublicKnowledgeAdminRoutes(mux, func(code string, _ http.HandlerFunc) http.HandlerFunc {
		return func(http.ResponseWriter, *http.Request) { permissions = append(permissions, code) }
	}, &Server{})
	for _, path := range []string{"/api/rag/sources", "/api/rag/sources/status", "/api/rag/sources/fixture:book:1/chunks"} {
		permissions = nil
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
		if len(permissions) != 1 || permissions[0] != "RAG:Knowledge:Manage" {
			t.Fatalf("path=%s permissions=%v", path, permissions)
		}
	}
	server := &Server{}
	mux = http.NewServeMux()
	registerPublicKnowledgeAdminRoutes(mux, server.requirePermission, server)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/rag/sources", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", rec.Code)
	}
}

func TestPublicKnowledgeListAndPreviewContract(t *testing.T) {
	fake := &fakePublicKnowledgeStore{enabled: true}
	server := &Server{publicKnowledge: fake}
	rec := httptest.NewRecorder()
	server.publicKnowledgeSources(rec, httptest.NewRequest(http.MethodGet, "/api/rag/sources?category=心理学&page=2&pageSize=5&enabled=false", nil))
	if rec.Code != 200 || fake.params.Page != 2 || fake.params.Category != "心理学" || !strings.Contains(rec.Body.String(), `"categories"`) || !strings.Contains(rec.Body.String(), `"pageSize":5`) {
		t.Fatalf("status=%d params=%+v body=%s", rec.Code, fake.params, rec.Body.String())
	}
	for _, path := range []string{"/api/rag/sources/a", "/api/rag/sources/a/chunks/extra"} {
		rec = httptest.NewRecorder()
		server.publicKnowledgeSourceChunks(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("invalid preview path %s status=%d", path, rec.Code)
		}
	}
	fake.err = publicknowledge.ErrNotFound
	rec = httptest.NewRecorder()
	server.publicKnowledgeSourceChunks(rec, httptest.NewRequest(http.MethodGet, "/api/rag/sources/missing/chunks", nil))
	if rec.Code != 404 {
		t.Fatalf("missing preview status=%d", rec.Code)
	}
}

func TestPublicKnowledgeStatusRejectsInvalidPayloadsBeforeWriting(t *testing.T) {
	fake := &fakePublicKnowledgeStore{enabled: true}
	server := &Server{publicKnowledge: fake}
	for _, body := range []string{`{}`, `{"ids":["a"]}`, `{"ids":[],"enabled":false}`, `{"ids":["a","a"],"enabled":false}`, `{"ids":["a"],"enabled":false,"extra":1}`, `{"ids":["a"],"enabled":false} {}`, strings.Repeat("x", 65537)} {
		rec := httptest.NewRecorder()
		server.publicKnowledgeSourceStatus(rec, httptest.NewRequest(http.MethodPost, "/api/rag/sources/status", strings.NewReader(body)))
		if rec.Code != 400 || fake.statusCalls != 0 || !fake.enabled {
			t.Fatalf("invalid body status=%d calls=%d enabled=%v", rec.Code, fake.statusCalls, fake.enabled)
		}
	}
	rec := httptest.NewRecorder()
	server.publicKnowledgeSourceStatus(rec, httptest.NewRequest(http.MethodPost, "/api/rag/sources/status", strings.NewReader(`{"ids":["a"],"enabled":false}`)))
	if rec.Code != 200 || fake.enabled || !strings.Contains(rec.Body.String(), `"updated":1`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRetrieveAppDocsIncludesLiveManagedSearchWithoutEmbedding(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "site.json")
	if err := os.WriteFile(configPath, []byte(miniappRAGTestConfig), 0600); err != nil {
		t.Fatal(err)
	}
	fake := &fakePublicKnowledgeStore{enabled: true}
	server := &Server{env: config.Env{SiteConfig: configPath}, ragDocs: &fakeRAGDocumentStore{enabledDocs: []rag.Document{{ID: "kb-8", Title: "手工条目", Content: "沟通"}}}, publicKnowledge: fake}
	for _, enabled := range []bool{true, false, true} {
		fake.enabled = enabled
		docs, err := server.retrieveAppDocsForQuery(context.Background(), "情绪管理", 6)
		if err != nil {
			t.Fatal(err)
		}
		managed, legacy := false, false
		for _, doc := range docs {
			managed = managed || doc.ID == "managed:1"
			legacy = legacy || doc.ID == "kb-8"
		}
		if managed != enabled || !legacy {
			t.Fatalf("enabled=%v managed=%v legacy=%v", enabled, managed, legacy)
		}
	}
	if fake.searchCalls != 3 {
		t.Fatalf("managed search must run each request, calls=%d", fake.searchCalls)
	}
	fake.err = errors.New("fixture query failure")
	docs, err := server.retrieveAppDocsForQuery(context.Background(), "情绪管理", 6)
	if err != nil || len(docs) == 0 {
		t.Fatalf("managed failure must preserve legacy fallback docs=%v err=%v", docs, err)
	}
}
