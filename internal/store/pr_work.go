package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var repoPattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`)

// PRIdentity accepts only canonical GitHub repository identities or full PR URLs.
func PRIdentity(repository string, number int64) (string, int64, error) {
	if strings.HasPrefix(repository, "https://") {
		u, e := url.Parse(repository)
		if e != nil || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return "", 0, errors.New("invalid PR URL")
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) != 4 || parts[2] != "pull" {
			return "", 0, errors.New("expected full GitHub PR URL")
		}
		n, e := strconv.ParseInt(parts[3], 10, 64)
		if e != nil || (number != 0 && number != n) {
			return "", 0, errors.New("PR number mismatch")
		}
		number = n
		repository = parts[0] + "/" + parts[1]
	}
	parts := strings.Split(repository, "/")
	if !repoPattern.MatchString(repository) || number <= 0 || len(parts) != 2 || parts[0] == "." || parts[0] == ".." || parts[1] == "." || parts[1] == ".." {
		return "", 0, errors.New("repository and positive PR number required")
	}
	return strings.ToLower(repository), number, nil
}

type PRLink struct {
	TaskID      int64     `json:"task_id"`
	Repository  string    `json:"repository"`
	Number      int64     `json:"number"`
	URL         string    `json:"url"`
	Role        string    `json:"role"`
	Source      string    `json:"source"`
	Evidence    string    `json:"evidence"`
	Uncertainty string    `json:"uncertainty"`
	ObservedAt  time.Time `json:"observed_at"`
	Corrected   bool      `json:"corrected"`
}

// User corrections are sticky, including incidental (a rejected association).
// Strict timestamp ordering prevents delayed classifiers from replacing newer evidence.
func PutPRLink(ctx context.Context, db *sql.DB, l PRLink) error {
	repo, n, err := PRIdentity(l.Repository, l.Number)
	if err != nil {
		return err
	}
	if l.TaskID <= 0 || (l.Role != "implementation" && l.Role != "review" && l.Role != "incidental") || l.Source == "" || strings.TrimSpace(l.Evidence) == "" || l.ObservedAt.IsZero() || len(l.Evidence) > 8000 || len(l.Uncertainty) > 2000 {
		return errors.New("invalid PR link evidence")
	}
	_, err = db.ExecContext(ctx, `INSERT INTO task_pr_links(task_id,repository,number,role,source,evidence,uncertainty,observed_at,corrected) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(task_id,repository,number) DO UPDATE SET role=excluded.role,source=excluded.source,evidence=excluded.evidence,uncertainty=excluded.uncertainty,observed_at=excluded.observed_at,corrected=excluded.corrected WHERE excluded.observed_at>task_pr_links.observed_at AND (task_pr_links.corrected=0 OR excluded.corrected=1)`, l.TaskID, repo, n, l.Role, l.Source, l.Evidence, l.Uncertainty, l.ObservedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), l.Corrected)
	return err
}
func PRLinks(ctx context.Context, db *sql.DB, taskID int64, repository string, number int64) ([]PRLink, error) {
	query := `SELECT task_id,repository,number,role,source,evidence,uncertainty,observed_at,corrected FROM task_pr_links WHERE `
	var args []any
	if taskID > 0 {
		query += "task_id=?"
		args = []any{taskID}
	} else {
		var err error
		repository, number, err = PRIdentity(repository, number)
		if err != nil {
			return nil, err
		}
		query += "repository=? AND number=?"
		args = []any{repository, number}
	}
	rows, err := db.QueryContext(ctx, query+" ORDER BY task_id,repository,number", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PRLink{}
	for rows.Next() {
		var l PRLink
		var at string
		if err = rows.Scan(&l.TaskID, &l.Repository, &l.Number, &l.Role, &l.Source, &l.Evidence, &l.Uncertainty, &at, &l.Corrected); err != nil {
			return nil, err
		}
		l.ObservedAt, _ = time.Parse(time.RFC3339Nano, at)
		l.URL = fmt.Sprintf("https://github.com/%s/pull/%d", l.Repository, l.Number)
		out = append(out, l)
	}
	return out, rows.Err()
}

type SessionWork struct {
	StateObservedAt time.Time `json:"state_observed_at"`
	StateStale      bool      `json:"state_stale"`
	Source          string    `json:"source"`
	SessionID       string    `json:"session_id"`
	TaskID          *int64    `json:"task_id"`
	State           string    `json:"state"`
	Summary         string    `json:"summary"`
	Evidence        string    `json:"evidence"`
	Uncertainty     string    `json:"uncertainty"`
	ObservedAt      time.Time `json:"observed_at"`
	Stale           bool      `json:"stale"`
}

const workTimeFormat = "2006-01-02T15:04:05.000000000Z"
const sessionWorkColumns = "session_id,task_id,state,summary,evidence,uncertainty,observed_at,state_observed_at"

func PutSessionWork(ctx context.Context, db *sql.DB, w SessionWork) error {
	if w.SessionID == "" || w.ObservedAt.IsZero() || len(w.Summary) > 4000 || len(w.Evidence) > 8000 || len(w.Uncertainty) > 2000 || strings.TrimSpace(w.Evidence) == "" {
		return errors.New("invalid session work")
	}
	// Summary and lifecycle updates have independent clocks. Old summary evidence
	// must never roll back a more recent idle/exit observation.
	_, err := db.ExecContext(ctx, `INSERT INTO session_work(session_id,task_id,state,summary,evidence,uncertainty,observed_at,state_observed_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(session_id) DO UPDATE SET summary=excluded.summary,evidence=excluded.evidence,uncertainty=excluded.uncertainty,observed_at=excluded.observed_at WHERE excluded.observed_at>session_work.observed_at`, w.SessionID, w.TaskID, w.State, w.Summary, w.Evidence, w.Uncertainty, w.ObservedAt.UTC().Format(workTimeFormat), w.ObservedAt.UTC().Format(workTimeFormat))
	return err
}
func PutSessionState(ctx context.Context, db *sql.DB, id string, taskID *int64, state string, at time.Time) error {
	if id == "" || at.IsZero() {
		return errors.New("session identity and observation time required")
	}
	switch state {
	case "working", "waiting", "idle", "error", "unknown", "exited":
	default:
		return errors.New("invalid session state")
	}
	_, err := db.ExecContext(ctx, `INSERT INTO session_work(session_id,task_id,state,summary,evidence,uncertainty,observed_at,state_observed_at) VALUES(?,?,?,'','','','',?) ON CONFLICT(session_id) DO UPDATE SET state=excluded.state,state_observed_at=excluded.state_observed_at WHERE excluded.state_observed_at>session_work.state_observed_at AND session_work.state!='exited'`, id, taskID, state, at.UTC().Format(workTimeFormat))
	return err
}
func scanSessionWork(row interface{ Scan(...any) error }) (SessionWork, error) {
	w := SessionWork{Source: "terminal_monitor"}
	var at, stateAt string
	err := row.Scan(&w.SessionID, &w.TaskID, &w.State, &w.Summary, &w.Evidence, &w.Uncertainty, &at, &stateAt)
	w.ObservedAt, _ = time.Parse(time.RFC3339Nano, at)
	w.StateObservedAt, _ = time.Parse(time.RFC3339Nano, stateAt)
	w.Stale = w.ObservedAt.IsZero() || time.Since(w.ObservedAt) > 5*time.Minute
	w.StateStale = w.State != "exited" && (w.StateObservedAt.IsZero() || time.Since(w.StateObservedAt) > 5*time.Minute)
	return w, err
}
func WorkForTask(ctx context.Context, db *sql.DB, taskID int64) ([]SessionWork, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+sessionWorkColumns+` FROM session_work WHERE task_id=? ORDER BY state_observed_at DESC LIMIT 20`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionWork{}
	for rows.Next() {
		w, e := scanSessionWork(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// ReconcileSessionStates runs in the existing monitor scheduler. Missing process
// identities after a restart are unknown, while observed exits are definitive.
func ReconcileSessionStates(ctx context.Context, db *sql.DB, processes map[string]bool, at time.Time) error {
	rows, err := db.QueryContext(ctx, `SELECT session_id,state FROM session_work WHERE state!='exited'`)
	if err != nil {
		return err
	}
	type change struct{ id, state string }
	changes := []change{}
	for rows.Next() {
		var id, state string
		if err = rows.Scan(&id, &state); err != nil {
			rows.Close()
			return err
		}
		running, exists := processes[id]
		next := "unknown"
		if exists {
			if running {
				continue
			}
			next = "exited"
		}
		if state != next {
			changes = append(changes, change{id, next})
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range changes {
		if err = PutSessionState(ctx, db, c.id, nil, c.state, at); err != nil {
			return err
		}
	}
	return nil
}

type RelatedTask struct {
	Link     PRLink        `json:"link"`
	Title    string        `json:"title"`
	Status   string        `json:"status"`
	Sessions []SessionWork `json:"sessions"`
}

func RelatedPRWork(ctx context.Context, db *sql.DB, repo string, n int64) ([]RelatedTask, error) {
	links, err := PRLinks(ctx, db, 0, repo, n)
	if err != nil {
		return nil, err
	}
	out := []RelatedTask{}
	for _, l := range links {
		if l.Role == "incidental" {
			continue
		}
		r := RelatedTask{Link: l}
		if err = db.QueryRowContext(ctx, `SELECT title,status FROM tasks WHERE id=?`, l.TaskID).Scan(&r.Title, &r.Status); err != nil {
			return nil, err
		}
		r.Sessions, err = WorkForTask(ctx, db, l.TaskID)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// Session links also cover Scratchpad and agents without a task. Task corrections
// are applied when reading, so rejected task associations cannot leak back in.
func PutSessionPRLink(ctx context.Context, db *sql.DB, sessionID string, l PRLink) error {
	repo, n, err := PRIdentity(l.Repository, l.Number)
	if err != nil {
		return err
	}
	if sessionID == "" || (l.Role != "review" && l.Role != "implementation") || l.Evidence == "" || l.ObservedAt.IsZero() {
		return errors.New("invalid session PR link")
	}
	_, err = db.ExecContext(ctx, `INSERT INTO session_pr_links(session_id,repository,number,role,source,evidence,uncertainty,observed_at,task_id) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(session_id,repository,number) DO UPDATE SET role=excluded.role,source=excluded.source,evidence=excluded.evidence,uncertainty=excluded.uncertainty,observed_at=excluded.observed_at,task_id=excluded.task_id WHERE excluded.observed_at>session_pr_links.observed_at`, sessionID, repo, n, l.Role, l.Source, l.Evidence, l.Uncertainty, l.ObservedAt.UTC().Format(workTimeFormat), nullableTaskID(l.TaskID))
	return err
}
func nullableTaskID(id int64) any {
	if id > 0 {
		return id
	}
	return nil
}
func RelatedPRSessions(ctx context.Context, db *sql.DB, repo string, n int64) ([]SessionWork, error) {
	repo, n, err := PRIdentity(repo, n)
	if err != nil {
		return nil, err
	}
	// The association is the driving row. A summary is optional, including for
	// links persisted before lifecycle tracking was introduced.
	rows, err := db.QueryContext(ctx, `SELECT l.session_id,COALESCE(l.task_id,w.task_id),COALESCE(w.state,'unknown'),COALESCE(w.summary,''),COALESCE(NULLIF(w.evidence,''),l.evidence),COALESCE(w.uncertainty,l.uncertainty),COALESCE(NULLIF(w.observed_at,''),l.observed_at),COALESCE(w.state_observed_at,'') FROM session_pr_links l LEFT JOIN session_work w ON w.session_id=l.session_id WHERE l.repository=? AND l.number=? AND NOT EXISTS(SELECT 1 FROM task_pr_links t WHERE t.task_id=COALESCE(l.task_id,w.task_id) AND t.repository=l.repository AND t.number=l.number AND t.role='incidental') ORDER BY COALESCE(w.state_observed_at,l.observed_at) DESC LIMIT 20`, repo, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionWork{}
	for rows.Next() {
		w, e := scanSessionWork(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func WorkForSession(ctx context.Context, db *sql.DB, id string) (*SessionWork, error) {
	w, err := scanSessionWork(db.QueryRowContext(ctx, `SELECT `+sessionWorkColumns+` FROM session_work WHERE session_id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &w, nil
}
func SessionPRLinks(ctx context.Context, db *sql.DB, id string) ([]PRLink, error) {
	rows, err := db.QueryContext(ctx, `SELECT l.repository,l.number,CASE WHEN t.corrected=1 THEN t.role ELSE l.role END,l.source,l.evidence,l.uncertainty,l.observed_at FROM session_pr_links l LEFT JOIN session_work w ON w.session_id=l.session_id LEFT JOIN task_pr_links t ON t.task_id=COALESCE(l.task_id,w.task_id) AND t.repository=l.repository AND t.number=l.number WHERE l.session_id=? AND (t.role IS NULL OR t.role!='incidental') ORDER BY l.repository,l.number`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PRLink{}
	for rows.Next() {
		var l PRLink
		var at string
		if err = rows.Scan(&l.Repository, &l.Number, &l.Role, &l.Source, &l.Evidence, &l.Uncertainty, &at); err != nil {
			return nil, err
		}
		l.ObservedAt, _ = time.Parse(time.RFC3339Nano, at)
		l.URL = fmt.Sprintf("https://github.com/%s/pull/%d", l.Repository, l.Number)
		out = append(out, l)
	}
	return out, rows.Err()
}
