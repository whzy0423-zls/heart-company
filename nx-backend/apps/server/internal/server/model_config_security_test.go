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
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/rag"
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

type runtimeChatGenerator struct {
	ping      llm.PingResult
	pingCalls int
	draft     string
	kind      string
}

func (*runtimeChatGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "answer", nil
}
func (*runtimeChatGenerator) GenerateStream(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error) {
	return "answer", nil
}
func (*runtimeChatGenerator) SummarizeConversation(_ context.Context, previous string, messages []rag.Message) (string, error) {
	return previous + ":" + messages[0].Content, nil
}
func (*runtimeChatGenerator) CompleteJSON(context.Context, string, string, int) (string, error) {
	return `{}`, nil
}
func (g *runtimeChatGenerator) Ping(context.Context) llm.PingResult {
	g.pingCalls++
	return g.ping
}
func (g *runtimeChatGenerator) PolishPrompt(_ context.Context, draft, kind string) (string, error) {
	g.draft = draft
	g.kind = kind
	return kind + ":" + draft, nil
}

var _ llm.ChatGenerator = (*runtimeChatGenerator)(nil)

func TestNewServerDoesNotUseMiniMaxEnvironmentForChat(t *testing.T) {
	s := newServer(config.Env{MiniMax: config.MiniMaxConfig{
		APIBase: "https://api.minimaxi.com",
		APIKey:  "legacy-secret",
		Model:   "MiniMax-M3",
	}}, nil)
	if got := s.generator(); got != nil {
		t.Fatalf("expected unconfigured chat generator, got %T", got)
	}
}

func TestApplyStoredModelConfigLeavesLegacyChatUnconfigured(t *testing.T) {
	db := openModelConfigViewTestDB(t, `{"chat":{"apiBase":"https://api.minimax.chat/v1","apiKey":"old","model":"MiniMax-M2.7"}}`)
	s := &Server{db: db, env: config.Env{MiniMax: config.MiniMaxConfig{APIKey: "env-secret"}}}

	s.applyStoredModelConfig()

	if got := s.generator(); got != nil {
		t.Fatalf("expected legacy stored chat to remain unconfigured, got %T", got)
	}
}

func TestApplyStoredModelConfigBuildsCompatibleChatAndHonorsAssistDisable(t *testing.T) {
	enabled := true
	disabled := false
	for _, tt := range []struct {
		name          string
		assist        *bool
		wantFactory   int
		wantGenerator bool
	}{
		{name: "enabled", assist: &enabled, wantFactory: 1, wantGenerator: true},
		{name: "disabled", assist: &disabled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			stored := modelconfig.Config{
				Chat: modelconfig.ChatConfig{
					Provider:       modelconfig.ProviderAnthropicCompatible,
					APIBase:        "https://api.anthropic.com/v1",
					APIKey:         "secret",
					Model:          "claude-model",
					TimeoutSeconds: 47,
				},
				Assist: modelconfig.AssistConfig{Enabled: tt.assist},
			}
			db, _ := openAtomicModelConfigTestDB(t, stored, nil)
			candidate := &runtimeChatGenerator{}
			factoryCalls := 0
			s := &Server{
				db: db,
				newChatGenerator: func(cfg llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
					factoryCalls++
					if cfg.Provider != modelconfig.ProviderAnthropicCompatible || cfg.Timeout != 47*time.Second {
						t.Fatalf("unexpected startup config: %+v", cfg)
					}
					return candidate, nil
				},
			}

			s.applyStoredModelConfig()

			if factoryCalls != tt.wantFactory {
				t.Fatalf("factory calls=%d want=%d", factoryCalls, tt.wantFactory)
			}
			if (s.generator() == candidate) != tt.wantGenerator {
				t.Fatalf("unexpected startup generator: %T", s.generator())
			}
			if tt.wantGenerator && s.chatRequestTimeout() != 47*time.Second {
				t.Fatalf("expected stored compatible timeout, got %s", s.chatRequestTimeout())
			}
		})
	}
}

func TestModelConfigSaveIsAtomicAcrossProbePersistenceAndRuntimeSwap(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "old-secret",
		Model:          "old-model",
		TimeoutSeconds: 20,
	}}

	tests := []struct {
		name       string
		ping       llm.PingResult
		execErr    error
		wantStatus int
		wantSwap   bool
		wantWrite  bool
	}{
		{name: "candidate probe fails", ping: llm.PingResult{Message: "unauthorized"}, wantStatus: http.StatusBadRequest},
		{name: "database write fails", ping: llm.PingResult{OK: true}, execErr: errors.New("write failed"), wantStatus: http.StatusInternalServerError},
		{name: "success", ping: llm.PingResult{OK: true}, wantStatus: http.StatusOK, wantSwap: true, wantWrite: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, state := openAtomicModelConfigTestDB(t, stored, tt.execErr)
			old := &runtimeChatGenerator{}
			candidate := &runtimeChatGenerator{ping: tt.ping}
			factoryCalls := 0
			s := &Server{
				db:          db,
				ragGen:      old,
				chatTimeout: 20 * time.Second,
				newChatGenerator: func(cfg llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
					factoryCalls++
					if cfg.Model != "new-model" || cfg.APIKey != "old-secret" {
						t.Fatalf("unexpected merged candidate config: %+v", cfg)
					}
					return candidate, nil
				},
			}
			payload := `{"chat":{"provider":"openai-compatible","apiBase":"https://api.openai.com/v1","apiKey":"","model":"new-model","timeoutSeconds":41}}`
			response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", payload)
			if response.Code != tt.wantStatus {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if factoryCalls != 1 || candidate.pingCalls != 1 {
				t.Fatalf("expected one factory/probe call, got factory=%d ping=%d", factoryCalls, candidate.pingCalls)
			}
			if got := s.generator(); (got == candidate) != tt.wantSwap {
				t.Fatalf("unexpected runtime swap: got=%T wantSwap=%v", got, tt.wantSwap)
			}
			if !tt.wantSwap && s.generator() != old {
				t.Fatal("failed save changed the previous live generator")
			}
			if state.writeCount != boolInt(tt.wantWrite) {
				t.Fatalf("writeCount=%d wantWrite=%v", state.writeCount, tt.wantWrite)
			}
			persisted, _, err := modelconfig.ReadStore(context.Background(), db)
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantWrite && persisted.Chat.Model != "new-model" {
				t.Fatalf("expected persisted candidate, got %+v", persisted.Chat)
			}
			if !tt.wantWrite && persisted.Chat.Model != "old-model" {
				t.Fatalf("failed save changed stored config: %+v", persisted.Chat)
			}
			if tt.wantSwap && s.chatRequestTimeout() != 41*time.Second {
				t.Fatalf("expected configured timeout after swap, got %s", s.chatRequestTimeout())
			}
		})
	}
}

func TestModelConfigSaveSkipsChatProbeForUnrelatedChanges(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "secret",
		Model:          "gpt-model",
		TimeoutSeconds: 30,
	}}
	db, state := openAtomicModelConfigTestDB(t, stored, nil)
	factoryCalls := 0
	s := &Server{
		db: db,
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			factoryCalls++
			return nil, errors.New("must not be called")
		},
	}
	payload := `{"video":{"apiBase":"https://video.example.com/v1","apiKey":"video-key","model":"video-model"}}`
	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", payload)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if factoryCalls != 0 {
		t.Fatalf("unrelated save invoked chat factory %d times", factoryCalls)
	}
	if state.writeCount != 1 {
		t.Fatalf("expected unrelated config persistence, writes=%d", state.writeCount)
	}
}

func TestModelConfigUnrelatedSaveIgnoresUnsafeUnconfiguredChatSection(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		APIBase: "http://127.0.0.1:8080/v1",
		APIKey:  "legacy-secret",
		Model:   "legacy-model",
	}}
	db, state := openAtomicModelConfigTestDB(t, stored, nil)
	factoryCalls := 0
	s := &Server{
		db: db,
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			factoryCalls++
			return nil, errors.New("must not be called")
		},
	}
	payload := `{"video":{"apiBase":"https://video.example.com/v1","apiKey":"video-key","model":"video-model"}}`

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", payload)

	if response.Code != http.StatusOK {
		t.Fatalf("unrelated save was blocked by unconfigured chat: status=%d body=%s", response.Code, response.Body.String())
	}
	if factoryCalls != 0 || state.writeCount != 1 {
		t.Fatalf("expected no chat build and one persistence, factory=%d writes=%d", factoryCalls, state.writeCount)
	}
}

func TestModelConfigReenablingAssistBuildsAndProbesChat(t *testing.T) {
	disabled := false
	stored := modelconfig.Config{
		Chat: modelconfig.ChatConfig{
			Provider:       modelconfig.ProviderOpenAICompatible,
			APIBase:        "https://api.openai.com/v1",
			APIKey:         "secret",
			Model:          "gpt-model",
			TimeoutSeconds: 30,
		},
		Assist: modelconfig.AssistConfig{Enabled: &disabled},
	}
	db, state := openAtomicModelConfigTestDB(t, stored, nil)
	candidate := &runtimeChatGenerator{ping: llm.PingResult{OK: true}}
	factoryCalls := 0
	s := &Server{
		db: db,
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			factoryCalls++
			return candidate, nil
		},
	}
	payload := `{"chat":{"provider":"openai-compatible","apiBase":"https://api.openai.com/v1","apiKey":"","model":"gpt-model","timeoutSeconds":30},"assist":{"enabled":true}}`

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", payload)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if factoryCalls != 1 || candidate.pingCalls != 1 || s.generator() != candidate || state.writeCount != 1 {
		t.Fatalf("expected build/probe/persist/swap, factory=%d ping=%d writes=%d generator=%T", factoryCalls, candidate.pingCalls, state.writeCount, s.generator())
	}
}

func TestModelConfigProviderChangeRequiresNewAPIKey(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "old-secret",
		Model:          "gpt-model",
		TimeoutSeconds: 20,
	}}
	db, state := openAtomicModelConfigTestDB(t, stored, nil)
	old := &runtimeChatGenerator{}
	s := &Server{db: db, ragGen: old}
	payload := `{"chat":{"provider":"anthropic-compatible","apiBase":"https://api.anthropic.com/v1","apiKey":"","model":"claude-model","timeoutSeconds":20}}`

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", payload)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected missing key rejection, got %d body=%s", response.Code, response.Body.String())
	}
	if state.writeCount != 0 || s.generator() != old {
		t.Fatal("rejected provider change modified persistence or runtime")
	}
}

func TestTestChatModelAndPolishPromptUseProviderNeutralCapability(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderAnthropicCompatible,
		APIBase:        "https://api.anthropic.com/v1",
		APIKey:         "secret",
		Model:          "claude-model",
		TimeoutSeconds: 30,
	}}
	db, _ := openAtomicModelConfigTestDB(t, stored, nil)
	gen := &runtimeChatGenerator{ping: llm.PingResult{OK: true}}
	factoryCalls := 0
	s := &Server{
		db:     db,
		ragGen: gen,
		newChatGenerator: func(cfg llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			factoryCalls++
			if cfg.Provider != modelconfig.ProviderAnthropicCompatible {
				t.Fatalf("unexpected provider: %q", cfg.Provider)
			}
			return gen, nil
		},
	}

	testResponse := performRawUnit(http.HandlerFunc(s.testChatModel), http.MethodPost, "/api/model-config/test-chat", `{}`)
	if testResponse.Code != http.StatusOK || factoryCalls != 1 || gen.pingCalls != 1 {
		t.Fatalf("test chat did not use shared factory: status=%d factory=%d ping=%d body=%s", testResponse.Code, factoryCalls, gen.pingCalls, testResponse.Body.String())
	}
	polishResponse := performRawUnit(http.HandlerFunc(s.polishPrompt), http.MethodPost, "/api/video/assets/polish-prompt", `{"prompt":"一只猫跑动","kind":"video"}`)
	if polishResponse.Code != http.StatusOK || gen.draft != "一只猫跑动" || gen.kind != "video" {
		t.Fatalf("provider-neutral polish lost draft/kind: status=%d draft=%q kind=%q body=%s", polishResponse.Code, gen.draft, gen.kind, polishResponse.Body.String())
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

const atomicModelConfigDriverName = "nine_xing_atomic_model_config"

var (
	registerAtomicModelConfigDriverOnce sync.Once
	atomicModelConfigStatesMu           sync.Mutex
	atomicModelConfigStates             = map[string]*atomicModelConfigState{}
)

type atomicModelConfigState struct {
	mu         sync.Mutex
	raw        []byte
	execErr    error
	writeCount int
}

type atomicModelConfigDriver struct{}
type atomicModelConfigConn struct{ state *atomicModelConfigState }

func (atomicModelConfigDriver) Open(name string) (driver.Conn, error) {
	atomicModelConfigStatesMu.Lock()
	defer atomicModelConfigStatesMu.Unlock()
	state := atomicModelConfigStates[name]
	if state == nil {
		return nil, errors.New("unknown atomic model config test state")
	}
	return &atomicModelConfigConn{state: state}, nil
}
func (*atomicModelConfigConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}
func (*atomicModelConfigConn) Close() error { return nil }
func (*atomicModelConfigConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}
func (c *atomicModelConfigConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	return &modelConfigViewTestRows{raw: append([]byte(nil), c.state.raw...)}, nil
}
func (c *atomicModelConfigConn) ExecContext(_ context.Context, _ string, args []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.execErr != nil {
		return nil, c.state.execErr
	}
	if len(args) < 2 {
		return nil, errors.New("missing config argument")
	}
	raw, ok := args[1].Value.(string)
	if !ok {
		return nil, errors.New("config argument is not a string")
	}
	c.state.raw = []byte(raw)
	c.state.writeCount++
	return driver.RowsAffected(1), nil
}

func openAtomicModelConfigTestDB(t *testing.T, stored modelconfig.Config, execErr error) (*sql.DB, *atomicModelConfigState) {
	t.Helper()
	registerAtomicModelConfigDriverOnce.Do(func() { sql.Register(atomicModelConfigDriverName, atomicModelConfigDriver{}) })
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	name := t.Name() + time.Now().Format("150405.000000000")
	state := &atomicModelConfigState{raw: raw, execErr: execErr}
	atomicModelConfigStatesMu.Lock()
	atomicModelConfigStates[name] = state
	atomicModelConfigStatesMu.Unlock()
	db, err := sql.Open(atomicModelConfigDriverName, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		atomicModelConfigStatesMu.Lock()
		delete(atomicModelConfigStates, name)
		atomicModelConfigStatesMu.Unlock()
	})
	return db, state
}
