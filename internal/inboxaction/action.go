// Package inboxaction defines the bounded application operations an Inbox option
// can authorize. It deliberately contains no model or arbitrary tool execution.
package inboxaction

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

const Version = 1

type Target struct {
	ProjectID int64 `json:"project_id"`
	PRNumber  int64 `json:"pr_number"`
}
type SessionGuard struct {
	TaskID int64  `json:"task_id"`
	Input  uint64 `json:"input"`
}
type Guards struct {
	HandoffSessions map[int64]string        `json:"handoff_sessions,omitempty"`
	ResumeRunID     string                  `json:"resume_run_id,omitempty"`
	Projects        map[int64]string        `json:"projects"`
	RuleSetID       string                  `json:"rule_set_id"`
	SourceIDs       []string                `json:"source_ids"`
	SourceURLs      []string                `json:"source_urls"`
	ReviewSeed      string                  `json:"review_seed"`
	TaskStatus      map[int64]string        `json:"task_status"`
	Sessions        map[string]SessionGuard `json:"sessions"`
}
type Sequence struct {
	Version int      `json:"version"`
	Actions []Action `json:"actions"`
	Guards  Guards   `json:"guards"`
}
type Action struct {
	BusyPolicy   string  `json:"busy_policy"`
	ID           string  `json:"id"`
	Kind         string  `json:"kind"`
	ProjectID    int64   `json:"project_id"`
	TaskID       int64   `json:"task_id"`
	TaskRef      string  `json:"task_ref"`
	SessionID    string  `json:"session_id"`
	PRNumbers    []int64 `json:"pr_numbers"`
	TaskName     string  `json:"task_name"`
	Instructions string  `json:"instructions"`
	Status       string  `json:"status"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	LinkRole     string  `json:"link_role"`
}

func (s Sequence) Validate() error {
	if s.Version != Version || len(s.Actions) == 0 || len(s.Actions) > 10 {
		return errors.New("action sequence requires version 1 and 1–10 operations")
	}
	refs := map[string]bool{}
	seenIDs := map[string]bool{}
	for _, a := range s.Actions {
		if a.ID == "" || len(a.ID) > 60 || seenIDs[a.ID] || strings.ContainsFunc(a.ID, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
		}) {
			return errors.New("operations require unique IDs using letters, digits, underscores or hyphens")
		}
		if a.TaskID < 0 || a.ProjectID < 0 || len(a.PRNumbers) > 20 {
			return errors.New("invalid action identity or too many PRs")
		}
		if a.TaskRef != "" && (!refs[a.TaskRef] || a.TaskID != 0) {
			return errors.New("task_ref must reference an earlier task-producing operation")
		}
		prs := map[int64]bool{}
		for _, n := range a.PRNumbers {
			if n <= 0 || prs[n] {
				return errors.New("PR numbers must be positive and unique")
			}
			prs[n] = true
		}
		for _, v := range []string{a.Instructions, a.Title, a.Description, a.TaskName} {
			if strings.ContainsFunc(v, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }) {
				return errors.New("action text contains terminal control characters")
			}
		}
		if len(a.Instructions) > 32000 || len(a.Title) > 400 || len(a.TaskName) > 400 || len(a.Description) > 32000 {
			return errors.New("action text exceeds limits")
		}
		if !slices.Contains([]string{"", "wait_idle", "send_now"}, a.BusyPolicy) {
			return errors.New("unsupported busy-session policy")
		}
		if a.BusyPolicy != "" && !slices.Contains([]string{"start_reviews", "continue_task", "create_investigation", "run_recurring_task", "send_to_session"}, a.Kind) {
			return errors.New("busy-session policy only applies to handoff operations")
		}
		task := a.TaskID > 0 || a.TaskRef != ""
		switch a.Kind {
		case "start_reviews":
			if a.ProjectID <= 0 || len(a.PRNumbers) == 0 || task || a.SessionID != "" {
				return errors.New("start_reviews requires a project and explicit PR targets")
			}
		case "run_recurring_task", "continue_task":
			if a.ProjectID <= 0 || !task || strings.TrimSpace(a.Instructions) == "" || a.SessionID != "" {
				return errors.New("task handoff requires a project, task and saved instructions")
			}
		case "create_investigation":
			if a.ProjectID <= 0 || task || strings.TrimSpace(a.TaskName) == "" || strings.TrimSpace(a.Instructions) == "" || a.SessionID != "" {
				return errors.New("investigation requires a project, title and instructions")
			}
		case "send_to_session":
			if a.ProjectID <= 0 || !task || a.SessionID == "" || strings.TrimSpace(a.Instructions) == "" {
				return errors.New("session delivery requires a task, session and instructions")
			}
		case "close_session":
			if !task || a.ProjectID <= 0 || a.SessionID == "" {
				return errors.New("close_session requires an explicit task/session")
			}
		case "complete_task":
			if !task || a.ProjectID <= 0 || !slices.Contains([]string{"done", "in_review"}, a.Status) {
				return errors.New("completion requires a task and done/in_review destination")
			}
		case "update_task":
			if !task || a.ProjectID <= 0 || a.Title == "" && a.Description == "" {
				return errors.New("task update requires a task and an explicit title or description")
			}
		case "associate_pr":
			if !task || a.ProjectID <= 0 || len(a.PRNumbers) == 0 || !slices.Contains([]string{"implementation", "review", "incidental"}, a.LinkRole) {
				return errors.New("PR association requires a task, PRs and role")
			}
		case "ignore", "open_task":
			if a.Kind == "open_task" && (!task || a.ProjectID <= 0) {
				return errors.New("open_task requires a task")
			}
			if len(s.Actions) != 1 {
				return errors.New("navigation and ignore must be standalone actions")
			}
		default:
			return fmt.Errorf("unsupported Inbox action %q", a.Kind)
		}
		seenIDs[a.ID] = true
		refs[a.ID] = slices.Contains([]string{"create_investigation", "continue_task", "run_recurring_task", "send_to_session", "open_task", "update_task", "associate_pr"}, a.Kind)
	}
	return nil
}
func (s Sequence) Summary() string {
	labels := []string{}
	for _, a := range s.Actions {
		task := fmt.Sprintf("task %d", a.TaskID)
		if a.TaskRef != "" {
			task = "the task from " + a.TaskRef
		}
		switch a.Kind {
		case "start_reviews":
			labels = append(labels, fmt.Sprintf("Open/reuse %d PR review workspaces, start/resume their agents, and submit the configured review prompt", len(a.PRNumbers)))
		case "run_recurring_task":
			labels = append(labels, "Run recurring "+task+" and submit the saved batch instructions")
		case "continue_task":
			labels = append(labels, "Start/resume "+task+" and submit the saved instructions")
		case "create_investigation":
			labels = append(labels, "Create investigation “"+a.TaskName+"”, start its agent, and submit the saved instructions")
		case "send_to_session":
			labels = append(labels, "Submit saved instructions to "+task+"'s specified session")
		case "close_session":
			labels = append(labels, "Close the specified session of "+task)
		case "complete_task":
			labels = append(labels, "Close the specified session, if any, and move "+task+" to "+strings.ReplaceAll(a.Status, "_", " "))
		case "update_task":
			labels = append(labels, "Update the saved title/description of "+task)
		case "associate_pr":
			labels = append(labels, fmt.Sprintf("Associate %d PRs with %s as %s", len(a.PRNumbers), task, a.LinkRole))
		case "ignore":
			labels = append(labels, "Leave these updates alone; no work is started")
		case "open_task":
			labels = append(labels, "Open "+task+"; no work is started")
		}
	}
	summary := strings.Join(labels, " → ")
	if slices.ContainsFunc(s.Actions, func(a Action) bool { return a.BusyPolicy == "send_now" }) {
		summary += ". Explicitly submit while the agent is working; permission dialogs still require attention"
	}
	return summary
}
