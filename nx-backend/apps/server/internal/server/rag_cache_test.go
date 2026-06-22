package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/ragstore"
)

func TestMiniappRAGCacheReusesDocumentsWithinTTL(t *testing.T) {
	cache := newMiniappRAGCache(time.Minute)
	calls := 0

	load := func(context.Context) ([]rag.Document, error) {
		calls++
		return []rag.Document{{ID: "doc", Title: "资料", Content: "内容"}}, nil
	}

	first, err := cache.Get(context.Background(), load)
	if err != nil {
		t.Fatalf("first Get returned error: %v", err)
	}
	second, err := cache.Get(context.Background(), load)
	if err != nil {
		t.Fatalf("second Get returned error: %v", err)
	}

	if calls != 1 {
		t.Fatalf("expected one load call, got %d", calls)
	}
	if len(first) != 1 || len(second) != 1 || first[0].ID != second[0].ID {
		t.Fatalf("unexpected cached docs: first=%+v second=%+v", first, second)
	}
	second[0].Title = "被调用方修改"
	third, err := cache.Get(context.Background(), load)
	if err != nil {
		t.Fatalf("third Get returned error: %v", err)
	}
	if third[0].Title != "资料" {
		t.Fatalf("expected cache to return a copy, got %+v", third[0])
	}
}

func TestMiniappRAGCacheInvalidateForcesReload(t *testing.T) {
	cache := newMiniappRAGCache(time.Minute)
	calls := 0

	load := func(context.Context) ([]rag.Document, error) {
		calls++
		return []rag.Document{{ID: "doc", Title: "资料", Content: "版本"}}, nil
	}

	if _, err := cache.Get(context.Background(), load); err != nil {
		t.Fatalf("first Get returned error: %v", err)
	}
	cache.Invalidate()
	if _, err := cache.Get(context.Background(), load); err != nil {
		t.Fatalf("second Get returned error: %v", err)
	}

	if calls != 2 {
		t.Fatalf("expected reload after invalidate, got %d calls", calls)
	}
}

func TestRAGDocumentCreateInvalidatesMiniappCache(t *testing.T) {
	cache := newMiniappRAGCache(time.Minute)
	if _, err := cache.Get(context.Background(), func(context.Context) ([]rag.Document, error) {
		return []rag.Document{{ID: "old", Title: "旧资料", Content: "旧内容"}}, nil
	}); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	store := &fakeRAGDocumentStore{
		saved: ragstore.Document{ID: "1", Title: "新资料", Content: "新内容", Status: ragstore.StatusEnabled},
	}
	server := &Server{ragCache: cache, ragDocs: store}

	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(ragstore.Document{Title: "新资料", Content: "新内容"}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/rag/documents", &body)
	response := httptest.NewRecorder()

	server.ragDocuments(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", response.Code, response.Body.String())
	}
	loads := 0
	docs, err := cache.Get(context.Background(), func(context.Context) ([]rag.Document, error) {
		loads++
		return []rag.Document{{ID: "new", Title: "新资料", Content: "新内容"}}, nil
	})
	if err != nil {
		t.Fatalf("get cache after create: %v", err)
	}
	if loads != 1 || len(docs) != 1 || docs[0].ID != "new" {
		t.Fatalf("expected cache reload after create, loads=%d docs=%+v", loads, docs)
	}
}

type fakeRAGDocumentStore struct {
	saved ragstore.Document
}

func (s *fakeRAGDocumentStore) DeleteDocument(context.Context, string) (bool, error) {
	return true, nil
}

func (s *fakeRAGDocumentStore) EnabledDocuments(context.Context) ([]rag.Document, error) {
	return nil, nil
}

func (s *fakeRAGDocumentStore) ListDocuments(context.Context, map[string]string) (ragstore.PageResult[ragstore.Document], error) {
	return ragstore.PageResult[ragstore.Document]{Items: []ragstore.Document{}}, nil
}

func (s *fakeRAGDocumentStore) SaveDocument(context.Context, ragstore.Document) (ragstore.Document, error) {
	return s.saved, nil
}
