package api

import (
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

// move handles POST /api/tasks/{id}/move — implemented in plan Task 3.
func (h *taskHandlers) move(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not implemented")
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
