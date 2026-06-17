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
	a := &agentHandlers{mgr: mgr, db: db, globRoot: defaultTranscriptGlobRoot()}
	mux.HandleFunc("GET /api/agents/status", a.status)
}

type agentHandlers struct {
	mgr      *session.Manager
	db       *sql.DB
	globRoot string // ~/.claude/projects; tests inject a temp dir
}

// agentStatusEntry is the exact JSON contract the 04-03 frontend consumes.
type agentStatusEntry struct {
	TaskID        int64  `json:"taskId"`
	ProjectID     int64  `json:"projectId"`
	SessionID     string `json:"sessionId"`
	Status        string `json:"status"` // working | idle | waiting | exited (Info.AgentStatus)
	ExitCode      *int   `json:"exitCode"`
	StopRequested bool   `json:"stopRequested"`
	Resumable     bool   `json:"resumable"` // RCVR-01/RCVR-02: transcript exists + worktree + no running agent (D-54b)
	PRNumber      *int64 `json:"prNumber"`  // null for source='manual'; the PR number for a github_pr review task (D-15)
	Source        string `json:"source"`    // "manual" | "github_pr"
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

	// taskMeta carries the per-task DB columns the resumable derivation needs,
	// plus the PR linkage (pr_number/source, D-15) joined onto every entry.
	type taskMeta struct {
		projectID int64
		csid      sql.NullString
		wtp       sql.NullString
		prNumber  sql.NullInt64
		source    string
	}

	// prNumberOf converts a nullable PR number column into the *int64 the wire
	// contract wants (null for manual tasks, the number for github_pr tasks).
	prNumberOf := func(n sql.NullInt64) *int64 {
		if !n.Valid {
			return nil
		}
		v := n.Int64
		return &v
	}

	entries := []agentStatusEntry{} // [] when empty — never null

	// Manager-derived pass: one entry per task with a live (running OR exited)
	// agent session, newest wins. A running agent is never resumable; an
	// exited one is resumable iff it has a stored id + worktree + transcript.
	if len(order) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(order)), ",")
		args := make([]any, len(order))
		for i, id := range order {
			args[i] = id
		}
		rows, err := a.db.Query(`SELECT id, project_id, claude_session_id, worktree_path, pr_number, source FROM tasks WHERE id IN (`+placeholders+`)`, args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		metas := make(map[int64]taskMeta, len(order))
		for rows.Next() {
			var id int64
			var m taskMeta
			if err := rows.Scan(&id, &m.projectID, &m.csid, &m.wtp, &m.prNumber, &m.source); err != nil {
				rows.Close()
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			metas[id] = m
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		rows.Close()
		for _, tid := range order {
			m, ok := metas[tid]
			if !ok {
				continue // task deleted under a still-tracked session
			}
			info := newest[tid]
			resumable := info.AgentStatus == "exited" && m.csid.Valid && m.wtp.Valid && transcriptExists(a.globRoot, m.csid.String)
			entries = append(entries, agentStatusEntry{
				TaskID:        tid,
				ProjectID:     m.projectID,
				SessionID:     info.ID,
				Status:        info.AgentStatus,
				ExitCode:      info.ExitCode,
				StopRequested: info.StopRequested,
				Resumable:     resumable,
				PRNumber:      prNumberOf(m.prNumber),
				Source:        m.source,
			})
		}
	}

	// DB-derived pass (RCVR-01 reconciliation, research Pattern 2): tasks with
	// a persisted session id + worktree but NO manager entry of any state —
	// i.e. post-restart survivors. The DB never records "running" (verified
	// across migrations 00001-00003), so an empty manager + these derived
	// entries is the whole reconciliation story: no startup mutation pass, no
	// migration. Emit ONLY when resumable (transcript exists) — non-resumable
	// past sessions get no dot and the plain pre-start state (D-57 only
	// constrains resumable tasks).
	rows, err := a.db.Query(`SELECT id, project_id, claude_session_id, worktree_path, pr_number, source FROM tasks
		WHERE claude_session_id IS NOT NULL AND worktree_path IS NOT NULL`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, pid int64
		var csid, wtp sql.NullString
		var prNumber sql.NullInt64
		var source string
		if err := rows.Scan(&id, &pid, &csid, &wtp, &prNumber, &source); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if _, hasManagerEntry := newest[id]; hasManagerEntry {
			continue // already covered by the manager-derived pass
		}
		if !transcriptExists(a.globRoot, csid.String) {
			continue
		}
		// A post-restart PR-review session (github_pr task carrying a
		// claude_session_id + worktree_path) legitimately surfaces here with its
		// prNumber — that is correct and desired: the PR card's dot/border
		// survive a restart (D-15).
		entries = append(entries, agentStatusEntry{
			TaskID:        id,
			ProjectID:     pid,
			SessionID:     "",
			Status:        "exited",
			ExitCode:      nil,
			StopRequested: false,
			Resumable:     true,
			PRNumber:      prNumberOf(prNumber),
			Source:        source,
		})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, entries)
}
