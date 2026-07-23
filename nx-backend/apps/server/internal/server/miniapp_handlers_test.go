package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/miniapp"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/wechat"
)

func TestMergeAppRAGDocumentsIncludesKnowledgeStore(t *testing.T) {
	docs := mergeAppRAGDocuments(
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

func TestMiniappSiteRAGDocumentsExcludeKnowledgeStore(t *testing.T) {
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
	if !foundSite || foundKnowledge {
		t.Fatalf("expected site document and no public knowledge documents, foundSite=%v foundKnowledge=%v docs=%+v", foundSite, foundKnowledge, docs)
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

func TestAppKnowledgeDocumentsAreSearchable(t *testing.T) {
	service := rag.NewService(mergeAppRAGDocuments(
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

type wxLoginUserServiceFake struct {
	id      int64
	err     error
	calls   int
	openid  string
	unionid string
	channel string
	scene   string
}

func (f *wxLoginUserServiceFake) UpsertUser(_ context.Context, openid, unionid, channel, scene string) (int64, error) {
	f.calls++
	f.openid = openid
	f.unionid = unionid
	f.channel = channel
	f.scene = scene
	return f.id, f.err
}

var wxLoginReadDriverOnce sync.Once

type wxLoginReadDriver struct{}
type wxLoginReadConn struct{}
type wxLoginReadRows struct {
	done bool
}

func (wxLoginReadDriver) Open(string) (driver.Conn, error) { return wxLoginReadConn{}, nil }
func (wxLoginReadConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare unavailable")
}
func (wxLoginReadConn) Close() error              { return nil }
func (wxLoginReadConn) Begin() (driver.Tx, error) { return nil, errors.New("transactions unavailable") }
func (wxLoginReadConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "SELECT id, nickname, avatar") {
		return nil, errors.New("unexpected non-read miniapp query")
	}
	return &wxLoginReadRows{}, nil
}
func (*wxLoginReadRows) Columns() []string {
	return []string{"id", "nickname", "avatar", "phone", "gender", "main_type", "member_level", "create_time"}
}
func (*wxLoginReadRows) Close() error { return nil }
func (r *wxLoginReadRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(42)
	dest[1] = "小芯"
	dest[2] = "https://example.com/avatar.png"
	dest[3] = ""
	dest[4] = ""
	dest[5] = int64(0)
	dest[6] = int64(0)
	dest[7] = time.Unix(1_700_000_000, 0)
	return nil
}

func openWxLoginReadDB(t *testing.T) *sql.DB {
	t.Helper()
	const driverName = "nine_xing_wx_login_read"
	wxLoginReadDriverOnce.Do(func() { sql.Register(driverName, wxLoginReadDriver{}) })
	database, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestWxLoginUsesMiniappUserServiceAndKeepsResponseCompatible(t *testing.T) {
	service := &wxLoginUserServiceFake{id: 42}
	s := &Server{
		env:            config.Env{JWTSecret: "test-secret"},
		miniapp:        miniapp.NewStore(openWxLoginReadDB(t)),
		miniappService: service,
		wx:             wechat.NewClient("", "", true),
	}

	response := performRawUnit(http.HandlerFunc(s.wxLogin), http.MethodPost, "/api/wx/login", `{"code":"login-code","channel":"campaign","scene":"1001"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 1 || !strings.HasPrefix(service.openid, "dev_") || service.unionid != "" || service.channel != "campaign" || service.scene != "1001" {
		t.Fatalf("unexpected service call: %+v", service)
	}
	var envelope struct {
		Data struct {
			AccessToken string       `json:"accessToken"`
			User        miniapp.User `json:"user"`
			DevMode     bool         `json:"devMode"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.AccessToken == "" || envelope.Data.User.ID != "42" || envelope.Data.User.Nickname != "小芯" || !envelope.Data.DevMode {
		t.Fatalf("incompatible login response: %+v", envelope.Data)
	}
}

func TestWxLoginStopsWhenMiniappUserTransactionFails(t *testing.T) {
	service := &wxLoginUserServiceFake{err: errors.New("transaction failed")}
	s := &Server{
		env:            config.Env{JWTSecret: "test-secret"},
		miniapp:        miniapp.NewStore(openWxLoginReadDB(t)),
		miniappService: service,
		wx:             wechat.NewClient("", "", true),
	}

	response := performRawUnit(http.HandlerFunc(s.wxLogin), http.MethodPost, "/api/wx/login", `{"code":"login-code"}`)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if service.calls != 1 {
		t.Fatalf("service calls = %d, want 1", service.calls)
	}
}
