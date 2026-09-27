package appknowledge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

var (
	ErrInvalidInput      = errors.New("app knowledge input is invalid")
	ErrRemoteEmptyResult = errors.New("remote knowledge returned no documents")
)

type Input struct {
	RequestID      string
	Scene          string
	UserID         int64
	SessionID      int64
	CardID         int64
	Query          string
	RequestedTypes []int
}

type RemoteRequest struct {
	RequestID           string
	Query               string
	Scene               string
	Public              bool
	TheoryReleaseIDs    []int64
	SkillReleaseIDs     []int64
	EnneagramReleaseIDs []int64
	MainType            int
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
	ResolveConversation(ctx context.Context, userID, sessionID, cardID int64, requestedTypes []int) (ConversationResolution, error)
}

type PublicSearcher interface {
	SearchPublic(ctx context.Context, query string, topK int) ([]rag.Document, error)
}

type ReleaseSearcher interface {
	SearchReleaseChunks(ctx context.Context, releaseID int64, query string, topK int, minScore float64) ([]rag.Document, error)
}

type ReleaseSetSearcher interface {
	SearchReleaseSetChunks(ctx context.Context, releaseIDs []int64, query string, topK int, minScore float64) ([]rag.Document, error)
}

type Limits struct {
	Public        int
	Theory        int
	EnneagramType int
	TotalRunes    int
}

var defaultLimits = Limits{Public: 4, Theory: 3, EnneagramType: 3, TotalRunes: 8000}
var requestedTypeLimits = Limits{Public: 2, Theory: 3, EnneagramType: 9, TotalRunes: 5000}

var enneagramTypeCanonicalNames = [...]string{
	"", "完美型", "助人型", "成就型", "自我型", "思考型", "忠诚型", "活跃型", "领袖型", "和平型",
}

type LayerHit struct {
	LibraryID   int64        `json:"library_id,omitempty"`
	LibraryKey  string       `json:"library_key,omitempty"`
	ReleaseID   int64        `json:"release_id,omitempty"`
	LibraryIDs  []int64      `json:"library_ids,omitempty"`
	LibraryKeys []string     `json:"library_keys,omitempty"`
	ReleaseIDs  []int64      `json:"release_ids,omitempty"`
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
	requestedTypes := normalizeRequestedTypes(input.RequestedTypes)
	input.RequestedTypes = requestedTypes
	resolved, err := c.resolver.ResolveConversation(ctx, input.UserID, input.SessionID, input.CardID, requestedTypes)
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
			return c.retrieveLocal(ctx, input, resolved), nil
		}
		return c.retrieveRemote(ctx, resolved, remoteRequest, requestedTypes)
	case "fallback":
		if !c.userInRollout(input.UserID) {
			return c.retrieveLocal(ctx, input, resolved), nil
		}
		result, remoteErr := c.retrieveRemote(ctx, resolved, remoteRequest, requestedTypes)
		if remoteErr == nil && len(result.Documents) > 0 {
			return result, nil
		}
		if remoteErr == nil {
			remoteErr = ErrRemoteEmptyResult
		}
		if !RemoteErrorAllowsFallback(remoteErr) {
			return Result{}, remoteErr
		}
		if c.fallbackObserver != nil {
			c.fallbackObserver(remoteErr)
		}
	case "shadow":
		local := c.retrieveLocal(ctx, input, resolved)
		if c.remote != nil {
			go c.compareShadow(context.WithoutCancel(ctx), remoteRequest, local)
		}
		return local, nil
	}
	return c.retrieveLocal(ctx, input, resolved), nil
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

func (c *Coordinator) retrieveLocal(ctx context.Context, input Input, resolved ConversationResolution) Result {
	requestedTypes := normalizeRequestedTypes(input.RequestedTypes)
	explicitTypes := len(requestedTypes) > 0
	limits := c.limits
	if explicitTypes {
		limits = requestedTypeLimits
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
	trace.EnneagramType = tracedEnneagramType(resolved.MainType, requestedTypes)
	for _, diagnostic := range resolved.Diagnostics {
		addLayerDiagnostic(trace.LayerHits, diagnostic)
	}

	theoryBindings, strictBookScope := theoryBindingsForQuery(resolved.Resolution, input.Query)
	var publicDocs []rag.Document
	if !strictBookScope {
		publicDocs = c.searchPublic(ctx, input.Query, limits.Public, &trace)
	}
	theoryDocs := c.searchTheoryBindings(ctx, input.Query, theoryBindings, limits.Theory, &trace)

	documentsByLayer := map[string][]rag.Document{
		LayerPublic: publicDocs, LayerTheory: theoryDocs,
	}
	typeLayers := []string{LayerEnneagramType}
	if strictBookScope {
		typeLayers = nil
	} else if explicitTypes {
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
	theoryBindings, strictBookScope := theoryBindingsForQuery(resolved.Resolution, input.Query)
	request := RemoteRequest{RequestID: requestID, Query: input.Query, Scene: scene, Public: !strictBookScope, MainType: resolved.MainType}
	for _, binding := range theoryBindings {
		if isSkillLibraryBinding(binding) {
			request.SkillReleaseIDs = append(request.SkillReleaseIDs, binding.ReleaseID)
		} else {
			request.TheoryReleaseIDs = append(request.TheoryReleaseIDs, binding.ReleaseID)
		}
	}
	if strictBookScope {
		return request
	}
	if len(input.RequestedTypes) > 0 {
		for _, binding := range resolved.RequestedTypeBindings {
			if binding != nil {
				request.EnneagramReleaseIDs = append(request.EnneagramReleaseIDs, binding.ReleaseID)
			}
		}
	} else if resolved.EnneagramType != nil {
		request.EnneagramReleaseIDs = []int64{resolved.EnneagramType.ReleaseID}
	}
	return request
}

func isSkillLibraryBinding(binding *Binding) bool {
	if binding == nil {
		return false
	}
	key := strings.ToLower(strings.TrimSpace(binding.LibraryKey))
	return strings.HasPrefix(key, "skill-") || strings.HasPrefix(key, "story-skill-")
}

func (c *Coordinator) retrieveRemote(ctx context.Context, resolved ConversationResolution, request RemoteRequest, requestedTypes []int) (Result, error) {
	if c.remote == nil {
		return Result{}, errors.New("remote knowledge retriever unavailable")
	}
	remote, err := c.remote.Retrieve(ctx, request)
	if err != nil {
		return Result{}, err
	}
	requestedTypes = normalizeRequestedTypes(requestedTypes)
	trace := Trace{CardID: resolved.CardID, CardRevision: resolved.CardRevision, LayerHits: map[string]LayerHit{
		LayerPublic: {LibraryKey: LayerPublic, ChunkIDs: []string{}, Diagnostics: []Diagnostic{{Layer: LayerPublic, Code: "remote_" + remote.RetrievalMethod}}},
		LayerTheory: {ChunkIDs: []string{}},
	}}
	if len(requestedTypes) == 0 {
		trace.LayerHits[LayerEnneagramType] = LayerHit{ChunkIDs: []string{}}
	} else {
		for _, typeNumber := range requestedTypes {
			trace.LayerHits[typeTraceKey(typeNumber)] = LayerHit{ChunkIDs: []string{}}
		}
	}
	for _, binding := range resolved.TheoryBindings {
		appendBindingMetadata(&trace, LayerTheory, binding)
	}
	if len(resolved.TheoryBindings) == 0 {
		appendBindingMetadata(&trace, LayerTheory, resolved.Theory)
	}
	for _, binding := range resolved.RequestedTypeBindings {
		if binding != nil && binding.EnneagramType != nil {
			appendBindingMetadata(&trace, typeTraceKey(*binding.EnneagramType), binding)
		}
	}
	if len(requestedTypes) == 0 {
		appendBindingMetadata(&trace, LayerEnneagramType, resolved.EnneagramType)
	}
	for _, document := range remote.Documents {
		library, releaseID := remoteDocumentScope(document)
		layer := LayerPublic
		switch library {
		case "theory", "skill":
			layer = LayerTheory
		case "enneagram":
			layer = remoteEnneagramTraceLayer(resolved, requestedTypes, releaseID)
		}
		hit := trace.LayerHits[layer]
		hit.ChunkIDs = append(hit.ChunkIDs, document.ID)
		trace.LayerHits[layer] = hit
	}
	scopedFallbacks := c.supplementMissingRemoteTypeScopes(ctx, request, resolved, requestedTypes, &trace, remote.Documents)
	if len(scopedFallbacks) > 0 {
		remote.Documents = append(scopedFallbacks, remote.Documents...)
	}
	trace.EnneagramType = tracedEnneagramType(resolved.MainType, requestedTypes)
	trace.TraceID = remote.TraceID
	trace.RetrievalMethod = remote.RetrievalMethod
	return Result{Documents: remote.Documents, Citations: remote.Citations, Trace: trace}, nil
}

func (c *Coordinator) supplementMissingRemoteTypeScopes(ctx context.Context, request RemoteRequest, resolved ConversationResolution, requestedTypes []int, trace *Trace, remoteDocuments []rag.Document) []rag.Document {
	if c == nil || c.releases == nil || trace == nil || len(request.EnneagramReleaseIDs) == 0 {
		return nil
	}
	requestedTypes = normalizeRequestedTypes(requestedTypes)
	if len(requestedTypes) > 0 {
		missingTypes := make([]int, 0, len(requestedTypes))
		for _, typeNumber := range requestedTypes {
			if len(trace.LayerHits[typeTraceKey(typeNumber)].ChunkIDs) == 0 {
				missingTypes = append(missingTypes, typeNumber)
			}
		}
		if len(missingTypes) == 0 {
			return nil
		}
		documentsByLayer := c.searchRequestedTypes(ctx, request.Query, missingTypes, resolved.RequestedTypeBindings, trace)
		fallbacks := make([]rag.Document, 0, len(missingTypes))
		for _, typeNumber := range missingTypes {
			layer := typeTraceKey(typeNumber)
			for _, document := range documentsByLayer[layer] {
				hit := trace.LayerHits[layer]
				hit.ChunkIDs = append(hit.ChunkIDs, document.ID)
				trace.LayerHits[layer] = hit
				addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: layer, Code: "local_scope_fallback"})
				if !documentIDExists(remoteDocuments, document.ID) && !documentIDExists(fallbacks, document.ID) {
					fallbacks = append(fallbacks, document)
				}
			}
		}
		return fallbacks
	}

	if resolved.MainType < 1 || resolved.MainType > 9 || len(trace.LayerHits[LayerEnneagramType].ChunkIDs) > 0 {
		return nil
	}
	temporaryLayer := typeTraceKey(resolved.MainType)
	temporaryTrace := Trace{LayerHits: map[string]LayerHit{temporaryLayer: {ChunkIDs: []string{}}}}
	documentsByLayer := c.searchRequestedTypes(
		ctx,
		request.Query,
		[]int{resolved.MainType},
		[]*Binding{resolved.EnneagramType},
		&temporaryTrace,
	)
	documents := documentsByLayer[temporaryLayer]
	if len(documents) == 0 {
		return nil
	}
	hit := trace.LayerHits[LayerEnneagramType]
	temporaryHit := temporaryTrace.LayerHits[temporaryLayer]
	if temporaryHit.LibraryID > 0 {
		hit.LibraryID = temporaryHit.LibraryID
		hit.LibraryKey = temporaryHit.LibraryKey
		hit.ReleaseID = temporaryHit.ReleaseID
	}
	hit.Diagnostics = append(hit.Diagnostics, temporaryHit.Diagnostics...)
	hit.ChunkIDs = append(hit.ChunkIDs, documents[0].ID)
	trace.LayerHits[LayerEnneagramType] = hit
	addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "local_scope_fallback"})
	if documentIDExists(remoteDocuments, documents[0].ID) {
		return nil
	}
	return documents[:1]
}

func documentIDExists(documents []rag.Document, documentID string) bool {
	for _, document := range documents {
		if document.ID == documentID {
			return true
		}
	}
	return false
}

func tracedEnneagramType(mainType int, requestedTypes []int) *int {
	requestedTypes = normalizeRequestedTypes(requestedTypes)
	if len(requestedTypes) == 1 {
		value := requestedTypes[0]
		return &value
	}
	if len(requestedTypes) > 1 || mainType < 1 || mainType > 9 {
		return nil
	}
	value := mainType
	return &value
}

func appendBindingMetadata(trace *Trace, layer string, binding *Binding) {
	if trace == nil || binding == nil {
		return
	}
	hit := trace.LayerHits[layer]
	if hit.LibraryID == 0 {
		hit.LibraryID, hit.LibraryKey, hit.ReleaseID = binding.LibraryID, binding.LibraryKey, binding.ReleaseID
	}
	if !containsBindingID(hit.LibraryIDs, binding.LibraryID) {
		hit.LibraryIDs = append(hit.LibraryIDs, binding.LibraryID)
		hit.LibraryKeys = append(hit.LibraryKeys, binding.LibraryKey)
		hit.ReleaseIDs = append(hit.ReleaseIDs, binding.ReleaseID)
	}
	trace.LayerHits[layer] = hit
}

func containsBindingID(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func remoteDocumentScope(document rag.Document) (string, int64) {
	var library string
	var releaseID int64
	for _, tag := range document.Tags {
		if value, ok := strings.CutPrefix(tag, "library:"); ok {
			library = strings.TrimSpace(value)
		}
		if value, ok := strings.CutPrefix(tag, "release:"); ok {
			releaseID, _ = strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		}
	}
	return library, releaseID
}

func remoteEnneagramTraceLayer(resolved ConversationResolution, requestedTypes []int, releaseID int64) string {
	for _, binding := range resolved.RequestedTypeBindings {
		if binding != nil && binding.EnneagramType != nil && binding.ReleaseID == releaseID {
			return typeTraceKey(*binding.EnneagramType)
		}
	}
	if len(requestedTypes) == 1 {
		return typeTraceKey(requestedTypes[0])
	}
	return LayerEnneagramType
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

func (c *Coordinator) searchTheoryBindings(ctx context.Context, query string, bindings []*Binding, limit int, trace *Trace) []rag.Document {
	if len(bindings) == 0 || limit == 0 {
		return nil
	}
	if len(bindings) == 1 {
		return c.searchBinding(ctx, query, bindings[0], limit, LayerTheory, trace)
	}

	hit := trace.LayerHits[LayerTheory]
	hit.LibraryID = bindings[0].LibraryID
	hit.LibraryKey = bindings[0].LibraryKey
	hit.ReleaseID = bindings[0].ReleaseID
	for _, binding := range bindings {
		hit.LibraryIDs = append(hit.LibraryIDs, binding.LibraryID)
		hit.LibraryKeys = append(hit.LibraryKeys, binding.LibraryKey)
		hit.ReleaseIDs = append(hit.ReleaseIDs, binding.ReleaseID)
	}
	trace.LayerHits[LayerTheory] = hit
	if c.releases == nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerTheory, Code: "search_unavailable"})
		return nil
	}

	if setSearcher, ok := c.releases.(ReleaseSetSearcher); ok {
		documents, err := setSearcher.SearchReleaseSetChunks(ctx, hit.ReleaseIDs, query, searchCandidateLimit(limit), 0.2)
		if err != nil {
			addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerTheory, Code: "search_failed"})
			return nil
		}
		return documents
	}

	documents := make([]rag.Document, 0, len(bindings)*limit)
	for _, binding := range bindings {
		matches, err := c.releases.SearchReleaseChunks(ctx, binding.ReleaseID, query, searchCandidateLimit(limit), 0.2)
		if err != nil {
			addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerTheory, Code: "search_failed"})
			continue
		}
		documents = append(documents, matches...)
	}
	return documents
}

func theoryBindingsForQuery(resolution Resolution, query string) ([]*Binding, bool) {
	bindings := configuredTheoryBindings(resolution)
	titles := explicitBookTitles(query)
	if len(titles) == 0 {
		return bindings, false
	}
	matched := make([]*Binding, 0, len(bindings))
	for _, binding := range bindings {
		libraryName := normalizeBookTitle(binding.LibraryName)
		if libraryName == "" {
			continue
		}
		for _, title := range titles {
			if strings.Contains(libraryName, title) || strings.Contains(title, libraryName) {
				matched = append(matched, binding)
				break
			}
		}
	}
	if len(matched) == 0 {
		return bindings, false
	}
	return matched, true
}

func explicitBookTitles(query string) []string {
	var titles []string
	for {
		start := strings.Index(query, "《")
		if start < 0 {
			break
		}
		query = query[start+len("《"):]
		end := strings.Index(query, "》")
		if end < 0 {
			break
		}
		if title := normalizeBookTitle(query[:end]); title != "" {
			titles = append(titles, title)
		}
		query = query[end+len("》"):]
	}
	return titles
}

func normalizeBookTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.NewReplacer(
		" ", "", "\t", "", "\n", "", "\r", "",
		"（", "", "）", "", "(", "", ")", "",
		"·", "", "-", "", "_", "",
		"第5版", "", "第五版", "", "5th", "",
	).Replace(value)
}

func configuredTheoryBindings(resolution Resolution) []*Binding {
	source := resolution.TheoryBindings
	if len(source) == 0 && resolution.Theory != nil {
		source = []*Binding{resolution.Theory}
	}
	bindings := make([]*Binding, 0, len(source))
	seen := make(map[int64]struct{}, len(source))
	for _, binding := range source {
		if binding == nil || binding.ReleaseID <= 0 {
			continue
		}
		if _, duplicate := seen[binding.ReleaseID]; duplicate {
			continue
		}
		seen[binding.ReleaseID] = struct{}{}
		bindings = append(bindings, binding)
	}
	return bindings
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
	hit := trace.LayerHits[LayerEnneagramType]
	hit.LibraryID = binding.LibraryID
	hit.LibraryKey = binding.LibraryKey
	hit.ReleaseID = binding.ReleaseID
	trace.LayerHits[LayerEnneagramType] = hit
	if c.releases == nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "search_unavailable"})
		return nil
	}
	documents, err := c.releases.SearchReleaseChunks(ctx, binding.ReleaseID, query, searchCandidateLimit(limit), 0.2)
	if err != nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "search_failed"})
		return nil
	}
	filtered := documents[:0]
	for _, document := range documents {
		if documentMatchesType(document, resolved.MainType) {
			filtered = append(filtered, document)
			continue
		}
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "cross_type_document"})
	}
	if len(filtered) > 0 {
		return filtered
	}

	fallback, err := c.releases.SearchReleaseChunks(
		ctx,
		binding.ReleaseID,
		requestedTypeSearchQuery(query, resolved.MainType),
		searchCandidateLimit(1),
		0,
	)
	if err != nil {
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "search_failed"})
		return nil
	}
	for _, document := range fallback {
		if !documentMatchesType(document, resolved.MainType) {
			addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "cross_type_document"})
			continue
		}
		document.Content = truncateRunes(document.Content, 360)
		addLayerDiagnostic(trace.LayerHits, Diagnostic{Layer: LayerEnneagramType, Code: "local_scope_fallback"})
		return []rag.Document{document}
	}
	return nil
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
			typeQuery := requestedTypeSearchQuery(query, typeNumber)
			documents, err := c.releases.SearchReleaseChunks(ctx, binding.ReleaseID, typeQuery, searchCandidateLimit(1), 0)
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

func requestedTypeSearchQuery(query string, typeNumber int) string {
	if typeNumber < 1 || typeNumber >= len(enneagramTypeCanonicalNames) {
		return ""
	}
	return fmt.Sprintf(
		"%s\n九型人格检索锚点：%d号%s；核心欲望、核心恐惧、防御机制、压力表现、关系模式、成长方向",
		strings.TrimSpace(query),
		typeNumber,
		enneagramTypeCanonicalNames[typeNumber],
	)
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
