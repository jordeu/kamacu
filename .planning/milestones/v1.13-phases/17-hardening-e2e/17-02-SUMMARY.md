---
phase: 17-hardening-e2e
plan: "02"
subsystem: testing
tags: [go-test, regression, sentinel-leak, gint-03, mcp-bridge, interlock, worktree-namespace]

requires:
  - phase: 15-global-sessions-backend
    provides: global spawn/status wire surface, the singleton architecture being proven leak-free, sessions_global_test harness conventions
  - phase: 14-global-config-api
    provides: PUT/GET /api/global grammar, managed-root namespace + Set*ForTest seams, D-21 clear semantics
  - phase: 13-global-data-foundation-safety-net
    provides: the singleton/not-a-task-row representation (D-01..D-03) the exclusion-by-construction proof exercises
  - phase: 17-hardening-e2e/01
    provides: the post-restart orphaned global row shape (orphaned:true + global:true) seeded into the MCP fixture
provides:
  - TestGlobalNoLeak — the consolidated per-surface leak family with a LIVE global agent + live global tmux tab (D-55/D-56)
  - TestGlobalInterlock* — both-direction managed-root ↔ project-delete interlock with deterministic filesystem assertions (D-58/SC3)
  - TestGlobalParity* — MCP bridge both-direction spot-checks: task tools leak-free, session tools honestly labeled, never 404 (D-57/GINT-02)
  - The D-55 audit table (below) — surface → query/guard file:line → assertion, destined for 17-VERIFICATION.md
affects: [17-VERIFICATION (carries the audit table), 17-04 (UAT references the proven surfaces), future enumeration surfaces (one row + one subtest pattern)]

tech-stack:
  added: [] # stdlib + in-repo seams only — zero new dependencies, zero production changes
  patterns:
    - "Per-surface leak subtest: one parent harness with LIVE global state (agent + tmux tab) + a live-state precondition (GET /api/global live counts) so every negative assertion is proven meaningful"
    - "Row-level negative assertions for path-carrying bodies (t.TempDir embeds the test name, and 'TestGlobalNoLeak' contains 'Global') — raw-byte leak checks reserved for bodies that carry no paths (Activity)"
    - "Interlock fake clone = committed clean tree + refs/remotes/origin/HEAD → remote-tracking main == HEAD, so the managed-delete git gates (DefaultBranch/Dirty/Unpushed/Stash) clear at 204 — the plain init+remote-add fake leaves origin/HEAD absent, which deleteManaged treats as a conservative blocker"
    - "Seam installs in a shared helper ride t.Cleanup — a defer inside the helper restores at helper exit, before any request runs"

key-files:
  created:
    - internal/api/global_noleak_test.go
    - internal/api/global_interlock_test.go
    - internal/mcp/global_parity_test.go
  modified: []

key-decisions:
  - "Empty-workspace DELETE asserted as 204 (the shipped wire, workspaces.go:232 + TestWorkspaceDeleteEmpty precedent) — the plan text said 200; resolved toward the locked contract like 17-01's stop-endpoint alignment"
  - "The orphaned+global row variant is locked as FILTERED by the bridge's D-13 operable-ids contract (isOrphanedRow drops id=\"\" rows, scope-agnostic) — the plan's 'survives the passthrough' wording contradicted the shipped filter, and the plan's own 'no production code changes' constraint means the test locks the shipped behavior; the LIVE global row (real operable id) is the honest-direction surface and is proven labeled-intact and never-404"
  - "interlockFakeClone instead of verbatim fakeGitClone: the gated project delete needs origin/HEAD + a clean committed tree to reach 204; documented in-file"
  - "Fourth seam (SetDescriptionRunnerForTest) installed in the interlock fixture: createByRepo captures a repo description when Available() is forced true — without it the test could exec a real gh; keeps the fixture offline per the task's own 'no network, no real gh' requirement"
  - "List/board/project leak checks operate on decoded row fields, not raw bytes: those bodies embed worktree_path under t.TempDir, whose path contains the test NAME — 'TestGlobalNoLeak' contains 'Global' and raw-byte checks false-positive; Activity (path-free body) keeps the plan-mandated raw-bytes posture"

patterns-established:
  - "Pattern: audit-table-driven leak proof — every enumeration surface gets one row (surface → guard file:line → assertion) plus one subtest; adding a future surface = one row + one subtest, reviewable in one diff (T-17-06)"
  - "Pattern: interlock-by-construction proof — same canonical ref cloned into BOTH namespaces, then each delete direction asserted with os.Stat on exactly the two directories; no cross-entity delete is possible because no cross-entity path exists"

requirements-completed: [GINT-02, GINT-03]

coverage:
  - id: D1
    description: "TestGlobalNoLeak family: live global agent + live global tmux tab; per-surface leak assertions (unscoped list, board fetch, move/positions, project list count, workspace guard 204+409, Activity raw bytes) + DB structural COUNT proof"
    requirement: GINT-03
    verification:
      - kind: integration
        ref: "internal/api/global_noleak_test.go#TestGlobalNoLeak"
        status: pass
    human_judgment: false
  - id: D2
    description: "Both-direction managed-root ↔ project-delete interlock with deterministic filesystem assertions (same-ref fixture; project delete 204 keeps global root; global clear 200 keeps project clone + row; folder/folder leg)"
    requirement: GINT-03
    verification:
      - kind: integration
        ref: "internal/api/global_interlock_test.go#TestGlobalInterlockProjectDeleteKeepsGlobalRoot|#TestGlobalInterlockClearKeepsProjectClone|#TestGlobalInterlockFolderFolderDirections"
        status: pass
    human_judgment: false
  - id: D3
    description: "MCP bridge parity: list_tasks leak-free; list_sessions surfaces the live global row with Scratchpad/Global labels intact; get_session of the global id 200-shaped, never 404; orphaned variant filtered per D-13"
    requirement: GINT-02
    verification:
      - kind: unit
        ref: "internal/mcp/global_parity_test.go#TestGlobalParityTaskToolsLeakFree|#TestGlobalParitySessionToolsHonestLabels|#TestGlobalParityGetSessionGlobalID"
        status: pass
    human_judgment: false
  - id: D4
    description: "The D-55 audit table (surface → query/guard file:line → assertion, 10 rows) recorded in this SUMMARY for the phase verifier to carry into 17-VERIFICATION.md"
    verification:
      - kind: other
        ref: "17-02-SUMMARY.md §D-55 Audit Table (guard file:line values re-verified against the working tree 2026-08-28 — zero drift from 17-PATTERNS); mirrored in the global_noleak_test.go file header"
        status: pass
    human_judgment: false

duration: 32 min
completed: 2026-08-28T10:47:00Z
status: complete
---

# Phase 17 Plan 02: Sentinel-Leak Sweep, Interlock & MCP Parity Summary

**One-liner:** Permanent regression proofs that the global entity cannot leak onto any enumeration surface (live-agent + live-tmux harness, DB structural counts), that the managed-root ↔ project-delete interlock holds in both directions (same-ref clone into both namespaces + os.Stat), and that the MCP bridge is leak-free and honestly labeled — plus the 10-row D-55 audit table for 17-VERIFICATION.md.

## D-55 Audit Table (for 17-VERIFICATION.md)

Guard file:line values re-verified against the working tree on 2026-08-28 — **zero drift** from the 17-PATTERNS table. Mirrored in the `global_noleak_test.go` file header.

| # | Surface | Query/guard (file:line) | Assertion |
|---|---------|------------------------|-----------|
| 1 | Unscoped task list | `tasks.go:194` — `WHERE source = 'manual'` | GET /api/tasks: manual fixtures present, zero global-titled row, count == pre-global-configure count |
| 2 | Board fetch | `tasks.go:233` — `project_id = ? AND source = 'manual'` | GET /api/projects/{id}/tasks: same two manual rows, zero global rows |
| 3 | Create position | `tasks.go:288` — `MIN(position) … AND source = 'manual'` subselect | peer fixture creation lands top-of-todo (the MIN stays manual-keyed) |
| 4 | Move/position arithmetic | `tasks.go:432` (MIN on move), `tasks.go:538` (nextPosition), `tasks.go:554` (renumberColumn) | move → 200; untouched task's position unchanged by global presence; board count still == manual baseline |
| 5 | Project list | `GET /api/projects` (projects.go `p.list`) | count unchanged after configure + spawn; no sentinel project row |
| 6 | Workspace delete guard | `workspaces.go:215` — `SELECT COUNT(*) FROM projects WHERE workspace_id = ?` | empty workspace DELETE → 204 while global is live (the COUNT never counts the singleton); holding-project DELETE → 409 `move or remove its 1 project(s) first` |
| 7 | Activity stats/lists | `activity.go:135` — `t.source = 'manual'` in the base WHERE | raw response bytes contain the done-manual fixture, zero occurrences of `Scratchpad` / `Global` |
| 8 | DB structural proof | by construction — the global is a singleton row, never a task/project row | `SELECT COUNT(*) FROM tasks` / `FROM projects` unchanged by configure + spawn |
| 9 | MCP task tools (leak direction) | `mcp/tasks.go:197` → `GET /api/tasks` passthrough (= row 1's guard) | bridge `list_tasks` result carries manual identifiers, zero `Scratchpad`/`Global` |
| 10 | MCP session tools (honest direction) | `mcp/sessions.go:516` (list passthrough + D-13 orphan filter), `:579` (get passthrough) | live global row listed with `taskTitle:"Scratchpad"`/`projectName:"Global"` intact; `get_session` of the global id is 200-shaped, never 404; orphaned (id="") variant filtered per the operable-ids contract |

Adding a future surface = one row above + one subtest (T-17-06).

## Performance

- **Duration:** 32 min
- **Started:** 2026-08-28T10:15:27Z
- **Completed:** 2026-08-28T10:47:00Z
- **Tasks:** 3
- **Files created:** 3 (test-only; zero production changes, zero new dependencies)

## Accomplishments

- **`internal/api/global_noleak_test.go`** — `TestGlobalNoLeak` (D-55/D-56/GINT-03): full-surface harness (`newGlobalSessionServerWithTmux`) + fake-claude agent config + `shell=tmux`, peer fixtures created BEFORE the global is configured so baselines predate it, then configure → spawn live agent + live global tmux tab (`kamacu-global-1`, awaitHasSession) → live-state precondition via `GET /api/global` `live.agent ≥ 1` and `live.tmux ≥ 1` → seven subtests (audit-table rows 1–8). The Activity subtest keeps the Phase-11 raw-bytes posture (`assertNoGlobalLeak`); list/board/project surfaces use row-level negative checks (see Deviations #5).
- **`internal/api/global_interlock_test.go`** — the `TestGlobalInterlock*` trio (D-58/SC3): SAME canonical ref (`Octo/Widgets`) cloned as a managed project (`~/.kamacu/repos/octo/widgets`) AND the managed global root (`~/.kamacu/repos/global/octo/widgets`) through the real POST/PUT with four offline seams; Direction 1 (project delete 204 → clone gone, global root present, singleton still configured), Direction 2 (clear 200 → row cleared, project clone + project row intact, cleared root dir stays per D-04), plus the cheap folder/folder leg (folder project delete touches neither its own repo — D-09 — nor the folder global root).
- **`internal/mcp/global_parity_test.go`** — the `TestGlobalParity*` trio (D-57/GINT-02): canned backend seeded with the REAL wire shapes (`sessionDetail` labels, `Task` manual-only array, the 17-01 post-restart orphaned shape); leak direction (`list_tasks` zero `Scratchpad`/`Global`), honest direction (`list_sessions` surfaces the live global row labels-intact through the filter's decode/re-marshal; `get_session` 200-shaped with correct path), read-only throughout.

## Verification Results

| Check | Result |
|---|---|
| `go test ./internal/api -run 'TestGlobalNoLeak' -count=1 -v` | PASS (0.37s; 7/7 subtests) |
| `go test ./internal/api -run 'TestGlobalInterlock' -count=1 -v` | PASS (3/3 legs) |
| `go test ./internal/api -run 'TestGlobalNoLeak\|TestGlobalInterlock' -count=1 -v` (plan verification block) | PASS |
| `go test ./internal/mcp -run 'TestGlobalParity' -count=1 -v` | PASS (3/3) |
| Full package `go test ./internal/mcp -count=1` | PASS 0.77s |
| Full package `go test ./internal/api -count=1 -timeout 15m` | PASS ×3 (226s / 226s / 249s) after one transient host-load failure (239s run) — the pre-existing D-63 flake, see Issues |
| `go vet ./internal/api ./internal/mcp` | PASS |
| Acceptance-criteria greps (kind agent + kind bash spawns; raw-bytes Activity assertion; three Set*ForTest seams; bridge{base,token,client} ×3) | all present |
| Guard file:line drift check vs 17-PATTERNS (tasks.go:194/:233/:288/:432/:538/:554, activity.go:135, workspaces.go:215) | zero drift |
| Isolation: user's real kamacu socket after runs | `no server running on /tmp/tmux-1000/kamacu` — never touched |

## Task Commits

Each task was committed atomically:

1. **Task 1: consolidated TestGlobalNoLeak\* family** — `b08432d` (test)
2. **Task 2: managed-root ↔ project-delete interlock, both directions** — `eab0486` (test)
3. **Task 3: MCP both-direction parity through the real bridge** — `b1ee7af` (test)

## Files Created/Modified

- `internal/api/global_noleak_test.go` — the per-surface leak family + the in-code copy of the audit table
- `internal/api/global_interlock_test.go` — the both-direction interlock tests + `interlockFakeClone`/`interlockSeams`/`interlockProjectDest` helpers
- `internal/mcp/global_parity_test.go` — the bridge parity spot-checks with real-wire-shape fixtures

## Decisions Made

See frontmatter `key-decisions` — the two load-bearing ones are the 204-not-200 workspace wire alignment and the orphaned+global-row D-13 contract lock (both detailed under Deviations).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Plan-text vs wire] Empty-workspace DELETE is 204, not 200**
- **Found during:** Task 1 (workspace subtest)
- **Issue:** the plan text (must_haves + task action) says "An empty workspace DELETE returns 200"; the shipped wire is `204 No Content` (workspaces.go:232, asserted by the existing `TestWorkspaceDeleteEmpty`).
- **Fix:** asserted 204 — resolved toward the locked wire contract, same class as 17-01's stop-endpoint 202 alignment. The 409 negative control matches the plan verbatim (`move or remove its 1 project(s) first`).
- **Files modified:** internal/api/global_noleak_test.go
- **Verification:** subtest green; message string asserted exactly
- **Committed in:** b08432d

**2. [Rule 1 - Plan-text vs shipped contract] The orphaned+global row is FILTERED, not passthrough-survived**
- **Found during:** Task 3 (honest-direction fixture)
- **Issue:** the plan says the orphaned variant (orphaned:true, global:true) should "survive the passthrough" — but the shipped D-13 filter (`isOrphanedRow`, mcp/sessions.go:555-562) drops EVERY `orphaned:true` row regardless of scope, because id="" rows are SPA reattach affordances, not operable session ids (locked by `TestBridge_ListSessions_FiltersOrphanedRows` since Phase 08). The plan's own Artifacts section forbids production changes, so "make it survive" was never an available reading.
- **Fix:** the fixture includes the orphaned+global variant exactly as planned, and the test locks the shipped contract: the LIVE global row (real operable id) survives with labels intact and `get_session` never 404s — GINT-02's agent-addressable surface — while the orphaned survivor is filtered scope-agnostically like every task-scoped orphaned row. The research's audit-table row (17-RESEARCH line 326) never required orphaned survival; only the plan task text did.
- **Files modified:** internal/mcp/global_parity_test.go
- **Verification:** `TestGlobalParitySessionToolsHonestLabels` green; orphaned count asserted 0 with the rationale documented in-file
- **Committed in:** b1ee7af

**3. [Rule 3 - Blocker] interlockFakeClone needs origin/HEAD + a committed tree (plain fakeGitClone 409s the delete)**
- **Found during:** Task 2 (Direction 1 returned 409, not 204)
- **Issue:** `deleteManaged` resolves `DefaultBranch` (origin/HEAD symref) first; the plan-specified `fakeGitClone` (git init + remote add, no commits) has no origin/HEAD → conservative "unpushed" blocker → 409.
- **Fix:** `interlockFakeClone` builds a real committed clean repo plus `refs/remotes/origin/main == HEAD` and `origin/HEAD → origin/main`, so DefaultBranch/Dirty/Unpushed/Stash all clear and the delete reaches 204. Documented in-file next to the helper.
- **Files modified:** internal/api/global_interlock_test.go
- **Verification:** Direction 1 green (204 + the three os.Stat/GET assertions)
- **Committed in:** eab0486

**4. [Rule 2 - Determinism] Fourth seam: SetDescriptionRunnerForTest**
- **Found during:** Task 2 (seam audit before writing the fixture)
- **Issue:** `createByRepo` calls `github.RepoDescription` when `Available()` is forced true — with only the three plan-named seams installed, that would exec a REAL `gh repo view` (network on an authed host), violating the task's own "no network, no real gh" requirement.
- **Fix:** installed `SetDescriptionRunnerForTest` (returns "") alongside the three plan-named seams; all restores ride `t.Cleanup` (a `defer` inside the shared helper would restore at helper exit — found the hard way when the first run 400'd with "Repository not found").
- **Files modified:** internal/api/global_interlock_test.go
- **Verification:** fixture fully offline; both managed legs green
- **Committed in:** eab0486

**5. [Rule 1 - False positive] Row-level (not raw-byte) leak checks for path-carrying bodies**
- **Found during:** Task 1 (first run: all raw-byte checks "leaked" `Global`)
- **Issue:** task/board/project bodies embed `worktree_path` under `t.TempDir()`, and the temp path contains the test NAME — `TestGlobalNoLeak` contains the substring `Global`, so raw-byte negative assertions false-positived on every list surface.
- **Fix:** those surfaces assert on decoded row fields (`title`/`name`); the plan-mandated raw-bytes posture is kept where it is meaningful and safe — Activity, whose body carries no paths (and which passed unchanged). Rationale documented at both assertion helpers.
- **Files modified:** internal/api/global_noleak_test.go
- **Verification:** full family green; the Activity raw-bytes assertion still negative-asserts both strings
- **Committed in:** b08432d

---

**Total deviations:** 5 auto-fixed (2 plan-text vs wire/contract alignments, 1 blocker, 1 determinism hardening, 1 false-positive test fix). **Impact:** all fixes keep the plan's own constraints intact (zero production changes, zero new dependencies); assertions target the shipped wire contracts. No scope creep.

## Issues Encountered

- **Full-suite flake (pre-existing, NOT caused by this plan):** the first full `./internal/api` run failed (239s, host-load slowdown) — the same transient class 17-01 documented (`TestInput_Happy_*` marker timeout under load, deferred item #2 → 17-03's D-63 baseline). Three subsequent full runs passed (226s / 226s / 249s). This plan's tests are test-only additions with unique tmux socket prefixes and per-test HOME sandboxes; the failing test is not touched by this plan. Evidence row appended to deferred-items.md.

## Known Stubs

None — no stubs, placeholders, or unwired data paths. Test-only plan; no production code was changed.

## Authentication Gates

None.

## Next Phase Readiness

- Ready for 17-03 (D-63 env hygiene + red-test investigation) per the phase directory's plan order.
- The audit table above is the D-55 deliverable the phase verifier must carry into 17-VERIFICATION.md.
- Threat register: T-17-05 (HOME sandbox) mitigated — every managed-namespace write happens under the harness's `t.Setenv("HOME", t.TempDir())`; T-17-06 (missing surface) mitigated by the audit table + one-subtest-per-row pattern; T-17-07 (wire-shape drift) mitigated by fixtures seeded from the verified handler shapes with the API-level proofs living against the real handlers in Task 1.

## Self-Check: PASSED

- [x] internal/api/global_noleak_test.go exists; `TestGlobalNoLeak` green
- [x] internal/api/global_interlock_test.go exists; all three `TestGlobalInterlock*` green
- [x] internal/mcp/global_parity_test.go exists; all three `TestGlobalParity*` green
- [x] Commits b08432d / eab0486 / b1ee7af present on task/add-global-session-301
- [x] Audit table ≥8 rows (10) in surface → guard → assertion shape
