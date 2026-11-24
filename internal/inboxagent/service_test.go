package inboxagent

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/monitor"
	"kamacu/internal/store"
)

type modelFunc func(context.Context, monitor.Profile, string) (Decision, error)

func (f modelFunc) GenerateValidatedJSON(ctx context.Context, p monitor.Profile, prompt string, _ any, validate func([]byte) error) ([]byte, error) {
	d, err := f(ctx, p, prompt)
	if err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(d)
	if err = validate(raw); err != nil {
		return raw, err
	}
	return raw, nil
}
func setup(t *testing.T) (*Service, inbox.Identity) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`INSERT INTO projects(id,name,repo_path,github_repo) VALUES(1,'Release','/tmp/release','acme/release')`, `INSERT INTO tasks(id,project_id,title,position) VALUES(1,1,'Release request',0)`, `INSERT INTO monitor_profiles(id,name,backend,model) VALUES(1,'Inbox','codex_cli','inbox-model'),(2,'Monitor','codex_cli','monitor-model')`, `UPDATE monitor_settings SET default_profile_id=2`, `UPDATE notification_settings SET profile_id=2`} {
		if _, err = db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	box := inbox.New(db, func(string) inbox.Session { return inbox.Session{TaskID: 1, Live: true, Revision: 1} })
	t.Cleanup(box.Close)
	s := &Service{DB: db, Inbox: box}
	s.Respond = func(ctx context.Context, req inbox.Response, guard inbox.AnswerGuard) (inbox.Result, error) {
		return box.AnswerAutomatic(ctx, req, guard)
	}
	c, err := s.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	id := int64(1)
	c.ProfileID = &id
	c.Rules = []Rule{{ID: "release", Text: "When the release task asks to proceed with the verified release, reply Yes, proceed.", Enabled: true}}
	if _, err = s.SaveConfig(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	box.SetBeforePublish(s.Process)
	return s, inbox.Identity{TaskID: 1, SessionID: "origin"}
}
func observed(t *testing.T, s *Service, who inbox.Identity, key string) inbox.Question {
	t.Helper()
	out, err := s.Inbox.Observe(context.Background(), who, inbox.Create{Key: key, Abstract: "Release verified", Summary: "Release is verified", Context: "The release is verified for acme/release.", Question: "Proceed with the verified release?", Options: []inbox.Option{{ID: "yes", Label: "Yes, proceed."}}}, "question", "text", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	return out.Question
}
func reply() Decision {
	return Decision{ClearMatch: true, Outcome: "reply", RuleIDs: []string{"release"}, Reason: "The verified release matches the rule.", Evidence: []string{"release is verified"}, OptionID: "yes"}
}
func skip() Decision {
	return Decision{Outcome: "skip", Reason: "The rule does not clearly authorize this case.", RuleIDs: []string{}, Evidence: []string{}}
}
func assertPending(t *testing.T, s *Service, n int) {
	t.Helper()
	page, err := s.Inbox.List(context.Background(), nil, false, 0)
	if err != nil || len(page.Items) != n || page.PendingCount != n {
		t.Fatalf("pending: %s %v; want %d", jsonText(page), err, n)
	}
	queue, err := s.Inbox.CatchUp(context.Background())
	if err != nil || len(queue) != n {
		t.Fatalf("catch up: %+v %v", queue, err)
	}
}
func TestPublicationArchivesClearMatchWithImmutableRuleAndResponse(t *testing.T) {
	s, who := setup(t)
	calls := 0
	s.Model = modelFunc(func(_ context.Context, p monitor.Profile, prompt string) (Decision, error) {
		calls++
		if p.ID != 1 || !strings.Contains(prompt, "acme/release") {
			t.Fatal("wrong profile or missing input")
		}
		assertPending(t, s, 0)
		return reply(), nil
	})
	q := observed(t, s, who, "release")
	assertPending(t, s, 0)
	if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	assertPending(t, s, 0)
	saved, err := s.Inbox.Get(context.Background(), q.ID)
	if err != nil || saved.Status != "answered" || saved.Answer != "Yes, proceed." || saved.AnswerSource != "inbox_agent" || saved.Automation == nil || len(saved.Automation.Rules) != 1 || saved.Automation.Rules[0].ID != "release" {
		t.Fatalf("saved %+v %v", saved, err)
	}
	if err = s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil || calls != 1 {
		t.Fatalf("duplicate assessment %d %v", calls, err)
	}
	c, _ := s.Config(context.Background())
	c.Rules = nil
	if _, err = s.SaveConfig(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	saved, _ = s.Inbox.Get(context.Background(), q.ID)
	if !strings.Contains(saved.Automation.Rules[0].Text, "verified release") {
		t.Fatal("historical rule was not preserved")
	}
	archived, err := s.Inbox.List(context.Background(), nil, true, 0)
	if err != nil || len(archived.Items) != 1 {
		t.Fatal(archived, err)
	}
}
func TestSkipDisabledAndFailuresPublishPending(t *testing.T) {
	for _, name := range []string{"ambiguous", "model_failure", "unknown_rule", "disabled_rule", "unknown_option", "not_clear", "invalid_evidence", "rules_changed", "source_changed", "disabled", "oversize"} {
		t.Run(name, func(t *testing.T) {
			s, who := setup(t)
			calls := 0
			s.Model = modelFunc(func(context.Context, monitor.Profile, string) (Decision, error) {
				calls++
				d := reply()
				switch name {
				case "ambiguous":
					return skip(), nil
				case "model_failure":
					return d, errors.New("model unavailable")
				case "unknown_rule":
					d.RuleIDs = []string{"unknown"}
				case "disabled_rule":
					d.RuleIDs = []string{"disabled"}
				case "unknown_option":
					d.OptionID = "invented"
				case "not_clear":
					d.ClearMatch = false
				case "invalid_evidence":
					d.Evidence = []string{"not in the input"}
				case "rules_changed":
					c, _ := s.Config(context.Background())
					c.Rules = nil
					if _, err := s.SaveConfig(context.Background(), c); err != nil {
						t.Fatal(err)
					}
				}
				return d, nil
			})
			c, _ := s.Config(context.Background())
			c.Rules = append(c.Rules, Rule{ID: "disabled", Text: "Disabled instruction", Enabled: false})
			if name == "disabled" {
				c.ProfileID = nil
			}
			if _, err := s.SaveConfig(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if name == "source_changed" {
				reads := 0
				s.Evidence = func(context.Context, inbox.Question) (json.RawMessage, error) {
					reads++
					if reads == 1 {
						return json.RawMessage(`{"head":"old"}`), nil
					}
					return json.RawMessage(`{"head":"new"}`), nil
				}
			}
			q := observed(t, s, who, "release")
			if name == "oversize" {
				if _, err := s.DB.Exec(`UPDATE inbox_questions SET context=? WHERE id=?`, strings.Repeat("x", 1600000), q.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil {
				t.Fatal(err)
			}
			assertPending(t, s, 1)
			saved, _ := s.Inbox.Get(context.Background(), q.ID)
			if saved.Answer != "" {
				t.Fatal("unexpected answer", saved)
			}
			if name == "ambiguous" || name == "disabled" {
				if saved.Automation != nil {
					t.Fatal("ordinary pending message got failure", saved.Automation)
				}
			} else if saved.Automation == nil || saved.Automation.Error == "" {
				t.Fatal("failure is not visible", saved)
			}
			if (name == "disabled" || name == "oversize") && calls != 0 {
				t.Fatal("unexpected inference")
			}
		})
	}
}
func TestUncertainDeliveryStaysPendingAndCannotResend(t *testing.T) {
	s, who := setup(t)
	calls := 0
	s.Model = modelFunc(func(context.Context, monitor.Profile, string) (Decision, error) { calls++; return reply(), nil })
	s.Respond = func(ctx context.Context, req inbox.Response, guard inbox.AnswerGuard) (inbox.Result, error) {
		out, err := s.Inbox.AnswerAutomatic(ctx, req, guard)
		if err != nil {
			return out, err
		}
		if err = s.Inbox.RecordDelivery(ctx, req.ID, "uncertain"); err != nil {
			return out, err
		}
		return out, errors.New("terminal delivery uncertain")
	}
	q := observed(t, s, who, "release")
	if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	assertPending(t, s, 1)
	saved, _ := s.Inbox.Get(context.Background(), q.ID)
	if saved.Answer != "Yes, proceed." || saved.Automation == nil || saved.Automation.Status != "failed" {
		t.Fatal(saved)
	}
	if _, err := s.Inbox.Answer(context.Background(), nil, q.ID, saved.Revision, "", "yes"); !errors.Is(err, inbox.ErrConflict) {
		t.Fatal("uncertain reply could be repeated", err)
	}
	if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil || calls != 1 {
		t.Fatal("automatically resent", calls, err)
	}
}
func TestInterruptedPublicationIsPublishedWithoutInference(t *testing.T) {
	s, who := setup(t)
	q := observed(t, s, who, "release")
	assertPending(t, s, 0)
	if err := s.Inbox.RecoverPublications(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertPending(t, s, 1)
	saved, _ := s.Inbox.Get(context.Background(), q.ID)
	if saved.Automation == nil || !strings.Contains(saved.Automation.Error, "interrupted") {
		t.Fatal(saved)
	}
}
func TestPermissionDialogsAreNeverAutomaticallyAnswered(t *testing.T) {
	s, who := setup(t)
	s.Model = modelFunc(func(context.Context, monitor.Profile, string) (Decision, error) {
		t.Fatal("permission dialog assessed")
		return reply(), nil
	})
	out, err := s.Inbox.Observe(context.Background(), who, inbox.Create{Key: "permission", Abstract: "Approve access", Summary: "Permission requested", Context: "The terminal needs permission", Question: "Allow?"}, "question", "terminal", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Inbox.FinalizePublication(context.Background(), out.Question.ID); err != nil {
		t.Fatal(err)
	}
	assertPending(t, s, 1)
}
func TestConfigurationUsesOptimisticRevisionAndSeparateModel(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	c, _ := s.Config(ctx)
	old := c
	c.Rules[0].Text = "Updated rule"
	if _, err := s.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveConfig(ctx, old); !errors.Is(err, ErrChanged) {
		t.Fatal("lost update", err)
	}
	for _, query := range []string{`SELECT default_profile_id FROM monitor_settings`, `SELECT profile_id FROM notification_settings`} {
		var id int64
		if err := s.DB.QueryRowContext(ctx, query).Scan(&id); err != nil || id != 2 {
			t.Fatal("another agent selection changed", id, err)
		}
	}
}
func TestAutomaticAnswerGuardRollsBack(t *testing.T) {
	s, who := setup(t)
	q := observed(t, s, who, "release")
	_, err := s.Inbox.AnswerAutomatic(context.Background(), inbox.Response{ID: q.ID, Revision: q.Revision, OptionID: "yes"}, func(*sql.Tx) error { return ErrChanged })
	if !errors.Is(err, ErrChanged) {
		t.Fatal(err)
	}
	saved, _ := s.Inbox.Get(context.Background(), q.ID)
	if saved.Status != "pending" || saved.Answer != "" || saved.Automation != nil {
		t.Fatal(saved)
	}
}

func TestConcurrentFinalizationDoesNotPublishOrReplyTwice(t *testing.T) {
	s, who := setup(t)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	s.Model = modelFunc(func(context.Context, monitor.Profile, string) (Decision, error) {
		close(entered)
		<-release
		return reply(), nil
	})
	q := observed(t, s, who, "release")
	go func() { done <- s.Inbox.FinalizePublication(context.Background(), q.ID) }()
	<-entered
	if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	assertPending(t, s, 0)
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	saved, _ := s.Inbox.Get(context.Background(), q.ID)
	if saved.Status != "answered" {
		t.Fatal(saved)
	}
	var attempts int
	if err := s.DB.QueryRow(`SELECT count(*) FROM inbox_agent_decisions WHERE question_id=?`, q.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal(attempts, err)
	}
}
func TestDeletingTaskCascadesAutomaticDecisionHistory(t *testing.T) {
	s, who := setup(t)
	s.Model = modelFunc(func(context.Context, monitor.Profile, string) (Decision, error) { return reply(), nil })
	q := observed(t, s, who, "release")
	if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`DELETE FROM tasks WHERE id=1`); err != nil {
		t.Fatal("automatic history blocked task deletion", err)
	}
	var n int
	if err := s.DB.QueryRow(`SELECT count(*) FROM inbox_agent_decisions`).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

func TestObsoleteAssessmentCannotReopenOrOverwriteMessage(t *testing.T) {
	for _, change := range []string{"supersede", "dismiss", "new_revision"} {
		t.Run(change, func(t *testing.T) {
			s, who := setup(t)
			ctx := context.Background()
			q := observed(t, s, who, "release")
			calls := 0
			s.Model = modelFunc(func(context.Context, monitor.Profile, string) (Decision, error) {
				calls++
				if calls == 1 {
					switch change {
					case "supersede":
						if err := s.Inbox.SupersedeObserved(ctx, who.SessionID); err != nil {
							t.Fatal(err)
						}
					case "dismiss":
						if _, err := s.Inbox.Dismiss(ctx, q.ID, q.Revision); err != nil {
							t.Fatal(err)
						}
					case "new_revision":
						if _, err := s.DB.Exec(`UPDATE inbox_questions SET revision=revision+1,question='Proceed with the revised release?' WHERE id=?`, q.ID); err != nil {
							t.Fatal(err)
						}
					}
				}
				return reply(), nil
			})
			if err := s.Inbox.FinalizePublication(ctx, q.ID); err != nil {
				t.Fatal(err)
			}
			saved, err := s.Inbox.Get(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if saved.Automation != nil || saved.Answer != "" {
				t.Fatalf("obsolete assessment changed message: %+v", saved)
			}
			if change == "new_revision" {
				assertPending(t, s, 0)
				if err = s.Inbox.FinalizePublication(ctx, q.ID); err != nil {
					t.Fatal(err)
				}
				saved, _ = s.Inbox.Get(ctx, q.ID)
				if calls != 2 || saved.Status != "answered" {
					t.Fatalf("new revision was not assessed: %+v calls=%d", saved, calls)
				}
			} else {
				expected := "unanswered"
				if change == "dismiss" {
					expected = "dismissed"
				}
				if saved.Status != expected {
					t.Fatalf("message resurrected: %+v", saved)
				}
			}
		})
	}
}
func TestRecoveryPreservesConfirmedRepliesAndGuardsUncertainDelivery(t *testing.T) {
	for _, mode := range []string{"submitted", "queued", "exit", "attempting_pending", "uncertain_published"} {
		t.Run(mode, func(t *testing.T) {
			s, who := setup(t)
			ctx := context.Background()
			q := observed(t, s, who, "release")
			origin, status, delivery, reason, publishing := "observer", "answered", mode, "", 1
			if mode == "queued" {
				origin = "notification"
			}
			if mode == "exit" {
				reason = "exit"
				delivery = ""
			}
			if mode == "attempting_pending" {
				status = "pending"
				delivery = "attempting"
				publishing = 0
			}
			if mode == "uncertain_published" {
				delivery = "uncertain"
				publishing = 0
			}
			a := inbox.Automation{Status: "answered", Answer: "Yes, proceed.", Rules: []inbox.AutomationRule{{ID: "release", Text: "Release rule"}}}
			if _, err := s.DB.Exec(`UPDATE inbox_questions SET origin=?,status=?,answer='Yes, proceed.',answer_source='inbox_agent',delivery_status=?,reason=?,publishing=?,automation=? WHERE id=?`, origin, status, delivery, reason, publishing, jsonText(a), q.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.Inbox.RecoverPublications(ctx); err != nil {
				t.Fatal(err)
			}
			saved, err := s.Inbox.Get(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(mode, "pending") || strings.Contains(mode, "published") {
				if saved.Status != "pending" || saved.DeliveryStatus != "uncertain" || saved.Automation.Status != "failed" {
					t.Fatalf("uncertain reply lost: %+v", saved)
				}
				if _, err = s.Inbox.Answer(ctx, nil, q.ID, saved.Revision, "", "yes"); !errors.Is(err, inbox.ErrConflict) {
					t.Fatal("uncertain reply can be repeated", err)
				}
			} else if saved.Status != "answered" || saved.Automation.Status != "answered" || saved.Automation.Error != "" {
				t.Fatalf("confirmed reply became a failure: %+v", saved)
			}
		})
	}
}

func TestPublishedPendingUpdatesDoNotReplayTheInboxAgent(t *testing.T) {
	s, who := setup(t)
	ctx := context.Background()
	calls := 0
	s.Model = modelFunc(func(context.Context, monitor.Profile, string) (Decision, error) { calls++; return skip(), nil })
	q := observed(t, s, who, "release")
	if err := s.Inbox.FinalizePublication(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	assertPending(t, s, 1)
	out, err := s.Inbox.Observe(ctx, who, inbox.Create{Key: "release", Abstract: "Release verified", Summary: "Updated release context", Context: "The release is verified for acme/release.", Question: "Proceed with the verified release?", Options: []inbox.Option{{ID: "yes", Label: "Yes, proceed."}}}, "question", "text", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	assertPending(t, s, 1)
	if err = s.Inbox.FinalizePublication(ctx, out.Question.ID); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("updated pending message was automatically reprocessed", calls)
	}
}

type quoteRepairModel struct {
	response func(string, any) any
}

func (m quoteRepairModel) GenerateValidatedJSON(_ context.Context, _ monitor.Profile, prompt string, schema any, check func([]byte) error) ([]byte, error) {
	raw, err := json.Marshal(m.response(prompt, schema))
	if err != nil {
		return nil, err
	}
	return raw, check(raw)
}
func TestAnnotatedEvidenceGetsOneCorrectionWithoutChangingTheAction(t *testing.T) {
	for _, mode := range []string{"correct_quotes", "invented_quote", "change_action"} {
		t.Run(mode, func(t *testing.T) {
			s, who := setup(t)
			calls := 0
			s.Model = quoteRepairModel{response: func(prompt string, schema any) any {
				calls++
				if calls == 1 {
					d := reply()
					d.Evidence = []string{"release is verified", "rule: When the release task asks to proceed with the verified release, reply Yes, proceed.", `options: {"id":"yes","label":"Yes, proceed."}`}
					return d
				}
				if calls > 2 {
					t.Fatal("more than one quote correction")
				}
				if !strings.Contains(prompt, "SAME rule and response") {
					t.Fatal("correction can replan the action")
				}
				shape := jsonText(schema)
				if strings.Contains(shape, "option_id") || strings.Contains(shape, "rule_ids") {
					t.Fatal("correction schema permits changing authorization", shape)
				}
				switch mode {
				case "invented_quote":
					return map[string]any{"evidence": []string{"Approval was definitely published on GitHub"}}
				case "change_action":
					return map[string]any{"evidence": []string{"release is verified"}, "option_id": "different"}
				default:
					return map[string]any{"evidence": []string{"release is verified"}}
				}
			}}
			q := observed(t, s, who, "release")
			if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil {
				t.Fatal(err)
			}
			saved, err := s.Inbox.Get(context.Background(), q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatal("unexpected calls", calls)
			}
			if mode == "correct_quotes" {
				if saved.Status != "answered" || saved.Answer != "Yes, proceed." || saved.Automation == nil || saved.Automation.Rules[0].ID != "release" {
					t.Fatal(saved)
				}
			} else {
				if saved.Status != "pending" || saved.Answer != "" || saved.Automation == nil || saved.Automation.Error == "" {
					t.Fatal("invalid correction authorized a reply", saved)
				}
				if saved.Automation.Answer != "Yes, proceed." || len(saved.Automation.Rules) != 1 {
					t.Fatal("attempted response attribution was lost", saved.Automation)
				}
				var decision string
				if err := s.DB.QueryRow(`SELECT decision FROM inbox_agent_decisions WHERE question_id=?`, q.ID).Scan(&decision); err != nil || !strings.Contains(decision, `"option_id":"yes"`) {
					t.Fatal("rejected proposal was lost", decision, err)
				}
			}
		})
	}
}

func TestQuoteCorrectionStopsBeforeUnnecessaryModelCalls(t *testing.T) {
	for _, mode := range []string{"cancelled", "budget", "unknown_option", "invalid_text"} {
		t.Run(mode, func(t *testing.T) {
			s, who := setup(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			if mode == "budget" {
				c, err := s.Config(ctx)
				if err != nil {
					t.Fatal(err)
				}
				c.ContextBytes = 64000
				if _, err = s.SaveConfig(ctx, c); err != nil {
					t.Fatal(err)
				}
			}
			s.Model = quoteRepairModel{response: func(prompt string, _ any) any {
				calls++
				if calls != 1 {
					t.Fatal("unnecessary quote correction", mode)
				}
				d := reply()
				d.Evidence = []string{"rule: this is an annotation"}
				switch mode {
				case "cancelled":
					cancel()
				case "budget":
					if len(prompt) > 64000 {
						t.Fatal("initial prompt exceeded budget")
					}
					d.Evidence = []string{strings.Repeat("z", 8000)}
				case "unknown_option":
					d.OptionID = "invented"
				case "invalid_text":
					d.OptionID = ""
					d.Answer = strings.Repeat("x", 16001)
				}
				return d
			}}
			q := observed(t, s, who, "release")
			if mode == "budget" {
				if _, err := s.DB.Exec(`UPDATE inbox_questions SET context=? WHERE id=?`, strings.Repeat("x", 57000)+"release is verified", q.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Inbox.FinalizePublication(ctx, q.ID); err != nil {
				t.Fatal(err)
			}
			saved, err := s.Inbox.Get(context.Background(), q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || saved.Status != "pending" || saved.Answer != "" || saved.Automation == nil {
				t.Fatalf("unexpected result: %+v calls=%d", saved, calls)
			}
			if mode == "budget" || mode == "cancelled" {
				if saved.Automation.Answer != "Yes, proceed." || len(saved.Automation.Rules) != 1 {
					t.Fatal("attempted response attribution was lost", saved.Automation)
				}
				var raw string
				if err := s.DB.QueryRow(`SELECT decision FROM inbox_agent_decisions WHERE question_id=?`, q.ID).Scan(&raw); err != nil || !strings.Contains(raw, `"option_id":"yes"`) {
					t.Fatal("original rejected proposal was lost", raw, err)
				}
			}
		})
	}
}
func TestQuoteCorrectionStillChecksChangedRulesAndSource(t *testing.T) {
	for _, mode := range []string{"rules", "source"} {
		t.Run(mode, func(t *testing.T) {
			s, who := setup(t)
			calls := 0
			source := `{"head":"old"}`
			s.Evidence = func(context.Context, inbox.Question) (json.RawMessage, error) { return json.RawMessage(source), nil }
			s.Model = quoteRepairModel{response: func(string, any) any {
				calls++
				if calls == 1 {
					d := reply()
					d.Evidence = []string{"option yes: Yes, proceed."}
					return d
				}
				if mode == "source" {
					source = `{"head":"new"}`
				} else {
					c, err := s.Config(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					c.Rules = nil
					if _, err = s.SaveConfig(context.Background(), c); err != nil {
						t.Fatal(err)
					}
				}
				return map[string]any{"evidence": []string{"release is verified"}}
			}}
			q := observed(t, s, who, "release")
			if err := s.Inbox.FinalizePublication(context.Background(), q.ID); err != nil {
				t.Fatal(err)
			}
			saved, err := s.Inbox.Get(context.Background(), q.ID)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || saved.Status != "pending" || saved.Answer != "" || saved.Automation == nil || saved.Automation.Error == "" {
				t.Fatal("correction bypassed freshness checks", saved, calls)
			}
		})
	}
}

func TestApprovedPRCompletionClosesWithCorrectedEvidence(t *testing.T) {
	s, who := setup(t)
	ctx := context.Background()
	c, err := s.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c.Rules = []Rule{{ID: "after-approval", Text: "after I approved a PR we can close the session", Enabled: true}}
	if _, err = s.SaveConfig(ctx, c); err != nil {
		t.Fatal(err)
	}
	quote := "I approved PR #2074 with an empty body and no inline comments, and GitHub now shows it as APPROVED."
	calls := 0
	s.Model = quoteRepairModel{response: func(string, any) any {
		calls++
		if calls == 1 {
			return Decision{ClearMatch: true, Outcome: "reply", RuleIDs: []string{"after-approval"}, Reason: "The approval was published, so the close-session rule applies.", OptionID: "exit", Evidence: []string{quote, `options: {"id":"exit","label":"Close session"}`, "rule: after I approved a PR we can close the session"}}
		}
		return map[string]any{"evidence": []string{quote}}
	}}
	out, err := s.Inbox.Observe(ctx, who, inbox.Create{Key: "approved-pr", Abstract: "PR approved", Summary: "Approved PR #2074 with empty body; token access still unverified", Context: quote + "\n\nOne thing is still open on the author's side: the user behind SCHED_GHA_PLATFORM_TOKEN needs access to the gha-runner workspace before the next release.", Question: "Work completed"}, "completion", "terminal", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Inbox.FinalizePublication(ctx, out.Question.ID); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Inbox.Get(ctx, out.Question.ID)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || saved.Status != "answered" || saved.Answer != "Close session" || saved.Reason != "exit" || saved.AnswerSource != "inbox_agent" || saved.Automation == nil || saved.Automation.Rules[0].Text != c.Rules[0].Text {
		t.Fatalf("completion was not automatically answered: %+v calls=%d", saved, calls)
	}
}
