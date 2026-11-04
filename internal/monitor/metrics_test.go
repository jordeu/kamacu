package monitor

import (
	"testing"
	"time"
)

func TestMetricsIncludeAnthropicCachesWithoutDoubleCountingOpenAI(t *testing.T) {
	m := &Metrics{}
	m.record(1, time.Second, false, false, &Usage{InputTokens: 2, CacheCreationInputTokens: 100, CacheReadInputTokens: 200, OutputTokens: 10, ReportedCostUSD: 0.1})
	u := &Usage{PromptTokens: 500, CompletionTokens: 20}
	u.PromptTokensDetails.CachedTokens = 300
	m.record(1, time.Second, false, false, u)
	got := m.Snapshot()[1]
	if got.InputTokens != 802 || got.CacheCreationInputTokens != 100 || got.CacheReadInputTokens != 500 || got.OutputTokens != 30 || got.ReportedCostUSD != 0.1 {
		t.Fatalf("%+v", got)
	}
}
