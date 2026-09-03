package appknowledge

import (
	"context"
	"errors"
	"reflect"
	"testing"

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
