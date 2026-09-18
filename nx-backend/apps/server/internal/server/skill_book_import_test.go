package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/jackc/pgx/v5/stdlib"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"nine-xing/nx-backend/apps/server/internal/testutil"
	"os"
	"strconv"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/skillcatalog"
	"nine-xing/nx-backend/apps/server/internal/theorystore"
)

// Uses a disposable *_test database, never production. Validates the actual
// multipart handler and publishing transaction against real schema constraints.
func TestBookUploadPublishRenameAppCatalogPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var libraryID, categoryID int64
	err = db.QueryRow(`INSERT INTO app_skill_libraries(key,name,status) VALUES('learning-growth-books','书籍测试库','enabled') ON CONFLICT(key) DO UPDATE SET status='enabled' RETURNING id`).Scan(&libraryID)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(`INSERT INTO app_skill_categories(library_id,key,name,status) VALUES($1,'book-upload-test','导入测试','enabled') ON CONFLICT(library_id,key) DO UPDATE SET status='enabled' RETURNING id`, libraryID).Scan(&categoryID)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	form.WriteField("name", "上传书籍测试")
	form.WriteField("categoryId", strconv.FormatInt(categoryID, 10))
	f, _ := form.CreateFormFile("file", "成长练习.txt")
	f.Write([]byte(strings.Repeat("每天完成十分钟刻意练习，通过观察反馈调整下一次行动。", 30)))
	form.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/skill-library-management/books/import", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	response := httptest.NewRecorder()
	server := &Server{db: db}
	server.importSkillBook(response, request)
	if response.Code != 200 {
		t.Fatalf("upload %d: %s", response.Code, response.Body)
	}
	var envelope struct {
		Data skillcatalog.BookImportResult `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	result := envelope.Data
	if result.ID <= 0 || result.Characters < 100 {
		t.Fatalf("invalid result %s", response.Body)
	}
	catalog := skillcatalog.NewStore(db)
	if _, err := catalog.GetSkill(ctx, result.ID); err == nil {
		t.Fatal("unpublished book exposed to App")
	}
	publish := httptest.NewRecorder()
	server.updateManagedSkillLifecycle(publish, httptest.NewRequest(http.MethodPost, "/", nil), result.ID, "publish")
	if publish.Code != 200 {
		t.Fatalf("publish %d %s", publish.Code, publish.Body)
	}
	detail, err := catalog.GetSkill(ctx, result.ID)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := theorystore.NewStore(db).SearchReleaseChunks(ctx, detail.Version.TheoryReleaseID, "刻意练习", 5, 0)
	if err != nil || len(docs) == 0 || !strings.Contains(docs[0].Content, "十分钟刻意练习") {
		t.Fatalf("actual book retrieval: %#v %v", docs, err)
	}
	renameBody := fmt.Sprintf(`{"name":"已改名书籍","summary":"保留真实书籍内容","description":"测试","categoryId":%d,"iconKey":"menu_book","colorToken":"green","sortOrder":0,"status":"enabled"}`, categoryID)
	rename := httptest.NewRecorder()
	server.updateManagedSkill(rename, httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(renameBody)), result.ID)
	if rename.Code != 200 {
		t.Fatalf("rename %d %s", rename.Code, rename.Body)
	}
	detail, err = catalog.GetSkill(ctx, result.ID)
	if err != nil || detail.Name != "已改名书籍" {
		t.Fatalf("App rename %#v %v", detail, err)
	}
	page, err := catalog.ListSkills(ctx, skillcatalog.SkillFilter{LibraryID: libraryID, Query: "已改名书籍", Limit: 50})
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("App catalog rename %#v %v", page, err)
	}
	hide := httptest.NewRecorder()
	server.updateManagedSkillLifecycle(hide, httptest.NewRequest(http.MethodPost, "/", nil), result.ID, "unpublish")
	if hide.Code != 200 {
		t.Fatalf("unpublish %d %s", hide.Code, hide.Body)
	}
	if _, err := catalog.GetSkill(ctx, result.ID); err == nil {
		t.Fatal("unpublished book still visible")
	}
}
