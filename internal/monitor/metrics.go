package monitor

import (
	"sync"
	"time"
)

// Metrics are process-local counters; terminal text and credentials are never
// retained here. Tokens are only counted when the provider reports them.
type Metrics struct {
	mu       sync.Mutex
	profiles map[int64]ProfileMetrics
}
type ProfileMetrics struct {
	Calls         uint64 `json:"calls"`
	Failures      uint64 `json:"failures"`
	Timeouts      uint64 `json:"timeouts"`
	LastLatencyMS int64  `json:"last_latency_ms"`
	InputTokens   uint64 `json:"input_tokens"`
	OutputTokens  uint64 `json:"output_tokens"`
	TokenReports  uint64 `json:"token_reports"`
}
type Usage struct {
	InputTokens      uint64 `json:"input_tokens"`
	OutputTokens     uint64 `json:"output_tokens"`
	PromptTokens     uint64 `json:"prompt_tokens"`
	CompletionTokens uint64 `json:"completion_tokens"`
}

func (m *Metrics) record(id int64, elapsed time.Duration, failed, timeout bool, usage *Usage) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profiles == nil {
		m.profiles = map[int64]ProfileMetrics{}
	}
	v := m.profiles[id]
	v.Calls++
	if failed {
		v.Failures++
	}
	if timeout {
		v.Timeouts++
	}
	v.LastLatencyMS = elapsed.Milliseconds()
	if usage != nil {
		v.TokenReports++
		v.InputTokens += max(usage.InputTokens, usage.PromptTokens)
		v.OutputTokens += max(usage.OutputTokens, usage.CompletionTokens)
	}
	m.profiles[id] = v
}
func (m *Metrics) Snapshot() map[int64]ProfileMetrics {
	out := map[int64]ProfileMetrics{}
	if m == nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, v := range m.profiles {
		out[id] = v
	}
	return out
}
