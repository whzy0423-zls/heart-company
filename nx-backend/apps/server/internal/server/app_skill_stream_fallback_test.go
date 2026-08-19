package server

import (
	"context"
	"errors"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/rag"
)

type skillStreamFallbackGenerator struct {
	streamDelta string
	streamErr   error
	answer      string
	generateN   int
}

func (g *skillStreamFallbackGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	g.generateN++
	return g.answer, nil
}

func (g *skillStreamFallbackGenerator) GenerateStream(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if g.streamDelta != "" && emit != nil {
		if err := emit(g.streamDelta); err != nil {
			return "", err
		}
	}
	return g.streamDelta, g.streamErr
}

func TestSkillStreamFallsBackBeforeAnyContent(t *testing.T) {
	upstream := &skillStreamFallbackGenerator{streamErr: errors.New("upstream stream failed"), answer: "同步降级回答"}
	generator := skillChatRuntimeGenerator{server: &Server{ragGen: upstream}}
	var deltas []string
	answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "怎么练习"}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil || answer != "同步降级回答" || upstream.generateN != 1 {
		t.Fatalf("answer=%q generateN=%d err=%v", answer, upstream.generateN, err)
	}
	if len(deltas) != 1 || deltas[0] != answer {
		t.Fatalf("deltas=%v", deltas)
	}
}

func TestSkillStreamDoesNotFallbackAfterPartialContent(t *testing.T) {
	upstream := &skillStreamFallbackGenerator{streamDelta: "部分回答", streamErr: errors.New("upstream stream failed"), answer: "不得重复"}
	generator := skillChatRuntimeGenerator{server: &Server{ragGen: upstream}}
	var deltas []string
	answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "怎么练习"}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if !errors.Is(err, upstream.streamErr) || answer != "部分回答" || upstream.generateN != 0 {
		t.Fatalf("answer=%q generateN=%d err=%v", answer, upstream.generateN, err)
	}
	if len(deltas) != 1 || deltas[0] != answer {
		t.Fatalf("deltas=%v", deltas)
	}
}
