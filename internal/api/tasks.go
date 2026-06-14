package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"kangent/internal/session"
	"kangent/internal/settings"
	"kangent/internal/tmux"
	"kangent/internal/worktree"
)

// Task is the JSON shape of a task row (RESEARCH.md Pattern 4). position is
// included so the client can sort, but the client never writes it — ordering
// is owned by the server via the move endpoint.
//
// The three worktree fields are all nullable; UI state is derived from them
// (D-26): worktree_path set → active; worktree_error set → failed (Retry);
// both NULL → absent ("Create worktree"). No status enum.
type Task struct {
	ID            int64   `json:"id"`
	ProjectID     int64   `json:"project_id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Status        string  `json:"status"`
	Position      float64 `json:"position"`
	CreatedAt     string  `json:"created_at"`
	UpdatedAt     string  `json:"updated_at"`
	Branch        *string `json:"branch"`
	WorktreePath  *string `json:"worktree_path"`
	WorktreeError *string `json:"worktree_error"`
	// source discriminates a manual task ('manual') from a PR review
	// ('github_pr'); the board excludes github_pr rows (GHREV-04). pr_number
	// and pr_base_ref are NULL for manual tasks and carry the PR's number +
	// base branch name for a review (the frontend branches on source).
	Source    string  `json:"source"`
	PRNumber  *int64  `json:"pr_number"`
	PRBaseRef *string `json:"pr_base_ref"`
}

var validStatuses = map[string]bool{
	"todo":        true,
	"in_progress": true,
	"in_review":   true,
	"done":        true,
}

type taskHandlers struct {
	db         *sql.DB
	wt         *worktree.Service
	mgr        *session.Manager
	tmuxClient tmux.Client
}

const taskColumns = `id, project_id, title, description, status, position, created_at, updated_at, branch, worktree_path, worktree_error, source, pr_number, pr_base_ref`

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status, &t.Position, &t.CreatedAt, &t.UpdatedAt,
		&t.Branch, &t.WorktreePath, &t.WorktreeError, &t.Source, &t.PRNumber, &t.PRBaseRef)
	return t, err
}

// provisionWorktree creates the task's branch + worktree (GIT-01) and records
// the outcome on the tasks row. It NEVER returns an error to fail the HTTP
// request: git problems land in worktree_error (D-25) and the caller still
// responds normally. Git operations run under a 30s timeout; per Pitfall 9 no
// DB transaction spans them — only simple UPDATEs after they finish.
//
// On git success: UPDATE branch + worktree_path, clear worktree_error (the
// Retry path of POST /api/tasks/{id}/worktree reuses this helper and must
// erase a previously recorded failure). Submodule init is best-effort and
// never fails creation (RESEARCH §Submodule Handling).
// On git failure: UPDATE worktree_error only; returns the git error so the
// caller can populate the response without re-reading.
func provisionWorktree(ctx context.Context, db *sql.DB, wt *worktree.Service, taskID int64, title, repoPath string) (branch, path string, provErr error) {
	slug := worktree.Slug(title)

	// fail routes settings/expansion errors through the SAME D-25 failure
	// path as git errors: worktree_error records it, the HTTP request still
	// succeeds.
	fail := func(err error) (string, string, error) {
		if _, dbErr := db.ExecContext(ctx,
			`UPDATE tasks SET worktree_error = ? WHERE id = ?`, err.Error(), taskID); dbErr != nil {
			slog.Error("recording worktree error on task", "task", taskID, "error", dbErr)
		}
		return "", "", err
	}

	// Phase 6 (BRANCH-01, WT-01): branch name and base dir come from settings,
	// read from the DB at every creation (SET-03 — never cached). This is the
	// single choke point for BOTH creation paths (task create and the
	// POST /worktree Retry), so a template/base change applies to the very
	// next provisioning. The base is stored raw (may carry "~") and expanded
	// only at use (Pitfall 6); Create's MkdirAll makes a not-yet-existing base.
	tpl, err := settings.Get(db, settings.KeyBranchTemplate)
	if err != nil {
		return fail(err)
	}
	branch = settings.ExpandTemplate(tpl, slug, taskID, title)
	baseRaw, err := settings.Get(db, settings.KeyWorktreeBase)
	if err != nil {
		return fail(err)
	}
	baseDir, err := settings.ExpandHome(baseRaw)
	if err != nil {
		return fail(err)
	}
	path = worktree.PathUnder(baseDir, repoPath, slug, taskID)

	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	// Create-time defense (BRANCH-02 second half): an expanded name that
	// escaped save-time validation (e.g. a hand-edited settings row) must
	// never reach `git worktree add` — it lands in worktree_error below,
	// never a crashed create.
	err = settings.CheckRefFormat(wctx, branch)
	if err == nil {
		var base string
		base, err = wt.ResolveBase(wctx, repoPath)
		if err == nil {
			err = wt.Create(wctx, repoPath, branch, path, base)
		}
	}
	if err == nil {
		if subErr := wt.EnsureSubmodules(wctx, path); subErr != nil {
			slog.Warn("submodule init failed; worktree still usable", "path", path, "error", subErr)
		}
		if _, dbErr := db.ExecContext(ctx,
			`UPDATE tasks SET branch = ?, worktree_path = ?, worktree_error = NULL WHERE id = ?`,
			branch, path, taskID); dbErr != nil {
			slog.Error("recording worktree on task", "task", taskID, "error", dbErr)
		}
		return branch, path, nil
	}
	if _, dbErr := db.ExecContext(ctx,
		`UPDATE tasks SET worktree_error = ? WHERE id = ?`, err.Error(), taskID); dbErr != nil {
		slog.Error("recording worktree error on task", "task", taskID, "error", dbErr)
	}
	return "", "", err
}

// listByProject handles GET /api/projects/{id}/tasks — the board fetch.
func (h *taskHandlers) listByProject(w http.ResponseWriter, r *http.Request) {
	pid, ok := pathID(w, r)
	if !ok {
		return
	}
	var exists int
	if err := h.db.QueryRow(`SELECT 1 FROM projects WHERE id = ?`, pid).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "project not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	// GHREV-04: the board lists MANUAL tasks only — PR reviews
	// (source='github_pr') own a worktree/agent/diff but never render as cards.
	rows, err := h.db.Query(`SELECT `+taskColumns+` FROM tasks WHERE project_id = ? AND source = 'manual' ORDER BY status, position ASC`, pid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	tasks := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tasks)
}

// create handles POST /api/projects/{id}/tasks. Status is ALWAYS "todo" on
// create (D-08/D-09); position lands at the top of the To Do column.
func (h *taskHandlers) create(w http.ResponseWriter, r *http.Request) {
	pid, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	var repoPath string
	if err := h.db.QueryRow(`SELECT repo_path FROM projects WHERE id = ?`, pid).Scan(&repoPath); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "project not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	t, err := scanTask(h.db.QueryRow(
		`INSERT INTO tasks (project_id, title, description, status, position)
		 VALUES (?, ?, ?, 'todo',
		   (SELECT COALESCE(MIN(position), 2.0) - 1.0 FROM tasks WHERE project_id = ? AND status = 'todo' AND source = 'manual'))
		 RETURNING `+taskColumns, pid, title, req.Description, pid))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// GIT-01: provision synchronously (30s cap). ALWAYS 201 with the task —
	// git problems never block idea capture; failures land in worktree_error
	// for the task view's Retry affordance (D-25).
	branch, path, provErr := provisionWorktree(r.Context(), h.db, h.wt, t.ID, t.Title, repoPath)
	if provErr != nil {
		msg := provErr.Error()
		t.WorktreeError = &msg
	} else {
		t.Branch = &branch
		t.WorktreePath = &path
	}
	writeJSON(w, http.StatusCreated, t)
}

// get handles GET /api/tasks/{id} — deep-link fetch for the task view.
func (h *taskHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := scanTask(h.db.QueryRow(`SELECT `+taskColumns+` FROM tasks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// update handles PATCH /api/tasks/{id} — content edits only (title and/or
// description). Status and position changes go through /move exclusively.
func (h *taskHandlers) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	sets := []string{}
	args := []any{}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			writeError(w, http.StatusBadRequest, "title is required")
			return
		}
		sets = append(sets, "title = ?")
		args = append(args, title)
	}
	if req.Description != nil {
		// clearing the description (empty string) is legal
		sets = append(sets, "description = ?")
		args = append(args, *req.Description)
	}
	if len(sets) == 0 {
		// nothing to change — return the current row
		h.get(w, r)
		return
	}
	sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')")
	args = append(args, id)
	t, err := scanTask(h.db.QueryRow(
		`UPDATE tasks SET `+strings.Join(sets, ", ")+` WHERE id = ? RETURNING `+taskColumns, args...))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// move handles POST /api/tasks/{id}/move. The client only sends intent
// ({status, after_id}); the server computes the fractional position inside a
// single write transaction (RESEARCH.md Pattern 6) and renormalizes the
// column when float midpoints are exhausted.
func (h *taskHandlers) move(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Status  string `json:"status"`
		AfterID *int64 `json:"after_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil) // DSN _txlock=immediate → write lock up front
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()

	// 1. Load the moving task; validate target status.
	// GHREV-04 guard (defense-in-depth, D-13): a PR review (source != 'manual')
	// can never receive a board position. Reject it with 409 BEFORE any
	// position math, even though the position queries already filter PR rows.
	var projectID int64
	var source string
	if err := tx.QueryRowContext(ctx, `SELECT project_id, source FROM tasks WHERE id = ?`, id).Scan(&projectID, &source); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	if source != "manual" {
		writeError(w, http.StatusConflict, "PR reviews are not board tasks")
		return
	}
	if !validStatuses[req.Status] {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}

	var pos float64
	if req.AfterID == nil {
		// 2. Drop at top of the target column. Exclude the moving task so
		// re-dropping at the top of its own column works.
		err = tx.QueryRowContext(ctx,
			`SELECT COALESCE(MIN(position), 2.0) - 1.0 FROM tasks
			 WHERE project_id = ? AND status = ? AND id != ? AND source = 'manual'`,
			projectID, req.Status, id).Scan(&pos)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		// 3. Drop after a specific task: it must exist, be in the same
		// project, already have the target status, and not be the mover.
		afterPos, ok, err := h.afterPosition(ctx, tx, *req.AfterID, id, projectID, req.Status)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeError(w, http.StatusBadRequest, "invalid after_id")
			return
		}
		next, err := h.nextPosition(ctx, tx, projectID, req.Status, afterPos, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// 4. Renormalization guard: float64 midpoint exhaustion.
		if next != nil && *next-afterPos < 1e-9 {
			if err := renumberColumn(ctx, tx, projectID, req.Status); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			afterPos, ok, err = h.afterPosition(ctx, tx, *req.AfterID, id, projectID, req.Status)
			if err != nil || !ok {
				writeError(w, http.StatusInternalServerError, "renormalization lost after task")
				return
			}
			next, err = h.nextPosition(ctx, tx, projectID, req.Status, afterPos, id)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if next == nil {
			pos = afterPos + 1.0
		} else {
			pos = (afterPos + *next) / 2
		}
	}

	// 5. Persist and return the updated task. Stamp the entered status's *_at
	// column (D-90, last-entry-wins): entering Done sets done_at — the reaper's
	// clock (REAP-01) — while the other three are banked for future stats.
	// statusAtCol comes from a fixed map keyed by the ALREADY-validated
	// req.Status (never raw user input), so concatenating it into the SQL is
	// safe. Leaving a status never clears its column; the reaper relies on the
	// status='done' gate, not done_at, to cancel reaping.
	statusAtCol := map[string]string{
		"todo":        "todo_at",
		"in_progress": "in_progress_at",
		"in_review":   "in_review_at",
		"done":        "done_at",
	}[req.Status]
	t, err := scanTask(tx.QueryRowContext(ctx,
		`UPDATE tasks SET status = ?, position = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now'),
		   `+statusAtCol+` = strftime('%Y-%m-%dT%H:%M:%fZ','now')
		 WHERE id = ? RETURNING `+taskColumns, req.Status, pos, id))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// afterPosition returns the position of the after-task if it is a valid
// anchor for the move (same project, already in the target column, not the
// moving task itself). ok=false means the anchor is invalid → 400.
func (h *taskHandlers) afterPosition(ctx context.Context, tx *sql.Tx, afterID, movingID, projectID int64, status string) (float64, bool, error) {
	if afterID == movingID {
		return 0, false, nil
	}
	var afterProject int64
	var afterStatus string
	var afterPos float64
	err := tx.QueryRowContext(ctx,
		`SELECT project_id, status, position FROM tasks WHERE id = ?`, afterID).
		Scan(&afterProject, &afterStatus, &afterPos)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if afterProject != projectID || afterStatus != status {
		return 0, false, nil
	}
	return afterPos, true, nil
}

// nextPosition returns the smallest position strictly greater than afterPos
// in the target column, excluding the moving task; nil if dropping at bottom.
func (h *taskHandlers) nextPosition(ctx context.Context, tx *sql.Tx, projectID int64, status string, afterPos float64, movingID int64) (*float64, error) {
	var next sql.NullFloat64
	err := tx.QueryRowContext(ctx,
		`SELECT MIN(position) FROM tasks
		 WHERE project_id = ? AND status = ? AND position > ? AND id != ? AND source = 'manual'`,
		projectID, status, afterPos, movingID).Scan(&next)
	if err != nil {
		return nil, err
	}
	if !next.Valid {
		return nil, nil
	}
	v := next.Float64
	return &v, nil
}

// renumberColumn resets the positions of a whole column to 1.0, 2.0, 3.0...
// preserving the current order. Runs inside the caller's transaction.
func renumberColumn(ctx context.Context, tx *sql.Tx, projectID int64, status string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM tasks WHERE project_id = ? AND status = ? AND source = 'manual' ORDER BY position, id`,
		projectID, status)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`UPDATE tasks SET position = ? WHERE id = ?`, float64(i+1), id); err != nil {
			return err
		}
	}
	return nil
}

// delete handles DELETE /api/tasks/{id} — hard delete (D-11). The task's
// running sessions (agent AND bash — D-42's one consistent rule) are stopped
// BEFORE the row delete, mirroring the worktree cleanup gate's blocking
// posture: a deleted task must never leave a headless claude holding its
// worktree (research gap #1 / Pitfall 4). StopAllForTask blocks up to the 5s
// SIGTERM grace — accepted single-user latency, same as DELETE /worktree.
func (h *taskHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h.mgr.StopAllForTask(id)
	// Kill the task's live tmux sessions BEFORE deleting its rows (D-93).
	// StopAllForTask only reaches in-memory sessions; a detached tmux survivor
	// (no in-memory session) would otherwise be orphaned cwd'd inside the
	// worktree. Order is load-bearing: kill -> delete tmux_sessions rows ->
	// delete task row. tmux_sessions has NO ON DELETE CASCADE (migration 00005)
	// and SQLite FKs default off, so the rows are removed explicitly — kill is
	// the correct action (do not add cascade). KillSession is idempotent; a
	// tmux failure is warn-only and never blocks the delete (Pitfall 5).
	ctx := r.Context()
	if names, err := h.taskTmuxNames(ctx, id); err == nil {
		for _, name := range names {
			if err := h.tmuxClient.KillSession(ctx, name); err != nil {
				slog.Warn("killing tmux session before task delete", "task", id, "name", name, "error", err)
			}
		}
	} else {
		slog.Warn("listing tmux_sessions for task delete", "task", id, "error", err)
	}
	if _, err := h.db.ExecContext(ctx, `DELETE FROM tmux_sessions WHERE task_id = ?`, id); err != nil {
		slog.Warn("deleting tmux_sessions rows on task delete", "task", id, "error", err)
	}
	res, err := h.db.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// taskTmuxNames returns the persisted tmux session names of the task (all rows,
// alive or not — the kill is idempotent so a dead name is a harmless no-op).
func (h *taskHandlers) taskTmuxNames(ctx context.Context, taskID int64) ([]string, error) {
	rows, err := h.db.QueryContext(ctx, `SELECT name FROM tmux_sessions WHERE task_id = ?`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
