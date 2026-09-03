package appknowledge

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

var ErrInvalidInput = errors.New("app knowledge input is invalid")

type Input struct {
	UserID    int64
	SessionID int64
	CardID    int64
	Query     string
}

type ConversationResolution struct {
	Resolution
	CardID       int64
	CardRevision int64
	MainType     int
}

type ConversationResolver interface {
	ResolveConversation(ctx context.Context, userID, sessionID, cardID int64) (ConversationResolution, error)
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
	resolved, err := c.resolver.ResolveConversation(ctx, input.UserID, input.SessionID, input.CardID)
	if err != nil {
		return Result{}, fmt.Errorf("resolve conversation knowledge: %w", err)
	}
	if resolved.CardID != input.CardID || resolved.CardRevision <= 0 {
		return Result{}, fmt.Errorf("resolve conversation knowledge: %w", ErrInvalidInput)
	}

	trace := Trace{
		CardID: resolved.CardID, CardRevision: resolved.CardRevision,
		LayerHits: map[string]LayerHit{
			LayerPublic:        {LibraryKey: LayerPublic, ChunkIDs: []string{}},
			LayerTheory:        {ChunkIDs: []string{}},
			LayerEnneagramType: {ChunkIDs: []string{}},
		},
	}
	if resolved.MainType >= 1 && resolved.MainType <= 9 {
		mainType := resolved.MainType
		trace.EnneagramType = &mainType
	}
	for _, diagnostic := range resolved.Diagnostics {
		addLayerDiagnostic(trace.LayerHits, diagnostic)
	}

	publicDocs := c.searchPublic(ctx, input.Query, &trace)
	theoryDocs := c.searchBinding(ctx, input.Query, resolved.Theory, c.limits.Theory, &trace)
	typeDocs := c.searchType(ctx, input.Query, resolved, &trace)

	documentsByLayer := map[string][]rag.Document{
		LayerPublic: publicDocs, LayerTheory: theoryDocs, LayerEnneagramType: typeDocs,
	}
	selected := c.selectDocuments(documentsByLayer)
	for layer, documents := range selected {
		hit := trace.LayerHits[layer]
		for _, document := range documents {
			hit.ChunkIDs = append(hit.ChunkIDs, document.ID)
		}
		trace.LayerHits[layer] = hit
	}

	documents := make([]rag.Document, 0, len(selected[LayerPublic])+len(selected[LayerTheory])+len(selected[LayerEnneagramType]))
	for _, layer := range []string{LayerPublic, LayerTheory, LayerEnneagramType} {
		documents = append(documents, selected[layer]...)
	}
	return Result{Documents: documents, Trace: trace}, nil
}

func (c *Coordinator) searchPublic(ctx context.Context, query string, trace *Trace) []rag.Document {
	if c.public == nil || c.limits.Public == 0 {
		return nil
	}
	documents, err := c.public.SearchPublic(ctx, query, searchCandidateLimit(c.limits.Public))
	if err != nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerPublic, Code: "search_failed"})
		return nil
	}
	return documents
}

func (c *Coordinator) searchBinding(ctx context.Context, query string, binding *Binding, limit int, trace *Trace) []rag.Document {
	if binding == nil || limit == 0 {
		return nil
	}
	hit := trace.LayerHits[binding.Layer]
	hit.LibraryID = binding.LibraryID
	hit.LibraryKey = binding.LibraryKey
	hit.ReleaseID = binding.ReleaseID
	trace.LayerHits[binding.Layer] = hit
	if c.releases == nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: binding.Layer, Code: "search_unavailable"})
		return nil
	}
	documents, err := c.releases.SearchReleaseChunks(ctx, binding.ReleaseID, query, searchCandidateLimit(limit), 0.2)
	if err != nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: binding.Layer, Code: "search_failed"})
		return nil
	}
	return documents
}

func (c *Coordinator) searchType(ctx context.Context, query string, resolved ConversationResolution, trace *Trace) []rag.Document {
	binding := resolved.EnneagramType
	if binding == nil || resolved.MainType < 1 || resolved.MainType > 9 || c.limits.EnneagramType == 0 {
		return nil
	}
	if binding.EnneagramType == nil || *binding.EnneagramType != resolved.MainType || binding.LibraryKey != fmt.Sprintf("enneagram-type-%02d", resolved.MainType) {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "cross_type_binding"})
		return nil
	}
	documents := c.searchBinding(ctx, query, binding, c.limits.EnneagramType, trace)
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

func (c *Coordinator) selectDocuments(byLayer map[string][]rag.Document) map[string][]rag.Document {
	selected := map[string][]rag.Document{
		LayerPublic: {}, LayerTheory: {}, LayerEnneagramType: {},
	}
	seenIDs := map[string]struct{}{}
	seenContents := map[[sha256.Size]byte]struct{}{}
	totalRunes := 0
	limits := map[string]int{
		LayerPublic: c.limits.Public, LayerTheory: c.limits.Theory, LayerEnneagramType: c.limits.EnneagramType,
	}
	// Formal definitions win duplicate selection, followed by the current type;
	// presentation is reordered to public -> theory -> type below.
	for _, layer := range []string{LayerTheory, LayerEnneagramType, LayerPublic} {
		for _, document := range byLayer[layer] {
			if len(selected[layer]) >= limits[layer] {
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
			if totalRunes+contentRunes > c.limits.TotalRunes {
				continue
			}
			selected[layer] = append(selected[layer], document)
			seenIDs[document.ID] = struct{}{}
			seenContents[digest] = struct{}{}
			totalRunes += contentRunes
		}
	}
	return selected
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
