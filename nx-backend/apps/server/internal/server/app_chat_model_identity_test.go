package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
)

const appChatIdentityQuestion = "以后回答简短一点，你是什么模型？"

func TestAppChatAskModelIdentitySkipsPromptDependenciesAndPersistsFixedPair(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.cardID = 77
	generator := &identityDependencyGenerator{}
	preferences := &identityDependencyPreferenceStore{}
	documents := &identityDependencyRAGStore{}
	database, queries := openIdentityDependencyDB(t)
	s := newAppChatStreamServer(store, generator)
	s.chatLimiter = newFixedWindowRateLimiter(100, time.Minute)
	s.userPreferences = preferences
	s.ragDocs = documents
	s.db = database

	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask", strings.NewReader(fmt.Sprintf(`{"question":%q}`, appChatIdentityQuestion)))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: 7}))

	s.appChatRouter(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data askResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	assertIdentityAnswer(t, payload.Data.Answer)
	assertIdentityPairSaved(t, store)
	assertIdentityDependenciesSkipped(t, preferences, documents, queries, store, generator)
}

func TestAppChatAskStreamModelIdentitySkipsPromptDependenciesAndUsesNormalSSEPersistence(t *testing.T) {
	store := newFakeAppChatStreamStore()
	store.cardID = 77
	generator := &identityDependencyGenerator{}
	preferences := &identityDependencyPreferenceStore{}
	documents := &identityDependencyRAGStore{}
	database, queries := openIdentityDependencyDB(t)
	s := newAppChatStreamServer(store, generator)
	s.userPreferences = preferences
	s.ragDocs = documents
	s.db = database
	writer := newAppChatBlockingStreamWriter()
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/ask/stream", strings.NewReader(fmt.Sprintf(`{"question":%q}`, appChatIdentityQuestion)))
	req = req.WithContext(context.WithValue(req.Context(), appContextKey{}, auth.UserInfo{ID: 7}))

	s.appChatRouter(writer, req)

	body := writer.BodyString()
	if strings.Count(body, "event: delta\n") != 1 || !strings.Contains(body, `"content":"`+rag.ModelIdentityReply+`"`) {
		t.Fatalf("identity stream delta mismatch: %q", body)
	}
	if strings.Count(body, "event: done\n") != 1 || strings.Contains(body, "event: error\n") || !strings.Contains(body, `"answer":"`+rag.ModelIdentityReply+`"`) {
		t.Fatalf("identity stream terminal mismatch: %q", body)
	}
	assertIdentityPairSaved(t, store)
	assertIdentityDependenciesSkipped(t, preferences, documents, queries, store, generator)
}

func TestVoiceChatModelIdentitySkipsPromptDependenciesAndPersistsFixedPair(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"text": appChatIdentityQuestion})
	}))
	defer upstream.Close()
	previousClientFactory := newASRHTTPClient
	newASRHTTPClient = func(timeout time.Duration) *http.Client {
		client := upstream.Client()
		client.Timeout = timeout
		return client
	}
	t.Cleanup(func() { newASRHTTPClient = previousClientFactory })

	store := &fakeVoiceChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 77
	generator := &identityDependencyGenerator{}
	preferences := &identityDependencyPreferenceStore{}
	documents := &identityDependencyRAGStore{}
	database, queries := openIdentityDependencyDB(t)
	s := newVoiceChatTestServer(store, generator)
	s.env.ASR = config.ASRConfig{APIBase: upstream.URL, APIKey: "test-key", Model: "whisper-1", TimeoutSeconds: 3}
	s.userPreferences = preferences
	s.ragDocs = documents
	s.db = database
	var profileCalls atomic.Int32
	s.appChatProfilesForCardOverride = func(context.Context, int64, int64) (rag.UserProfile, rag.ConversationCard) {
		profileCalls.Add(1)
		return rag.UserProfile{}, rag.ConversationCard{}
	}
	s.voiceAssetCreate = func(_ context.Context, _ uploadasset.CreateInput) (uploadasset.Asset, error) {
		return uploadasset.Asset{ID: 88}, nil
	}
	body, contentType := voiceChatMultipartBody(t, "voice.aac", "audio/aac", "audio", "2100")
	req := httptest.NewRequest(http.MethodPost, "/api/app/chat/sessions/42/voice", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	response := httptest.NewRecorder()

	s.appChatRouter(response, req)

	if response.Code != http.StatusOK {
		t.Fatalf("voice status = %d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Answer rag.Answer `json:"answer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	assertIdentityAnswer(t, payload.Data.Answer)
	if store.transcript != appChatIdentityQuestion || store.assistantAnswer != rag.ModelIdentityReply || string(store.assistantSources) != "[]" {
		t.Fatalf("voice identity pair mismatch: transcript=%q answer=%q sources=%s", store.transcript, store.assistantAnswer, store.assistantSources)
	}
	assertIdentityDependenciesSkipped(t, preferences, documents, queries, store.fakeAppChatStreamStore, generator)
	if profileCalls.Load() != 0 {
		t.Fatalf("identity voice loaded profile %d times", profileCalls.Load())
	}
}

func assertIdentityAnswer(t *testing.T, answer rag.Answer) {
	t.Helper()
	if answer.Answer != rag.ModelIdentityReply || len(answer.Sources) != 0 || len(answer.Suggestions) != 0 {
		t.Fatalf("identity answer mismatch: %+v", answer)
	}
}

func assertIdentityPairSaved(t *testing.T, store *fakeAppChatStreamStore) {
	t.Helper()
	messages, sources := store.savedMessagesAndSources()
	if store.saveCallCount() != 1 || len(messages) != 2 {
		t.Fatalf("identity pair save mismatch: calls=%d messages=%+v", store.saveCallCount(), messages)
	}
	if messages[0].Role != "user" || messages[0].Content != appChatIdentityQuestion || messages[1].Role != "assistant" || messages[1].Content != rag.ModelIdentityReply {
		t.Fatalf("identity pair content mismatch: %+v", messages)
	}
	if string(sources) != "[]" {
		t.Fatalf("identity persisted sources = %s, want []", sources)
	}
}

func assertIdentityDependenciesSkipped(t *testing.T, preferences *identityDependencyPreferenceStore, documents *identityDependencyRAGStore, queries *identityDependencyQueryRecorder, store *fakeAppChatStreamStore, generator *identityDependencyGenerator) {
	t.Helper()
	if preferences.listCalls.Load() != 0 || preferences.applyCalls.Load() != 0 {
		t.Fatalf("identity path touched preferences: list=%d apply=%d", preferences.listCalls.Load(), preferences.applyCalls.Load())
	}
	if documents.calls.Load() != 0 {
		t.Fatalf("identity path retrieved RAG documents %d times", documents.calls.Load())
	}
	if got := queries.promptDependencyQueries(); len(got) != 0 {
		t.Fatalf("identity path queried prompt dependencies: %+v", got)
	}
	if store.contextCallCount() != 0 {
		t.Fatalf("identity path loaded conversation history %d times", store.contextCallCount())
	}
	if generator.generateCalls.Load() != 0 || generator.streamCalls.Load() != 0 {
		t.Fatalf("identity path called generator: generate=%d stream=%d", generator.generateCalls.Load(), generator.streamCalls.Load())
	}
}

type identityDependencyPreferenceStore struct {
	listCalls  atomic.Int32
	applyCalls atomic.Int32
}

func (s *identityDependencyPreferenceStore) List(context.Context, int64) ([]userpreference.Preference, error) {
	s.listCalls.Add(1)
	return nil, errors.New("preference read should be skipped")
}

func (s *identityDependencyPreferenceStore) Apply(context.Context, int64, []userpreference.Mutation) error {
	s.applyCalls.Add(1)
	return errors.New("preference write should be skipped")
}

type identityDependencyRAGStore struct {
	ragDocumentStore
	calls atomic.Int32
}

func (s *identityDependencyRAGStore) EnabledDocuments(context.Context) ([]rag.Document, error) {
	s.calls.Add(1)
	return nil, errors.New("RAG retrieval should be skipped")
}

type identityDependencyGenerator struct {
	generateCalls atomic.Int32
	streamCalls   atomic.Int32
}

func (g *identityDependencyGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	g.generateCalls.Add(1)
	return "", errors.New("generation should be skipped")
}

func (g *identityDependencyGenerator) GenerateStream(context.Context, rag.GenerateInput, rag.StreamEmitter) (string, error) {
	g.streamCalls.Add(1)
	return "", errors.New("stream generation should be skipped")
}

type identityDependencyQueryRecorder struct {
	mu      sync.Mutex
	queries []string
}

func (r *identityDependencyQueryRecorder) add(query string) {
	r.mu.Lock()
	r.queries = append(r.queries, query)
	r.mu.Unlock()
}

func (r *identityDependencyQueryRecorder) promptDependencyQueries() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []string
	for _, query := range r.queries {
		if strings.Contains(query, "site_configs") || strings.Contains(query, "app_memories") {
			result = append(result, query)
		}
	}
	return result
}

func openIdentityDependencyDB(t *testing.T) (*sql.DB, *identityDependencyQueryRecorder) {
	t.Helper()
	recorder := &identityDependencyQueryRecorder{}
	database := sql.OpenDB(identityDependencyConnector{recorder: recorder})
	t.Cleanup(func() { _ = database.Close() })
	return database, recorder
}

type identityDependencyConnector struct {
	recorder *identityDependencyQueryRecorder
}

func (c identityDependencyConnector) Connect(context.Context) (driver.Conn, error) {
	return identityDependencyConn{recorder: c.recorder}, nil
}

func (identityDependencyConnector) Driver() driver.Driver { return identityDependencyDriver{} }

type identityDependencyDriver struct{}

func (identityDependencyDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("use connector")
}

type identityDependencyConn struct {
	recorder *identityDependencyQueryRecorder
}

func (identityDependencyConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (identityDependencyConn) Close() error                        { return nil }
func (identityDependencyConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c identityDependencyConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.recorder.add(query)
	if strings.Contains(query, "site_configs") {
		return identityDependencyNoRows{}, nil
	}
	return nil, errors.New("prompt dependency query should be skipped")
}

func (identityDependencyConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

type identityDependencyNoRows struct{}

func (identityDependencyNoRows) Columns() []string         { return []string{"config"} }
func (identityDependencyNoRows) Close() error              { return nil }
func (identityDependencyNoRows) Next([]driver.Value) error { return io.EOF }
