package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

const modelConfigViewTestDriverName = "nine_xing_model_config_view"

var registerModelConfigViewDriverOnce sync.Once

type modelConfigViewTestDriver struct{}

type modelConfigViewTestConn struct {
	raw []byte
}

type modelConfigViewTestRows struct {
	raw  []byte
	done bool
}

func (modelConfigViewTestDriver) Open(raw string) (driver.Conn, error) {
	return &modelConfigViewTestConn{raw: []byte(raw)}, nil
}

func (c *modelConfigViewTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (c *modelConfigViewTestConn) Close() error { return nil }

func (c *modelConfigViewTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (c *modelConfigViewTestConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &modelConfigViewTestRows{raw: c.raw}, nil
}

func (r *modelConfigViewTestRows) Columns() []string { return []string{"config"} }
func (r *modelConfigViewTestRows) Close() error      { return nil }

func (r *modelConfigViewTestRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = r.raw
	return nil
}

func TestModelConfigSavedChatNeverEmitsLegacyGroupID(t *testing.T) {
	cfg := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "secret",
		GroupID:        "must-not-leak",
		Model:          "gpt-5.5",
		TimeoutSeconds: 30,
	}}

	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]map[string]any
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatal(err)
	}
	if _, ok := saved["chat"]["groupId"]; ok {
		t.Fatalf("expected saved chat config to omit legacy groupId, got %s", body)
	}
}

func TestModelConfigChatAuditSnapshotUsesCompatibleContract(t *testing.T) {
	snapshot := modelConfigAuditSnapshot(modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderAnthropicCompatible,
		APIBase:        "https://api.anthropic.com/v1",
		APIKey:         "secret",
		GroupID:        "must-not-leak",
		Model:          "claude-sonnet",
		TimeoutSeconds: 55,
	}})

	chat, ok := snapshot["chat"].(map[string]any)
	if !ok {
		t.Fatalf("expected chat audit map, got %+v", snapshot["chat"])
	}
	if chat["provider"] != modelconfig.ProviderAnthropicCompatible || chat["timeoutSeconds"] != 55 {
		t.Fatalf("expected compatible provider and timeout in chat audit snapshot, got %+v", chat)
	}
	if _, ok := chat["groupId"]; ok {
		t.Fatalf("expected chat audit snapshot to omit groupId, got %+v", chat)
	}
}

func TestModelConfigGETChatUsesCompatiblePublicContract(t *testing.T) {
	raw := `{"chat":{"provider":"openai-compatible","apiBase":"https://api.openai.com/v1","apiKey":"secret","model":"gpt-5.5","timeoutSeconds":42}}`
	db := openModelConfigViewTestDB(t, raw)
	s := &Server{
		db: db,
		env: config.Env{MiniMax: config.MiniMaxConfig{
			APIBase:        "https://api.minimaxi.com",
			APIKey:         "env-secret",
			GroupID:        "env-group",
			Model:          "MiniMax-M2.7",
			TimeoutSeconds: 99,
		}},
	}

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodGet, "/api/model-config", "")
	if response.Code != http.StatusOK {
		t.Fatalf("expected model config GET 200, got %d body=%s", response.Code, response.Body.String())
	}
	chat := decodeModelConfigViewChat(t, response.Body.Bytes())

	if chat["provider"] != modelconfig.ProviderOpenAICompatible {
		t.Fatalf("expected compatible provider in public chat view, got %+v", chat)
	}
	if chat["timeoutSeconds"] != float64(42) {
		t.Fatalf("expected timeoutSeconds 42 in public chat view, got %+v", chat)
	}
	if chat["apiBase"] != "https://api.openai.com/v1" || chat["model"] != "gpt-5.5" || chat["apiKeySet"] != true {
		t.Fatalf("expected stored compatible chat fields in public view, got %+v", chat)
	}
	if _, ok := chat["groupId"]; ok {
		t.Fatalf("expected public chat view to omit groupId, got %+v", chat)
	}
}

func TestModelConfigGETLegacyChatRemainsUnconfigured(t *testing.T) {
	raw := `{"chat":{"apiBase":"https://api.minimax.chat/v1","apiKey":"old","groupId":"legacy","model":"MiniMax-M2.7"}}`
	db := openModelConfigViewTestDB(t, raw)
	s := &Server{
		db: db,
		env: config.Env{MiniMax: config.MiniMaxConfig{
			APIBase:        "https://api.minimaxi.com",
			APIKey:         "env-secret",
			GroupID:        "env-group",
			Model:          "MiniMax-M2.7",
			TimeoutSeconds: 99,
		}},
	}

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodGet, "/api/model-config", "")
	if response.Code != http.StatusOK {
		t.Fatalf("expected model config GET 200, got %d body=%s", response.Code, response.Body.String())
	}
	chat := decodeModelConfigViewChat(t, response.Body.Bytes())

	if provider, ok := chat["provider"]; !ok || provider != "" {
		t.Fatalf("expected legacy chat provider to remain explicitly unconfigured, got %+v", chat)
	}
	if timeout, ok := chat["timeoutSeconds"]; !ok || timeout != float64(0) {
		t.Fatalf("expected legacy chat timeout to remain unconfigured, got %+v", chat)
	}
	if _, ok := chat["groupId"]; ok {
		t.Fatalf("expected legacy public chat view to omit groupId, got %+v", chat)
	}
}

func openModelConfigViewTestDB(t *testing.T, raw string) *sql.DB {
	t.Helper()
	registerModelConfigViewDriverOnce.Do(func() {
		sql.Register(modelConfigViewTestDriverName, modelConfigViewTestDriver{})
	})
	db, err := sql.Open(modelConfigViewTestDriverName, raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func decodeModelConfigViewChat(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var response struct {
		Code int `json:"code"`
		Data struct {
			Chat map[string]any `json:"chat"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response.Code != 0 {
		t.Fatalf("expected success response, got %s", body)
	}
	return response.Data.Chat
}
