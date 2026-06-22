package server

import (
	"context"
	"sync"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/ragstore"
)

type ragDocumentStore interface {
	DeleteDocument(ctx context.Context, id string) (bool, error)
	EnabledDocuments(ctx context.Context) ([]rag.Document, error)
	ListDocuments(ctx context.Context, query map[string]string) (ragstore.PageResult[ragstore.Document], error)
	SaveDocument(ctx context.Context, input ragstore.Document) (ragstore.Document, error)
}

type miniappRAGCache struct {
	mu        sync.Mutex
	docs      []rag.Document
	expiresAt time.Time
	ttl       time.Duration
}

func newMiniappRAGCache(ttl time.Duration) *miniappRAGCache {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	return &miniappRAGCache{ttl: ttl}
}

func (c *miniappRAGCache) Get(ctx context.Context, load func(context.Context) ([]rag.Document, error)) ([]rag.Document, error) {
	now := time.Now()
	c.mu.Lock()
	if len(c.docs) > 0 && now.Before(c.expiresAt) {
		docs := cloneRAGDocuments(c.docs)
		c.mu.Unlock()
		return docs, nil
	}
	c.mu.Unlock()

	docs, err := load(ctx)
	if err != nil {
		return nil, err
	}
	docs = cloneRAGDocuments(docs)

	c.mu.Lock()
	c.docs = docs
	c.expiresAt = time.Now().Add(c.ttl)
	copied := cloneRAGDocuments(c.docs)
	c.mu.Unlock()
	return copied, nil
}

func (c *miniappRAGCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.docs = nil
	c.expiresAt = time.Time{}
}

func cloneRAGDocuments(docs []rag.Document) []rag.Document {
	if len(docs) == 0 {
		return nil
	}
	copied := make([]rag.Document, len(docs))
	for i, doc := range docs {
		copied[i] = doc
		if len(doc.Tags) > 0 {
			copied[i].Tags = append([]string(nil), doc.Tags...)
		}
	}
	return copied
}
