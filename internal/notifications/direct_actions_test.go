package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

type directRuntime struct {
	s            *Service
	sessions     map[string]ActionSession
	submitted    []string
	stopped      []string
	moved        []string
	submitErr    error
	readyErr     error
	beforeSubmit func()
}

func (r *directRuntime) Session(_ context.Context, id string) (ActionSession, error) {
	v, ok := r.sessions[id]
	if !ok {
		return v, errors.New("unknown session")
	}
	return v, nil
}
func (r *directRuntime) Ready(ctx context.Context, id string, policy string) (ActionSession, error) {
	if r.readyErr != nil {
		return ActionSession{}, r.readyErr
	}
	return r.Session(ctx, id)
}
func (r *directRuntime) Submit(ctx context.Context, v ActionSession, msg string) error {
	if r.beforeSubmit != nil {
		r.beforeSubmit()
	}
	current, err := r.Session(ctx, v.ID)
	if err != nil {
		return err
	}
	if current.TaskID != v.TaskID || current.Input != v.Input {
		return fmt.Errorf("%w: session changed", ErrActionBlocked)
	}
	r.submitted = append(r.submitted, msg)
	if r.submitErr != nil {
		return r.submitErr
	}
	current.Input++
	r.sessions[v.ID] = current
	return nil
}
func (r *directRuntime) Stop(ctx context.Context, v ActionSession) error {
	current, err := r.Session(ctx, v.ID)
	if err != nil {
		return err
	}
	if current.Input != v.Input {
		return ErrActionBlocked
	}
	r.stopped = append(r.stopped, v.ID)
	delete(r.sessions, v.ID)
	return nil
}
func (r *directRuntime) Move(ctx context.Context, id int64, expected, status string) error {
	var current string
	if err := r.s.DB.QueryRow(`SELECT status FROM tasks WHERE id=?`, id).Scan(&current); err != nil {
		return err
	}
	if current != expected {
		return ErrActionBlocked
	}
	r.moved = append(r.moved, status)
	_, err := r.s.DB.Exec(`UPDATE tasks SET status=? WHERE id=?`, status, id)
	return err
}

type directTools struct {
	s      *Service
	r      *directRuntime
	calls  []string
	args   []json.RawMessage
	fail   string
	before func(string)
}

func (t *directTools) List(context.Context) ([]Tool, error) { return []Tool{}, nil }
func (t *directTools) Call(ctx context.Context, name string, raw json.RawMessage) (string, error) {
	t.calls = append(t.calls, name)
	t.args = append(t.args, append(json.RawMessage(nil), raw...))
	if t.before != nil {
		t.before(name)
	}
	if t.fail == name {
		return "", errors.New("simulated transport failure")
	}
	var a struct {
		Project     int64  `json:"project_id"`
		PR          int64  `json:"pr_number"`
		Task        int64  `json:"task_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return "", err
	}
	switch name {
	case "list_sessions":
		out := []map[string]any{}
		for id, s := range t.r.sessions {
			if s.TaskID == a.Task {
				out = append(out, map[string]any{"id": id, "taskId": s.TaskID, "kind": "agent", "status": "running"})
			}
		}
		return mustJSON(out), nil
	case "open_review":
		var id int64
		err := t.s.DB.QueryRow(`SELECT id FROM tasks WHERE project_id=? AND pr_number=? AND source='github_pr'`, a.Project, a.PR).Scan(&id)
		if err != nil {
			err = t.s.DB.QueryRow(`INSERT INTO tasks(project_id,title,source,pr_number,status,position) VALUES(?,'Review','github_pr',?,'todo',0) RETURNING id`, a.Project, a.PR).Scan(&id)
		}
		return mustJSON(map[string]any{"task": map[string]any{"id": id}}), err
	case "start_task_agent":
		id := fmt.Sprintf("agent-%d", a.Task)
		t.r.sessions[id] = ActionSession{ID: id, TaskID: a.Task, Revision: 1}
		return mustJSON(map[string]any{"id": id, "taskId": a.Task}), nil
	case "create_task":
		var id int64
		err := t.s.DB.QueryRow(`INSERT INTO tasks(project_id,title,description,status,position) VALUES(?,?,?,'todo',0) RETURNING id`, a.Project, a.Title, a.Description).Scan(&id)
		return mustJSON(map[string]any{"id": id}), err
	case "update_task":
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		if _, ok := fields["title"]; ok {
			if _, err := t.s.DB.Exec(`UPDATE tasks SET title=? WHERE id=?`, a.Title, a.Task); err != nil {
				return "", err
			}
		}
		if _, ok := fields["description"]; ok {
			if _, err := t.s.DB.Exec(`UPDATE tasks SET description=? WHERE id=?`, a.Description, a.Task); err != nil {
				return "", err
			}
		}
		return `{"ok":true}`, nil
	case "set_task_pr_link":
		return `{"ok":true}`, nil
	default:
		return "", fmt.Errorf("unexpected tool %s", name)
	}
}
func directFixture(t *testing.T) (*Service, *fakeModel, *directRuntime, *directTools, Source) {
	t.Helper()
	s, m, _, source := fixture(t)
	r := &directRuntime{s: s, sessions: map[string]ActionSession{}}
	tools := &directTools{s: s, r: r}
	s.Actions = r
	s.Tools = tools
	s.TaskSession = func(task int64) string {
		for id, v := range r.sessions {
			if v.TaskID == task {
				return id
			}
		}
		return ""
	}
	s.SessionTask = func(id string) (int64, bool) { v, ok := r.sessions[id]; return v.TaskID, ok }
	return s, m, r, tools, source
}
func directCard(t *testing.T, s *Service, source Source, actions ...inboxaction.Action) inbox.Question {
	t.Helper()
	ctx := context.Background()
	e := event()
	id := seed(t, s, source, e)
	set, err := s.activeRuleSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seq, err := s.prepareSequence(ctx, inboxaction.Sequence{Version: 1, Actions: actions}, set.ID, map[string]Source{source.ID: source}, map[string]Event{"source": e}, false)
	if err != nil {
		t.Fatal(err)
	}
	c := ask()
	c.Options = []inbox.Option{{ID: "go", Label: "Run the saved action", Action: &seq, ActionSummary: seq.Summary()}}
	q, err := s.Inbox.PublishNotification(ctx, id, "GitHub", e.URL, c)
	if err != nil {
		t.Fatal(err)
	}
	attachTestPlan(t, s, q.ID)
	return q
}
func runDirect(t *testing.T, s *Service, q inbox.Question) inbox.Question {
	t.Helper()
	ctx := context.Background()
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, err := s.Inbox.Get(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	return out
}
func TestDirectSevenReviewsNeedNoModelAndSubmitEveryConfiguredPrompt(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	if err := settings.Set(s.DB, settings.KeyPRReviewSeed, "Configured review <n>: <title>."); err != nil {
		t.Fatal(err)
	}
	q := directCard(t, s, source, inboxaction.Action{ID: "reviews", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{11, 12, 13, 14, 15, 16, 17}, Instructions: "Never replace the configured initial prompt"})
	// Direct options remain executable even if no inference profile is selected.
	if _, err := s.DB.Exec(`UPDATE notification_settings SET profile_id=NULL`); err != nil {
		t.Fatal(err)
	}
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(m.prompts) != 0 || len(r.submitted) != 7 || len(out.LinkedTasks) != 7 {
		t.Fatal(out, len(m.prompts), r.submitted)
	}
	for i, msg := range r.submitted {
		want := fmt.Sprintf("Configured review %d: Change.", 11+i)
		if msg != want {
			t.Fatal(msg, want)
		}
	}
	if len(tools.calls) != 21 {
		t.Fatal("expected open, sessions and start per review", tools.calls)
	}
	run, err := s.ActionRun(context.Background(), q.ID)
	if err != nil || len(run.Steps) != 43 {
		t.Fatal(run, err)
	}
	if err = s.executeNext(context.Background()); err != nil || len(r.submitted) != 7 {
		t.Fatal("completed action was replayed", err)
	}
}
func TestRecurringDirectHandoffDoesNotNeedCompletionInference(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	s.DB.Exec(`UPDATE tasks SET recurrent=1 WHERE id=1`)
	q := directCard(t, s, source, inboxaction.Action{ID: "renovate", Kind: "run_recurring_task", ProjectID: 1, TaskID: 1, Instructions: "Review all four linked Renovate PRs together. Report findings locally."})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || len(m.prompts) != 0 {
		t.Fatal(out, r.submitted, m.prompts)
	}
}
func TestDirectContinueReusesLiveAgent(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	r.sessions["existing"] = ActionSession{ID: "existing", TaskID: 1, Revision: 1}
	q := directCard(t, s, source, inboxaction.Action{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Address the reviewer feedback and report the result."})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || len(m.prompts) > 0 || slices.Contains(tools.calls, "start_task_agent") {
		t.Fatal(out, tools.calls)
	}
}
func TestInvestigationSequenceResolvesNewTaskReference(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	q := directCard(t, s, source, inboxaction.Action{ID: "investigate", Kind: "create_investigation", ProjectID: 1, TaskName: "Investigate source", Instructions: "Investigate the reported issue; draft findings."}, inboxaction.Action{ID: "describe", Kind: "update_task", ProjectID: 1, TaskRef: "investigate", Description: "Saved source reference and accepted investigation scope."})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || len(m.prompts) != 0 || len(out.LinkedTasks) != 1 || !slices.Contains(tools.calls, "update_task") {
		t.Fatal(out, tools.calls)
	}
}
func TestDirectCompletionStopsOnlyCapturedSessionThenMovesTask(t *testing.T) {
	for _, status := range []string{"in_review", "done"} {
		t.Run(status, func(t *testing.T) {
			s, m, r, _, source := directFixture(t)
			r.sessions["owned"] = ActionSession{ID: "owned", TaskID: 1, Input: 2}
			r.sessions["other"] = ActionSession{ID: "other", TaskID: 2}
			q := directCard(t, s, source, inboxaction.Action{ID: "finish", Kind: "complete_task", ProjectID: 1, TaskID: 1, SessionID: "owned", Status: status})
			out := runDirect(t, s, q)
			if out.DeliveryStatus != "succeeded" || len(m.prompts) > 0 || len(r.stopped) != 1 || r.stopped[0] != "owned" || len(r.moved) != 1 || r.moved[0] != status {
				t.Fatal(out, r.stopped, r.moved)
			}
			if _, ok := r.sessions["other"]; !ok {
				t.Fatal("unrelated session stopped")
			}
		})
	}
}
func TestDirectStaleSessionOrTaskBlocksBeforeSideEffects(t *testing.T) {
	for _, change := range []string{"input", "status", "role", "rules", "template", "source"} {
		t.Run(change, func(t *testing.T) {
			s, m, r, tools, source := directFixture(t)
			r.sessions["owned"] = ActionSession{ID: "owned", TaskID: 1, Input: 2}
			a := inboxaction.Action{ID: "finish", Kind: "complete_task", ProjectID: 1, TaskID: 1, SessionID: "owned", Status: "done"}
			if change == "role" {
				a = inboxaction.Action{ID: "run", Kind: "run_recurring_task", ProjectID: 1, TaskID: 1, Instructions: "Run saved task instructions"}
				s.DB.Exec(`UPDATE tasks SET recurrent=1 WHERE id=1`)
			}
			if change == "template" {
				a = inboxaction.Action{ID: "review", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7}}
			}
			q := directCard(t, s, source, a)
			switch change {
			case "input":
				v := r.sessions["owned"]
				v.Input++
				r.sessions["owned"] = v
			case "status":
				s.DB.Exec(`UPDATE tasks SET status='done' WHERE id=1`)
			case "role":
				s.DB.Exec(`UPDATE tasks SET recurrent=0 WHERE id=1`)
			case "rules":
				s.DB.Exec(`UPDATE notification_rule_state SET active_set_id=NULL`)
			case "template":
				settings.Set(s.DB, settings.KeyPRReviewSeed, "Changed prompt")
			case "source":
				s.EnableSource(context.Background(), source.ID, false)
			}
			out := runDirect(t, s, q)
			if !slices.Contains([]string{"blocked", "failed"}, out.DeliveryStatus) || len(r.stopped) > 0 || len(r.submitted) > 0 || len(tools.calls) > 0 || len(m.prompts) > 0 {
				t.Fatal(out, tools.calls)
			}
		})
	}
}
func TestUncertainSubmissionIsRecordedAndNeverResent(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	r.submitErr = errors.New("connection lost after write")
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the review feedback"})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "uncertain" || len(r.submitted) != 1 {
		t.Fatal(out, r.submitted)
	}
	if err := s.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	run, err := s.ActionRun(context.Background(), q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.submitted) != 1 || len(m.prompts) != 0 || run.Steps[len(run.Steps)-1].State != "uncertain" {
		t.Fatal(run, r.submitted, m.prompts)
	}
}
func TestReadyFailureDoesNotClaimPromptDelivery(t *testing.T) {
	s, _, r, _, source := directFixture(t)
	r.readyErr = fmt.Errorf("%w: agent is busy", ErrActionBlocked)
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the feedback"})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "partial" || len(r.submitted) > 0 || !strings.Contains(out.ExecutionResult, "agent is busy") {
		t.Fatal(out)
	}
}
func TestDirectIgnoreNeedsNeitherModelNorSourceAccess(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	q := directCard(t, s, source, inboxaction.Action{ID: "ignore", Kind: "ignore"})
	s.EnableSource(context.Background(), source.ID, false)
	s.Model = nil
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(m.prompts) > 0 || len(r.submitted) > 0 || len(tools.calls) > 0 {
		t.Fatal(out)
	}
}
func TestFreeTextReplyPlansOnceThenExecutesWithoutModelLoop(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	q := directCard(t, s, source, inboxaction.Action{ID: "ignore", Kind: "ignore"})
	m.responses = []any{map[string]any{"actions": []inboxaction.Action{{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check this feedback and report findings"}}, "question": "", "context": "", "authorization_quote": "Check the feedback in task 1"}}
	ctx := context.Background()
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Check the feedback in task 1", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	if out.DeliveryStatus != "succeeded" || len(m.prompts) != 1 || len(r.submitted) != 1 || len(tools.calls) != 2 {
		t.Fatal(out, len(m.prompts), r.submitted, tools.calls)
	}
}
func TestUnsupportedFreeTextBecomesOneClarificationWithoutMutation(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	q := directCard(t, s, source, inboxaction.Action{ID: "ignore", Kind: "ignore"})
	m.responses = []any{map[string]any{"actions": []inboxaction.Action{}, "question": "Which task should receive the investigation?", "context": "Two tasks cover different parts of this request. Choose the target.", "authorization_quote": ""}}
	ctx := context.Background()
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Look into the two related issues", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	cards, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if out.DeliveryStatus != "succeeded" || len(m.prompts) != 1 || len(r.submitted) > 0 || len(tools.calls) > 0 || len(cards.Items) != 1 {
		t.Fatal(out, cards)
	}
}

func TestOptionSpecificTargetsAreSavedAndOnlySelectedSubsetRuns(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	ctx := context.Background()
	e := event()
	id := seed(t, s, source, e)
	other := event()
	other.ExternalID = "other"
	other.URL = "https://github.com/acme/repo/pull/8"
	id2 := seed(t, s, source, other)
	rules, _ := s.activeRuleSet(ctx)
	snapshot, err := s.agentSnapshot(ctx, []RawNotification{{ID: id, SourceID: source.ID, Event: e}, {ID: id2, SourceID: source.ID, Event: other}}, rules)
	if err != nil {
		t.Fatal(err)
	}
	out := finalResponse(snapshot, false)
	for i := range out.Decisions {
		out.Decisions[i].Outcome = "message"
	}
	msg := AgentMessage{EventIDs: []int64{id, id2}, Summary: "Two reviews await feedback", Context: "Review requests for two PRs", Question: "Review both or only the second?", Why: "Both requested review", EvidenceRefs: []string{snapshot.Subjects[0].Event.Evidence[0].ID, snapshot.Subjects[1].Event.Evidence[0].ID}, Plan: AgentPlan{Workflow: "route_review", ProjectID: 1, PRNumbers: []int64{7, 8}, WorkKind: "review"}, Options: []AgentOption{
		{ID: "all", Label: "Review both", Mode: "action", Actions: []inboxaction.Action{{ID: "reviews", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7, 8}}}},
		{ID: "second", Label: "Review only the second", Mode: "action", Actions: []inboxaction.Action{{ID: "reviews", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{8}}}},
	}}
	out.Messages = []AgentMessage{msg}
	actions, _, _, err := s.agentActions(ctx, snapshot, out)
	if err != nil {
		t.Fatal(err)
	}
	q, err := s.Inbox.PublishNotification(ctx, id, "GitHub", e.URL, actions[0].Request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "second"); err != nil {
		t.Fatal(err)
	}
	if err = s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Inbox.Get(ctx, q.ID)
	if current.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || !strings.Contains(r.submitted[0], "#8") || len(m.prompts) > 0 {
		t.Fatal(current, r.submitted, m.prompts)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM tasks WHERE source='github_pr' AND pr_number=7`).Scan(&count)
	if count != 0 {
		t.Fatal("unselected PR was opened")
	}
}
func TestActionRejectsInventedTargetsAndArbitraryTools(t *testing.T) {
	for _, mode := range []string{"other-pr", "wrong-project", "arbitrary-tool", "review-as-task", "forward-ref"} {
		t.Run(mode, func(t *testing.T) {
			s, _, _, _, source := directFixture(t)
			ctx := context.Background()
			e := event()
			id := seed(t, s, source, e)
			rules, _ := s.activeRuleSet(ctx)
			snap, err := s.agentSnapshot(ctx, []RawNotification{{ID: id, SourceID: source.ID, Event: e}}, rules)
			if err != nil {
				t.Fatal(err)
			}
			out := finalResponse(snap, true)
			out.Messages[0].Plan.WorkKind = "review"
			out.Messages[0].Plan.Workflow = "route_review"
			out.Messages[0].Plan.PRNumbers = []int64{7}
			a := inboxaction.Action{ID: "review", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7}}
			switch mode {
			case "other-pr":
				a.PRNumbers = []int64{999}
			case "wrong-project":
				a.ProjectID = 2
			case "arbitrary-tool":
				a.Kind = "delete_task"
			case "review-as-task":
				a = inboxaction.Action{ID: "task", Kind: "create_investigation", ProjectID: 1, TaskName: "Review", Instructions: "Review"}
			case "forward-ref":
				a = inboxaction.Action{ID: "next", Kind: "continue_task", ProjectID: 1, TaskRef: "future", Instructions: "Continue"}
			}
			out.Messages[0].Options = []AgentOption{{ID: "go", Label: "Run", Mode: "action", Actions: []inboxaction.Action{a}}}
			if _, _, _, err = s.agentActions(ctx, snap, out); err == nil {
				t.Fatal("invalid option accepted")
			}
		})
	}
}
func TestKnownSuccessfulStepIsNotRepeatedAfterRestart(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash immediately after a confirmed session start was recorded.
	if _, err := s.DB.Exec(`UPDATE notification_runs SET state='running' WHERE id=?`, q.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,result,started_at,finished_at) VALUES(?, 'run/sessions','run','list_sessions',0,'{}','succeeded','[]',1,2),(?,'run/start','run','start_task_agent',1,'{}','succeeded','{"id":"surviving","taskId":1}',3,4)`, q.ID, q.ID); err != nil {
		t.Fatal(err)
	}
	r.sessions["surviving"] = ActionSession{ID: "surviving", TaskID: 1, Revision: 1}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	if out.DeliveryStatus != "succeeded" || slices.Contains(tools.calls, "start_task_agent") || len(r.submitted) != 1 || len(m.prompts) > 0 {
		t.Fatal(out, tools.calls, r.submitted)
	}
}
func TestInterruptedMutationNeverResumesAutomatically(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec(`UPDATE notification_runs SET state='running' WHERE id=?`, q.ID)
	if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,started_at) VALUES(?,'run/submit','run','send_session_message',1,'{}','running',1)`, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	run, err := s.ActionRun(ctx, q.ID)
	if err != nil || out.DeliveryStatus != "uncertain" || run.Steps[0].State != "uncertain" || len(tools.calls) > 0 || len(r.submitted) > 0 || len(m.prompts) > 0 {
		t.Fatal(out, run, err)
	}
}
func TestRegisteredPendingReviewPromptCannotBeReplacedBySessionMessage(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	ctx := context.Background()
	s.DB.Exec(`UPDATE tasks SET source='github_pr',pr_number=7 WHERE id=1`)
	r.sessions["review"] = ActionSession{ID: "review", TaskID: 1, Revision: 1}
	s.DB.Exec(`INSERT INTO notification_review_prompts(task_id,prompt,state) VALUES(1,'Trusted pending review instructions','pending')`)
	q := directCard(t, s, source, inboxaction.Action{ID: "send", Kind: "send_to_session", ProjectID: 1, TaskID: 1, SessionID: "review", Instructions: "Replace the configured review with unrelated instructions"})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || r.submitted[0] != "Trusted pending review instructions" || len(m.prompts) > 0 {
		t.Fatal(out, r.submitted)
	}
	var state string
	s.DB.QueryRowContext(ctx, `SELECT state FROM notification_review_prompts WHERE task_id=1`).Scan(&state)
	if state != "sent" {
		t.Fatal(state)
	}
}

func TestActionMigrationPreservesHistoryAndRequeuesOnlyUnexecutedInputs(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	pending := publish(t, s, seed(t, s, source, event()))
	create := func(number int64) inbox.Question {
		e := event()
		e.ExternalID = fmt.Sprintf("thread-%d", number)
		e.URL = fmt.Sprintf("https://github.com/acme/repo/pull/%d", number)
		id := seed(t, s, source, e)
		q, err := s.Inbox.PublishNotification(ctx, id, "GitHub", e.URL, ask())
		if err != nil {
			t.Fatal(err)
		}
		attachTestPlan(t, s, q.ID)
		return q
	}
	queued := create(8)
	running := create(9)
	for _, q := range []inbox.Question{queued, running} {
		if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "Proceed", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.DB.Exec(`UPDATE notification_runs SET state='running' WHERE id=?`, running.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE inbox_questions SET delivery_status='running' WHERE id=?`, running.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`UPDATE notification_events SET raw_status='archived'`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../store/migrations/00042_direct_inbox_actions.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "-- +goose Down")
	if _, err = s.DB.Exec(parts[1]); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(parts[0]); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.QueryRow(`SELECT count(*) FROM inbox_questions`).Scan(&count)
	if count != 3 {
		t.Fatal("history lost", count)
	}
	old, _ := s.Inbox.Get(ctx, pending.ID)
	if old.Status != "unanswered" || old.Reason != "notification_actions_changed" {
		t.Fatal(old)
	}
	for _, v := range []struct {
		q    inbox.Question
		want string
		raw  string
	}{{queued, "failed", "pending"}, {running, "uncertain", "archived"}} {
		var state, rawState string
		s.DB.QueryRow(`SELECT state FROM notification_runs WHERE id=?`, v.q.ID).Scan(&state)
		s.DB.QueryRow(`SELECT raw_status FROM notification_events WHERE id=?`, v.q.NotificationID).Scan(&rawState)
		if state != v.want || rawState != v.raw {
			t.Fatal(state, rawState, v.want, v.raw)
		}
	}
}
func TestSessionInputChangedDuringModelPlanningCannotBeRebased(t *testing.T) {
	s, _, r, _, source := directFixture(t)
	ctx := context.Background()
	r.sessions["owned"] = ActionSession{ID: "owned", TaskID: 1, Input: 1}
	e := event()
	id := seed(t, s, source, e)
	rules, _ := s.activeRuleSet(ctx)
	snap, err := s.agentSnapshot(ctx, []RawNotification{{ID: id, SourceID: source.ID, Event: e}}, rules)
	if err != nil {
		t.Fatal(err)
	}
	msg := finalResponse(snap, true).Messages[0]
	o := AgentOption{ID: "finish", Label: "Close session and move task to done", Mode: "action", Actions: []inboxaction.Action{{ID: "finish", Kind: "complete_task", ProjectID: 1, TaskID: 1, SessionID: "owned", Status: "done"}}}
	state := r.sessions["owned"]
	state.Input++
	r.sessions["owned"] = state
	if _, err = s.optionSequence(ctx, snap, msg, o); !errors.Is(err, ErrActionBlocked) {
		t.Fatal("stale model context was rebased onto new user input", err)
	}
}
func TestLargeSourceBodiesDoNotCreateAnExecutionPrompt(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	s.DB.Exec(`UPDATE tasks SET recurrent=1 WHERE id=1`)
	q := directCard(t, s, source, inboxaction.Action{ID: "batch", Kind: "run_recurring_task", ProjectID: 1, TaskID: 1, Instructions: "Review the referenced PRs as one batch"})
	huge := event()
	huge.Context = strings.Repeat("historical discussion ", 20000)
	if _, err := s.DB.Exec(`UPDATE notification_events SET payload=?`, mustJSON(huge)); err != nil {
		t.Fatal(err)
	}
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(m.prompts) > 0 || len(r.submitted) != 1 {
		t.Fatal(out, len(m.prompts))
	}
}

func TestSequenceAdvancesItsOwnSessionInputGuard(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	r.sessions["existing"] = ActionSession{ID: "existing", TaskID: 1, Input: 1, Revision: 1}
	q := directCard(t, s, source, inboxaction.Action{ID: "first", Kind: "send_to_session", ProjectID: 1, TaskID: 1, SessionID: "existing", Instructions: "First approved instruction"}, inboxaction.Action{ID: "second", Kind: "send_to_session", ProjectID: 1, TaskID: 1, SessionID: "existing", Instructions: "Second approved instruction"})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 2 || len(m.prompts) > 0 {
		t.Fatal(out, r.submitted)
	}
}
func TestTaskStatusChangeWhileWaitingBlocksCompletionWithoutRebasing(t *testing.T) {
	s, _, r, tools, source := directFixture(t)
	r.sessions["owned"] = ActionSession{ID: "owned", TaskID: 1, Input: 0, Revision: 1}
	q := directCard(t, s, source, inboxaction.Action{ID: "first", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Inspect the feedback"}, inboxaction.Action{ID: "finish", Kind: "complete_task", ProjectID: 1, TaskID: 1, SessionID: "owned", Status: "done"})
	tools.before = func(name string) {
		if name == "list_sessions" {
			s.DB.Exec(`UPDATE tasks SET status='in_review' WHERE id=1`)
		}
	}
	out := runDirect(t, s, q)
	if !slices.Contains([]string{"blocked", "partial"}, out.DeliveryStatus) || len(r.stopped) > 0 || len(r.moved) > 0 {
		t.Fatal(out, r.stopped, r.moved)
	}
}

func TestBlockedHandoffOffersVisibleDirectRetryWithoutRepeatingStartup(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	r.readyErr = fmt.Errorf("%w: input is busy", ErrActionBlocked)
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the feedback"})
	out := runDirect(t, s, q)
	pending, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if err != nil || len(pending.Items) != 1 {
		t.Fatal(out, pending, err)
	}
	next := pending.Items[0]
	if next.Options[0].ID != "retry_remaining" {
		t.Fatal(next.Options)
	}
	r.readyErr = nil
	before := len(tools.calls)
	if _, err = s.Inbox.Answer(ctx, nil, next.ID, next.Revision, "", "retry_remaining"); err != nil {
		t.Fatal(err)
	}
	if err = s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Inbox.Get(ctx, next.ID)
	if current.DeliveryStatus != "succeeded" || len(m.prompts) > 0 || len(r.submitted) != 1 || len(tools.calls) != before+1 {
		t.Fatal(current, tools.calls, r.submitted)
	}
}
func TestUnknownWriteFollowupHasNoRetryOption(t *testing.T) {
	s, _, r, _, source := directFixture(t)
	r.submitErr = errors.New("lost acknowledgement")
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the feedback"})
	runDirect(t, s, q)
	pending, err := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	if err != nil || len(pending.Items) != 1 {
		t.Fatal(pending, err)
	}
	for _, o := range pending.Items[0].Options {
		if o.ID == "retry_remaining" {
			t.Fatal("unknown write was offered an automatic retry")
		}
	}
}
func TestExplicitBusyDeliveryOptionIsLabelled(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	e := event()
	id := seed(t, s, source, e)
	rules, _ := s.activeRuleSet(ctx)
	snap, err := s.agentSnapshot(ctx, []RawNotification{{ID: id, SourceID: source.ID, Event: e}}, rules)
	if err != nil {
		t.Fatal(err)
	}
	msg := finalResponse(snap, true).Messages[0]
	option := msg.Options[0]
	option.Actions[0].BusyPolicy = "send_now"
	out, err := s.optionSequence(ctx, snap, msg, option)
	if err != nil || !strings.Contains(out.Label, "send now, even if working") {
		t.Fatal(out, err)
	}
}

func TestFreeTextConfirmationSelectsSavedRetryAndPreservesCompletedSteps(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	r.readyErr = fmt.Errorf("%w: busy", ErrActionBlocked)
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	runDirect(t, s, q)
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	next := pending.Items[0]
	r.readyErr = nil
	m.responses = []any{map[string]any{"option_id": "retry_remaining", "actions": []inboxaction.Action{}, "question": "", "context": "", "authorization_quote": "Retry the remaining steps"}}
	if _, err := s.Inbox.Answer(ctx, nil, next.ID, next.Revision, "Retry the remaining steps", ""); err != nil {
		t.Fatal(err)
	}
	before := len(tools.calls)
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, next.ID)
	if out.DeliveryStatus != "succeeded" || len(m.prompts) != 1 || len(tools.calls) != before+1 || len(r.submitted) != 1 {
		t.Fatal(out, len(m.prompts), tools.calls, r.submitted)
	}
}
func TestInvalidResumeChainCannotBecomeAFreshRetry(t *testing.T) {
	s, _, r, _, source := directFixture(t)
	ctx := context.Background()
	r.readyErr = fmt.Errorf("%w: busy", ErrActionBlocked)
	q := directCard(t, s, source, inboxaction.Action{ID: "run", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check feedback"})
	runDirect(t, s, q)
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	next := pending.Items[0]
	// If the source run is subsequently recognized as uncertain, its saved retry
	// must be invalidated, never laundered into a fresh run with empty history.
	s.DB.Exec(`UPDATE notification_runs SET state='uncertain' WHERE id=?`, q.ID)
	if _, err := s.Inbox.Answer(ctx, nil, next.ID, next.Revision, "", "retry_remaining"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ = s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(pending.Items) != 1 {
		t.Fatal(pending)
	}
	for _, o := range pending.Items[0].Options {
		if o.ID == "retry_remaining" {
			t.Fatal("invalid resume became a fresh retry")
		}
	}
}

type changingReviewProvider struct {
	*fakeProvider
	s      *Service
	states map[int64]string
	onRead func(int64)
}

func (p *changingReviewProvider) ActionPR(ctx context.Context, source Source, e Event) (Event, error) {
	_, n, err := store.PRIdentity(e.URL, 0)
	if err != nil {
		return e, err
	}
	state := p.states[n]
	if state == "" {
		state = "open"
	}
	draft := false
	e.SubjectTitle = "Change"
	e.SubjectState = state
	e.PRFacts = &PRFacts{Available: true, State: state, Draft: &draft}
	if p.onRead != nil {
		p.onRead(n)
	}
	return e, nil
}
func TestRetryRefreshesPRStateAndNeverRepeatsDeliveredReviews(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	p := &changingReviewProvider{fakeProvider: &fakeProvider{}, s: s, states: map[int64]string{}}
	p.onRead = func(n int64) {
		if n == 8 {
			s.EnableSource(ctx, source.ID, false)
		}
	}
	s.Providers["github"] = p
	q := directCard(t, s, source, inboxaction.Action{ID: "reviews", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7, 8}})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "partial" || len(r.submitted) != 1 {
		t.Fatal(out, r.submitted)
	}
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	next := pending.Items[0]
	p.onRead = nil
	p.states[7] = "closed"
	p.states[8] = "closed"
	s.EnableSource(ctx, source.ID, true)
	if _, err := s.Inbox.Answer(ctx, nil, next.ID, next.Revision, "", "retry_remaining"); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	current, _ := s.Inbox.Get(ctx, next.ID)
	if current.DeliveryStatus != "partial" || len(r.submitted) != 1 || len(m.prompts) > 0 {
		t.Fatal(current, r.submitted)
	}
	opens := 0
	for _, name := range tools.calls {
		if name == "open_review" {
			opens++
		}
	}
	if opens != 1 {
		t.Fatal("closed PR was opened from stale cached verification", tools.calls)
	}
}

func TestReviewResumeProvidesAnInstructionRatherThanOnlyAURL(t *testing.T) {
	s, m, r, _, source := directFixture(t)
	s.DB.Exec(`UPDATE tasks SET source='github_pr',pr_number=7 WHERE id=1`)
	s.DB.Exec(`INSERT INTO notification_review_prompts(task_id,prompt,state) VALUES(1,'Original configured prompt','sent')`)
	r.sessions["existing"] = ActionSession{ID: "existing", TaskID: 1, Revision: 1}
	q := directCard(t, s, source, inboxaction.Action{ID: "review", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7}})
	out := runDirect(t, s, q)
	if out.DeliveryStatus != "succeeded" || len(r.submitted) != 1 || !strings.Contains(r.submitted[0], "Continue the PR review") || !strings.Contains(r.submitted[0], "https://github.com/acme/repo/pull/7") || len(m.prompts) > 0 {
		t.Fatal(out, r.submitted)
	}
}
func TestResumeUsesOnlyTheSelectedAgentConversation(t *testing.T) {
	for _, tc := range []struct {
		engine, idColumn string
		want             bool
	}{{"codex", "claude_session_id", false}, {"codex", "codex_session_id", true}, {"opencode", "claude_session_id", false}, {"opencode", "opencode_session_id", true}, {"custom", "claude_session_id", false}} {
		t.Run(tc.engine+"/"+tc.idColumn, func(t *testing.T) {
			s, _, _, tools, source := directFixture(t)
			var agent int64
			if tc.engine == "custom" {
				if err := s.DB.QueryRow(`INSERT INTO agents(name,command,engine) VALUES('Test custom','custom','custom') RETURNING id`).Scan(&agent); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := s.DB.QueryRow(`SELECT id FROM agents WHERE engine=?`, tc.engine).Scan(&agent); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := s.DB.Exec(`UPDATE projects SET agent_id=? WHERE id=1`, agent); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DB.Exec(`UPDATE tasks SET ` + tc.idColumn + `='old-conversation' WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			q := directCard(t, s, source, inboxaction.Action{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Inspect this change"})
			out := runDirect(t, s, q)
			if out.DeliveryStatus != "succeeded" {
				t.Fatal(out)
			}
			for i, name := range tools.calls {
				if name == "start_task_agent" {
					var args struct {
						Resume bool `json:"resume"`
					}
					if err := json.Unmarshal(tools.args[i], &args); err != nil {
						t.Fatal(err)
					}
					if args.Resume != tc.want {
						t.Fatal("wrong backend conversation selected", args.Resume, tc.want)
					}
				}
			}
		})
	}
}
func TestRecordedStepCannotBeReusedByAnotherOperation(t *testing.T) {
	s, _, r, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the change"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,result,started_at) VALUES(?,'continue/start','different','start_task_agent',1,'{}','succeeded','{"taskId":1,"id":"old"}',1)`, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	out, _ := s.Inbox.Get(ctx, q.ID)
	if out.DeliveryStatus != "uncertain" || len(r.submitted) > 0 {
		t.Fatal(out)
	}
}
func TestInterruptedActionNoticeSurvivesRestartAndIsDeliveredOnce(t *testing.T) {
	s, m, r, tools, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the change"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	s.DB.Exec(`UPDATE notification_runs SET state='running' WHERE id=?`, q.ID)
	if _, err := s.DB.Exec(`INSERT INTO notification_action_steps(run_id,step_key,operation_id,kind,is_mutation,arguments,state,started_at) VALUES(?,'continue/submit','continue','send_session_message',1,'{}','running',1)`, q.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	pending, err := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if err != nil || len(pending.Items) != 1 || pending.Items[0].AttentionSourceID != q.ID {
		t.Fatal(pending, err)
	}
	for _, o := range pending.Items[0].Options {
		if o.ID == "retry_remaining" {
			t.Fatal("uncertain write offered a retry")
		}
	}
	old, _ := s.Inbox.Get(ctx, q.ID)
	if _, err = s.Inbox.Dismiss(ctx, pending.Items[0].ID, pending.Items[0].Revision); err != nil {
		t.Fatal(err)
	}
	if err = s.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	now, _ := s.Inbox.Get(ctx, q.ID)
	pending, _ = s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(pending.Items) > 0 || now.ExecutionResult != old.ExecutionResult || len(m.prompts) > 0 || len(r.submitted) > 0 || len(tools.calls) > 0 {
		t.Fatal(pending, old, now, tools.calls)
	}
}
func TestFailureNoticeRetriesPublicationWithoutLosingOrDuplicatingIt(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the change"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if err := s.finishRun(ctx, q.ID, "failed", "A specific failure before notice publication", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`CREATE TRIGGER fail_attention BEFORE INSERT ON inbox_questions WHEN NEW.notification_group LIKE 'action-attention:%' BEGIN SELECT RAISE(ABORT,'simulated notice storage failure');END`); err != nil {
		t.Fatal(err)
	}
	if err := s.executeNext(ctx); err == nil {
		t.Fatal("notice failure was not reported")
	}
	var queued int
	s.DB.QueryRow(`SELECT attention_pending FROM notification_runs WHERE id=?`, q.ID).Scan(&queued)
	if queued != 1 {
		t.Fatal("notice was lost")
	}
	s.DB.Exec(`DROP TRIGGER fail_attention`)
	if err := s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.deliverActionAttention(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(pending.Items) != 1 || pending.Items[0].AttentionSourceID != q.ID || !strings.Contains(pending.Items[0].Context, "specific failure") {
		t.Fatal(pending)
	}
}
func TestAttentionDoesNotDisappearBehindAnExistingPendingReply(t *testing.T) {
	s, _, _, _, source := directFixture(t)
	ctx := context.Background()
	q := directCard(t, s, source, inboxaction.Action{ID: "continue", Kind: "continue_task", ProjectID: 1, TaskID: 1, Instructions: "Check the change"})
	if _, err := s.Inbox.Answer(ctx, nil, q.ID, q.Revision, "", "go"); err != nil {
		t.Fatal(err)
	}
	if err := s.finishRun(ctx, q.ID, "failed", "New failure that needs attention", nil); err != nil {
		t.Fatal(err)
	}
	existing, err := s.Inbox.PublishNotificationFollowup(ctx, q.ID, inbox.Create{Abstract: "Other question", Summary: "Other question", Context: "Preserve this question", Question: "Which part should be checked?"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.executeNext(ctx); err != nil {
		t.Fatal(err)
	}
	pending, _ := s.Inbox.ListWithOrder(ctx, nil, false, 0, true)
	if len(pending.Items) != 2 {
		t.Fatal(pending)
	}
	old, _ := s.Inbox.Get(ctx, existing.ID)
	if old.Question != existing.Question || old.Context != existing.Context {
		t.Fatal("existing question overwritten", old)
	}
}

func TestReviewReadinessNoticeExplainsRecoveryAndRetriesWithoutDuplicateTasks(t *testing.T) {
	s, model, r, tools, source := directFixture(t)
	r.readyErr = fmt.Errorf("%w: agent did not become ready within the action deadline; open its terminal", ErrActionBlocked)
	q := directCard(t, s, source, inboxaction.Action{ID: "reviews", Kind: "start_reviews", ProjectID: 1, PRNumbers: []int64{7, 8, 9}})
	out := runDirect(t, s, q)
	pending, err := s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	if err != nil || len(pending.Items) != 1 {
		t.Fatal(pending, err)
	}
	notice := pending.Items[0]
	if out.DeliveryStatus != "partial" || !strings.Contains(notice.Summary, "3 review prompts") || strings.Contains(notice.Summary, "partial") || !strings.Contains(notice.Context, "Open the linked task terminals") || !strings.Contains(notice.Context, "stopped before sending 3 review prompts") || len(r.submitted) != 0 || len(notice.LinkedTasks) != 3 {
		t.Fatal(out, notice, r.submitted)
	}
	retry := false
	dismiss := false
	for _, option := range notice.Options {
		if option.ID == "retry_remaining" {
			retry = option.Label == "Retry sending review prompts" && option.Action != nil && option.Action.Guards.ResumeRunID == q.ID
		}
		if option.ID == "ignore" {
			dismiss = option.Label == "Dismiss this notice" && strings.Contains(option.ActionSummary, "do not start or cancel work")
		}
	}
	if !retry || !dismiss {
		t.Fatal(notice.Options)
	}
	initialCreates := 0
	for _, name := range tools.calls {
		if name == "open_review" || name == "start_task_agent" {
			initialCreates++
		}
	}
	r.readyErr = nil
	if _, err = s.Inbox.Answer(context.Background(), nil, notice.ID, notice.Revision, "", "retry_remaining"); err != nil {
		t.Fatal(err)
	}
	if err = s.executeNext(context.Background()); err != nil {
		t.Fatal(err)
	}
	retried, err := s.Inbox.Get(context.Background(), notice.ID)
	finalCreates := 0
	for _, name := range tools.calls {
		if name == "open_review" || name == "start_task_agent" {
			finalCreates++
		}
	}
	if err != nil || retried.DeliveryStatus != "succeeded" || len(r.submitted) != 3 || finalCreates != initialCreates || len(model.prompts) != 0 {
		t.Fatal(retried, r.submitted, tools.calls, model.prompts, err)
	}
}
