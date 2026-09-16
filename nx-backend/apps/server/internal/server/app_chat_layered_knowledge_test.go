package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

const layeredKnowledgeQuestion = "工作压力很大时应该怎么办？"

func TestAppChatAskUsesLayeredKnowledgeAndPersistsInternalTrace(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 77
	resolver := &layeredKnowledgeResolver{mainType: 3, revision: 4}
	searcher := newLayeredKnowledgeSearcher()
	generator := &layeredKnowledgeGenerator{}
	server := newLayeredKnowledgeServer(t, store, resolver, searcher, generator)

	response := httptest.NewRecorder()
	request := layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask", layeredKnowledgeQuestion)
	server.appChatRouter(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if resolver.calls != 1 || resolver.lastUserID != 7 || resolver.lastSessionID != 42 || resolver.lastCardID != 77 {
		t.Fatalf("resolution calls=%d input=%d/%d/%d", resolver.calls, resolver.lastUserID, resolver.lastSessionID, resolver.lastCardID)
	}
	trace := store.singleTrace(t)
	if trace.CardID != 77 || trace.CardRevision != 4 || trace.EnneagramType == nil || *trace.EnneagramType != 3 {
		t.Fatalf("trace identity = %+v", trace)
	}
	assertLayeredTrace(t, trace.LayerHits, "type-3")
	assertSourceIDs(t, generator.lastSources(), "public", "theory", "type-3")
	if strings.Contains(response.Body.String(), "layer_hits") || strings.Contains(response.Body.String(), "card_revision") {
		t.Fatalf("internal trace leaked in response: %s", response.Body.String())
	}
}

func TestAppChatAskStreamUsesLayeredKnowledgeAndPersistsInternalTrace(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 77
	resolver := &layeredKnowledgeResolver{mainType: 6, revision: 8}
	searcher := newLayeredKnowledgeSearcher()
	generator := &layeredKnowledgeGenerator{}
	server := newLayeredKnowledgeServer(t, store, resolver, searcher, generator)
	writer := newAppChatBlockingStreamWriter()

	server.appChatRouter(writer, layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask/stream", layeredKnowledgeQuestion))

	body := writer.BodyString()
	if !strings.Contains(body, "event: done\n") || strings.Contains(body, "event: error\n") {
		t.Fatalf("stream body = %q", body)
	}
	trace := store.singleTrace(t)
	if trace.EnneagramType == nil || *trace.EnneagramType != 6 || trace.CardRevision != 8 {
		t.Fatalf("stream trace identity = %+v", trace)
	}
	assertLayeredTrace(t, trace.LayerHits, "type-6")
	assertSourceIDs(t, generator.lastSources(), "public", "theory", "type-6")
}

func TestAppChatAskSurfacesNonRetryableKnowledgeFailure(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 77
	resolver := &layeredKnowledgeResolver{mainType: 3, revision: 1}
	server := newLayeredKnowledgeServer(t, store, resolver, newLayeredKnowledgeSearcher(), &layeredKnowledgeGenerator{})
	server.appKnowledge = appknowledge.NewCoordinator(
		resolver,
		newLayeredKnowledgeSearcher(),
		newLayeredKnowledgeSearcher(),
		appknowledge.WithRemote("langchain", failingKnowledgeRetriever{err: &appknowledge.RemoteError{StatusCode: 422}}, nil),
	)

	response := httptest.NewRecorder()
	server.appChatRouter(response, layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask", layeredKnowledgeQuestion))

	if response.Code != http.StatusBadGateway || !strings.Contains(response.Body.String(), "知识检索失败") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if store.saveCallCount() != 0 {
		t.Fatalf("saved messages after knowledge failure: %d", store.saveCallCount())
	}
}

func TestAppChatAskStreamEmitsKnowledgeFailureWithoutDone(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 77
	resolver := &layeredKnowledgeResolver{mainType: 3, revision: 1}
	server := newLayeredKnowledgeServer(t, store, resolver, newLayeredKnowledgeSearcher(), &layeredKnowledgeGenerator{})
	server.appKnowledge = appknowledge.NewCoordinator(
		resolver,
		newLayeredKnowledgeSearcher(),
		newLayeredKnowledgeSearcher(),
		appknowledge.WithRemote("langchain", failingKnowledgeRetriever{err: &appknowledge.RemoteError{StatusCode: 422}}, nil),
	)
	writer := newAppChatBlockingStreamWriter()

	server.appChatRouter(writer, layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask/stream", layeredKnowledgeQuestion))

	body := writer.BodyString()
	if !strings.Contains(body, "event: error\n") || !strings.Contains(body, "知识检索失败") || strings.Contains(body, "event: done\n") {
		t.Fatalf("stream body=%q", body)
	}
	if store.saveCallCount() != 0 {
		t.Fatalf("saved messages after knowledge failure: %d", store.saveCallCount())
	}
}

type failingKnowledgeRetriever struct{ err error }

func (r failingKnowledgeRetriever) Retrieve(context.Context, appknowledge.RemoteRequest) (appknowledge.RemoteResult, error) {
	return appknowledge.RemoteResult{}, r.err
}

func TestAppChatTextEndpointsResolveAndPassRequestedTier(t *testing.T) {
	for _, path := range []string{
		"/api/app/chat/sessions/42/ask",
		"/api/app/chat/sessions/42/ask/stream",
	} {
		t.Run(path, func(t *testing.T) {
			store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
			generator := &layeredKnowledgeGenerator{}
			server := newLayeredKnowledgeServer(t, store, &layeredKnowledgeResolver{mainType: 3}, newLayeredKnowledgeSearcher(), generator)
			server.appChatPlanLoader = func(context.Context, int64) (appPlanConfig, error) {
				plan := defaultAppPlan("vip_month")
				plan.DeepChatEnabled = true
				return plan, nil
			}

			response := httptest.NewRecorder()
			request := layeredKnowledgeTierRequest(t, path, layeredKnowledgeQuestion, "deep")
			server.appChatRouter(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
			}
			if got := generator.lastTier(); got != "deep" {
				t.Fatalf("generator tier = %q, want deep", got)
			}
		})
	}
}

func TestAppChatTextEndpointsRejectMemberTierForFreeUser(t *testing.T) {
	for _, path := range []string{
		"/api/app/chat/sessions/42/ask",
		"/api/app/chat/sessions/42/ask/stream",
	} {
		t.Run(path, func(t *testing.T) {
			store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
			generator := &layeredKnowledgeGenerator{}
			server := newLayeredKnowledgeServer(t, store, &layeredKnowledgeResolver{mainType: 3}, newLayeredKnowledgeSearcher(), generator)
			server.appChatPlanLoader = func(context.Context, int64) (appPlanConfig, error) {
				return defaultAppPlan("free"), nil
			}

			response := httptest.NewRecorder()
			server.appChatRouter(response, layeredKnowledgeTierRequest(t, path, layeredKnowledgeQuestion, "deep"))

			if response.Code != http.StatusForbidden {
				t.Fatalf("status = %d body=%s, want 403", response.Code, response.Body.String())
			}
			if got := generator.lastTier(); got != "" {
				t.Fatalf("generator unexpectedly received tier %q", got)
			}
		})
	}
}

func TestAppChatTextEndpointsHonorDisabledMemberTierBenefit(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	generator := &layeredKnowledgeGenerator{}
	server := newLayeredKnowledgeServer(t, store, &layeredKnowledgeResolver{mainType: 3}, newLayeredKnowledgeSearcher(), generator)
	server.appChatPlanLoader = func(context.Context, int64) (appPlanConfig, error) {
		plan := defaultAppPlan("vip_month")
		plan.DeepChatEnabled = false
		return plan, nil
	}

	response := httptest.NewRecorder()
	server.appChatRouter(response, layeredKnowledgeTierRequest(t, "/api/app/chat/sessions/42/ask", layeredKnowledgeQuestion, "deep"))

	if response.Code != http.StatusForbidden || generator.lastTier() != "" {
		t.Fatalf("disabled deep benefit status=%d tier=%q body=%s", response.Code, generator.lastTier(), response.Body.String())
	}
}

func TestAppChatTextEndpointsRejectLongQuestionBeforeQuotaReservation(t *testing.T) {
	for _, path := range []string{
		"/api/app/chat/sessions/42/ask",
		"/api/app/chat/sessions/42/ask/stream",
	} {
		t.Run(path, func(t *testing.T) {
			store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
			generator := &layeredKnowledgeGenerator{}
			quota := &recordingAppChatQuotaManager{}
			server := newLayeredKnowledgeServer(t, store, &layeredKnowledgeResolver{mainType: 3}, newLayeredKnowledgeSearcher(), generator)
			server.appChatQuota = quota
			server.appChatPlanLoader = func(context.Context, int64) (appPlanConfig, error) {
				return defaultAppPlan("free"), nil
			}

			response := httptest.NewRecorder()
			server.appChatRouter(response, layeredKnowledgeTierRequest(t, path, strings.Repeat("问", 301), "basic"))

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "300") {
				t.Fatalf("status = %d body=%s, want 400 length error", response.Code, response.Body.String())
			}
			reserve, _, _, _, _ := quota.counts()
			if reserve != 0 || generator.lastTier() != "" || store.saveCallCount() != 0 {
				t.Fatalf("long question reached downstream: reserve=%d tier=%q saves=%d", reserve, generator.lastTier(), store.saveCallCount())
			}
		})
	}
}

func TestAppChatLayeredKnowledgeUsesLatestCardTypeAndDegradesWithoutValidType(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 77
	resolver := &layeredKnowledgeResolver{mainType: 2, revision: 1}
	searcher := newLayeredKnowledgeSearcher()
	generator := &layeredKnowledgeGenerator{}
	server := newLayeredKnowledgeServer(t, store, resolver, searcher, generator)

	server.appChatRouter(httptest.NewRecorder(), layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask", layeredKnowledgeQuestion))
	resolver.mainType = 8
	resolver.revision = 2
	server.appChatRouter(httptest.NewRecorder(), layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask", layeredKnowledgeQuestion))
	resolver.mainType = 0
	resolver.revision = 3
	server.appChatRouter(httptest.NewRecorder(), layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask", layeredKnowledgeQuestion))

	traces := store.allTraces()
	if len(traces) != 3 {
		t.Fatalf("trace count = %d", len(traces))
	}
	if traces[0].EnneagramType == nil || *traces[0].EnneagramType != 2 || traces[1].EnneagramType == nil || *traces[1].EnneagramType != 8 {
		t.Fatalf("updated type traces = %+v", traces)
	}
	if traces[2].EnneagramType != nil || traces[2].CardRevision != 3 {
		t.Fatalf("invalid type trace = %+v", traces[2])
	}
	var layerHits map[string]appknowledge.LayerHit
	if err := json.Unmarshal(traces[2].LayerHits, &layerHits); err != nil {
		t.Fatal(err)
	}
	if got := layerHits[appknowledge.LayerEnneagramType].ChunkIDs; len(got) != 0 {
		t.Fatalf("invalid type used type chunks: %v", got)
	}
	if len(layerHits[appknowledge.LayerPublic].ChunkIDs) == 0 || len(layerHits[appknowledge.LayerTheory].ChunkIDs) == 0 {
		t.Fatalf("invalid type lost shared layers: %+v", layerHits)
	}
}

func TestAppChatModelIdentitySkipsLayeredKnowledge(t *testing.T) {
	store := &layeredKnowledgeChatStore{fakeAppChatStreamStore: newFakeAppChatStreamStore()}
	store.cardID = 77
	resolver := &layeredKnowledgeResolver{mainType: 3, revision: 1}
	server := newLayeredKnowledgeServer(t, store, resolver, newLayeredKnowledgeSearcher(), &layeredKnowledgeGenerator{})

	response := httptest.NewRecorder()
	server.appChatRouter(response, layeredKnowledgeRequest(t, "/api/app/chat/sessions/42/ask", appChatIdentityQuestion))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	if resolver.calls != 0 {
		t.Fatalf("identity question resolved layered knowledge %d times", resolver.calls)
	}
	if len(store.allTraces()) != 0 || store.saveCallCount() != 1 {
		t.Fatalf("identity persistence trace=%d ordinary=%d", len(store.allTraces()), store.saveCallCount())
	}
}

type layeredKnowledgeResolver struct {
	mainType      int
	revision      int64
	calls         int
	lastUserID    int64
	lastSessionID int64
	lastCardID    int64
}

func (r *layeredKnowledgeResolver) ResolveConversation(_ context.Context, userID, sessionID, cardID int64) (appknowledge.ConversationResolution, error) {
	r.calls++
	r.lastUserID, r.lastSessionID, r.lastCardID = userID, sessionID, cardID
	resolution := appknowledge.ConversationResolution{
		CardID: cardID, CardRevision: r.revision, MainType: r.mainType,
		Resolution: appknowledge.Resolution{Theory: &appknowledge.Binding{
			Layer: appknowledge.LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", ReleaseID: 100,
		}},
	}
	if r.mainType >= 1 && r.mainType <= 9 {
		typeValue := r.mainType
		resolution.EnneagramType = &appknowledge.Binding{
			Layer: appknowledge.LayerEnneagramType, EnneagramType: &typeValue,
			LibraryID: int64(10 + r.mainType), LibraryKey: fmt.Sprintf("enneagram-type-%02d", r.mainType),
			ReleaseID: int64(200 + r.mainType),
		}
	}
	return resolution, nil
}

type layeredKnowledgeSearcher struct{}

func newLayeredKnowledgeSearcher() *layeredKnowledgeSearcher { return &layeredKnowledgeSearcher{} }

func (*layeredKnowledgeSearcher) SearchPublic(context.Context, string, int) ([]rag.Document, error) {
	return []rag.Document{{ID: "public", Title: "公共支持", Content: layeredKnowledgeQuestion + " 先确认现实压力来源。"}}, nil
}

func (*layeredKnowledgeSearcher) SearchReleaseChunks(_ context.Context, releaseID int64, _ string, _ int, _ float64) ([]rag.Document, error) {
	if releaseID == 100 {
		return []rag.Document{{ID: "theory", Title: "正式理论", Content: layeredKnowledgeQuestion + " 先区分动机、行为和防御模式。"}}, nil
	}
	typeValue := int(releaseID - 200)
	return []rag.Document{{
		ID: fmt.Sprintf("type-%d", typeValue), Title: fmt.Sprintf("%d号型号库", typeValue),
		Content: layeredKnowledgeQuestion + " 使用当前型号特有的观察和成长建议。", Tags: []string{fmt.Sprintf("type-%02d", typeValue)},
	}}, nil
}

type layeredKnowledgeGenerator struct {
	mu      sync.Mutex
	sources []rag.Source
	tier    string
}

func (g *layeredKnowledgeGenerator) Generate(_ context.Context, input rag.GenerateInput) (string, error) {
	g.capture(input.Sources, input.Tier)
	return "已结合三层知识回答。", nil
}

func (g *layeredKnowledgeGenerator) GenerateStream(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	g.capture(input.Sources, input.Tier)
	answer := "已结合三层知识回答。"
	if err := emit(answer); err != nil {
		return "", err
	}
	return answer, nil
}

func (g *layeredKnowledgeGenerator) capture(sources []rag.Source, tier string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sources = append([]rag.Source(nil), sources...)
	g.tier = tier
}

func (g *layeredKnowledgeGenerator) lastSources() []rag.Source {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]rag.Source(nil), g.sources...)
}

func (g *layeredKnowledgeGenerator) lastTier() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.tier
}

type layeredKnowledgeChatStore struct {
	*fakeAppChatStreamStore
	traceMu sync.Mutex
	traces  []chat.KnowledgeTrace
}

func (s *layeredKnowledgeChatStore) SavePairWithKnowledgeTrace(ctx context.Context, sessionID int64, question, answer string, sources json.RawMessage, trace chat.KnowledgeTrace) (int64, error) {
	s.traceMu.Lock()
	s.traces = append(s.traces, trace)
	s.traceMu.Unlock()
	return s.fakeAppChatStreamStore.SavePair(ctx, sessionID, question, answer, sources)
}

func (s *layeredKnowledgeChatStore) allTraces() []chat.KnowledgeTrace {
	s.traceMu.Lock()
	defer s.traceMu.Unlock()
	return append([]chat.KnowledgeTrace(nil), s.traces...)
}

func (s *layeredKnowledgeChatStore) singleTrace(t *testing.T) chat.KnowledgeTrace {
	t.Helper()
	traces := s.allTraces()
	if len(traces) != 1 {
		t.Fatalf("trace count = %d, want 1", len(traces))
	}
	return traces[0]
}

func newLayeredKnowledgeServer(t *testing.T, store appChatStore, resolver appknowledge.ConversationResolver, searcher *layeredKnowledgeSearcher, generator rag.Generator) *Server {
	t.Helper()
	server := newAppChatStreamServer(store, generator)
	database, _ := openIdentityDependencyDB(t)
	server.db = database
	server.chatLimiter = newFixedWindowRateLimiter(100, time.Minute)
	server.appKnowledge = appknowledge.NewCoordinator(resolver, searcher, searcher)
	server.appChatProfilesForCardOverride = func(_ context.Context, _, _ int64) (rag.UserProfile, rag.ConversationCard) {
		mainType := 0
		if current, ok := resolver.(*layeredKnowledgeResolver); ok {
			mainType = current.mainType
		}
		return rag.UserProfile{MainType: 9}, rag.ConversationCard{MainType: mainType}
	}
	return server
}

func layeredKnowledgeRequest(t *testing.T, path, question string) *http.Request {
	return layeredKnowledgeTierRequest(t, path, question, "")
}

func layeredKnowledgeTierRequest(t *testing.T, path, question, tier string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]string{"question": question, "tier": tier})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	return request.WithContext(contextWithAppUser(request.Context(), auth.UserInfo{ID: 7}))
}

func assertLayeredTrace(t *testing.T, raw json.RawMessage, typeChunkID string) {
	t.Helper()
	var hits map[string]appknowledge.LayerHit
	if err := json.Unmarshal(raw, &hits); err != nil {
		t.Fatal(err)
	}
	for layer, chunkID := range map[string]string{
		appknowledge.LayerPublic: "public", appknowledge.LayerTheory: "theory", appknowledge.LayerEnneagramType: typeChunkID,
	} {
		if got := hits[layer].ChunkIDs; len(got) != 1 || got[0] != chunkID {
			t.Fatalf("%s chunks = %v, want [%s]", layer, got, chunkID)
		}
	}
}

func assertSourceIDs(t *testing.T, sources []rag.Source, wanted ...string) {
	t.Helper()
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		seen[source.ID] = true
	}
	for _, id := range wanted {
		if !seen[id] {
			t.Fatalf("sources %v missing %q", sources, id)
		}
	}
}
