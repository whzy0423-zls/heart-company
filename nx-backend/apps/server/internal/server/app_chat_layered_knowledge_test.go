package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
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

func TestAppChatLayeredKnowledgeRequestedTypeLimitsAndLegacyBoundary(t *testing.T) {
	explicitResolver := &layeredKnowledgeResolver{mainType: 6, revision: 1}
	explicitSearcher := newLayeredKnowledgeSearcher()
	explicitCoordinator := appknowledge.NewCoordinator(explicitResolver, explicitSearcher, explicitSearcher)
	result, err := explicitCoordinator.Retrieve(context.Background(), appknowledge.Input{
		UserID: 7, SessionID: 42, CardID: 77, Query: "1 2 3 4号分别是什么", RequestedTypes: []int{4, 1, 2, 3, 4, 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(explicitResolver.requestedTypes, []int{1, 2, 3, 4}) {
		t.Fatalf("requested types = %v, want [1 2 3 4]", explicitResolver.requestedTypes)
	}
	publicTopKs, releaseIDs, releaseTopKs := explicitSearcher.capturedCalls()
	sort.Slice(releaseIDs, func(i, j int) bool { return releaseIDs[i] < releaseIDs[j] })
	if !reflect.DeepEqual(publicTopKs, []int{6}) || !reflect.DeepEqual(releaseIDs, []int64{100, 201, 202, 203, 204}) {
		t.Fatalf("explicit calls public=%v releases=%v", publicTopKs, releaseIDs)
	}
	if !reflect.DeepEqual(releaseTopKs[100], []int{9}) {
		t.Fatalf("explicit theory topK = %v, want [9]", releaseTopKs[100])
	}
	for _, releaseID := range []int64{201, 202, 203, 204} {
		if !reflect.DeepEqual(releaseTopKs[releaseID], []int{3}) {
			t.Fatalf("explicit type release %d topK = %v, want [3]", releaseID, releaseTopKs[releaseID])
		}
	}
	for _, key := range []string{"enneagram_type_01", "enneagram_type_02", "enneagram_type_03", "enneagram_type_04"} {
		if _, ok := result.Trace.LayerHits[key]; !ok {
			t.Fatalf("explicit trace missing %s: %+v", key, result.Trace.LayerHits)
		}
	}
	if _, leaked := result.Trace.LayerHits["enneagram_type_06"]; leaked {
		t.Fatalf("current-card type leaked into explicit trace: %+v", result.Trace.LayerHits)
	}

	legacyResolver := &layeredKnowledgeResolver{mainType: 6, revision: 1}
	legacySearcher := newLayeredKnowledgeSearcher()
	legacyCoordinator := appknowledge.NewCoordinator(legacyResolver, legacySearcher, legacySearcher)
	_, err = legacyCoordinator.Retrieve(context.Background(), appknowledge.Input{
		UserID: 7, SessionID: 42, CardID: 77, Query: "最近关系压力很大", RequestedTypes: []int{0, 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	publicTopKs, releaseIDs, releaseTopKs = legacySearcher.capturedCalls()
	sort.Slice(releaseIDs, func(i, j int) bool { return releaseIDs[i] < releaseIDs[j] })
	if len(legacyResolver.requestedTypes) != 0 || !reflect.DeepEqual(publicTopKs, []int{12}) || !reflect.DeepEqual(releaseIDs, []int64{100, 206}) {
		t.Fatalf("legacy boundary requested=%v public=%v releases=%v", legacyResolver.requestedTypes, publicTopKs, releaseIDs)
	}
	if !reflect.DeepEqual(releaseTopKs[100], []int{9}) || !reflect.DeepEqual(releaseTopKs[206], []int{9}) {
		t.Fatalf("legacy release topKs theory=%v type=%v", releaseTopKs[100], releaseTopKs[206])
	}
}

type layeredKnowledgeResolver struct {
	mainType       int
	revision       int64
	calls          int
	lastUserID     int64
	lastSessionID  int64
	lastCardID     int64
	requestedTypes []int
}

func (r *layeredKnowledgeResolver) ResolveConversation(_ context.Context, userID, sessionID, cardID int64, requestedTypes []int) (appknowledge.ConversationResolution, error) {
	r.calls++
	r.lastUserID, r.lastSessionID, r.lastCardID = userID, sessionID, cardID
	r.requestedTypes = append([]int(nil), requestedTypes...)
	resolution := appknowledge.ConversationResolution{
		CardID: cardID, CardRevision: r.revision, MainType: r.mainType,
		Resolution: appknowledge.Resolution{Theory: &appknowledge.Binding{
			Layer: appknowledge.LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", ReleaseID: 100,
		}},
	}
	if len(requestedTypes) > 0 {
		for _, requestedType := range requestedTypes {
			typeValue := requestedType
			resolution.RequestedTypeBindings = append(resolution.RequestedTypeBindings, &appknowledge.Binding{
				Layer: appknowledge.LayerEnneagramType, EnneagramType: &typeValue,
				LibraryID: int64(10 + requestedType), LibraryKey: fmt.Sprintf("enneagram-type-%02d", requestedType),
				ReleaseID: int64(200 + requestedType),
			})
		}
	} else if r.mainType >= 1 && r.mainType <= 9 {
		typeValue := r.mainType
		resolution.EnneagramType = &appknowledge.Binding{
			Layer: appknowledge.LayerEnneagramType, EnneagramType: &typeValue,
			LibraryID: int64(10 + r.mainType), LibraryKey: fmt.Sprintf("enneagram-type-%02d", r.mainType),
			ReleaseID: int64(200 + r.mainType),
		}
	}
	return resolution, nil
}

type layeredKnowledgeSearcher struct {
	mu           sync.Mutex
	publicTopKs  []int
	releaseTopKs map[int64][]int
	releaseIDs   []int64
}

func newLayeredKnowledgeSearcher() *layeredKnowledgeSearcher {
	return &layeredKnowledgeSearcher{releaseTopKs: make(map[int64][]int)}
}

func (s *layeredKnowledgeSearcher) SearchPublic(_ context.Context, _ string, topK int) ([]rag.Document, error) {
	s.mu.Lock()
	s.publicTopKs = append(s.publicTopKs, topK)
	s.mu.Unlock()
	return []rag.Document{{ID: "public", Title: "公共支持", Content: layeredKnowledgeQuestion + " 先确认现实压力来源。"}}, nil
}

func (s *layeredKnowledgeSearcher) SearchReleaseChunks(_ context.Context, releaseID int64, _ string, topK int, _ float64) ([]rag.Document, error) {
	s.mu.Lock()
	s.releaseIDs = append(s.releaseIDs, releaseID)
	s.releaseTopKs[releaseID] = append(s.releaseTopKs[releaseID], topK)
	s.mu.Unlock()
	if releaseID == 100 {
		return []rag.Document{{ID: "theory", Title: "正式理论", Content: layeredKnowledgeQuestion + " 先区分动机、行为和防御模式。"}}, nil
	}
	typeValue := int(releaseID - 200)
	return []rag.Document{{
		ID: fmt.Sprintf("type-%d", typeValue), Title: fmt.Sprintf("%d号型号库", typeValue),
		Content: fmt.Sprintf("%s %d号使用当前型号特有的观察和成长建议。", layeredKnowledgeQuestion, typeValue), Tags: []string{fmt.Sprintf("type-%02d", typeValue)},
	}}, nil
}

func (s *layeredKnowledgeSearcher) capturedCalls() ([]int, []int64, map[int64][]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	publicTopKs := append([]int(nil), s.publicTopKs...)
	releaseIDs := append([]int64(nil), s.releaseIDs...)
	releaseTopKs := make(map[int64][]int, len(s.releaseTopKs))
	for releaseID, topKs := range s.releaseTopKs {
		releaseTopKs[releaseID] = append([]int(nil), topKs...)
	}
	return publicTopKs, releaseIDs, releaseTopKs
}

type layeredKnowledgeGenerator struct {
	mu      sync.Mutex
	sources []rag.Source
}

func (g *layeredKnowledgeGenerator) Generate(_ context.Context, input rag.GenerateInput) (string, error) {
	g.capture(input.Sources)
	return "已结合三层知识回答。", nil
}

func (g *layeredKnowledgeGenerator) GenerateStream(_ context.Context, input rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	g.capture(input.Sources)
	answer := "已结合三层知识回答。"
	if err := emit(answer); err != nil {
		return "", err
	}
	return answer, nil
}

func (g *layeredKnowledgeGenerator) capture(sources []rag.Source) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sources = append([]rag.Source(nil), sources...)
}

func (g *layeredKnowledgeGenerator) lastSources() []rag.Source {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]rag.Source(nil), g.sources...)
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
	t.Helper()
	body, err := json.Marshal(map[string]string{"question": question})
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
