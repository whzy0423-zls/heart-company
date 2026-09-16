package appknowledge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

var ErrInvalidInput = errors.New("app knowledge input is invalid")

type Input struct {
	RequestID string
	Scene     string
	UserID    int64
	SessionID int64
	CardID    int64
	Query     string
}

type RemoteRequest struct {
	RequestID           string
	Query               string
	Scene               string
	Public              bool
	TheoryReleaseIDs    []int64
	EnneagramReleaseIDs []int64
	MainType            int
	WingType            int
}

type RemoteResult struct {
	Documents       []rag.Document
	Citations       []rag.Citation
	RetrievalMethod string
	TraceID         string
}

type RemoteRetriever interface {
	Retrieve(context.Context, RemoteRequest) (RemoteResult, error)
}

type RemoteError struct {
	StatusCode int
	Err        error
}

func (e *RemoteError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("remote knowledge service returned HTTP %d", e.StatusCode)
}

func (e *RemoteError) Unwrap() error { return e.Err }

type ShadowComparison struct {
	RequestID         string
	LocalDocumentIDs  []string
	RemoteDocumentIDs []string
	RemoteError       error
	Duration          time.Duration
}

type ShadowObserver func(ShadowComparison)
type FallbackObserver func(error)

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
	CardID          int64               `json:"card_id"`
	EnneagramType   *int                `json:"enneagram_type,omitempty"`
	CardRevision    int64               `json:"card_revision"`
	LayerHits       map[string]LayerHit `json:"layer_hits"`
	TraceID         string              `json:"trace_id,omitempty"`
	RetrievalMethod string              `json:"retrieval_method,omitempty"`
}

type Result struct {
	Documents []rag.Document
	Citations []rag.Citation
	Trace     Trace
}

type Option func(*Coordinator)

func WithLimits(limits Limits) Option {
	return func(coordinator *Coordinator) {
		coordinator.limits = normalizeLimits(limits)
	}
}

func WithRemote(backend string, remote RemoteRetriever, observer ShadowObserver) Option {
	return func(coordinator *Coordinator) {
		coordinator.backend = strings.ToLower(strings.TrimSpace(backend))
		coordinator.remote = remote
		coordinator.shadowObserver = observer
	}
}

func WithFallbackObserver(observer FallbackObserver) Option {
	return func(coordinator *Coordinator) {
		coordinator.fallbackObserver = observer
	}
}

func WithRolloutPercent(percent int) Option {
	return func(coordinator *Coordinator) {
		if percent < 0 {
			percent = 0
		}
		if percent > 100 {
			percent = 100
		}
		coordinator.rolloutPercent = percent
	}
}

type Coordinator struct {
	resolver         ConversationResolver
	public           PublicSearcher
	releases         ReleaseSearcher
	limits           Limits
	backend          string
	remote           RemoteRetriever
	shadowObserver   ShadowObserver
	fallbackObserver FallbackObserver
	rolloutPercent   int
}

func NewCoordinator(resolver ConversationResolver, public PublicSearcher, releases ReleaseSearcher, options ...Option) *Coordinator {
	coordinator := &Coordinator{resolver: resolver, public: public, releases: releases, limits: defaultLimits, backend: "local", rolloutPercent: 100}
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
	remoteRequest := remoteRequestFromResolution(input, resolved)
	switch c.backend {
	case "langchain":
		if !c.userInRollout(input.UserID) {
			return c.retrieveLocal(ctx, input.Query, resolved), nil
		}
		return c.retrieveRemote(ctx, resolved, remoteRequest)
	case "fallback":
		if !c.userInRollout(input.UserID) {
			return c.retrieveLocal(ctx, input.Query, resolved), nil
		}
		result, remoteErr := c.retrieveRemote(ctx, resolved, remoteRequest)
		if remoteErr == nil {
			return result, nil
		}
		if !RemoteErrorAllowsFallback(remoteErr) {
			return Result{}, remoteErr
		}
		if c.fallbackObserver != nil {
			c.fallbackObserver(remoteErr)
		}
	case "shadow":
		local := c.retrieveLocal(ctx, input.Query, resolved)
		if c.remote != nil {
			go c.compareShadow(context.WithoutCancel(ctx), remoteRequest, local)
		}
		return local, nil
	}
	return c.retrieveLocal(ctx, input.Query, resolved), nil
}

func (c *Coordinator) userInRollout(userID int64) bool {
	if c.rolloutPercent >= 100 {
		return true
	}
	if c.rolloutPercent <= 0 {
		return false
	}
	return int(userID%100) < c.rolloutPercent
}

func (c *Coordinator) retrieveLocal(ctx context.Context, query string, resolved ConversationResolution) Result {

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

	publicDocs := c.searchPublic(ctx, query, &trace)
	theoryDocs := c.searchBinding(ctx, query, resolved.Theory, c.limits.Theory, &trace)
	typeDocs := c.searchType(ctx, query, resolved, &trace)

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
	return Result{Documents: documents, Trace: trace}
}

func remoteRequestFromResolution(input Input, resolved ConversationResolution) RemoteRequest {
	requestID := strings.TrimSpace(input.RequestID)
	if requestID == "" {
		bytes := make([]byte, 16)
		if _, err := rand.Read(bytes); err == nil {
			requestID = hex.EncodeToString(bytes)
		} else {
			requestID = fmt.Sprintf("app-chat-%d-%d", input.SessionID, input.CardID)
		}
	}
	scene := strings.TrimSpace(input.Scene)
	if scene == "" {
		scene = "app_chat"
	}
	request := RemoteRequest{RequestID: requestID, Query: input.Query, Scene: scene, Public: true, MainType: resolved.MainType}
	if resolved.Theory != nil {
		request.TheoryReleaseIDs = []int64{resolved.Theory.ReleaseID}
	}
	if resolved.EnneagramType != nil {
		request.EnneagramReleaseIDs = []int64{resolved.EnneagramType.ReleaseID}
	}
	return request
}

func (c *Coordinator) retrieveRemote(ctx context.Context, resolved ConversationResolution, request RemoteRequest) (Result, error) {
	if c.remote == nil {
		return Result{}, errors.New("remote knowledge retriever unavailable")
	}
	remote, err := c.remote.Retrieve(ctx, request)
	if err != nil {
		return Result{}, err
	}
	trace := Trace{CardID: resolved.CardID, CardRevision: resolved.CardRevision, LayerHits: map[string]LayerHit{
		LayerPublic: {LibraryKey: LayerPublic, ChunkIDs: remoteDocumentIDs(remote.Documents), Diagnostics: []Diagnostic{{Layer: LayerPublic, Code: "remote_" + remote.RetrievalMethod}}},
		LayerTheory: {ChunkIDs: []string{}}, LayerEnneagramType: {ChunkIDs: []string{}},
	}}
	if resolved.MainType >= 1 && resolved.MainType <= 9 {
		value := resolved.MainType
		trace.EnneagramType = &value
	}
	trace.TraceID = remote.TraceID
	trace.RetrievalMethod = remote.RetrievalMethod
	return Result{Documents: remote.Documents, Citations: remote.Citations, Trace: trace}, nil
}

func (c *Coordinator) compareShadow(ctx context.Context, request RemoteRequest, local Result) {
	started := time.Now()
	remote, err := c.remote.Retrieve(ctx, request)
	if c.shadowObserver != nil {
		c.shadowObserver(ShadowComparison{RequestID: request.RequestID, LocalDocumentIDs: remoteDocumentIDs(local.Documents), RemoteDocumentIDs: remoteDocumentIDs(remote.Documents), RemoteError: err, Duration: time.Since(started)})
	}
}

func remoteDocumentIDs(documents []rag.Document) []string {
	ids := make([]string, len(documents))
	for index := range documents {
		ids[index] = documents[index].ID
	}
	return ids
}

func RemoteErrorAllowsFallback(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var remoteErr *RemoteError
	if !errors.As(err, &remoteErr) || remoteErr.StatusCode == 0 {
		return true
	}
	return remoteErr.StatusCode == 408 || remoteErr.StatusCode == 429 || remoteErr.StatusCode >= 500
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
