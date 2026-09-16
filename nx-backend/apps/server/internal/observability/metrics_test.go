package observability

import (
	"errors"
	"testing"
	"time"
)

func TestMetricsSnapshotTracksRealtimeTTSAndLLM(t *testing.T) {
	m := New()
	m.RealtimeOpened()
	m.TTSEnter(2 * time.Millisecond)
	m.TTSExit(3*time.Millisecond, errors.New("tts"))
	m.LLMExit(4*time.Millisecond, nil)
	s := m.Snapshot()
	if s.ActiveRealtime != 1 || s.TTSTotal != 1 || s.TTSInFlight != 0 || s.TTSErrors != 1 || s.LLMTotal != 1 || s.LLMErrors != 0 {
		t.Fatalf("snapshot=%+v", s)
	}
	if s.TTSWaitMsTotal < 2 || s.TTSRunMsTotal < 3 || s.LLMRunMsTotal < 4 {
		t.Fatalf("durations=%+v", s)
	}
}

func TestMetricsSnapshotTracksKnowledgeShadowRemoteAndFallbackWithoutPayloads(t *testing.T) {
	m := New()
	m.KnowledgeRemoteExit(125*time.Millisecond, nil)
	m.KnowledgeRemoteExit(75*time.Millisecond, errors.New("timeout"))
	m.KnowledgeShadowCompared([]string{"a", "b"}, []string{"b", "c"}, nil, 90*time.Millisecond)
	m.KnowledgeShadowCompared([]string{"a"}, nil, errors.New("failed"), 20*time.Millisecond)
	m.KnowledgeFallback()

	s := m.Snapshot()
	if s.KnowledgeRemoteTotal != 2 || s.KnowledgeRemoteErrors != 1 || s.KnowledgeRemoteRunMsTotal < 200 {
		t.Fatalf("remote metrics=%+v", s)
	}
	if s.KnowledgeShadowTotal != 2 || s.KnowledgeShadowErrors != 1 || s.KnowledgeShadowOverlap != 1 || s.KnowledgeShadowComparedDocs != 2 {
		t.Fatalf("shadow metrics=%+v", s)
	}
	if s.KnowledgeFallbackTotal != 1 {
		t.Fatalf("fallback metrics=%+v", s)
	}
}
