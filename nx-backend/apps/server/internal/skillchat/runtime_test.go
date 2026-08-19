package skillchat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestRuntimeUsesOnlyFixedSkillVersionAndCurrentSessionContext(t *testing.T) {
	store := &runtimeStoreStub{
		session: Session{
			ID: 41, AppUserID: 7, SkillID: 9, SkillVersionID: 91,
			SkillKey: "art-of-learning", SkillName: "学习之道", Version: "1.1.0",
			Instructions:    "只基于《学习之道》的方法帮助用户设计练习。",
			TheoryReleaseID: 71, VersionStatus: "published", Scene: "skill_chat",
			LibraryStatus: "enabled", CategoryStatus: "enabled", SkillStatus: "enabled",
			GenerationRevision: 3,
		},
		state: chat.ConversationState{Summary: "仅本会话摘要", SummaryThroughMessageID: 2},
		messages: []chat.Message{
			{ID: 1, SessionID: 41, Role: "user", Content: "我在练习象棋"},
			{ID: 2, SessionID: 41, Role: "assistant", Content: "先缩小练习圈"},
		},
	}
	search := &runtimeSearchStub{documents: []rag.Document{{ID: "theory:711", Title: "划小圈", Content: "把复杂技能拆成一个可以反复校准的小单元。"}}}
	gen := &runtimeGeneratorStub{answer: "先选一个局面主题，连续复盘三次。"}
	runtime := NewRuntime(store, search, gen)

	result, err := runtime.Ask(context.Background(), 7, 41, "我今天怎么练？")
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != gen.answer || result.MessageID != 88 {
		t.Fatalf("result=%+v", result)
	}
	if search.releaseID != 71 {
		t.Fatalf("release id=%d, want fixed release 71", search.releaseID)
	}
	if got := gen.input.RuntimeInstructions; got != store.session.Instructions {
		t.Fatalf("runtime instructions=%q", got)
	}
	if gen.input.ConversationSummary != "仅本会话摘要" || len(gen.input.History) != 2 {
		t.Fatalf("context=%+v", gen.input)
	}
	if gen.input.UserProfile.Nickname != "" || gen.input.UserProfile.MainType != 0 || len(gen.input.UserProfile.Memories) != 0 {
		t.Fatalf("skill runtime leaked user profile: %+v", gen.input.UserProfile)
	}
	if gen.input.ConversationCard != (rag.ConversationCard{}) || len(gen.input.UserPreferences) != 0 || len(gen.input.CurrentDirectives) != 0 {
		t.Fatalf("skill runtime leaked ordinary chat context: %+v", gen.input)
	}
	if len(gen.input.Sources) != 1 || gen.input.Sources[0].Snippet != search.documents[0].Content {
		t.Fatalf("generator did not receive full release chunk: %+v", gen.input.Sources)
	}
	if store.savedSessionID != 41 || store.savedQuestion != "我今天怎么练？" || store.savedAnswer != gen.answer {
		t.Fatalf("saved=%+v", store)
	}
	if store.savedRevision != 3 {
		t.Fatalf("saved revision=%d, want 3", store.savedRevision)
	}
	if store.savedTrace.GenerationRevision != 3 || store.savedTrace.SkillVersionID != 91 || store.savedTrace.TheoryReleaseID != 71 || len(store.savedTrace.ChunkIDs) != 1 || store.savedTrace.ChunkIDs[0] != 711 {
		t.Fatalf("saved trace=%+v", store.savedTrace)
	}
	traceJSON, _ := json.Marshal(store.savedTrace)
	for _, forbidden := range []string{"我今天怎么练", gen.answer, search.documents[0].Content} {
		if strings.Contains(string(traceJSON), forbidden) {
			t.Fatalf("trace leaked user/model content %q: %s", forbidden, traceJSON)
		}
	}
}

func TestSkillRuntimeInstructionsApplyPublishedSafetyProfile(t *testing.T) {
	relationship := skillRuntimeInstructions("亲密关系规则", "sensitive-relationships-v1")
	for _, expected := range []string{"亲密关系规则", "胁迫控制", "性同意", "即时危险", "不要把胁迫或暴力当作普通沟通冲突"} {
		if !strings.Contains(relationship, expected) {
			t.Fatalf("relationship safety instructions missing %q: %s", expected, relationship)
		}
	}
	health := skillRuntimeInstructions("健康技能规则", "health-information-v1")
	for _, expected := range []string{"健康技能规则", "不作诊断", "处方", "不得以技能回答延误就医"} {
		if !strings.Contains(health, expected) {
			t.Fatalf("health safety instructions missing %q: %s", expected, health)
		}
	}
}

func TestRuntimeRejectsCrossUserOrNonRunnableSessionBeforeRetrieval(t *testing.T) {
	store := &runtimeStoreStub{getErr: ErrNotFound}
	search := &runtimeSearchStub{}
	gen := &runtimeGeneratorStub{}
	_, err := NewRuntime(store, search, gen).Ask(context.Background(), 8, 41, "越权问题")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v", err)
	}
	if search.called || gen.called {
		t.Fatal("rejected session reached retrieval or model")
	}
}

func TestAskStreamPersistsWithIndependentContextAfterGenerationDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &runtimeStoreStub{session: runnableRuntimeSession()}
	gen := &runtimeGeneratorStub{
		answer:        "已经生成的回答",
		afterGenerate: cancel,
	}
	runtime := NewRuntime(store, &runtimeSearchStub{}, gen)

	result, err := runtime.AskStream(ctx, 7, 41, "怎么练习？", func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.MessageID != 88 || store.saveCtxErr != nil {
		t.Fatalf("result=%+v save context err=%v", result, store.saveCtxErr)
	}
}

func TestRuntimeRejectsAnswerWhenSessionWasClearedDuringGeneration(t *testing.T) {
	store := &runtimeStoreStub{session: runnableRuntimeSession(), saveErr: ErrSessionChanged}
	runtime := NewRuntime(store, &runtimeSearchStub{}, &runtimeGeneratorStub{answer: "旧上下文回答"})

	_, err := runtime.Ask(context.Background(), 7, 41, "清空前的问题")
	if !errors.Is(err, ErrSessionChanged) {
		t.Fatalf("err=%v, want ErrSessionChanged", err)
	}
	if store.savedRevision != store.session.GenerationRevision {
		t.Fatalf("saved revision=%d session revision=%d", store.savedRevision, store.session.GenerationRevision)
	}
}

func runnableRuntimeSession() Session {
	return Session{
		ID: 41, AppUserID: 7, SkillID: 9, SkillVersionID: 91,
		SkillKey: "art-of-learning", SkillName: "学习之道", Version: "1.1.0",
		Instructions: "仅使用当前技能", TheoryReleaseID: 71,
		VersionStatus: "published", Scene: "skill_chat", LibraryStatus: "enabled",
		CategoryStatus: "enabled", SkillStatus: "enabled",
		GenerationRevision: 9,
	}
}

type runtimeStoreStub struct {
	session        Session
	state          chat.ConversationState
	messages       []chat.Message
	getErr         error
	savedSessionID int64
	savedQuestion  string
	savedAnswer    string
	savedSources   json.RawMessage
	saveCtxErr     error
	saveErr        error
	savedRevision  int64
	savedTrace     GenerationTrace
}

func (s *runtimeStoreStub) GetSession(context.Context, int64, int64) (Session, error) {
	return s.session, s.getErr
}
func (s *runtimeStoreStub) GetConversationState(context.Context, int64, int64) (chat.ConversationState, error) {
	return s.state, nil
}
func (s *runtimeStoreStub) ListRecentMessages(context.Context, int64, int64, int) ([]chat.Message, error) {
	return append([]chat.Message(nil), s.messages...), nil
}
func (s *runtimeStoreStub) SavePair(ctx context.Context, _ int64, sessionID int64, trace GenerationTrace, question, answer string, sources json.RawMessage) (int64, error) {
	s.saveCtxErr = ctx.Err()
	if s.saveCtxErr != nil {
		return 0, s.saveCtxErr
	}
	s.savedSessionID, s.savedRevision, s.savedTrace, s.savedQuestion, s.savedAnswer, s.savedSources = sessionID, trace.GenerationRevision, trace, question, answer, sources
	if s.saveErr != nil {
		return 0, s.saveErr
	}
	return 88, nil
}

type runtimeSearchStub struct {
	documents []rag.Document
	releaseID int64
	called    bool
}

func (s *runtimeSearchStub) SearchReleaseChunks(_ context.Context, releaseID int64, _ string, _ int, _ float64) ([]rag.Document, error) {
	s.called, s.releaseID = true, releaseID
	return append([]rag.Document(nil), s.documents...), nil
}

type runtimeGeneratorStub struct {
	answer        string
	input         rag.GenerateInput
	called        bool
	afterGenerate func()
}

func (g *runtimeGeneratorStub) Generate(_ context.Context, input rag.GenerateInput) (string, error) {
	g.called, g.input = true, input
	if g.afterGenerate != nil {
		g.afterGenerate()
	}
	return g.answer, nil
}
