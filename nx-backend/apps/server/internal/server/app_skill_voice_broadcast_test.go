package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/skillchat"
)

func TestSkillChatStreamDoesNotWaitForSlowVoiceClose(t *testing.T) {
	provider := &blockingVoiceBroadcastProvider{started: make(chan struct{}), release: make(chan struct{})}
	store := &slowVoiceSkillRuntimeStore{}
	runtime := skillchat.NewRuntime(store, slowVoiceSkillSearcher{}, slowVoiceSkillGenerator{})
	s := &Server{
		skillChatRuntime: runtime,
		chatTimeout:      time.Second,
		voiceBroadcastPreferences: func() voiceBroadcastPreferenceStore {
			preferences := newMemoryVoiceBroadcastPreferenceStore()
			if err := preferences.Set(context.Background(), 7, true); err != nil {
				t.Fatal(err)
			}
			return preferences
		}(),
		voiceBroadcastConfigLoader: func(context.Context) (voiceBroadcastConfig, error) {
			return voiceBroadcastConfig{
				Enabled: true, Provider: voiceBroadcastProviderBailian, APIKey: "test",
				Model: voiceBroadcastDefaultModel, Voice: voiceBroadcastDefaultVoice,
			}, nil
		},
		voiceBroadcastSynthesizerFactory: func(context.Context) (voiceBroadcastSynthesizer, error) {
			return provider, nil
		},
	}
	request := httptest.NewRequest(http.MethodPost, "/api/app/skill-sessions/42/ask/stream", strings.NewReader(`{"question":"请回答"}`))
	writer := newAppChatBlockingStreamWriter()
	handlerDone := make(chan struct{})
	go func() {
		s.appSkillSessionAskStream(writer, request, 7)
		close(handlerDone)
	}()
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("skill voice provider did not start")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("skill text stream remained blocked by slow voice close")
	}
	close(provider.release)
	if body := writer.BodyString(); !strings.Contains(body, "event: done\n") {
		t.Fatalf("skill stream missing done event: %q", body)
	}
	if got := store.saves.Load(); got != 1 {
		t.Fatalf("skill answer save count=%d, want 1", got)
	}
}

type slowVoiceSkillRuntimeStore struct {
	saves atomic.Int32
}

func (s *slowVoiceSkillRuntimeStore) GetSession(context.Context, int64, int64) (skillchat.Session, error) {
	return skillchat.Session{
		ID: 42, SkillVersionID: 1, SkillKey: "fixture", Scene: "skill_chat",
		TheoryReleaseID: 1, VersionStatus: "published", LibraryStatus: "enabled",
		CategoryStatus: "enabled", SkillStatus: "enabled", GenerationRevision: 1,
	}, nil
}

func (*slowVoiceSkillRuntimeStore) GetConversationState(context.Context, int64, int64) (chat.ConversationState, error) {
	return chat.ConversationState{}, nil
}

func (*slowVoiceSkillRuntimeStore) ListRecentMessages(context.Context, int64, int64, int) ([]chat.Message, error) {
	return nil, nil
}

func (s *slowVoiceSkillRuntimeStore) SavePair(context.Context, int64, int64, skillchat.GenerationTrace, string, string, json.RawMessage) (int64, error) {
	s.saves.Add(1)
	return 101, nil
}

type slowVoiceSkillSearcher struct{}

func (slowVoiceSkillSearcher) SearchReleaseChunks(context.Context, int64, string, int, float64) ([]rag.Document, error) {
	return nil, nil
}

type slowVoiceSkillGenerator struct{}

func (slowVoiceSkillGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "技能回答。", nil
}

func (slowVoiceSkillGenerator) GenerateStream(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	const answer = "技能回答。"
	if err := emit(answer); err != nil {
		return "", err
	}
	return answer, nil
}

var _ rag.StreamingGenerator = slowVoiceSkillGenerator{}
var _ skillchat.RuntimeStore = (*slowVoiceSkillRuntimeStore)(nil)
var _ skillchat.ReleaseSearcher = slowVoiceSkillSearcher{}
