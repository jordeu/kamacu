package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
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
	mux.HandleFunc("POST /api/sessions/{id}/input", s.input)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.delete)
	mux.HandleFunc("PATCH /api/sessions/{id}", s.rename)
	// Phase 08 read-only endpoints (MCPSESS-01/02/03 server-side): the bridge
	// subcommand reaches the in-memory engine only through these. They consume
	// ONLY session.Info/Snapshot (D-14) — never the PTY-write primitive.
	mux.HandleFunc("GET /api/sessions/{id}", s.getSession)
	mux.HandleFunc("GET /api/sessions/{id}/output", s.getSessionOutput)
	// MCPSESS-04 server-side: subscribe is plain HTTP chunked octet-stream —
	// never the WS frame protocol (D-14). The handler is the per-request reader;
	// no goroutine launched here outlives the request.
	mux.HandleFunc("GET /api/sessions/{id}/subscribe", s.subscribeSessionOutput)
}

type sessionHandlers struct {
	mgr        *session.Manager
	db         *sql.DB
	globRoot   string      // ~/.claude/projects; tests inject a temp dir for resume validation
	tmuxClient tmux.Client // socket/config for the TMUX-05 has-session reconcile probe
}

// list handles GET /api/sessions — newest first, JSON [] when empty.
// ?task_id=N filters to that task's sessions (running AND exited — the
// exited-ghost handling is client-side per D-28). ?project_id=N (Phase 08,
// MCPSESS-01) filters to sessions whose task belongs to that project. The two
// filters are mutually exclusive; project_id never triggers the tmux reconcile
// pass (dev lists have no orphaned rows). ?scope=global (Phase 15) returns
// exactly the global sessions plus the scoped tmux reconcile — the /global
// view's tab strip. Every entry also carries taskTitle/projectName/agentName
// via the D-10 JOIN (empty for dev sessions); global entries get their honest
// synthesized Scratchpad/Global labels instead (GINT-02).
func (h *sessionHandlers) list(w http.ResponseWriter, r *http.Request) {
	var infos []session.Info
	if r.URL.Query().Get("scope") == "global" {
		// GLOBAL branch (Phase 15): exactly the engine's global sessions,
		// then the scoped reconcile — the reconcileTmux survivor/lazy-GC
		// logic with the WHERE clause on the 00018 scope discriminator
		// instead of task_id.
		infos = h.reconcileGlobalTmux(r, h.mgr.ListGlobal())
	} else if q := r.URL.Query().Get("task_id"); q == "" {
		// Phase 08 project_id filter (MCPSESS-01): scope to sessions whose
		// task belongs to the project. One query builds the task-ID set, then
		// an in-memory filter narrows mgr.List() (D-11: never N round-trips).
		if pidQ := r.URL.Query().Get("project_id"); pidQ != "" {
			pid, err := strconv.ParseInt(pidQ, 10, 64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid project_id")
				return
			}
			taskIDs := make(map[int64]struct{})
			rows, err := h.db.Query(`SELECT id FROM tasks WHERE project_id = ?`, pid)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			for rows.Next() {
				var tid int64
				if err := rows.Scan(&tid); err != nil {
					rows.Close()
					writeError(w, http.StatusInternalServerError, err.Error())
					return
				}
				taskIDs[tid] = struct{}{}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			rows.Close()
			for _, info := range h.mgr.List() {
				if _, ok := taskIDs[info.TaskID]; ok {
					infos = append(infos, info)
				}
			}
		} else {
			infos = h.mgr.List()
		}
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
	// Attach the D-10 task->project->agent JOIN (taskTitle/projectName/
	// agentName) and serialize as sessionDetail. Dev sessions (TaskID==0)
	// keep empty JOIN fields (omitted on the wire via omitempty). Global
	// entries are synthesized per-entry (GINT-02, D-09/D-10): taskTitle
	// "Scratchpad", projectName "Global", agentName from ONE singleton agent
	// JOIN read per request — and NEVER a zero-keyed ctxByTask entry (the
	// map is keyed by the field global sessions don't have; key 0 would leak
	// the Scratchpad labels onto every dev session — Pitfall 3).
	ctxByTask := h.joinSessionContext(infos)
	var gctx *sessionContext
	out := make([]sessionDetail, len(infos))
	for i, info := range infos {
		if info.Global {
			if gctx == nil {
				c := h.globalSessionContext()
				gctx = &c
			}
			out[i] = sessionDetail{Info: info, TaskTitle: "Scratchpad", ProjectName: "Global", AgentName: gctx.agentName}
			continue
		}
		ctx := ctxByTask[info.TaskID]
		out[i] = sessionDetail{Info: info, TaskTitle: ctx.taskTitle, ProjectName: ctx.projectName, AgentName: ctx.agentName}
	}
	writeJSON(w, http.StatusOK, out)
}

// reconcileGlobalTmux is the scoped sibling of reconcileTmux (Phase 15,
// GSESS-03): the same survivor/lazy-GC pass over the scope='global' rows.
// A row is a survivor iff (a) NO live in-memory session is bound to its name
// AND (b) tmux has-session reports it alive; dead rows are lazily GC'd
// (D-89); inconclusive probes surface nothing (Pitfall 6). The synthesized
// orphaned Info gains Global:true (the /global view keys off it) and the
// empty-label fallback goes through defaultTmuxLabel exactly as tasks do
// ("kamacu-global-3" already yields "Bash 3").
func (h *sessionHandlers) reconcileGlobalTmux(r *http.Request, infos []session.Info) []session.Info {
	rows, err := h.db.Query(`SELECT name, label, created_at FROM tmux_sessions WHERE scope = 'global'`)
	if err != nil {
		slog.Warn("reconcile global tmux sessions: query", "error", err)
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
			slog.Warn("reconcile global tmux sessions: scan", "error", err)
			return infos
		}
		candidates = append(candidates, rw)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		slog.Warn("reconcile global tmux sessions: rows", "error", err)
		return infos
	}
	rows.Close()

	for _, rw := range candidates {
		if h.mgr.HasLiveTmux(rw.name) {
			continue
		}
		alive, err := h.tmuxClient.HasSession(r.Context(), rw.name)
		switch {
		case err != nil:
			continue
		case alive:
			label := rw.label
			if label == "" {
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
				Global:    true,
				CreatedAt: created,
				Orphaned:  true,
				TmuxName:  rw.name,
			})
		default:
			if _, derr := h.db.Exec(`DELETE FROM tmux_sessions WHERE name = ?`, rw.name); derr != nil {
				slog.Warn("reconcile global tmux sessions: GC dead row", "name", rw.name, "error", derr)
			}
		}
	}
	return infos
}

// globalSessionContext reads the singleton's agent name — the ONE per-request
// JOIN behind the honest global labels (GINT-02). Degrades to an empty
// agentName (never fails the read path); ErrNoRows is unreachable (00017
// seed + BackfillGlobalTask) and also degrades — the labels Scratchpad/
// Global stay honest even if the agent JOIN hiccups.
func (h *sessionHandlers) globalSessionContext() sessionContext {
	var agentName string
	if err := h.db.QueryRow(
		`SELECT a.name FROM global_task g JOIN agents a ON a.id = g.agent_id WHERE g.id = 1`,
	).Scan(&agentName); err != nil {
		slog.Warn("globalSessionContext: query", "error", err)
		return sessionContext{}
	}
	return sessionContext{taskTitle: "Scratchpad", projectName: "Global", agentName: agentName}
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
//
// Phase 15: {"scope":"global"} (mutually exclusive with task_id) spawns in
// the global Scratchpad root behind the D-28/D-29 gates — plain bash and
// tmux tabs ("kamacu-global-<n>") with the same shell options as tasks.
func (h *sessionHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TaskID int64  `json:"task_id"`
		Kind   string `json:"kind"`
		Resume bool   `json:"resume"` // RCVR-02: resume the task's stored claude session (agent-only)
		// Scope is the Phase 15 global discriminator: "" (default — task/dev
		// behavior unchanged) or "global" — the session spawns in the global
		// root behind the D-28/D-29 gates. Mutually exclusive with task_id.
		Scope string `json:"scope"`
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
	// Scope is a closed set (T-15-05): "" or "global", anything else is the
	// invalid-kind family of 400.
	if req.Scope != "" && req.Scope != "global" {
		writeError(w, http.StatusBadRequest, "invalid scope")
		return
	}
	// Mutual exclusion (the global.go repo/root_path family): the global
	// scope is task-less by construction — a body carrying both is ambiguous.
	if req.Scope == "global" && req.TaskID != 0 {
		writeError(w, http.StatusBadRequest, "supply either scope or task_id, not both")
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
		// Global scope is the one task-less context tmux rows exist for
		// (00018 scope discriminator); every other task-less reattach is the
		// dev route, which keeps its honest 409.
		if req.TaskID <= 0 && req.Scope != "global" {
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
	// csid holds the task's persisted claude session id; ocsid holds its
	// opencode counterpart (the opaque ses_… opencode mints, captured ASYNC in
	// T03). Both are read fresh inside the handler (Pitfall 6: this in-handler
	// read is the single source of truth at spawn time — a stale client Resume
	// after a Reset minted a new id simply resumes the NEW id, which is correct
	// newest-wins behavior).
	var csid, ocsid sql.NullString
	var agentEngine, agentCommand, agentExtraParams string // M001: resolved alongside the task's worktree
	if req.TaskID > 0 {
		var path sql.NullString
		err := h.db.QueryRow(
			`SELECT t.worktree_path, t.claude_session_id, t.opencode_session_id, a.engine, a.command, a.extra_params
			 FROM tasks t
			 JOIN projects p ON p.id = t.project_id
			 JOIN agents a ON a.id = p.agent_id
			 WHERE t.id = ?`, req.TaskID,
		).Scan(&path, &csid, &ocsid, &agentEngine, &agentCommand, &agentExtraParams)
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
	} else if req.Scope == "global" {
		// GLOBAL branch (Phase 15, D-24/D-28..D-33): the singleton read-at-use —
		// root + agent + resume ids resolved fresh per request, exactly where
		// the task worktree query feeds every kind above. ErrNoRows is a
		// corrupted invariant (00017 seed + BackfillGlobalTask) → 500, the
		// loadGlobalConfig fail-loud posture.
		var rootPath string
		err := h.db.QueryRow(
			`SELECT g.root_path, g.claude_session_id, g.opencode_session_id, a.engine, a.command, a.extra_params
			 FROM global_task g
			 JOIN agents a ON a.id = g.agent_id
			 WHERE g.id = 1`,
		).Scan(&rootPath, &csid, &ocsid, &agentEngine, &agentCommand, &agentExtraParams)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "global task row missing")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// One gate block for every kind (D-30), order per D-33: unconfigured
		// root first (D-28) — never a silent home/cwd fallback; then the
		// vanished-root stat (D-29) with the stored path verbatim.
		if rootPath == "" {
			writeError(w, http.StatusConflict, "global root not configured")
			return
		}
		if fi, serr := os.Stat(rootPath); serr != nil || !fi.IsDir() {
			writeError(w, http.StatusConflict, "global root no longer exists on disk: "+rootPath)
			return
		}
		opts.Cwd, opts.Global = rootPath, true
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
	// the client's resumable snapshot. The gate is ENGINE-BRANCHED (M002/S03):
	// opencode has NO transcript files (its sessions live in opencode.db, not
	// ~/.claude/projects), so it keys off the persisted opencode_session_id alone
	// — an empty/stale id is an honest 409 "no session to resume" (mirror claude's
	// posture, NEVER silently fork a fresh session). The claude path is unchanged
	// byte-for-byte (csid + the transcript glob that self-heals D-56's resumable).
	if req.Resume {
		if agentEngine == "opencode" {
			if !ocsid.Valid {
				writeError(w, http.StatusConflict, "no session to resume")
				return
			}
			// opts.ResumeSessionID stays "" — opencode does NOT route through
			// claude's --resume (MEM027); the `-s <id>` flag is appended to
			// AgentArgs in the custom spawn arm below.
		} else {
			if !csid.Valid || !transcriptExists(h.globRoot, csid.String) {
				writeError(w, http.StatusConflict, "no session to resume")
				return
			}
			opts.ResumeSessionID = csid.String
		}
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
		// M001 gate follow-up: extras now live on the agent row (relocated from
		// the global agent_extra_params setting). AGENT-01 unchanged: extras
		// ride every claude spawn. Custom agents ignore extras (their flags
		// are in the command template).
		opts.ExtraArgs = settings.Tokenize(agentExtraParams)
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
			// M002/S03: an opencode task with a persisted opencode_session_id
			// resumes by appending `opencode -s <id>` (opencode's own resume
			// flag), NOT claude's --resume and NOT a fresh `opencode`. The
			// fresh opencode spawn stays renderAgentCommand("opencode",...) ==
			// ["opencode"] (NO -s; the seed command has no placeholders). The
			// fake-opencode argv stub locks this exact fresh-vs-resume argv.
			if agentEngine == "opencode" && req.Resume && ocsid.Valid {
				opts.AgentArgs = append(opts.AgentArgs, "-s", ocsid.String)
			}
		}
	} else if reattach {
		// Reattach variant (TMUX-05, D-88): reconnect to a surviving tmux row by
		// name instead of minting a new one. The worktree query above already set
		// opts.Cwd/TaskID (and rejected a missing worktree with 409 "task has no
		// worktree"); the global branch likewise set opts.Cwd/Global behind the
		// root gates. The lookup itself is SCOPE-SCOPED (D-32): task scope reads
		// its task rows, global scope reads scope='global' rows — a foreign row
		// simply does not exist in this scope, so both directions get the same
		// honest 404 with no scope-mismatch oracle and no information leak.
		// Reuse the persisted name — no mint, no INSERT (the row already exists;
		// MAX(n)+1 stays correct because it persists). Spawn runs new-session -A
		// against the live session. The persisted label is captured into
		// reattachLabel and reapplied after Spawn (GAP-01) so a renamed survivor
		// keeps its custom name.
		var err error
		if req.Scope == "global" {
			err = h.db.QueryRow(`SELECT label FROM tmux_sessions WHERE scope = 'global' AND name = ?`, req.ReattachTmuxName).Scan(&reattachLabel)
		} else {
			err = h.db.QueryRow(`SELECT label FROM tmux_sessions WHERE task_id = ? AND name = ?`, req.TaskID, req.ReattachTmuxName).Scan(&reattachLabel)
		}
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
		// Covers task bash tabs, global bash tabs, AND the unscoped /terminal
		// dev spawn — one code path (SHELL-02); the global scope reads the
		// same settings shell as tasks (GVIEW-03).
		sh, err := settings.Get(h.db, settings.KeyShell)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "couldn't start a session")
			return
		}
		if sh == "tmux" {
			if req.Scope == "global" {
				// GLOBAL mint (D-11/D-02): the scope-scoped counter, name
				// kamacu-global-<n>. The 00018 XOR CHECK requires (NULL
				// task_id, scope 'global') to agree; the real mint-race
				// backstop is name UNIQUE — NULL task_ids are DISTINCT under
				// UNIQUE(task_id,n), so that composite never protects global
				// rows (same single-user localhost window the task path
				// accepts). n from the DB, NEVER the in-memory counter (it
				// resets on restart); insert BEFORE Spawn, exactly the task
				// posture.
				var n int64
				if err := h.db.QueryRow(`SELECT COALESCE(MAX(n),0)+1 FROM tmux_sessions WHERE scope = 'global'`).Scan(&n); err != nil {
					writeError(w, http.StatusInternalServerError, "couldn't start a session")
					return
				}
				name := fmt.Sprintf("kamacu-global-%d", n)
				// D-04 belt-and-braces: persist the default "Bash N" label at
				// INSERT (same two-writes-agree posture as the task mint).
				label := fmt.Sprintf("Bash %d", n)
				if _, err := h.db.Exec(`INSERT INTO tmux_sessions (task_id, scope, n, name, label) VALUES (NULL, 'global', ?, ?, ?)`, n, name, label); err != nil {
					writeError(w, http.StatusInternalServerError, "couldn't start a session")
					return
				}
				opts.TmuxName = name
			} else {
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
			}
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
		// M002/S03/T03 (Strategy B — subprocess discovery): opencode mints its
		// own opaque ses_… id (unlike claude, where kamacu mints --session-id),
		// so kamacu must DISCOVER it. The opencode session row is NOT created
		// until the user's first turn (a TUI boot with no input writes nothing
		// to opencode.db — verified by a host-gated spike, MEM034), so the
		// capture is an ASYNC bounded poll launched here, never a spawn-time
		// read. It persists the discovered id to tasks.opencode_session_id,
		// which is the restart-resume key (T02's resume argv reads it). Resume
		// reuses the stored id (no discovery) — only a FRESH opencode spawn
		// captures. Best-effort + warn-only: a capture failure costs only
		// restart-resume, never the live session. Never block the reply.
		if agentEngine == "opencode" && !req.Resume {
			go captureOpencodeSessionAsync(h.db, sess.Done(), req.TaskID, opts.Cwd)
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

// input handles POST /api/sessions/{id}/input — writes a message to the
// session's PTY via the existing WriteInput primitive (the same code path
// the browser WS handler uses at internal/ws/handler.go:164). This is the
// server-side half of the v1.11 D-14 read-only-terminal REVERSAL for the
// agent delegate surface: the MCP bridge reaches the PTY-write primitive
// through this HTTP endpoint, NOT through the WS frame protocol. The browser
// WS interactive surface is unchanged.
//
// The message is sent to the PTY as TWO SEPARATE WriteInput calls:
//  1. The body wrapped in ANSI bracketed paste markers (ESC[200~<body>ESC[201~).
//  2. A single "\r" submit key, written AFTER the closing bracket.
//
// The bracketed wrap is the deterministic fix. Raw-mode TUIs (Claude Code
// via Ink, opencode, anything readline/bubbletea-based with bracketed paste
// enabled) run a heuristic paste detector on stdin chunks; programmatic
// writes arrive at PTY-read speed (far faster than human typing), so the
// heuristic fires on essentially any write and absorbs any embedded "\r" as
// paste content rather than the Enter key. The explicit brackets tell the
// TUI "this is one paste event", bypassing the heuristic; the trailing "\r"
// lands OUTSIDE the bracket as a standalone Enter keystroke that submits the
// captured paste. See wrapInputForWrite for the full rationale + the
// iterative diagnosis trail. No quoting, escaping, or content sanitization
// — verbatim passthrough (the brackets are transport, not content).
func (h *sessionHandlers) input(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body, submit := wrapInputForWrite(req.Message)
	if err := sess.WriteInput([]byte(body)); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	time.Sleep(100 * time.Millisecond)
	if err := sess.WriteInput([]byte(submit)); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"bytes_written": len(body) + len(submit)})
}

// wrapInputForWrite decomposes a request message into the ordered pair of
// WriteInput payloads the input handler sends to the PTY: the message (any
// request-supplied trailing terminator stripped) wrapped in ANSI bracketed
// paste markers (ESC[200~ ... ESC[201~) as the FIRST payload, then a single
// "\r" submit key as the SECOND. The handler writes them as two separate
// WriteInput calls, never one combined write.
//
// Raw-mode TUIs (Claude Code via Ink, opencode, anything readline/bubbletea-
// based with bracketed paste enabled) run a heuristic paste detector on stdin
// chunks: a single programmatic write arrives at PTY-read speed (microseconds,
// far faster than human typing), so the heuristic fires on essentially any
// write and buffers the chunk for paste-preview. Any "\r" embedded in that
// chunk is consumed as paste CONTENT rather than the Enter key — the prompt
// text lands but never submits. The explicit bracketed paste sequence tells
// the TUI "this is one paste event, here is where it starts and ends",
// bypassing the heuristic entirely. The trailing "\r" is written as a
// SEPARATE call AFTER the closing bracket so the TUI reads it as a standalone
// Enter keystroke and submits the captured paste. This is the canonical
// "paste and submit" sequence every modern terminal emulator uses.
//
// "\r" (carriage return, 0x0d) is the submit key, not "\n": raw-mode TUIs
// read "\r" as Enter and treat "\n" as a literal line feed that does not
// submit. Internal "\n"s in a multi-line body are preserved verbatim — only
// the trailing terminator is stripped here and re-added as a standalone "\r"
// write by the caller.
//
// Iterative diagnosis trail: 260728-s5a established the "\r" (not "\n")
// submit key; 260728-sm5 split the body and submit key into two WriteInput
// calls; 260728-t4c added the bracketed paste wrap (the deterministic fix —
// the split alone was necessary but insufficient, as the body chunk still
// tripped the heuristic without explicit brackets). Verified live against
// Claude Code v2.1.22 / Opus 5 on session 70e5cae0-cb84-41f2-8c14-3f2897e7ac61:
// an idle agent transitions to working within seconds once the bracketed
// body + standalone CR land. See quick-task 260728-t4c SUMMARY.
func wrapInputForWrite(msg string) (body, submit string) {
	msg = strings.TrimSuffix(msg, "\n")
	msg = strings.TrimSuffix(msg, "\r")
	// The xterm bracketed-paste spec: the MODE a terminal enables is CSI ?2004
	// h, but the paste DELIMITERS it sends back are CSI 200 ~ / CSI 201 ~ —
	// 200/201, never 2004 (the original constants confused the two, leaking a
	// stray '~' into readline command lines — every plain-bash submission
	// through this endpoint parsed as "~<cmd>" and failed; caught by the
	// Phase-15 cwd-proof test).
	const (
		pasteStart = "\x1b[200~" // ESC[200~ — bracketed paste start (6 bytes)
		pasteEnd   = "\x1b[201~" // ESC[201~ — bracketed paste end (6 bytes)
	)
	return pasteStart + msg + pasteEnd, "\r"
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

// --- opencode session-id discovery (M002/S03/T03, Strategy B) ---------------
//
// opencode owns + mints its own opaque ses_… id (stored in
// ~/.local/share/opencode/opencode.db, keyed by the session's directory = the
// worktree path). Unlike claude — where kamacu mints --session-id <uuid> and
// always knows it — kamacu must DISCOVER opencode's id. Strategy A (capture
// the id from the plugin's session.created hook) was ruled out by a host-gated
// spike (MEM034): opencode 1.17.15 does NOT fire session.created for a TUI
// spawn. Strategy B discovers the id by parsing `opencode session list
// --format json` and filtering on directory == worktree.
//
// The session row only appears AFTER the user's first turn (a TUI boot with no
// input writes nothing to opencode.db — verified empirically), so discovery is
// an ASYNC bounded poll, never a spawn-time read (the async-capture-timing
// pitfall: asserting a non-empty id immediately after Spawn would race).

// discoverOpenCodeSession returns the opencode ses_… id whose directory matches
// dir, choosing the most-recently-updated one, or "" if none. It is an
// injectable package var so CI tests STUB it (no real opencode binary in CI);
// the default implementation shells out to the real `opencode session list`.
//
// Two opencode behaviors make the cwd + $PWD pinning load-bearing (MEM035/MEM036):
//   - opencode resolves a session's `directory` from $PWD, not getcwd().
//   - `opencode session list` is scoped to the CURRENT project (resolved from
//     $PWD), so it lists only the worktree's sessions — NOT every session on
//     the host. Running it without PWD=worktree queries the wrong project.
//
// Both cmd.Dir and PWD are pinned to the worktree so the listing is scoped to
// the right project AND the recorded directory matches the filter.
var discoverOpenCodeSession = func(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "opencode", "session", "list", "--format", "json")
	cmd.Dir = dir                       // scope the listing to the worktree's project
	cmd.Env = append(os.Environ(), "PWD="+dir) // opencode resolves the project from $PWD
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("opencode session list: %w", err)
	}
	return parseOpenCodeSessionList(out, dir)
}

// opencodeSessionEntry is the JSON shape of one element of `opencode session
// list --format json` (verified against opencode 1.17.15). directory is the
// session's worktree path (== kamacu's spawn cwd), so it is the stable
// per-task key across a restart; updated/created are ms-epoch.
type opencodeSessionEntry struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Updated   int64  `json:"updated"`
	Created   int64  `json:"created"`
	ProjectID string `json:"projectId"`
	Directory string `json:"directory"`
}

// parseOpenCodeSessionList returns the most-recently-updated opencode session id
// whose directory matches dir, or "" if none. Tolerant of the "no sessions"
// shapes opencode emits (empty output, [] , null). An unexpected non-array JSON
// shape is an error so the caller logs it (rather than silently returning "",
// which would look identical to "no session yet" and hide a parser drift).
func parseOpenCodeSessionList(out []byte, dir string) (string, error) {
	if len(bytes.TrimSpace(out)) == 0 {
		return "", nil // opencode prints nothing when there are zero sessions
	}
	var entries []opencodeSessionEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return "", fmt.Errorf("parse opencode session list json: %w", err)
	}
	var bestID string
	var bestUpdated int64
	for _, e := range entries {
		if e.ID == "" || e.Directory != dir {
			continue
		}
		// Most-recently-updated wins: a manual opencode in the same worktree
		// would otherwise let an older id resume the wrong conversation.
		if e.Updated > bestUpdated {
			bestID = e.ID
			bestUpdated = e.Updated
		}
	}
	return bestID, nil
}

// captureOpencodeSessionAsync polls opencode's session DB until the freshly
// spawned opencode session's ses_id appears (after the user's first turn) and
// persists it to tasks.opencode_session_id. It runs in its own goroutine
// launched by the create handler for a fresh opencode spawn.
//
// Exit conditions: discovery succeeds (id persisted + return); the session
// exits (done fires — the opencode row persists in opencode.db keyed by
// directory, so one final best-effort capture handles the case where the turn
// ran after the active window); or the hard cap elapses (paranoia backstop —
// done should always fire). The active window bounds subprocess churn (a poll
// is one `opencode session list` invocation); after it, the goroutine idles
// (blocked on the session's done channel) until the on-exit final attempt.
// All failures are warn-only: capture is best-effort and must never affect the
// live session.
func captureOpencodeSessionAsync(db *sql.DB, done <-chan struct{}, taskID int64, worktreeDir string) {
	const (
		pollInterval = 2 * time.Second
		activeWindow = 10 * time.Minute // bounds polling churn; covers realistic first-turn latency
		hardCap      = 60 * time.Minute // goroutine-lifetime backstop; done should always fire first
	)
	// attempt runs ONE discovery + persist. Returns true once captured (caller stops).
	attempt := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		id, err := discoverOpenCodeSession(ctx, worktreeDir)
		if err != nil {
			slog.Warn("discovering opencode session id", "task", taskID, "error", err)
			return false
		}
		if id == "" {
			return false // no session row yet (the user hasn't run a turn)
		}
		if _, err := db.Exec(`UPDATE tasks SET opencode_session_id = ? WHERE id = ?`, id, taskID); err != nil {
			slog.Warn("persisting opencode session id", "task", taskID, "error", err)
			return false
		}
		slog.Info("captured opencode session id", "task", taskID, "opencode_session_id", id)
		return true
	}

	active := time.NewTimer(activeWindow)
	defer active.Stop()
	hard := time.NewTimer(hardCap)
	defer hard.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Immediate first attempt (cheap; occasionally the turn already ran).
	if attempt() {
		return
	}
	for {
		select {
		case <-done:
			// Session exited. The opencode row persists in opencode.db, so one
			// final capture handles a turn that ran late (even after the active
			// window). If discovery still finds nothing, the user never ran a
			// turn — there is genuinely nothing to resume.
			attempt()
			return
		case <-hard.C:
			return // backstop; done() should have fired
		case <-active.C:
			// Active window elapsed: stop polling, idle until the session exits
			// (an idle goroutine is free), then make the final capture attempt.
			ticker.Stop()
			select {
			case <-done:
				attempt()
				return
			case <-hard.C:
				return
			}
		case <-ticker.C:
			if attempt() {
				return
			}
		}
	}
}

// --- Phase 08 read-only endpoints (MCPSESS-01/02/03 server-side) -------------
//
// These handlers back the MCP bridge's session tools. They consume ONLY
// session.Info/Snapshot — never the PTY-write primitive (D-14). The bridge
// (separate process) crosses the loopback boundary to read session state + the
// ring snapshot here; the type-level read-only contract is proven by the
// scoped grep gate (zero references to the write primitive below).

// defaultOutputBytes is the default byte count returned by get_session_output
// when ?bytes is absent or empty (D-05). 4 KiB is enough for an MCP agent to
// read the recent tail without paying the full 512 KiB cost on every poll.
const defaultOutputBytes = 4096

// maxOutputBytes caps get_session_output's ?bytes value (D-06 / Pitfall 8 /
// T-08-02): the ring is 1 MiB, but the bridge's maxBodyBytes ceiling is 1 MiB
// and base64 inflates by ~4/3, so a 512 KiB slice yields a ~683 KiB envelope
// that fits comfortably under the cap.
const maxOutputBytes = 512 * 1024

// sessionDetail is the wire shape for the read endpoints: it embeds
// session.Info (so every existing JSON tag flows through unchanged) and adds
// the D-10 task->project->agent JOIN fields taskTitle/projectName/agentName.
// Dev sessions (TaskID==0) keep empty JOIN fields (omitted on the wire via
// omitempty).
type sessionDetail struct {
	session.Info
	TaskTitle   string `json:"taskTitle,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
	AgentName   string `json:"agentName,omitempty"`
}

// sessionOutputEnvelope is the D-08 snapshot shape returned by
// get_session_output: base64-encoded last-N ring bytes plus a clamped flag.
type sessionOutputEnvelope struct {
	Encoding string `json:"encoding"` // always "base64" — PTY output is arbitrary bytes; text frames would corrupt split UTF-8
	Output   string `json:"output"`
	Bytes    int    `json:"bytes"`    // the number of raw PTY bytes encoded (≤ requested)
	Clamped  bool   `json:"clamped"`  // true when the requested bytes value exceeded maxOutputBytes
}

// sessionContext bundles the per-task DB columns attached to a session.Info
// for the read endpoints' wire shape (D-10): the task's title, the project's
// name, and the agent's name. Empty when the task row is missing from the DB
// (a session outlived its task row) — mirrors agents.go's `continue` posture.
type sessionContext struct {
	taskTitle   string
	projectName string
	agentName   string
}

// joinSessionContext loads taskTitle/projectName/agentName for each task ID
// present in infos in ONE query (D-11: never N round-trips per session). The
// shape mirrors the canonical tasks->projects->agents JOIN at agents.go:99.
// A query failure degrades to empty fields (the read path stays usable);
// entries whose taskID is missing from the DB (deleted under a still-tracked
// session) also get zero values. Returns a map keyed by task ID. Dev sessions
// (TaskID==0) are skipped and never reach the DB.
func (h *sessionHandlers) joinSessionContext(infos []session.Info) map[int64]sessionContext {
	out := make(map[int64]sessionContext, len(infos))
	ids := make([]any, 0, len(infos))
	for _, info := range infos {
		if info.TaskID <= 0 {
			continue
		}
		if _, seen := out[info.TaskID]; seen {
			continue
		}
		out[info.TaskID] = sessionContext{}
		ids = append(ids, info.TaskID)
	}
	if len(ids) == 0 {
		return out
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	rows, err := h.db.Query(
		`SELECT t.id, t.title, p.name, a.name
		 FROM tasks t
		 JOIN projects p ON p.id = t.project_id
		 JOIN agents a ON a.id = p.agent_id
		 WHERE t.id IN (`+placeholders+`)`, ids...)
	if err != nil {
		slog.Warn("joinSessionContext: query", "error", err)
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var (
			tid      int64
			ctx      sessionContext
			projName sql.NullString
			agName   sql.NullString
		)
		if err := rows.Scan(&tid, &ctx.taskTitle, &projName, &agName); err != nil {
			slog.Warn("joinSessionContext: scan", "error", err)
			return out
		}
		ctx.projectName = projName.String
		ctx.agentName = agName.String
		out[tid] = ctx
	}
	if err := rows.Err(); err != nil {
		slog.Warn("joinSessionContext: rows", "error", err)
	}
	return out
}

// getSession handles GET /api/sessions/{id} (MCPSESS-02 server-side). Returns
// session.Info plus the D-10 JOIN fields for a live OR exited in-memory
// session (D-12: Snapshot/Info survive exit). Unknown ids return 404.
func (h *sessionHandlers) getSession(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	info := sess.Info()
	// Global entries get the honest synthesized labels (GINT-02) — same
	// per-entry gating as the list loop, never a 0-keyed ctxByTask lookup.
	if info.Global {
		gctx := h.globalSessionContext()
		writeJSON(w, http.StatusOK, sessionDetail{
			Info:        info,
			TaskTitle:   "Scratchpad",
			ProjectName: "Global",
			AgentName:   gctx.agentName,
		})
		return
	}
	ctxByTask := h.joinSessionContext([]session.Info{info})
	ctx := ctxByTask[info.TaskID]
	writeJSON(w, http.StatusOK, sessionDetail{
		Info:        info,
		TaskTitle:   ctx.taskTitle,
		ProjectName: ctx.projectName,
		AgentName:   ctx.agentName,
	})
}

// getSessionOutput handles GET /api/sessions/{id}/output?bytes=N (MCPSESS-03
// server-side). Returns a D-08 JSON envelope with the LAST N ring bytes
// base64-encoded. bytes defaults to defaultOutputBytes and is clamped to
// [1, maxOutputBytes]; clamped=true when the requested value exceeded the cap
// (T-08-02). Unknown ids return 404.
func (h *sessionHandlers) getSessionOutput(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Parse bytes: default when absent/empty, clamp to [1, maxOutputBytes].
	// An unparseable value is a 400 — same posture as the list filter.
	want := defaultOutputBytes
	clamped := false
	if raw := r.URL.Query().Get("bytes"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "invalid bytes")
			return
		}
		if parsed > maxOutputBytes {
			clamped = true
			parsed = maxOutputBytes
		}
		if parsed == 0 {
			parsed = 1 // clamp the floor at 1 — a 0-byte slice is never useful
		}
		want = parsed
	}
	// Snapshot is read-only on the existing 1 MiB ring — no new long-lived
	// state. The last-N slice is the tail; base64 handles arbitrary bytes
	// safely (a text frame would corrupt split multi-byte PTY sequences).
	snap := sess.Snapshot()
	start := len(snap) - want
	if start < 0 {
		start = 0
	}
	lastN := snap[start:]
	writeJSON(w, http.StatusOK, sessionOutputEnvelope{
		Encoding: "base64",
		Output:   base64.StdEncoding.EncodeToString(lastN),
		Bytes:    len(lastN),
		Clamped:  clamped,
	})
}

// defaultSubscribeDuration is the default bound on a subscribe stream when
// ?duration_seconds is absent (MCPSESS-04). 30s matches the agent's typical
// poll cadence without holding a request open indefinitely.
const defaultSubscribeDuration = 30 * time.Second

// maxSubscribeDuration caps the subscribe stream duration (MCPSESS-04 /
// T-08-03): a misbehaving bridge can never pin a goroutine on this handler
// past 5 minutes; the SDK ctx cancel is the real backstop on the bridge side.
const maxSubscribeDuration = 300 * time.Second

// subscribeSessionOutput handles GET /api/sessions/{id}/subscribe
// (MCPSESS-04 server-side). It streams raw PTY octets as
// application/octet-stream for a bounded duration (default 30s, cap 300s),
// drains the ring replay when include_history is absent (D-02), and Detaches
// on EVERY return path (SC3) — including client disconnect.
//
// Read-only contract (D-14): the handler consumes ONLY session.Attach/Detach/
// Done — never the PTY-write primitive, never the WS frame protocol. It is
// plain HTTP chunked output. The handler is the per-request reader; no
// goroutine launched here outlives the request (milestone rule: no new
// long-lived/background goroutines inside Kamacu).
//
// Mirrors internal/ws/handler.go's attach->stream->detach loop MINUS the
// readLoop (this is the read-only half: no WS PTY-input frame, no PTY-write
// call, no Resize — the bridge is read-only).
func (h *sessionHandlers) subscribeSessionOutput(w http.ResponseWriter, r *http.Request) {
	sess, ok := h.mgr.Get(r.PathValue("id"))
	if !ok {
		// 404 BEFORE writing any stream bytes — the bridge reads status + body.
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	// Parse duration_seconds: default when absent; clamp to [1s, maxSubscribeDuration].
	// A parse error or a negative value is a 400 — same posture as the other readers.
	duration := defaultSubscribeDuration
	if raw := r.URL.Query().Get("duration_seconds"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds < 0 {
			writeError(w, http.StatusBadRequest, "invalid duration_seconds")
			return
		}
		if seconds < 1 {
			seconds = 1
		}
		d := time.Duration(seconds) * time.Second
		if d > maxSubscribeDuration {
			d = maxSubscribeDuration
		}
		duration = d
	}

	// include_history defaults false (D-02: live-only unless explicitly opted in).
	includeHistory := false
	if raw := r.URL.Query().Get("include_history"); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes":
			includeHistory = true
		}
	}

	// Headers FIRST: set the content type before WriteHeader. The body is raw
	// PTY octets (application/octet-stream) — never a WS frame, never JSON.
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)

	// http.Flusher may be nil on some ResponseWriter wrappers; guard every Flush.
	flusher, _ := w.(http.Flusher)
	if flusher != nil {
		flusher.Flush() // push the 200 + headers immediately
	}

	connID := uuid.NewString()
	q := sess.Attach(connID)
	// SC3: defer Detach runs on EVERY return path — client disconnect, duration
	// cap, queue close, or a panic recovery. Detach NEVER touches the PTY
	// (session.go:356) — it only removes the conn from the fan-out map.
	defer sess.Detach(connID)

	if !includeHistory {
		// D-02: drain the ring-replay first message under the same lock that
		// registered the queue (session.go:346) so only post-attach output
		// streams. Guard with r.Context().Done() so a client that disconnected
		// during replay-drain still returns promptly (defer Detach handles cleanup).
		select {
		case _, ok := <-q:
			if !ok {
				return // session exited before any replay — queue already closed
			}
		case <-r.Context().Done():
			return
		}
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()
	for {
		select {
		case chunk, ok := <-q:
			if !ok {
				// Queue closed = session exited (markExited session.go:192-194);
				// buffered output was already delivered before close.
				return
			}
			if _, err := w.Write(chunk); err != nil {
				// Client went away mid-write — defer Detach cleans up.
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		case <-timer.C:
			// Duration cap backstop (D-11 / T-08-03) — the SDK ctx cancel on
			// the bridge side is the real backstop, but the server enforces its
			// own bound regardless of bridge behavior.
			return
		case <-r.Context().Done():
			// Client/bridge closed the request — the cancellation path that
			// makes SC3 testable (defer Detach runs on this return).
			return
		}
	}
}
