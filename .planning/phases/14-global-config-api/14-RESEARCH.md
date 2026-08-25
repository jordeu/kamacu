# Phase 14: Global config API - Research

**Researched:** 2026-08-25
**Domain:** Go HTTP API composition over the Phase-13 `global_task` singleton (folder validation, gh managed-clone lifecycle, gated reconfiguration) — pure in-repo integration, zero new dependencies
**Confidence:** HIGH — every claim verified directly against the live codebase in this worktree at file:line level (same posture as the v1.13 milestone research docs)

## Summary

Phase 14 is a **composition phase with a complete in-repo precedent kit**. Every mechanism GCONF-01..04 needs already exists and was read at file:line: `validateRepoPath` (projects.go:78), the v1.4 8-step `createByRepo` atomicity ordering (projects.go:361-449), `reattachManaged` (projects.go:456), the partial-PATCH pointer idiom with agent-existence 400 (projects.go:495-629), the gated-delete 409 grammar (projects.go:681-689, 849-855), the Phase-13 `global_task` storage contract (00017) with boot backfill (global_backfill.go), the scope-aware tmux schema (00018) + sweep fix (serve.go:384), and the exported github test seams (github/testhooks.go). The work is one new handler file (`internal/api/global.go`), one route registration, and tests — "the v1.4 sequence with UPDATE swapped for INSERT," exactly as the milestone research put it.

The three things the planner must get right, none of which have a copy-paste answer: **(1) the PUT dispatch grammar** — D-20/D-21 define a three-way branch (`repo` → managed clone, non-empty `root_path` → folder, `root_path:""` → clear) riding pointer-decode semantics where `""` is NOT the folder variant but the reset; **(2) the 409 wire shape** — 13-CONTEXT D-15 sketches `reasons:["..."]` as strings but says "mirrors the app-wide gated-delete grammar," and the actual grammar in code is a STRUCTURED list `reasons:[{kind, target}]` (deleteBlocker, projects.go:686-689) — this discrepancy needs an explicit planner decision (recommendation: structured form); **(3) the forward-wired GCONF-04 gate** — no global session can exist until Phase 15 (no `Global` flag on `Info`, no spawn path), but the tmux half of the gate CAN be real today (`SELECT name FROM tmux_sessions WHERE scope='global'` + exact-match `HasSession` probe) and is fully testable now via the worktrees_test.go detached-tmux pattern (worktrees_test.go:398-435).

Structural safety of the namespace is provable, not just tested-for [VERIFIED: in-repo path math]: project clones are always exactly `~/.kamacu/repos/<owner>/<name>` (2 segments under `repos/`, projects.go:386-391) while global roots are always `~/.kamacu/repos/global/<owner>/<name>` (3 segments) — no exact-path collision is representable, and D-27's folder-branch block on `~/.kamacu` closes the remaining folder-root direction. D-03's "same repo as project + global" therefore means two independent directories on disk, by design.

**Primary recommendation:** Build `internal/api/global.go` as `GlobalRoutes(mux, db, mgr, tmuxClient)` + a `globalHandlers` struct following the `SessionRoutes`/`projectHandlers` pattern; implement the PUT as a single dispatch handler with gate-check-early / validate-everything-before-clone / single-final-UPDATE ordering; make the 409 reasons list the structured `{kind, target}` shape; implement the tmux half of the live-gate for real and leave the manager half as a documented zero-count seam Phase 15 widens with `ListGlobal()`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Carry-Forward Locked (from 13-CONTEXT — do not reopen)**

- **Namespace:** managed global root = `~/.kamacu/repos/global/<owner>/<name>` (D-01/D-02); same repo as project + global allowed, no cross-entity guards (D-03); reconfiguring never deletes the old clone, same-repo re-PUT reattaches (D-04).
- **Folder validation:** `validateRepoPath` semantics — git repo required (D-05); `$HOME`/`~`/`/` rejected 400 (D-07); no boot revalidation — a vanished root fails honestly at spawn time (D-08).
- **Gate semantics (GCONF-04):** any live global PTY blocks — agent + plain-bash + tmux (D-13); exited sessions and persisted resume ids never block (D-14); 409 shape `{error, reasons:[...]}` mirroring the app-wide gated-delete grammar (D-15); resume ids cleared on ANY successful root change, no no-op-re-PUT exception (D-16).
- **Atomicity (SC2):** v1.4 `createByRepo` 8-step ordering with UPDATE swapped for INSERT — validate before clone, clone before row; a failed clone leaves no half-configured row or directory.
- **Agent delete-guard:** already landed in Phase 13 (`agents_crud.go:274` 409 + FK RESTRICT backstop) — SC3's "cannot be deleted" half is done; this phase ships the SETTABLE half.

**GET wire shape (new)**

- **D-17: Config + live truths.** `GET /api/global` returns the stored singleton (root_path, github_repo, agent_id, updated_at) PLUS derived state: `root_exists` (cheap `os.Stat` at GET time — honest about a vanished root without violating D-08's no-boot-revalidation), live global session counts, and an embedded agent summary (id, name, engine). One round-trip serves Phase 16's Settings section and the `/global` view's honest unconfigured state (GVIEW-04).
- **D-18: Resume ids internal only.** `claude_session_id` / `opencode_session_id` NEVER appear on the GET wire — they are Phase-15 spawn-path internals (the server constructs `--resume`; the frontend never needs the ids).
- **D-19: Ship the count fields now as zeros.** The live-session count fields ship in the Phase 14 wire shape (`{agent:0, bash:0, tmux:0}` until Phase 15's spawn path exists) — the wire contract is stable from day one, and the 409 gate check is forward-wired the same way (trivially passes until global sessions can exist).

**PUT grammar & clear (new)**

- **D-20: Single partial PUT.** One `PUT /api/global` with pointer fields `root_path` (*string), `repo` (*string), `agent_id` (*int64) — omitted = untouched (the app-wide partial-PATCH pointer idiom). Root dispatch mirrors `create()`: non-empty `repo` → managed-clone variant; non-empty `root_path` → folder variant; both supplied → 400; `root_path:""` is the clear (not a folder variant).
- **D-21: `root_path:""` clears the root.** Explicit empty string resets `root_path` → `''` AND `github_repo` → `NULL`, behind the same live-session 409 gate as any root change, resume ids cleared on success (D-16 uniform).
- **D-22: Field names `root_path` + `repo` + `agent_id`.** `root_path` matches the 00017 column (GET and PUT share one name); `repo` matches the v1.4 create dispatch field exactly; `agent_id` as everywhere.
- **D-23: PUT returns the GET shape.** Updated config + root_exists + counts + agent summary — one wire type for the endpoint; curl shows the clone/reattach outcome immediately.

**Default-agent change (new)**

- **D-24: Agent change allowed anytime — no 409.** A live global session keeps the agent it was spawned with; the change applies at the next Start. Exactly the per-project pattern (`projects.go:616` — existence-validated 400, no session gate, read-at-use at spawn). Root gating exists because a cwd change under a running shell is a lie; an agent-default change mutates nothing under a live session.
- **D-25: Agent change never touches resume ids.** D-16 (clear on root change) stays the ONLY clearing path — ids are engine-keyed, not agent-keyed; a claude id sitting unused while opencode is the default is harmless, and switching back restores the resumable conversation.

**Folder-overlap policy (new)**

- **D-26: Any valid git folder allowed.** No project-overlap guard — the folder branch validates git-repo + D-07 footguns, nothing else. Mirrors D-03's no-cross-entity-guards stance: the global root claims no ownership (reconfigure never deletes; folder-project deletes never touch the dir), a shared checkout is the user's explicit choice.
- **D-27: Block paths inside `~/.kamacu` (400).** One HasPrefix check on the expanded data-dir path with clear copy — everything under it is Kamacu-managed machinery (a task worktree gets gated-removed by cleanup; a managed clone belongs to a project), so a global root there is always a trap, never a choice. Extends the D-07 footgun block in the same spirit.

### the agent's Discretion

- Clone-failure status code (v1.4 `createByRepo` uses 500 + one inline error — mirroring it is the default).
- Empty-body `PUT {}` behavior (the workspaces "nothing to update" 400 is the precedent).
- Same-value re-PUT of `agent_id` (no-op success is fine — no gate applies).
- Exact JSON spelling of the agent summary and count fields (D-17/D-19 fix the content, not the key names).
- Whether the reattach check reuses `reattachManaged` verbatim or a thin wrapper (it is exactly the same semantics).
- How the forward-wired live check queries (in-memory manager surface vs tmux `has-session`) — Phase 15's `ListGlobal()` lands the real thing; Phase 14 wires the gate so Phase 15's spawn path makes it bite.

### Deferred Ideas (OUT OF SCOPE)
None — discussion stayed within phase scope. GT-FUT-01..07 remain parked in REQUIREMENTS.md (promotion, multiple scratchpads, per-workspace scratchpads, MCP write parity, sidebar entry, root-path line, git-status line).
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GCONF-01 | Configure the global root as a validated local folder path | Folder branch of `PUT /api/global`: `validateRepoPath` verbatim (projects.go:78) + D-07 footgun equality checks + D-27 `~/.kamacu` HasPrefix block; `GET /api/global` returns config + `root_exists` derived state (Pattern 2/3 below) |
| GCONF-02 | Configure the root as a GitHub repo — gh-validate, clone into the separate global namespace, atomic failure, reattach on re-config | Managed branch: v1.4 8-step ordering (projects.go:361-449) with UPDATE-for-INSERT, dest `~/.kamacu/repos/global/<owner>/<name>` (D-02), `reattachManaged` verbatim (projects.go:456), Clone's built-in RemoveAll-on-failure (clone.go:50); namespace collision-proof by path-depth math (see Pattern 4) |
| GCONF-03 | Set the global task's default agent from configured agents | `agent_id *int64` on PUT with the projects.go:616-628 existence-check 400 pattern, no gate (D-24), no resume-id touch (D-25); delete-guard half already shipped (agents_crud.go:274-281) |
| GCONF-04 | Root change/clear refused 409 + reasons while global sessions live; resume ids cleared on success | `globalLiveBlockers` gate helper: real tmux half today (`tmux_sessions WHERE scope='global'` + exact-match HasSession), manager half zero until Phase 15's `ListGlobal()`; structured `{kind, target}` reasons (see Open Question 1); id clearing in the single final UPDATE, unconditional on root-field supply (D-16) |
</phase_requirements>

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Config storage + derived state (`GET /api/global`) | API handler (new `global.go`) + SQLite singleton | — | The 00017 row is the single source of truth (settings-KV rejected at milestone research); GET derives `root_exists`/counts at read time, never persists them |
| Folder-root validation | API layer (`validateRepoPath` reuse) | — | Validation is a PUT-time concern (D-08: no boot revalidation); the validator already lives in package `api` |
| Managed-clone lifecycle | API layer orchestrating `internal/github` + `os` | disk (`~/.kamacu/repos/global/`) | The v1.4 shape: handler owns ordering/atomicity, `github.Clone` owns the exec + self-cleanup, nothing else in the app touches the global namespace |
| Live-session gate (GCONF-04) | API layer | tmux (liveness probe) + session manager (Phase 15) | D-13 needs ALL live surfaces: in-memory PTYs (Phase 15's `ListGlobal()`) + detached tmux survivors (probeable today via `HasSession`) |
| Agent resolution | `global_task.agent_id` FK (read-at-use) | Phase-15 spawn path | Config changes need no restart — spawn reads the FK fresh (the SET-03 rule, projects.agent_id precedent) |
| Route registration | `cmd/kamacu/serve.go` | `routes.go` pattern | Per-resource `XxxRoutes(mux, deps)` constructor registered alongside `SessionRoutes` (serve.go:273-288) |
| Resume-id clearing | API layer (single final UPDATE) | — | D-16: clearing rides the successful root-change UPDATE only — never a separate write, never on agent-only PUTs (D-25) |

## Standard Stack

**Add nothing.** Zero new dependencies is locked (13-CONTEXT carry-forward, verified against `go.mod` by the milestone STACK research). This phase composes existing in-repo assets only.

### Core (in-repo assets this phase composes)

| Asset | Location | Purpose | Verification |
|-------|----------|---------|--------------|
| `validateRepoPath` | internal/api/projects.go:78 | Folder-root validator reused verbatim (D-05): `~` expansion, abs check, dir check, `git rev-parse --git-dir` | [VERIFIED: in-repo, read at line] |
| `createByRepo` 8-step ordering | internal/api/projects.go:361-449 | The atomicity template SC2 adapts (UPDATE swapped for INSERT) | [VERIFIED: in-repo, read at line] |
| `reattachManaged` | internal/api/projects.go:456-478 | Reattach-on-origin-match: git check → origin read → case-insensitive canonical compare → reuse, never clobber | [VERIFIED: in-repo, read at line] |
| `github.ParseRepoRef` / `ValidateRepo` / `Clone` / `Available` / `RepoDescription` | internal/github/{github,clone}.go | The gh surface; degrade-don't-break copy (`msgRepoNotFound`/`msgGHUnavailable`, projects.go:482-485) | [VERIFIED: in-repo, read at line] |
| `settings.ExpandHome` | internal/settings/home.go:10 | `~` expansion for `reposBase` + the D-27 block anchor | [VERIFIED: in-repo, read at line] |
| `reposBase` const | internal/api/projects.go:355 | `~/.kamacu/repos/` — the global namespace nests inside it as `global/<owner>/<name>` | [VERIFIED: in-repo, read at line] |
| 409 grammar `{error, reasons:[{kind,target}]}` | internal/api/projects.go:686-689, 849-855 | D-15's template — structured blocker list (see Open Question 1) | [VERIFIED: in-repo, read at line] |
| Pointer-decode partial update | internal/api/projects.go:500-529 + workspaces.go:142-152 | nil = omitted / "" = explicit clear; "nothing to update" 400 on all-nil | [VERIFIED: in-repo, read at line] |
| `global_task` singleton + backfill | migrations/00017 + internal/api/global_backfill.go | Storage contract: `root_path=''` = unconfigured; `github_repo NULL` ⇒ folder, NOT NULL ⇒ managed | [VERIFIED: in-repo, read at line] |
| `tmux_sessions.scope='global'` + exact-match `HasSession` | migrations/00018 + internal/tmux/tmux.go:88 | The real half of the forward-wired gate | [VERIFIED: in-repo, read at line] |
| Test seams | internal/github/testhooks.go + projects_test.go:31-57,431-446 | `SetValidateRunnerForTest`/`SetCloneRunnerForTest`/`SetAvailableForTest`, `newRepoTestServer` (HOME sandbox), `doJSON`, `gitRepo` | [VERIFIED: in-repo, read at line] |

**Installation:** none — `go build ./...` and `go test ./internal/api/` with the existing module.

## Package Legitimacy Audit

**No external packages are installed by this phase** (zero-new-deps is a locked carry-forward decision). All functionality composes Go stdlib (`net/http`, `database/sql`, `os`, `os/exec`, `path/filepath`, `encoding/json`) and existing in-repo packages. Registry checks: not applicable.

## Architecture Patterns

### System Architecture Diagram

```
curl / Phase-16 Settings UI (later)
  │
  ▼
PUT /api/global  ── dispatch (D-20) ──────────────────────────────────────────┐
  │                                                                          │
  │  body decoded into *string/*int64 pointers (nil = untouched)             │
  │                                                                          │
  ├─ repo non-empty ──────► MANAGED VARIANT                                  │
  │     1. gate check (409 if live global sessions — fail fast, pre-clone)   │
  │     2. ParseRepoRef        ── 400 on syntax (the only hard, gh-free x)   │
  │     3. ValidateRepo (gh)   ── 400 msgRepoNotFound / msgGHUnavailable     │
  │     4. dest = ExpandHome(~/.kamacu/repos)/global/<canonical>             │
  │     5. dest exists? ──► reattachManaged(dest, canonical) ── 409 on mism  │
  │     6. else github.Clone  ── fail → RemoveAll + ONE inline error,        │
  │                             singleton keeps its PREVIOUS value           │
  │                                                                          │
  ├─ root_path non-empty ► FOLDER VARIANT                                    │
  │     1. gate check                                                        │
  │     2. validateRepoPath (git repo required, D-05)                        │
  │     3. D-07 footgun block: abs == $HOME or "/" → 400                     │
  │     4. D-27 block: HasPrefix(abs, ~/.kamacu) → 400 (with the WHY copy)   │
  │                                                                          │
  ├─ root_path:"" (non-nil) ► CLEAR (D-21)                                   │
  │     1. gate check                                                        │
  │     2. resets root_path='' AND github_repo=NULL                          │
  │                                                                          │
  ├─ agent_id supplied ────► existence check vs agents → 400 "agent not      │
  │                          found" (NO gate, D-24; NO id clearing, D-25)    │
  │                                                                          │
  ├─ all-nil body ────────► 400 "nothing to update" (workspaces precedent)   │
  │  repo + root_path both non-empty ──► 400                                 │
  ▼                                                                          │
  SINGLE FINAL UPDATE global_task SET <changed fields>,                       │
    claude_session_id=NULL, opencode_session_id=NULL   ← ONLY when a root    │
    field was supplied (D-16 — unconditional, no no-op-re-PUT exception)      │
    + updated_at bump, RETURNING the row                                      │
  ▼                                                                          │
  respond 200 with the GET wire shape (D-23) ◄───────────────────────────────┘

GET /api/global ─► singleton row (guaranteed by boot backfill)
                + os.Stat(root_path) → root_exists (honest, no boot reval, D-08)
                + JOIN agents → agent summary {id, name, engine}
                + live counts {agent:0, bash:0, tmux:0}   ← tmux half real,
                  manager half zero until Phase 15's ListGlobal() (D-19)
                (resume ids NEVER on the wire, D-18)

GATE (GCONF-04): globalLiveBlockers(ctx, db, mgr, tmuxClient)
  tmux half (REAL today):  SELECT name FROM tmux_sessions WHERE scope='global'
                           → HasSession("="+name) exact match; alive&&err==nil
                           → blocker {kind:"sessions", target:"<name>"}
  manager half (stub):     zero agent/bash PTYs possible until Phase 15 adds
                           Info.Global + ListGlobal() — wired as the seam
  → non-empty → 409 {error: "...", reasons: [...]} — NOTHING mutated
```

### Recommended Project Structure

```
internal/api/
├── global.go        # NEW — globalHandlers + GlobalRoutes(mux, db, mgr, tmuxClient)
│                    #   get() / put() / dispatch + variant helpers
│                    #   globalLiveBlockers() — the forward-wired GCONF-04 gate
├── global_test.go   # NEW — dispatch/validation/atomicity/gate/clear tests
└── routes.go        # UNTOUCHED (global registers via its own XxxRoutes from serve.go)
cmd/kamacu/serve.go  # +1 line: api.GlobalRoutes(mux, db, mgr, tmuxClient)
~/.kamacu/repos/global/<owner>/<name>/   # NEW on-disk namespace (first written here)
```

### Pattern 1: Handler-file-per-resource with an `XxxRoutes` constructor

**What:** Every resource ships a handlers struct + one `XxxRoutes(mux, deps...)` constructor called from serve.go — `SessionRoutes` (sessions.go:32), `WorktreeRoutes`, `AgentRoutes`, etc. serve.go:273-288 is the registry. [VERIFIED: in-repo]

**When to use:** Here. `GlobalRoutes(mux, db, mgr, tmuxClient)` — take `mgr` NOW even though the manager half of the gate returns zeros, so Phase 15's `ListGlobal()` widens the helper without a signature change (mirrors how `SessionRoutes` carries `db` for later phases).

**Example:**
```go
// Source: internal/api/sessions.go:32-49 (the pattern to copy)
func GlobalRoutes(mux *http.ServeMux, db *sql.DB, mgr *session.Manager, tmuxClient tmux.Client) {
	g := &globalHandlers{db: db, mgr: mgr, tmuxClient: tmuxClient}
	mux.HandleFunc("GET /api/global", g.get)
	mux.HandleFunc("PUT /api/global", g.put)
}
```

### Pattern 2: One wire type for GET and PUT responses (D-17/D-23)

**What:** A single response struct carries stored config + derived truths. Derivations are computed at read time, never persisted.

**Example:**
```go
// Recommended shape (D-17/D-18/D-19/D-22 — key names at planner discretion for
// the agent summary/counts; root_path/github_repo/agent_id/updated_at are fixed)
type globalConfig struct {
	RootPath   string  `json:"root_path"`             // '' = unconfigured (00017 contract)
	GithubRepo *string `json:"github_repo"`           // nil ⇒ folder root; non-nil ⇒ managed
	AgentID    int64   `json:"agent_id"`
	UpdatedAt  string  `json:"updated_at"`
	RootExists bool    `json:"root_exists"`           // os.Stat at GET time (D-08 complement)
	Live       globalLive `json:"live"`                // {agent, bash, tmux} — zeros until Phase 15 (D-19)
	Agent      *globalAgent `json:"agent"`             // {id, name, engine} via JOIN agents
}
// claude_session_id / opencode_session_id are NEVER fields here (D-18)
```

**Asymmetry is intentional (D-22):** PUT input field is `repo` (matches v1.4 create dispatch); GET output field is `github_repo` (matches the 00017 column / Project JSON). Exactly the projects create() split.

### Pattern 3: Pointer-decode partial PUT with dispatch-on-non-empty (D-20/D-21)

**What:** The app-wide idiom — `*string`/`*int64` decode where nil = omitted-untouched and `""` = explicit clear — extended with `create()`'s dispatch rule (non-empty `repo` wins the managed branch). [VERIFIED: projects.go:207-232 + 500-529]

**Example:**
```go
// Source pattern: internal/api/projects.go:500-524 + workspaces.go:142-152
var req struct {
	RootPath *string `json:"root_path"`
	Repo     *string `json:"repo"`
	AgentID  *int64  `json:"agent_id"`
}
// decode...
rootSupplied := req.RootPath != nil || (req.Repo != nil && strings.TrimSpace(*req.Repo) != "")
if req.RootPath == nil && req.Repo == nil && req.AgentID == nil {
	writeError(w, http.StatusBadRequest, "nothing to update") // workspaces.go:150 precedent
	return
}
if req.Repo != nil && strings.TrimSpace(*req.Repo) != "" && req.RootPath != nil && *req.RootPath != "" {
	writeError(w, http.StatusBadRequest, "supply either repo or root_path, not both") // D-20
	return
}
```

**The trap this kills:** a naive `*string` handler treats `root_path:""` as "folder variant with empty path" → validateRepoPath errors "path must be absolute" — wrong message, wrong branch. D-21 makes `""` the CLEAR: dispatch on it BEFORE validation, gate it like any root change, reset both columns.

### Pattern 4: Managed-clone atomicity — the 8 steps adapted to UPDATE (SC2)

**What:** createByRepo's ordering with the row write swapped: validate BEFORE any clone, clone BEFORE any row mutation, mutate only after exit 0. [VERIFIED: projects.go:361-449]

The adapted sequence (with the gate hoisted to step 0 — a clone takes minutes; never discover a 409 after one):

1. **Gate** (`globalLiveBlockers`) — 409 before any gh/clone work, nothing mutated
2. `github.ParseRepoRef` — 400, the only hard host-independent reject
3. `github.ValidateRepo` — 400 with `msgRepoNotFound` / `msgGHUnavailable` chosen by `github.Available()` (the degrade copy, projects.go:375-382)
4. `dest := filepath.Join(base, "global", canonical)` where `base, _ := settings.ExpandHome(reposBase)` — the D-02 namespace
5. dest exists → `reattachManaged(r.Context(), dest, canonical)` verbatim — 409 on mismatch/non-git/no-origin, reuse on match (never clobber, D-04)
6. not reattaching → `github.Clone(ctx, canonical, dest)`; on failure `_ = os.RemoveAll(dest)` (belt-and-braces; Clone already self-cleans, clone.go:50) + ONE inline 500 error — **the singleton keeps its previous value**
7. single `UPDATE global_task SET root_path=?, github_repo=?, ... RETURNING` — only now (agent_id rides the same UPDATE if supplied and validated)
8. respond 200 + GET shape (D-23) — 200, not 201: this is an update of a guaranteed row

**Namespace safety is structural [VERIFIED: path math]:** project clones are always exactly `repos/<owner>/<name>` — two segments (projects.go:386-391); global roots are always `repos/global/<owner>/<name>` — three segments. An exact-path collision would require a two-segment path to equal a three-segment path: unrepresentable. A project clone for a GitHub owner literally named "global" (e.g. repo `global/foo`) lands at `repos/global/foo` — *inside* the global subtree but never equal to any global root (global roots are three segments deep) and never RemoveAll'd by anything in the global path (D-04: reconfigure never deletes). Cosmetic nesting only; no guard needed.

### Pattern 5: The forward-wired GCONF-04 gate

**What:** A helper returning blocker entries, real on every surface that can be live today, zero-count on the one that cannot. [Discretion area — this is the recommended resolution]

```go
// globalLiveBlockers returns why a root change must be refused (GCONF-04 /
// D-13..D-15). Today ONLY the tmux half can be non-empty — no global PTY can
// exist until Phase 15's spawn path; the manager half is the seam Phase 15
// widens (mgr.ListGlobal() when Info.Global lands). Inconclusive tmux probes
// are NEVER read as alive (tmux.go HasSession contract, Pitfall-6 posture).
func (h *globalHandlers) globalLiveBlockers(ctx context.Context) []deleteBlocker {
	var blockers []deleteBlocker
	// tmux half — REAL now: rows exist (00018), liveness probeable
	rows, err := h.db.QueryContext(ctx,
		`SELECT name FROM tmux_sessions WHERE scope = 'global'`)
	// ... for each name: alive, herr := h.tmuxClient.HasSession(ctx, name)
	//     if alive && herr == nil → blockers = append(..., {Kind:"sessions", Target: name})
	// manager half — zero until Phase 15 (no Info.Global to filter on; no spawn path)
	return blockers
}
```

**Why tmux-first is right:** it is the only live surface with storage today (00018 rows + `scope='global'`), it is exactly what a restart-survivor gate must see (D-13 includes tmux tabs), and it is testable NOW — seed a row + a real detached tmux session on a per-test socket and assert the 409 (the worktrees_test.go:398-435 pattern, skip-guarded on `exec.LookPath("tmux")`).

### Anti-Patterns to Avoid

- **Treating `root_path:""` as a folder-variant validation error** — it is the clear (D-21). Dispatch before validating.
- **A no-op-re-PUT exception** ("same value, skip the gate/clear") — explicitly rejected by D-16. Any PUT supplying a root field gates and clears. Test it.
- **Cloning before gating** — the gate is step 0; a 409 discovered after a 3-minute clone is a support ticket.
- **Splitting the UPDATE** (root now, ids later) — D-16's clearing rides the SAME successful UPDATE; a second write is a partial-failure window.
- **Clearing resume ids on agent-only PUTs** — D-25 forbids it; ids are engine-keyed, not agent-keyed.
- **Pointing the gate at `StopAllForTask(0)` or any task-shaped helper** — P4: the gate only *counts*; it never stops anything. Stopping is the user's job (the 409 says so).
- **A `managed` bool on the wire** — 00017's contract is `github_repo NULL|NOT NULL` as the marker; GET exposes exactly that (`*string` nil = folder). No second flag to drift.
- **Registering routes in routes.go** — the per-resource pattern registers from serve.go; routes.go stays the workspaces/agents/projects/tasks registry. (Cosmetic, but consistency is cheap.)

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Folder-path validation | A new validator | `validateRepoPath` (projects.go:78) | D-05 locks its semantics verbatim; it already handles `~` expansion, abs, dir, and `.git`-as-file (worktrees/submodules) |
| Reattach-on-existing-dir | A wrapper re-imimating origin compare | `reattachManaged` (projects.go:456) | Same semantics exactly (discretion even allows verbatim reuse); handles ssh+https origins, case-insensitive canonical compare, all three refuse messages |
| gh exec + cleanup | Direct `exec.Command("gh", "repo", "clone", ...)` | `github.Clone` | Self-cleans on failure (clone.go:50), strips `fatal: `, no-short-timeout contract, and the test seam (`SetCloneRunnerForTest`) already exists |
| 409 reason plumbing | A bespoke `{error, details}` shape | The `deleteBlocker` `{kind, target}` grammar | D-15 says mirror the app-wide grammar; Phase 16's Settings section will enumerate it exactly like the cleanup dialogs do |
| `~` expansion | `os.UserHomeDir` string surgery | `settings.ExpandHome` | Already the app-wide expansion used by `reposBase` and tests sandbox it via `t.Setenv("HOME", ...)` |
| Singleton boot guarantee | A GET-time re-seed | The existing `BackfillGlobalTask` boot hook | 00017 + backfill make the row structurally guaranteed; GET must not paper over a (hand-delete-only) missing row — fail honestly (see Open Question 3) |

**Key insight:** every novel-looking piece of this phase (mutable cwd source, gated reconfiguration, resume-id lifecycle) is a *composition* of two proven shapes — the v1.4 clone lifecycle and the gated-delete philosophy — with the row write inverted (UPDATE an always-present singleton vs INSERT-once). The genuine novelty is only the dispatch grammar and the clear semantics, which is why D-20/D-21/D-16 exist.

## Common Pitfalls

### Pitfall 1: The dispatch edge cases of D-20/D-21
**What goes wrong:** `root_path:""` reaches `validateRepoPath` and 400s "path must be absolute" (wrong branch); or `{"repo":"", "root_path":"/x"}` double-supplies and the both-400 rule misfires on the empty string; or `repo:""` is silently treated as a managed-variant trigger.
**Why it happens:** Pointer-idiom handlers naturally validate-then-branch; the clear inverts that.
**How to avoid:** Branch FIRST on the decoded values (clear / managed / folder / both-400 / all-nil-400), validate AFTER the branch knows what it holds. For `repo:""` explicitly (non-nil, empty-after-trim): treat as *not* a managed trigger — either ignore (mirror `create()`, which treats empty `repo` as absent) or 400 with guidance. See Open Question 2.
**Warning signs:** Tests only cover the three happy dispatches; no table test over `{nil, "", "x"}³`.

### Pitfall 2: A clone failure that half-configures the singleton
**What goes wrong:** The UPDATE runs before Clone's exit-0, or runs unconditionally after a caught error — the row now points at a directory that doesn't exist, `root_exists:false` forever, and the previous working config is lost.
**Why it happens:** Copying createByRepo's step numbers without its invariant: the row write is step 7, strictly after step-6 exit 0.
**How to avoid:** Single UPDATE after all work; on clone failure return with the row untouched (the singleton's PRIOR value survives — that is the atomicity SC2 demands). Assert in tests: failed clone → GET shows the OLD config byte-for-byte + `os.Stat(dest)` fails.
**Warning signs:** A `defer`-style or early UPDATE anywhere in the managed branch.

### Pitfall 3: The 409 shape drifting from the app-wide grammar
**What goes wrong:** Shipping `reasons:["agent session running"]` (plain strings, the 13-CONTEXT sketch) when every other gated surface ships `reasons:[{kind:"sessions", target:...}]` — Phase 16's dialog code forks on shape.
**Why it happens:** D-15's illustrative JSON shows strings while its normative sentence says "mirrors the app-wide gated-delete grammar" — and the grammar in code is structured.
**How to avoid:** Decide once at planning (Open Question 1; recommendation: structured `deleteBlocker` reuse) and lock it in the plan's wire contract.
**Warning signs:** RESEARCH/PLAN wire examples and test assertions disagreeing on the reasons element type.

### Pitfall 4: tmux prefix-matching and inconclusive probes in the gate
**What goes wrong:** `HasSession` without the `=` exact-match prefix compares `kamacu-global-1` → matches `kamacu-global-1-10`-style names (tmux 3.4 verified behavior, tmux.go:78-89); or a missing-tmux probe error is read as alive/dead instead of inconclusive.
**Why it happens:** Hand-rolling the probe instead of using `tmux.Client.HasSession` (which embeds `=-` exact matching) and its three-state contract.
**How to avoid:** Use `h.tmuxClient.HasSession(ctx, name)` verbatim; count alive ONLY on `(true, nil)` — mirror `liveTmuxNames` (projects.go:964-985).
**Warning signs:** Any `exec.Command("tmux", "has-session", "-t", name)` without `"="+name`.

### Pitfall 5: D-27's HasPrefix done wrong
**What goes wrong:** Comparing against the literal `"~/.kamacu"` (never matches — paths are expanded); or blocking `~/.kamacu` itself but not `~/.kamacu/` children via a missing trailing separator; or breaking the existing reposBase flow (which legitimately writes under `~/.kamacu/repos`).
**Why it happens:** The block applies to USER-SUPPLIED folder paths only, at PUT time — not to the managed variant's own dest math.
**How to avoid:** `base, _ := settings.ExpandHome("~/.kamacu")`; reject when `abs == base || strings.HasPrefix(abs, base + string(os.PathSeparator))`. Folder branch only. Copy explains WHY (Kamacu-managed machinery gets cleaned up — the CONTEXT specifics line).
**Warning signs:** Tests asserting `~/.kamacu/repos/foo` is accepted as a folder root (it must 400) while the managed variant still clones under it.

### Pitfall 6: D-07 equality checks on the wrong form
**What goes wrong:** Rejecting the literal `"~"` but not `"/home/user"` (the same dir post-expansion), or missing `"/"`.
**Why it happens:** Checking the raw input instead of the expanded/cleaned absolute path.
**How to avoid:** Run the D-07 equality set (`$HOME`, `/`) on the post-`validateRepoPath` absolute value (the function already returns the cleaned abs path). `~` and `~/` expand to `$HOME` inside validateRepoPath, so one comparison catches all spellings.
**Warning signs:** Footgun tests passing for `"~"` but not for `os.UserHomeDir()`.

### Pitfall 7: The gate tested only against zero sessions
**What goes wrong:** The forward-wired gate ships with only trivially-passing tests; Phase 15's spawn path then makes it bite for the first time in production code — with whatever bug the never-exercised 409 branch carries.
**Why it happens:** No global PTY can exist in Phase 14, so the obvious test is "gate passes."
**How to avoid:** The tmux half is exercisable NOW: hand-`INSERT INTO tmux_sessions (scope, n, name, label) VALUES ('global', 1, 'kamacu-global-1', 'Bash 1')` + a real detached tmux session on a per-test socket (`new-session -d`), assert PUT → 409 + reasons + row/disk untouched (worktrees_test.go:398-435 is the exact recipe, including the LookPath skip-guard and KillServer cleanup).
**Warning signs:** No 409-path test anywhere in Phase 14.

### Pitfall 8: Forgetting `updated_at` and the managed→folder transition marker
**What goes wrong:** Folder PUT on a previously-managed config leaves a stale `github_repo` value — GET now shows a folder root that still claims `github_repo:"owner/name"` (contradicting the 00017 marker contract); or PUTs don't bump `updated_at`.
**Why it happens:** Treating the folder variant as "just set root_path."
**How to avoid:** Folder variant sets `root_path=?` AND `github_repo=NULL` in the same UPDATE (mirroring D-21's clear, which also resets both). Always append `updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')` (projects.go:631 pattern).
**Warning signs:** A GET test matrix over config-history sequences (managed → folder → clear → managed) missing from the plan.

## Code Examples

### GET handler skeleton (single JOIN, honest 500 on the unreachable-missing row)

```go
// Source pattern: projects.go get() + the scanProject RETURNING idiom
func (h *globalHandlers) get(w http.ResponseWriter, r *http.Request) {
	var g globalConfig
	var repo sql.NullString
	err := h.db.QueryRow(`
		SELECT g.root_path, g.github_repo, g.agent_id, g.updated_at,
		       a.id, a.name, a.engine
		FROM global_task g JOIN agents a ON a.id = g.agent_id
		WHERE g.id = 1`).Scan(
		&g.RootPath, &repo, &g.AgentID, &g.UpdatedAt,
		&g.Agent.ID, &g.Agent.Name, &g.Agent.Engine)
	if errors.Is(err, sql.ErrNoRows) {
		// Unreachable-by-design (00017 seed + BackfillGlobalTask); fail loudly,
		// never paper over (Pitfall posture — see Open Question 3)
		writeError(w, http.StatusInternalServerError, "global task row missing")
		return
	}
	if err != nil { writeError(w, http.StatusInternalServerError, err.Error()); return }
	if repo.Valid { g.GithubRepo = &repo.String }
	g.RootExists = g.RootPath != "" && dirExists(g.RootPath) // os.Stat; "" never probes
	// live counts: tmux half real, manager half zero (Pattern 5)
	writeJSON(w, http.StatusOK, g)
}
```

The INNER JOIN is safe: `agent_id` is `NOT NULL REFERENCES agents(id) ON DELETE RESTRICT` (00017) — the agent always exists (and agents_crud.go:274 refuses deleting it).

### The clear + gate + single-final-UPDATE core (D-16/D-21)

```go
// Source pattern: projects.go update() SET-builder (626-633) + deleteManaged 409 (849-855)
if rootSupplied {
	if blockers := h.globalLiveBlockers(r.Context()); len(blockers) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "the Scratchpad root can't be changed while sessions are running",
			"reasons": blockers, // []deleteBlocker — Open Question 1's recommendation
		})
		return
	}
}
// ... variant work fills newRoot (string) + newRepo (sql.NullString), or neither
var sets []string
var args []any
if rootSupplied {
	sets = append(sets, "root_path = ?", "github_repo = ?")
	args = append(args, newRoot, newRepo) // clear → ("", NULL); folder → (abs, NULL); managed → (dest, canonical)
	sets = append(sets, "claude_session_id = NULL", "opencode_session_id = NULL") // D-16 — unconditional
}
if req.AgentID != nil { /* validated earlier */ sets = append(sets, "agent_id = ?"); args = append(args, *req.AgentID) }
sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')")
args = append(args, 1) // the singleton id
// UPDATE global_task SET ... WHERE id = 1 — then re-read for the GET-shape response (D-23)
```

### Test recipe: the gate tripped by a live global tmux tab (Pitfall 7)

```go
// Source pattern: internal/api/worktrees_test.go:398-435 (verbatim recipe, adapted)
func TestPutGlobalRootBlockedByLiveTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil { t.Skip("tmux not on PATH") }
	c := tmux.Client{Socket: fmt.Sprintf("ktest-global-%d", os.Getpid()), ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	srv, db, _ := newRepoTestServer(t) // HOME sandbox — repos/global/ lands in temp
	// seed a live global tab: row + real detached session
	if _, err := db.Exec(`INSERT INTO tmux_sessions (scope, n, name, label)
		VALUES ('global', 1, 'kamacu-global-1', 'Bash 1')`); err != nil { t.Fatal(err) }
	detach := append(c.BaseArgs(), "new-session", "-d", "-s", "kamacu-global-1", "-c", t.TempDir())
	if err := exec.Command("tmux", detach...).Run(); err != nil { t.Fatal(err) }
	status, body := doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{"root_path": ""})
	if status != http.StatusConflict { t.Fatalf("status = %d, want 409; body=%v", status, body) }
	// reasons non-empty; row + disk untouched
}
```

### Curl smoke script (SC1-SC4 shape; every criteria is curl-first)

```bash
# GET — fresh install: unconfigured honest state
curl -s localhost:7333/api/global
# → {"root_path":"","github_repo":null,"agent_id":1,"updated_at":"...",
#    "root_exists":false,"live":{"agent":0,"bash":0,"tmux":0},
#    "agent":{"id":1,"name":"Claude Code","engine":"claude"}}

# SC1: folder variant (validated git dir)
curl -s -X PUT localhost:7333/api/global -d '{"root_path":"/abs/git/dir"}'
# SC2: managed variant → clone into ~/.kamacu/repos/global/<owner>/<name>
curl -s -X PUT localhost:7333/api/global -d '{"repo":"owner/name"}'
# SC2: same repo again → reattach (response shows same root; no re-clone)
curl -s -X PUT localhost:7333/api/global -d '{"repo":"owner/name"}'
# SC2: failed clone → previous config intact, no dir at dest
curl -s -X PUT localhost:7333/api/global -d '{"repo":"owner/nonexistent"}'
# SC3: agent set + delete-guard (guard itself: curl -X DELETE /api/agents/{id} → 409)
curl -s -X PUT localhost:7333/api/global -d '{"agent_id":2}'
# SC4: clear
curl -s -X PUT localhost:7333/api/global -d '{"root_path":""}'
# SC4 gate (after Phase 15; or today with a hand-seeded live global tmux tab): 409 + reasons
```

## State of the Art

In-repo precedent evolution (no ecosystem drift applies — zero new dependencies):

| Prior Approach | Current Approach (this phase) | What Changed | Impact |
|----------------|-------------------------------|--------------|--------|
| v1.4 `createByRepo`: validate → clone → INSERT-once | Same ordering, UPDATE on a guaranteed singleton | The row always exists (00017+backfill); failure semantics invert from "no row" to "previous value survives" | Atomicity tests assert on the PRIOR config surviving, not row-count-zero |
| projects `PATCH` pointer idiom: `""` = clear (description/github_repo) | D-20/D-21: `root_path:""` = clear BOTH root columns, plus gate | First pointer-clear that is also lifecycle-gated | The clear shares one code path with change (D-16 uniformity) |
| Gated-delete 409: computed over worktrees/clone (deleteManaged) | Gate computed over live sessions only (no worktrees exist for global) | D-04: no gated removal, no dirty/unpushed/stash gates — the dir is never touched | Gate is sessions-only: `{kind:"sessions", ...}` entries |
| agents delete-guard (projects COUNT → 409) | Phase 13 already extended it to `global_task` (agents_crud.go:274) | Shipped | This phase only ADDS the settable half |

**Deprecated/outdated:** nothing — this phase deprecates no existing surface. `GET /api/global` is net-new; no existing route changes.

## Assumptions Log

> All claims in this research were verified directly in-repo at file:line (no external lookups needed — zero new dependencies, every mechanism precedented). The table below lists the only judgment calls a reader might mistake for verified facts.

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Structured `{kind, target}` reasons is the correct reading of D-15's "mirrors the app-wide gated-delete grammar" (recommendation, not a locked decision — see Open Question 1) | Patterns 3/5, Pitfall 3 | Low: shape swap is one struct change; but deciding late forks Phase 16 dialog code |
| A2 | `GlobalRoutes` taking `mgr` now (unused-until-Phase-15) is preferable to a `(mux, db, tmuxClient)` signature | Recommended Structure | Trivial: Phase 15 could also widen the signature |

**No `[ASSUMED]` package/tooling claims** — gh 2.82.0 / git 2.43.0 / tmux 3.4 / sqlite3 verified present on this host (Environment Availability below); all code-level facts read at line in this worktree.

## Open Questions

1. **409 reasons element shape: strings vs `{kind, target}` objects**
   - What we know: 13-CONTEXT D-15 sketches `reasons:["agent session running", ...]` but normatively says "mirrors the app-wide gated-delete grammar"; the grammar in code (deleteManaged, cleanup dialogs) is `reasons:[{kind, target}]` with machine-stable `kind` tokens (projects.go:686-689).
   - What's unclear: whether the sketch's string form was intentional (a lighter global-only grammar) or illustrative.
   - Recommendation: **structured `{kind, target}`** — one grammar app-wide, `kind:"sessions"` is machine-readable for Phase 16's Settings inline copy (mirroring the cleanup dialogs' reason lists), and it reuses `deleteBlocker` verbatim. Planner locks it in the wire contract.

2. **`repo:""` (explicit empty string, non-nil) semantics on PUT**
   - What we know: D-20 dispatches on non-empty `repo`; `create()` treats an empty `repo` field as "not the repo path" (folder fallback). D-21 designates `root_path:""` as THE clear.
   - What's unclear: whether an explicit-but-empty `repo` should be ignored-as-omitted, treated as a managed-variant syntax error, or accepted as an alternative clear spelling.
   - Recommendation: **400 with guidance** ("repo must be owner/name; to clear the root send root_path:\"\"") — explicitness beats silent ignore on a partial-PATCH surface, and it avoids TWO clear spellings. Planner's call (discretion-adjacent).

3. **GET behavior on the (unreachable) missing singleton row**
   - What we know: 00017 seeds it; `BackfillGlobalTask` re-arms it every boot; no API path deletes it. Only a hand-SQL DELETE mid-run can drop it.
   - What's unclear: 500 (honest, fail-loud) vs synthesizing defaults (degrade).
   - Recommendation: **500** — the row-missing state is a corrupted invariant, and degrade would mask it (the app-wide posture: gates warn, invariants fail loudly).

4. **Does the PUT gate also need to *re-read* the singleton first (current root) for anything?**
   - What we know: The gate is unconditional on root-field supply (D-16 kills the no-op exception), so the CURRENT value never affects gating. Reattach needs only dest+canonical.
   - What's unclear: nothing material — listed only so the planner doesn't add a spurious "value actually changed?" pre-check that D-16 forbids.
   - Recommendation: no current-value comparison anywhere in the PUT path.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| git | validateRepoPath, reattachManaged, test `gitRepo` fixture | ✓ | 2.43.0 | — (project premise guarantees git) |
| gh CLI | managed variant (ValidateRepo/Clone) — soft dep | ✓ | 2.82.0 | Degrade-don't-break: `msgGHUnavailable` 400 path (github.Available()); unit tests never need live gh (test seams) |
| tmux | gate liveness probe + the 409 gate test | ✓ | 3.4 | Skip-guarded tests (`LookPath("tmux")` → t.Skip, the package convention); probe errors are never read as alive |
| sqlite3 (CLI) | manual DB inspection during UAT | ✓ | present | Not required by code (modernc.org/sqlite embedded) |
| Go toolchain | build/test | ✓ | 1.26 (go.mod) | — |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none missing; gh absence and tmux absence both have designed degrade/skip paths that must be exercised by at least one test each (`SetAvailableForTest(false)` → `msgGHUnavailable`; tmux-skip is environmental).

## Security Domain

Applicable ASVS categories for a localhost-only, single-user, no-auth API adding a config endpoint:

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | App is localhost-only by design (no auth surface exists; `--insecure-allow-remote` is the standing deferred item) |
| V3 Session Management | no | No user sessions; PTY "sessions" are not auth sessions |
| V4 Access Control | no | Single-user local app; no privilege distinctions |
| V5 Input Validation | **yes** | Pointer-decode + dispatch validation; `ParseRepoRef` (regex + leading-dash rejection — the argv injection guard); `validateRepoPath` (abs/dir/git checks); D-07 footgun equality set; D-27 `~/.kamacu` prefix block; agent_id existence check. All shell-outs use arg arrays, never `sh -c` (the package invariant, github.go:6) |
| V6 Cryptography | no | Nothing cryptographic in scope |

### Known Threat Patterns for this surface

| Pattern | STRIDE | Standard Mitigation (in-repo) |
|---------|--------|-------------------------------|
| Path-traversal/footgun root (`~/.ssh`, `$HOME`, `/`, `~/.kamacu` machinery) → skip-permissions agent in a sensitive dir | Tampering/Elevation | D-07 equality rejects + D-27 prefix reject at PUT time (PITFALLS security table row 3); git-repo requirement (D-05) keeps the recovery story |
| Argv injection via repo ref (`-bad/owner`, `--flag/name`) | Tampering | `ParseRepoRef` rejects leading-dash segments (github.go:62-65); all execs are arg-array only |
| Repo-ref confusion (casing/host-forms) → reattach to wrong repo | Spoofing | gh-canonicalized casing wins on verified hits; `reattachManaged` compares origins case-insensitively and refuses on any mismatch (never clobbers) |
| Clone-leftover residue on failure → phantom "configured-looking" dir | Tampering | `github.Clone` self-RemoveAll (clone.go:50) + the handler's belt-and-braces RemoveAll; UPDATE strictly after exit 0 |
| Resume-id leakage on the wire | Information disclosure | D-18: ids never serialize (no JSON tags reachable — they are not fields on the wire struct) |

Threat-model note for the README/UAT (P3 carry-over): folder mode runs a skip-permissions agent directly in the user's chosen checkout with no worktree isolation — the D-07/D-27 blocks and the git-repo requirement are the configure-time posture; the persistent in-view warning is Phase 15/16's half (D-06).

## Sources

### Primary (HIGH confidence — read directly in this worktree, this session)

- `internal/api/projects.go` — validateRepoPath (:78-101), create dispatch (:207-232), createByRepo 8-step (:361-449), reattachManaged (:456-478), degrade copy (:482-485), partial-PATCH pointer idiom + agent_id 400 (:495-629), deleteBlocker 409 grammar (:681-689, 849-855), liveTmuxNames exact-probe pattern (:964-985), pathID (:1001)
- `internal/store/migrations/00017_global_task.sql` — the singleton storage contract (root_path/github_repo semantics in-line); `00018_tmux_scope.sql` — scope discriminator + XOR CHECK
- `internal/api/global_backfill.go` + `internal/store/global_task_migration_test.go` — Phase-13 landed invariants
- `internal/api/agents_crud.go:238-292` — the shipped global_task delete-guard (SC3's done half)
- `internal/github/{github,clone,testhooks}.go` — ParseRepoRef/ValidateRepo/Clone/Available contracts + the exported test seams
- `internal/tmux/tmux.go:46-110` — Client struct (value type, per-test sockets), HasSession exact-match + three-state contract
- `internal/session/manager.go:40-80` — SpawnOpts/Info today (NO Global flag — Phase 15's to add); List/ListByTask/StopAllForTask surface
- `cmd/kamacu/serve.go` — backfill wiring order (:194-199), route registry (:273-288), scope-aware sweep (:364-384)
- `internal/api/{routes,sessions,workspaces,respond}.go` — registration patterns, "nothing to update" precedent (:150), writeJSON/writeError
- `internal/api/projects_test.go` (:31-57, 425-510) + `worktrees_test.go` (:390-440) — the full test-harness kit: newRepoTestServer/newTestServer/doJSON/gitRepo, seam fakes, detached-tmux gate-trip recipe
- `.planning/research/{SUMMARY,ARCHITECTURE,PITFALLS}.md` — the milestone-locked architecture (§"Phase 2: Global config API", component 1, P3/P7/P8) — canonical inputs per the phase brief
- `.planning/phases/13-global-data-foundation-safety-net/13-CONTEXT.md` — D-01..D-16 locked inputs

### Secondary / Tertiary

- None — no external lookups were performed. Justification: zero new dependencies (locked), and every technical question this phase raises is codebase-verifiable; the milestone research already cross-validated the only externally-facing claims (gh exit-code unreliability cli/cli#8845, tmux prefix-matching — both cited in-code where relied upon). No web search providers are enabled in `.planning/config.json`.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — "add nothing" verified; every composed asset read at file:line
- Architecture: HIGH — handler-file-per-resource + 8-step ordering are proven in-repo shapes; the only novel code is dispatch/gate glue
- Pitfalls: HIGH — each pitfall mechanic verified against the cited lines (dispatch edges from the pointer idiom's semantics; gate testing from the worktrees_test recipe; namespace safety by path-depth argument)

**Research date:** 2026-08-25
**Valid until:** stable for the phase (in-repo integration research; no external-version drift surface)
