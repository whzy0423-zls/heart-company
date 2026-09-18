package skillchat

import (
	"context"
	"errors"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"reflect"
	"strings"
	"testing"
)

type sceneStoreStub struct {
	runtimeStoreStub
	initial string
}

func (s *sceneStoreStub) FirstQuestion(context.Context, int64, int64) (string, error) {
	return s.initial, nil
}

type sceneSearchStub struct {
	runtimeSearchStub
	keys  []string
	query string
}

func (s *sceneSearchStub) SearchEnneagramReleaseChunks(_ context.Context, id int64, q string, keys []string, _ int, _ float64) ([]rag.Document, error) {
	s.releaseID = id
	s.keys = keys
	s.query = q
	return []rag.Document{{ID: "theory:711", Content: "核心知识"}}, nil
}
func TestSceneKeepsInitialContextAndRoutesExplicitTypes(t *testing.T) {
	initial := "场景：职场沟通\n我：6 号\n对方：同事，9 号\n具体事件：" + strings.Repeat("项目延期需要协商。", 50) + "\n期望结果：明确分工\n问题：怎么说？"
	for _, followup := range []bool{false, true} {
		s := &sceneStoreStub{runtimeStoreStub: runtimeStoreStub{session: runnableRuntimeSession()}}
		s.session.SkillKey = "enneagram-personality-library"
		q := initial
		if followup {
			s.initial = initial
			q = "再给我一句简短的"
		}
		search := &sceneSearchStub{}
		gen := &runtimeGeneratorStub{answer: "先确认事实，再商量分工。"}
		_, err := NewRuntime(s, search, gen).Ask(context.Background(), 7, 41, q)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(search.keys, []string{"enneagram-core", "enneagram-type-06", "enneagram-type-09"}) {
			t.Fatalf("keys=%v", search.keys)
		}
		if !strings.Contains(gen.input.RuntimeInstructions, initial) || !strings.Contains(search.query, "项目延期") {
			t.Fatal("initial context lost")
		}
	}
}
func TestSceneUnknownTypeNeverGuessesFromEvent(t *testing.T) {
	keys := sceneKnowledgeKeys("场景：职场\n我：暂不确定（请勿猜型）\n对方：同事，暂不确定\n具体事件：我：3 号\n问题：他像8号吗")
	if !reflect.DeepEqual(keys, []string{"enneagram-core"}) {
		t.Fatalf("keys=%v", keys)
	}
}
func TestOrdinarySkillStillRejectsLongQuestion(t *testing.T) {
	_, err := NewRuntime(&runtimeStoreStub{session: runnableRuntimeSession()}, &runtimeSearchStub{}, &runtimeGeneratorStub{}).Ask(context.Background(), 7, 41, strings.Repeat("长", 301))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err=%v", err)
	}
}
