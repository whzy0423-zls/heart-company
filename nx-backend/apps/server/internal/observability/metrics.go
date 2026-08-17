package observability

import (
	"sync/atomic"
	"time"
)

// Metrics contains low-cardinality process metrics for the realtime path.
// Values are intentionally process-local; aggregate them across instances.
type Metrics struct {
	activeRealtime atomic.Int64
	ttsInFlight    atomic.Int64
	ttsTotal       atomic.Uint64
	ttsErrors      atomic.Uint64
	ttsWaitNanos   atomic.Uint64
	ttsRunNanos    atomic.Uint64
	llmTotal       atomic.Uint64
	llmErrors      atomic.Uint64
	llmRunNanos    atomic.Uint64
}

type Snapshot struct {
	ActiveRealtime int64   `json:"activeRealtime"`
	TTSTotal       uint64  `json:"ttsTotal"`
	TTSInFlight    int64   `json:"ttsInFlight"`
	TTSErrors      uint64  `json:"ttsErrors"`
	TTSWaitMsTotal float64 `json:"ttsWaitMsTotal"`
	TTSRunMsTotal  float64 `json:"ttsRunMsTotal"`
	LLMTotal       uint64  `json:"llmTotal"`
	LLMErrors      uint64  `json:"llmErrors"`
	LLMRunMsTotal  float64 `json:"llmRunMsTotal"`
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

func (m *Metrics) Snapshot() Snapshot {
	if m == nil {
		return Snapshot{}
	}
	return Snapshot{
		ActiveRealtime: m.activeRealtime.Load(), TTSTotal: m.ttsTotal.Load(), TTSInFlight: m.ttsInFlight.Load(),
		TTSErrors: m.ttsErrors.Load(), TTSWaitMsTotal: float64(m.ttsWaitNanos.Load()) / float64(time.Millisecond),
		TTSRunMsTotal: float64(m.ttsRunNanos.Load()) / float64(time.Millisecond), LLMTotal: m.llmTotal.Load(),
		LLMErrors: m.llmErrors.Load(), LLMRunMsTotal: float64(m.llmRunNanos.Load()) / float64(time.Millisecond),
	}
}
