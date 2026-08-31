# Phase 17: Hardening & E2E - Pattern Map

**Mapped:** 2026-08-28 (revision 1: added `e2e_opencode_resume_test.go` row)
**Files analyzed:** 11 (6 new test/doc files, 2 docs, 3 modifications)
**Analogs found:** 10 / 11 (the real-binary process-restart loop itself has NO in-repo analog — see No Analog Found)

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `internal/api/e2e_global_restart_test.go` (new; `cmd/kamacu` is the alternative home — planner picks one) | test | request-response + process lifecycle (spawn/SIGTERM/restart) | `internal/api/sessions_global_test.go` (harness/host-gate/fake-claude) + `scripts/smoke.sh` (restart loop, manual) + `cmd/kamacu/serve.go` (the SUT) | role-match; the process-spawn core itself has **no analog** |
| `internal/api/e2e_opencode_resume_test.go` (new — revision addendum; omitted from the original census) | test | process lifecycle + argv capture (real-HOME posture, wrapper agent) | `internal/api/e2e_global_restart_test.go` (the plan-17-01 harness — reuses its unexported helpers verbatim) + `internal/api/sessions_global_test.go` `TestGlobalOpencodeCaptureHost` (:1134-1193 — out-of-band first-turn/capture poller block) | exact (extends the phase's own harness; 17-RESEARCH Pattern 3) |
| `internal/api/global_noleak_test.go` (new) | test | CRUD (enumeration surfaces) | `internal/api/sessions_global_test.go` `TestGlobalActivityExclusion` (:1444-1486) | exact |
| `internal/api/global_interlock_test.go` (new) | test | CRUD + file-I/O | `internal/api/global_test.go` seams block (:386-425) | exact |
| `internal/mcp/global_parity_test.go` (new) | test | request-response (in-memory bridge) | `internal/mcp/sessions_test.go` (:23-52) | exact |
| `internal/session/manager.go` (modify, custom spawn arm ~:170-210) | service | process spawn (env construction) | itself — the edit site; env-append idiom at :189-209 | exact (edit-in-place) |
| `internal/session/opencode_engine_test.go` (modify, :134-162) | test | process spawn env assertion | itself + the `writeEnvDumpStub`/`envValue` helpers in the same file | exact (edit-in-place) |
| `web/src/api/types.ts` (modify, :62) | model (wire types) | n/a (type-only) | the same file's `Task.source` union (:90) — widen-in-place precedent | exact |
| `web/src/pages/TaskPage.tsx` (modify, :64 + :535) | component | n/a (predicate) | `web/src/pages/BoardPage.tsx` :21-22/:55 (the SAME predicate — see Shared Patterns) | exact |
| `.planning/phases/17-hardening-e2e/17-UAT.md` (new at UAT time) | doc | n/a | `.planning/phases/16-global-view-settings-bar-integration/16-UAT.md` | exact |
| `.planning/phases/17-hardening-e2e/17-VERIFICATION.md` (new; carries the D-55 audit table) | doc | n/a | 17-RESEARCH.md Pattern 4 table shape + prior phase VERIFICATION docs | exact |

## Pattern Assignments

### `internal/api/e2e_global_restart_test.go` (test, request-response + process lifecycle)

**Analogs:** `internal/api/sessions_global_test.go` (primary), `scripts/smoke.sh`, `cmd/kamacu/serve.go` (SUT mechanics), `internal/api/agent_integration_test.go` (fake-claude wiring)

**Host-gate + tmux isolation pattern** — `sessions_global_test.go` :263-271 (and identically `global_test.go` :566-572):
```go
if _, err := exec.LookPath("tmux"); err != nil {
    t.Skip("tmux not on PATH")
}
// Per-test socket + kill-server cleanup registered BEFORE the server
// (LIFO: runs after the harness teardown).
c := tmux.Client{Socket: fmt.Sprintf("ktest-gmint-%d", os.Getpid()), ConfPath: "/dev/null"}
t.Cleanup(func() { _ = c.KillServer(context.Background()) })
```
NOTE for the REAL-binary harness: the in-process suites isolate via a per-test `Socket` name, but the spawned server hardcodes `tmux.DefaultSocket` ("kamacu", serve.go:269). The real-binary harness MUST isolate via `TMUX_TMPDIR=<sandbox>` in the **child env AND every harness-side tmux probe** — NOT a different socket name, NOT `TMUX_TMP_DIR` (silently ignored — empirically verified, 17-RESEARCH Pitfall 1). Failure mode: server B's `sweepOrphanTmux` (serve.go:369-423) kills the user's real `kamacu-*` sessions on the shared socket.

**fake-claude wiring pattern** — `sessions_global_test.go` :730-740:
```go
mgr.SetAgentConfig(session.AgentConfig{
    BaseURL:   "http://127.0.0.1:7333",
    Token:     testHookToken,
    ClaudeBin: testdataFakeClaude(t),
})
globRoot := t.TempDir()
argsFile := filepath.Join(t.TempDir(), "args")
pwdFile := filepath.Join(t.TempDir(), "pwd")
t.Setenv("FAKE_CLAUDE_ARGS_FILE", argsFile)
t.Setenv("FAKE_CLAUDE_PWD_FILE", pwdFile)
```
For the real binary, injection is the `--claude-bin` flag (serve.go:63 → `AgentConfig.ClaudeBin`, serve.go:267): pass `--claude-bin <repo>/internal/api/testdata/fake-claude` on the serve command line, and put `FAKE_CLAUDE_ARGS_FILE`/`FAKE_CLAUDE_PWD_FILE` in the CHILD env (t.Setenv won't reach the spawned process). `testdataFakeClaude(t)` (agent_integration_test.go:35-45) shows the chmod-0o755 defensive reapply. The stub (`internal/api/testdata/fake-claude`, 16 lines) records argv to `$FAKE_CLAUDE_ARGS_FILE`, pwd to `$FAKE_CLAUDE_PWD_FILE`, traps TERM → exit 143, and has a `FAKE_CLAUDE_RESUME_FAIL` mode — use verbatim, do not build a new fake.

**argv helpers to copy** — `sessions_global_test.go` :824-853 (`readStubFile` polls a recorder file ≤3s; `argvHasPair` matches consecutive tokens like `--resume <csid>`) and `sessions_test.go` :342-354 (`readArgv`).

**Transcript fixture pattern** (powers every resumable/resume assertion) — `sessions_test.go` :328-339:
```go
func seedTranscript(t *testing.T, globRoot, csid string) {
    t.Helper()
    dir := filepath.Join(globRoot, "-home-x-wt")
    if err := os.MkdirAll(dir, 0o755); err != nil { ... }
    if err := os.WriteFile(filepath.Join(dir, csid+".jsonl"), []byte("{}"), 0o644); err != nil { ... }
}
```
Glob shape is `filepath.Glob(filepath.Join(globRoot, "*", claudeSessionID+".jsonl"))` (resume.go:16-25) — any one dir level works. In the HOME-sandboxed harness write under `<sbx>/.claude/projects/<anydir>/<csid>.jsonl`; DELETE the fixture to flip resume into the 409 (one fixture, positive + D-54 negative).

**Real-opencode leg pattern** — `sessions_global_test.go` :1134-1168 (`TestGlobalOpencodeCaptureHost`): NOT HOME-sandboxed (real opencode resolves config/auth from real HOME — its doc comment :1131-1133), out-of-band first turn:
```go
runCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
defer cancel()
cmd := exec.CommandContext(runCtx, "opencode", "run", "Reply with just: ok")
cmd.Dir = root
cmd.Env = append(os.Environ(), "PWD="+root)
out, rerr := cmd.CombinedOutput() // model errors tolerated — the session row is created regardless
```
Plus the capture poller loop (:1171-1184, 200ms polls / 60s deadline against `global_task.opencode_session_id`). The D-52 wrapper trick rides `PATCH /api/agents/{id}` `{command}` — command is editable on system agents, engine is locked (agents_crud.go:161-166 engine-lock gate; :193-200 command update). Restore the command in a `t.Cleanup` registered IMMEDIATELY after the PATCH (research Pitfall 9).

**Restart loop core (no in-repo analog)** — the process-management block in 17-RESEARCH.md "Server process management" is the template; its mechanics are verified against `scripts/smoke.sh` :26-42 (`start_server`: spawn `serve --addr --db` → poll `/api/healthz` 50×100ms; `stop_server`: kill + wait) and serve.go: no signal handler exists ("No graceful shutdown exists — process death stops it", :340-343; `http.ListenAndServe` :354) — default SIGTERM death IS the restart semantic. Boot order the second process re-runs: migrate → backfills (incl. `BackfillGlobalTask` :199) → `opencode.InstallPlugin` (:215) → tmux conf next to DB (:223-224) → `sweepOrphanTmux` (:326, :369-423 — global rows are known, :383-389). Build in-test: `go build -o <sandbox>/kamacu ./cmd/kamacu` with `cmd.Dir` = module root (`../..` from `internal/api`, `..` from `cmd/kamacu`; `web/dist/index.html` placeholder is committed so embed never fails).

---

### `internal/api/global_noleak_test.go` (test, CRUD enumeration)

**Analogs:** `internal/api/sessions_global_test.go` `TestGlobalActivityExclusion` (:1444-1486), harness from `global_test.go` (:27-56)

**Core leak-assertion posture** — `sessions_global_test.go` :1444-1486 (re-home this posture, generalized to every surface, with a LIVE global session):
```go
// RUNNING global sessions — agent AND plain bash.
status, gbody := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "agent"})
...
status, bbody := doJSON(t, "POST", srv.URL+"/api/sessions", map[string]any{"scope": "global", "kind": "bash"})
...
st, raw := getJSON(t, asrv.URL+"/api/activity?window=week")
if !strings.Contains(string(raw), "Activity Fixture") { ... }   // the surface's one legitimate row
for _, leak := range []string{"Scratchpad", "Global"} {
    if strings.Contains(string(raw), leak) { ... }              // GINT-03 violation
}
```

**Harness + wire-assertion style** — `global_test.go` :27-56 (`newGlobalTestServer`: HOME sandbox via `t.Setenv("HOME", t.TempDir())` → temp `store.Open`+`Migrate` → `GlobalRoutes` → `t.Cleanup(srv.Close; db.Close)`) and the exact-field assertion style of `TestGetGlobalUnconfigured` (:62-112: `body["root_path"]`, `body["github_repo"]` present/null checks). Use `newGlobalSessionServerWithTmux` (:33-66) as the full-surface shape (Routes + SessionRoutes + GlobalRoutes over one DB+Manager) so boards/tasks/projects/workspace/activity are all reachable.

**The enumerated surfaces (assertion targets — every guard is `source='manual'`-keyed, so global rows are excluded BY CONSTRUCTION; the test proves each stays that way):**
| Surface | Guard (file:line, verified) |
|---|---|
| Unscoped task list | `tasks.go:194` `WHERE source = 'manual'` |
| Board fetch | `tasks.go:233` |
| Move/position (3 of 5 queries) | `tasks.go:288` (MIN position), `tasks.go:432` (recount), `tasks.go:538`/`:554` (shift + reorder) |
| Activity stats/lists | `activity.go:135` `t.source = 'manual'` |
| Workspace delete COUNT guard | `workspaces.go:215` `SELECT COUNT(*) FROM projects WHERE workspace_id = ?` |
| Project list | `GET /api/projects` (no global entity exists to exclude — count-unchanged assertion) |
| DB structural proof | `SELECT COUNT(*) FROM tasks` / `FROM projects` unchanged after configuring + spawning global (mirrors `TestGlobalAgentSpawn` :931-938 "tasks rows = 0" assert) |

Plus a manual-task peer fixture (createProject + createTask + move still 200s — the board keeps working, per `TestGlobalStatusLiveEntry`'s peer-task idiom :1241-1247). MCP surfaces live in the mcp file (below), not here.

---

### `internal/api/global_interlock_test.go` (test, CRUD + file-I/O)

**Analog:** `internal/api/global_test.go` :353-425 (seams) + `internal/api/projects.go` :888-917 (the gated delete under test)

**Clone seams + namespace helper** — `global_test.go` :356-380, :388-396:
```go
func fakeGitClone(t *testing.T, ref, dest string) error {
    t.Helper()
    if err := os.MkdirAll(dest, 0o755); err != nil { return err }
    if out, err := exec.Command("git", "init", dest).CombinedOutput(); err != nil { ... }
    if out, err := exec.Command("git", "-C", dest, "remote", "add", "origin",
        "https://github.com/"+ref+".git").CombinedOutput(); err != nil { ... }
    return nil
}

defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
    return "", fakeGitClone(t, ref, dest)
})()
defer github.SetAvailableForTest(true)()
defer github.SetValidateRunnerForTest(func(_ context.Context, parsed string) (string, bool, error) {
    return "octo/widgets", true, nil
})()

func globalManagedDest(t *testing.T, canonical string) string {
    // filepath.Join(home, ".kamacu", "repos", "global", filepath.FromSlash(canonical))
}
```
The project-side clone lands at `~/.kamacu/repos/<owner>/<name>` (reposBase); the global root at `~/.kamacu/repos/global/<owner>/<name>` (globalManagedDest) — same ref allowed on both sides (D-03); the filesystem assertions compare exactly these two dirs (os.Stat after each direction's delete/clear). Project delete path: `removeManaged` (projects.go:888-917) — gates pass (no tasks → no worktrees) → `os.RemoveAll(clone)` on the PROJECT clone only (:912-917). Global clear: `PUT /api/global {"root_path":""}` → 200 (D-21 semantics, `TestPutGlobalClear` global_test.go:242-265). Folder-direction cheap assert: temp dirs, no seams needed.

---

### `internal/mcp/global_parity_test.go` (test, in-memory bridge)

**Analog:** `internal/mcp/sessions_test.go` :23-52 (house "in-memory transport" pattern)

**Bridge construction pattern** — `sessions_test.go` :29-40:
```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    gotPath = r.URL.Path
    gotRaw = r.URL.RawQuery
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    _, _ = w.Write([]byte(`[]`))
}))
t.Cleanup(srv.Close)

b := &bridge{base: srv.URL, token: "t", client: &http.Client{Timeout: 10 * time.Second}}
if _, err := b.listSessions(context.Background(), newCallToolRequest(nil)); err != nil { ... }
```
For D-57 the canned backend is replaced by (or seeded with) the REAL Kamacu handler output — a live global session row with `taskTitle:"Scratchpad"/projectName:"Global"` labels — asserting BOTH directions:
- **Leak direction:** `list_tasks` → `GET /api/tasks` passthrough (mcp/tasks.go:182-198 — `b.call(ctx, http.MethodGet, "/api/tasks", nil)`) contains no global entity.
- **Honest direction:** `list_sessions`/`get_session` passthrough (mcp/sessions.go:516/:579 per research) lists/describes the global session with the labels, never 404. Result-unwrapping idiom: `res.Content[0].(*mcpsdk.TextContent)` → `json.Unmarshal` (sessions_test.go:128-141, incl. the orphaned-row filter precedent for field-level asserts).

---

### `internal/session/manager.go` (modify — D-63 production fix, custom spawn arm)

**Edit site:** :170-210 (the engine-branched arm). Current env posture (:189-209):
```go
envExtra := []string{"TERM=xterm-256color", "COLORTERM=truecolor"}
if opts.AgentEngine == "opencode" {
    envExtra = append(envExtra,
        "KAMACU_SESSION_ID="+id,
        "KAMACU_HOOK_TOKEN="+cfg.Token,
        "KAMACU_HOOK_BASE="+cfg.BaseURL, // D014
        "PWD="+dir,
    )
}
cmd.Env = append(os.Environ(), envExtra...)
```
**Fix shape (research OQ3 recommendation):** before the opencode gate, strip `KAMACU_SESSION_ID`/`KAMACU_HOOK_TOKEN`/`KAMACU_HOOK_BASE` from the inherited `os.Environ()` slice (the opencode gate then re-adds them) — closes the secret-propagation hole when `go test`/the server runs nested inside a Kamacu session (the parent exports all three). Keep the append-last-value-wins idiom already in the comment block (:200-206). The companion test edit is a comment in `opencode_engine_test.go` `TestCustomEngineDoesNotGetHookEnv` (:134-162) documenting the under-Kamacu context — the test body keeps asserting absence and then passes everywhere.

---

### `web/src/api/types.ts` + `web/src/pages/TaskPage.tsx` (modify — D-62, type-only)

**types.ts:62 (current):**
```typescript
engine: "claude" | "custom";
```
**Widen to `"claude" | "custom" | "opencode"`** — matches the Go wire + migration 00015 seed. In-place union-widening precedent in the same file: `Task.source: "manual" | "github_pr"` (:90, with its provenance comment). Follow the file's commenting convention (each wire field carries a "why" comment citing the backend serialization).

**TaskPage.tsx:62-64 (current predicate):**
```typescript
const projectAgentId = allProjects?.find((p) => p.id === projectId)?.agent_id;
const projectEngine = allAgents?.find((a) => a.id === projectAgentId)?.engine;
const isClaudeAgent = projectEngine !== "custom"; // undefined/"" (loading) or "claude" -> show
```
**Flip to `projectEngine === "claude"`** (render site :535 `{isClaudeAgent ? <QuotaIndicator /> : null}`). Loading flips show→hide — cosmetic, note it in the task, don't engineer around it (research Pitfall 7). **Discovered consistency site:** `web/src/pages/BoardPage.tsx` :21-22 + :55 carries the IDENTICAL `projectEngine !== "custom"` predicate — same fix applies if the planner scopes it in (D-62's text names TaskPage only). `GlobalTaskPage.tsx` (:450) renders unconditionally — leave alone. Gates: `tsc`/build + lint-clean (edits, not new files).

---

### `.planning/phases/17-hardening-e2e/17-UAT.md` + `17-VERIFICATION.md` (docs)

**UAT doc analog:** `16-UAT.md` (40 lines) — YAML frontmatter (`status/phase/source/started/updated`), `## Tests` with `### N. Title (gate-id)` blocks each carrying one-line `expected:` + `result:`, then the `## Summary` counter block (total/passed/issues/pending/skipped/blocked) + `## Gaps`. D-59 flows: configure → agent → bash → reattach → stop; both engines' resume (D-53); interlock sanity; leak visual sanity. D-60 safety posture must name the concrete risk (skip-permissions agent, no worktree net, writing directly into the chosen checkout); D-12 settles here with the live bar in front of the user.

**VERIFICATION audit-table analog:** the row shape is locked in 17-RESEARCH.md Pattern 4 — `| Surface | Query/guard (file:line) | Assertion |` — one row per surface so future surfaces = one row + one subtest. The guard file:line values verified this session are in the noleak table above; copy them into the doc.

## Shared Patterns

### Host-gating (apply to every new test touching tmux/git/opencode)
**Source:** `sessions_global_test.go` :264-266, :1135-1140; `sweep_test.go` :23-25 (~30 sites house-wide)
```go
if _, err := exec.LookPath("tmux"); err != nil {
    t.Skip("tmux not on PATH")
}
```
The real-binary E2E additionally gates on the built binary (build failure = fatal, not skip) and `git`.

### JSON HTTP helpers (apply to all in-package API tests)
**Source:** `projects_test.go` :468 (`doJSON`), :501 (`doJSONList`), `sessions_test.go` :1457 (`getJSON`) — return `(status, map[string]any)` / raw bytes; reuse, don't re-implement. The real-binary harness drives a plain `http.Client` against the allocated port — same assertion shapes, URL = `http://127.0.0.1:<port>` instead of `srv.URL`.

### Poll-with-deadline convention
**Source:** `readStubFile` (≤3s, 50ms), `waitGlobalSessionExited` (8s — budgets the D-14 SIGKILL grace), capture poller (60s, 200ms), smoke.sh healthz (5s, 100ms) — every wait in the codebase is a bounded loop, never a sleep. Copy the budgets; add `TMUX_TMPDIR`-env'd `tmux.Client.HasSession` probes for D-49 (exact-match `"="+name` is handled inside `HasSession`, tmux.go:78-103 per research).

### 409 grammar assertion shape
**Source:** `global_test.go` :596-616 (`TestPutGlobalRootBlockedByLiveTmux`)
```go
reasons, ok := body["reasons"].([]any)
if !ok || len(reasons) == 0 { t.Fatalf(...) }
first, ok := reasons[0].(map[string]any)
// first["kind"] == "sessions"; first["target"] == <the live session name>
```
The E2E's SC1 leg asserts this same `{error, reasons[{kind,target}]}` shape over real HTTP; the id-clearing proof is the follow-up `POST /api/sessions {scope:global,kind:agent,resume:true}` → 409 `"no global claude session to resume"` (wire copy locked by `TestGlobalAgentResumeNoId409`, sessions_global_test.go:987-993).

### LIFO t.Cleanup discipline
**Source:** `sessions_global_test.go` :267-270 (KillServer registered BEFORE the harness), :56-64 (harness cleanup stops all manager sessions + closes DB). The E2E adds: stop spawned servers → KillServer the sandbox tmux socket → temp dirs auto-removed; restore-PATCH before server teardown (Pitfall 9).

## No Analog Found

| File | Role | Data Flow | Reason |
|------|------|-----------|--------|
| `internal/api/e2e_global_restart_test.go` — the process-management core only (in-test `go build`, spawn with scrubbed env, healthz poll, SIGTERM+Wait, re-spawn against the same DB) | test harness | process lifecycle | Nothing in-repo: `scripts/smoke.sh` does it in bash (manual); the Phase-14 "curl smoke" was a recorded transcript; Phase-15 restart tests are in-process sims (fresh Manager + routes over same DB — `TestGlobalSessionRestartOrphan` :633-640). **Use 17-RESEARCH.md Pattern 1 + the "Server process management" excerpt as the template** — its mechanics are file:line-verified against serve.go/smoke.sh. Everything AROUND the core (host-gates, fake-claude, fixtures, wire shapes, opencode leg) has the exact analogs mapped above. |

## Metadata

**Analog search scope:** `internal/api`, `internal/session`, `internal/mcp`, `internal/tmux`, `cmd/kamacu`, `scripts/`, `web/src` (pages, api, components), `.planning/phases/16-*`
**Files scanned:** ~15 read in full/targeted; guard lines verified by grep across `internal/api/*.go`
**Pattern extraction date:** 2026-08-28
**Notes for planner:** (1) Harness placement is open (research OQ1 recommends `internal/api`, one new file, private helpers, NO refactor of the Phase-15 suite); (2) Wave-0 full-suite baseline run should precede the D-63 fix (research Pitfall 4: `TestInput_*` is already green — don't plan a phantom fix); (3) opencode leg gets its own temp DB + real HOME (Pitfall 3); (4) `BoardPage.tsx` predicate is a discovered consistency edit — scope decision left to planning.
