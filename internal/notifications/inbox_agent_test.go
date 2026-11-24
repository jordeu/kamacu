package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/inboxagent"
	"kamacu/internal/monitor"
)

type inboxModelFunc func(string) (inboxagent.Decision, error)

func (f inboxModelFunc) GenerateValidatedJSON(_ context.Context, _ monitor.Profile, prompt string, _ any, validate func([]byte) error) ([]byte, error) {
	d, err := f(prompt)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(d)
	if err = validate(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
func TestNotificationPublicationPassesThroughInboxAgent(t *testing.T) {
	for _, mode := range []string{"match", "ambiguous", "failure"} {
		t.Run(mode, func(t *testing.T) {
			s, model, _, source := fixture(t)
			ctx := context.Background()
			seed(t, s, source, event())
			model.respond = func(prompt string) (any, error) { return finalResponse(inputSnapshot(t, prompt), true), nil }
			agent := &inboxagent.Service{DB: s.DB, Inbox: s.Inbox, Evidence: s.InboxEvidence, Respond: func(ctx context.Context, req inbox.Response, guard inbox.AnswerGuard) (inbox.Result, error) {
				return s.Inbox.AnswerAutomatic(ctx, req, guard)
			}}
			config, err := agent.Config(ctx)
			if err != nil {
				t.Fatal(err)
			}
			id := int64(1)
			config.ProfileID = &id
			config.Rules = []inboxagent.Rule{{ID: "investigate", Text: "Start an investigation when a concrete issue needs investigation in acme/repo.", Enabled: true}}
			if _, err = agent.SaveConfig(ctx, config); err != nil {
				t.Fatal(err)
			}
			calls := 0
			agent.Model = inboxModelFunc(func(prompt string) (inboxagent.Decision, error) {
				calls++
				pending, err := s.Inbox.List(ctx, nil, false, 0)
				if err != nil || pending.PendingCount != 0 || len(pending.Items) != 0 {
					t.Fatalf("message visible before assessment: %+v %v", pending, err)
				}
				if !strings.Contains(prompt, "acme/repo") || !strings.Contains(prompt, "Create investigation task") {
					t.Fatal("missing source or saved action")
				}
				if mode == "failure" {
					return inboxagent.Decision{}, errors.New("inbox model offline")
				}
				if mode == "ambiguous" {
					return inboxagent.Decision{Outcome: "skip", Reason: "The rule is not a clear match.", RuleIDs: []string{}, Evidence: []string{}}, nil
				}
				return inboxagent.Decision{ClearMatch: true, Outcome: "reply", RuleIDs: []string{"investigate"}, Reason: "This is an investigation request in the configured repository.", Evidence: []string{"A concrete issue needs investigation"}, OptionID: "investigate"}, nil
			})
			s.Inbox.SetBeforePublish(agent.Process)
			processing := processAgent(t, s)
			if processing.Status != "completed" {
				t.Fatal(processing)
			}
			if calls != 1 {
				t.Fatalf("%d calls", calls)
			}
			pending, err := s.Inbox.List(ctx, nil, false, 0)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "match" {
				if pending.PendingCount != 0 {
					t.Fatal(pending)
				}
				archive, err := s.Inbox.List(ctx, nil, true, 0)
				if err != nil || len(archive.Items) != 1 {
					t.Fatal(archive, err)
				}
				q := archive.Items[0]
				if q.AnswerSource != "inbox_agent" || q.Automation == nil || q.Automation.Rules[0].ID != "investigate" {
					t.Fatal(q)
				}
				var state, action string
				if err = s.DB.QueryRow(`SELECT state,action FROM notification_runs WHERE id=?`, q.ID).Scan(&state, &action); err != nil || state != "queued" || !strings.Contains(action, "create_investigation") {
					t.Fatal(state, action, err)
				}
			} else {
				if pending.PendingCount != 1 {
					t.Fatal(pending)
				}
				if mode == "failure" && pending.Items[0].Automation == nil {
					t.Fatal("missing visible error")
				}
			}
		})
	}
}

func TestInboxEvidenceIsFreshStableAndIndependentOfActivityCursors(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	e := event()
	e.SubjectURL = "https://api.github.com/repos/acme/repo/pulls/7"
	e.Version = "2026-11-03T12:00:00Z"
	id := seed(t, s, source, e)
	q := publish(t, s, id)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	body := "Release request"
	requests := 0
	s.Providers["github"] = GitHub{DB: s.DB, Now: func() time.Time { return now }, Run: func(_ context.Context, args ...string) ([]byte, error) {
		path := args[len(args)-1]
		if strings.Contains(path, "since=") {
			t.Fatal("inbox evidence depends on a moving retrieval cursor", path)
		}
		switch path {
		case "user":
			return response(`{"login":"alice"}`, ""), nil
		case "repos/acme/repo/pulls/7":
			requests++
			return response(fmt.Sprintf(`{"id":7,"html_url":"https://github.com/acme/repo/pull/7","state":"open","draft":false,"title":"Release","body":%q,"user":{"login":"release-bot"},"head":{"sha":"head"},"updated_at":"2026-11-03T12:00:00Z"}`, body), ""), nil
		default:
			return response(`[]`, ""), nil
		}
	}}
	// An inherited cache from earlier processing must not satisfy revalidation.
	ctx = freshGitHubReadCache(ctx)
	first, err := s.InboxEvidence(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	second, err := s.InboxEvidence(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("unchanged source produced different evidence:\n%s\n%s", first, second)
	}
	var subjects int
	if err = s.DB.QueryRow(`SELECT count(*) FROM notification_activity_subjects`).Scan(&subjects); err != nil || subjects != 0 {
		t.Fatal("inbox assessment changed notification activity cursors", subjects, err)
	}
	body = "Changed release request"
	third, err := s.InboxEvidence(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(second, third) || requests < 3 {
		t.Fatal("source change was hidden by read cache", requests)
	}
}
