package notifications

import (
	"context"
	"errors"
	"fmt"
	"kamacu/internal/inbox"
	"slices"
)

func (s *Service) addPlanBinding(ctx context.Context, plan *ActionPlan, id int64, role, source, evidence string) error {
	if !slices.Contains([]string{"implementation", "review", "recurring", "investigation"}, role) {
		return errors.New("unknown task binding role")
	}
	var b inbox.TaskLink
	b.TaskID = id
	b.Role = role
	b.Source = source
	b.Evidence = evidence
	var taskSource string
	var recurrent bool
	if err := s.DB.QueryRowContext(ctx, `SELECT project_id,title,status,source,recurrent FROM tasks WHERE id=?`, id).Scan(&b.ProjectID, &b.Title, &b.Status, &taskSource, &recurrent); err != nil {
		return err
	}
	if role == "review" && taskSource != "github_pr" {
		return errors.New("review requires a PR review task, not an implementation task")
	}
	if (role == "implementation" || role == "investigation") && taskSource == "github_pr" {
		return errors.New("implementation ownership cannot use a PR review task")
	}
	if role == "recurring" && !recurrent {
		return errors.New("recurring workflow requires a recurring task")
	}
	if b.ProjectID != plan.ProjectID {
		return errors.New("task binding belongs to another project")
	}
	for i, old := range plan.Bindings {
		if old.TaskID == id {
			if source != "user_reply" {
				b.Source = old.Source
				b.Evidence = old.Evidence
				b.Role = old.Role
			}
			plan.Bindings[i] = b
			return nil
		}
	}
	plan.Bindings = append(plan.Bindings, b)
	return nil
}
func (s *Service) setPlanSession(plan *ActionPlan) {
	plan.SessionID = ""
	switch {
	case plan.Workflow == "clarify":
		plan.SessionMode = "clarify"
	case plan.TaskID == 0:
		plan.SessionMode = "new"
	default:
		plan.SessionMode = "resume"
		if s.TaskHasSession != nil && s.TaskHasSession(plan.TaskID) {
			plan.SessionMode = "continue"
		}
		if s.TaskSession != nil {
			plan.SessionID = s.TaskSession(plan.TaskID)
			if plan.SessionID != "" {
				plan.SessionMode = "continue"
			}
		}
	}
}

func notificationActionLabel(plan ActionPlan) string {
	switch plan.Workflow {
	case "route_review":
		if len(plan.Targets) > 1 {
			return fmt.Sprintf("Review %d pull requests", len(plan.Targets))
		}
		if plan.TaskID > 0 {
			if plan.SessionMode == "continue" {
				return fmt.Sprintf("Continue review in task %d", plan.TaskID)
			}
			return fmt.Sprintf("Resume review in task %d", plan.TaskID)
		}
		return "Start review"
	case "route_implementation":
		if plan.SessionMode == "continue" {
			return fmt.Sprintf("Continue task %d", plan.TaskID)
		}
		return fmt.Sprintf("Resume task %d", plan.TaskID)
	case "route_recurring":
		return fmt.Sprintf("Run recurring task %d", plan.TaskID)
	case "clarify":
		return "Clarify the task or next step"
	default:
		if plan.TaskID > 0 {
			return fmt.Sprintf("Resume task %d", plan.TaskID)
		}
		return "Create investigation task"
	}
}

func (s *Service) validateOwnerAssociation(ctx context.Context, plan ActionPlan) error {
	if plan.Workflow != "route_implementation" || plan.TaskID == 0 {
		return nil
	}
	for _, b := range plan.Bindings {
		if b.TaskID == plan.TaskID && b.Source == "user_reply" {
			return nil
		}
	}
	for _, target := range plan.Targets {
		var rejected int
		if err := s.DB.QueryRowContext(ctx, `SELECT count(*) FROM task_pr_links l JOIN projects p ON p.id=? WHERE l.task_id=? AND lower(l.repository)=lower(p.github_repo) AND l.number=? AND l.role!='implementation'`, target.ProjectID, plan.TaskID, target.PRNumber).Scan(&rejected); err != nil {
			return err
		}
		if rejected > 0 {
			return errors.New("task ownership changed; clarify the task before continuing")
		}
	}
	return nil
}

func (s *Service) reviewTaskIDs(ctx context.Context, project, pr int64) ([]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM tasks WHERE project_id=? AND source='github_pr' AND pr_number=? ORDER BY id`, project, pr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Source ownership may span repositories/projects. Navigation preserves that
// proven relationship without changing the project of an explicit review or
// recurring action. Target bindings still use addPlanBinding's project check.
func (s *Service) addSourceBinding(ctx context.Context, plan *ActionPlan, id int64, role, source, evidence string) error {
	var project int64
	if err := s.DB.QueryRowContext(ctx, `SELECT project_id FROM tasks WHERE id=?`, id).Scan(&project); err != nil {
		return err
	}
	scoped := *plan
	scoped.ProjectID = project
	scoped.Bindings = append([]inbox.TaskLink(nil), plan.Bindings...)
	if err := s.addPlanBinding(ctx, &scoped, id, role, source, evidence); err != nil {
		return err
	}
	plan.Bindings = scoped.Bindings
	return nil
}
