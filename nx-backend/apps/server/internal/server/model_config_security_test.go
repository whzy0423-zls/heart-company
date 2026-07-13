package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
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
	ping        llm.PingResult
	pingCalls   int
	pingStarted chan struct{}
	pingRelease <-chan struct{}
	pingFunc    func(context.Context) llm.PingResult
	draft       string
	kind        string
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
func (g *runtimeChatGenerator) Ping(ctx context.Context) llm.PingResult {
	if g.pingFunc != nil {
		g.pingCalls++
		return g.pingFunc(ctx)
	}
	g.pingCalls++
	if g.pingStarted != nil {
		select {
		case <-g.pingStarted:
		default:
			close(g.pingStarted)
		}
	}
	if g.pingRelease != nil {
		<-g.pingRelease
	}
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
	db := openModelConfigViewTestDB(t, `{"chat":{"apiBase":"http://127.0.0.1:8080/v1","apiKey":"old","model":"MiniMax-M2.7"},"video":{"apiBase":"https://video.example.com/v1","apiKey":"video-key","model":"video-model"}}`)
	s := &Server{db: db, env: config.Env{MiniMax: config.MiniMaxConfig{APIKey: "env-secret"}}}

	s.applyStoredModelConfig()

	if got := s.generator(); got != nil {
		t.Fatalf("expected legacy stored chat to remain unconfigured, got %T", got)
	}
	if s.videoStore() == nil {
		t.Fatal("unsafe unconfigured legacy chat blocked valid non-chat startup config")
	}
}

func TestApplyStoredModelConfigLoadsValidChatWhenVideoSectionIsUnsafe(t *testing.T) {
	stored := modelconfig.Config{
		Chat: modelconfig.ChatConfig{
			Provider:       modelconfig.ProviderOpenAICompatible,
			APIBase:        "https://api.openai.com/v1",
			APIKey:         "secret",
			Model:          "gpt-model",
			TimeoutSeconds: 30,
		},
		Video: modelconfig.VideoConfig{APIBase: "http://127.0.0.1:8080/v1", APIKey: "unsafe", Model: "unsafe-video"},
	}
	db, _ := openAtomicModelConfigTestDB(t, stored, nil)
	candidate := &runtimeChatGenerator{}
	s := &Server{
		db:  db,
		env: config.Env{Video: config.VideoConfig{APIBase: "https://safe-video.example/v1", APIKey: "env-video", Model: "env-video"}},
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			return candidate, nil
		},
	}

	s.applyStoredModelConfig()

	if s.generator() != candidate {
		t.Fatalf("unsafe video section blocked valid compatible chat startup: %T", s.generator())
	}
}

func TestStoredRuntimeSectionsFallbackIndependently(t *testing.T) {
	env := config.Env{
		Video:   config.VideoConfig{APIBase: "https://env-video.example/v1", APIKey: "env-video-key", Model: "env-video"},
		Image:   config.ImageConfig{APIBase: "https://env-image.example/v1", APIKey: "env-image-key", Model: "env-image"},
		MiniMax: config.MiniMaxConfig{APIBase: "https://api.minimaxi.com", APIKey: "env-analysis-key", GroupID: "env-group", Model: "MiniMax-M3"},
	}
	cfg := modelconfig.Config{
		Video:    modelconfig.VideoConfig{APIBase: "http://127.0.0.1:8080/v1", APIKey: "bad-video", Model: "bad-video"},
		Image:    modelconfig.ImageConfig{APIBase: "https://stored-image.example/v1", APIKey: "stored-image-key", Model: "stored-image"},
		Analysis: modelconfig.AnalysisConfig{APIBase: "https://analysis.example/v1", Model: "MiniMax-M3-Preview"},
	}

	videoCfg := safeStoredVideoConfig(cfg, env.Video)
	imageCfg := safeStoredImageConfig(cfg, env.Image)
	analysisCfg := safeStoredAnalysisConfig(cfg, env.MiniMax)
	if videoCfg != env.Video {
		t.Fatalf("unsafe video did not fall back to env: %+v", videoCfg)
	}
	if imageCfg.Model != "stored-image" || imageCfg.APIKey != "stored-image-key" {
		t.Fatalf("valid image section was not applied: %+v", imageCfg)
	}
	if analysisCfg.Model != "MiniMax-M3-Preview" {
		t.Fatalf("valid analysis section was not applied: %+v", analysisCfg)
	}

	cfg.Video = modelconfig.VideoConfig{APIBase: "https://stored-video.example/v1", APIKey: "stored-video-key", Model: "stored-video"}
	cfg.Image = modelconfig.ImageConfig{APIBase: "http://127.0.0.1:8080/v1", APIKey: "bad-image", Model: "bad-image"}
	cfg.Analysis = modelconfig.AnalysisConfig{APIBase: "http://127.0.0.1:8080/v1", Model: "MiniMax-M3-Preview"}
	videoCfg = safeStoredVideoConfig(cfg, env.Video)
	imageCfg = safeStoredImageConfig(cfg, env.Image)
	analysisCfg = safeStoredAnalysisConfig(cfg, env.MiniMax)
	if videoCfg.Model != "stored-video" || videoCfg.APIKey != "stored-video-key" {
		t.Fatalf("valid video section was not applied: %+v", videoCfg)
	}
	if imageCfg != env.Image {
		t.Fatalf("unsafe image did not fall back to env: %+v", imageCfg)
	}
	wantAnalysis := modelconfig.Config{}.ApplyAnalysis(env.MiniMax)
	if analysisCfg != wantAnalysis {
		t.Fatalf("unsafe analysis did not fall back to env baseline: got=%+v want=%+v", analysisCfg, wantAnalysis)
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

func TestModelConfigSaveCapsBlockingProbeWhileHoldingUpdateLock(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "old-secret",
		Model:          "old-model",
		TimeoutSeconds: 20,
	}}
	db, state := openAtomicModelConfigTestDB(t, stored, nil)
	old := &runtimeChatGenerator{}
	observedDeadline := make(chan time.Duration, 1)
	candidate := &runtimeChatGenerator{pingFunc: func(ctx context.Context) llm.PingResult {
		deadline, ok := ctx.Deadline()
		if !ok {
			observedDeadline <- 0
		} else {
			observedDeadline <- time.Until(deadline)
		}
		<-ctx.Done()
		return llm.PingResult{Message: ctx.Err().Error()}
	}}
	s := &Server{
		db:                      db,
		ragGen:                  old,
		modelConfigProbeTimeout: 35 * time.Millisecond,
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			return candidate, nil
		},
	}

	started := time.Now()
	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"chat":{"provider":"openai-compatible","apiBase":"https://api.openai.com/v1","apiKey":"","model":"new-model","timeoutSeconds":300}}`)
	elapsed := time.Since(started)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected timed out probe rejection, status=%d body=%s", response.Code, response.Body.String())
	}
	if elapsed > 300*time.Millisecond {
		t.Fatalf("probe was not capped: elapsed=%s", elapsed)
	}
	if deadline := <-observedDeadline; deadline <= 0 || deadline > 70*time.Millisecond {
		t.Fatalf("candidate observed uncapped deadline: %s", deadline)
	}
	if state.writeCount != 0 || s.generator() != old {
		t.Fatal("timed out probe changed persistence or runtime")
	}
}

func TestModelConfigSwapsRuntimeBeforeAuditRecording(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "secret",
		Model:          "old-model",
		TimeoutSeconds: 20,
	}}
	db, state := openAtomicModelConfigTestDB(t, stored, nil)
	state.auditStarted = make(chan struct{})
	state.auditRelease = make(chan struct{})
	candidate := &runtimeChatGenerator{ping: llm.PingResult{OK: true}}
	s := &Server{
		db:        db,
		auditLogs: auditlog.NewStore(db),
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			return candidate, nil
		},
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"chat":{"provider":"openai-compatible","apiBase":"https://api.openai.com/v1","apiKey":"","model":"new-model","timeoutSeconds":20}}`)
	}()

	select {
	case <-state.auditStarted:
	case <-time.After(time.Second):
		t.Fatal("audit recording did not start")
	}
	if s.generator() != candidate {
		t.Fatalf("runtime was not swapped before audit: %T", s.generator())
	}
	persisted, _, err := modelconfig.ReadStore(context.Background(), db)
	if err != nil || persisted.Chat.Model != "new-model" {
		t.Fatalf("config was not persisted before audit: cfg=%+v err=%v", persisted.Chat, err)
	}
	close(state.auditRelease)
	select {
	case response := <-done:
		if response.Code != http.StatusOK {
			t.Fatalf("save failed after audit release: %d %s", response.Code, response.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("save did not finish after audit release")
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

func TestModelConfigChatOnlySaveIgnoresAndDoesNotRebuildOmittedUnsafeNonChatSections(t *testing.T) {
	stored := modelconfig.Config{
		Chat:      modelconfig.ChatConfig{Provider: modelconfig.ProviderOpenAICompatible, APIBase: "https://api.openai.com/v1", APIKey: "secret", Model: "old-model", TimeoutSeconds: 20},
		Video:     modelconfig.VideoConfig{APIBase: "http://127.0.0.1:8080/v1", APIKey: "legacy-video", Model: "legacy-video"},
		Image:     modelconfig.ImageConfig{APIBase: "http://127.0.0.1:8081/v1", APIKey: "legacy-image", Model: "legacy-image"},
		Analysis:  modelconfig.AnalysisConfig{APIBase: "http://127.0.0.1:8082/v1", Model: "MiniMax-M3"},
		Admin:     modelconfig.CompatibleModelConfig{APIBase: "http://127.0.0.1:8083/v1"},
		DailyQuiz: modelconfig.CompatibleModelConfig{APIBase: "http://127.0.0.1:8084/v1"},
	}
	db, _ := openAtomicModelConfigTestDB(t, stored, nil)
	candidate := &runtimeChatGenerator{ping: llm.PingResult{OK: true}}
	s := &Server{db: db, newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) { return candidate, nil }}

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"chat":{"model":"new-model"}}`)

	if response.Code != http.StatusOK {
		t.Fatalf("chat-only save was blocked by omitted unsafe non-chat config: %d %s", response.Code, response.Body.String())
	}
	if s.generator() != candidate {
		t.Fatalf("chat runtime was not swapped: %T", s.generator())
	}
	if s.videoStore() != nil || s.imageStore() != nil || s.analysisGenerator() != nil {
		t.Fatal("omitted non-chat runtime sections were rebuilt")
	}
}

func TestModelConfigFullPageChatSaveIgnoresUnchangedUnsafeNonChatSections(t *testing.T) {
	stored := modelconfig.Config{
		Chat:      modelconfig.ChatConfig{Provider: modelconfig.ProviderOpenAICompatible, APIBase: "https://api.openai.com/v1", APIKey: "chat-key", Model: "old-model", TimeoutSeconds: 20},
		Video:     modelconfig.VideoConfig{APIBase: "http://127.0.0.1:8080/v1", APIKey: "video-key", Model: "legacy-video"},
		Image:     modelconfig.ImageConfig{APIBase: "http://127.0.0.1:8081/v1", APIKey: "image-key", Model: "legacy-image"},
		Analysis:  modelconfig.AnalysisConfig{APIBase: "http://127.0.0.1:8082/v1", APIKey: "analysis-key", GroupID: "analysis-group", Model: "MiniMax-M3"},
		Admin:     modelconfig.CompatibleModelConfig{Provider: "minimax", APIBase: "http://127.0.0.1:8083/v1", APIKey: "admin-key", GroupID: "admin-group", Model: "admin-model", TimeoutSeconds: 30},
		DailyQuiz: modelconfig.CompatibleModelConfig{Provider: modelconfig.ProviderOpenAICompatible, APIBase: "http://127.0.0.1:8084/v1", APIKey: "quiz-key", GroupID: "quiz-group", Model: "quiz-model", TimeoutSeconds: 30},
	}
	db, _ := openAtomicModelConfigTestDB(t, stored, nil)
	candidate := &runtimeChatGenerator{ping: llm.PingResult{OK: true}}
	s := &Server{db: db, newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) { return candidate, nil }}
	payload := `{
		"chat":{"provider":"openai-compatible","apiBase":"https://api.openai.com/v1","apiKey":"","model":"new-model","timeoutSeconds":20},
		"video":{"apiBase":"http://127.0.0.1:8080/v1","apiKey":"","model":"legacy-video"},
		"image":{"apiBase":"http://127.0.0.1:8081/v1","apiKey":"","model":"legacy-image"},
		"analysis":{"apiBase":"http://127.0.0.1:8082/v1","apiKey":"","groupId":"analysis-group","model":"MiniMax-M3"},
		"admin":{"provider":"minimax","apiBase":"http://127.0.0.1:8083/v1","apiKey":"","groupId":"admin-group","model":"admin-model","timeoutSeconds":30},
		"dailyQuiz":{"provider":"openai-compatible","apiBase":"http://127.0.0.1:8084/v1","apiKey":"","groupId":"quiz-group","model":"quiz-model","timeoutSeconds":30}
	}`

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", payload)

	if response.Code != http.StatusOK {
		t.Fatalf("full-page chat save was blocked by unchanged unsafe sections: %d %s", response.Code, response.Body.String())
	}
	if s.generator() != candidate || s.videoStore() != nil || s.imageStore() != nil || s.analysisGenerator() != nil {
		t.Fatalf("unexpected runtime activation/rebuild: chat=%T video=%v image=%v analysis=%v", s.generator(), s.videoStore(), s.imageStore(), s.analysisGenerator())
	}
}

func TestModelConfigPresentUnsafeNonChatSectionIsStillValidated(t *testing.T) {
	stored := modelconfig.Config{Video: modelconfig.VideoConfig{APIBase: "https://video.example.com/v1", APIKey: "video-key", Model: "video-model"}}
	db, _ := openAtomicModelConfigTestDB(t, stored, nil)
	s := &Server{db: db}

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"video":{"apiBase":"http://127.0.0.1:8080/v1","model":"video-model"}}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("present unsafe video section was not validated: %d %s", response.Code, response.Body.String())
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

func TestModelConfigDisabledAssistCanSavePromptWithoutConfiguredChat(t *testing.T) {
	disabled := false
	stored := modelconfig.Config{Assist: modelconfig.AssistConfig{Enabled: &disabled}}
	db, state := openAtomicModelConfigTestDB(t, stored, nil)
	factoryCalls := 0
	s := &Server{
		db: db,
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			factoryCalls++
			return nil, errors.New("must not be called")
		},
	}

	response := performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"assist":{"enabled":false,"systemPrompt":"保持温和简洁"}}`)

	if response.Code != http.StatusOK {
		t.Fatalf("disabled assist prompt save failed: status=%d body=%s", response.Code, response.Body.String())
	}
	if factoryCalls != 0 || state.writeCount != 1 || s.generator() != nil {
		t.Fatalf("disabled assist prompt save touched chat runtime: factory=%d writes=%d generator=%T", factoryCalls, state.writeCount, s.generator())
	}
	persisted, _, err := modelconfig.ReadStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Assist.SystemPrompt != "保持温和简洁" || persisted.Assist.Enabled == nil || *persisted.Assist.Enabled {
		t.Fatalf("unexpected persisted assist config: %+v", persisted.Assist)
	}
}

func TestChatRuntimeReturnsGeneratorAndTimeoutFromSameSwap(t *testing.T) {
	t.Parallel()

	first := &runtimeChatGenerator{}
	second := &runtimeChatGenerator{}
	s := &Server{ragGen: first, chatTimeout: 11 * time.Second}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			s.modelMu.Lock()
			if i%2 == 0 {
				s.ragGen = first
				s.chatTimeout = 11 * time.Second
			} else {
				s.ragGen = second
				s.chatTimeout = 22 * time.Second
			}
			s.modelMu.Unlock()
		}
	}()
	for i := 0; i < 2000; i++ {
		generator, timeout := s.chatRuntime()
		if (generator == first && timeout != 11*time.Second) || (generator == second && timeout != 22*time.Second) {
			t.Fatalf("observed torn runtime snapshot: generator=%T timeout=%s", generator, timeout)
		}
	}
	<-done
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
	stored := modelconfig.Config{
		Chat: modelconfig.ChatConfig{
			Provider:       modelconfig.ProviderAnthropicCompatible,
			APIBase:        "https://api.anthropic.com/v1",
			APIKey:         "secret",
			Model:          "claude-model",
			TimeoutSeconds: 30,
		},
		Video: modelconfig.VideoConfig{APIBase: "http://127.0.0.1:8080/v1"},
	}
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

func TestConcurrentModelConfigSavesPreserveBothUpdates(t *testing.T) {
	stored := modelconfig.Config{Chat: modelconfig.ChatConfig{
		Provider:       modelconfig.ProviderOpenAICompatible,
		APIBase:        "https://api.openai.com/v1",
		APIKey:         "secret",
		Model:          "old-model",
		TimeoutSeconds: 30,
	}}
	db, _ := openAtomicModelConfigTestDB(t, stored, nil)
	pingStarted := make(chan struct{})
	pingRelease := make(chan struct{})
	candidate := &runtimeChatGenerator{
		ping:        llm.PingResult{OK: true},
		pingStarted: pingStarted,
		pingRelease: pingRelease,
	}
	s := &Server{
		db: db,
		newChatGenerator: func(llm.ChatGeneratorConfig) (llm.ChatGenerator, error) {
			return candidate, nil
		},
	}

	chatDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		chatDone <- performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"chat":{"provider":"openai-compatible","apiBase":"https://api.openai.com/v1","apiKey":"","model":"new-model","timeoutSeconds":30}}`)
	}()
	select {
	case <-pingStarted:
	case <-time.After(time.Second):
		t.Fatal("chat save did not reach probe")
	}

	videoDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		videoDone <- performRawUnit(http.HandlerFunc(s.modelConfig), http.MethodPut, "/api/model-config", `{"video":{"apiBase":"https://video.example.com/v1","apiKey":"video-key","model":"video-model"}}`)
	}()
	var earlyVideo *httptest.ResponseRecorder
	select {
	case earlyVideo = <-videoDone:
		// Without update serialization the unrelated save can overtake the probe.
	case <-time.After(50 * time.Millisecond):
	}
	close(pingRelease)

	for _, done := range []chan *httptest.ResponseRecorder{chatDone} {
		select {
		case response := <-done:
			if response.Code != http.StatusOK {
				t.Fatalf("concurrent save failed: status=%d body=%s", response.Code, response.Body.String())
			}
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent save did not finish")
		}
	}
	if earlyVideo == nil {
		select {
		case earlyVideo = <-videoDone:
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent video save did not finish")
		}
	}
	if earlyVideo.Code != http.StatusOK {
		t.Fatalf("concurrent video save failed: status=%d body=%s", earlyVideo.Code, earlyVideo.Body.String())
	}

	persisted, _, err := modelconfig.ReadStore(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Chat.Model != "new-model" || persisted.Video.Model != "video-model" {
		t.Fatalf("concurrent saves lost an update: %+v", persisted)
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
	mu           sync.Mutex
	raw          []byte
	execErr      error
	writeCount   int
	auditStarted chan struct{}
	auditRelease chan struct{}
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
func (c *atomicModelConfigConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "admin_operation_logs") {
		c.state.mu.Lock()
		started := c.state.auditStarted
		release := c.state.auditRelease
		c.state.mu.Unlock()
		if started != nil {
			select {
			case <-started:
			default:
				close(started)
			}
		}
		if release != nil {
			<-release
		}
		return driver.RowsAffected(1), nil
	}
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
