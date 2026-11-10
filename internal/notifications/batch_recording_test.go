package notifications

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

type batchRecordingDelay struct {
	*fakeProvider
	db          *sql.DB
	released    chan struct{}
	pauseSource string
	pauseErr    error
}

func (p *batchRecordingDelay) Enrich(ctx context.Context, s Source, e Event) (Event, error) {
	out, err := p.fakeProvider.Enrich(ctx, s, e)
	out.SubjectUpdatedAt = "unchanged"
	return out, err
}
func (p *batchRecordingDelay) SubjectVersion(ctx context.Context, _ Source, _ Event) (string, error) {
	held, err := p.db.Conn(ctx)
	if err != nil {
		return "", err
	}
	go func() {
		defer close(p.released)
		defer held.Close()
		time.Sleep(5300 * time.Millisecond)
		if p.pauseSource != "" {
			_, p.pauseErr = held.ExecContext(context.Background(), `UPDATE notification_sources SET enabled=0 WHERE id=?`, p.pauseSource)
		}
	}()
	return "unchanged", nil
}

func TestBatchRecordingRetriesCompletedModelResponseWithoutRegeneration(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	provider := &batchRecordingDelay{fakeProvider: &fakeProvider{}, db: s.DB, released: make(chan struct{})}
	s.Providers["github"] = provider
	seed(t, s, source, event())
	model.respond = func(prompt string) (any, error) { return finalResponse(inputSnapshot(t, prompt), true), nil }
	batch := processAgent(t, s)
	<-provider.released
	status, _ := s.Status(ctx)
	cards, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if batch.Status != "completed" || status.PendingCount != 0 || len(model.prompts) != 1 || len(cards.Items) != 1 || len(tools.calls) != 0 || err != nil {
		t.Fatal(batch, status, len(model.prompts), cards, tools.calls, err)
	}
	var result string
	if err = s.DB.QueryRow(`SELECT processor_result FROM notification_batches WHERE id=?`, batch.ID).Scan(&result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `"action":"finish"`) {
		t.Fatal("completed result not retained", result)
	}
}

func TestBatchRecordingFailureIdentifiesStoragePhaseAndKeepsNotifications(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_batch_result BEFORE UPDATE OF processor_result ON notification_batches BEGIN SELECT RAISE(ABORT,'result storage unavailable');END`); err != nil {
		t.Fatal(err)
	}
	model.respond = func(prompt string) (any, error) { return finalResponse(inputSnapshot(t, prompt), true), nil }
	batch := processAgent(t, s)
	status, _ := s.Status(ctx)
	cards, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if batch.Status != "failed" || !strings.Contains(batch.Error, "could not save notification model results") || status.PendingCount != 1 || len(model.prompts) != 1 || len(cards.Items) != 0 || len(tools.calls) != 0 || err != nil {
		t.Fatal(batch, status, len(model.prompts), cards, tools.calls, err)
	}
}

func TestBatchRecordingFailureRetainsTheOriginalModelError(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_batch_diagnostics BEFORE UPDATE OF processor_result ON notification_batches BEGIN SELECT RAISE(ABORT,'diagnostic storage unavailable');END`); err != nil {
		t.Fatal(err)
	}
	model.respond = func(string) (any, error) { return nil, errors.New("model service unavailable") }
	batch := processAgent(t, s)
	if batch.Status != "failed" || !strings.Contains(batch.Error, "model service unavailable") || !strings.Contains(batch.Error, "could not save notification failure diagnostics") || strings.Contains(batch.Error, "could not save notification model results") {
		t.Fatal(batch)
	}
	if len(model.prompts) != 1 {
		t.Fatal(model.prompts)
	}
}

func TestSourcePausedDuringRecordingRetryCannotPublishDecisions(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	provider := &batchRecordingDelay{fakeProvider: &fakeProvider{}, db: s.DB, released: make(chan struct{}), pauseSource: source.ID}
	s.Providers["github"] = provider
	seed(t, s, source, event())
	model.respond = func(prompt string) (any, error) { return finalResponse(inputSnapshot(t, prompt), true), nil }
	batch := processAgent(t, s)
	<-provider.released
	if provider.pauseErr != nil {
		t.Fatal(provider.pauseErr)
	}
	status, _ := s.Status(ctx)
	cards, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if batch.Status != "failed" || !strings.Contains(batch.Error, "source is paused") || status.PendingCount != 1 || len(cards.Items) != 0 || len(model.prompts) != 1 || len(tools.calls) != 0 || err != nil {
		t.Fatal(batch, status, cards, len(model.prompts), tools.calls, err)
	}
}

func TestSnapshotStorageFailureDoesNotInvokeTheModel(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_batch_input BEFORE UPDATE OF processor_snapshot ON notification_batches BEGIN SELECT RAISE(ABORT,'snapshot storage unavailable');END`); err != nil {
		t.Fatal(err)
	}
	batch := processAgent(t, s)
	if batch.Status != "failed" || !strings.Contains(batch.Error, "before model processing") || len(model.prompts) != 0 {
		t.Fatal(batch, len(model.prompts))
	}
}
