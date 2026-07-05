package articlestore

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeArticleTrimsAndDefaults(t *testing.T) {
	doc, err := NormalizeArticle(Article{
		Title:   "  九型与亲密关系  ",
		Summary: "  在关系里照见自己  ",
		Content: "  # 标题\n正文内容  ",
		Author:  "  芯之力  ",
		Tags:    []string{" 关系 ", "成长", "关系", ""},
	})
	if err != nil {
		t.Fatalf("NormalizeArticle returned error: %v", err)
	}
	if doc.Title != "九型与亲密关系" {
		t.Fatalf("unexpected title: %q", doc.Title)
	}
	if doc.Summary != "在关系里照见自己" {
		t.Fatalf("unexpected summary: %q", doc.Summary)
	}
	if doc.Author != "芯之力" {
		t.Fatalf("unexpected author: %q", doc.Author)
	}
	if doc.Status != StatusPublished {
		t.Fatalf("expected default status published, got %q", doc.Status)
	}
	if len(doc.Tags) != 2 || doc.Tags[0] != "关系" || doc.Tags[1] != "成长" {
		t.Fatalf("unexpected tags: %+v", doc.Tags)
	}
}

func TestNormalizeArticleKeepsDraftStatus(t *testing.T) {
	doc, err := NormalizeArticle(Article{
		Title:   "草稿",
		Content: "内容",
		Status:  StatusDraft,
	})
	if err != nil {
		t.Fatalf("NormalizeArticle returned error: %v", err)
	}
	if doc.Status != StatusDraft {
		t.Fatalf("expected draft status preserved, got %q", doc.Status)
	}
}

func TestNormalizeArticleRequiresTitleAndContent(t *testing.T) {
	if _, err := NormalizeArticle(Article{Content: "正文"}); err == nil {
		t.Fatal("expected error for missing title")
	}
	if _, err := NormalizeArticle(Article{Title: "标题"}); err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestPublicArticleAssetReferenceQueriesIncludeContent(t *testing.T) {
	articleReferenceRecorder.reset()
	database, err := sql.Open("article_reference_query_test", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	store := &Store{db: database}
	if _, err := store.PublicAssetReferenced(context.Background(), 42); err != nil {
		t.Fatalf("PublicAssetReferenced returned error: %v", err)
	}
	if query := articleReferenceRecorder.query(); !strings.Contains(query, "content") {
		t.Fatalf("expected public upload asset reference query to include article content, query:\n%s", query)
	}

	articleReferenceRecorder.reset()
	if _, err := store.PublicLocalUploadReferenced(context.Background(), "/api/uploads/article/body.png"); err != nil {
		t.Fatalf("PublicLocalUploadReferenced returned error: %v", err)
	}
	if query := articleReferenceRecorder.query(); !strings.Contains(query, "content") {
		t.Fatalf("expected public local upload reference query to include article content, query:\n%s", query)
	}
}

func TestArticleContentReferenceMatchingRequiresExactUploadURL(t *testing.T) {
	if articleContentReferencesUploadAsset(`![x](/api/upload-assets/123)`, 12) {
		t.Fatal("asset id 12 must not match /api/upload-assets/123")
	}
	if !articleContentReferencesUploadAsset(`![x](/api/upload-assets/12)`, 12) {
		t.Fatal("asset id 12 should match exact private upload asset URL")
	}
	if !articleContentReferencesUploadAsset(`<img src="/api/public/article-assets/12">`, 12) {
		t.Fatal("asset id 12 should match exact public article asset URL")
	}
	if articleContentReferencesLocalUpload(`![x](/api/uploads/article/a.png.bak)`, "/api/uploads/article/a.png") {
		t.Fatal("local upload a.png must not match a.png.bak")
	}
	if !articleContentReferencesLocalUpload(`![x](/api/uploads/article/a.png)`, "/api/uploads/article/a.png") {
		t.Fatal("local upload should match exact private URL")
	}
	if !articleContentReferencesLocalUpload(`<img src="/api/public/article-uploads/article/a.png">`, "/api/uploads/article/a.png") {
		t.Fatal("local upload should match exact public article upload URL")
	}
}

var articleReferenceRecorder = &articleReferenceQueryRecorder{}

func init() {
	sql.Register("article_reference_query_test", articleReferenceDriver{})
}

type articleReferenceQueryRecorder struct {
	mu    sync.Mutex
	value string
}

func (r *articleReferenceQueryRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = ""
}

func (r *articleReferenceQueryRecorder) set(query string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value = query
}

func (r *articleReferenceQueryRecorder) query() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.value
}

type articleReferenceDriver struct{}

func (articleReferenceDriver) Open(string) (driver.Conn, error) {
	return articleReferenceConn{}, nil
}

type articleReferenceConn struct{}

func (articleReferenceConn) Prepare(string) (driver.Stmt, error) { return nil, nil }
func (articleReferenceConn) Close() error                        { return nil }
func (articleReferenceConn) Begin() (driver.Tx, error)           { return nil, nil }
func (articleReferenceConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	articleReferenceRecorder.set(query)
	return &articleReferenceRows{}, nil
}

type articleReferenceRows struct {
	done bool
}

func (articleReferenceRows) Columns() []string { return []string{"exists"} }
func (articleReferenceRows) Close() error      { return nil }
func (r *articleReferenceRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = false
	r.done = true
	return nil
}

var _ driver.QueryerContext = articleReferenceConn{}
