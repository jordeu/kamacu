# Phase 17: Hardening & E2E - Context

**Gathered:** 2026-08-27
**Status:** Ready for planning

<domain>
## Phase Boundary

The lifecycle gates proven end-to-end — the flows invisible until a restart or a delete happens: the reconfigure-while-live gate (GCONF-04) and restart-resume for both engines + tmux (GSESS-02/03) through a REAL binary restart, the managed-root ↔ project-delete interlock (SC3), the sentinel-leak sweep across every enumeration surface (SC4 + GINT-02/GINT-03 closure), and the folder-mode UAT against a real repo checkout (SC5). Phase 17 owns no requirements — it closes verification gates. Plus the carried-forward debt explicitly parked for this phase: the stale `types.ts` `Agent.engine` union + TaskPage QuotaIndicator predicate (16-01) and the pre-existing red tests (`TestInput_*` family, `TestCustomEngineDoesNotGetHookEnv` — 13/14 deferred-items). D-12 (bar-row visual treatment) settles at this phase's UAT.

</domain>

<decisions>
## Implementation Decisions

### Carry-Forward Locked (Phases 13–16 + research — do not reopen)

- **Gate semantics shipped:** the 409 family (D-28..D-34), `{error, reasons[...]}` grammar (D-15), resume-id clearing on ANY successful root change (D-16), gate order root-gates-first (D-33).
- **Namespaces separated by construction:** global managed root `~/.kamacu/repos/global/<owner>/<name>` vs project clones `~/.kamacu/repos/<owner>/<name>` (13-CTX D-01/D-02); same repo allowed on both sides, no cross-entity guards (D-03) — the interlock PROVES this, it doesn't add new guards.
- **Phase 15 shipped restart-*sims* + host-gated opencode capture** (`TestGlobalStatusPostRestart`, `TestGlobalSessionRestartOrphan`, `TestGlobalOpencodeCaptureHost`); the live-server curl story (real restart, resume same conversation) is THIS phase's deferred scope (15-VERIFICATION deferred table).
- **16-UAT already verified live:** the 409 dialog flow through Settings (test 4), /global interactive parity (test 2), the 7-state matrix (test 3). Phase 17 does not re-verify Phase-16 UX; it closes lifecycle gates.
- **Sentinel-leak posture:** the global entity is a singleton row, never a task/project row (research P2) — surfaces exclude by construction; the sweep proves each one stays that way.

### Restart E2E mechanics (new — D-47..D-50)

- **D-47: SC2 restart E2E = a real-binary host-gated Go test.** Spawn the actual `bin/kamacu serve` with the Phase-14 curl-smoke env isolation recipe (HOME=sandbox + temp `--db` + free 127.0.0.1 port), drive spawn via real HTTP, SIGTERM, restart a second process against the same DB, assert resume offers + tmux rows via HTTP. NOT a shell script, NOT more in-process sims.
- **D-48: SC1 folds into the same harness — one curl story.** The harness covers the full reconfigure cycle: configure → spawn agent + bash/tmux → root change/clear 409s with reasons → stop them → change succeeds → persisted resume ids cleared (observable via the resume-refusal 409s). One test narrative proves both SC1 and SC2.
- **D-49: tmux reattach asserted at API level.** After restart: the global tmux row survives (`GET /api/sessions?scope=global`) with its label preserved (the v1.8 GAP-01 regression replayed for the global scope) and `tmux has-session` is alive on the real socket. The invisible browser-reattach visual stays in the UAT.
- **D-50: The E2E lives inline in `go test ./...`**, host-gated with the house `exec.LookPath` skip convention (skip when tmux/binary prerequisites are absent) — NOT a separate make target.

### Resume-proof realism (new — D-51..D-54)

- **D-51: claude resume = fake-claude argv proof, automated.** Inside the real-binary harness, fake-claude proves the spawn→restart→resume cycle carries the persisted csid on the argv AND the transcript gate 409s when the transcript is gone (`transcriptExists` is cwd-agnostic — works verbatim). NO real-claude automated test (auth/quota/judgment don't belong in a test gate).
- **D-52: opencode resume = real host-gated restart cycle.** Extend the existing harness (`TestGlobalOpencodeCaptureHost` prior art; opencode provisioned on this host) through capture `ses_…` id → real restart → resume argv carries it → process returns.
- **D-53: The human UAT verifies BOTH engines' real "same conversation"** — ask the agent something identifiable, stop/restart, Resume, confirm it remembers. Rides the SC5 UAT.
- **D-54: The D-31 resume-refusal edges replay in the E2E gates** — resume with no persisted id, claude transcript vanished, opencode stale ocsid — at least at API level where not already covered by Phase-15 tests.

### Leak/interlock proof form (new — D-55..D-58)

- **D-55: The sentinel-leak sweep = permanent regression tests + a documented audit table.** Tests for every surface AND an enumeration table in the phase VERIFICATION (surface → query/guard → assertion) so future surfaces have a pattern to copy.
- **D-56: One consolidated leak test file** (e.g. a `TestGlobalNoLeak*` family) walking every enumerated surface with a configured + live global session — SC4's enumeration legible in one place, trivially extensible. Named surfaces: kanban boards (all 5 board/position queries), project/task listings (`GET /api/tasks`, `GET /api/projects`), workspace non-empty delete guard (COUNT), Activity stats/lists (`GET /api/activity`), MCP task tools.
- **D-57: The MCP spot-check goes both directions through the real bridge** (in-memory transport, house pattern): task tools NEVER surface anything global (leak direction) AND session tools list/describe the live global session with honest labels, never 404 (GINT-02 — the research's MCP parity spot-check; read-only contract untouched).
- **D-58: The interlock is automated in both directions.** Create a managed project clone AND a managed global root (same repo allowed — D-03): the gated project delete removes only the project clone; clearing/reconfiguring the global root touches nothing project-side. The folder-root/folder-project trivial direction is asserted cheaply. Deterministic filesystem assertions — no UAT dependency.

### UAT shape + leftovers (new — D-59..D-63)

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

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap
- `.planning/ROADMAP.md` §Phase 17 — goal, success criteria 1–5, "Plans: TBD"
- `.planning/REQUIREMENTS.md` — Phase 17 owns no requirements; line 105 assigns the verification gates (GCONF-04, GSESS-02, GSESS-03, GINT-02, GINT-03, sentinel-leak invariant); GT-FUT-01..07 stay parked

### Milestone research (architecture-locked)
- `.planning/research/ARCHITECTURE.md` §5 "Phase 5 — Hardening & E2E" — interlock, restart-resume E2E (claude + opencode + tmux survivor), MCP parity spot-check, UAT
- `.planning/research/SUMMARY.md` §"Phase 5: Hardening & E2E" — the phase brief
- `.planning/research/PITFALLS.md` — P1 (invisibility filters), P4 (TaskID=0 overload), P5 (restart amnesia) — the invariants the E2E proves

### Prior-phase decisions this phase exercises
- `.planning/phases/13-global-data-foundation-safety-net/13-CONTEXT.md` — D-01..D-03 (namespace, no-cross-guards — the interlock's substrate), D-12 (bar visuals → Phase 17 UAT)
- `.planning/phases/14-global-config-api/14-CONTEXT.md` — D-15/D-16 (409 grammar, resume-id clearing — what SC1 observes), D-20..D-23 (PUT grammar the curl story drives)
- `.planning/phases/15-global-sessions-backend/15-CONTEXT.md` — D-28..D-34 (the 409 family), D-31 (resume-refusal edges D-54 replays)
- `.planning/phases/15-global-sessions-backend/15-VERIFICATION.md` — the deferred table naming THIS phase's live-server curl story
- `.planning/phases/16-global-view-settings-bar-integration/16-CONTEXT.md` — D-45 (409 inline surfacing, already UAT-verified), the 16-01 deferred union/predicate flags
- `.planning/phases/16-global-view-settings-bar-integration/16-UAT.md` — the walkthrough-doc pattern D-59 mirrors; what's already verified live
- `.planning/phases/13-global-data-foundation-safety-net/deferred-items.md` + `.planning/phases/14-global-config-api/deferred-items.md` — the pre-existing red tests D-63 absorbs (with their baseline-verified reproduction notes)

### Code-level precedents (read in-repo)
- `internal/api/sessions_global_test.go` — the Phase-15 suite this phase builds on: restart-sims (:609, :1330), host-gated real-opencode capture (:1134), fake-claude global harness conventions
- `internal/opencode/e2e_test.go` — the v1.10 host-gated real-opencode harness pattern (LookPath skips, receiver assertions)
- `internal/api/global_test.go` — the Phase-14 gate tests (409 family) + the curl-smoke env-isolation recipe's in-package sibling
- `cmd/kamacu/sweep_test.go` + `cmd/kamacu/main_test.go` — real-binary test prior art in the cmd package
- `internal/api/agent_integration_test.go` — the fake-claude integration harness (argv capture conventions)
- `internal/mcp/` test files — the in-memory-transport bridge test pattern D-57 reuses
- `smoke.sh` — the build+serve smoke baseline
- `web/src/api/types.ts` + `web/src/pages/TaskPage.tsx` — D-62's stale union + predicate sites (16-01/16-RESEARCH Pitfall 2 notes)
- `internal/session/tmux_lifecycle_test.go` — tmux host-gated test conventions for D-49

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Phase-14 real-binary curl-smoke recipe (STATE.md decision: `HOME=sandbox + temp --db + free port` env isolation — every ~-relative resolution follows HOME, the exact fresh-install contract) — D-47's foundation
- `sessions_global_test.go` global harness (seedGlobalRoot, fake-claude argv capture, restart-sim seeding) — extend, don't rebuild
- `internal/opencode/e2e_test.go` host-gating + receiver patterns — D-52's template
- `internal/mcp` in-memory transport tests — D-57's both-direction bridge assertions
- v1.8 GAP-01 regression test (`TestSessionTmuxReattachPreservesCustomLabel`) — the label-preservation pattern D-49 replays for global scope
- `16-UAT.md` — the walkthrough doc shape D-59 copies

### Established Patterns
- Host-gated tests: `exec.LookPath` + `t.Skip` (house convention, ~30 sites) — D-50
- Restart-sims vs real restart: sims recreate the API server in-process; D-47's harness goes further (real process, real SIGTERM, same DB file)
- Degrade-don't-break + honest 409s — every gate the curl story hits already speaks this grammar
- Gated-delete 409 `{error, reasons[...]}` — the interlock test asserts project delete still emits it while the global root stays untouched

### Integration Points
- Enumeration surfaces (D-55/D-56): the 5 board/position queries (`source='manual'` guards), `GET /api/tasks` (manual-only), `GET /api/projects`, workspace delete COUNT guard, `GET /api/activity` (task-keyed), `internal/mcp` task tools (bridge → GET /api/tasks)
- `global_task` singleton + `tmux_sessions` scope rows (00017/00018) — what the restart E2E's DB carries across the SIGTERM
- `~/.kamacu/repos/global/<owner>/<name>` vs `~/.kamacu/repos/<owner>/<name>` — D-58's filesystem assertions
- `web/src/api/types.ts` `Agent.engine` + TaskPage QuotaIndicator predicate — D-62's two edit sites

</code_context>

<specifics>
## Specific Ideas

- The one curl story tells the whole phase: configure folder root → spawn agent + tmux bash → root change 409s with reasons → stop → change succeeds → resume 409s (ids cleared) → spawn again → SIGTERM → restart → resume offer carries the id → tmux row back with label.
- The interlock test deliberately configures the SAME repo as project and global root (D-03 allows it) — proving namespace separation, not repo identity, is what protects the clones.
- The audit table format: one row per surface — `surface → query/guard → assertion` — so adding a surface later is adding a row + a test case.
- The UAT's safety-posture documentation should name the real risk concretely: a skip-permissions agent with no worktree net, writing directly into the checkout the user picked.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope. GT-FUT-01..07 remain parked in REQUIREMENTS.md. D-12 settles at this phase's UAT (D-61); the red tests and stale-union debt are folded IN (D-62/D-63), not deferred.

</deferred>

---

*Phase: 17-Hardening & E2E*
*Context gathered: 2026-08-27*
