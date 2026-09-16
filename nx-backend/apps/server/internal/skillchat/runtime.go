package skillchat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/answerhygiene"
	"nine-xing/nx-backend/apps/server/internal/appknowledge"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

const (
	skillHistoryLimit        = 20
	skillSearchLimit         = 6
	skillSearchMinScore      = 0.20
	skillContextRunes        = 4000
	skillContextChunkRunes   = 1600
	publicSourceSnippetRunes = 120
	skillPersistenceTimeout  = 5 * time.Second
)

type RuntimeStore interface {
	GetSession(context.Context, int64, int64) (Session, error)
	GetConversationState(context.Context, int64, int64) (chat.ConversationState, error)
	ListRecentMessages(context.Context, int64, int64, int) ([]chat.Message, error)
	SavePair(context.Context, int64, int64, GenerationTrace, string, string, json.RawMessage) (int64, error)
}

type skillSummaryStore interface {
	ListMessagesAfter(context.Context, int64, int64, int64) ([]chat.Message, error)
	UpdateConversationSummary(context.Context, int64, int64, int64, int64, string, int64) (bool, error)
}

type ReleaseSearcher interface {
	SearchReleaseChunks(context.Context, int64, string, int, float64) ([]rag.Document, error)
}

type Runtime struct {
	store          RuntimeStore
	searcher       ReleaseSearcher
	generator      rag.Generator
	remote         appknowledge.RemoteRetriever
	remoteBackend  string
	rolloutPercent int
}

type Result struct {
	Answer             string          `json:"answer"`
	Sources            []rag.Source    `json:"sources"`
	Suggestions        []string        `json:"suggestions"`
	Citations          []rag.Citation  `json:"citations,omitempty"`
	TraceID            string          `json:"traceId,omitempty"`
	RetrievalMethod    string          `json:"retrievalMethod,omitempty"`
	MessageID          int64           `json:"messageId"`
	GenerationRevision int64           `json:"-"`
	Trace              GenerationTrace `json:"-"`
}

type RuntimeOption func(*Runtime)

func WithRemoteKnowledge(backend string, remote appknowledge.RemoteRetriever, rolloutPercent int) RuntimeOption {
	return func(runtime *Runtime) {
		runtime.remoteBackend = strings.ToLower(strings.TrimSpace(backend))
		runtime.remote = remote
		if rolloutPercent < 0 {
			rolloutPercent = 0
		}
		if rolloutPercent > 100 {
			rolloutPercent = 100
		}
		runtime.rolloutPercent = rolloutPercent
	}
}

func NewRuntime(store RuntimeStore, searcher ReleaseSearcher, generator rag.Generator, options ...RuntimeOption) *Runtime {
	runtime := &Runtime{store: store, searcher: searcher, generator: generator, remoteBackend: "local", rolloutPercent: 100}
	for _, option := range options {
		option(runtime)
	}
	return runtime
}

func (r *Runtime) Ask(ctx context.Context, appUserID, sessionID int64, question string) (Result, error) {
	result, err := r.Generate(ctx, appUserID, sessionID, question, nil)
	if err != nil {
		return Result{}, err
	}
	sourcesJSON, _ := json.Marshal(result.Sources)
	messageID, err := r.store.SavePair(ctx, appUserID, sessionID, result.Trace, question, result.Answer, sourcesJSON)
	if err != nil {
		return Result{}, fmt.Errorf("save skill answer: %w", err)
	}
	result.MessageID = messageID
	return result, nil
}

func (r *Runtime) AskStream(ctx context.Context, appUserID, sessionID int64, question string, emit rag.StreamEmitter) (Result, error) {
	result, err := r.Generate(ctx, appUserID, sessionID, question, emit)
	if err != nil {
		return Result{}, err
	}
	sourcesJSON, _ := json.Marshal(result.Sources)
	persistCtx, cancelPersist := context.WithTimeout(context.WithoutCancel(ctx), skillPersistenceTimeout)
	defer cancelPersist()
	messageID, err := r.store.SavePair(persistCtx, appUserID, sessionID, result.Trace, question, result.Answer, sourcesJSON)
	if err != nil {
		return Result{}, fmt.Errorf("save skill answer: %w", err)
	}
	result.MessageID = messageID
	return result, nil
}

// Generate builds a skill-isolated answer without persisting a text message.
// Voice handlers use it before atomically saving the audio transcript + answer.
func (r *Runtime) Generate(ctx context.Context, appUserID, sessionID int64, question string, emit rag.StreamEmitter) (Result, error) {
	question = strings.TrimSpace(question)
	if r == nil || r.store == nil || r.searcher == nil || r.generator == nil {
		return Result{}, errors.New("skill chat runtime unavailable")
	}
	if appUserID <= 0 || sessionID <= 0 || question == "" || utf8.RuneCountInString(question) > 300 {
		return Result{}, ErrInvalidInput
	}
	session, err := r.store.GetSession(ctx, appUserID, sessionID)
	if err != nil {
		return Result{}, err
	}
	if !session.Runnable() {
		return Result{}, ErrVersionUnavailable
	}
	summary, history, err := r.loadConversationContext(ctx, appUserID, sessionID, session.GenerationRevision)
	if err != nil {
		return Result{}, err
	}
	retrieval, err := r.retrieveKnowledge(ctx, appUserID, session, question)
	if err != nil {
		return Result{}, fmt.Errorf("search skill knowledge: %w", err)
	}
	documents := retrieval.Documents
	generationSources, publicSources := skillSources(documents)
	input := rag.GenerateInput{
		History:             history,
		ConversationSummary: summary,
		Question:            question,
		Sources:             generationSources,
		RuntimeInstructions: skillRuntimeInstructions(session.Instructions, session.SafetyProfile),
	}
	var answer string
	if emit != nil {
		if streaming, ok := r.generator.(rag.StreamingGenerator); ok {
			answer, err = streaming.GenerateStream(ctx, input, emit)
		} else {
			answer, err = r.generator.Generate(ctx, input)
			if err == nil && strings.TrimSpace(answer) != "" {
				err = emit(answer)
			}
		}
	} else {
		answer, err = r.generator.Generate(ctx, input)
	}
	if err != nil {
		return Result{}, fmt.Errorf("generate skill answer: %w", err)
	}
	answer = answerhygiene.Clean(question, strings.TrimSpace(answer))
	if answer == "" {
		return Result{}, errors.New("skill chat: model returned empty answer")
	}
	trace := GenerationTrace{
		GenerationRevision: session.GenerationRevision,
		SkillVersionID:     session.SkillVersionID,
		TheoryReleaseID:    session.TheoryReleaseID,
		ChunkIDs:           skillChunkIDs(documents),
	}
	return Result{
		Answer: answer, Sources: publicSources, Suggestions: []string{},
		Citations: retrieval.Citations, TraceID: retrieval.TraceID, RetrievalMethod: retrieval.RetrievalMethod,
		GenerationRevision: session.GenerationRevision, Trace: trace,
	}, nil
}

func (r *Runtime) retrieveKnowledge(ctx context.Context, appUserID int64, session Session, question string) (appknowledge.RemoteResult, error) {
	local := func() (appknowledge.RemoteResult, error) {
		documents, err := r.searcher.SearchReleaseChunks(ctx, session.TheoryReleaseID, question, skillSearchLimit, skillSearchMinScore)
		return appknowledge.RemoteResult{Documents: documents, RetrievalMethod: "local"}, err
	}
	request := appknowledge.RemoteRequest{
		RequestID:        newSkillRequestID(session.ID),
		Query:            question,
		Scene:            "skill_chat",
		Public:           false,
		TheoryReleaseIDs: []int64{session.TheoryReleaseID},
	}
	selected := r.rolloutPercent >= 100 || (r.rolloutPercent > 0 && int(appUserID%100) < r.rolloutPercent)
	switch r.remoteBackend {
	case "langchain":
		if selected && r.remote != nil {
			result, err := r.remote.Retrieve(ctx, request)
			return result, err
		}
	case "fallback":
		if selected && r.remote != nil {
			result, err := r.remote.Retrieve(ctx, request)
			if err == nil {
				return result, nil
			}
			if !appknowledge.RemoteErrorAllowsFallback(err) {
				return appknowledge.RemoteResult{}, err
			}
		}
	case "shadow":
		if r.remote != nil {
			go func() { _, _ = r.remote.Retrieve(context.WithoutCancel(ctx), request) }()
		}
	}
	return local()
}

func newSkillRequestID(sessionID int64) string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err == nil {
		return hex.EncodeToString(value)
	}
	return fmt.Sprintf("skill-chat-%d-%d", sessionID, time.Now().UnixNano())
}

func skillChunkIDs(documents []rag.Document) []int64 {
	out := make([]int64, 0, len(documents))
	seen := make(map[int64]bool, len(documents))
	for _, document := range documents {
		value := strings.TrimPrefix(strings.TrimSpace(document.ID), "theory:")
		id, err := strconv.ParseInt(value, 10, 64)
		if err == nil && id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func skillRuntimeInstructions(instructions, safetyProfile string) string {
	parts := []string{strings.TrimSpace(instructions)}
	switch strings.TrimSpace(safetyProfile) {
	case "sensitive-relationships-v1":
		parts = append(parts, "安全规则：识别暴力、胁迫控制、性同意、自伤或即时危险信号。出现此类风险时优先帮助用户保障当下安全并联系当地紧急服务或合适的专业支持；不要把胁迫或暴力当作普通沟通冲突，也不要把共同协商作为高危情形的首要方案。")
	case "health-information-v1":
		parts = append(parts, "健康信息规则：只提供一般性知识与就医沟通准备，不作诊断、处方或药物调整建议，并明确证据和不确定性。出现急症或危险信号时优先建议及时联系当地急救或合格医疗专业人员，不得以技能回答延误就医。")
	case "sensitive-guidance-v1":
		parts = append(parts, "敏感议题规则：不作医学或心理诊断；出现自伤、他伤或即时危险信号时，优先建议联系当地紧急服务或合适的专业支持。")
	}
	return strings.Join(parts, "\n\n")
}

func (r *Runtime) loadConversationContext(ctx context.Context, appUserID, sessionID, generationRevision int64) (string, []rag.Message, error) {
	state, err := r.store.GetConversationState(ctx, appUserID, sessionID)
	if err != nil {
		return "", nil, fmt.Errorf("load skill session state: %w", err)
	}
	summary := strings.TrimSpace(state.Summary)
	compactStore, canCompact := r.store.(skillSummaryStore)
	summarizer, canSummarize := r.generator.(rag.ConversationSummarizer)
	if canCompact && canSummarize {
		messages, listErr := compactStore.ListMessagesAfter(ctx, appUserID, sessionID, state.SummaryThroughMessageID)
		valid := validSkillMessages(messages)
		if listErr == nil && len(valid) > skillHistoryLimit {
			oldCount := len(valid) - 12
			updated, summarizeErr := summarizer.SummarizeConversation(ctx, summary, skillHistory(valid[:oldCount]))
			updated = strings.TrimSpace(updated)
			if summarizeErr == nil && updated != "" {
				throughID := valid[oldCount-1].ID
				if changed, updateErr := compactStore.UpdateConversationSummary(ctx, appUserID, sessionID, generationRevision, state.SummaryThroughMessageID, updated, throughID); updateErr == nil && changed {
					summary = updated
					return summary, skillHistory(valid[oldCount:]), nil
				}
			}
		}
	}
	messages, err := r.store.ListRecentMessages(ctx, appUserID, sessionID, skillHistoryLimit)
	if err != nil {
		return "", nil, fmt.Errorf("load skill session history: %w", err)
	}
	return summary, skillHistory(messages), nil
}

func skillHistory(messages []chat.Message) []rag.Message {
	out := make([]rag.Message, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		content := strings.TrimSpace(message.EffectiveContent())
		if (role == "user" || role == "assistant") && content != "" {
			out = append(out, rag.Message{Role: role, Content: content})
		}
	}
	return out
}

func validSkillMessages(messages []chat.Message) []chat.Message {
	out := make([]chat.Message, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(message.Role)
		if (role == "user" || role == "assistant") && strings.TrimSpace(message.EffectiveContent()) != "" {
			out = append(out, message)
		}
	}
	return out
}

func skillSources(documents []rag.Document) ([]rag.Source, []rag.Source) {
	generation := make([]rag.Source, 0, len(documents))
	public := make([]rag.Source, 0, len(documents))
	remaining := skillContextRunes
	for _, document := range documents {
		content := strings.TrimSpace(document.Content)
		if content == "" || remaining <= 0 {
			continue
		}
		limit := skillContextChunkRunes
		if remaining < limit {
			limit = remaining
		}
		full := trimRunes(content, limit)
		generation = append(generation, rag.Source{ID: document.ID, Title: strings.TrimSpace(document.Title), Snippet: full})
		public = append(public, rag.Source{ID: document.ID, Title: strings.TrimSpace(document.Title), Snippet: trimRunes(content, publicSourceSnippetRunes)})
		remaining -= utf8.RuneCountInString(full)
	}
	return generation, public
}

func trimRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
