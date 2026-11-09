package notifications

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"kamacu/internal/inbox"
	"kamacu/internal/inboxaction"
	"kamacu/internal/settings"
	"kamacu/internal/store"
)

// ActionRuntime supplies the same session and board primitives as the UI. The
// executor never asks a model whether a tool succeeded or what to call next.
type ActionSession struct {
	ID       string `json:"id"`
	TaskID   int64  `json:"task_id"`
	Input    uint64 `json:"input"`
	Revision uint64 `json:"revision"`
}
type ActionRuntime interface {
	Session(context.Context, string) (ActionSession, error)
	Ready(context.Context, string, string) (ActionSession, error)
	Submit(context.Context, ActionSession, string) error
	Stop(context.Context, ActionSession) error
	Move(context.Context, int64, string, string) error
}

type AgentOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// reply is only for genuine missing-information choices. All work/no-op
	// buttons must have an action and can never fall through to model execution.
	Mode    string               `json:"mode"`
	Actions []inboxaction.Action `json:"actions"`
}

func (s *Service) prepareSequence(ctx context.Context, seq inboxaction.Sequence, rules string, sources map[string]Source, subjects map[string]Event, sourceScoped bool) (inboxaction.Sequence, error) {
	if err := seq.Validate(); err != nil {
		return seq, err
	}
	if len(sources) == 0 && seq.Actions[0].Kind != "ignore" && seq.Actions[0].Kind != "open_task" {
		return seq, errors.New("notification work requires a bound source identity")
	}
	seq.Guards = inboxaction.Guards{RuleSetID: rules, TaskStatus: map[int64]string{}, Sessions: map[string]inboxaction.SessionGuard{}, Projects: map[int64]string{}}
	for id := range sources {
		seq.Guards.SourceIDs = append(seq.Guards.SourceIDs, id)
	}
	slices.Sort(seq.Guards.SourceIDs)
	targets := map[inboxaction.Target]bool{}
	sourceProjects := map[int64]bool{}
	for _, e := range subjects {
		seq.Guards.SourceURLs = append(seq.Guards.SourceURLs, e.URL)
		var project int64
		err := s.DB.QueryRowContext(ctx, `SELECT id FROM projects WHERE lower(github_repo)=lower(?)`, e.Repository).Scan(&project)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return seq, err
		}
		sourceProjects[project] = true
		if _, pr, err := store.PRIdentity(e.URL, 0); err == nil {
			targets[inboxaction.Target{ProjectID: project, PRNumber: pr}] = true
		}
	}
	slices.Sort(seq.Guards.SourceURLs)
	seq.Guards.SourceURLs = slices.Compact(seq.Guards.SourceURLs)
	refs := map[string]int64{}
	for _, a := range seq.Actions {
		if a.Kind == "ignore" {
			continue
		}
		var repo string
		if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(github_repo,'') FROM projects WHERE id=?`, a.ProjectID).Scan(&repo); err != nil {
			return seq, err
		}
		seq.Guards.Projects[a.ProjectID] = repo
		if a.TaskRef != "" && refs[a.TaskRef] != a.ProjectID {
			return seq, errors.New("task reference belongs to another project")
		}
		if a.Kind == "create_investigation" && sourceScoped && !sourceProjects[a.ProjectID] {
			return seq, errors.New("new investigation project is not one of the source projects")
		}
		if a.TaskID > 0 {
			var project int64
			var source, status string
			var recurring bool
			if err := s.DB.QueryRowContext(ctx, `SELECT project_id,source,status,recurrent FROM tasks WHERE id=?`, a.TaskID).Scan(&project, &source, &status, &recurring); err != nil {
				return seq, err
			}
			if project != a.ProjectID {
				return seq, errors.New("action task belongs to another project")
			}
			if a.Kind == "run_recurring_task" && !recurring {
				return seq, errors.New("action requires an existing recurring task")
			}
			if a.Kind == "continue_task" && source == "github_pr" {
				return seq, errors.New("PR reviews must use start_reviews and the registered review workflow")
			}
			if a.Kind == "complete_task" && source != "manual" {
				return seq, errors.New("board completion requires a manual task")
			}
			seq.Guards.TaskStatus[a.TaskID] = status
			if a.Kind == "continue_task" {
				p := ActionPlan{Workflow: "route_implementation", TaskID: a.TaskID, ProjectID: a.ProjectID}
				for t := range targets {
					p.Targets = append(p.Targets, ReviewTarget{t.ProjectID, t.PRNumber})
				}
				if err := s.validateOwnerAssociation(ctx, p); err != nil {
					return seq, err
				}
			}
		}
		if a.SessionID != "" {
			if s.Actions == nil {
				return seq, errors.New("session actions are unavailable")
			}
			sess, err := s.Actions.Session(ctx, a.SessionID)
			if err != nil {
				return seq, err
			}
			if sess.TaskID != a.TaskID || a.TaskRef != "" {
				return seq, errors.New("action session does not belong to its explicit task")
			}
			seq.Guards.Sessions[a.SessionID] = inboxaction.SessionGuard{TaskID: a.TaskID, Input: sess.Input}
		}
		if a.Kind == "complete_task" && a.SessionID == "" && a.TaskID > 0 && s.TaskSession != nil && s.TaskSession(a.TaskID) != "" {
			return seq, errors.New("completing a live task requires its explicit session ID")
		}
		if len(a.PRNumbers) > 0 {
			if repo == "" {
				return seq, errors.New("PR action requires a registered repository")
			}
			for _, n := range a.PRNumbers {
				if sourceScoped && !targets[inboxaction.Target{ProjectID: a.ProjectID, PRNumber: n}] {
					return seq, errors.New("action PR target is not one of this message's source PRs")
				}
			}
		}
		if a.Kind == "start_reviews" {
			var err error
			seq.Guards.ReviewSeed, err = settings.Get(s.DB, settings.KeyPRReviewSeed)
			if err != nil {
				return seq, err
			}
			if strings.TrimSpace(seq.Guards.ReviewSeed) == "" {
				return seq, errors.New("configure the PR review prompt in Settings before offering a review action")
			}
		}
		refs[a.ID] = a.ProjectID
	}
	for i := range seq.Actions {
		a := &seq.Actions[i]
		if !slices.Contains([]string{"continue_task", "run_recurring_task", "create_investigation"}, a.Kind) {
			continue
		}
		urls := seq.Guards.SourceURLs
		if len(a.PRNumbers) > 0 {
			var repo string
			if err := s.DB.QueryRowContext(ctx, `SELECT github_repo FROM projects WHERE id=?`, a.ProjectID).Scan(&repo); err != nil {
				return seq, err
			}
			urls = []string{}
			for _, n := range a.PRNumbers {
				urls = append(urls, fmt.Sprintf("https://github.com/%s/pull/%d", repo, n))
			}
		}
		missing := []string{}
		for _, url := range urls {
			if !strings.Contains(a.Instructions, url) {
				missing = append(missing, url)
			}
		}
		if len(missing) > 0 {
			a.Instructions += "\n\nSource references:\n" + strings.Join(missing, "\n")
		}
	}
	if err := seq.Validate(); err != nil {
		return seq, err
	}
	return seq, nil
}

func (s *Service) optionSequence(ctx context.Context, snapshot AgentSnapshot, m AgentMessage, o AgentOption) (inbox.Option, error) {
	if o.ID == "" || len(o.ID) > 100 || strings.TrimSpace(o.Label) == "" || len(o.Label) > 500 {
		return inbox.Option{}, errors.New("options require bounded IDs and labels")
	}
	if o.Mode == "reply" {
		if m.Plan.Workflow != "clarify" || len(o.Actions) > 0 {
			return inbox.Option{}, errors.New("only clarification choices can be conversational replies")
		}
		return inbox.Option{ID: o.ID, Label: o.Label}, nil
	}
	if o.Mode != "action" {
		return inbox.Option{}, errors.New("option mode must be action or reply")
	}
	sources := map[string]Source{}
	subjects := map[string]Event{}
	for _, sub := range snapshot.Subjects {
		if !slices.ContainsFunc(sub.EventIDs, func(id int64) bool { return slices.Contains(m.EventIDs, id) }) {
			continue
		}
		subjects[sub.ID] = sub.Event
		for _, raw := range sub.Notifications {
			sources[raw.SourceID] = Source{ID: raw.SourceID}
		}
	}
	seq, err := s.prepareSequence(ctx, inboxaction.Sequence{Version: inboxaction.Version, Actions: o.Actions}, snapshot.Rules.ID, sources, subjects, true)
	if err != nil {
		return inbox.Option{}, err
	}
	if err := freezeSequenceProjects(&seq, snapshot.Projects); err != nil {
		return inbox.Option{}, err
	}
	if err := freezeSequenceInputs(&seq, snapshot.Tasks); err != nil {
		return inbox.Option{}, err
	}
	// Review relevance still belongs to the model; task-role correctness does not.
	if m.Plan.WorkKind == "review" {
		for _, a := range seq.Actions {
			if slices.Contains([]string{"create_investigation", "continue_task"}, a.Kind) {
				return inbox.Option{}, errors.New("review work must use a PR review workspace or an explicit recurring task")
			}
		}
	}
	label := o.Label
	if slices.ContainsFunc(seq.Actions, func(a inboxaction.Action) bool { return a.BusyPolicy == "send_now" }) {
		label += " (send now, even if working)"
	}
	if len(label) > 500 {
		return inbox.Option{}, errors.New("busy-session option label exceeds limits")
	}
	return inbox.Option{ID: o.ID, Label: label, Action: &seq, ActionSummary: seq.Summary()}, nil
}

func actionLinks(ctx context.Context, s *Service, seq inboxaction.Sequence) ([]inbox.TaskLink, error) {
	out := []inbox.TaskLink{}
	for _, a := range seq.Actions {
		if a.TaskID == 0 {
			continue
		}
		var link inbox.TaskLink
		var source string
		var recurring bool
		link.TaskID = a.TaskID
		link.Source = "saved_action"
		link.Evidence = seq.Summary()
		if err := s.DB.QueryRowContext(ctx, `SELECT project_id,title,status,source,recurrent FROM tasks WHERE id=?`, a.TaskID).Scan(&link.ProjectID, &link.Title, &link.Status, &source, &recurring); err != nil {
			return nil, err
		}
		link.Role = "implementation"
		if recurring {
			link.Role = "recurring"
		}
		if source == "github_pr" {
			link.Role = "review"
		}
		if !slices.ContainsFunc(out, func(v inbox.TaskLink) bool { return v.TaskID == link.TaskID }) {
			out = append(out, link)
		}
	}
	return out, nil
}

// Never rebase a destructive option onto session input/task status that changed
// while its model response was being generated. The user's button approval is
// bound to the same observation that justified offering the operation.
func freezeSequenceInputs(seq *inboxaction.Sequence, tasks []AgentTask) error {
	for _, a := range seq.Actions {
		if a.TaskID == 0 {
			continue
		}
		for _, t := range tasks {
			if t.ID != a.TaskID {
				continue
			}
			if seq.Guards.TaskStatus[a.TaskID] != t.Status {
				return fmt.Errorf("%w: task changed during planning; refresh the context", ErrActionBlocked)
			}
			if a.SessionID != "" {
				expected := t.SessionState
				if expected == nil || expected.ID != a.SessionID {
					return fmt.Errorf("%w: session was not present in the frozen task context", ErrActionBlocked)
				}
				guard, ok := seq.Guards.Sessions[a.SessionID]
				if !ok || guard.Input != expected.Input {
					return fmt.Errorf("%w: session input changed during planning", ErrActionBlocked)
				}
			}
		}
	}
	return nil
}

func freezeSequenceProjects(seq *inboxaction.Sequence, projects []AgentProject) error {
	for id, repo := range seq.Guards.Projects {
		if !slices.ContainsFunc(projects, func(p AgentProject) bool { return p.ID == id && p.Repository == repo }) {
			return fmt.Errorf("%w: project repository changed during planning", ErrActionBlocked)
		}
	}
	return nil
}
