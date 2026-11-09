package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
)

// A frozen task-status guard cannot be repaired by retrying the same sequence.
// Describe this deterministically, including failures before a journaled step.
func (ex *actionExecutor) staleTaskAttention(ctx context.Context, run ActionRun) (*attentionCopy, *inbox.Option, error) {
	for id, original := range ex.seq.Guards.TaskStatus {
		needed, e := ex.taskGuardRequired(ctx, id)
		if e != nil {
			return nil, nil, e
		}
		if !needed {
			continue
		}
		var status, title string
		err := ex.s.DB.QueryRowContext(ctx, `SELECT status,title FROM tasks WHERE id=?`, id).Scan(&status, &title)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, nil, err
		}
		expected, e := ex.expectedStatus(ctx, id, status)
		if e != nil {
			return nil, nil, e
		}
		if err == nil && status == expected {
			continue
		}
		actual := status
		if errors.Is(err, sql.ErrNoRows) {
			actual = "deleted"
		}
		copy := attentionCopy{
			summary:  fmt.Sprintf("Task %d is %s; this saved action is outdated", id, actual),
			context:  fmt.Sprintf("The action targeted task %d (%s) when its status was %s. Its current status is %s. The safety check stopped the action before it could continue. Retrying the unchanged action would fail again; it will not reopen a completed task or switch tasks automatically.", id, title, original, actual),
			question: "Open the task to check what was handled, or dismiss this outdated request.",
		}
		ex.retrySafe = false
		// A replacement is a new explicit user choice, never an automatic retarget.
		if len(ex.seq.Actions) != 1 || ex.seq.Actions[0].Kind != "run_recurring_task" || status != "done" {
			return &copy, nil, nil
		}
		for _, step := range run.Steps {
			if step.Kind == "send_session_message" && (step.State == "succeeded" || step.State == "uncertain" || step.State == "running") {
				return &copy, nil, nil
			}
		}
		a := ex.seq.Actions[0]
		if strings.TrimSpace(a.TaskName) == "" {
			return &copy, nil, nil
		}
		var candidate int64
		rows, e := ex.s.DB.QueryContext(ctx, `SELECT id FROM tasks WHERE project_id=? AND recurrent=1 AND source='manual' AND status='todo' AND title=? AND id!=? ORDER BY id LIMIT 2`, a.ProjectID, a.TaskName, id)
		if e != nil {
			return nil, nil, e
		}
		count := 0
		for rows.Next() {
			if e = rows.Scan(&candidate); e != nil {
				rows.Close()
				return nil, nil, e
			}
			count++
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, nil, e
		}
		if count != 1 {
			return &copy, nil, nil
		}
		set, e := ex.s.activeRuleSet(ctx)
		if e != nil {
			if errors.Is(e, sql.ErrNoRows) {
				return &copy, nil, nil
			}
			return nil, nil, e
		}
		if set.ID != ex.seq.Guards.RuleSetID {
			return &copy, nil, nil
		}
		sources := map[string]Source{}
		for _, sid := range ex.seq.Guards.SourceIDs {
			sources[sid] = Source{ID: sid}
		}
		subjects := map[string]Event{}
		for _, url := range ex.seq.Guards.SourceURLs {
			subjects[url] = Event{URL: url, Repository: ex.seq.Guards.Projects[a.ProjectID]}
		}
		a.TaskID = candidate
		replacement, e := ex.s.prepareSequence(ctx, inboxaction.Sequence{Version: 1, Actions: []inboxaction.Action{a}}, set.ID, sources, subjects, true)
		if e != nil {
			return &copy, nil, nil
		}
		copy.context += fmt.Sprintf("\n\nA matching pending recurring task is %d (%s). No successful instruction delivery was recorded for this action. Check whether the PRs still need review before starting another run.", candidate, a.TaskName)
		copy.question = fmt.Sprintf("If these PRs still need review, run %s using pending task %d instead?", a.TaskName, candidate)
		label := fmt.Sprintf("Run %s on task %d", a.TaskName, candidate)
		if a.BusyPolicy == "send_now" {
			label += " (send now, even if working)"
		}
		option := inbox.Option{ID: "use_current_recurrence", Label: label, Action: &replacement, ActionSummary: replacement.Summary()}
		return &copy, &option, nil
	}
	return nil, nil, nil
}

func permanentActionGuard(err error) bool {
	if !errors.Is(err, ErrActionBlocked) {
		return false
	}
	for _, text := range []string{"task status changed", "moved from", "rules changed", "review prompt changed", "received new input", "no longer recurring", "another task", "target changed"} {
		if strings.Contains(err.Error(), text) {
			return true
		}
	}
	return false
}
