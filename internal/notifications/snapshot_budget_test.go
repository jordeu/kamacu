package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"kamacu/internal/store"
)

func TestCompactBatchSuppliesUnevenNewActivityWithoutRequiredLookups(t *testing.T) {
	s, model, tools, _ := fixture(t)
	snapshot := AgentSnapshot{Tasks: []AgentTask{{ID: 1, ReadID: "T1", Description: strings.Repeat("Task details. ", 40000)}}}
	next := int64(1)
	for i := 0; i < 42; i++ {
		sub := AgentSubject{ID: "S" + formatID(int64(i+1)), EventIDs: []int64{int64(i + 1)}, ProjectID: 1, Event: event()}
		sub.Event.URL += "?subject=" + formatID(int64(i))
		sub.Event.Evidence = nil
		sub.Event.ActivityDelta = &store.NotificationDelta{Complete: true}
		count := 1
		if i == 0 {
			count = 147 // More mandatory evidence than the optional lookup limit.
		}
		for j := 0; j < count; j++ {
			ref := "A" + formatID(next) + "/V1"
			sub.Event.Evidence = append(sub.Event.Evidence, SubjectEvidence{ID: sub.ID + "/E" + formatID(int64(j+1)), ActivityRef: ref, Text: strings.Repeat("New feedback. ", 70), Complete: true})
			sub.Event.ActivityDelta.Activities = append(sub.Event.ActivityDelta.Activities, store.NotificationActivityRef{Ref: ref})
			next++
		}
		if i == 0 {
			sub.Event.Evidence = append(sub.Event.Evidence, SubjectEvidence{ID: sub.ID + "/old", ActivityRef: "A9999/V1", Text: strings.Repeat("Older optional discussion. ", 9000), ContextOnly: true, Complete: true})
		}
		snapshot.Subjects = append(snapshot.Subjects, sub)
	}
	model.respond = func(prompt string) (any, error) {
		if !strings.Contains(prompt, "\nREQUIRED ACTIVITY READS:\n[]") {
			t.Fatal("new evidence still depends on optional lookups")
		}
		if strings.Contains(prompt, "Older optional discussion.") {
			t.Fatal("optional discussion was retained instead of fitting new evidence")
		}
		turn := finalResponse(snapshot, false)
		for i, sub := range snapshot.Subjects {
			for _, ref := range sub.Event.ActivityDelta.Activities {
				turn.Decisions[i].ActivityRefs = append(turn.Decisions[i].ActivityRefs, ref.Ref)
			}
		}
		return turn, nil
	}
	_, trace, err := s.runNotificationAgent(context.Background(), snapshot, Config{ContextBytes: 800000, MaxReads: 1})
	if err != nil || len(trace) != 0 || len(model.prompts) != 1 || len(tools.calls) != 0 {
		t.Fatal("compact batch could not finish with all evidence supplied", err, len(trace), len(model.prompts), tools.calls)
	}
}

type budgetRefreshProvider struct {
	fakeProvider
	updated bool
}

func (p *budgetRefreshProvider) SubjectVersion(context.Context, Source, Event) (string, error) {
	if p.updated {
		return "new", nil
	}
	return "old", nil
}

func (p *budgetRefreshProvider) Enrich(ctx context.Context, source Source, e Event) (Event, error) {
	e, err := p.fakeProvider.Enrich(ctx, source, e)
	version, ref := "old", "A1/V1"
	if p.updated {
		version, ref = "new", "A1/V2"
	}
	e.Context = strings.Repeat(version+" ", 22000)
	e.SubjectUpdatedAt = version
	e.Evidence = []SubjectEvidence{{ID: "body", ActivityRef: ref, Text: e.Context, Complete: true}}
	e.ActivityDelta = &store.NotificationDelta{Complete: true, Activities: []store.NotificationActivityRef{{Ref: ref}}}
	return e, err
}

func TestCompactRefreshedBatchDoesNotDuplicateRequiredEvidence(t *testing.T) {
	s, model, _, source := fixture(t)
	provider := &budgetRefreshProvider{}
	s.Providers["github"] = provider
	id := seed(t, s, source, event())
	cfg, err := s.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.ContextBytes, cfg.MaxReads = 256000, 1
	if _, err := s.SaveConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	model.onGenerate = func(string) { provider.updated = true }
	model.respond = func(prompt string) (any, error) {
		_, raw, ok := strings.Cut(prompt, "\nBATCH:\n")
		if !ok {
			t.Fatal("missing batch")
		}
		raw, _, _ = strings.Cut(raw, "\nREAD-ONLY TOOLS:\n")
		var index struct {
			Subjects []struct {
				IDs      []int64                  `json:"event_ids"`
				Evidence []SubjectEvidence        `json:"activity_evidence"`
				Delta    *store.NotificationDelta `json:"activity_delta"`
			} `json:"subjects"`
		}
		if err := json.Unmarshal([]byte(raw), &index); err != nil {
			t.Fatal(err)
		}
		if len(index.Subjects) != 1 || len(index.Subjects[0].Evidence) != 1 {
			t.Fatal("required evidence missing from compact index")
		}
		sub := index.Subjects[0]
		if strings.Count(prompt, sub.Evidence[0].Text) != 1 {
			t.Fatal("required evidence duplicated outside compact index")
		}
		return AgentTurn{Action: "finish", Decisions: []AgentDecision{{EventIDs: sub.IDs, Outcome: "ignore", Reason: "No unresolved request", EvidenceRefs: []string{sub.Evidence[0].ID}, ActivityRefs: []string{sub.Delta.Activities[0].Ref}}}}, nil
	}
	rules, err := s.activeRuleSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := s.agentSnapshot(context.Background(), []RawNotification{{ID: id, SourceID: source.ID, Event: event()}}, rules)
	if err != nil {
		t.Fatal(err)
	}
	_, trace, err := s.runNotificationAgentWithUpdates(context.Background(), &snapshot, cfg, true)
	if err != nil || len(model.prompts) != 2 {
		t.Fatal("source refresh exhausted context despite fitting evidence", err, len(model.prompts))
	}
	if !strings.Contains(mustJSON(trace), strings.Repeat("new ", 22000)) {
		t.Fatal("audit trace lost full refreshed source")
	}
}

func TestOversizedRequiredEvidenceFailsBeforeModelAndKeepsPending(t *testing.T) {
	s, model, _, source := fixture(t)
	cfg, err := s.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	cfg.ContextBytes = 64000
	if _, err := s.SaveConfig(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	e := event()
	e.Evidence = []SubjectEvidence{{ID: "body", ActivityRef: "A1/V1", Text: strings.Repeat("Large new feedback. ", 10000), Complete: true}}
	e.ActivityDelta = &store.NotificationDelta{Complete: true, Activities: []store.NotificationActivityRef{{Ref: "A1/V1"}}}
	seed(t, s, source, e)
	p := processAgent(t, s)
	if p.Status != "failed" || !strings.Contains(p.Error, "context budget") || len(model.prompts) != 0 {
		t.Fatal("oversized evidence reached the model", p, len(model.prompts))
	}
	var pending int
	if err := s.DB.QueryRow(`SELECT count(*) FROM notification_events WHERE raw_status='pending'`).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("oversized batch consumed notifications", pending, err)
	}
}
