package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"kamacu/internal/session"
	"kamacu/internal/settings"
	"kamacu/internal/tmux"
)

// SessionRoutes registers terminal session endpoints on mux. db is consulted to
// resolve a task's worktree path on a task-scoped spawn (TERM-04) and to
// reconcile surviving tmux_sessions rows after a restart (TMUX-05). tmuxClient
// drives the has-session liveness probe that decides which persisted rows are
// post-restart survivors.
func SessionRoutes(mux *http.ServeMux, mgr *session.Manager, db *sql.DB, tmuxClient tmux.Client) {
	s := &sessionHandlers{mgr: mgr, db: db, globRoot: defaultTranscriptGlobRoot(), tmuxClient: tmuxClient}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	mux.HandleFunc("POST /api/sessions/{id}/stop", s.stop)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
	mux.HandleFunc("PATCH /api/sessions/{id}", s.rename)
}

type sessionHandlers struct {
	mgr        *session.Manager
	db         *sql.DB
	globRoot   string      // ~/.claude/projects; tests inject a temp dir for resume validation
	tmuxClient tmux.Client // socket/config for the TMUX-05 has-session reconcile probe
}

// list handles GET /api/sessions — newest first, JSON [] when empty.
// ?task_id=N filters to that task's sessions (running AND exited — the
// exited-ghost handling is client-side per D-28).
func (h *sessionHandlers) list(w http.ResponseWriter, r *http.Request) {
	var infos []session.Info
	if q := r.URL.Query().Get("task_id"); q == "" {
		infos = h.mgr.List()
	} else {
		id, err := strconv.ParseInt(q, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid task_id")
			return
		}
		infos = h.mgr.ListByTask(id)
		// TMUX-05 restart reconcile (D-88), mirroring the agents.go two-pass:
		// the manager-derived pass above is the live in-memory sessions; this
		// DB-derived pass appends one entry per surviving tmux_sessions row that
		// is alive per has-session but has NO live in-memory session — a
		// post-restart survivor. Dead rows are lazily GC'd (D-89) and never
		// surface. Scoped to task lists only: unscoped dev lists have no rows.
		infos = h.reconcileTmux(r, id, infos)
	}
	if infos == nil {
		infos = []session.Info{}
	}
	writeJSON(w, http.StatusOK, infos)
}

// reconcileTmux appends orphaned (restored) tmux survivor entries to infos for
// the given task and lazily GCs rows whose tmux session died while Kamacu was
// down (D-89). A row is a survivor iff (a) NO live in-memory session is bound to
// its name (else it is already in infos) AND (b) tmux has-session reports it
// alive. A row whose probe is conclusively dead (exit 1) is DELETEd; an
// inconclusive probe (tmux binary broken/hung — Pitfall 6) leaves the row
// untouched for a later poll and surfaces nothing. A query failure degrades to
// the live-only list rather than failing the whole request.
func (h *sessionHandlers) reconcileTmux(r *http.Request, taskID int64, infos []session.Info) []session.Info {
	rows, err := h.db.Query(`SELECT name, label, created_at FROM tmux_sessions WHERE task_id = ?`, taskID)
	if err != nil {
		slog.Warn("reconcile tmux sessions: query", "task", taskID, "error", err)
		return infos
	}
	type row struct {
		name, label, createdAt string
	}
	var candidates []row
	for rows.Next() {
		var rw row
		if err := rows.Scan(&rw.name, &rw.label, &rw.createdAt); err != nil {
			rows.Close()
			slog.Warn("reconcile tmux sessions: scan", "task", taskID, "error", err)
			return infos
		}
		candidates = append(candidates, rw)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		slog.Warn("reconcile tmux sessions: rows", "task", taskID, "error", err)
		return infos
	}
	rows.Close()

	for _, rw := range candidates {
		// A live in-memory session for this name is already in infos as a normal
		// running bash entry — skip, it is no survivor.
		if h.mgr.HasLiveTmux(rw.name) {
			continue
		}
		alive, err := h.tmuxClient.HasSession(r.Context(), rw.name)
		switch {
		case err != nil:
			// Inconclusive probe (tmux missing/hung): never GC on an honest
			// unknown (Pitfall 6); leave the row, surface nothing.
			continue
		case alive:
			label := rw.label
			if label == "" {
				// D-04: never surface the old Bash-question-mark sentinel for an
				// empty label (a pre-write survivor or a failed spawn back-fill).
				// Re-derive the deterministic Bash <n> default from the machine-minted
				// tmux name kamacu-<task>-<n>, exactly the label the spawn UPDATE would
				// have written, so a restarted survivor always shows a real name.
				label = defaultTmuxLabel(rw.name)
			}
			created, perr := time.Parse(time.RFC3339, rw.createdAt)
			if perr != nil {
				created = time.Now()
			}
			infos = append(infos, session.Info{
				Label:     label,
				Status:    session.StatusRunning,
				Kind:      session.KindBash,
				TaskID:    taskID,
				CreatedAt: created,
				Orphaned:  true,
				TmuxName:  rw.name,
			})
		default:
			// Conclusively dead (exit 1): the session died while Kamacu was
			// down — lazy GC the row (D-89), warn-only on failure.
			if _, derr := h.db.Exec(`DELETE FROM tmux_sessions WHERE name = ?`, rw.name); derr != nil {
				slog.Warn("reconcile tmux sessions: GC dead row", "name", rw.name, "error", derr)
			}
		}
	}
	return infos
}

// create handles POST /api/sessions — spawns a bash session, or with
// {"kind":"agent"} the task's Claude Code agent session (TERM-01). An
// optional {"task_id":N} body scopes the session to a task: it spawns in the
// task's worktree (TERM-04) with the per-task "Bash N" label. An empty body
// (the /terminal dev route sends none) spawns an unscoped dev session exactly
// as before. Agents require a task worktree and are limited to ONE running
// per task (D-38) — the API enforces both.
func (h *sessionHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID int64  `json:"task_id"`
		Kind   string `json:"kind"`
		Resume bool   `json:"resume"` // RCVR-02: resume the task's stored claude session (agent-only)
		// ReattachTmuxName, when set, reattaches to an EXISTING persisted tmux
		// session by name instead of minting a new one (TMUX-05, D-88). The
		// frontend fires it for a restored orphaned entry; new-session -A is
		// attach-or-create, so spawning with the surviving name reconnects
		// losslessly. Bash-only; never mints/inserts a row.
		ReattachTmuxName string `json:"reattach_tmux_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	kind := session.KindBash
	switch req.Kind {
	case "", "bash":
	case "agent":
		kind = session.KindAgent
	default:
		writeError(w, http.StatusBadRequest, "invalid kind")
		return
	}
	// Resume is a variant of the agent spawn only — never a bash session.
	if req.Resume && kind != session.KindAgent {
		writeError(w, http.StatusBadRequest, "resume requires kind agent")
		return
	}
	// Reattach is a bash-only variant: it reconnects to a surviving tmux
	// session, which agents never use. Validate eagerly so the error copy is
	// crisp before any DB work.
	reattach := req.ReattachTmuxName != ""
	if reattach {
		if kind == session.KindAgent {
			writeError(w, http.StatusBadRequest, "reattach requires a bash session")
			return
		}
		if req.TaskID <= 0 {
			writeError(w, http.StatusConflict, "tmux shells need a task — open a task's terminal")
			return
		}
	}
	// Agents always run in a task worktree: no task means no worktree — the
	// same gate (and copy) as a worktree-less task.
	if kind == session.KindAgent && req.TaskID <= 0 {
		writeError(w, http.StatusConflict, "task has no worktree")
		return
	}
	opts := session.SpawnOpts{Kind: kind}
	// csid holds the task's persisted claude session id, read fresh inside the
	// handler (Pitfall 6: this in-handler read is the single source of truth at
	// spawn time — a stale client Resume after a Reset minted a new id simply
	// resumes the NEW id, which is correct newest-wins behavior).
	var csid sql.NullString
	var agentEngine, agentCommand string // M001: resolved alongside the task's worktree
	if req.TaskID > 0 {
		var path sql.NullString
		err := h.db.QueryRow(
			`SELECT t.worktree_path, t.claude_session_id, a.engine, a.command
			 FROM tasks t
			 JOIN projects p ON p.id = t.project_id
			 JOIN agents a ON a.id = p.agent_id
			 WHERE t.id = ?`, req.TaskID,
		).Scan(&path, &csid, &agentEngine, &agentCommand)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "task not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !path.Valid {
			writeError(w, http.StatusConflict, "task has no worktree") // D-30 server side
			return
		}
		opts.Cwd, opts.TaskID = path.String, req.TaskID
	}
	// One-agent-per-task gate (D-38), checked BEFORE spawning. An EXITED
	// agent never blocks — that is the "Reset session" path (D-41, revised at checkpoint).
	// Resume rides this gate unchanged — it IS D-67's never-two-PTYs guarantee.
	if kind == session.KindAgent {
		for _, info := range h.mgr.ListByTask(req.TaskID) {
			if info.Kind == session.KindAgent && info.Status == session.StatusRunning {
				writeError(w, http.StatusConflict, "agent session already running")
				return
			}
		}
	}
	// Resume validation, AFTER the one-per-task gate: the server never trusts
	// the client's resumable snapshot. A NULL stored id or a missing transcript
	// is an honest 409 (the transcript glob self-heals D-56's resumable:false).
	if req.Resume {
		if !csid.Valid || !transcriptExists(h.globRoot, csid.String) {
			writeError(w, http.StatusConflict, "no session to resume")
			return
		}
		opts.ResumeSessionID = csid.String
	}
	// reattachLabel carries the persisted tmux_sessions.label from the reattach
	// branch below through to after Spawn, so a restored survivor keeps its custom
	// name (GAP-01): Manager.Spawn has no label param and would otherwise re-derive
	// "Bash N", which the post-spawn back-fill would then persist over the stored label.
	var reattachLabel string
	// SET-03 read-at-use: settings come from the DB at EVERY spawn — never
	// cached — so edits apply at the next Start with no restart. A real DB
	// error (absent rows read as defaults) is exceptional on local SQLite:
	// fail the spawn rather than silently falling back.
	if kind == session.KindAgent {
		// One read covers BOTH the fresh and resume variants — opts is shared
		// (AGENT-01: extras ride every claude spawn).
		raw, err := settings.Get(h.db, settings.KeyAgentExtraParams)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "couldn't start a session")
			return
		}
		opts.ExtraArgs = settings.Tokenize(raw)
		// M001: carry the resolved agent's engine + (for custom) its rendered
		// command into SpawnOpts. The claude path ignores AgentArgs and builds
		// its own argv; the custom path runs AgentArgs as a plain command.
		opts.AgentEngine = agentEngine
		if agentEngine != "" && agentEngine != "claude" {
			// Custom agents don't resume, but {{session_id}} is offered as an
			// informational stable uuid for users who wire it into their
			// template. Minted here (Manager.Spawn's internal id isn't visible
			// to the handler); cheap, no persistence.
			opts.AgentArgs = renderAgentCommand(agentCommand, opts.Cwd, uuid.NewString())
		}
	} else if reattach {
		// Reattach variant (TMUX-05, D-88): reconnect to a surviving tmux row by
		// name instead of minting a new one. The worktree query above already set
		// opts.Cwd/TaskID (and rejected a missing worktree with 409 "task has no
		// worktree"). Verify the row belongs to THIS task so a client can never
		// reattach to an arbitrary name, then reuse the persisted name — no
		// mint, no INSERT (the row already exists; MAX(n)+1 stays correct because
		// it persists). Spawn runs new-session -A against the live session. The
		// persisted label is captured into reattachLabel and reapplied after Spawn
		// (GAP-01) so a renamed survivor keeps its custom name.
		err := h.db.QueryRow(`SELECT label FROM tmux_sessions WHERE task_id = ? AND name = ?`, req.TaskID, req.ReattachTmuxName).Scan(&reattachLabel)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "no session to reattach")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "couldn't start a session")
			return
		}
		opts.TmuxName = req.ReattachTmuxName
	} else {
		// Covers task bash tabs AND the unscoped /terminal dev spawn — one
		// code path (SHELL-02).
		sh, err := settings.Get(h.db, settings.KeyShell)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "couldn't start a session")
			return
		}
		if sh == "tmux" {
			// tmux shells are task-scoped: the name embeds the task id and the row
			// references tasks(id). The unscoped /terminal dev route gets an honest
			// 409 (D-56 posture) — NEVER a bare `tmux` exec, which would open an
			// unnamed session on the user's DEFAULT socket.
			if req.TaskID <= 0 {
				writeError(w, http.StatusConflict, "tmux shells need a task — open a task's terminal")
				return
			}
			// n from the DB, NEVER the in-memory counter (it resets on restart; a
			// collision would make new-session -A silently attach a second tab to a
			// surviving shell — research Pitfall 4). Insert BEFORE Spawn reserves n
			// under UNIQUE(task_id,n); single-user localhost makes the read-then-
			// insert race window acceptable, with the constraint as backstop.
			var n int64
			if err := h.db.QueryRow(`SELECT COALESCE(MAX(n),0)+1 FROM tmux_sessions WHERE task_id = ?`, req.TaskID).Scan(&n); err != nil {
				writeError(w, http.StatusInternalServerError, "couldn't start a session")
				return
			}
			name := fmt.Sprintf("kamacu-%d-%d", req.TaskID, n)
			// D-04 belt-and-braces: persist the default "Bash N" label at INSERT so
			// the column is never transiently '' even if the later sess.Info().Label
			// back-fill UPDATE (below) fails. The two writes agree (both "Bash N"),
			// so the back-fill is idempotent reconciliation, not a conflict.
			label := fmt.Sprintf("Bash %d", n)
			if _, err := h.db.Exec(`INSERT INTO tmux_sessions (task_id, n, name, label) VALUES (?, ?, ?, ?)`, req.TaskID, n, name, label); err != nil {
				writeError(w, http.StatusInternalServerError, "couldn't start a session")
				return
			}
			opts.TmuxName = name
		} else {
			opts.Shell = sh
		}
	}
	// Spawn's stat pre-check covers a vanished worktree dir → same 500 path.
	sess, err := h.mgr.Spawn(opts)
	if err != nil {
		// Release the reserved n ONLY on a fresh spawn — the name was never
		// used. A reattach row is a pre-existing survivor, NOT a freshly
		// reserved n: never delete it on a transient spawn failure (D-89 owns
		// its eventual GC when the underlying tmux session is conclusively dead).
		if opts.TmuxName != "" && !reattach {
			if _, derr := h.db.Exec(`DELETE FROM tmux_sessions WHERE name = ?`, opts.TmuxName); derr != nil {
				slog.Warn("releasing tmux session row", "name", opts.TmuxName, "error", derr)
			}
		}
		if errors.Is(err, session.ErrTmuxNotFound) {
			// D-84: honest error, never a silent bash fallback. 409 (not 500) so
			// the frontend renders the message verbatim.
			writeError(w, http.StatusConflict, "tmux not found — change the shell setting or reinstall")
			return
		}
		writeError(w, http.StatusInternalServerError, "couldn't start a session")
		return
	}
	// Persist the Phase 5 --resume key BEFORE replying (latest spawn wins).
	// A write failure degrades Phase 5 resume only — the session is usable.
	if kind == session.KindAgent {
		if _, err := h.db.Exec(`UPDATE tasks SET claude_session_id = ? WHERE id = ?`, sess.ClaudeSessionID(), req.TaskID); err != nil {
			slog.Warn("persisting claude_session_id", "task", req.TaskID, "error", err)
		}
	}
	// GAP-01: restore the survivor's persisted custom label onto the reattached
	// session. Spawn re-derived a fresh "Bash N" (it has no label param); without
	// this the wire reply shows the wrong name AND the back-fill below would
	// overwrite the stored custom label with that default. An empty stored label
	// is left as the derived default (matches reconcile's empty-label handling).
	if reattach && reattachLabel != "" {
		sess.SetLabel(reattachLabel)
	}
	// tmux label back-fill, warn-only (same degradation posture as the agent
	// claude_session_id persist above): a failed write costs only the Phase 9
	// resume label, never the session.
	if opts.TmuxName != "" {
		if _, err := h.db.Exec(`UPDATE tmux_sessions SET label = ? WHERE name = ?`, sess.Info().Label, opts.TmuxName); err != nil {
			slog.Warn("persisting tmux session label", "name", opts.TmuxName, "error", err)
		}
	}
	writeJSON(w, http.StatusCreated, sess.Info())
}

// stop handles POST /api/sessions/{id}/stop. Stop blocks up to the 5s
// SIGTERM grace (D-14), so it runs in a goroutine and the reply is 202
// immediately. Idempotent: stopping an already-exited session is a no-op
// that still returns 202.
func (h *sessionHandlers) stop(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	go sess.Stop()
	w.WriteHeader(http.StatusAccepted)
}

// maxLabelRunes bounds a stored+rendered session label (T-24-01): an unbounded
// user-supplied label could bloat the tmux_sessions row and the rendered tab
// strip. 200 runes is generous for a human tab name.
const maxLabelRunes = 200

// defaultTmuxLabel re-derives the deterministic "Bash <n>" default from a
// machine-minted tmux session name (kamacu-<task>-<n>). The name is
// server-controlled, not user input. On a malformed name it returns a safe
// non-empty "Bash" — never the old question-mark sentinel — so a restarted
// survivor and an empty-reset both always show a real, non-sentinel name.
func defaultTmuxLabel(tmuxName string) string {
	if i := strings.LastIndexByte(tmuxName, '-'); i >= 0 {
		if n, err := strconv.Atoi(tmuxName[i+1:]); err == nil {
			return fmt.Sprintf("Bash %d", n)
		}
	}
	return "Bash"
}

// rename handles PATCH /api/sessions/{id} — renames a session's display label
// (TABS-01/02). The label is set in memory via the mutex-guarded SetLabel (so it
// survives navigating away and reopening the task for ALL bash tabs), and for
// tmux-backed tabs is ALSO persisted to tmux_sessions.label so it survives a
// server restart. An unknown session id is 404 (T-24-02: h.mgr.Get gates against
// arbitrary/foreign ids). A trimmed-empty label RESETS a tmux tab to its
// re-derived "Bash N" default (D-05). The label is trimmed and capped at
// maxLabelRunes (T-24-01). Persistence is warn-only, mirroring the spawn
// back-fill.
func (h *sessionHandlers) rename(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req struct {
		Label *string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// Absent label: nothing to rename — return the current snapshot unchanged.
	if req.Label == nil {
		writeJSON(w, http.StatusOK, sess.Info())
		return
	}
	newLabel := strings.TrimSpace(*req.Label)
	// D-05: an empty/whitespace commit resets to the auto default. For a tmux tab
	// the default is re-derivable from the kamacu-<task>-<n> name. A non-tmux
	// (plain-bash) tab has no derivable ordinal here (the per-task counter lives
	// in the manager and is not exposed on Session), so an empty reset leaves the
	// current in-memory label unchanged — a plain-bash tab does not survive a
	// restart anyway, so there is nothing to re-derive for it (Claude's discretion).
	if newLabel == "" {
		if sess.TmuxName() != "" {
			newLabel = defaultTmuxLabel(sess.TmuxName())
		} else {
			newLabel = sess.Info().Label
		}
	}
	// Cap the trimmed label (T-24-01) to bound the stored+rendered string.
	if runes := []rune(newLabel); len(runes) > maxLabelRunes {
		newLabel = string(runes[:maxLabelRunes])
	}
	sess.SetLabel(newLabel)
	// tmux tabs additionally persist so the rename survives a restart — the SAME
	// statement the spawn back-fill uses, warn-only on error (same degradation
	// posture: a failed write costs only the restart label, never the session).
	if sess.TmuxName() != "" {
		if _, err := h.db.Exec(`UPDATE tmux_sessions SET label = ? WHERE name = ?`, newLabel, sess.TmuxName()); err != nil {
			slog.Warn("persisting tmux session label", "name", sess.TmuxName(), "error", err)
		}
	}
	writeJSON(w, http.StatusOK, sess.Info())
}

// delete handles DELETE /api/sessions/{id} — removes an EXITED session,
// freeing its replay ring. Running sessions must be stopped first.
func (h *sessionHandlers) delete(w http.ResponseWriter, r *http.Request) {
	err := h.mgr.Remove(r.PathValue("id"))
	switch {
	case errors.Is(err, session.ErrNotFound):
		writeError(w, http.StatusNotFound, "session not found")
	case errors.Is(err, session.ErrStillRunning):
		writeError(w, http.StatusConflict, "session is still running. Stop it first.")
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
