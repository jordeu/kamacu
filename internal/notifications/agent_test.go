package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
	"kamacu/internal/store"
	"os"
	"strings"
	"testing"
	"time"

	"kamacu/internal/monitor"
)

func inputSnapshot(t *testing.T, prompt string) AgentSnapshot {
	t.Helper()
	_, raw, ok := strings.Cut(prompt, "\nBATCH:\n")
	if !ok {
		t.Fatal("missing batch input")
	}
	raw, _, _ = strings.Cut(raw, "\nREAD-ONLY TOOLS:\n")
	var s AgentSnapshot
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		t.Fatal(err)
	}
	return s
}
func finalResponse(snapshot AgentSnapshot, feedback bool) AgentTurn {
	out := AgentTurn{Action: "finish", Reads: []AgentRead{}, Messages: []AgentMessage{}, Decisions: []AgentDecision{}, SupersedeIDs: []string{}}
	for i, s := range snapshot.Subjects {
		outcome := "ignore"
		if feedback && i == 0 {
			outcome = "message"
		}
		refs := []string{s.Event.Evidence[0].ID}
		out.Decisions = append(out.Decisions, AgentDecision{EventIDs: s.EventIDs, Outcome: outcome, Reason: "Current source and Inbox history establish this decision", RuleIDs: []string{}, EvidenceRefs: refs})
		if outcome == "message" {
			out.Messages = append(out.Messages, AgentMessage{EventIDs: s.EventIDs, Summary: "A concrete issue needs investigation", Context: "A source update requests an investigation: " + s.Event.URL, Question: "Create an investigation task for this issue?", Why: "This is an unresolved request without an existing owner", RuleIDs: []string{}, EvidenceRefs: refs, LinkedTaskIDs: []int64{}, Options: []AgentOption{{ID: "investigate", Label: "Create investigation task", Mode: "action", Actions: []inboxaction.Action{{ID: "investigate", Kind: "create_investigation", ProjectID: s.ProjectID, TaskName: "Investigate source update", Instructions: "Investigate this source and report findings: " + s.Event.URL}}}}, Plan: AgentPlan{Workflow: "route_investigation", ProjectID: s.ProjectID, PRNumbers: []int64{}, TaskName: "Investigate source update", WorkKind: "investigation"}})
		}
	}
	return out
}
func processAgent(t *testing.T, s *Service) Processing {
	t.Helper()
	ctx := context.Background()
	if _, err := s.RequestProcessing(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.processNext(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx)
	if err != nil || status.Processing == nil {
		t.Fatal(status, err)
	}
	return *status.Processing
}

type recordingModel struct {
	base     *fakeModel
	profiles []monitor.Profile
}

func (m *recordingModel) GenerateValidatedJSON(ctx context.Context, p monitor.Profile, prompt string, schema any, validate func([]byte) error) ([]byte, error) {
	m.profiles = append(m.profiles, p)
	return m.base.GenerateValidatedJSON(ctx, p, prompt, schema, validate)
}

func TestNotificationAgentUsesIndependentModelAndOneGlobalDecision(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	if _, err := s.DB.Exec(`INSERT INTO monitor_profiles(id,name,backend,model,endpoint) VALUES(3,'Strong notifications','openai_compatible','strong','http://localhost');UPDATE notification_settings SET profile_id=3 WHERE id=1;UPDATE tasks SET status='done',description='This work is waiting for an approval before merging' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	owner := int64(1)
	if err := store.PutSessionWork(ctx, s.DB, store.SessionWork{SessionID: "closed-work", TaskID: &owner, State: "exited", Summary: "Implementation is complete and awaiting approval", Evidence: "Waiting for reviewer", ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	set, err := s.SaveRules(ctx, 1, []NotificationRule{{ID: "R003", Text: "Group review work in the recurring task when useful", Enabled: true}})
	if err != nil {
		t.Fatal(err)
	}
	e := event()
	seed(t, s, source, e)
	other := event()
	other.ExternalID = "other-project"
	other.Repository = "acme/other"
	other.URL = "https://github.com/acme/other/issues/9"
	other.SubjectType = "Issue"
	seed(t, s, source, other)
	prior := publish(t, s, seed(t, s, source, Event{ExternalID: "prior", Version: "old", Repository: e.Repository, URL: e.URL, SubjectType: "PullRequest"}))
	if _, err = s.Inbox.Dismiss(ctx, prior.ID, prior.Revision); err != nil {
		t.Fatal(err)
	}
	// Only the fresh notifications are pending in this processing snapshot.
	if _, err = s.DB.Exec(`UPDATE notification_events SET raw_status='archived' WHERE external_id='prior'`); err != nil {
		t.Fatal(err)
	}
	model.respond = func(prompt string) (any, error) {
		snap := inputSnapshot(t, prompt)
		if len(snap.Subjects) != 2 || len(snap.Rules.Rules) != 1 || snap.Rules.Rules[0].ID != "R003" || snap.Rules.ID != set.ID {
			t.Fatal("incomplete global context", snap)
		}
		if len(snap.History) != 1 || snap.History[0].Status != "dismissed" {
			t.Fatal("missing handled history", snap.History)
		}
		if !strings.Contains(prompt, "waiting for an approval") || !strings.Contains(prompt, "closed-work") {
			t.Fatal("task description or closed-session evidence was omitted")
		}
		if !strings.Contains(prompt, `"status":"done"`) {
			t.Fatal("closed task context was omitted")
		}
		return finalResponse(snap, true), nil
	}
	recorder := &recordingModel{base: model}
	s.Model = recorder
	p := processAgent(t, s)
	if p.Status != "completed" || p.ActionCount != 1 || len(model.prompts) != 1 {
		t.Fatal(p, len(model.prompts))
	}
	if len(recorder.profiles) != 1 || recorder.profiles[0].ID != 3 || len(tools.calls) != 0 {
		t.Fatal("wrong model or work started during processing", recorder.profiles, tools.calls)
	}
	monitorProfile, err := monitor.Resolve(ctx, s.DB, 0)
	if err != nil || monitorProfile.ID != 1 {
		t.Fatal("notification selection affected monitoring", monitorProfile, err)
	}
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(cards.Items) != 1 || cards.Items[0].Options[0].ID != "investigate" || cards.Items[0].Options[0].Label != "Create investigation task" {
		t.Fatal("actionable proposal missing", cards)
	}
	var snapshot, result, config string
	s.DB.QueryRow(`SELECT processor_snapshot,processor_result,processor_config FROM notification_batches WHERE id=?`, p.ID).Scan(&snapshot, &result, &config)
	if !strings.Contains(snapshot, "R003") || !strings.Contains(result, "why_it_matters") || !strings.Contains(config, "strong") {
		t.Fatal("missing run provenance")
	}
}

func TestNotificationModelMustBeExplicitlySelected(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	s.DB.Exec(`UPDATE notification_settings SET profile_id=NULL WHERE id=1`)
	if _, err := s.RequestProcessing(ctx); err == nil || !strings.Contains(err.Error(), "choose a notification processing model") {
		t.Fatal("monitoring fallback was used", err)
	}
	status, err := s.Status(ctx)
	if err != nil || status.ModelConfigured || status.PendingCount != 1 {
		t.Fatal(status, err)
	}
}

func TestAgentReadToolsAreBoundedReadOnlyAndInOneConversation(t *testing.T) {
	s, model, tools, source := fixture(t)
	seed(t, s, source, event())
	calls := 0
	model.respond = func(prompt string) (any, error) {
		calls++
		snap := inputSnapshot(t, prompt)
		if strings.Contains(prompt, `"name":"create_task"`) || strings.Contains(prompt, `"name":"send_session_message"`) {
			t.Fatal("mutation tools were exposed to processing")
		}
		if calls == 1 {
			return AgentTurn{Action: "read", Reads: []AgentRead{{Name: "list_tasks", Arguments: `{"project_id":1}`}}, Messages: []AgentMessage{}, Decisions: []AgentDecision{}, SupersedeIDs: []string{}}, nil
		}
		if !strings.Contains(prompt, "read:1") || !strings.Contains(prompt, "bytes_written") {
			t.Fatal("read evidence lost between turns")
		}
		return finalResponse(snap, false), nil
	}
	p := processAgent(t, s)
	if p.Status != "completed" || len(tools.calls) != 1 || tools.calls[0] != "list_tasks" || p.ActionCount != 0 {
		t.Fatal(p, tools.calls)
	}
}

func TestAgentRejectsMutationAndInvalidCoverageWithoutArchiving(t *testing.T) {
	for _, mode := range []string{"mutation", "omitted", "unknown-rule", "unknown-evidence", "review-as-todo", "negative-task", "duplicate-pr"} {
		t.Run(mode, func(t *testing.T) {
			s, model, tools, source := fixture(t)
			seed(t, s, source, event())
			model.respond = func(prompt string) (any, error) {
				snap := inputSnapshot(t, prompt)
				out := finalResponse(snap, true)
				switch mode {
				case "mutation":
					out = AgentTurn{Action: "read", Reads: []AgentRead{{Name: "create_task", Arguments: `{"project_id":1}`}}}
				case "omitted":
					out.Decisions = nil
				case "unknown-rule":
					out.Messages[0].RuleIDs = []string{"invented"}
				case "unknown-evidence":
					out.Messages[0].EvidenceRefs = []string{"invented"}
				case "review-as-todo":
					out.Messages[0].Plan.WorkKind = "review"
				case "negative-task":
					out.Messages[0].Plan.TaskID = -1
				case "duplicate-pr":
					out.Messages[0].Plan.PRNumbers = []int64{7, 7}
				}
				return out, nil
			}
			p := processAgent(t, s)
			status, _ := s.Status(context.Background())
			if p.Status != "failed" || status.PendingCount != 1 || p.ActionCount != 0 || len(tools.calls) != 0 || len(model.prompts) != 2 {
				t.Fatal("invalid model response changed state or retried unboundedly", p, status, tools.calls, len(model.prompts))
			}
		})
	}
}

func TestAgentCorrectsAnInvalidResponseOnce(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	calls := 0
	model.respond = func(prompt string) (any, error) {
		calls++
		snap := inputSnapshot(t, prompt)
		out := finalResponse(snap, false)
		if calls == 1 {
			out.Decisions = nil
		} else if !strings.Contains(prompt, "Previous reply was rejected") {
			t.Fatal("missing bounded correction feedback")
		}
		return out, nil
	}
	if p := processAgent(t, s); p.Status != "completed" || calls != 2 {
		t.Fatal(p, calls)
	}
}

func TestAgentReadBudgetExhaustionKeepsInputsPending(t *testing.T) {
	s, model, tools, source := fixture(t)
	seed(t, s, source, event())
	s.DB.Exec(`UPDATE notification_settings SET max_reads=1 WHERE id=1`)
	model.respond = func(string) (any, error) {
		return AgentTurn{Action: "read", Reads: []AgentRead{{Name: "list_tasks", Arguments: `{"project_id":1}`}}}, nil
	}
	p := processAgent(t, s)
	status, _ := s.Status(context.Background())
	if p.Status != "failed" || status.PendingCount != 1 || len(tools.calls) != 1 {
		t.Fatal(p, status, tools.calls)
	}
}

func TestSettingsAndRuleChangesInvalidateAProcessingRun(t *testing.T) {
	for _, change := range []string{"settings", "rules"} {
		t.Run(change, func(t *testing.T) {
			s, model, _, source := fixture(t)
			seed(t, s, source, event())
			ctx := context.Background()
			model.respond = func(prompt string) (any, error) {
				snap := inputSnapshot(t, prompt)
				if change == "settings" {
					c, _ := s.Config(ctx)
					c.TimeoutSeconds++
					if _, err := s.SaveConfig(ctx, c); err != nil {
						t.Fatal(err)
					}
				} else {
					if _, err := s.SaveRules(ctx, 1, nil); err != nil {
						t.Fatal(err)
					}
				}
				return finalResponse(snap, true), nil
			}
			p := processAgent(t, s)
			status, _ := s.Status(ctx)
			cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
			if p.Status != "failed" || status.PendingCount != 1 || len(cards.Items) != 0 {
				t.Fatal("stale decisions published", p, status, cards)
			}
		})
	}
}

type changedSourceProvider struct{ *fakeProvider }

func (p *changedSourceProvider) Enrich(ctx context.Context, s Source, e Event) (Event, error) {
	out, err := p.fakeProvider.Enrich(ctx, s, e)
	out.SubjectUpdatedAt = "before"
	return out, err
}
func (p *changedSourceProvider) SubjectVersion(context.Context, Source, Event) (string, error) {
	return "after", nil
}
func TestSourceChangesBeforePublicationKeepNotificationsPending(t *testing.T) {
	s, model, _, source := fixture(t)
	s.Providers["github"] = &changedSourceProvider{&fakeProvider{}}
	seed(t, s, source, event())
	model.respond = func(p string) (any, error) { return finalResponse(inputSnapshot(t, p), true), nil }
	p := processAgent(t, s)
	status, _ := s.Status(context.Background())
	if p.Status != "failed" || status.PendingCount != 1 {
		t.Fatal(p, status)
	}
}

func TestAgentCannotEraseUnprocessedPartsOfGroupedHistory(t *testing.T) {
	snapshot := AgentSnapshot{Subjects: []AgentSubject{{EventIDs: []int64{1}, Event: Event{URL: "https://github.com/acme/repo/pull/1"}}}, HistorySources: map[string][]string{"old": {"https://github.com/acme/repo/pull/1", "https://github.com/acme/repo/pull/2"}}}
	snapshot.History = append(snapshot.History, inbox.Question{ID: "old", Status: "pending"})
	out := AgentTurn{Action: "finish", Decisions: []AgentDecision{{EventIDs: []int64{1}, Outcome: "ignore", Reason: "Resolved"}}, SupersedeIDs: []string{"old"}}
	if err := validateAgentResponse(snapshot, out, map[string]bool{}); err == nil {
		t.Fatal("unprocessed grouped request was silently retired")
	}
}

func TestTextRulesSaveDirectlyWithIDsAndOptimisticRevisions(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	set, err := s.SaveRules(ctx, 1, []NotificationRule{{ID: "R003", Text: "Group all bot reviews in the recurrent task", Enabled: true}})
	if err != nil || set.Status != "active" || set.Rules[0].ID != "R003" {
		t.Fatal(set, err)
	}
	if _, err = s.SaveRules(ctx, 1, nil); !errors.Is(err, ErrRulesChanged) {
		t.Fatal("stale rule edits accepted", err)
	}
	current, err := s.activeRuleSet(ctx)
	if err != nil || current.ID != set.ID {
		t.Fatal(current, err)
	}
}

func TestPreviewRunsTheSameAgentWithoutPublishingOrStartingWork(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	set, _ := s.activeRuleSet(ctx)
	model.respond = func(p string) (any, error) { return finalResponse(inputSnapshot(t, p), true), nil }
	p, err := s.QueueRulePreview(ctx, set.ID, []int64{id})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.processRulePreview(ctx); err != nil {
		t.Fatal(err)
	}
	result, _ := s.RulePreview(ctx, p.ID)
	status, _ := s.Status(ctx)
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if result.Status != "completed" || !strings.Contains(string(result.Result), "why_it_matters") || status.PendingCount != 1 || len(cards.Items) != 0 || len(tools.calls) != 0 {
		t.Fatal(result, status, cards)
	}
}

func TestAgentMigrationPreservesRuleTextAndRequeuesCards(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	id := seed(t, s, source, event())
	q := publish(t, s, id)
	raw, err := os.ReadFile("../store/migrations/00041_notification_agent.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "-- +goose Down")
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	rules := `[{"id":"R003","revision":2,"text":"Group bot reviews in a recurring task","enabled":true,"mode":"compiled","clauses":[{"op":"always"}]}]`
	if _, err = s.DB.Exec(`UPDATE notification_rule_sets SET rules_json=? WHERE status='active'`, rules); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE notification_events SET raw_status='archived'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO notification_runs(id,state,history,updated_at) VALUES(?,'running','[]',0)`, q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(parts[0]); err != nil {
		t.Fatal(err)
	}
	current, err := s.activeRuleSet(ctx)
	if err != nil || len(current.Rules) != 1 || current.Rules[0].ID != "R003" {
		t.Fatal(current, err)
	}
	var stored string
	s.DB.QueryRow(`SELECT rules_json FROM notification_rule_sets WHERE status='active'`).Scan(&stored)
	if strings.Contains(stored, "clauses") || strings.Contains(stored, "mode") {
		t.Fatal("compiled payload survived", stored)
	}
	status, _ := s.Status(ctx)
	old, _ := s.Inbox.Get(ctx, q.ID)
	var run string
	s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, q.ID).Scan(&run)
	if status.PendingCount != 1 || status.ModelConfigured || old.Status != "unanswered" || run != "uncertain" {
		t.Fatal("migration lost pending work or replayed execution", status, old, run)
	}
}

func TestModelSuggestionsNeverSaveRulesAutomatically(t *testing.T) {
	s, model, _, _ := fixture(t)
	ctx := context.Background()
	set, err := s.SaveRules(ctx, 1, []NotificationRule{{ID: "R001", Enabled: true, Text: "Ignore irrelevant activity"}})
	if err != nil {
		t.Fatal(err)
	}
	model.responses = []any{map[string]any{"diagnosis": "Clarify which activity is relevant", "cause": "rule", "risks": "Preserve direct requests", "changes": []RuleTextChange{{RuleID: "R001", Text: "Ignore routine bot no-findings reports", Explanation: "Explicit preference"}}}}
	result, err := s.ImproveRules(ctx, RuleImprovementRequest{BaseRevision: set.Revision, Kind: "not_relevant", Instructions: "Ignore routine bot no-findings messages"})
	if err != nil || len(result.Proposal) != 1 {
		t.Fatal(result, err)
	}
	current, _ := s.activeRuleSet(ctx)
	if current.ID != set.ID || current.Rules[0].Text != set.Rules[0].Text {
		t.Fatal("suggestion silently changed active rules")
	}
}

func TestPublicationChecksModelAndSourceInsideItsTransaction(t *testing.T) {
	for _, change := range []string{"model", "source", "integration"} {
		t.Run(change, func(t *testing.T) {
			s, _, _, source := fixture(t)
			ctx := context.Background()
			id := seed(t, s, source, event())
			batch, err := s.RequestProcessing(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.DB.Exec(`UPDATE notification_batches SET status='running' WHERE id=?`, batch.ID); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "model":
				c, _ := s.Config(ctx)
				c.TimeoutSeconds++
				if _, err = s.SaveConfig(ctx, c); err != nil {
					t.Fatal(err)
				}
			case "source":
				if err = s.EnableSource(ctx, source.ID, false); err != nil {
					t.Fatal(err)
				}
			case "integration":
				if _, err = s.DB.Exec(`INSERT INTO settings(key,value) VALUES('github_integration','off') ON CONFLICT(key) DO UPDATE SET value='off'`); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.Inbox.ApplyNotificationBatch(ctx, batch.ID, nil, nil); !errors.Is(err, inbox.ErrConflict) {
				t.Fatal("stale publication was accepted", err)
			}
			var status string
			s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, id).Scan(&status)
			if status != "pending" {
				t.Fatal("publication partially archived input", status)
			}
		})
	}
}
func TestAgentMessagesHaveSourceScopedEvidenceAndNoDuplicates(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	first := event()
	other := event()
	other.ExternalID = "second"
	other.URL = "https://github.com/acme/repo/pull/8"
	events := []RawNotification{{ID: seed(t, s, source, first), SourceID: source.ID, Event: first}, {ID: seed(t, s, source, other), SourceID: source.ID, Event: other}}
	rules, _ := s.activeRuleSet(ctx)
	snapshot, err := s.agentSnapshot(ctx, events, rules)
	if err != nil {
		t.Fatal(err)
	}
	evidence := map[string]bool{}
	for _, sub := range snapshot.Subjects {
		for _, e := range sub.Event.Evidence {
			evidence[e.ID] = true
		}
	}
	out := finalResponse(snapshot, true)
	out.Messages[0].EvidenceRefs = []string{snapshot.Subjects[1].Event.Evidence[0].ID}
	if err = validateAgentResponse(snapshot, out, evidence); err == nil {
		t.Fatal("unrelated source evidence was accepted")
	}
	out = finalResponse(snapshot, true)
	out.Messages = append(out.Messages, out.Messages[0])
	if err = validateAgentResponse(snapshot, out, evidence); err == nil {
		t.Fatal("duplicated feedback card was accepted")
	}
}
func TestSuggestedRepliesRequireInterpretationRatherThanAutomaticExecution(t *testing.T) {
	s, _, _, source := fixture(t)
	ctx := context.Background()
	e := event()
	events := []RawNotification{{ID: seed(t, s, source, e), SourceID: source.ID, Event: e}}
	rules, _ := s.activeRuleSet(ctx)
	snapshot, err := s.agentSnapshot(ctx, events, rules)
	if err != nil {
		t.Fatal(err)
	}
	out := finalResponse(snapshot, true)
	out.Messages[0].Plan.Workflow = "clarify"
	out.Messages[0].Options = []AgentOption{{ID: "explain", Label: "Explain the missing information", Mode: "reply", Actions: []inboxaction.Action{}}}
	actions, _, _, err := s.agentActions(ctx, snapshot, out)
	if err != nil {
		t.Fatal(err)
	}
	options := actions[0].Request.Options
	if len(options) != 2 || options[0].ID != "explain" || options[0].Action != nil || options[1].ID != "triage_noop" {
		t.Fatal("reply choices were conflated with approval", options)
	}
}

func TestMigrationKeepsSavedRulesThatNeverCompiled(t *testing.T) {
	s, _, _, _ := fixture(t)
	ctx := context.Background()
	raw, err := os.ReadFile("../store/migrations/00041_notification_agent.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "-- +goose Down")
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE notification_rule_state SET active_set_id=NULL;UPDATE notification_rule_sets SET status='invalid',rules_json='[{"id":"R003","revision":1,"text":"Group bot reviews in the recurrent task","enabled":true,"mode":"llm","clauses":[]}]'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(parts[0]); err != nil {
		t.Fatal(err)
	}
	rules, err := s.activeRuleSet(ctx)
	if err != nil || len(rules.Rules) != 1 || rules.Rules[0].ID != "R003" || rules.Status != "active" {
		t.Fatal("uncompiled user instructions were replaced with an empty list", rules, err)
	}
}

func TestNotificationAgentWithoutReadsUsesFullContextBudget(t *testing.T) {
	s, model, _, _ := fixture(t)
	snapshot := AgentSnapshot{Rules: RuleSet{Rules: []NotificationRule{}}}
	for i := 0; i < 10; i++ {
		snapshot.Rules.Rules = append(snapshot.Rules.Rules, NotificationRule{ID: strings.Repeat("R", i+1), Text: strings.Repeat("a", 4000), Enabled: true})
	}
	model.respond = func(prompt string) (any, error) { return finalResponse(inputSnapshot(t, prompt), false), nil }
	_, _, err := s.runNotificationAgent(context.Background(), snapshot, Config{ContextBytes: 64000, MaxReads: 0})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCompactAgentIndexPreservesEvidenceAndDeliveryOutcomes(t *testing.T) {
	snapshot := AgentSnapshot{
		Subjects: []AgentSubject{{ID: "S1", EventIDs: []int64{1}, Event: Event{URL: "https://github.com/acme/repo/pull/7", Context: strings.Repeat("source ", 10000), Evidence: []SubjectEvidence{{ID: "S1/E1"}}}}},
		History:  []inbox.Question{{ID: "Q1", Status: "answered", Answer: "Continue", DeliveryStatus: "uncertain", ExecutionResult: "Delivery was interrupted; check the task"}},
	}
	raw, err := snapshotView(snapshot, 64000)
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Subjects []struct {
			EvidenceIDs     []string `json:"evidence_ids"`
			DetailAvailable bool     `json:"detail_available"`
		} `json:"subjects"`
		History []inbox.Question `json:"inbox_history"`
	}
	if err = json.Unmarshal([]byte(raw), &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Subjects) != 1 || !index.Subjects[0].DetailAvailable || len(index.Subjects[0].EvidenceIDs) != 1 || index.Subjects[0].EvidenceIDs[0] != "S1/E1" || len(index.History) != 1 || index.History[0].DeliveryStatus != "uncertain" || index.History[0].ExecutionResult == "" {
		t.Fatal(raw)
	}
}

func TestAgentRejectsUnavailableLookupAsEvidence(t *testing.T) {
	s, model, _, source := fixture(t)
	seed(t, s, source, event())
	model.respond = func(prompt string) (any, error) {
		snap := inputSnapshot(t, prompt)
		if len(model.prompts) == 1 {
			return AgentTurn{Action: "read", Reads: []AgentRead{{Name: "read_subject", Arguments: `{"id":"missing","offset":0}`}}}, nil
		}
		out := finalResponse(snap, true)
		out.Messages[0].EvidenceRefs = append(out.Messages[0].EvidenceRefs, "read:1")
		return out, nil
	}
	p := processAgent(t, s)
	if p.Status != "failed" || p.ActionCount != 0 || !strings.Contains(p.Error, "unknown evidence") {
		t.Fatal(p)
	}
	status, _ := s.Status(context.Background())
	if status.PendingCount != 1 {
		t.Fatal(status)
	}
}

func TestAgentMessageNeedsItsOwnSourceEvidenceAlongsideLookups(t *testing.T) {
	snap := AgentSnapshot{Subjects: []AgentSubject{{EventIDs: []int64{1}, Event: Event{Evidence: []SubjectEvidence{{ID: "S1/E1"}}}}}}
	out := finalResponse(snap, true)
	out.Messages[0].EvidenceRefs = []string{"read:1"}
	if err := validateAgentResponse(snap, out, map[string]bool{"read:1": true, "S1/E1": true}); err == nil {
		t.Fatal("unrelated lookup accepted as the only source evidence")
	}
}

func TestSavingRulesRequeuesUnexecutedInputsButNotRunningActions(t *testing.T) {
	for _, state := range []string{"queued", "running"} {
		t.Run(state, func(t *testing.T) {
			s, _, tools, source := fixture(t)
			ctx := context.Background()
			eventID := seed(t, s, source, event())
			q := publish(t, s, eventID)
			if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Check and fix", ""); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`UPDATE notification_events SET raw_status='archived'`); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`UPDATE notification_runs SET state=?`, state); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`UPDATE inbox_questions SET delivery_status=? WHERE id=?`, state, q.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.SaveRules(ctx, 1, []NotificationRule{{ID: "R1", Enabled: true, Text: "Show actionable feedback"}}); err != nil {
				t.Fatal(err)
			}
			current, err := s.Inbox.Get(ctx, q.ID)
			if err != nil {
				t.Fatal(err)
			}
			var rawStatus, runState string
			if err = s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, eventID).Scan(&rawStatus); err != nil {
				t.Fatal(err)
			}
			if err = s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, q.ID).Scan(&runState); err != nil {
				t.Fatal(err)
			}
			if state == "queued" {
				if rawStatus != "pending" || runState != "failed" || current.DeliveryStatus != "failed" || !strings.Contains(current.ExecutionResult, "rules changed") {
					t.Fatal(rawStatus, runState, current)
				}
				if err = s.executeNext(ctx); err != nil || len(tools.calls) > 0 {
					t.Fatal("cancelled action was executed", err, tools.calls)
				}
			} else if rawStatus != "archived" || runState != "running" || current.DeliveryStatus != "running" {
				t.Fatal("running action was replayed or recategorized", rawStatus, runState, current)
			}
		})
	}
}

func TestAgentAuditDecisionLinksToItsProcessingRun(t *testing.T) {
	s, model, _, source := fixture(t)
	id := seed(t, s, source, event())
	model.respond = func(prompt string) (any, error) { return finalResponse(inputSnapshot(t, prompt), false), nil }
	p := processAgent(t, s)
	decisions, err := s.EventDecisions(context.Background(), id)
	if err != nil || p.Status != "completed" || len(decisions) != 1 || decisions[0].BatchID != p.ID {
		t.Fatal(p, decisions, err)
	}
}

func TestAgentClarificationPreservesKnownTaskWithoutExecution(t *testing.T) {
	s, model, tools, source := fixture(t)
	ctx := context.Background()
	seed(t, s, source, event())
	model.respond = func(prompt string) (any, error) {
		out := finalResponse(inputSnapshot(t, prompt), true)
		out.Messages[0].Question = "Which part of the feedback should this task address?"
		out.Messages[0].Plan = AgentPlan{Workflow: "clarify", TaskID: 1, ProjectID: 0, WorkKind: "implementation"}
		out.Messages[0].Options = nil
		return out, nil
	}
	p := processAgent(t, s)
	cards, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if err != nil || p.Status != "completed" || len(cards.Items) != 1 {
		t.Fatal(p, cards, err)
	}
	card := cards.Items[0]
	if len(card.LinkedTasks) != 1 || card.LinkedTasks[0].TaskID != 1 || card.LinkedTasks[0].Role != "implementation" || len(card.Options) != 1 || card.Options[0].ID != "triage_noop" || len(tools.calls) > 0 {
		t.Fatal(card, tools.calls)
	}
}
