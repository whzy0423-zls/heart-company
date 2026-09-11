package appknowledge

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

type coordinatorResolverStub struct {
	resolution ConversationResolution
	err        error
	input      Input
	requested  []int
}

func (s *coordinatorResolverStub) ResolveConversation(_ context.Context, userID, sessionID, cardID int64, requestedTypes []int) (ConversationResolution, error) {
	s.input = Input{UserID: userID, SessionID: sessionID, CardID: cardID}
	s.requested = append([]int(nil), requestedTypes...)
	return s.resolution, s.err
}

type publicSearchStub struct {
	docs  []rag.Document
	err   error
	calls int
	topKs []int
}

func (s *publicSearchStub) SearchPublic(_ context.Context, _ string, topK int) ([]rag.Document, error) {
	s.calls++
	s.topKs = append(s.topKs, topK)
	return append([]rag.Document(nil), s.docs...), s.err
}

type releaseSearchStub struct {
	docsByRelease map[int64][]rag.Document
	errors        map[int64]error
	releaseIDs    []int64
	topKs         map[int64][]int
	mu            sync.Mutex
}

func (s *releaseSearchStub) SearchReleaseChunks(_ context.Context, releaseID int64, _ string, topK int, _ float64) ([]rag.Document, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseIDs = append(s.releaseIDs, releaseID)
	if s.topKs == nil {
		s.topKs = make(map[int64][]int)
	}
	s.topKs[releaseID] = append(s.topKs[releaseID], topK)
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

func TestCoordinatorRequestedTypesSearchesOnlyExplicitBindingsInStableOrder(t *testing.T) {
	bindings := make([]*Binding, 0, 4)
	docsByRelease := map[int64][]rag.Document{100: {{ID: "theory", Title: "理论", Content: "正式理论"}}}
	for _, typeNumber := range []int{1, 2, 3, 4} {
		typeValue := typeNumber
		bindings = append(bindings, &Binding{
			Layer: LayerEnneagramType, EnneagramType: &typeValue,
			LibraryID: int64(10 + typeNumber), LibraryKey: "enneagram-type-0" + string(rune('0'+typeNumber)),
			ReleaseID: int64(100 + typeNumber),
		})
		docsByRelease[int64(100+typeNumber)] = []rag.Document{{
			ID: "type-" + string(rune('0'+typeNumber)), Title: "型号", Content: string(rune('A'+typeNumber)) + strings.Repeat("深", 400),
			Tags: []string{"type-0" + string(rune('0'+typeNumber))},
		}}
	}
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 3, MainType: 6,
		Resolution: Resolution{
			Theory:                &Binding{Layer: LayerTheory, LibraryID: 10, LibraryKey: "enneagram-core", ReleaseID: 100},
			RequestedTypeBindings: bindings,
		},
	}}
	public := &publicSearchStub{docs: []rag.Document{
		{ID: "public-1", Title: "公共一", Content: "公共一"},
		{ID: "public-2", Title: "公共二", Content: "公共二"},
		{ID: "public-3", Title: "公共三", Content: "不应入选"},
	}}
	releases := &releaseSearchStub{docsByRelease: docsByRelease, errors: map[int64]error{102: errors.New("type 2 failed")}}

	result, err := NewCoordinator(resolver, public, releases).Retrieve(context.Background(), Input{
		UserID: 7, SessionID: 8, CardID: 9, Query: "1 2 3 4号分别是什么", RequestedTypes: []int{4, 1, 2, 3, 4, 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolver.requested, []int{1, 2, 3, 4}) {
		t.Fatalf("resolver requested types = %v, want [1 2 3 4]", resolver.requested)
	}
	gotReleaseIDs := append([]int64(nil), releases.releaseIDs...)
	sort.Slice(gotReleaseIDs, func(i, j int) bool { return gotReleaseIDs[i] < gotReleaseIDs[j] })
	if !reflect.DeepEqual(gotReleaseIDs, []int64{100, 101, 102, 103, 104}) {
		t.Fatalf("release searches = %v; current-card type 6 must not be queried", gotReleaseIDs)
	}
	if got := documentIDs(result.Documents); !reflect.DeepEqual(got, []string{"public-1", "public-2", "theory", "type-1", "type-3", "type-4"}) {
		t.Fatalf("documents = %v", got)
	}
	for _, document := range result.Documents {
		if strings.HasPrefix(document.ID, "type-") && len([]rune(document.Content)) > 360 {
			t.Fatalf("%s snippet has %d runes, want <= 360", document.ID, len([]rune(document.Content)))
		}
	}
	for _, typeNumber := range []int{1, 2, 3, 4} {
		key := typeTraceKey(typeNumber)
		if _, ok := result.Trace.LayerHits[key]; !ok {
			t.Fatalf("trace missing %q: %+v", key, result.Trace.LayerHits)
		}
	}
	if !containsDiagnostic(result.Trace.LayerHits[typeTraceKey(2)].Diagnostics, "search_failed") {
		t.Fatalf("type 2 diagnostics = %+v", result.Trace.LayerHits[typeTraceKey(2)].Diagnostics)
	}
	if _, leaked := result.Trace.LayerHits[typeTraceKey(6)]; leaked {
		t.Fatalf("unrequested current-card type trace leaked: %+v", result.Trace.LayerHits)
	}
	if !reflect.DeepEqual(public.topKs, []int{6}) || !reflect.DeepEqual(releases.topKs[100], []int{9}) {
		t.Fatalf("explicit candidate limits public=%v theory=%v", public.topKs, releases.topKs[100])
	}
	for _, releaseID := range []int64{101, 102, 103, 104} {
		if !reflect.DeepEqual(releases.topKs[releaseID], []int{3}) {
			t.Fatalf("type release %d topKs = %v, want [3]", releaseID, releases.topKs[releaseID])
		}
	}
	if runeLength(result.Documents) > 5000 {
		t.Fatalf("combined reference = %d runes, want <= 5000", runeLength(result.Documents))
	}
}

func TestCoordinatorEmptyRequestedTypesPreservesLegacyLimitsAndCurrentCard(t *testing.T) {
	typeSix := 6
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 1, MainType: 6,
		Resolution: Resolution{
			Theory:        &Binding{Layer: LayerTheory, ReleaseID: 100},
			EnneagramType: &Binding{Layer: LayerEnneagramType, EnneagramType: &typeSix, LibraryKey: "enneagram-type-06", ReleaseID: 106},
		},
	}}
	public := &publicSearchStub{}
	releases := &releaseSearchStub{docsByRelease: map[int64][]rag.Document{}}

	_, err := NewCoordinator(resolver, public, releases).Retrieve(context.Background(), Input{
		UserID: 7, SessionID: 8, CardID: 9, Query: "最近关系压力很大", RequestedTypes: []int{0, 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolver.requested) != 0 {
		t.Fatalf("resolver requested = %v, want empty normalized list", resolver.requested)
	}
	if !reflect.DeepEqual(public.topKs, []int{12}) || !reflect.DeepEqual(releases.topKs[100], []int{9}) || !reflect.DeepEqual(releases.topKs[106], []int{9}) {
		t.Fatalf("legacy candidate limits public=%v theory=%v type=%v", public.topKs, releases.topKs[100], releases.topKs[106])
	}
}

type concurrentRequestedTypeSearcher struct {
	started chan int64
	release chan struct{}
}

func (s *concurrentRequestedTypeSearcher) SearchReleaseChunks(ctx context.Context, releaseID int64, _ string, _ int, _ float64) ([]rag.Document, error) {
	if releaseID == 100 {
		return []rag.Document{{ID: "theory", Title: "理论", Content: "正式理论"}}, nil
	}
	select {
	case s.started <- releaseID:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-s.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	typeNumber := int(releaseID - 100)
	return []rag.Document{{
		ID: "type-" + string(rune('0'+typeNumber)), Title: "型号", Content: "型号知识" + string(rune('0'+typeNumber)),
		Tags: []string{"type-0" + string(rune('0'+typeNumber))},
	}}, nil
}

func TestCoordinatorRequestedTypeSearchesRunConcurrentlyAndCollectInNumericOrder(t *testing.T) {
	typeOne, typeTwo := 1, 2
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 1, MainType: 6,
		Resolution: Resolution{
			Theory: &Binding{Layer: LayerTheory, ReleaseID: 100},
			RequestedTypeBindings: []*Binding{
				{Layer: LayerEnneagramType, EnneagramType: &typeOne, LibraryKey: "enneagram-type-01", ReleaseID: 101},
				{Layer: LayerEnneagramType, EnneagramType: &typeTwo, LibraryKey: "enneagram-type-02", ReleaseID: 102},
			},
		},
	}}
	searcher := &concurrentRequestedTypeSearcher{started: make(chan int64, 2), release: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resultChannel := make(chan Result, 1)
	errorChannel := make(chan error, 1)
	go func() {
		result, err := NewCoordinator(resolver, &publicSearchStub{}, searcher).Retrieve(ctx, Input{
			UserID: 7, SessionID: 8, CardID: 9, Query: "1号和2号", RequestedTypes: []int{2, 1},
		})
		resultChannel <- result
		errorChannel <- err
	}()

	started := make([]int64, 0, 2)
	for len(started) < 2 {
		select {
		case releaseID := <-searcher.started:
			started = append(started, releaseID)
		case <-ctx.Done():
			close(searcher.release)
			t.Fatalf("only %d requested type search(es) started before timeout; searches are not concurrent", len(started))
		}
	}
	close(searcher.release)
	if err := <-errorChannel; err != nil {
		t.Fatal(err)
	}
	result := <-resultChannel
	if got := documentIDs(result.Documents); !reflect.DeepEqual(got, []string{"theory", "type-1", "type-2"}) {
		t.Fatalf("stable documents = %v", got)
	}
}

type lexicalRequestedTypeSearcher struct {
	mu      sync.Mutex
	queries map[int]string
}

func (s *lexicalRequestedTypeSearcher) SearchReleaseChunks(_ context.Context, releaseID int64, query string, _ int, _ float64) ([]rag.Document, error) {
	if releaseID == 100 {
		return []rag.Document{{ID: "theory", Title: "理论", Content: "正式理论"}}, nil
	}
	typeNumber := int(releaseID - 100)
	names := []string{"", "完美型", "助人型", "成就型", "自我型", "思考型", "忠诚型", "活跃型", "领袖型", "和平型"}
	s.mu.Lock()
	if s.queries == nil {
		s.queries = make(map[int]string)
	}
	s.queries[typeNumber] = query
	s.mu.Unlock()
	wantAnchor := fmt.Sprintf("%d号%s", typeNumber, names[typeNumber])
	if !strings.Contains(query, "九型人格") || !strings.Contains(query, wantAnchor) {
		return nil, nil
	}
	for otherType := 1; otherType <= 9; otherType++ {
		if otherType != typeNumber && (strings.Contains(query, fmt.Sprintf("%d号", otherType)) || strings.Contains(query, names[otherType])) {
			return nil, fmt.Errorf("type %d query contains type %d anchor", typeNumber, otherType)
		}
	}
	return []rag.Document{{
		ID: fmt.Sprintf("type-%d", typeNumber), Title: wantAnchor, Content: wantAnchor + "知识",
		Tags: []string{fmt.Sprintf("type-%02d", typeNumber)},
	}}, nil
}

func TestCoordinatorRequestedTypeQueriesCarryOnlyCurrentCanonicalLexicalAnchor(t *testing.T) {
	bindings := make([]*Binding, 0, 9)
	requestedTypes := make([]int, 0, 9)
	for typeNumber := 1; typeNumber <= 9; typeNumber++ {
		typeValue := typeNumber
		requestedTypes = append(requestedTypes, typeNumber)
		bindings = append(bindings, &Binding{
			Layer: LayerEnneagramType, EnneagramType: &typeValue,
			LibraryKey: fmt.Sprintf("enneagram-type-%02d", typeNumber), ReleaseID: int64(100 + typeNumber),
		})
	}
	resolver := &coordinatorResolverStub{resolution: ConversationResolution{
		CardID: 9, CardRevision: 1, MainType: 6,
		Resolution: Resolution{
			Theory: &Binding{Layer: LayerTheory, ReleaseID: 100}, RequestedTypeBindings: bindings,
		},
	}}
	searcher := &lexicalRequestedTypeSearcher{}

	result, err := NewCoordinator(resolver, &publicSearchStub{}, searcher).Retrieve(context.Background(), Input{
		UserID: 7, SessionID: 8, CardID: 9,
		Query:          "比较1号完美型、2号助人型、3号成就型、4号自我型、5号思考型、6号忠诚型、7号活跃型、8号领袖型和9号和平型",
		RequestedTypes: requestedTypes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := documentIDs(result.Documents); len(got) != 10 {
		t.Fatalf("documents = %v, want theory plus all nine lexically matched type documents", got)
	}
	for typeNumber := 1; typeNumber <= 9; typeNumber++ {
		query := searcher.queries[typeNumber]
		if query == "" {
			t.Fatalf("type %d query was not captured", typeNumber)
		}
	}
}

func TestSelectRequestedTypeDocumentsEnforcesExactCombinedRuneBoundaryAndPriority(t *testing.T) {
	document := func(id string, marker rune, runes int) rag.Document {
		return rag.Document{ID: id, Title: id, Content: string(marker) + strings.Repeat("文", runes-1)}
	}
	byLayer := map[string][]rag.Document{
		LayerTheory: {
			document("theory-1", 'A', 1000), document("theory-2", 'B', 1000), document("theory-3", 'C', 1000),
		},
		typeTraceKey(1): {document("type-1", 'D', 360)},
		typeTraceKey(2): {document("type-2", 'E', 360)},
		LayerPublic: {
			document("public-exact", 'F', 1280), document("public-over-budget", 'G', 1),
		},
	}

	selected := selectDocuments(byLayer, []string{typeTraceKey(1), typeTraceKey(2)}, requestedTypeLimits, true)
	selectedDocuments := append([]rag.Document{}, selected[LayerTheory]...)
	selectedDocuments = append(selectedDocuments, selected[typeTraceKey(1)]...)
	selectedDocuments = append(selectedDocuments, selected[typeTraceKey(2)]...)
	selectedDocuments = append(selectedDocuments, selected[LayerPublic]...)
	if got := runeLength(selectedDocuments); got != 5000 {
		t.Fatalf("selected runes = %d, want exact 5000 boundary", got)
	}
	if got := documentIDs(selected[LayerPublic]); !reflect.DeepEqual(got, []string{"public-exact"}) {
		t.Fatalf("public selection = %v; lower-priority over-budget chunk must be excluded", got)
	}
	if got := documentIDs(selected[LayerTheory]); !reflect.DeepEqual(got, []string{"theory-1", "theory-2", "theory-3"}) {
		t.Fatalf("higher-priority theory selection = %v", got)
	}
	if len(selected[typeTraceKey(1)]) != 1 || len(selected[typeTraceKey(2)]) != 1 {
		t.Fatalf("requested type selections = %+v", selected)
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
