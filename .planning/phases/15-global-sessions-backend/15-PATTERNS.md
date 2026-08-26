# Phase 15: Global sessions backend - Pattern Map

**Mapped:** 2026-08-26
**Files analyzed:** 6 (5 modified + 1 new test file)
**Analogs found:** 6 / 6 — this is a composition phase: every new seam has an in-file task-scoped analog. No external patterns needed.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/session/manager.go` (modify) | service (session engine) | event-driven (PTY lifecycle) + in-memory registry | SAME FILE: `ListByTask`/`listWhere` (:414-448), `StopAllForTask` (:450-475), label switch (:331-345), `SpawnOpts` zero-value fields (:54-80) | exact (in-file task variant) |
| `internal/session/session.go` (modify) | model | event-driven | SAME FILE: `Orphaned`/`TmuxName` additive `Info` fields (:59-68) + `Session` fields (:79-118) | exact |
| `internal/api/sessions.go` (modify) | controller | request-response (spawn/status/list) | SAME FILE: `create` task path (:223-496), `reconcileTmux` (:144-214), `joinSessionContext` (:917-964), `captureOpencodeSessionAsync` (:793-858) | exact |
| `internal/api/agents.go` (modify) | controller | request-response (read aggregation, 5s poll) | SAME FILE: manager-derived pass (:44-152) + DB-derived synthesized pass (:154-218) | exact |
| `internal/api/global.go` (modify) | controller | request-response + CRUD (config singleton) | SAME FILE: tmux half of `liveGlobalTmuxNames`/`globalLiveBlockers` (:88-129), `deriveGlobalState` zero-seams (:223-234) | exact |
| `internal/api/sessions_global_test.go` (NEW — name illustrative; may instead be additions to existing suites) | test | request-response (integration) | `internal/api/agent_integration_test.go` (full harness + stage pattern + fake-claude) | exact |

**No new routes, no new migrations** — `POST /api/sessions` gains a `scope` body field (D-28..D-34 widen `create` in place); migrations 00017/00018 already landed the schema this phase writes against.

---

## Pattern Assignments

### `internal/session/manager.go` (service — engine scope flag, counter, ListGlobal, StopAllForScope)

**Analog:** the file's own task-scoped surface.

**Zero-value additive field pattern** — `SpawnOpts` (:53-80). `Global bool` rides the exact `Kind`/`Shell`/`TmuxName` precedent (zero value = current behavior):

```go
// manager.go:54-79 (excerpt — the pattern to extend)
type SpawnOpts struct {
	Cwd    string // "" -> user home (preserves Phase 2 /terminal dev behavior)
	TaskID int64  // 0 -> unscoped dev session, label "bash #N" (global counter)
	Kind   Kind   // zero value = KindBash (full Phase 2/3 backward compatibility)
	// ...
	Shell string // bash-only: "" keeps the $SHELL fallback (back-compat)
	TmuxName string // bash-only: "" = plain shell. Mutually exclusive with Shell.
}
// NEW: Global bool — "" (false) = task/dev scope, exactly as researched.
```

**Label switch — the load-bearing ordering** (:331-345). The `opts.Global` arm MUST be inserted BEFORE the `opts.TaskID == 0` dev arm (Pitfall 2 — else global bash tabs get `bash #N` dev labels):

```go
// manager.go:331-345 (verbatim — insert a Global arm between :337 and :338)
m.mu.Lock()
m.seq++
seq := m.seq
var label string
switch {
case kind == KindAgent:
	label = "Agent" // one agent per task — no counter (API enforces)
case opts.TaskID == 0:            // <-- Global arm goes ABOVE this line
	m.counter++
	label = fmt.Sprintf("bash #%d", m.counter)
default:
	m.taskCounters[opts.TaskID]++
	label = fmt.Sprintf("Bash %d", m.taskCounters[opts.TaskID])
}
m.mu.Unlock()
```

Counter wiring: add `globalCounter int` next to `counter`/`taskCounters` in the `Manager` struct (:35-43) — same monotonic never-reused contract.

**ListGlobal analog** — `ListByTask`/`listWhere` (:414-448). Same comparator contract, predicate swapped to `s.global`:

```go
// manager.go:418-423 (verbatim shape to mirror)
func (m *Manager) ListByTask(taskID int64) []Info {
	return m.listWhere(func(s *Session) bool { return s.taskID == taskID })
}
// NEW: ListGlobal() = listWhere(func(s *Session) bool { return s.global })
```

**StopAllForScope analog** — `StopAllForTask` (:450-475). Same collect-under-lock → concurrent Stop → Wait shape with the predicate swapped. **NEVER call `StopAllForTask(0)`** — it matches every dev terminal (GSESS-01 landmine):

```go
// manager.go:456-475 (verbatim shape — swap `s.taskID == taskID` for `s.global`)
func (m *Manager) StopAllForTask(taskID int64) {
	m.mu.Lock()
	targets := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		if s.taskID == taskID && s.Info().Status == StatusRunning {
			targets = append(targets, s)
		}
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, s := range targets {
		wg.Add(1)
		go func(s *Session) { defer wg.Done(); s.Stop() }(s)
	}
	wg.Wait()
}
```

**Session construction** (:347-365): add `global: opts.Global` to the struct literal — one line, next to `taskID: opts.TaskID` (:350).

**DO NOT touch** the engine's claude/custom argv arms (:152-243), PTY/ring/attach machinery, or `Spawn`'s cwd validation (:136-143) — task paths must diff byte-for-byte (Pitfall 9). `Spawn` is already cwd-driven; `Global` is opaque to everything except labels/listing/stopping.

---

### `internal/session/session.go` (model — `global` field + `Info.Global`)

**Analog:** the `Orphaned`/`TmuxName` additive-field pattern on `Info` (:59-68).

```go
// session.go:44-68 (excerpt — the extension point)
type Info struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	Status    Status    `json:"status"`
	ExitCode  *int      `json:"exitCode,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	TaskID    int64     `json:"taskId,omitempty"` // 0 omitted for dev sessions
	Kind      Kind      `json:"kind,omitempty"`
	Engine    string    `json:"engine,omitempty"`
	// ...
	Orphaned bool   `json:"orphaned,omitempty"` // <- additive-field precedent
	TmuxName string `json:"tmuxName,omitempty"`
}
// NEW: Global bool `json:"global,omitempty"` — zero value omits for task/dev
// sessions, exactly like TaskID/Kind/Orphaned before it.
```

**Session struct** (:79-91): add unexported `global bool` next to `taskID int64` (:82, "immutable after Spawn" — same contract). Populate it in `Info()` (:199-224) alongside `TaskID: s.taskID` (:211).

---

### `internal/api/sessions.go` (controller — the spawn path being widened)

**Analog:** the task path of `create` itself (:223-496). The global branch slots in exactly where the worktree query feeds every kind today.

**Request body** (:224-234): add `Scope string \`json:"scope"\`` to the existing anonymous struct — no new endpoint (anti-pattern: side-channel spawn route forks the engine-branched logic). Validate against the closed set at the kind switch (:239-247): unknown value → 400 "invalid scope" (the `invalid kind` family). `scope:"global"` + non-zero `task_id` → 400 "supply either" family (global.go:288 copy).

**Gate block — the D-28..D-33 template** (:267-304). The task path's worktree resolution is the exact structural analog; the global branch is a singleton SELECT + os.Stat instead of the task JOIN:

```go
// sessions.go:282-304 (verbatim task analog)
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
}
// GLOBAL branch (new): SELECT g.root_path, g.agent_id, g.claude_session_id,
//   g.opencode_session_id FROM global_task g JOIN agents a ON a.id = g.agent_id
//   WHERE g.id = 1  (read-at-use, D-24; mirrors loadGlobalConfig global.go:198-215)
// Gate order (D-33): root_path == "" → 409 "global root not configured" (D-28);
//   os.Stat fails/not-dir → 409 "global root no longer exists on disk: <path>"
//   (D-29, path verbatim); THEN one-agent gate; then opts.Cwd, opts.Global set.
```

**One-agent gate analog** (:305-315) — D-34 mirrors the copy voice:

```go
// sessions.go:308-315 (verbatim task gate — swap ListByTask for ListGlobal)
if kind == session.KindAgent {
	for _, info := range h.mgr.ListByTask(req.TaskID) {
		if info.Kind == session.KindAgent && info.Status == session.StatusRunning {
			writeError(w, http.StatusConflict, "agent session already running")
			return
		}
	}
}
// GLOBAL: "global agent already running" — only RUNNING blocks (SC1)
```

**Engine-branched resume validation analog** (:316-339) — D-31's 409 mirrors this posture exactly (never silent fresh spawn):

```go
// sessions.go:323-338 (verbatim)
if req.Resume {
	if agentEngine == "opencode" {
		if !ocsid.Valid {
			writeError(w, http.StatusConflict, "no session to resume")
			return
		}
		// opts.ResumeSessionID stays "" — the `-s <id>` flag is appended to
		// AgentArgs in the custom spawn arm below (:371-373).
	} else {
		if !csid.Valid || !transcriptExists(h.globRoot, csid.String) {
			writeError(w, http.StatusConflict, "no session to resume")
			return
		}
		opts.ResumeSessionID = csid.String
	}
}
// GLOBAL: 409 "no global <engine> session to resume" — csid/ocsid read from
// the singleton, transcriptExists is cwd-agnostic (resume.go:16) so it works
// verbatim against the global root.
```

**Reattach scoped-lookup analog** (:375-394) — D-32 is this query with the WHERE clause scoped:

```go
// sessions.go:385-389 (verbatim task-scoped lookup)
err := h.db.QueryRow(`SELECT label FROM tmux_sessions WHERE task_id = ? AND name = ?`, req.TaskID, req.ReattachTmuxName).Scan(&reattachLabel)
if errors.Is(err, sql.ErrNoRows) {
	writeError(w, http.StatusNotFound, "no session to reattach")
	return
}
// GLOBAL: WHERE scope = 'global' AND name = ? — a foreign (task) row simply
// doesn't exist in this scope → the same 404. No scope-mismatch branch.
```

**tmux mint analog** (:403-436) — scope-scoped counter + INSERT-before-Spawn + `kamacu-global-<n>`:

```go
// sessions.go:417-431 (verbatim task mint)
var n int64
if err := h.db.QueryRow(`SELECT COALESCE(MAX(n),0)+1 FROM tmux_sessions WHERE task_id = ?`, req.TaskID).Scan(&n); err != nil {
	writeError(w, http.StatusInternalServerError, "couldn't start a session")
	return
}
name := fmt.Sprintf("kamacu-%d-%d", req.TaskID, n)
label := fmt.Sprintf("Bash %d", n)
if _, err := h.db.Exec(`INSERT INTO tmux_sessions (task_id, n, name, label) VALUES (?, ?, ?, ?)`, req.TaskID, n, name, label); err != nil {
	writeError(w, http.StatusInternalServerError, "couldn't start a session")
	return
}
opts.TmuxName = name
// GLOBAL: counter WHERE scope = 'global'; name = fmt.Sprintf("kamacu-global-%d", n);
// INSERT (NULL, 'global', n, name, label) — the 00018 XOR CHECK
// (task_id IS NULL) = (scope = 'global') requires both to agree.
// Real race backstop is name UNIQUE (Pitfall 6 — NULL task_ids are DISTINCT
// under UNIQUE(task_id,n)). Plain bash: settings shell read verbatim (:398).
```

`defaultTmuxLabel` (:610-622) parses the LAST `-` segment → `defaultTmuxLabel("kamacu-global-3")` == "Bash 3" — works unchanged.

**Spawn failure release** (:437-457): the DELETE-on-failed-spawn release (:444-448) and `ErrTmuxNotFound` 409 (:449-454) apply to global rows verbatim (`WHERE name = ?` is scope-agnostic).

**csid persist + capture launch** (:458-478) — re-target the UPDATE, parametrize the poller (never fork it — Pitfall 8):

```go
// sessions.go:460-477 (verbatim task path)
if kind == session.KindAgent {
	if _, err := h.db.Exec(`UPDATE tasks SET claude_session_id = ? WHERE id = ?`, sess.ClaudeSessionID(), req.TaskID); err != nil {
		slog.Warn("persisting claude_session_id", "task", req.TaskID, "error", err) // warn-only (Pattern 5)
	}
	if agentEngine == "opencode" && !req.Resume {
		go captureOpencodeSessionAsync(h.db, sess.Done(), req.TaskID, opts.Cwd)
	}
}
// GLOBAL: UPDATE global_task SET claude_session_id = ? WHERE id = 1;
// capture launch passes a global owner marker (owner enum or persist closure)
// so the poller's UPDATE targets global_task — :476 is the ONLY call site.
```

**captureOpencodeSessionAsync** (:793-858): only the persist statement changes (:811 `UPDATE tasks SET opencode_session_id...`); the poll/timing/exit conditions and `discoverOpenCodeSession` (:726-735 — `cmd.Dir` + `PWD=dir`, both cwd-parameterized) are unchanged. `parseOpenCodeSessionList`'s directory filter (:766) and most-recently-updated-wins (:771-774) work verbatim against the global root.

**reconcileTmux global variant** (:144-214): same survivor/GC logic, `WHERE scope = 'global'` instead of `WHERE task_id = ?`; the synthesized `session.Info{...Orphaned: true, TmuxName: rw.name}` entry (:196-204) gains `Global: true` and the empty-label fallback goes through `defaultTmuxLabel` (:190) exactly as tasks do.

**list `?scope=global`** (:65-134): new filter branch in the existing chain (task_id :67 / project_id :71 / default :104) → `mgr.ListGlobal()` + global reconcile. **Pitfall 3 — the context application loop** (:127-132):

```go
// sessions.go:127-132 (verbatim — the leak point)
ctxByTask := h.joinSessionContext(infos)
out := make([]sessionDetail, len(infos))
for i, info := range infos {
	ctx := ctxByTask[info.TaskID]        // <-- key 0 would leak onto dev sessions
	out[i] = sessionDetail{Info: info, TaskTitle: ctx.taskTitle, ...}
}
// GLOBAL: synthesize the context per-entry gated on info.Global (taskTitle
// "Scratchpad", projectName "Global", agentName from the singleton JOIN) —
// NEVER insert a 0-keyed entry into the map. Same discipline at getSession
// (:976-977). joinSessionContext itself (:917-923) already skips TaskID <= 0.
```

---

### `internal/api/agents.go` (controller — both status passes widened)

**Analog:** the file's own two passes; the DB-derived synthesized entry (:201-213) is the exact shape precedent for BOTH new global entries.

**Wire contract** (:26-38) — `Source` union extends with `"global"`; zero ids keep TS non-nullable:

```go
// agents.go:26-38 (verbatim — the contract Spike 1 settles)
type agentStatusEntry struct {
	TaskID        int64  `json:"taskId"`      // global entry: 0 (rowids start at 1)
	ProjectID     int64  `json:"projectId"`   // global entry: 0
	SessionID     string `json:"sessionId"`
	Status        string `json:"status"`
	ExitCode      *int   `json:"exitCode"`
	StopRequested bool   `json:"stopRequested"`
	Resumable     bool   `json:"resumable"`
	PRNumber      *int64 `json:"prNumber"`    // global entry: nil
	Source        string `json:"source"`      // "manual" | "github_pr" | +"global"
	TaskTitle     string `json:"taskTitle"`   // global entry: "Scratchpad" (D-09)
	ProjectName   string `json:"projectName"` // global entry: "Global" (D-10)
}
```

**Manager pass** (:46-58) — the `TaskID <= 0` filter at :50 already skips global sessions; collect the newest global agent OUTSIDE the `newest[TaskID]` map (Pitfall 3):

```go
// agents.go:49-58 (verbatim loop — global sessions are invisible here by design)
for _, info := range a.mgr.List() {
	if info.Kind != session.KindAgent || info.TaskID <= 0 {
		continue
	}
	// ...newest[info.TaskID] map fill...
}
// NEW Pass 1b: separate scan for Kind==agent && Global → newest wins → ONE
// synthesized entry appended to entries. Resumable derivation mirrors
// :135-137 but root_path plays worktree's role (Pitfall 10 — NEVER reference
// worktree_path): engine == "opencode" && ocsid.Valid || csid.Valid &&
// transcriptExists(...) — ids read from the global_task singleton.
```

**DB-derived pass entry shape** (:201-213) — the post-restart global entry (Pass 2b) copies this verbatim with global labels, gated on "no manager entry" and reading `global_task` csid/ocsid:

```go
// agents.go:201-213 (verbatim task precedent — the shape to synthesize)
entries = append(entries, agentStatusEntry{
	TaskID:      id,
	ProjectID:   pid,
	SessionID:   "",
	Status:      "exited",
	ExitCode:    nil,
	StopRequested: false,
	Resumable:   true,
	PRNumber:    prNumberOf(prNumber),
	Source:      source,
	TaskTitle:   title,
	ProjectName: projectName,
})
```

**Never** rewrite the INNER JOINs (:99-103, :164-168) — append synthesized entries only (Pattern 2). Error handling: `writeError(w, http.StatusInternalServerError, err.Error())` on every query/scan failure, `writeJSON(w, http.StatusOK, entries)` at the end (:219).

---

### `internal/api/global.go` (controller — counts become real, gate widens)

**Analog:** the file's own tmux half — the manager half mirrors it via `ListGlobal()`.

```go
// global.go:123-129 (verbatim — the helper this phase widens)
func (h *globalHandlers) globalLiveBlockers(ctx context.Context) []deleteBlocker {
	var blockers []deleteBlocker
	for _, name := range h.liveGlobalTmuxNames(ctx) {
		blockers = append(blockers, deleteBlocker{Kind: "sessions", Target: name})
	}
	return blockers
}
// WIDEN: after the tmux loop, iterate mgr.ListGlobal() — any RUNNING session
// appends a deleteBlocker (GCONF-04 must bite once global PTYs exist —
// Pitfall 4). The mgr field is already carried (:63-67) for exactly this seam.

// global.go:229-233 (verbatim — the zero-seams D-19 forward-wired)
// tmux half is REAL today (00018 rows + exact-match probe). The manager
// half is a zero-count seam until Phase 15's ListGlobal() (D-19).
g.Live.Tmux = len(h.liveGlobalTmuxNames(ctx))
g.Live.Agent = 0 // Phase-15 ListGlobal() seam (D-19)   <- becomes real
g.Live.Bash  = 0 // Phase-15 ListGlobal() seam (D-19)   <- becomes real
// Derivation (Open Question 1): Agent = RUNNING KindAgent count; Bash =
// plain-bash RUNNING count (tmux counted separately — the fields are disjoint
// by name); Tmux unchanged.
```

`loadGlobalConfig` (:198-215) is the read-at-use singleton SELECT the spawn branch mirrors (Pattern 4).

---

### `internal/api/sessions_global_test.go` (NEW — test)

**Analog:** `internal/api/agent_integration_test.go` — the full-wiring harness + stage pattern + fake-claude stub.

**Harness** (:52-92): `newAgentIntegrationServer` wires every route family over one DB + manager with `AgentConfig.ClaudeBin` pointed at `testdata/fake-claude`. The global suite reuses it verbatim plus `GlobalRoutes` registration and a `PUT /api/global` seed step. Helpers to reuse from the same package: `doJSON`/`doJSONList` (projects_test.go:468/:501), `gitRepoWithCommit` (tasks_test.go:45 — temp repo as folder root), `agentStatusForTask`/`waitAgentStatus` (:110-139 — poll the 5s status feed).

**Stage pattern** (:165-170) — sequential stages with `t.FailNow` on failure:

```go
// agent_integration_test.go:165-170 (verbatim)
stage := func(name string, fn func(t *testing.T)) {
	t.Helper()
	if !t.Run(name, fn) {
		t.FailNow()
	}
}
```

**Skip-guard convention** — `exec.LookPath` guard per capability (agent_integration_test.go:147-149 for git; sessions_test.go:673 for tmux; the host-gated opencode e2e gates on `exec.LookPath("opencode")`):

```go
// agent_integration_test.go:147-149 (verbatim shape)
if _, err := exec.LookPath("git"); err != nil {
	t.Skip("git not on PATH")
}
```

Coverage map (from RESEARCH.md Test Strategy): D-28/D-29/D-30 gates (CI, no tmux), D-31/D-32 (CI), D-33/D-34 one-agent 409 (fake-claude), GSESS-01/SC3 status entry live + post-restart, GSESS-02 restart-sim resume, GSESS-03 tmux survivor (tmux-guarded), GSESS-04/GINT-03 assert-by-construction, GINT-02 MCP labels + Pitfall 3 dev-label regression, capture re-target (CI stub + one host-gated e2e), GCONF-04 bite, task-path byte-for-byte green.

---

## Shared Patterns

### 1. Zero-value additive scope field (back-compat)
**Source:** `manager.go` `SpawnOpts` (:54-80), `session.go` `Info` omitempty fields (:51-52, :66-67)
**Apply to:** `SpawnOpts.Global`, `Info.Global`, `Session.global` — false = current behavior; checked BEFORE the `TaskID == 0` dev arm everywhere (manager.go:338, sessions.go:921, agents.go:50).

### 2. One-gate-before-kind-dispatch with honest 409s
**Source:** `sessions.go` :262-304 (reattach task gate :262, agent task gate :269, worktree 409 :299-302)
**Apply to:** the global root gates (D-28 unconfigured / D-29 vanished / D-31 resume / D-34 one-agent) — ONE root gate block before kind dispatch, order per D-33. Copy voice: `"task has no worktree"` → `"global root not configured"`, `"global root no longer exists on disk: <path>"`, `"global agent already running"`.

### 3. Read-at-use singleton resolution
**Source:** `global.go` `loadGlobalConfig` (:198-215), task JOIN `sessions.go` :284-290
**Apply to:** the global spawn branch's root+agent+resume-id SELECT — fresh per request, agent changes apply at next Start (D-24), `ErrNoRows` = corrupted invariant → 500.

### 4. Warn-only persistence degradation
**Source:** `sessions.go` :461-463 (csid persist), :490-493 (tmux label back-fill), :446 (row release)
**Apply to:** global csid persist to `global_task`, global tmux label back-fill — `slog.Warn` non-fatal; a failed write costs resume/label only, never the live session.

### 5. Synthesized entries, never LEFT-JOIN surgery
**Source:** `agents.go` DB-derived append (:201-213), `reconcileTmux` orphaned `Info` synthesis (sessions.go :196-204)
**Apply to:** both status-pass global entries, `joinSessionContext` global labels (per-entry off the map — Pitfall 3), global reconcile entries. Labels are locked strings: `projectName:"Global"`, `taskTitle:"Scratchpad"`.

### 6. Engine-branched resume validation
**Source:** `sessions.go` :323-339, `agents.go` :135-137 / :192-193
**Apply to:** global resume — opencode keys on ocsid alone; claude on csid + `transcriptExists` (cwd-agnostic, resume.go:16). 409, never silent fresh spawn.

### 7. Response helpers
**Source:** `internal/api/respond.go` — `writeJSON` (:9), `writeError` (:16)
**Apply to:** all new branches. GCONF-04 blocker responses use the `writeJSON(w, 409, map[string]any{"error": ..., "reasons": blockers})` shape (global.go :312-315).

### 8. DB write targets (schema already landed — Phase 13)
**Source:** `internal/store/migrations/00017_global_task.sql` (:21-30 — `claude_session_id`/`opencode_session_id` columns, singleton `CHECK (id = 1)`), `00018_tmux_scope.sql` (:25-35 — `scope` CHECK, `name UNIQUE`, XOR CHECK)
**Apply to:** csid/ocsid persist → `UPDATE global_task ... WHERE id = 1`; global tmux INSERT → `(task_id NULL, scope 'global', n, name, label)`. **No migration files this phase.**

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| — | — | — | None. Every seam has an in-file task-scoped analog (composition phase). The only genuinely new micro-shapes — the `source:"global"` union value and the capture-poll persist-target parameter — are anchored to `agentStatusEntry.Source` (:35) and the single `captureOpencodeSessionAsync` call site (:476) respectively. |

## Anti-Patterns (from RESEARCH.md — planner must not emit these)

- Separate `/api/global/sessions` endpoint — widen `POST /api/sessions` with `scope`.
- `StopAllForTask(0)` anywhere in the global path — kills every dev terminal.
- A second copy of `captureOpencodeSessionAsync` — parametrize the persist target.
- LEFT-JOIN surgery on the hot status queries — append synthesized entries.
- Silent `$HOME` fallback on unconfigured root (D-28 forbids).
- Global label arm placed after the `TaskID == 0` dev arm (Pitfall 2) or a 0-keyed `ctxByTask` entry (Pitfall 3).
- A global resumable branch referencing `worktree_path` (Pitfall 10 — root_path plays that role).
- Raw `tmux has-session` execs — all probes via `tmux.Client.HasSession` (exact-match `=name`, tmux.go:78).

## Metadata

**Analog search scope:** `internal/session/`, `internal/api/`, `internal/store/migrations/`, `internal/tmux/` (cited), test files in `internal/api/`
**Files scanned:** 8 read in full (manager.go, session.go, sessions.go, agents.go, global.go, agent_integration_test.go, 00017, 00018) + targeted greps for helper/test-convention citations
**Pattern extraction date:** 2026-08-26
**Verified against:** worktree `add-global-session-301` at HEAD; line numbers from direct reads this session
