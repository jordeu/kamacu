package api

import (
	"database/sql"
	"net/http"
	"strings"

	"kangent/internal/session"
)

// AgentRoutes registers the board's agent status source (STAT-01). The
// frontend polls it every 5s and fans one response out to card dots, the
// Agent-tab dot, and the sidebar waiting chips (04-RESEARCH.md Pattern 4).
func AgentRoutes(mux *http.ServeMux, mgr *session.Manager, db *sql.DB) {
	a := &agentHandlers{mgr: mgr, db: db}
	mux.HandleFunc("GET /api/agents/status", a.status)
}

type agentHandlers struct {
	mgr *session.Manager
	db  *sql.DB
}

// agentStatusEntry is the exact JSON contract the 04-03 frontend consumes.
type agentStatusEntry struct {
	TaskID        int64  `json:"taskId"`
	ProjectID     int64  `json:"projectId"`
	SessionID     string `json:"sessionId"`
	Status        string `json:"status"` // working | idle | waiting | exited (Info.AgentStatus)
	ExitCode      *int   `json:"exitCode"`
	StopRequested bool   `json:"stopRequested"`
}

// status handles GET /api/agents/status — one entry per task, newest agent
// session wins. An exited agent stays listed until a fresh agent spawn for
// that task replaces it (UI-SPEC: "Exited dot persists until a new agent
// session starts"). Bash sessions never drive board state (D-48).
func (a *agentHandlers) status(w http.ResponseWriter, r *http.Request) {
	// mgr.List() is newest-first (contract), so the FIRST agent entry seen
	// per task is the newest.
	newest := make(map[int64]session.Info)
	order := make([]int64, 0)
	for _, info := range a.mgr.List() {
		if info.Kind != session.KindAgent || info.TaskID <= 0 {
			continue
		}
		if _, seen := newest[info.TaskID]; seen {
			continue
		}
		newest[info.TaskID] = info
		order = append(order, info.TaskID)
	}

	entries := []agentStatusEntry{} // [] when empty — never null
	if len(order) > 0 {
		// Resolve projectId for all tasks in ONE query; sessions whose task
		// row vanished are skipped.
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(order)), ",")
		args := make([]any, len(order))
		for i, id := range order {
			args[i] = id
		}
		rows, err := a.db.Query(`SELECT id, project_id FROM tasks WHERE id IN (`+placeholders+`)`, args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		projects := make(map[int64]int64, len(order))
		for rows.Next() {
			var id, pid int64
			if err := rows.Scan(&id, &pid); err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			projects[id] = pid
		}
		if err := rows.Err(); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, tid := range order {
			pid, ok := projects[tid]
			if !ok {
				continue // task deleted under a still-tracked session
			}
			info := newest[tid]
			entries = append(entries, agentStatusEntry{
				TaskID:        tid,
				ProjectID:     pid,
				SessionID:     info.ID,
				Status:        info.AgentStatus,
				ExitCode:      info.ExitCode,
				StopRequested: info.StopRequested,
			})
		}
	}
	writeJSON(w, http.StatusOK, entries)
}
