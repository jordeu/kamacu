package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Task is the JSON shape of a task row (RESEARCH.md Pattern 4). position is
// included so the client can sort, but the client never writes it — ordering
// is owned by the server via the move endpoint.
type Task struct {
	ID          int64   `json:"id"`
	ProjectID   int64   `json:"project_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	Position    float64 `json:"position"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

var validStatuses = map[string]bool{
	"todo":        true,
	"in_progress": true,
	"in_review":   true,
	"done":        true,
}

type taskHandlers struct{ db *sql.DB }

const taskColumns = `id, project_id, title, description, status, position, created_at, updated_at`

func scanTask(row interface{ Scan(...any) error }) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.ProjectID, &t.Title, &t.Description, &t.Status, &t.Position, &t.CreatedAt, &t.UpdatedAt)
	return t, err
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
	rows, err := h.db.Query(`SELECT `+taskColumns+` FROM tasks WHERE project_id = ? ORDER BY status, position ASC`, pid)
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
	var exists int
	if err := h.db.QueryRow(`SELECT 1 FROM projects WHERE id = ?`, pid).Scan(&exists); err != nil {
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
		   (SELECT COALESCE(MIN(position), 2.0) - 1.0 FROM tasks WHERE project_id = ? AND status = 'todo'))
		 RETURNING `+taskColumns, pid, title, req.Description, pid))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
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
	var projectID int64
	if err := tx.QueryRowContext(ctx, `SELECT project_id FROM tasks WHERE id = ?`, id).Scan(&projectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
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
			 WHERE project_id = ? AND status = ? AND id != ?`,
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

	// 5. Persist and return the updated task.
	t, err := scanTask(tx.QueryRowContext(ctx,
		`UPDATE tasks SET status = ?, position = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
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
		 WHERE project_id = ? AND status = ? AND position > ? AND id != ?`,
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
		`SELECT id FROM tasks WHERE project_id = ? AND status = ? ORDER BY position, id`,
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

// delete handles DELETE /api/tasks/{id} — hard delete (D-11).
func (h *taskHandlers) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
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
