package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Use a fixed-width UTC representation so indexed deadline comparisons are chronological.
const snoozeTimeFormat = "2006-01-02T15:04:05.000Z"

func (h *taskHandlers) snooze(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Until time.Time `json:"until"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.Until.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "choose a future snooze time")
		return
	}
	ctx := r.Context()
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	t, err := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id=?`, id))
	if err != nil {
		snoozeError(w, err)
		return
	}
	if t.Source != "manual" || (t.Status != "in_progress" && t.Status != "in_review") {
		writeError(w, http.StatusConflict, "only In Progress and In Review tasks can be snoozed")
		return
	}
	var pos float64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=? AND status='todo' AND source='manual'`, t.ProjectID).Scan(&pos); err != nil {
		snoozeError(w, err)
		return
	}
	// Move first; then save the origin and deadline in the same transaction.
	if _, _, err = h.persistTaskMove(ctx, tx, id, "todo", pos); err != nil {
		snoozeError(w, err)
		return
	}
	t, err = scanTask(tx.QueryRowContext(ctx, `UPDATE tasks SET snoozed_until=?, snoozed_from=? WHERE id=? RETURNING `+taskColumns, req.Until.UTC().Format(snoozeTimeFormat), t.Status, id))
	if err != nil {
		snoozeError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		snoozeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func snoozeError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task not found")
	} else {
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *taskHandlers) reactivate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := h.reactivateTask(r.Context(), id, nil)
	if err != nil {
		snoozeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// Re-read under the write lock: a concurrent manual move, resnooze, or deletion
// must win over an outdated scheduler scan. Repeated manual reactivation is harmless.
func (h *taskHandlers) reactivateTask(ctx context.Context, id int64, due *time.Time) (Task, error) {
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback()
	t, err := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id=?`, id))
	if err != nil {
		return Task{}, err
	}
	if t.Status != "todo" || t.SnoozedUntil == nil || t.SnoozedFrom == nil {
		return t, nil
	}
	if due != nil && *t.SnoozedUntil > due.UTC().Format(snoozeTimeFormat) {
		return t, nil
	}
	var pos float64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(MIN(position),2.0)-1.0 FROM tasks WHERE project_id=? AND status=? AND source='manual'`, t.ProjectID, *t.SnoozedFrom).Scan(&pos); err != nil {
		return Task{}, err
	}
	t, _, err = h.persistTaskMove(ctx, tx, id, *t.SnoozedFrom, pos)
	if err != nil {
		return Task{}, err
	}
	return t, tx.Commit()
}

func (h *taskHandlers) reactivateDue(ctx context.Context, now time.Time) error {
	rows, err := h.db.QueryContext(ctx, `SELECT id FROM tasks WHERE status='todo' AND snoozed_until<=? ORDER BY snoozed_until, id`, now.UTC().Format(snoozeTimeFormat))
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close() // Release the connection before opening write transactions.
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := h.reactivateTask(ctx, id, &now); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	return nil
}

// RunTaskSnoozer catches overdue tasks at startup and wakes tasks even with no browser open.
func RunTaskSnoozer(ctx context.Context, db *sql.DB) {
	h := &taskHandlers{db: db}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := h.reactivateDue(ctx, time.Now()); err != nil && ctx.Err() == nil {
			slog.Error("reactivating snoozed tasks", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
