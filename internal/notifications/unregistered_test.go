package notifications

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
	"kamacu/internal/store"
)

func TestUnregisteredReviewReachesInboxWithCrossProjectTask(t *testing.T) {
	f := newActivityFixture(t)
	// The PR's repository is deliberately absent from projects. The owning task
	// remains in another registered repository and references this PR explicitly.
	if _, err := f.s.DB.Exec(`UPDATE projects SET github_repo='acme/infrastructure' WHERE id=1; UPDATE tasks SET description='Depends on https://github.com/acme/repo/pull/7' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	f.addReview()
	f.reviews[0]["state"] = "CHANGES_REQUESTED"
	f.reviews[0]["body"] = "Please fix the validation before merging"
	f.event.Reason = "assign"
	f.model.respond = func(prompt string) (any, error) {
		snapshot := inputSnapshot(t, prompt)
		sub := snapshot.Subjects[0]
		if sub.RepositoryRegistered || sub.ProjectID != 0 || !strings.Contains(mustJSON(sub), "Please fix the validation") {
			t.Fatalf("missing unregistered review context: %+v", sub)
		}
		if len(snapshot.Tasks) != 2 || !strings.Contains(snapshot.Tasks[0].Description, f.event.URL) {
			t.Fatalf("cross-project open tasks missing: %+v", snapshot.Tasks)
		}
		turn := finalResponse(snapshot, true)
		for _, ref := range sub.Event.ActivityDelta.Activities {
			turn.Decisions[0].ActivityRefs = append(turn.Decisions[0].ActivityRefs, ref.Ref)
		}
		m := &turn.Messages[0]
		m.Plan = AgentPlan{Workflow: "clarify", WorkKind: "investigation"}
		m.Question = "Register this repository as a project to address the review?"
		m.LinkedTaskIDs = []int64{1}
		m.Options = nil
		return turn, nil
	}
	// Exercise the real poller and ingestion before running the processing agent.
	run := f.github.Run
	f.github.Run = func(ctx context.Context, args ...string) ([]byte, error) {
		if strings.HasPrefix(args[len(args)-1], "notifications?") {
			return response(mustJSON([]any{map[string]any{
				"id": f.event.ExternalID, "updated_at": f.event.Version, "reason": "assign",
				"repository": map[string]any{"full_name": f.event.Repository},
				"subject":    map[string]any{"type": "PullRequest", "title": f.event.Title, "url": f.event.SubjectURL},
			}}), ""), nil
		}
		return run(ctx, args...)
	}
	f.source.Checkpoint = f.now.Add(-time.Hour).Format(time.RFC3339)
	batch, err := f.github.Collect(context.Background(), f.source, nil)
	if err != nil || len(batch.Events) != 1 {
		t.Fatalf("unregistered notification was not collected: %+v %v", batch, err)
	}
	if err := f.s.ingest(context.Background(), f.source, batch); err != nil {
		t.Fatal(err)
	}
	f.s.Providers["github"] = f.github
	if p := processAgent(t, f.s); p.Status != "completed" || p.ActionCount != 1 {
		t.Fatalf("processing: %+v", p)
	}
	cards, err := f.s.Inbox.ListWithOrder(context.Background(), nil, false, 0, true)
	if err != nil || len(cards.Items) != 1 {
		t.Fatalf("missing inbox message: %+v %v", cards, err)
	}
	var links int
	if err := f.s.DB.QueryRow(`SELECT count(*) FROM notification_task_links WHERE question_id=? AND task_id=1`, cards.Items[0].ID).Scan(&links); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatal("cross-project task link was lost")
	}
}

func TestAgentReadAllowsUnregisteredGitHubContextOnly(t *testing.T) {
	s, _, tools, _ := fixture(t)
	tools.result = `{"reviews":[{"body":"Please fix validation"}]}`
	snapshot := AgentSnapshot{Subjects: []AgentSubject{{Event: Event{Repository: "acme/unregistered"}}}}
	raw, err := s.agentRead(context.Background(), snapshot, AgentRead{Name: "get_pr_context", Arguments: `{"repository":"acme/unregistered","number":7}`}, Config{ContextBytes: 64000})
	if err != nil || !strings.Contains(raw, "Please fix validation") || len(tools.calls) != 1 {
		t.Fatalf("read failed: %q %v %v", raw, err, tools.calls)
	}
	for _, tool := range []string{"open_review", "create_task", "start_task_agent"} {
		if _, err := s.agentRead(context.Background(), snapshot, AgentRead{Name: tool, Arguments: `{"repository":"acme/unregistered"}`}, Config{ContextBytes: 64000}); err == nil {
			t.Fatalf("mutation %s accepted", tool)
		}
	}
	if len(tools.calls) != 1 {
		t.Fatal("mutation reached tool backend")
	}
}

func TestUnregisteredSourceCannotStartReviewsOrCreateTasks(t *testing.T) {
	s, _, _, source := fixture(t)
	e := event()
	e.Repository = "acme/unregistered"
	e.URL = "https://github.com/acme/unregistered/pull/7"
	subjects := map[string]Event{"S1": e}
	// Also reject using an unrelated registered project as a substitute.
	for _, project := range []int64{0, 1} {
		for _, kind := range []string{"start_reviews", "create_investigation"} {
			a := inboxaction.Action{ID: "work", Kind: kind, ProjectID: project, TaskName: "Investigate review", Instructions: "Investigate " + e.URL}
			if kind == "start_reviews" {
				a.PRNumbers = []int64{7}
			}
			_, err := s.prepareSequence(context.Background(), inboxaction.Sequence{Version: inboxaction.Version, Actions: []inboxaction.Action{a}}, "", map[string]Source{source.ID: source}, subjects, true)
			if err == nil {
				t.Fatalf("%s accepted for project %d", kind, project)
			}
			if !strings.Contains(err.Error(), "register this repository as a project") {
				t.Fatalf("unhelpful refusal: %v", err)
			}
		}
	}
	snapshot := AgentSnapshot{Subjects: []AgentSubject{{EventIDs: []int64{1}, Event: e}}}
	for _, workflow := range []string{"route_review", "route_investigation"} {
		_, _, _, err := s.agentActions(context.Background(), snapshot, AgentTurn{Messages: []AgentMessage{{EventIDs: []int64{1}, Plan: AgentPlan{Workflow: workflow}}}})
		if err == nil || !strings.Contains(err.Error(), "register this repository as a project") {
			t.Fatalf("unhelpful plan refusal: %v", err)
		}
	}
}

func TestCompactSubjectKeepsRepositoryRegistrationFlag(t *testing.T) {
	snapshot := AgentSnapshot{Subjects: []AgentSubject{{ID: "S1", Event: Event{Repository: "acme/unregistered", Context: strings.Repeat("context ", 5000)}}}}
	raw, err := snapshotView(snapshot, 20000)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Subjects []map[string]json.RawMessage `json:"subjects"`
	}
	if err := json.Unmarshal([]byte(raw), &view); err != nil {
		t.Fatal(err)
	}
	if string(view.Subjects[0]["repository_registered"]) != "false" {
		t.Fatalf("registration flag omitted from compact context: %s", raw)
	}
}

func TestLargeCrossProjectTaskIndexDoesNotBlockSourceEvidence(t *testing.T) {
	s, _, _, _ := fixture(t)
	snapshot := AgentSnapshot{Subjects: []AgentSubject{{ID: "S1", Event: Event{Repository: "acme/unregistered", ActivityDelta: &store.NotificationDelta{Complete: true, Activities: []store.NotificationActivityRef{{Ref: "A1/V1"}}}, Evidence: []SubjectEvidence{{ID: "S1/E1", ActivityRef: "A1/V1", Current: true, Text: "Please fix validation", Complete: true}}}}}}
	for i := int64(1); i <= 300; i++ {
		snapshot.Tasks = append(snapshot.Tasks, AgentTask{ID: i, ReadID: "T" + formatID(i), ProjectID: i, Title: strings.Repeat("Candidate task ", 20), Description: "Full description for task " + formatID(i)})
	}
	raw, err := snapshotView(snapshot, 64000)
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Tasks []AgentTask `json:"tasks"`
		Next  *int        `json:"tasks_next_offset"`
		Count int         `json:"task_count"`
	}
	if err := json.Unmarshal([]byte(raw), &index); err != nil {
		t.Fatal(err)
	}
	if index.Next == nil || *index.Next != len(index.Tasks) || index.Count != len(snapshot.Tasks) || !strings.Contains(raw, "Please fix validation") {
		t.Fatalf("missing discovery cursor or source evidence: %s", raw)
	}
	seen := len(index.Tasks)
	offset := *index.Next
	for offset >= 0 {
		pageRaw, err := s.agentRead(context.Background(), snapshot, AgentRead{Name: "read_tasks", Arguments: mustJSON(map[string]any{"offset": offset})}, Config{ContextBytes: 64000})
		if err != nil {
			t.Fatal(err)
		}
		var page struct {
			Tasks []AgentTask `json:"tasks"`
			Next  int         `json:"next_offset"`
			Count int         `json:"task_count"`
		}
		if err := json.Unmarshal([]byte(pageRaw), &page); err != nil {
			t.Fatal(err)
		}
		for _, task := range page.Tasks {
			if task.ID != int64(seen+1) || task.ReadID == "" || !task.DetailAvailable || task.Description != "" {
				t.Fatalf("bad frozen index entry: %+v", task)
			}
			seen++
		}
		if page.Count != len(snapshot.Tasks) || (page.Next >= 0 && page.Next <= offset) {
			t.Fatalf("invalid index cursor: %s", pageRaw)
		}
		offset = page.Next
	}
	if seen != len(snapshot.Tasks) {
		t.Fatalf("lost tasks: %d", seen)
	}
	detail, err := s.agentRead(context.Background(), snapshot, AgentRead{Name: "read_task", Arguments: `{"id":"T300","offset":0}`}, Config{ContextBytes: 64000, MaxReads: 10})
	if err != nil || !strings.Contains(detail, "Full description for task 300") {
		t.Fatalf("missing frozen detail: %s %v", detail, err)
	}
	for _, offset := range []int{-1, 301} {
		if _, err := s.agentRead(context.Background(), snapshot, AgentRead{Name: "read_tasks", Arguments: mustJSON(map[string]any{"offset": offset})}, Config{ContextBytes: 64000}); err == nil {
			t.Fatalf("accepted invalid cursor %d", offset)
		}
	}
}

func TestRegistrationReplyDoesNotUseFallbackIgnoreAsPrimaryAction(t *testing.T) {
	s, model, tools, source := fixture(t)
	e := event()
	e.Repository = "acme/unregistered"
	e.URL = "https://github.com/acme/unregistered/pull/7"
	id := seed(t, s, source, e)
	request := ask()
	request.Question = "Register this repository as a project to address the review?"
	ignore := inboxaction.Sequence{Version: inboxaction.Version, Actions: []inboxaction.Action{{ID: "ignore", Kind: "ignore"}}}
	request.Options = []inbox.Option{{ID: "triage_noop", Label: "Leave these updates alone", Action: &ignore}}
	q, err := s.Inbox.PublishNotification(context.Background(), id, "GitHub", e.URL, request)
	if err != nil {
		t.Fatal(err)
	}
	q.Answer = "yes"
	model.respond = func(prompt string) (any, error) {
		_, raw, ok := strings.Cut(prompt, "INPUT:\n")
		if !ok {
			t.Fatal("missing reply input")
		}
		var input map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &input); err != nil {
			t.Fatal(err)
		}
		if _, ok := input["primary_saved_option"]; ok {
			t.Fatal("fallback ignore was selected as confirmation target")
		}
		return map[string]any{"actions": []inboxaction.Action{}, "summary": "Repository registration needs a project", "question": "Add acme/unregistered in Projects, then return here, or name an existing task?", "context": e.URL, "explanation": "This repository is not registered. Add it in Projects before starting a review or creating a task.", "authorization_quote": ""}, nil
	}
	seq, clarification, err := s.planActionReply(context.Background(), q)
	if err != nil || clarification == nil || len(seq.Actions) != 0 || len(tools.calls) != 0 {
		t.Fatalf("registration reply dismissed or started work: %+v %+v %v", seq, clarification, err)
	}
	var projects int
	if err := s.DB.QueryRow(`SELECT count(*) FROM projects`).Scan(&projects); err != nil || projects != 2 {
		t.Fatalf("automatic registration: %d %v", projects, err)
	}
}
