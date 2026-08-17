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
