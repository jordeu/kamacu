# Phase 17: Hardening & E2E - Research

**Researched:** 2026-08-27
**Domain:** Real-binary E2E test harness (Go), lifecycle-gate verification, sentinel-leak regression tests, managed-root interlock, frontend type debt, UAT authoring
**Confidence:** HIGH (nearly every claim verified directly in this worktree this session — file:line reads, live test runs, an empirical tmux socket probe, and env-isolation reproduction)

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### Carry-Forward Locked (Phases 13–16 + research — do not reopen)

- **Gate semantics shipped:** the 409 family (D-28..D-34), `{error, reasons[...]}` grammar (D-15), resume-id clearing on ANY successful root change (D-16), gate order root-gates-first (D-33).
- **Namespaces separated by construction:** global managed root `~/.kamacu/repos/global/<owner>/<name>` vs project clones `~/.kamacu/repos/<owner>/<name>` (13-CTX D-01/D-02); same repo allowed on both sides, no cross-entity guards (D-03) — the interlock PROVES this, it doesn't add new guards.
- **Phase 15 shipped restart-*sims* + host-gated opencode capture** (`TestGlobalStatusPostRestart`, `TestGlobalSessionRestartOrphan`, `TestGlobalOpencodeCaptureHost`); the live-server curl story (real restart, resume same conversation) is THIS phase's deferred scope (15-VERIFICATION deferred table).
- **16-UAT already verified live:** the 409 dialog flow through Settings (test 4), /global interactive parity (test 2), the 7-state matrix (test 3). Phase 17 does not re-verify Phase-16 UX; it closes lifecycle gates.
- **Sentinel-leak posture:** the global entity is a singleton row, never a task/project row (research P2) — surfaces exclude by construction; the sweep proves each one stays that way.

#### Restart E2E mechanics (new — D-47..D-50)

- **D-47: SC2 restart E2E = a real-binary host-gated Go test.** Spawn the actual `bin/kamacu serve` with the Phase-14 curl-smoke env isolation recipe (HOME=sandbox + temp `--db` + free 127.0.0.1 port), drive spawn via real HTTP, SIGTERM, restart a second process against the same DB, assert resume offers + tmux rows via HTTP. NOT a shell script, NOT more in-process sims.
- **D-48: SC1 folds into the same harness — one curl story.** The harness covers the full reconfigure cycle: configure → spawn agent + bash/tmux → root change/clear 409s with reasons → stop them → change succeeds → persisted resume ids cleared (observable via the resume-refusal 409s). One test narrative proves both SC1 and SC2.
- **D-49: tmux reattach asserted at API level.** After restart: the global tmux row survives (`GET /api/sessions?scope=global`) with its label preserved (the v1.8 GAP-01 regression replayed for the global scope) and `tmux has-session` is alive on the real socket. The invisible browser-reattach visual stays in the UAT.
- **D-50: The E2E lives inline in `go test ./...`**, host-gated with the house `exec.LookPath` skip convention (skip when tmux/binary prerequisites are absent) — NOT a separate make target.

#### Resume-proof realism (new — D-51..D-54)

- **D-51: claude resume = fake-claude argv proof, automated.** Inside the real-binary harness, fake-claude proves the spawn→restart→resume cycle carries the persisted csid on the argv AND the transcript gate 409s when the transcript is gone (`transcriptExists` is cwd-agnostic — works verbatim). NO real-claude automated test (auth/quota/judgment don't belong in a test gate).
- **D-52: opencode resume = real host-gated restart cycle.** Extend the existing harness (`TestGlobalOpencodeCaptureHost` prior art; opencode provisioned on this host) through capture `ses_…` id → real restart → resume argv carries it → process returns.
- **D-53: The human UAT verifies BOTH engines' real "same conversation"** — ask the agent something identifiable, stop/restart, Resume, confirm it remembers. Rides the SC5 UAT.
- **D-54: The D-31 resume-refusal edges replay in the E2E gates** — resume with no persisted id, claude transcript vanished, opencode stale ocsid — at least at API level where not already covered by Phase-15 tests.

#### Leak/interlock proof form (new — D-55..D-58)

- **D-55: The sentinel-leak sweep = permanent regression tests + a documented audit table.** Tests for every surface AND an enumeration table in the phase VERIFICATION (surface → query/guard → assertion) so future surfaces have a pattern to copy.
- **D-56: One consolidated leak test file** (e.g. a `TestGlobalNoLeak*` family) walking every enumerated surface with a configured + live global session — SC4's enumeration legible in one place, trivially extensible. Named surfaces: kanban boards (all 5 board/position queries), project/task listings (`GET /api/tasks`, `GET /api/projects`), workspace non-empty delete guard (COUNT), Activity stats/lists (`GET /api/activity`), MCP task tools.
- **D-57: The MCP spot-check goes both directions through the real bridge** (in-memory transport, house pattern): task tools NEVER surface anything global (leak direction) AND session tools list/describe the live global session with honest labels, never 404 (GINT-02 — the research's MCP parity spot-check; read-only contract untouched).
- **D-58: The interlock is automated in both directions.** Create a managed project clone AND a managed global root (same repo allowed — D-03): the gated project delete removes only the project clone; clearing/reconfiguring the global root touches nothing project-side. The folder-root/folder-project trivial direction is asserted cheaply. Deterministic filesystem assertions — no UAT dependency.

#### UAT shape + leftovers (new — D-59..D-63)

- **D-59: SC5 UAT = a walkthrough doc** in the 16-UAT.md pattern — numbered flows with expected results, human drives the browser, results recorded in `17-UAT.md`. Flows: configure → agent → bash → reattach → stop, both engines' resume (D-53), the interlock sanity, the sentinel-leak visual sanity (board/settings/activity show nothing global).
- **D-60: The UAT runs against a real repo, the user's pick at UAT time** (e.g. the kamacu checkout itself — dogfooding). The no-worktree safety posture is documented honestly: a skip-permissions agent writing into a checkout the user actually cares about.
- **D-61: D-12 (bar-row visual treatment) settles AT the UAT** — text-only `Global · Scratchpad` stays unless the walkthrough reveals a real confusion need; the decision is made with the live bar in front of the user, exactly as D-12 locked.
- **D-62: The stale `types.ts` `Agent.engine` union widening + TaskPage QuotaIndicator predicate cleanup fold into this phase** as a small type-only task (16-01 Pitfall 2 flagged it here; no behavior risk).
- **D-63: The pre-existing red tests get a bounded investigate-and-fix task** — the `TestInput_*` family (bracketed-paste byte-count drift, quick-task 260728-t4c area) AND `TestCustomEngineDoesNotGetHookEnv` (custom-engine env leak). A hardening phase does not ship with known-red tests; root-cause fix preferred over expectation-adjustment, but the investigation decides.

### the agent's Discretion

- Harness file placement, binary-build strategy inside the test (build to temp dir vs assume bin/kamacu), timeout budgets, cleanup discipline (sandbox HOME teardown).
- Test function naming and how the curl-story narrative splits across test functions.
- The audit table's exact mechanics (which queries to enumerate, how deep the grep/reasoning goes) — planner/researcher territory.
- Whether harness helpers get shared/extracted with existing global tests.
- The red-test fix approach (root cause vs test-expectation correction) — decided by the investigation, not here.
- UAT doc exact flow numbering and copy (D-59 locks the form, not the wording).

### Deferred Ideas (OUT OF SCOPE)

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked in REQUIREMENTS.md. D-12 settles at this phase's UAT (D-61); the red tests and stale-union debt are folded IN (D-62/D-63), not deferred.
</user_constraints>

<phase_requirements>
## Phase Requirements

Phase 17 owns NO requirements — it closes end-to-end verification gates for requirements whose implementation phases shipped test-level evidence but deferred the live-flow halves to this phase.

| Gate | Description (from REQUIREMENTS.md) | Implementation Phase | What Phase 17 Adds |
|------|------------------------------------|----------------------|--------------------|
| GCONF-04 | Changing/clearing the root while global sessions are live is refused with 409 + reasons; on success the persisted resume ids are cleared | 14 (gate), 16 (dialog UX, UAT-verified) | The full cycle through a REAL binary: live sessions → 409 → stop → change → ids-cleared observable via resume-refusal 409s (D-48) |
| GSESS-02 | After a server restart, global agent sessions reconcile and offer Resume (claude `--resume` / opencode `--resume`, keyed on persisted singleton ids, engine-branched) | 15 (restart-sims + argv tests) | Real SIGTERM + real second process on the same DB; fake-claude argv carries `--resume <csid>`; real-opencode argv carries `-s <ses_…>`; both engines (D-47/D-51/D-52) |
| GSESS-03 | Global tmux bash tabs survive a server restart and reattach invisibly | 15 (restart-sim `TestGlobalSessionRestartOrphan`) | Real restart: row survives with label + `tmux has-session` alive on the socket (D-49); browser-invisible reattach stays in the UAT (D-59) |
| GINT-02 | MCP session tools handle task-less global sessions with honest labels — never 404 or garbled; read-only contract unchanged | 15 (API labels + bridge passthrough wiring) | Both-direction bridge spot-check through the real in-memory transport: honest labels AND task tools never leak (D-57) |
| GINT-03 | Global sessions excluded from Activity stats and lists | 15 (`TestGlobalActivityExclusion`) | Consolidated permanent regression in the `TestGlobalNoLeak*` family + VERIFICATION audit table row (D-55/D-56) |
| Sentinel-leak invariant (Phase-13 architecture) | No global entity appears on ANY enumeration surface (boards, listings, workspace guard, Activity, MCP task tools) | 13 (singleton design — exclusion by construction) | Every surface enumerated and asserted in one legible test family + the audit table pattern for future surfaces (D-55/D-56) |
| SC3 interlock | Managed-root ↔ project-delete never cross-delete in either direction | 13 D-01..D-03 (namespace by construction) | Both-direction automated proof with deterministic filesystem assertions (D-58) |
| SC5 UAT | Folder-mode root against a real repo checkout end-to-end | — | The walkthrough doc + its execution recording (D-59/D-60/D-53), D-12 settling (D-61) |

**REQUIREMENTS.md checkbox state:** all 19 v1.13 requirements are already `[x]`-checked (verified this session) — Phase 17 does NOT flip checkboxes; it produces the verification evidence (17-VERIFICATION.md audit table, E2E tests, 17-UAT.md) that the earlier phases' deferred tables point at.
</phase_requirements>

## Summary

Phase 17 is a verification-debt phase: every gate it closes already has implementation and (mostly) in-process test evidence from Phases 13–16. The genuinely new engineering is ONE thing — D-47's real-binary restart harness: a host-gated Go test that builds `cmd/kamacu`, spawns it with full env isolation, drives the whole SC1+SC2 curl story over real HTTP, SIGTERMs it, restarts a second process against the same DB, and asserts resume offers + tmux survival. **Nothing like it exists in-repo yet**: `scripts/smoke.sh` does build/spawn/kill/restart but is manual bash; the Phase-14 "curl smoke" was a recorded transcript in a SUMMARY, not an automated test; the Phase-15 restart tests are in-process sims. The harness is therefore the phase's risk center, and this research maps its mechanics precisely (build, env isolation, SIGTERM semantics, readiness polling, wire shapes, fake-claude wiring via `--claude-bin`, transcript fixtures in the sandbox HOME, and the opencode wrapper-agent trick for argv proof).

Three findings materially change the plan the CONTEXT anticipated:

1. **The E2E harness MUST set `TMUX_TMPDIR` (not `TMUX_TMP_DIR`) in the spawned server's env.** The tmux socket name is hardcoded (`tmux.DefaultSocket = "kamacu"`, serve.go:269) and tmux 3.4 puts `-L` sockets in `$TMUX_TMPDIR/tmux-<uid>/` (empirically verified this session + man7 tmux(1)). Without it, the restarted TEST server's startup orphan sweep would see the USER's real live `kamacu-*` tabs on the shared socket as unknown in the test DB and **kill the user's real sessions** — a live data-loss hazard on the dev host, not a theoretical one.
2. **The D-63 red-test picture is different from what the CONTEXT carried forward.** All three `TestInput_*` tests now PASS on this branch (Phase 15's delimiter fix 67f39ef healed them — verified by live runs this session). The one genuinely red test, `TestCustomEngineDoesNotGetHookEnv`, fails **only when `go test` runs inside a Kamacu session**: the parent shell exports `KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE` (this session has all three), the custom spawn arm inherits `os.Environ()` verbatim (manager.go:187), and the stub sees them. Reproduced and confirmed: `env -u KAMACU_SESSION_ID -u KAMACU_HOOK_TOKEN -u KAMACU_HOOK_BASE go test …` → PASS. The fix has a security angle: the parent's HOOK_TOKEN (a secret) currently propagates into arbitrary custom-agent children.
3. **The opencode resume-argv proof (D-52) has a clean, zero-production-change mechanism**: the system OpenCode agent's `command` is editable via `PATCH /api/agents/{id}` (engine is locked for system agents, command is not — agents_crud.go:161-214). Point it at a wrapper script that records argv then `exec`s the real `opencode`, and the E2E observes the `-s <ses_…>` argv while the real binary still runs with its real config/auth (which requires that leg to run NOT HOME-sandboxed, exactly like `TestGlobalOpencodeCaptureHost`).

Everything else — the leak sweep, the interlock, the D-62 type cleanup, the UAT doc — extends existing in-repo patterns (enumerated below with file:line references). No new packages, no frontend framework work, no migrations.

**Primary recommendation:** Build the phase around ONE new test file (or small family) containing the real-binary harness with the locked isolation recipe (`HOME=<sandbox>` + `TMUX_TMPDIR=<sandbox>` + temp `--db` + free port + scrubbed `KAMACU_*` + `--claude-bin` fake), split SC1/SC2 as one curl-story narrative across a few test functions; add the consolidated `TestGlobalNoLeak*` file, the in-package interlock test using the `github.Set*ForTest` seams, the two MCP bridge spot-check tests, the D-62 two-line frontend cleanup, and the D-63 env-scrub fix; close with the UAT doc.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Real-binary lifecycle E2E (SC1+SC2) | Go test harness (new, host-gated) | `cmd/kamacu serve` as SUT | The only way to prove SIGTERM → real restart → same-DB reconciliation; in-process sims already exist and are insufficient by definition (D-47) |
| Resume-argv proof (claude) | fake-claude via `--claude-bin` | testdata recorder files | D-51 locks fake-claude; the flag is the production injection point (serve.go:57 → `AgentConfig.ClaudeBin`) |
| Resume-argv proof (opencode) | Real opencode behind a wrapper agent | PATCH-restorable system-agent command | D-52 demands the real binary; the wrapper records argv without touching production code |
| Sentinel-leak sweep (SC4) | In-package Go tests over httptest servers | VERIFICATION audit table | Deterministic, fast, CI-safe; the real binary adds nothing to enumeration proofs |
| Interlock (SC3) | In-package Go tests + `github.Set*ForTest` seams | Filesystem assertions | D-58: deterministic fs assertions, no UAT dependency; seams exist and are the Phase-14 precedent |
| MCP parity (GINT-02 closure) | `internal/mcp` bridge tests (httptest backend) | — | D-57's "in-memory transport, house pattern" = `b := &bridge{base: srv.URL…}` shape (sessions_test.go:47) |
| tmux survival assertion | HTTP (`?scope=global`) + `tmux has-session` probe | — | D-49: API-level assertion; harness probes need the SAME `TMUX_TMPDIR` env as the server |
| Frontend type debt (D-62) | `web/src/api/types.ts` + `TaskPage.tsx` | — | Two edit sites, type-only + one predicate |
| "Same conversation" realism (D-53) | Human UAT | — | Auth/quota/judgment never belong in an automated gate |
| D-12 bar-row visual | Human UAT decision | — | Locked to settle with the live bar in front of the user (D-61) |

## Project Constraints (from CLAUDE.md / project instructions)

No root `AGENTS.md` exists. The worktree `CLAUDE.md` carries GSD-managed sections (Project, Stack, Conventions, Architecture, Workflow enforcement) plus user-level instructions:

- **GSD workflow enforcement:** repo edits go through GSD entry points — this phase IS a `/gsd-plan-phase`/`/gsd-execute-phase` flow, so the constraint is satisfied structurally. [VERIFIED: worktree CLAUDE.md]
- **User commit/PR conventions** (from `~/.claude/CLAUDE.md`): never mention or co-author Claude on commits/PRs; PR review comments short with empty global body when inline comments exist; PRs always open as drafts; PR descriptions start with a short abstract section. Relevant only if this phase ships via PR. [VERIFIED: ~/.claude/CLAUDE.md]
- **Project skills** (`.claude/skills/`): code-review-excellence, go-concurrency-patterns, error-handling-patterns, sql-* etc. — none specialize in Go test-harness construction or this phase's domain; no skill directives bind the plan. [VERIFIED: ls .claude/skills]

## Standard Stack

### Core (all existing — zero new dependencies)

| Library / Facility | Version | Purpose | Why Standard |
|---|---|---|---|
| Go `testing` + `os/exec` | go1.26.0 (host) | The E2E harness: build, spawn, signal, wait, re-spawn | House style — every integration test in `internal/api` is stdlib-only; D-50 requires inline `go test ./...` |
| `net/http` + `httptest` (client side only) | stdlib | Driving the real binary over loopback HTTP | The in-package suites already use `doJSON`-style helpers (global_test.go); the harness needs only a plain `http.Client` against the free port |
| `syscall.SIGTERM` | stdlib | Kill the first server process | No signal handler exists in serve (serve.go:346 comment: "No graceful shutdown exists — process death stops it") — default SIGTERM death IS the restart semantic [VERIFIED: cmd/kamacu/serve.go:345-350] |
| `internal/api/testdata/fake-claude` | in-repo | claude argv/pwd recorder + SIGTERM trap (exit 143) + `FAKE_CLAUDE_RESUME_FAIL` mode | The D-51 instrument; injected via the real `--claude-bin` flag [VERIFIED: testdata/fake-claude, serve.go:57] |
| `github.SetValidateRunnerForTest` / `SetCloneRunnerForTest` / `SetAvailableForTest` | in-repo seams | Managed-clone flows without network (interlock test) | The Phase-14 managed-test precedent (global_test.go:386-425) [VERIFIED] |
| `internal/mcp` `bridge` + httptest backend | in-repo | D-57 both-direction spot-checks | House "in-memory transport" pattern: `b := &bridge{base: srv.URL, token: "t", client: …}` [VERIFIED: internal/mcp/sessions_test.go:46-47] |
| goose (existing migrations) | v3.x in go.mod | Run automatically by `store.Migrate` at server boot | The restarted harness server migrates the same temp DB idempotently — no harness work needed |

### Supporting

| Facility | Purpose | When to Use |
|---|---|---|
| `GET /api/healthz` | Readiness poll for spawned servers | smoke.sh polls it 50×100ms — copy that budget [VERIFIED: scripts/smoke.sh:29-36, serve.go:295] |
| `net.Listen("tcp", "127.0.0.1:0")` → close → reuse | Free-port allocation | Better than smoke.sh's fixed 7402 (parallel-safe); tiny TOCTOU window is acceptable on localhost |
| `go build -o <tmp>/kamacu ./cmd/kamacu` | Build the SUT inside the test | Works even on fresh clones: `.gitignore` keeps a placeholder `web/dist/index.html` so `//go:embed all:dist` never fails [VERIFIED: .gitignore, web/embed.go:7] |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|---|---|---|
| In-test `go build` | Assume prebuilt `bin/kamacu` (make build) | `bin/` is gitignored — a fresh checkout/CI run would silently exercise a STALE or MISSING binary; building in-test (~seconds, cached by go) is deterministic. CONTEXT leaves this to discretion; recommend build-in-test |
| One big E2E test function | Several narratively-ordered functions sharing one harness | CONTEXT explicitly allows either; ordered subtests (`t.Run` sequence) keep the one-curl-story legibility while giving independent failure attribution |
| Docker isolation | Env-var isolation | Rejected by the Phase-14 precedent (STATE.md decision: env isolation, not containers) — D-47 inherits it |

**Installation:** nothing to install. **Version verification:** not applicable (no new packages).

## Package Legitimacy Audit

**No external packages are installed by this phase.** All facilities are Go stdlib plus in-repo test infrastructure (fake-claude/fake-opencode testdata, github test seams, mcp bridge). The two NPM-side edits (D-62) change no dependencies — `web/package.json` is untouched.

**Packages removed due to [SLOP] verdict:** none. **Packages flagged as suspicious [SUS]:** none.

## Architecture Patterns

### System Architecture Diagram (the E2E harness data flow)

```
                    ┌────────────────────────────────────────────────────────┐
                    │ Go test (host-gated: tmux+git on PATH, else t.Skip)     │
                    │                                                        │
                    │ 1. go build -o <sandbox>/bin/kamacu ./cmd/kamacu        │
                    │ 2. sandbox: HOME, TMUX_TMPDIR, temp DB, free port       │
                    │    env: HOME=<sbx> TMUX_TMPDIR=<sbx>/tmp                │
                    │         XDG_CONFIG_HOME=<sbx>/.config  PATH inherited   │
                    │         KAMACU_* scrubbed  FAKE_CLAUDE_* recorded       │
                    │         --claude-bin <repo>/internal/api/testdata/…     │
                    └───────┬────────────────────────────────────────────────┘
                            │ spawn + poll GET /api/healthz
                            ▼
   ┌──────────────────────────────────────────────────────────────────┐
   │ SERVER A: bin/kamacu serve --addr 127.0.0.1:P --db <sbx>/k.db     │
   │                                                                  │
   │  THE CURL STORY (real HTTP):                                     │
   │  PUT /api/global {root_path:<git repo>} ───────────────► 200     │
   │  PUT /api/settings/shell {value:"tmux"} ───────────────► 200     │
   │  POST /api/sessions {scope:global,kind:agent} ────────► 201      │
   │    └─ fake-claude runs in root; argv → FAKE_CLAUDE_ARGS_FILE      │
   │       (--session-id <uuid> = the persisted csid)                 │
   │  POST /api/sessions {scope:global,kind:bash} ─────────► 201      │
   │    └─ tmux server (separate process!) on <sbx>/tmux socket       │
   │  PUT /api/global {root_path:"<other>"} ────────► 409 + reasons   │
   │  PUT /api/global {root_path:""} ───────────────► 409 + reasons   │
   │  POST /api/sessions/{id}/stop (agent); stop/delete tmux tab      │
   │  PUT /api/global {root_path:""} ────────────────► 200 (ids       │
   │    cleared ← proven by: POST …/sessions {resume:true} ► 409)     │
   └───────┬──────────────────────────────────────────────────────────┘
            │ SIGTERM (no handler → hard death; PTY children die,
            │ tmux server + DB rows SURVIVE)
            ▼
   ┌──────────────────────────────────────────────────────────────────┐
   │ (quiescent window: DB readable directly if needed — no writer)    │
   └───────┬──────────────────────────────────────────────────────────┘
            │ spawn again, SAME --db + env
            ▼
   ┌──────────────────────────────────────────────────────────────────┐
   │ SERVER B (fresh process):                                        │
   │  boot sequence: migrate → BackfillGlobalTask → InstallPlugin     │
   │   (sandbox XDG) → tmux conf next to DB → sweepOrphanTmux         │
   │   (sandbox socket; global row KNOWN → tab survives)              │
   │                                                                  │
   │  GET /api/agents/status ──► ONE global entry: source "global",   │
   │    status "exited", resumable:true (needs the transcript fixture │
   │    at <sbx>/.claude/projects/*/<csid>.jsonl — harness wrote it)  │
   │  GET /api/sessions?scope=global ──► tmux row back, label kept,   │
   │    orphaned:true, global:true  (GAP-01 replay, D-49)             │
   │  tmux -L kamacu has-session =kamacu-global-N (harness probe,     │
   │    SAME TMUX_TMPDIR) ──► alive                                   │
   │  POST /api/sessions {scope:global,kind:agent,resume:true} ► 201  │
   │    └─ fake-claude argv now carries --resume <csid> (D-51)        │
   │  transcript fixture deleted → resume again ► 409 "no global      │
   │    claude session to resume" (D-51 transcript-gate edge)         │
   └──────────────────────────────────────────────────────────────────┘

   OPENCODE LEG (D-52) — same harness, DIFFERENT env posture:
   │ NOT HOME-sandboxed (real opencode needs real config/auth)
   │ HOME=real  TMUX_TMPDIR=<sbx>  --db=<sbx>/k2.db  free port
   │ PATCH /api/agents/{oc-id} {command:"<wrapper>"}   ← engine-locked,
   │   command is NOT (agents_crud.go:161-214)
   │ wrapper = #!/bin/sh; printf '%s\n' "$@" > $OC_ARGV_FILE; exec opencode "$@"
   │ spawn → argv (fresh, no -s) → `opencode run "Reply ok"` (out-of-band
   │   first turn, 120s budget, model errors tolerated — row created anyway,
   │   probe-verified on opencode 1.18.22) → capture poller persists ses_…
   │ stop → SIGTERM → SERVER B → resume:true → 201 + wrapper argv contains
   │   "-s","ses_…"  → stop → PATCH command back to "opencode" (cleanup)
```

### Recommended Project Structure

```
internal/api/
├── e2e_global_restart_test.go   # NEW — the real-binary harness (D-47..D-54),
│                                #   host-gated (tmux+git+go), SC1+SC2 one story
├── global_noleak_test.go        # NEW — consolidated TestGlobalNoLeak* family
│                                #   (D-55/D-56); absorbs/references
│                                #   TestGlobalActivityExclusion posture
├── global_interlock_test.go     # NEW — D-58 both directions (github seams)
├── sessions_global_test.go      # EXISTING — extend only if helpers extract
└── testdata/fake-claude         # EXISTING — used verbatim via --claude-bin
internal/mcp/
└── global_parity_test.go        # NEW — D-57 both-direction bridge spot-checks
cmd/kamacu/                      # (alternative harness home; see discretion)
web/src/api/types.ts             # EDIT — engine union widen (D-62)
web/src/pages/TaskPage.tsx       # EDIT — QuotaIndicator predicate (D-62)
.planning/phases/17-hardening-e2e/
└── 17-UAT.md                    # NEW at UAT time — walkthrough doc (D-59)
```

(Harness placement is explicitly the agent's discretion; `internal/api` keeps the `doJSON`/`gitRepo` helpers adjacent, but building the binary from a test in that package needs the module root via `../..` — `cmd/kamacu` is the other natural home. Either works; pick one and keep the helpers private.)

### Pattern 1: The host-gate + sandbox + restart harness helper

**What:** One setup function returns a handle that can (re)start the real binary against a stable sandbox.
**When to use:** every E2E test function.
**Example (shape, not verbatim):**

```go
// Source: composed this session from scripts/smoke.sh + cmd/kamacu/sweep_test.go
// + internal/api/sessions_global_test.go conventions
func newE2EServer(t *testing.T) *e2eServer {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil { t.Skip("tmux not on PATH") }
	if _, err := exec.LookPath("git"); err != nil { t.Skip("git not on PATH") }

	sbx := t.TempDir()
	t.Setenv("TMUX_TMPDIR", filepath.Join(sbx, "tmux")) // CRITICAL: see Pitfall 1
	// build once per test (go build cache makes repeats cheap)
	bin := filepath.Join(sbx, "kamacu")
	if out, err := exec.Command("go", "build", "-o", bin, "./cmd/kamacu").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out) // cmd.Dir must be the module root
	}
	// ... allocate free port, write env (HOME=sbx, scrub KAMACU_*, add FAKE_CLAUDE_*),
	// start() = exec + poll /api/healthz, stop() = SIGTERM + Wait with deadline
	// t.Cleanup: stop if running + kill tmux server on the sandbox socket
}
```

### Pattern 2: Transcript fixture for the claude resume gate

**What:** `transcriptExists(globRoot, csid)` globs `<globRoot>/*/<csid>.jsonl` where globRoot = `~/.claude/projects` resolved from HOME (resume.go:16-36). In the HOME-sandboxed harness the fixture is a file the test writes anywhere under `<sbx>/.claude/projects/<anydir>/<csid>.jsonl`; deleting it flips resumable off and turns resume into the 409.
**When to use:** every claude resume/resumable assertion (D-51, D-54).

```go
// Source: internal/api/resume.go:16-25 (the glob shape) + agents_test.go:95 fixture idiom
csid := csidFromFakeArgv(t, argsFile) // parse "--session-id <uuid>" or "--resume <uuid>"
projDir := filepath.Join(sbx, ".claude", "projects", "e2e-fixture")
os.MkdirAll(projDir, 0o755)
os.WriteFile(filepath.Join(projDir, csid+".jsonl"), []byte("{}\n"), 0o644)
```

### Pattern 3: The opencode wrapper agent (argv proof with the real binary)

**What:** PATCH the system OpenCode agent's `command` to a wrapper; restore in cleanup. Engine stays locked `opencode` (system lock is engine-only — agents_crud.go:161-165), so the engine-branched resume path (`-s <ocsid>` append, sessions.go:571-574) is exercised verbatim while argv is recorded.
**When to use:** D-52 only (the real-opencode leg).

```sh
#!/bin/sh
# wrapper: record argv, then become the real opencode
printf '%s\n' "$@" > "${OC_ARGV_FILE}"
exec opencode "$@"
```

```go
// drive first turn out-of-band (TestGlobalOpencodeCaptureHost precedent, :1162-1168)
cmd := exec.CommandContext(ctx, "opencode", "run", "Reply with just: ok")
cmd.Dir = root; cmd.Env = append(os.Environ(), "PWD="+root)
// model errors tolerated — the session row is created regardless (probe-verified 1.18.22)
```

### Pattern 4: Consolidated leak family + audit table row shape

**What:** One test family walks every surface with a configured + LIVE global session (agent via fake-claude + a tmux tab), asserting zero global presence. The VERIFICATION doc carries the mirror-image table so adding a surface later = one row + one subtest.

| Surface | Query / guard (file:line) | Assertion |
|---|---|---|
| Unscoped task list | `GET /api/tasks` — `WHERE source='manual'` (tasks.go:194) | response contains no global-titled row; count unchanged pre/post global spawn |
| Board fetch | `GET /api/projects/{id}/tasks` (tasks.go:233) | same |
| Move/position arithmetic | tasks.go:288/432/538/554 (`source='manual'` on all MIN/ORDER queries) | a move of a manual task still 200s; positions unaffected by global presence |
| Project list | `GET /api/projects` | no sentinel project; count unchanged after configuring global |
| Workspace delete guard | `SELECT COUNT(*) FROM projects WHERE workspace_id=?` (workspaces.go:215) | a workspace with zero projects deletes 200 with a live global (global never counts) |
| Activity stats/lists | `GET /api/activity` (task-keyed; manual filter activity.go:135) | no global rows in tasks[]/stats with a live global session (Phase-15 `TestGlobalActivityExclusion` posture, re-homed) |
| MCP task tools | `list_tasks` → `GET /api/tasks` passthrough (mcp/tasks.go:197) | bridge result has no global entity |
| MCP session tools | `list_sessions`/`get_session` → `/api/sessions[/{id}]` passthrough (mcp/sessions.go:516,579) | global session LISTED with `taskTitle:"Scratchpad"/projectName:"Global"` labels; `get_session` of the global id 200s, never 404 (D-57 honest direction) |
| DB structural proof | `SELECT COUNT(*) FROM tasks/projects` with configured+live global | tables gained zero rows — exclusion by CONSTRUCTION, not filtering |

### Anti-Patterns to Avoid

- **Don't re-verify Phase-16 UX** (the 409 dialog, 7-state matrix, bar click-through) — 16-UAT already did, live. Phase 17 hits the same gates through the API in the E2E narrative.
- **Don't add a sentinel row "just for the test"** — the whole sweep proves the singleton design; any fixture that inserts a tasks/projects row for the global would be testing a fiction.
- **Don't run the opencode leg HOME-sandboxed** — real opencode resolves config/auth from real HOME; sandboxing breaks it exactly the way `TestGlobalOpencodeCaptureHost` avoided (its doc comment, sessions_global_test.go:1131-1133).
- **Don't spawn more in-process restart-sims** — D-47 explicitly supersedes them; sims exist (`TestGlobalStatusPostRestart`, `TestGlobalSessionRestartOrphan`) and stay as fast regression layers.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---|---|---|---|
| claude engine stand-in | A new fake agent binary | `internal/api/testdata/fake-claude` via `--claude-bin` | Already records argv+pwd, traps TERM (exit 143), has `FAKE_CLAUDE_RESUME_FAIL`; the in-process suites lock their contract |
| Managed-clone without network | Real `gh` calls in tests | `github.SetValidateRunnerForTest` / `SetCloneRunnerForTest` / `SetAvailableForTest` + `fakeGitClone` | The Phase-14 seam precedent (global_test.go:355-372, 386-425); deterministic, offline |
| MCP transport for spot-checks | A real MCP client | `bridge` struct + httptest backend (house "in-memory transport") | `internal/mcp/sessions_test.go:46-47` pattern; asserts the bridge's actual handler output |
| tmux liveness probing | New probe code | `tmux.Client.HasSession` with `"="+name` exact-match | Prefix-matching gotcha (tmux.go:78-89, Empirical Finding 2) — `HasSession` already handles it; harness probes just need the sandbox `TMUX_TMPDIR` in env |
| Server readiness | Sleeps | Poll `GET /api/healthz` | smoke.sh's 50×100ms loop; deterministic |
| Free port | Fixed ports | `net.Listen("tcp","127.0.0.1:0")` → close → use | `go test ./...` runs packages in parallel; fixed ports collide |

**Key insight:** every deceptively hard piece of this phase (PTY semantics, tmux exact-match, clone flows, bridge calls) already has an in-repo instrument. The ONLY genuinely new machinery is the process-spawn/restart loop itself — spend the effort there.

## Common Pitfalls

### Pitfall 1: The shared tmux socket — the test's sweep can kill the USER's real sessions (and vice versa)

**What goes wrong:** The serve command hardcodes `tmux.Client{Socket: tmux.DefaultSocket}` ("kamacu", serve.go:269). tmux puts `-L` sockets in `$TMUX_TMPDIR/tmux-<uid>/` (or `/tmp/tmux-<uid>/`). If the harness spawns the real binary without isolating the socket dir, THREE failure modes fire:
1. Server B's startup `sweepOrphanTmux` lists sessions on the SHARED socket, finds the user's real live `kamacu-*` tabs, sees no rows for them in the TEST DB, and **kills the user's real work** (`sweepOrphanTmux` kills any `kamacu-*`-prefixed session with no known row — serve.go:362-420).
2. The user's real kamacu (if it restarts mid-test) kills the test's tabs the same way.
3. The test's `has-session` probes and the server's tmux calls race on one socket.

**Why it happens:** The socket NAME is deliberately shared in production (one tmux server per user); only the socket DIRECTORY separates instances.

**How to avoid:** Set `TMUX_TMPDIR=<sandbox>/tmp` in the spawned server's env AND in every harness-side tmux probe. Verified empirically this session on host tmux 3.4: with the var, the socket lives at `<sbx>/tmux-1000/kamacu` and probes WITHOUT the var fail with "error connecting to /tmp/tmux-1000/kamacu" — isolation is complete in both directions. Cross-validated against man7 tmux(1): "tmux stores the server socket in a directory under TMUX_TMPDIR or /tmp if it is unset." [VERIFIED: empirical probe + tmux(1)]

**Warning signs:** The variable is spelled **`TMUX_TMPDIR`** — `TMUX_TMP_DIR` (underscored) is silently ignored and gives NO isolation (verified: my first probe used the wrong name and the socket landed in the default dir). Also: cleanup should `KillServer` the sandbox socket so the tmux server process doesn't outlive the test.

### Pitfall 2: `KAMACU_*` env inheritance — tests run RED (or worse, biased) inside Kamacu sessions

**What goes wrong:** This agent session itself exports `KAMACU_SESSION_ID`, `KAMACU_HOOK_TOKEN`, `KAMACU_HOOK_BASE` (verified: 3 vars present). Any `go test` spawned from a Kamacu terminal (agent or bash tab) inherits them; `TestCustomEngineDoesNotGetHookEnv` then FAILS because the custom spawn arm inherits `os.Environ()` verbatim (manager.go:187) and the env-dump stub sees the parent's vars. This is the CURRENT red test's actual root cause — not a production regression: `env -u KAMACU_SESSION_ID -u KAMACU_HOOK_TOKEN -u KAMACU_HOOK_BASE go test ./internal/session -run TestCustomEngineDoesNotGetHookEnv` → PASS (verified this session).

**Why it happens:** The test asserts absence of vars the parent may legitimately carry; the code's injection gate (`opts.AgentEngine == "opencode"`, manager.go:190) is correct.

**How to avoid (pick at implementation, both defensible):**
- **Root-cause fix (recommended):** the custom arm strips `KAMACU_SESSION_ID/HOOK_TOKEN/HOOK_BASE` from the inherited env (then the opencode gate re-adds them). Bonus: the parent's HOOK_TOKEN is a secret — currently propagating into arbitrary custom-agent children when Kamacu itself spawns the server/test. This honors "custom agents get none of these" (D014) in ALL contexts, not just clean shells.
- **Test-side fix:** the test scrubs the three vars before spawning (`os.Unsetenv` + `t.Cleanup` restore — `t.Setenv` cannot unset). Keeps production untouched but leaves the secret-propagation issue.

The E2E harness must scrub `KAMACU_*` from the spawned server's env regardless (it would otherwise run as a "nested" Kamacu child — harmless to the server itself, which mints its own token, but dishonest isolation).

**Warning signs:** any new test that asserts on child-env contents; verification runs that pass "sometimes" depending on which terminal ran them.

### Pitfall 3: HOME sandbox vs real opencode — one recipe does not fit both legs

**What goes wrong:** HOME-sandboxing the server breaks the real opencode leg (config/auth resolve from real HOME), while NOT sandboxing the claude leg pollutes the real `~/.claude/projects` and `~/.kamacu`-adjacent surfaces.

**How to avoid:** Two explicit env postures, both under D-47's "one harness":
- **claude/tmux/interlock legs:** `HOME=<sandbox>` — DB via `--db <sbx>/k.db`, transcript fixtures under `<sbx>/.claude/projects/`, `XDG_CONFIG_HOME=<sbx>/.config` (keeps `opencode.InstallPlugin`'s boot write inside the sandbox), managed-clone namespace under `<sbx>/.kamacu/repos/…`.
- **opencode leg (D-52):** real HOME + temp `--db` + sandboxed `TMUX_TMPDIR` — exactly `TestGlobalOpencodeCaptureHost`'s NOT-sandboxed posture (sessions_global_test.go:1131-1133). The only state that matters (the singleton row) lives in the temp DB; a folder root (temp git repo) never touches `~/.kamacu` paths.

**Warning signs:** opencode spawn exiting immediately (no config), or fixtures appearing in the real `~/.claude`.

### Pitfall 4: Stale D-63 inputs — `TestInput_*` is already green; don't plan a phantom fix

**What goes wrong:** The CONTEXT (via the Phase-13/14 deferred-items docs) carries the `TestInput_*` family as red. All three PASS on this branch now (verified by live runs this session) — Phase 15's delimiter fix (commit 67f39ef, ESC[2004~ → ESC[200~) healed them. Planning a "bracketed-paste investigation" burns a task on nothing.

**How to avoid:** D-63's task becomes: (a) run the full `go test ./...` baseline first (Wave 0) to enumerate the ACTUAL red set — expected: only `TestCustomEngineDoesNotGetHookEnv`, and only under Kamacu (Pitfall 2); (b) fix per Pitfall 2; (c) record the baseline in the SUMMARY. If the full suite surprises (it's ~4-5 min for `internal/api`), the task absorbs whatever it finds — that's the "investigate-and-fix" mandate.

**Warning signs:** any plan task named after a specific red test without a fresh baseline run in the same session.

### Pitfall 5: resume assertions need the transcript fixture BEFORE resumable shows up

**What goes wrong:** After SIGTERM+restart, `GET /api/agents/status`'s global entry shows `resumable:true` only if `transcriptExists(globRoot, csid)` finds `<globRoot>/*/<csid>.jsonl` (agents.go:329 gate; globRoot = HOME-derived `~/.claude/projects`). A harness that never writes the fixture sees `resumable:false` and a `resume:true` POST 409s — looking like a restart bug when it's a missing fixture.

**How to avoid:** Parse the fresh csid from fake-claude's argv file (`--session-id <uuid>`), write the fixture (Pattern 2), then assert resumable/201; then DELETE the fixture and assert the 409 — one fixture powers both the positive and the D-54 negative. [VERIFIED: resume.go:16-36, sessions.go:519/534, agents.go:329]

**Warning signs:** resumable:false with a valid persisted csid; "no global claude session to resume" despite the id.

### Pitfall 6: the clear-then-reconfigure story must assert id-clearing WITHOUT reading ids

**What goes wrong:** D-48 wants "persisted resume ids are cleared" observable. The GET /api/global wire does NOT expose resume ids (D-17 shape: root/github_repo/agent/live counts). Reading the SQLite DB while server A runs risks lock contention assumptions.

**How to avoid:** The observable proxy is already locked in the CONTEXT: after the successful root change, `POST /api/sessions {scope:global,kind:agent,resume:true}` → 409 "no global claude session to resume" (the D-31 gate keys on the now-NULL csid). Zero DB reads needed for the whole claude story (csid from argv; ocsid by `ses_` prefix from the wrapper argv). If a belt-and-braces DB check is wanted, do it in the quiescent window between SIGTERM and server B (no writer).

### Pitfall 7: D-62's predicate flip has a loading-state nuance

**What goes wrong:** `isClaudeAgent = projectEngine !== "custom"` (TaskPage.tsx:64) SHOWS the claude-quota chip during loading (`undefined`) and — the actual bug — for runtime `"opencode"` agents (the stale `"claude" | "custom"` union makes TS blind to it; types.ts:62). Switching to `projectEngine === "claude"` fixes opencode but flips loading from show→hide.
**How to avoid:** Widen the union to `"claude" | "custom" | "opencode"` (matches the Go wire + 00015 seed) and align the predicate with the /global view's locked rule (`engine === "claude"`, 16-RESEARCH Pitfall 2). The loading-flip is a cosmetic improvement (no chip until agents resolve — honest, not broken); note it in the task rather than engineering around it. `tsc`/build + lint-clean-in-isolation are the gates (new-file lint rule doesn't apply — these are edits).

### Pitfall 8: parallel-package test collisions and the api package's runtime budget

**What goes wrong:** `go test ./...` runs packages in parallel. The api package already takes ~190-215s (PTY/tmux suites); adding the E2E (build + two server lifecycles + opencode turn, ~60-150s) stretches it further. Fixed ports, shared sockets, or shared HOMEs would collide with sibling packages.
**How to avoid:** Everything sandbox-unique (TempDir per test, allocated port, TMUX_TMPDIR, temp DB) keeps the E2E parallel-safe. Consider `-timeout` guards on the E2E test itself (Go default 10m is fine; per-step polls 5-60s). Don't serialize what doesn't need it.

### Pitfall 9: wrapper-agent cleanup ordering (opencode leg)

**What goes wrong:** The D-52 wrapper PATCHes the system OpenCode agent's command; if the test fatals before restoring, the temp DB is deleted anyway — BUT if the harness ever points at a persistent DB, a leftover wrapper would poison it. Also the restore must happen before `t.Cleanup` kills servers (order matters if later asserts re-POST).
**How to avoid:** `t.Cleanup(restoreCommand)` registered IMMEDIATELY after the PATCH succeeds; use a dedicated temp DB for this leg so even a missed restore is contained.

### Pitfall 10: UAT safety posture is a deliverable, not boilerplate

**What goes wrong:** SC5's no-worktree documentation reads as a disclaimer paragraph nobody writes seriously.
**How to avoid:** The UAT doc must name the concrete risk (PITFALLS.md P3): a `--dangerously-skip-permissions` agent writing directly into the checkout the user picked — no worktree net, no diff tab, no gated cleanup. Suggest (but don't require) running the UAT against a disposable clone if the user is wary of dogfooding the kamacu checkout itself.

## Code Examples

### The wire surfaces the curl story drives (all verified in-repo this session)

```text
# Source: internal/api/global.go (put/get), sessions.go (create), settings.go (put)
PUT  /api/global               {"root_path": "<abs git repo>"}        → 200 (folder root)
PUT  /api/settings/shell       {"value": "tmux"}                      → 200 (tmux bash tabs)
POST /api/sessions             {"scope":"global","kind":"agent"}      → 201 (fake-claude in root)
POST /api/sessions             {"scope":"global","kind":"bash"}       → 201 (tmux: kamacu-global-N)
PUT  /api/global               {"root_path": ""}                      → 409 {error, reasons[{kind,target}]} while live
POST /api/sessions/{id}/stop                                            → 200 (agent); delete tmux tab via DELETE /api/sessions/{id}
PUT  /api/global               {"root_path": ""}                      → 200 (cleared; ids cleared)
POST /api/sessions             {"scope":"global","kind":"agent","resume":true}
                                 → 409 "no global claude session to resume"   (id-clearing proof)
— SIGTERM; restart same --db —
GET  /api/agents/status        → one {source:"global", taskId:0, projectId:0,
                                     projectName:"Global", taskTitle:"Scratchpad",
                                     status:"exited", resumable:true, engine:"claude|opencode"}
GET  /api/sessions?scope=global → tmux row {orphaned:true, global:true, label:"Bash N",
                                     tmuxName:"kamacu-global-N"}            (D-49)
POST /api/sessions             {"scope":"global","kind":"bash","reattach_tmux_name":"kamacu-global-N"}
                                                                      → 201, label preserved (GAP-01 replay)
POST /api/sessions             {…,"resume":true}                        → 201; argv: ["--resume","<csid>"]
```

Request field spellings verified: `scope`/`kind`/`resume`/`reattach_tmux_name` (sessions.go:337-349); settings value body `{"value":…}` (settings.go:54-76); 409 body `{error, reasons:[{kind,target}]}` (global.go:344-348).

### Server process management (the restart loop core)

```go
// Source: mechanics verified from scripts/smoke.sh + serve.go this session
proc := exec.Command(bin, "serve", "--addr", addr, "--db", dbPath, "--claude-bin", fakeClaude)
proc.Env = childEnv // HOME/TMUX_TMPDIR/XDG_CONFIG_HOME/PATH/FAKE_CLAUDE_*; KAMACU_* scrubbed
proc.Stdout, proc.Stderr = logBuf, logBuf // capture for failure diagnostics
proc.Start()
pollHealthz(t, baseURL, 5*time.Second) // GET /api/healthz until 200

proc.Process.Signal(syscall.SIGTERM) // no handler exists — hard death by design
done := make(chan error, 1)
go func() { done <- proc.Wait() }()
select {
case <-done: // exited — restart is just a fresh exec with identical args/env
case <-time.After(10 * time.Second):
	proc.Process.Kill(); t.Fatalf("server ignored SIGTERM")
}
```

### The interlock test skeleton (D-58, in-package, seams — no network)

```go
// Source: global_test.go:386-425 seam usage, projects.go gated delete, global.go:153-207
defer github.SetCloneRunnerForTest(func(_ context.Context, ref, dest string) (string, error) {
	return "", fakeGitClone(t, ref, dest) // real dirs, origin remote set
})()
defer github.SetValidateRunnerForTest(func(_ context.Context, _ string) (string, bool, error) {
	return "octo/widgets", true, nil
})()
defer github.SetAvailableForTest(true)()

// project side: POST /api/projects {"repo":"Octo/Widgets"} → clone at ~/.kamacu/repos/octo/widgets
// global side:  PUT /api/global {"repo":"Octo/Widgets"}    → clone at ~/.kamacu/repos/global/octo/widgets
// (same canonical ref allowed by construction — D-03; the TEST proves namespace separation)

// Direction 1: DELETE /api/projects/{id} (no tasks → no worktrees → gates clear,
// os.RemoveAll on the PROJECT clone only — projects.go:913-914)
//   assert: project clone dir GONE; global root dir STILL EXISTS + still configured
// Direction 2: PUT /api/global {"root_path":""} (clear)
//   assert: global row cleared; project clone dir STILL EXISTS + project row intact
// Folder direction (cheap): folder root inside a temp dir; delete a folder-project; neither touches the other
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|---|---|---|---|
| Manual curl-smoke transcripts (Phase 14/02) | Automated real-binary Go harness (D-47) | this phase | restart/reconfigure gates become CI/regression-covered instead of one-time evidence |
| In-process restart sims (Phase 15) | Sims stay as fast layer + real-binary E2E on top | this phase | sims catch logic drift cheaply; E2E catches process-boundary bugs (boot order, sweep, env resolution) |
| Deferred red tests ("environment-dependent") | Root-caused: Kamacu-session env inheritance | this session | the "flake" was deterministic context — fixable in production code |

**Deprecated/outdated (this phase retires):**
- The "TestInput_* is red" belief (Phase-13/14 deferred-items docs) — superseded by Phase-15's 67f39ef; this research's live runs are the current truth.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The full-suite red set is exactly `{TestCustomEngineDoesNotGetHookEnv}` (under Kamacu) — based on targeted live runs of the four named tests, not a full `go test ./...` baseline | D-63 / Pitfall 4 | Another red test lurks elsewhere; mitigation is the Wave-0 baseline run already recommended as D-63's first step |
| A2 | The `exec opencode "$@"` wrapper is transparent to opencode's interactive TUI behavior (PTY, config resolution, session row creation) | Pattern 3 / D-52 | If the wrapper perturbs the TUI, fall back to asserting the fresh-vs-resume argv with the committed `testdata/fake-opencode` stub at API level and leave "process returns" to the UAT (D-53) — the capture/restart/`ses_` mechanics themselves are host-proven (`TestGlobalOpencodeCaptureHost` PASS, 15-VERIFICATION) |
| A3 | `go build ./cmd/kamacu` from within a test works with `cmd.Dir=<module root>` on this checkout (web/dist placeholder committed; go toolchain present because `go test` is running) | Pattern 1 | If the embed ever fails on fresh clones, the harness can `make build` (needs node) — unlikely; placeholder verified in .gitignore |
| A4 | The user's real kamacu install may be running concurrently on this host during tests (unknown — not probed; irrelevant given TMUX_TMPDIR + free-port isolation, which is mandatory anyway) | Pitfall 1 | None — isolation is unconditional |
| A5 | `XDG_CONFIG_HOME` set in the child env suffices to keep `opencode.InstallPlugin`'s boot write inside the sandbox (it reads `${XDG_CONFIG_HOME:-~/.config}`) | Pattern 1 env posture | Worst case: a status-plugin file lands in the real ~/.config/opencode — the file is idempotently regenerated at every real boot anyway (warn-only, serve.go:222-227); harmless |

**All other claims in this document were verified this session** — by direct file reads (file:line cited), live test runs, an empirical tmux probe, or env reproduction. `[CITED]`-grade: the man7 tmux(1) cross-validation.

## Open Questions

1. **Harness file placement + helper sharing (agent's discretion, decide at planning)**
   - What we know: `internal/api` has the adjacent helpers (`doJSON`, `gitRepo`, `worktreeTask`); `cmd/kamacu` is the SUT's package; both are viable homes; the binary build needs the module root (`../..` from `internal/api`, `..` from `cmd/kamacu`).
   - What's unclear: whether extracting shared helpers out of `sessions_global_test.go` is worth the churn.
   - Recommendation: one new file in `internal/api` (helpers stay unexported, duplicated where trivial); do NOT refactor the existing suite — Phase-15's "byte-for-byte untouched" posture is worth preserving.

2. **Whether the opencode leg's out-of-band `opencode run` needs the wrapper too**
   - What we know: the capture poller discovers sessions by directory (PWD-pin); `opencode run` in the root creates the row (host-proven at 1.18.22).
   - What's unclear: nothing material — run it exactly as `TestGlobalOpencodeCaptureHost` does (:1162-1168).
   - Recommendation: copy that block verbatim; only the SPAWNED session goes through the wrapper agent.

3. **D-63 production-fix vs test-fix (locked: "investigation decides")**
   - What we know: the root cause is fully characterized (Pitfall 2); the production fix is small (strip 3 vars in the custom arm before the opencode gate re-adds them) and closes a secret-propagation hole; the test fix is smaller but leaves the hole.
   - Recommendation: production fix + a comment in the test documenting the under-Kamacu context; keep the test asserting absence (it then passes everywhere).

4. **Does the E2E run in CI?** No CI config exists in-repo (no .github/workflows found); D-50 says "inline in `go test ./...`" — host-gating makes it self-selecting (skips where tmux/git/opencode absent). No action needed; noted so the planner doesn't invent a CI story.

## Environment Availability

Probed this session on the dev host (all required tools present):

| Dependency | Required By | Available | Version | Fallback |
|---|---|---|---|---|
| go | harness build + all tests | ✓ | go1.26.0 linux/amd64 | — |
| tmux | SC2 tmux survival, sweeps, interlock | ✓ | 3.4 | tests `t.Skip` (house host-gate) |
| git | repo fixtures, clone seams' fakeGitClone | ✓ | 2.43.0 | `t.Skip` |
| opencode | D-52 real-binary leg | ✓ | 1.18.22 (`~/.opencode/bin`) | `t.Skip` — D-52's live-argv proof degrades to the API-level + UAT halves |
| claude (real) | NOT required — D-51 forbids real-claude gates | ✓ (2.1.247) | unused by tests | UAT only (D-53) |
| gh | not needed (clone flows run via seams) | ✓ | 2.82.0 | — |
| node/npm | NOT required — `web/dist/index.html` placeholder is committed; binary embeds without a frontend build | ✓ | v24.4.1 / 11.6.0 | `make build` if embed ever fails |
| free loopback ports | harness servers | ✓ | — | `127.0.0.1:0` allocation |

**Missing dependencies with no fallback:** none.
**Missing dependencies with fallback:** none missing; host-gates cover the conditional ones.

## Validation Architecture

**SKIPPED — `workflow.nyquist_validation: false` in `.planning/config.json` (verified this session).** The phase's verification contract is the GSD verifier + the tests the phase itself ships (which is the phase's entire point) + the 17-UAT walkthrough.

## Security Domain

`security_enforcement` is absent from config (→ enabled). Phase 17 ships tests + a two-line type cleanup + the D-63 fix; it adds no new endpoints, no UI surfaces, no crypto. Applicable categories:

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---|---|---|
| V2 Authentication | no | unchanged (localhost, no-auth by design) |
| V3 Session Management | no (only exercised, not changed) | existing PTY/session machinery |
| V4 Access Control | no | loopback bind + hostCheck stay as-is (main_test.go locks them) |
| V5 Input Validation | no new inputs | E2E drives existing validated endpoints |
| V6 Cryptography | no | hook token machinery untouched (per-start random, in-memory — serve.go:224-236) |
| V14 Config (adjacent) | yes (test isolation) | the sandbox env recipe IS the control: fresh HOME, temp DB, isolated tmux socket, scrubbed `KAMACU_*` secrets |

### Known Threat Patterns for this phase's surface

| Pattern | STRIDE | Standard Mitigation |
|---|---|---|
| Test harness binds non-loopback / collides with user services | Tampering/DoS | `--addr 127.0.0.1:<allocated>` only (ensureLoopback would reject wildcards anyway) |
| Test's orphan sweep kills user's real tmux sessions | Denial-of-Service (user data) | `TMUX_TMPDIR` sandbox isolation — Pitfall 1, mandatory |
| `KAMACU_HOOK_TOKEN` (secret) propagates into arbitrary custom-agent child env when Kamacu nests | Information Disclosure | D-63 production fix (strip in custom arm); harness env scrubbing regardless |
| Fake binaries on PATH shadowing real tools in tests | Spoofing | `--claude-bin` absolute path to testdata; wrapper via absolute path; no PATH games |
| E2E leaves stray processes (servers, tmux servers, fake agents) | DoS (host) | `t.Cleanup` LIFO: stop servers, KillServer the sandbox socket, wait all children |

## Sources

### Primary (HIGH confidence — read/verified directly this session)
- `cmd/kamacu/serve.go` — full boot sequence, flags (`--addr/--db/--claude-bin/--dev-origin/--insecure-allow-remote`), no-signal-handler restart semantics (:345-350), hardcoded tmux socket (:269), sweepOrphanTmux (:362-420), healthz (:295), opencode plugin install + tmux conf placement
- `internal/api/sessions.go` — global spawn gates (D-28/29/30/33/34), engine-branched resume incl. transcriptExists call sites (:496-545), opencode `-s` append (:571-574), reattach field + scoped lookup (:344-349, :588-604), request wire tags (:337-349)
- `internal/api/resume.go:16-36` — transcriptExists glob + defaultTranscriptGlobRoot (~/.claude/projects via HOME)
- `internal/api/global.go` — PUT dispatch/409 gate/clear semantics (:292-420), managed dest namespace (:177), putManagedRoot atomicity (:153-207), D-27 ~/.kamacu block
- `internal/api/projects.go` — reposBase (:355), gated delete → os.RemoveAll on clone (:885-914)
- `internal/api/workspaces.go:215` — non-empty delete COUNT guard
- `internal/api/tasks.go:194/233/288/432/538/554` — the 5 board/position `source='manual'` surfaces + unscoped list
- `internal/api/agents_crud.go:81-214` — custom-create engine, system engine-lock (command editable), `engine must be 'claude' or 'custom'` update gate
- `internal/session/manager.go:160-230` — custom/opencode spawn arm, os.Environ inheritance, opencode-gated KAMACU_* injection (:187-194)
- `internal/tmux/tmux.go` — DefaultSocket (:22), exact-match `"="+name` (:78-103)
- `internal/api/sessions_global_test.go` — the full Phase-15 suite: harness shapes (:26-106, :700-836), restart-sim (:614-660), fake-claude argv helpers, host opencode e2e (:1134-1193)
- `internal/api/global_test.go` — gate tests + seams + fakeGitClone + globalManagedDest (:340-425)
- `internal/api/testdata/fake-claude` — the recorder stub (argv/pwd/RESUME_FAIL/TERM-trap)
- `internal/mcp/sessions_test.go:23-47`, `internal/mcp/tasks.go:178-197` — bridge test pattern + task-tools passthrough
- `scripts/smoke.sh` — build/spawn/healthz/kill/restart loop (the manual prior art)
- `web/src/api/types.ts:62`, `web/src/pages/TaskPage.tsx:64,535` — D-62 exact edit sites
- `.gitignore` — `web/dist/*` + `!web/dist/index.html` placeholder (embed-safe builds)
- Live runs: the three `TestInput_*` tests PASS; `TestCustomEngineDoesNotGetHookEnv` FAILS under Kamacu env, PASSES with the three vars unset
- Empirical tmux probe (this host, tmux 3.4): `TMUX_TMPDIR` socket isolation works both directions; wrong var name (`TMUX_TMP_DIR`) silently does nothing
- `.planning/phases/15-global-sessions-backend/15-VERIFICATION.md` — deferred table (this phase's mandate), 17/17 truths, DEV-1 delimiter fix history
- `.planning/phases/13|14-*/deferred-items.md` — the D-63 inputs (now partially stale — see Pitfall 4)
- `.planning/phases/16-*/16-UAT.md`, `16-RESEARCH.md` (Pitfall 2), `16-VERIFICATION.md` — walkthrough pattern, D-62 flag origin, what's already live-verified
- `.planning/research/PITFALLS.md` — P1/P2/P4/P5 invariants the E2E proves; P3 safety posture for the UAT
- `.planning/REQUIREMENTS.md` — all 19 v1.13 reqs checked; line 105 gate assignment

### Secondary (MEDIUM confidence)
- man7.org tmux(1) — "tmux stores the server socket in a directory under TMUX_TMPDIR or /tmp if it is unset" (cross-validation of the empirical probe)
- github.com/tmux/tmux issue #1646, superuser.com/questions/1758465 — corroborating TMUX_TMPDIR usage

### Tertiary (LOW confidence)
- None used.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — nothing new; every instrument verified in-repo
- Harness mechanics: HIGH — boot order, env resolution, signal semantics, wire shapes all read at file:line; the two genuinely empirical facts (TMUX_TMPDIR, KAMACU_* inheritance) reproduced live this session
- Pitfalls: HIGH — Pitfalls 2/4/5 are live-reproduced, not inferred; 1 is empirically + doc verified
- Red-test picture: MEDIUM — targeted runs only; full-suite baseline deferred to Wave 0 (A1)

**Research date:** 2026-08-27
**Valid until:** 2026-09-27 (stable, codebase-anchored; re-verify the red-test table if Phase 16→17 interim commits land)


