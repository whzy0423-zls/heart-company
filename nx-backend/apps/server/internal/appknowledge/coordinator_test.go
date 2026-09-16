package appknowledge

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

type coordinatorResolverStub struct {
	resolution ConversationResolution
	err        error
	input      Input
}

func (s *coordinatorResolverStub) ResolveConversation(_ context.Context, userID, sessionID, cardID int64) (ConversationResolution, error) {
	s.input = Input{UserID: userID, SessionID: sessionID, CardID: cardID}
	return s.resolution, s.err
}

type publicSearchStub struct {
	docs  []rag.Document
	err   error
	calls int
}

func (s *publicSearchStub) SearchPublic(_ context.Context, _ string, _ int) ([]rag.Document, error) {
	s.calls++
	return append([]rag.Document(nil), s.docs...), s.err
}

type releaseSearchStub struct {
	docsByRelease map[int64][]rag.Document
	errors        map[int64]error
	releaseIDs    []int64
}

type remoteRetrieverStub struct {
	result RemoteResult
	err    error
	input  RemoteRequest
	calls  int
	delay  time.Duration
}

func (s *remoteRetrieverStub) Retrieve(_ context.Context, input RemoteRequest) (RemoteResult, error) {
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	s.calls++
	s.input = input
	return s.result, s.err
}

func TestCoordinatorLangChainUsesResolvedReleaseScope(t *testing.T) {
	typeThree := 3
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 4, MainType: 3,
		Resolution: Resolution{
			Theory:        &Binding{Layer: LayerTheory, ReleaseID: 101},
			EnneagramType: &Binding{Layer: LayerEnneagramType, ReleaseID: 103, EnneagramType: &typeThree, LibraryKey: "enneagram-type-03"},
		},
	}}
	public := &publicSearchStub{}
	remote := &remoteRetrieverStub{result: RemoteResult{Documents: []rag.Document{{ID: "remote", Title: "远程", Content: "结果"}}, RetrievalMethod: "hybrid"}}
	coordinator := NewCoordinator(resolver, public, &releaseSearchStub{}, WithRemote("langchain", remote, nil))

	result, err := coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题", RequestID: "req-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(documentIDs(result.Documents), []string{"remote"}) || public.calls != 0 {
		t.Fatalf("result=%+v local calls=%d", result, public.calls)
	}
	if !reflect.DeepEqual(remote.input.TheoryReleaseIDs, []int64{101}) || !reflect.DeepEqual(remote.input.EnneagramReleaseIDs, []int64{103}) || remote.input.MainType != 3 {
		t.Fatalf("remote scope = %+v", remote.input)
	}
}

func TestCoordinatorGeneratesRemoteRequestIDWhenCallerHasNone(t *testing.T) {
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{CardID: 9, CardRevision: 1}}
	remote := &remoteRetrieverStub{result: RemoteResult{}}
	coordinator := NewCoordinator(resolver, &publicSearchStub{}, &releaseSearchStub{}, WithRemote("langchain", remote, nil))

	if _, err := coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"}); err != nil {
		t.Fatal(err)
	}
	if remote.input.RequestID == "" {
		t.Fatal("remote request ID must not be empty")
	}
}

func TestCoordinatorFallbackUsesLocalOnlyForRetryableRemoteErrors(t *testing.T) {
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{CardID: 9, CardRevision: 1}}
	public := &publicSearchStub{docs: []rag.Document{{ID: "local", Title: "本地", Content: "结果"}}}
	remote := &remoteRetrieverStub{err: &RemoteError{StatusCode: 503}}
	coordinator := NewCoordinator(resolver, public, &releaseSearchStub{}, WithRemote("fallback", remote, nil))

	result, err := coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})
	if err != nil || !reflect.DeepEqual(documentIDs(result.Documents), []string{"local"}) {
		t.Fatalf("expected local fallback, result=%+v err=%v", result, err)
	}

	remote.err = &RemoteError{StatusCode: 400}
	if _, err := coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"}); err == nil {
		t.Fatal("4xx contract errors must not silently fall back")
	}
}

func TestRemoteErrorAllowsFallbackClassifiesTimeoutTransportAndClientErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "deadline", err: context.DeadlineExceeded, want: true},
		{name: "transport", err: errors.New("connection reset"), want: true},
		{name: "request timeout", err: &RemoteError{StatusCode: 408}, want: true},
		{name: "rate limited", err: &RemoteError{StatusCode: 429}, want: true},
		{name: "server error", err: &RemoteError{StatusCode: 503}, want: true},
		{name: "canceled", err: context.Canceled, want: false},
		{name: "unauthorized", err: &RemoteError{StatusCode: 401}, want: false},
		{name: "contract", err: &RemoteError{StatusCode: 422}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := remoteErrorAllowsFallback(tt.err); got != tt.want {
				t.Fatalf("remoteErrorAllowsFallback(%v)=%v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestCoordinatorReportsOnlyExecutedFallbacks(t *testing.T) {
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{CardID: 9, CardRevision: 1}}
	public := &publicSearchStub{docs: []rag.Document{{ID: "local", Title: "本地", Content: "结果"}}}
	remote := &remoteRetrieverStub{err: &RemoteError{StatusCode: 503}}
	var observed []error
	coordinator := NewCoordinator(
		resolver,
		public,
		&releaseSearchStub{},
		WithRemote("fallback", remote, nil),
		WithFallbackObserver(func(err error) { observed = append(observed, err) }),
	)

	_, _ = coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})
	remote.err = &RemoteError{StatusCode: 400}
	_, _ = coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})

	if len(observed) != 1 {
		t.Fatalf("fallback observations=%d, want 1", len(observed))
	}
	var remoteErr *RemoteError
	if !errors.As(observed[0], &remoteErr) || remoteErr.StatusCode != 503 {
		t.Fatalf("fallback error=%v", observed[0])
	}
}

func TestCoordinatorRolloutSelectsStableFivePercentByUser(t *testing.T) {
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{CardID: 9, CardRevision: 1}}
	public := &publicSearchStub{docs: []rag.Document{{ID: "local", Title: "本地", Content: "结果"}}}
	remote := &remoteRetrieverStub{result: RemoteResult{Documents: []rag.Document{{ID: "remote", Title: "远程", Content: "结果"}}}}
	coordinator := NewCoordinator(
		resolver,
		public,
		&releaseSearchStub{},
		WithRemote("fallback", remote, nil),
		WithRolloutPercent(5),
	)

	remoteResults := 0
	for userID := int64(1); userID <= 100; userID++ {
		result, err := coordinator.Retrieve(context.Background(), Input{UserID: userID, SessionID: 8, CardID: 9, Query: "问题"})
		if err != nil {
			t.Fatalf("user %d: %v", userID, err)
		}
		if reflect.DeepEqual(documentIDs(result.Documents), []string{"remote"}) {
			remoteResults++
		}
	}
	if remoteResults != 5 || remote.calls != 5 || public.calls != 95 {
		t.Fatalf("remote results=%d calls=%d local calls=%d", remoteResults, remote.calls, public.calls)
	}

	for range 2 {
		result, err := coordinator.Retrieve(context.Background(), Input{UserID: 1, SessionID: 8, CardID: 9, Query: "问题"})
		if err != nil || !reflect.DeepEqual(documentIDs(result.Documents), []string{"remote"}) {
			t.Fatalf("selected user was not stable: result=%+v err=%v", result, err)
		}
	}
	for range 2 {
		result, err := coordinator.Retrieve(context.Background(), Input{UserID: 50, SessionID: 8, CardID: 9, Query: "问题"})
		if err != nil || !reflect.DeepEqual(documentIDs(result.Documents), []string{"local"}) {
			t.Fatalf("local user was not stable: result=%+v err=%v", result, err)
		}
	}
}

func TestCoordinatorRolloutDoesNotLimitShadowComparison(t *testing.T) {
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{CardID: 9, CardRevision: 1}}
	remote := &remoteRetrieverStub{}
	reported := make(chan ShadowComparison, 1)
	coordinator := NewCoordinator(
		resolver,
		&publicSearchStub{},
		&releaseSearchStub{},
		WithRemote("shadow", remote, func(comparison ShadowComparison) { reported <- comparison }),
		WithRolloutPercent(0),
	)

	if _, err := coordinator.Retrieve(context.Background(), Input{UserID: 50, SessionID: 8, CardID: 9, Query: "问题"}); err != nil {
		t.Fatal(err)
	}
	<-reported
	if remote.calls != 1 {
		t.Fatalf("shadow remote calls=%d", remote.calls)
	}
}

func TestCoordinatorShadowReturnsLocalAndReportsComparison(t *testing.T) {
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{CardID: 9, CardRevision: 1}}
	public := &publicSearchStub{docs: []rag.Document{{ID: "local", Title: "本地", Content: "结果"}}}
	remote := &remoteRetrieverStub{result: RemoteResult{Documents: []rag.Document{{ID: "remote", Title: "远程", Content: "结果"}}}, delay: time.Millisecond}
	reported := make(chan ShadowComparison, 1)
	coordinator := NewCoordinator(resolver, public, &releaseSearchStub{}, WithRemote("shadow", remote, func(comparison ShadowComparison) { reported <- comparison }))

	result, err := coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})
	if err != nil || !reflect.DeepEqual(documentIDs(result.Documents), []string{"local"}) {
		t.Fatalf("expected local result, result=%+v err=%v", result, err)
	}
	comparison := <-reported
	if !reflect.DeepEqual(comparison.LocalDocumentIDs, []string{"local"}) || !reflect.DeepEqual(comparison.RemoteDocumentIDs, []string{"remote"}) {
		t.Fatalf("comparison=%+v", comparison)
	}
	if comparison.Duration < time.Millisecond {
		t.Fatalf("comparison duration=%v", comparison.Duration)
	}
}

func (s *releaseSearchStub) SearchReleaseChunks(_ context.Context, releaseID int64, _ string, _ int, _ float64) ([]rag.Document, error) {
	s.releaseIDs = append(s.releaseIDs, releaseID)
	return append([]rag.Document(nil), s.docsByRelease[releaseID]...), s.errors[releaseID]
}

func TestCoordinatorReturnsPublicTheoryAndCurrentTypeWithTrace(t *testing.T) {
	typeThree := 3
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 4, MainType: 3,
		Resolution: Resolution{
			Theory:        &Binding{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", ReleaseID: 100},
			EnneagramType: &Binding{Layer: LayerEnneagramType, EnneagramType: &typeThree, LibraryID: 13, LibraryKey: "enneagram-type-03", ReleaseID: 103},
		},
	}}
	public := &publicSearchStub{docs: []rag.Document{{ID: "kb-1", Title: "公共", Content: "公共知识"}}}
	releases := &releaseSearchStub{docsByRelease: map[int64][]rag.Document{
		100: {{ID: "theory:1001", Title: "核心", Content: "理论核心"}},
		103: {{ID: "theory:3001", Title: "三号", Content: "三号知识", Tags: []string{"enneagram", "type-03"}}},
	}}

	result, err := NewCoordinator(resolver, public, releases).Retrieve(context.Background(), Input{
		UserID: 7, SessionID: 8, CardID: 9, Query: "我在工作中总想证明自己",
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if !reflect.DeepEqual([]string{"kb-1", "theory:1001", "theory:3001"}, documentIDs(result.Documents)) {
		t.Fatalf("documents = %+v", result.Documents)
	}
	if resolver.input.UserID != 7 || resolver.input.SessionID != 8 || resolver.input.CardID != 9 {
		t.Fatalf("resolver input = %+v", resolver.input)
	}
	if result.Trace.CardID != 9 || result.Trace.CardRevision != 4 || result.Trace.EnneagramType == nil || *result.Trace.EnneagramType != 3 {
		t.Fatalf("trace identity = %+v", result.Trace)
	}
	if !reflect.DeepEqual([]string{"kb-1"}, result.Trace.LayerHits[LayerPublic].ChunkIDs) ||
		result.Trace.LayerHits[LayerTheory].LibraryID != 10 ||
		result.Trace.LayerHits[LayerTheory].ReleaseID != 100 ||
		result.Trace.LayerHits[LayerEnneagramType].LibraryID != 13 ||
		result.Trace.LayerHits[LayerEnneagramType].ReleaseID != 103 {
		t.Fatalf("layer hits = %+v", result.Trace.LayerHits)
	}
}

func TestCoordinatorDeduplicatesByIDAndContentWithLimitsAndStableOrder(t *testing.T) {
	typeThree := 3
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 1, MainType: 3,
		Resolution: Resolution{
			Theory:        &Binding{Layer: LayerTheory, LibraryID: 10, ReleaseID: 100},
			EnneagramType: &Binding{Layer: LayerEnneagramType, EnneagramType: &typeThree, LibraryID: 13, LibraryKey: "enneagram-type-03", ReleaseID: 103},
		},
	}}
	public := &publicSearchStub{docs: []rag.Document{
		{ID: "duplicate-id", Title: "公共重复", Content: "被正式理论替代"},
		{ID: "public-2", Title: "公共二", Content: "公共内容二"},
		{ID: "public-3", Title: "公共三", Content: "公共内容三"},
	}}
	releases := &releaseSearchStub{docsByRelease: map[int64][]rag.Document{
		100: {
			{ID: "duplicate-id", Title: "理论保留", Content: "理论优先内容"},
			{ID: "theory-content", Title: "内容重复", Content: "相同 内容"},
		},
		103: {
			{ID: "type-duplicate", Title: "型号重复", Content: "相同内容", Tags: []string{"type-03"}},
			{ID: "type-2", Title: "型号二", Content: "123456", Tags: []string{"type-03"}},
		},
	}}
	limits := Limits{Public: 1, Theory: 2, EnneagramType: 2, TotalRunes: 20}
	coordinator := NewCoordinator(resolver, public, releases, WithLimits(limits))

	first, err := coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("results are not stable:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if got := documentIDs(first.Documents); !reflect.DeepEqual(got, []string{"duplicate-id", "theory-content", "type-2"}) {
		t.Fatalf("deduplicated limited documents = %v", got)
	}
	if runeLength(first.Documents) > limits.TotalRunes {
		t.Fatalf("content length = %d, limit = %d", runeLength(first.Documents), limits.TotalRunes)
	}
}

func TestCoordinatorKeepsAvailableLayersAndNeverFallsBackToAnotherType(t *testing.T) {
	typeThree := 3
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 2, MainType: 3,
		Resolution: Resolution{
			Theory:        &Binding{Layer: LayerTheory, LibraryID: 10, ReleaseID: 100},
			EnneagramType: &Binding{Layer: LayerEnneagramType, EnneagramType: &typeThree, LibraryID: 13, LibraryKey: "enneagram-type-03", ReleaseID: 103},
		},
	}}
	public := &publicSearchStub{docs: []rag.Document{{ID: "public", Title: "公共", Content: "公共可用"}}}
	releases := &releaseSearchStub{
		docsByRelease: map[int64][]rag.Document{100: {{ID: "theory", Title: "理论", Content: "可用"}}},
		errors:        map[int64]error{103: errors.New("type search failed")},
	}

	result, err := NewCoordinator(resolver, public, releases).Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(documentIDs(result.Documents), []string{"public", "theory"}) {
		t.Fatalf("documents = %+v", result.Documents)
	}
	if !reflect.DeepEqual(releases.releaseIDs, []int64{100, 103}) {
		t.Fatalf("release searches = %v; no other type release may be queried", releases.releaseIDs)
	}
	if !containsDiagnostic(result.Trace.LayerHits[LayerEnneagramType].Diagnostics, "search_failed") {
		t.Fatalf("type diagnostics = %+v", result.Trace.LayerHits[LayerEnneagramType].Diagnostics)
	}
}

func TestCoordinatorDropsCrossTypeDocuments(t *testing.T) {
	typeThree := 3
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 2, MainType: 3,
		Resolution: Resolution{EnneagramType: &Binding{
			Layer: LayerEnneagramType, EnneagramType: &typeThree, LibraryID: 13,
			LibraryKey: "enneagram-type-03", ReleaseID: 103,
		}},
	}}
	releases := &releaseSearchStub{docsByRelease: map[int64][]rag.Document{103: {
		{ID: "wrong", Title: "二号", Content: "错误型号", Tags: []string{"type-02"}},
		{ID: "right", Title: "三号", Content: "正确型号", Tags: []string{"type-03"}},
	}}}

	result, err := NewCoordinator(resolver, &publicSearchStub{}, releases).Retrieve(context.Background(), Input{UserID: 7, SessionID: 8, CardID: 9, Query: "问题"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(documentIDs(result.Documents), []string{"right"}) {
		t.Fatalf("documents = %+v", result.Documents)
	}
	if !containsDiagnostic(result.Trace.LayerHits[LayerEnneagramType].Diagnostics, "cross_type_document") {
		t.Fatalf("type diagnostics = %+v", result.Trace.LayerHits[LayerEnneagramType].Diagnostics)
	}
}

func documentIDs(documents []rag.Document) []string {
	ids := make([]string, len(documents))
	for index := range documents {
		ids[index] = documents[index].ID
	}
	return ids
}

func runeLength(documents []rag.Document) int {
	total := 0
	for _, document := range documents {
		total += len([]rune(document.Content))
	}
	return total
}

func containsDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
