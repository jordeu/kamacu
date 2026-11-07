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
	TypingDeferrals          uint64  `json:"typing_deferrals"`
	InputSettledCalls        uint64  `json:"input_settled_calls"`
	GenerationDeferrals      uint64  `json:"generation_deferrals"`
	CosmeticReuses           uint64  `json:"cosmetic_reuses"`
	FallbackCalls            uint64  `json:"fallback_calls"`
	AcceptedDecisions        uint64  `json:"accepted_decisions"`
	StaleResults             uint64  `json:"stale_results"`
	ObsoleteResults          uint64  `json:"obsolete_results"`
	DroppedWorkUpdates       uint64  `json:"dropped_work_updates"`
	DroppedPRLinks           uint64  `json:"dropped_pr_links"`
	CacheCreationInputTokens uint64  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     uint64  `json:"cache_read_input_tokens"`
	ReportedCostUSD          float64 `json:"reported_cost_usd"`
	Canceled                 uint64  `json:"canceled"`
	MissingTokenReports      uint64  `json:"missing_token_reports"`
	Calls                    uint64  `json:"calls"`
	Failures                 uint64  `json:"failures"`
	Timeouts                 uint64  `json:"timeouts"`
	LastLatencyMS            int64   `json:"last_latency_ms"`
	InputTokens              uint64  `json:"input_tokens"`
	OutputTokens             uint64  `json:"output_tokens"`
	TokenReports             uint64  `json:"token_reports"`
}

// Deferrals count transitions into waiting for generation, not scheduler ticks.
func (m *Metrics) monitorEvent(id int64, event string, count uint64) {
	if m == nil || count == 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.profiles == nil {
		m.profiles = map[int64]ProfileMetrics{}
	}
	v := m.profiles[id]
	switch event {
	case "typing_deferred":
		v.TypingDeferrals += count
	case "input_settled":
		v.InputSettledCalls += count
	case "generation_deferred":
		v.GenerationDeferrals += count
	case "cosmetic_reuse":
		v.CosmeticReuses += count
	case "fallback":
		v.FallbackCalls += count
	case "accepted":
		v.AcceptedDecisions += count
	case "stale":
		v.StaleResults += count
	case "obsolete":
		v.ObsoleteResults += count
	case "dropped_work":
		v.DroppedWorkUpdates += count
	case "dropped_link":
		v.DroppedPRLinks += count
	}
	m.profiles[id] = v
}

type Usage struct {
	CacheCreationInputTokens uint64  `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     uint64  `json:"cache_read_input_tokens"`
	ReportedCostUSD          float64 `json:"reported_cost_usd,omitempty"`
	PromptTokensDetails      struct {
		CachedTokens uint64 `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
	InputTokens      uint64 `json:"input_tokens"`
	OutputTokens     uint64 `json:"output_tokens"`
	PromptTokens     uint64 `json:"prompt_tokens"`
	CompletionTokens uint64 `json:"completion_tokens"`
}

func (m *Metrics) record(id int64, elapsed time.Duration, failed, timeout bool, usage *Usage, canceled bool) {
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
	if canceled {
		v.Canceled++
	}
	if failed {
		v.Failures++
	}
	if timeout {
		v.Timeouts++
	}
	v.LastLatencyMS = elapsed.Milliseconds()
	if usage == nil {
		v.MissingTokenReports++
	}
	if usage != nil {
		v.TokenReports++
		// Anthropic's input count excludes cached tokens; OpenAI's prompt count
		// includes them. Never add OpenAI's cached subset a second time.
		v.InputTokens += max(usage.InputTokens+usage.CacheCreationInputTokens+usage.CacheReadInputTokens, usage.PromptTokens)
		v.CacheCreationInputTokens += usage.CacheCreationInputTokens
		v.CacheReadInputTokens += max(usage.CacheReadInputTokens, usage.PromptTokensDetails.CachedTokens)
		v.ReportedCostUSD += usage.ReportedCostUSD
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
