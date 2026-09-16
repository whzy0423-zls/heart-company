package observability

import (
	"sync/atomic"
	"time"
)

// Metrics contains low-cardinality process metrics for the realtime path.
// Values are intentionally process-local; aggregate them across instances.
type Metrics struct {
	activeRealtime              atomic.Int64
	ttsInFlight                 atomic.Int64
	ttsTotal                    atomic.Uint64
	ttsErrors                   atomic.Uint64
	ttsWaitNanos                atomic.Uint64
	ttsRunNanos                 atomic.Uint64
	llmTotal                    atomic.Uint64
	llmErrors                   atomic.Uint64
	llmRunNanos                 atomic.Uint64
	knowledgeRemoteTotal        atomic.Uint64
	knowledgeRemoteErrors       atomic.Uint64
	knowledgeRemoteRunNanos     atomic.Uint64
	knowledgeShadowTotal        atomic.Uint64
	knowledgeShadowErrors       atomic.Uint64
	knowledgeShadowOverlap      atomic.Uint64
	knowledgeShadowComparedDocs atomic.Uint64
	knowledgeShadowRunNanos     atomic.Uint64
	knowledgeFallbackTotal      atomic.Uint64
}

type Snapshot struct {
	ActiveRealtime              int64   `json:"activeRealtime"`
	TTSTotal                    uint64  `json:"ttsTotal"`
	TTSInFlight                 int64   `json:"ttsInFlight"`
	TTSErrors                   uint64  `json:"ttsErrors"`
	TTSWaitMsTotal              float64 `json:"ttsWaitMsTotal"`
	TTSRunMsTotal               float64 `json:"ttsRunMsTotal"`
	LLMTotal                    uint64  `json:"llmTotal"`
	LLMErrors                   uint64  `json:"llmErrors"`
	LLMRunMsTotal               float64 `json:"llmRunMsTotal"`
	KnowledgeRemoteTotal        uint64  `json:"knowledgeRemoteTotal"`
	KnowledgeRemoteErrors       uint64  `json:"knowledgeRemoteErrors"`
	KnowledgeRemoteRunMsTotal   float64 `json:"knowledgeRemoteRunMsTotal"`
	KnowledgeShadowTotal        uint64  `json:"knowledgeShadowTotal"`
	KnowledgeShadowErrors       uint64  `json:"knowledgeShadowErrors"`
	KnowledgeShadowOverlap      uint64  `json:"knowledgeShadowOverlap"`
	KnowledgeShadowComparedDocs uint64  `json:"knowledgeShadowComparedDocs"`
	KnowledgeShadowOverlapRate  float64 `json:"knowledgeShadowOverlapRate"`
	KnowledgeShadowRunMsTotal   float64 `json:"knowledgeShadowRunMsTotal"`
	KnowledgeFallbackTotal      uint64  `json:"knowledgeFallbackTotal"`
}

func New() *Metrics { return &Metrics{} }

func (m *Metrics) RealtimeOpened() {
	if m != nil {
		m.activeRealtime.Add(1)
	}
}
func (m *Metrics) RealtimeClosed() {
	if m != nil {
		m.activeRealtime.Add(-1)
	}
}
func (m *Metrics) TTSEnter(wait time.Duration) {
	if m != nil {
		m.ttsInFlight.Add(1)
		m.ttsTotal.Add(1)
		m.ttsWaitNanos.Add(uint64(wait))
	}
}
func (m *Metrics) TTSExit(run time.Duration, err error) {
	if m != nil {
		m.ttsInFlight.Add(-1)
		m.ttsRunNanos.Add(uint64(run))
		if err != nil {
			m.ttsErrors.Add(1)
		}
	}
}
func (m *Metrics) LLMExit(run time.Duration, err error) {
	if m != nil {
		m.llmTotal.Add(1)
		m.llmRunNanos.Add(uint64(run))
		if err != nil {
			m.llmErrors.Add(1)
		}
	}
}

func (m *Metrics) KnowledgeRemoteExit(run time.Duration, err error) {
	if m == nil {
		return
	}
	m.knowledgeRemoteTotal.Add(1)
	m.knowledgeRemoteRunNanos.Add(uint64(run))
	if err != nil {
		m.knowledgeRemoteErrors.Add(1)
	}
}

func (m *Metrics) KnowledgeShadowCompared(localIDs, remoteIDs []string, err error, run time.Duration) {
	if m == nil {
		return
	}
	m.knowledgeShadowTotal.Add(1)
	m.knowledgeShadowRunNanos.Add(uint64(run))
	if err != nil {
		m.knowledgeShadowErrors.Add(1)
		return
	}
	remote := make(map[string]struct{}, len(remoteIDs))
	for _, id := range remoteIDs {
		remote[id] = struct{}{}
	}
	var overlap uint64
	for _, id := range localIDs {
		if _, ok := remote[id]; ok {
			overlap++
		}
	}
	m.knowledgeShadowOverlap.Add(overlap)
	m.knowledgeShadowComparedDocs.Add(uint64(len(localIDs)))
}

func (m *Metrics) KnowledgeFallback() {
	if m != nil {
		m.knowledgeFallbackTotal.Add(1)
	}
}

func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	compared := m.knowledgeShadowComparedDocs.Load()
	overlap := m.knowledgeShadowOverlap.Load()
	overlapRate := 0.0
	if compared > 0 {
		overlapRate = float64(overlap) / float64(compared)
	}
	return Snapshot{
		ActiveRealtime: m.activeRealtime.Load(), TTSTotal: m.ttsTotal.Load(), TTSInFlight: m.ttsInFlight.Load(),
		TTSErrors: m.ttsErrors.Load(), TTSWaitMsTotal: float64(m.ttsWaitNanos.Load()) / float64(time.Millisecond),
		TTSRunMsTotal: float64(m.ttsRunNanos.Load()) / float64(time.Millisecond), LLMTotal: m.llmTotal.Load(),
		LLMErrors: m.llmErrors.Load(), LLMRunMsTotal: float64(m.llmRunNanos.Load()) / float64(time.Millisecond),
		KnowledgeRemoteTotal: m.knowledgeRemoteTotal.Load(), KnowledgeRemoteErrors: m.knowledgeRemoteErrors.Load(),
		KnowledgeRemoteRunMsTotal: float64(m.knowledgeRemoteRunNanos.Load()) / float64(time.Millisecond),
		KnowledgeShadowTotal:      m.knowledgeShadowTotal.Load(), KnowledgeShadowErrors: m.knowledgeShadowErrors.Load(),
		KnowledgeShadowOverlap: overlap, KnowledgeShadowComparedDocs: compared, KnowledgeShadowOverlapRate: overlapRate,
		KnowledgeShadowRunMsTotal: float64(m.knowledgeShadowRunNanos.Load()) / float64(time.Millisecond),
		KnowledgeFallbackTotal:    m.knowledgeFallbackTotal.Load(),
	}
}
