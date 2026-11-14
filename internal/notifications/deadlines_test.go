package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kamacu/internal/monitor"
)

type contextTestModel func(context.Context, string, func([]byte) error) ([]byte, error)

func (m contextTestModel) GenerateValidatedJSON(ctx context.Context, _ monitor.Profile, prompt string, _ any, validate func([]byte) error) ([]byte, error) {
	return m(ctx, prompt, validate)
}

type delayedNotificationProvider struct{ fakeProvider }

func (p *delayedNotificationProvider) Enrich(ctx context.Context, source Source, event Event) (Event, error) {
	select {
	case <-time.After(425 * time.Millisecond):
		return p.fakeProvider.Enrich(ctx, source, event)
	case <-ctx.Done():
		return event, ctx.Err()
	}
}

func TestBatchModelCallsDoNotShareSourceOrEarlierCallDeadline(t *testing.T) {
	s, _, _, source := fixture(t)
	// Scale the production timeout down for this regression. Setup and two
	// individually valid model calls together exceed the configured second.
	if _, err := s.DB.Exec(`PRAGMA ignore_check_constraints=ON; UPDATE notification_settings SET timeout_seconds=1,max_reads=1; PRAGMA ignore_check_constraints=OFF`); err != nil {
		t.Fatal(err)
	}
	s.Providers["github"] = &delayedNotificationProvider{}
	seed(t, s, source, event())
	calls := 0
	s.Model = contextTestModel(func(ctx context.Context, prompt string, validate func([]byte) error) ([]byte, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 900*time.Millisecond || time.Until(deadline) > time.Second {
			t.Fatal("model call did not receive a fresh bounded deadline", deadline)
		}
		select {
		case <-time.After(425 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		turn := finalResponse(inputSnapshot(t, prompt), false)
		if calls == 1 {
			turn = AgentTurn{Action: "read", Reads: []AgentRead{{Name: "read_task", Arguments: `{"id":"T1","offset":0}`}}}
		}
		raw, err := json.Marshal(turn)
		if err == nil {
			err = validate(raw)
		}
		return raw, err
	})
	started := time.Now()
	p := processAgent(t, s)
	if p.Status != "completed" || calls != 2 || time.Since(started) <= time.Second {
		t.Fatal("valid multi-step batch was cut short", p, calls, time.Since(started))
	}
}

func TestModelCallTimeoutKeepsNotificationsPendingWithoutRepairRetry(t *testing.T) {
	s, _, _, source := fixture(t)
	// Only this isolated test DB bypasses the production 30-second minimum.
	if _, err := s.DB.Exec(`PRAGMA ignore_check_constraints=ON; UPDATE notification_settings SET timeout_seconds=1; PRAGMA ignore_check_constraints=OFF`); err != nil {
		t.Fatal(err)
	}
	seed(t, s, source, event())
	calls := 0
	s.Model = contextTestModel(func(ctx context.Context, _ string, validate func([]byte) error) ([]byte, error) {
		calls++
		// A provider can have rejected/partial output before its deadline.
		// Timing out must not start another paid contract-repair attempt.
		_ = validate([]byte(`{}`))
		<-ctx.Done()
		return nil, ctx.Err()
	})
	p := processAgent(t, s)
	if p.Status != "failed" || !strings.Contains(p.Error, "model call exceeded its 1-second deadline") || !strings.Contains(p.Error, "context deadline exceeded") || calls != 1 {
		t.Fatal(p, calls)
	}
	var pending int
	if err := s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='pending'`).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("timed-out batch consumed notification", pending, err)
	}
}

type waitingNotificationTools struct {
	fakeTools
	cancelParent context.CancelFunc
}

func (w *waitingNotificationTools) Call(ctx context.Context, _ string, _ json.RawMessage) (string, error) {
	if w.cancelParent != nil {
		w.cancelParent()
	}
	<-ctx.Done()
	return "", ctx.Err()
}

func TestReadDeadlineOrCancellationStopsBatchBeforeAnotherModelCall(t *testing.T) {
	for _, cancelParent := range []bool{false, true} {
		name := "lookup deadline"
		if cancelParent {
			name = "parent cancellation"
		}
		t.Run(name, func(t *testing.T) {
			s, model, _, source := fixture(t)
			if _, err := s.DB.Exec(`PRAGMA ignore_check_constraints=ON; UPDATE notification_settings SET timeout_seconds=1; PRAGMA ignore_check_constraints=OFF`); err != nil {
				t.Fatal(err)
			}
			seed(t, s, source, event())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tools := &waitingNotificationTools{}
			if cancelParent {
				tools.cancelParent = cancel
			}
			s.Tools = tools
			calls := 0
			model.respond = func(prompt string) (any, error) {
				calls++
				if calls == 1 {
					return AgentTurn{Action: "read", Reads: []AgentRead{{Name: "list_tasks", Arguments: `{}`}}}, nil
				}
				return finalResponse(inputSnapshot(t, prompt), false), nil
			}
			if _, err := s.RequestProcessing(ctx); err != nil {
				t.Fatal(err)
			}
			if err := s.processNext(ctx); err != nil {
				t.Fatal(err)
			}
			status, err := s.Status(context.Background())
			if err != nil || status.Processing == nil {
				t.Fatal(status, err)
			}
			p := status.Processing
			if p.Status != "failed" || calls != 1 {
				t.Fatal("timed-out or canceled read allowed another model decision", p, calls)
			}
			var pending int
			if err := s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='pending'`).Scan(&pending); err != nil || pending != 1 {
				t.Fatal("failed read consumed notification", pending, err)
			}
			var raw string
			if err := s.DB.QueryRow(`SELECT processor_trace FROM notification_batches WHERE id=?`, p.ID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var trace []AgentReadResult
			if err := json.Unmarshal([]byte(raw), &trace); err != nil || len(trace) != 1 || !trace[0].Unavailable {
				t.Fatal("failed read missing from diagnostics", trace, err)
			}
		})
	}
}

func TestDecisionContextRetainsParentCancellationAndSingleCallLimits(t *testing.T) {
	s, _, _, _ := fixture(t)
	parent, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	run, stop, _, err := s.decisionContext(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	deadline, _ := parent.Deadline()
	if got, _ := run.Deadline(); !got.Equal(deadline) {
		t.Fatal("decision workflow escaped parent deadline", got, deadline)
	}
	cancel()
	if run.Err() != context.Canceled {
		t.Fatal("decision workflow ignored cancellation", run.Err())
	}
	one, stopOne, cfg, err := s.modelContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stopOne()
	deadline, _ = one.Deadline()
	if time.Until(deadline) > time.Duration(cfg.TimeoutSeconds)*time.Second {
		t.Fatal("single-call operations inherited the larger workflow deadline")
	}
}
