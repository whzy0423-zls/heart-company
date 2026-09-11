package appknowledge

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

var ErrInvalidInput = errors.New("app knowledge input is invalid")

type Input struct {
	UserID         int64
	SessionID      int64
	CardID         int64
	Query          string
	RequestedTypes []int
}

type ConversationResolution struct {
	Resolution
	CardID       int64
	CardRevision int64
	MainType     int
}

type ConversationResolver interface {
	ResolveConversation(ctx context.Context, userID, sessionID, cardID int64, requestedTypes []int) (ConversationResolution, error)
}

type PublicSearcher interface {
	SearchPublic(ctx context.Context, query string, topK int) ([]rag.Document, error)
}

type ReleaseSearcher interface {
	SearchReleaseChunks(ctx context.Context, releaseID int64, query string, topK int, minScore float64) ([]rag.Document, error)
}

type Limits struct {
	Public        int
	Theory        int
	EnneagramType int
	TotalRunes    int
}

var defaultLimits = Limits{Public: 4, Theory: 3, EnneagramType: 3, TotalRunes: 8000}
var requestedTypeLimits = Limits{Public: 2, Theory: 3, EnneagramType: 9, TotalRunes: 5000}

type LayerHit struct {
	LibraryID   int64        `json:"library_id,omitempty"`
	LibraryKey  string       `json:"library_key,omitempty"`
	ReleaseID   int64        `json:"release_id,omitempty"`
	ChunkIDs    []string     `json:"chunk_ids"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Trace struct {
	CardID        int64               `json:"card_id"`
	EnneagramType *int                `json:"enneagram_type,omitempty"`
	CardRevision  int64               `json:"card_revision"`
	LayerHits     map[string]LayerHit `json:"layer_hits"`
}

type Result struct {
	Documents []rag.Document
	Trace     Trace
}

type Option func(*Coordinator)

func WithLimits(limits Limits) Option {
	return func(coordinator *Coordinator) {
		coordinator.limits = normalizeLimits(limits)
	}
}

type Coordinator struct {
	resolver ConversationResolver
	public   PublicSearcher
	releases ReleaseSearcher
	limits   Limits
}

func NewCoordinator(resolver ConversationResolver, public PublicSearcher, releases ReleaseSearcher, options ...Option) *Coordinator {
	coordinator := &Coordinator{resolver: resolver, public: public, releases: releases, limits: defaultLimits}
	for _, option := range options {
		option(coordinator)
	}
	return coordinator
}

func (c *Coordinator) Retrieve(ctx context.Context, input Input) (Result, error) {
	input.Query = strings.TrimSpace(input.Query)
	if c == nil || c.resolver == nil || input.UserID <= 0 || input.SessionID <= 0 || input.CardID <= 0 || input.Query == "" {
		return Result{}, ErrInvalidInput
	}
	requestedTypes := normalizeRequestedTypes(input.RequestedTypes)
	explicitTypes := len(requestedTypes) > 0
	limits := c.limits
	if explicitTypes {
		limits = requestedTypeLimits
	}
	resolved, err := c.resolver.ResolveConversation(ctx, input.UserID, input.SessionID, input.CardID, requestedTypes)
	if err != nil {
		return Result{}, fmt.Errorf("resolve conversation knowledge: %w", err)
	}
	if resolved.CardID != input.CardID || resolved.CardRevision <= 0 {
		return Result{}, fmt.Errorf("resolve conversation knowledge: %w", ErrInvalidInput)
	}

	trace := Trace{
		CardID: resolved.CardID, CardRevision: resolved.CardRevision,
		LayerHits: map[string]LayerHit{
			LayerPublic: {LibraryKey: LayerPublic, ChunkIDs: []string{}},
			LayerTheory: {ChunkIDs: []string{}},
		},
	}
	if explicitTypes {
		for _, typeNumber := range requestedTypes {
			trace.LayerHits[typeTraceKey(typeNumber)] = LayerHit{ChunkIDs: []string{}}
		}
	} else {
		trace.LayerHits[LayerEnneagramType] = LayerHit{ChunkIDs: []string{}}
	}
	if resolved.MainType >= 1 && resolved.MainType <= 9 {
		mainType := resolved.MainType
		trace.EnneagramType = &mainType
	}
	for _, diagnostic := range resolved.Diagnostics {
		addLayerDiagnostic(trace.LayerHits, diagnostic)
	}

	publicDocs := c.searchPublic(ctx, input.Query, limits.Public, &trace)
	theoryDocs := c.searchBinding(ctx, input.Query, resolved.Theory, limits.Theory, LayerTheory, &trace)

	documentsByLayer := map[string][]rag.Document{
		LayerPublic: publicDocs, LayerTheory: theoryDocs,
	}
	typeLayers := []string{LayerEnneagramType}
	if explicitTypes {
		typeLayers = make([]string, 0, len(requestedTypes))
		for layer, documents := range c.searchRequestedTypes(ctx, input.Query, requestedTypes, resolved.RequestedTypeBindings, &trace) {
			documentsByLayer[layer] = documents
		}
		for _, typeNumber := range requestedTypes {
			typeLayers = append(typeLayers, typeTraceKey(typeNumber))
		}
	} else {
		documentsByLayer[LayerEnneagramType] = c.searchType(ctx, input.Query, resolved, limits.EnneagramType, &trace)
	}
	selected := selectDocuments(documentsByLayer, typeLayers, limits, explicitTypes)
	for layer, documents := range selected {
		hit := trace.LayerHits[layer]
		for _, document := range documents {
			hit.ChunkIDs = append(hit.ChunkIDs, document.ID)
		}
		trace.LayerHits[layer] = hit
	}

	documents := make([]rag.Document, 0, len(selected[LayerPublic])+len(selected[LayerTheory])+limits.EnneagramType)
	outputLayers := append([]string{LayerPublic, LayerTheory}, typeLayers...)
	for _, layer := range outputLayers {
		documents = append(documents, selected[layer]...)
	}
	return Result{Documents: documents, Trace: trace}, nil
}

func (c *Coordinator) searchPublic(ctx context.Context, query string, limit int, trace *Trace) []rag.Document {
	if c.public == nil || limit == 0 {
		return nil
	}
	documents, err := c.public.SearchPublic(ctx, query, searchCandidateLimit(limit))
	if err != nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerPublic, Code: "search_failed"})
		return nil
	}
	return documents
}

func (c *Coordinator) searchBinding(ctx context.Context, query string, binding *Binding, limit int, traceLayer string, trace *Trace) []rag.Document {
	if binding == nil || limit == 0 {
		return nil
	}
	hit := trace.LayerHits[traceLayer]
	hit.LibraryID = binding.LibraryID
	hit.LibraryKey = binding.LibraryKey
	hit.ReleaseID = binding.ReleaseID
	trace.LayerHits[traceLayer] = hit
	if c.releases == nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: traceLayer, Code: "search_unavailable"})
		return nil
	}
	documents, err := c.releases.SearchReleaseChunks(ctx, binding.ReleaseID, query, searchCandidateLimit(limit), 0.2)
	if err != nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: traceLayer, Code: "search_failed"})
		return nil
	}
	return documents
}

func (c *Coordinator) searchType(ctx context.Context, query string, resolved ConversationResolution, limit int, trace *Trace) []rag.Document {
	binding := resolved.EnneagramType
	if binding == nil || resolved.MainType < 1 || resolved.MainType > 9 || limit == 0 {
		return nil
	}
	if binding.EnneagramType == nil || *binding.EnneagramType != resolved.MainType || binding.LibraryKey != fmt.Sprintf("enneagram-type-%02d", resolved.MainType) {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "cross_type_binding"})
		return nil
	}
	documents := c.searchBinding(ctx, query, binding, limit, LayerEnneagramType, trace)
	filtered := documents[:0]
	for _, document := range documents {
		if documentMatchesType(document, resolved.MainType) {
			filtered = append(filtered, document)
			continue
		}
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "cross_type_document"})
	}
	return filtered
}

type requestedTypeSearchResult struct {
	layer       string
	documents   []rag.Document
	hit         LayerHit
	diagnostics []Diagnostic
}

func (c *Coordinator) searchRequestedTypes(ctx context.Context, query string, requestedTypes []int, bindings []*Binding, trace *Trace) map[string][]rag.Document {
	bindingsByType := make(map[int]*Binding, len(bindings))
	for _, binding := range bindings {
		if binding == nil || binding.EnneagramType == nil {
			continue
		}
		typeNumber := *binding.EnneagramType
		if typeNumber < 1 || typeNumber > 9 || binding.LibraryKey != fmt.Sprintf("enneagram-type-%02d", typeNumber) {
			continue
		}
		bindingsByType[typeNumber] = binding
	}
	results := make([]requestedTypeSearchResult, len(requestedTypes))
	var waitGroup sync.WaitGroup
	for index, typeNumber := range requestedTypes {
		index, typeNumber := index, typeNumber
		layer := typeTraceKey(typeNumber)
		binding := bindingsByType[typeNumber]
		if binding == nil {
			results[index] = requestedTypeSearchResult{layer: layer, diagnostics: []Diagnostic{{Layer: layer, Code: "binding_unavailable"}}}
			continue
		}
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			result := requestedTypeSearchResult{
				layer: layer,
				hit:   LayerHit{LibraryID: binding.LibraryID, LibraryKey: binding.LibraryKey, ReleaseID: binding.ReleaseID, ChunkIDs: []string{}},
			}
			if c.releases == nil {
				result.diagnostics = append(result.diagnostics, Diagnostic{Layer: layer, Code: "search_unavailable"})
				results[index] = result
				return
			}
			documents, err := c.releases.SearchReleaseChunks(ctx, binding.ReleaseID, query, searchCandidateLimit(1), 0.2)
			if err != nil {
				result.diagnostics = append(result.diagnostics, Diagnostic{Layer: layer, Code: "search_failed"})
				results[index] = result
				return
			}
			for _, document := range documents {
				if !documentMatchesType(document, typeNumber) {
					result.diagnostics = append(result.diagnostics, Diagnostic{Layer: layer, Code: "cross_type_document"})
					continue
				}
				document.Content = truncateRunes(document.Content, 360)
				result.documents = []rag.Document{document}
				break
			}
			results[index] = result
		}()
	}
	waitGroup.Wait()
	documentsByLayer := make(map[string][]rag.Document, len(results))
	for _, result := range results {
		hit := trace.LayerHits[result.layer]
		if result.hit.LibraryID > 0 {
			hit.LibraryID = result.hit.LibraryID
			hit.LibraryKey = result.hit.LibraryKey
			hit.ReleaseID = result.hit.ReleaseID
		}
		trace.LayerHits[result.layer] = hit
		for _, diagnostic := range result.diagnostics {
			addLayerDiagnostic(trace.LayerHits, diagnostic)
		}
		documentsByLayer[result.layer] = result.documents
	}
	return documentsByLayer
}

func selectDocuments(byLayer map[string][]rag.Document, typeLayers []string, limits Limits, explicitTypes bool) map[string][]rag.Document {
	selected := map[string][]rag.Document{LayerPublic: {}, LayerTheory: {}}
	for _, layer := range typeLayers {
		selected[layer] = []rag.Document{}
	}
	seenIDs := map[string]struct{}{}
	seenContents := map[[sha256.Size]byte]struct{}{}
	totalRunes := 0
	layerLimits := map[string]int{LayerPublic: limits.Public, LayerTheory: limits.Theory}
	for _, layer := range typeLayers {
		layerLimits[layer] = limits.EnneagramType
		if explicitTypes {
			layerLimits[layer] = 1
		}
	}
	// Formal definitions win duplicate selection, followed by the current type;
	// presentation is reordered to public -> theory -> type below.
	priorityLayers := append([]string{LayerTheory}, typeLayers...)
	priorityLayers = append(priorityLayers, LayerPublic)
	typeCount := 0
	for _, layer := range priorityLayers {
		for _, document := range byLayer[layer] {
			if len(selected[layer]) >= layerLimits[layer] || (explicitTypes && strings.HasPrefix(layer, LayerEnneagramType+"_") && typeCount >= limits.EnneagramType) {
				break
			}
			document.ID = strings.TrimSpace(document.ID)
			document.Title = strings.TrimSpace(document.Title)
			document.Content = strings.TrimSpace(document.Content)
			if document.ID == "" || document.Title == "" || document.Content == "" {
				continue
			}
			digest := normalizedDocumentDigest(document.Content)
			if _, duplicate := seenIDs[document.ID]; duplicate {
				continue
			}
			if _, duplicate := seenContents[digest]; duplicate {
				continue
			}
			contentRunes := len([]rune(document.Content))
			if totalRunes+contentRunes > limits.TotalRunes {
				continue
			}
			selected[layer] = append(selected[layer], document)
			seenIDs[document.ID] = struct{}{}
			seenContents[digest] = struct{}{}
			totalRunes += contentRunes
			if explicitTypes && strings.HasPrefix(layer, LayerEnneagramType+"_") {
				typeCount++
			}
		}
	}
	return selected
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func normalizeLimits(limits Limits) Limits {
	if limits.Public < 0 {
		limits.Public = 0
	}
	if limits.Theory < 0 {
		limits.Theory = 0
	}
	if limits.EnneagramType < 0 {
		limits.EnneagramType = 0
	}
	if limits.TotalRunes <= 0 {
		limits.TotalRunes = defaultLimits.TotalRunes
	}
	return limits
}

func searchCandidateLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	return limit * 3
}

func addLayerDiagnostic(hits map[string]LayerHit, diagnostic Diagnostic) {
	if diagnostic.Layer == "" {
		return
	}
	hit := hits[diagnostic.Layer]
	if hit.ChunkIDs == nil {
		hit.ChunkIDs = []string{}
	}
	for _, existing := range hit.Diagnostics {
		if existing.Code == diagnostic.Code {
			return
		}
	}
	hit.Diagnostics = append(hit.Diagnostics, diagnostic)
	hits[diagnostic.Layer] = hit
}

func normalizedDocumentDigest(content string) [sha256.Size]byte {
	normalized := strings.Map(func(value rune) rune {
		if unicode.IsSpace(value) || unicode.IsPunct(value) {
			return -1
		}
		return unicode.ToLower(value)
	}, strings.TrimSpace(content))
	return sha256.Sum256([]byte(normalized))
}

func documentMatchesType(document rag.Document, mainType int) bool {
	want := fmt.Sprintf("type-%02d", mainType)
	for _, tag := range document.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), want) {
			return true
		}
	}
	return false
}
