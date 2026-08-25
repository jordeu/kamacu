# Phase 14: Global config API - Pattern Map

**Mapped:** 2026-08-25
**Files analyzed:** 3 (2 new, 1 modified)
**Analogs found:** 3 / 3 (plus 4 sub-patterns with no in-repo analog, covered by RESEARCH recipes)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/api/global.go` | controller (handler-file-per-resource) | request-response (GET) + CRUD-with-side-effects (PUT: validate → clone → UPDATE) | `internal/api/projects.go` | exact |
| `internal/api/global_test.go` | test | request-response | `internal/api/projects_test.go` + `internal/api/worktrees_test.go:391-449` | exact |
| `cmd/kamacu/serve.go` (+1 line) | route registration (config) | wiring only | `cmd/kamacu/serve.go:272-288` (itself — the registry block) | exact (self) |

Non-code artifact: `~/.kamacu/repos/global/<owner>/<name>/` is an on-disk runtime namespace first written by this phase — not a file to create, listed only as an integration point.

---

## Pattern Assignments

### `internal/api/global.go` (controller, request-response + CRUD)

**Primary analog:** `internal/api/projects.go` — the handler covers GET-singleton + partial-PUT + managed-clone lifecycle + gated-409, all of which exist there at file:line precision. **Secondary analog:** `internal/api/sessions.go` for the routes constructor.

#### Routes constructor + handler struct

**Analog:** `internal/api/sessions.go:27-56` (constructor) and `internal/api/projects.go:62-72` (dependency struct). Per-resource `XxxRoutes` constructor called from serve.go — NOT registered inside `routes.go` (RESEARCH anti-pattern list).

```go
// Source: internal/api/sessions.go:32-38 (constructor shape to copy)
func SessionRoutes(mux *http.ServeMux, mgr *session.Manager, db *sql.DB, tmuxClient tmux.Client) {
	s := &sessionHandlers{mgr: mgr, db: db, globRoot: defaultTranscriptGlobRoot(), tmuxClient: tmuxClient}
	mux.HandleFunc("GET /api/sessions", s.list)
	mux.HandleFunc("POST /api/sessions", s.create)
	// ...
}

// Source: internal/api/projects.go:67-72 (handler struct shape to copy)
type projectHandlers struct {
	db         *sql.DB
	wt         *worktree.Service
	mgr        *session.Manager
	tmuxClient tmux.Client
}
```

Phase-14 shape: `GlobalRoutes(mux *http.ServeMux, db *sql.DB, mgr *session.Manager, tmuxClient tmux.Client)` registering `GET /api/global` + `PUT /api/global` on a `globalHandlers{db, mgr, tmuxClient}` struct. **Take `mgr` NOW even though the manager half of the gate returns zeros** — Phase 15's `ListGlobal()` widens the helper without a signature change (RESEARCH A2). No `wt` field: no worktrees exist for the global scope.

#### Wire type (GET and PUT response share one struct — D-17/D-23)

**Analog:** `internal/api/projects.go:23-60` (`Project` struct with `GithubRepo *string` so unmanaged serializes as JSON null — the exact nullable-repo convention the global config needs).

```go
// Source: internal/api/projects.go:31 (the nullable-repo pointer convention)
	GithubRepo  *string `json:"github_repo"`
```

RESEARCH Pattern 2 gives the recommended `globalConfig` struct verbatim (`root_path`, `github_repo *string`, `agent_id`, `updated_at`, `root_exists`, `live{agent,bash,tmux}`, `agent{id,name,engine}`). Key constraints: resume-id fields NEVER appear (D-18); no `managed` bool — `github_repo NULL|NOT NULL` is the marker (00017 contract).

#### GET handler — single-row QueryRow + Scan

**Analog:** the `scanProject(h.db.QueryRow(...))` + `ErrNoRows` idiom from `internal/api/projects.go:633-639` and `internal/api/workspaces.go:167-177`.

```go
// Source: internal/api/projects.go:633-638 (single-row RETURNING/Scan + ErrNoRows split)
	p, err := scanProject(h.db.QueryRow(query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
```

Phase-14 difference: the singleton row is guaranteed (00017 seed + `BackfillGlobalTask`), so `ErrNoRows` maps to a **500 "global task row missing"** (fail-loud on the corrupted invariant — RESEARCH Open Question 3 recommendation), not 404. INNER JOIN on `agents` is safe: `agent_id` is `NOT NULL ... ON DELETE RESTRICT` (00017) and `agents_crud.go:274-281` refuses deleting the referenced agent. `root_exists` is a read-time `os.Stat` — note `dirExists` exists only in `internal/migrate/migrate.go:291` (different package); the api package has no such helper, so stat inline.

#### PUT — pointer decode + dispatch grammar (D-20/D-21)

**Analog 1 (pointer decode + all-nil 400):** `internal/api/projects.go:500-524` and `internal/api/workspaces.go:142-152`.

```go
// Source: internal/api/projects.go:515-524 (partial-decode + nothing-to-update)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Name == nil && req.Description == nil && req.GithubRepo == nil &&
		req.IconLetters == nil && req.IconColor == nil && req.WorkspaceID == nil &&
		req.AgentID == nil {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}
```

**Analog 2 (dispatch on non-empty `repo`):** `internal/api/projects.go:243-249`.

```go
// Source: internal/api/projects.go:243-249 (repo-non-empty wins the managed branch)
	if strings.TrimSpace(req.Repo) != "" {
		h.createByRepo(w, r, req.Repo, req.Name, wsID, agID)
		return
	}

	// --- Folder path (unchanged) ---
	abs, err := validateRepoPath(req.RepoPath)
```

**Analog 3 (explicit `""` = clear):** `internal/api/projects.go:545-548` — the pointer-clear that D-21 extends to BOTH root columns behind the gate.

```go
// Source: internal/api/projects.go:545-548 ("" clears → store NULL, no validation)
	if req.GithubRepo != nil {
		if strings.TrimSpace(*req.GithubRepo) == "" {
			// Explicit "" unlinks → store NULL (no validation).
			sets = append(sets, "github_repo = NULL")
```

Phase-14 trap (RESEARCH Pitfall 1): **branch FIRST (clear / managed / folder / both-400 / all-nil-400), validate AFTER the branch knows what it holds** — a naive validate-then-branch sends `root_path:""` into `validateRepoPath` and 400s "path must be absolute" (wrong branch, wrong message).

#### PUT managed variant — the 8-step ordering with UPDATE swapped for INSERT (SC2)

**Analog:** `internal/api/projects.go:357-449` (`createByRepo`). The ordering is load-bearing; the full excerpt:

```go
// Source: internal/api/projects.go:361-391 (steps 1-3: parse → gh-validate → dest)
	// 1. Parse/canonicalize the ref. The ONLY hard, host-independent reject.
	if _, err := github.ParseRepoRef(repoInput); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 2. gh-validate (RPROJ-05/D-03). ...
	canonical, verified, err := github.ValidateRepo(r.Context(), repoInput)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !verified {
		msg := msgRepoNotFound
		if !github.Available() {
			msg = msgGHUnavailable
		}
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	// 3. Compute the managed dest: ~/.kamacu/repos/<owner>/<name>. ...
	base, err := settings.ExpandHome(reposBase)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dest := filepath.Join(base, canonical)
```

```go
// Source: internal/api/projects.go:393-422 (steps 4-6: reattach-or-409 → dedup → clone-or-fail)
	reattached := false
	if _, statErr := os.Stat(dest); statErr == nil {
		if rerr := reattachManaged(r.Context(), dest, canonical); rerr != nil {
			writeError(w, http.StatusConflict, rerr.Error())
			return
		}
		reattached = true
	}
	// ...
	if !reattached {
		if cerr := github.Clone(r.Context(), canonical, dest); cerr != nil {
			_ = os.RemoveAll(dest)
			writeError(w, http.StatusInternalServerError, cerr.Error())
			return
		}
	}
```

```go
// Source: internal/api/projects.go:440-448 (step 7-8: row write only now, then respond)
	p, err := scanProject(h.db.QueryRow(
		`INSERT INTO projects (name, repo_path, github_repo, managed, ...) VALUES (...) RETURNING `+projectColumns,
		...))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
```

Phase-14 adaptations:
- **Step 0 is the gate** — `globalLiveBlockers` 409 BEFORE any gh/clone work (RESEARCH Pitfall: a clone takes minutes; never discover a 409 after one).
- `dest := filepath.Join(base, "global", canonical)` — the D-02 three-segment namespace (`reposBase` const at projects.go:355 is `"~/.kamacu/repos/"`, already imported/expanded via `settings.ExpandHome`).
- Step 7 becomes the single `UPDATE global_task SET ... WHERE id = 1 RETURNING` (agent_id rides the same UPDATE if supplied); no projects-dedup pre-check (step 5's `SELECT 1 FROM projects WHERE repo_path = ?` is project-shaped — the singleton has no dedup surface).
- Respond **200, not 201** (update of a guaranteed row), body = the GET shape (D-23).
- Clone-failure status is agent-discretion defaulting to the analog's 500 + one inline error; the singleton keeps its PRIOR value (assert that in tests — RESEARCH Pitfall 2).

#### Reattach — verbatim reuse

**Analog:** `internal/api/projects.go:456-478` (`reattachManaged`). CONTEXT discretion explicitly allows verbatim reuse; it is a package-level func in `api`, callable as-is with the global dest.

```go
// Source: internal/api/projects.go:456-477 (git check → origin read → case-insensitive compare)
func reattachManaged(ctx context.Context, dest, canonical string) error {
	if err := exec.CommandContext(ctx, "git", "-C", dest, "rev-parse", "--git-dir").Run(); err != nil {
		return fmt.Errorf("a directory already exists at %s but is not a git repository", dest)
	}
	out, err := exec.CommandContext(ctx, "git", "-C", dest, "remote", "get-url", "origin").Output()
	if err != nil {
		return fmt.Errorf("a directory already exists at %s with no origin remote", dest)
	}
	got, perr := github.ParseRepoRef(strings.TrimSpace(string(out)))
	if perr != nil || !strings.EqualFold(got, canonical) {
		return fmt.Errorf("a different repository is already checked out at %s", dest)
	}
	return nil
}
```

Degrade-don't-break copy for step 2 (also the `repo:""`-guidance tone), `internal/api/projects.go:482-485`:

```go
const (
	msgRepoNotFound  = "Repository not found on GitHub — check the name or your access."
	msgGHUnavailable = "Couldn't verify the repository — the gh CLI isn't available."
)
```

#### PUT folder variant — validator reuse + net-new footgun blocks

**Analog:** `internal/api/projects.go:74-101` (`validateRepoPath`) — reused verbatim (D-05); it returns the cleaned absolute path the D-07 equality set must run against (RESEARCH Pitfall 6).

```go
// Source: internal/api/projects.go:78-101
func validateRepoPath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("path must be absolute")
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be absolute")
	}
	abs := filepath.Clean(p)
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", abs)
	}
	// handles .git-as-directory AND .git-as-file (worktrees/submodules);
	// arg array only — never sh -c
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("not a git repository: %s", abs)
	}
	return abs, nil
}
```

⚠️ The D-07 equality checks (`abs == $HOME`, `abs == "/"`) and the D-27 `~/.kamacu` prefix block are **net-new** — no such block exists anywhere in `internal/api` today (verified by grep). Recipe in "No Analog Found" below.

#### PUT agent field — existence check, no gate, no id clearing (D-24/D-25)

**Analog:** `internal/api/projects.go:616-629` (exact semantics to copy: validated 400, no session gate, read-at-use at spawn).

```go
// Source: internal/api/projects.go:616-629
	if req.AgentID != nil {
		var exists int
		err := h.db.QueryRow(`SELECT 1 FROM agents WHERE id = ?`, *req.AgentID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "agent not found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sets = append(sets, "agent_id = ?")
		args = append(args, *req.AgentID)
	}
```

#### Single final UPDATE — SET-builder + updated_at + D-16 id clearing

**Analog:** `internal/api/projects.go:626-633` (dynamic SET builder, unconditional `updated_at` bump, id last).

```go
// Source: internal/api/projects.go:631-633
	sets = append(sets, "updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')")
	args = append(args, id)
	query := `UPDATE projects SET ` + strings.Join(sets, ", ") + ` WHERE id = ? RETURNING ` + projectColumns
```

Phase-14 additions to the SET list, all in ONE UPDATE (RESEARCH Pitfall 8 + anti-pattern "splitting the UPDATE"): root variant appends `root_path = ?`, `github_repo = ?` AND unconditionally `claude_session_id = NULL, opencode_session_id = NULL` (D-16 — only when a root field was supplied; never on agent-only PUTs per D-25). Folder variant sets `github_repo = NULL` too (never leaves a stale marker). `WHERE id = 1` (singleton).

#### 409 gate — grammar + liveness probe

**Analog 1 (blocker struct + 409 body):** `internal/api/projects.go:681-689` and `848-855`.

```go
// Source: internal/api/projects.go:686-689 (the structured reasons element)
type deleteBlocker struct {
	Kind   string `json:"kind"`   // "uncommitted" | "unpushed" | "stash" | "sessions"
	Target string `json:"target"` // "task #<id>" | "the managed checkout"
}

// Source: internal/api/projects.go:848-855 (the 409 write — REMOVE NOTHING)
	if len(blockers) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "the project can't be deleted yet",
			"reasons": blockers,
		})
		return
	}
```

REUSE `deleteBlocker` verbatim — `{kind:"sessions", target:"<name>"}` entries. This resolves RESEARCH Open Question 1 (structured form, the recommendation) and Pitfall 3 (13-CONTEXT's string sketch was illustrative; the code grammar is structured).

**Analog 2 (rows → HasSession → live-only):** `internal/api/projects.go:964-985` (`liveTmuxNames` — near-verbatim with the predicate swapped to `WHERE scope = 'global'`).

```go
// Source: internal/api/projects.go:964-985
func (h *projectHandlers) liveTmuxNames(ctx context.Context, taskID int64) []string {
	rows, err := h.db.QueryContext(ctx, `SELECT name FROM tmux_sessions WHERE task_id = ?`, taskID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}
	live := make([]string, 0, len(names))
	for _, name := range names {
		if alive, herr := h.tmuxClient.HasSession(ctx, name); alive && herr == nil {
			live = append(live, name)
		}
	}
	return live
}
```

**Analog 3 (probe contract):** `internal/tmux/tmux.go:75-98` — call `h.tmuxClient.HasSession(ctx, name)` verbatim; count alive ONLY on `(true, nil)` (three-state: `(false, err)` is inconclusive, never dead). Never hand-roll `has-session` without the `"="+name` exact-match prefix (tmux 3.4 prefix-matching, RESEARCH Pitfall 4). The manager half of the gate is a documented zero-count seam until Phase 15's `ListGlobal()` (D-19). The gate only COUNTS — it never stops anything.

---

### `internal/api/global_test.go` (test, request-response)

**Analogs:** `internal/api/projects_test.go` (harness + seams) + `internal/api/worktrees_test.go:391-449` (detached-tmux gate recipe).

#### Harness — HOME-sandboxed server (managed dest lands in temp)

**Analog:** `internal/api/projects_test.go:27-37` (`newRepoTestServer`) wrapping `newTestServer` (`:431-453`).

```go
// Source: internal/api/projects_test.go:31-37
func newRepoTestServer(t *testing.T) (*httptest.Server, *sql.DB, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home) // ExpandHome resolves ~ via os.UserHomeDir → HOME
	srv, db, _ := newTestServer(t)
	return srv, db, filepath.Join(home, ".kamacu", "repos")
}
```

⚠️ **Planner note:** `newTestServer` (projects_test.go:445-446) registers ONLY `Routes(mux, db, ...)` — it does not know about `GlobalRoutes`. `global_test.go` needs its own thin harness (or extension) that builds the mux with `GlobalRoutes(mux, db, mgr, tmuxClient)` registered and a real `tmux.Client` wired for gate tests — mirroring how the worktree tests use their own `newWorktreeServerWithTmux(t, c)` (worktrees_test.go:404). Same `store.Open` + `store.Migrate` + `t.Cleanup(srv.Close/db.Close)` skeleton.

#### Request helper + git fixture

**Analog:** `internal/api/projects_test.go:468-498` (`doJSON`) and `:456-464` (`gitRepo` — `git init` in `t.TempDir()`, the folder-variant fixture). Use `doJSON(t, "PUT", srv.URL+"/api/global", map[string]any{...})` for every case.

#### Seams — no live gh ever needed

**Analog:** `internal/api/projects_test.go:49-57` (`failCloneNeverCalled` via `github.SetCloneRunnerForTest`) and `internal/github/testhooks.go:15/:26/:47` (`SetCloneRunnerForTest` / `SetValidateRunnerForTest` / `SetAvailableForTest`, each returning a restore func). Exercise the degrade path explicitly: `SetAvailableForTest(false)` → assert the `msgGHUnavailable` 400 (RESEARCH "Missing dependencies" requirement).

#### The 409-gate recipe (RESEARCH Pitfall 7 — the one test that must exist)

**Analog:** `internal/api/worktrees_test.go:397-440` — skip-guard, per-test socket, KillServer cleanup BEFORE server construction (LIFO), seeded row + real detached session, gate assert, disk-intact assert.

```go
// Source: internal/api/worktrees_test.go:397-417 (verbatim recipe, scope swapped)
func TestWorktreeCleanupCountsAndKillsDetachedTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux on PATH")
	}
	// Per-test socket + kill-server cleanup registered BEFORE the server (LIFO).
	c := tmux.Client{Socket: fmt.Sprintf("ktest-wtclean-%d", os.Getpid()), ConfPath: "/dev/null"}
	t.Cleanup(func() { _ = c.KillServer(context.Background()) })
	srv, env := newWorktreeServerWithTmux(t, c)
	// ...
	name := fmt.Sprintf("kangent-%d-1", id)
	if _, err := env.db.Exec(
		`INSERT INTO tmux_sessions (task_id, n, name, label) VALUES (?, 1, ?, 'Bash 1')`, id, name); err != nil {
		t.Fatalf("seed tmux_sessions row: %v", err)
	}
	detach := append(c.BaseArgs(), "new-session", "-d", "-s", name, "-c", wtPath)
	if err := exec.Command("tmux", detach...).Run(); err != nil {
		t.Fatalf("seed surviving tmux session: %v", err)
	}
	awaitHasSession(t, c, name, true)
```

Phase-14 swap: `INSERT INTO tmux_sessions (scope, n, name, label) VALUES ('global', 1, 'kamacu-global-1', 'Bash 1')` (00018 columns — `task_id` NULL + `scope='global'` per the XOR CHECK), then `doJSON PUT {"root_path":""}` → assert 409 + non-empty `reasons` + row/disk untouched. `awaitHasSession` lives at `internal/api/sessions_test.go:652`.

Additional table test the pitfalls demand: the `{nil, "", "x"}³` dispatch matrix over `repo`/`root_path` (RESEARCH Pitfall 1 warning sign), and the config-history sequence managed → folder → clear → managed asserting `github_repo` never goes stale (Pitfall 8 warning sign).

---

### `cmd/kamacu/serve.go` (+1 line) (route registration)

**Analog:** the registry block at `cmd/kamacu/serve.go:272-288` — itself. One line alongside the other per-resource constructors:

```go
// Source: cmd/kamacu/serve.go:272-278 (where the new line goes)
	mux := http.NewServeMux()
	api.Routes(mux, db, wtSvc, mgr, tmuxClient)
	api.SessionRoutes(mux, mgr, db, tmuxClient)
	api.WorktreeRoutes(mux, db, wtSvc, mgr, tmuxClient)
	// ... add: api.GlobalRoutes(mux, db, mgr, tmuxClient)
```

All four deps (`db`, `mgr`, `tmuxClient`) are already in scope at that point (tmuxClient built at :269). No backfill wiring change needed — `BackfillGlobalTask(db)` already runs at serve.go:199 (Phase 13). `routes.go` stays untouched (per-resource registration lives in serve.go — the SessionRoutes/WorktreeRoutes convention, RESEARCH anti-pattern list).

---

## Shared Patterns

### Response helpers
**Source:** `internal/api/respond.go:9-18` — apply to every handler in `global.go`.
```go
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
```

### Error-handling grammar
**Source:** every projects.go handler — early `writeError(...)` + `return` on every path; `errors.Is(err, sql.ErrNoRows)` split from real DB errors (500 `err.Error()`); validation rejects BEFORE anything is appended to sets/args so a reject leaves the row untouched (projects.go:545-571 idiom). Apply to all PUT branches: gate 409 and validation 400 must leave row AND disk byte-for-byte intact.

### Degrade-don't-break gh messaging
**Source:** `internal/api/projects.go:375-382` + `:482-485` — not-found vs no-gh copy chosen by `github.Available()`. Apply to the managed variant's ValidateRepo step and any `repo` guidance errors.

### `updated_at` bump
**Source:** `internal/api/projects.go:631` — `updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')` appended unconditionally to every UPDATE SET list. Same literal in `global.go`.

### Arg-array-only exec
**Source:** `internal/api/projects.go:95-96`, `internal/github/clone.go:67-68`, `internal/tmux/tmux.go:69-73` — every shell-out is an arg array, never `sh -c` (the package invariant; repo refs are attacker-adjacent input).

### tmux three-state liveness
**Source:** `internal/tmux/tmux.go:75-98` — alive only on `(true, nil)`; `(false, nil)` is dead; `(false, err)` is inconclusive and NEVER read as alive. Exact-match `"="+name` is embedded in `Client.HasSession` — never bypass it.

## No Analog Found

Net-new code with no in-repo precedent — planner should follow the RESEARCH.md recipes (cited):

| Item | Reason no analog | RESEARCH recipe |
|------|------------------|-----------------|
| D-07 footgun equality checks (`abs == $HOME`, `abs == "/"`) after `validateRepoPath` | Verified by grep: no such block exists in `internal/api` — projects never had one (validateRepoPath's own `~`-expansion/abs/dir/git checks are the closest, projects.go:79-99) | Pitfall 6: run the equality set on the POST-expansion absolute value |
| D-27 `~/.kamacu` prefix block (400) | New policy; the only HasPrefix path logic in the app is `validateRepoPath`'s `~/` expansion (projects.go:79) | Pitfall 5: `base, _ := settings.ExpandHome("~/.kamacu")`; reject `abs == base \|\| strings.HasPrefix(abs, base+string(os.PathSeparator))`; folder branch only; copy explains WHY |
| Gated-clear dispatch (`root_path:""` → clear, behind 409) | Pointer-clear exists (projects.go:545-548) but no existing clear is lifecycle-gated | Pattern 3 + Pitfall 1: branch before validating; gate like any root change |
| `scope='global'` gate predicate + forward-wired manager half | `liveTmuxNames` is task-shaped (`WHERE task_id = ?`); no scope='global' query exists yet; no `Info.Global`/`ListGlobal()` until Phase 15 | Pattern 5: predicate swap + zero-count seam, testable via the worktrees_test detached-tmux recipe |

## Metadata

**Analog search scope:** `internal/api/` (all handler + test files), `internal/github/`, `internal/tmux/`, `internal/settings/`, `internal/store/migrations/`, `cmd/kamacu/`
**Files read:** projects.go (6 targeted ranges), sessions.go, workspaces.go, respond.go, routes.go, agents_crud.go, global_backfill.go, clone.go, home.go, tmux.go, serve.go, projects_test.go (3 ranges), worktrees_test.go, 00017 + 00018 migrations + greps for helpers/seams
**Pattern extraction date:** 2026-08-25
