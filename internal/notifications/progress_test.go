package notifications

import (
	"context"
	"sync/atomic"
	"testing"
)

type progressProvider struct {
	fakeProvider
	service *Service
	fetches atomic.Int32
	t       *testing.T
}

func (p *progressProvider) Enrich(ctx context.Context, source Source, event Event) (Event, error) {
	status, err := p.service.Status(ctx)
	if err != nil {
		p.t.Error(err)
		return event, err
	}
	if status.Processing == nil || status.Processing.Progress == nil || status.Processing.Progress.Stage != "fetching" || status.Processing.Progress.Total != 2 {
		p.t.Errorf("retrieval progress not available through status: %+v", status.Processing)
	}
	p.fetches.Add(1)
	return p.fakeProvider.Enrich(ctx, source, event)
}

func TestNotificationStatusExposesLiveProcessingProgress(t *testing.T) {
	s, model, _, source := fixture(t)
	provider := &progressProvider{service: s, t: t}
	s.Providers["github"] = provider
	seed(t, s, source, event())
	second := event()
	second.ExternalID, second.URL = "second", "https://github.com/acme/repo/pull/8"
	seed(t, s, source, second)
	calls := 0
	model.respond = func(prompt string) (any, error) {
		calls++
		status, err := s.Status(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		progress := status.Processing.Progress
		if progress == nil || progress.Stage != "analyzing" || progress.ModelPasses != int64(calls) || progress.Lookups != calls-1 || provider.fetches.Load() != 2 {
			t.Fatalf("incorrect live model progress: %+v", progress)
		}
		if calls == 1 {
			return AgentTurn{Action: "read", Reads: []AgentRead{{Name: "read_task", Arguments: `{"id":"T1","offset":0}`}}}, nil
		}
		return finalResponse(inputSnapshot(t, prompt), false), nil
	}
	p := processAgent(t, s)
	if p.Status != "completed" || p.Progress != nil || calls != 2 {
		t.Fatal(p, calls)
	}
}

func TestProgressCountsParallelWorkAndDoesNotLeakBetweenRuns(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := withNotificationRun(context.Background(), "active")
	s.setProcessingStage(ctx, "fetching", 100)
	if err := parallelNotifications(ctx, 100, 4, func(ctx context.Context, _ int) error {
		s.advanceProcessing(ctx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	p := s.processingProgress(Processing{ID: "active", Status: "running"})
	if p == nil || p.Completed != 100 || p.Total != 100 {
		t.Fatal(p)
	}
	p.Completed = 0
	preview := withNotificationRun(context.Background(), "preview:other")
	s.setProcessingStage(preview, "analyzing", 0)
	if got := s.processingProgress(Processing{ID: "active", Status: "running"}); got == nil || got.Completed != 100 {
		t.Fatal("preview or status consumer overwrote live progress", got)
	}
	for _, run := range []Processing{{ID: "active", Status: "failed"}, {ID: "active", Status: "completed"}, {ID: "other", Status: "running"}} {
		if got := s.processingProgress(run); got != nil {
			t.Fatal("unrelated or finished run exposed active progress", run, got)
		}
	}
}
