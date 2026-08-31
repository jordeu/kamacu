---
phase: 17-hardening-e2e
verified: 2026-08-28T13:55:00Z
status: passed
score: 12/14 must-haves verified
behavior_unverified: 2 # truths present + wired whose runtime behavior no automated test exercises (the SC2 same-conversation memory judgment and the SC5 walkthrough execution — both owned by 17-UAT.md, intentionally deferred to /gsd-verify-work)
overrides_applied: 0
behavior_unverified_items:

  - truth: "SC2 browser half — after a restart, a global claude/opencode session resumes the SAME conversation (the agent remembers the identifiable thing)"
    test: "Execute 17-UAT.md flows 2 and 3: ask the live agent something identifiable, SIGTERM/kill the serve process, restart on the same DB, click 'Resume session', ask the agent to recall it"
    expected: "Heading 'Agent session ended.' with the 'Reset session'/'Resume session' pair; after 'Resuming…' the same conversation streams and the agent recalls the identifiable thing (a fresh terminal that forgot it is a fail)"
    why_human: "The E2E legs prove the resume plumbing carries the persisted id end-to-end (claude: fake-claude argv '--resume <same csid>'; opencode: REAL binary, captured 'ses_fb760fae…' resumed with '-s <same id>') — but 'it remembers' is a real-binary conversation-memory judgment no fake can assert"

  - truth: "SC5 — a UAT script exercises a folder-mode root against a real repo checkout end-to-end (configure → agent → bash → reattach → stop), documenting the no-worktree safety posture"
    test: "Execute 17-UAT.md flows 1-6 in the browser (including the D-12 bar-row settlement decision in flow 6) and record every result:/decision: line + Summary counters"
    expected: "All six flows pass as written; flow 1 covers the full folder-mode lifecycle incl. invisible bash reattach and the 'No root configured yet.' post-clear state; flow 6 records KEEP or FLIP — <reason>"
    why_human: "Auth, quota, visual-reattach invisibility, bar-row distinguishability and the keep-or-flip settlement are inherently human judgments; the doc is authored-pending by design (17-04 output spec) and the orchestrator confirmed the walkthrough runs via /gsd-verify-work after this verification"
human_verification:

  - test: "Run /gsd-verify-work 17 and execute 17-UAT.md flow 1 (SC5 folder-mode lifecycle against a real repo)"
    expected: "Configure root → 'Start agent' CTA → live terminal → 'Bash <n>' tab → navigate away/back reattaches both with scrollback (bash reattach invisible, label byte-for-byte) → ⋯ menu 'Stop' works → concurrent 'Start agent' surfaces 'global agent already running' → Clear root returns 'No root configured yet.'"
    why_human: "Browser walkthrough with a real repo, real auth and visual reattach behavior; automation proved the API lifecycle gates but not the browser surfacing"

  - test: "Execute 17-UAT.md flows 2-3 (D-53 same-conversation resume, both engines, across a real serve restart)"
    expected: "'Agent session ended.' + 'Resume session' → 'Resuming…' → same conversation; the agent remembers the identifiable thing for BOTH claude and opencode"
    why_human: "Real-binary conversation memory is the point of the flow; the E2E argv proofs cannot see past the id plumbing"

  - test: "Execute 17-UAT.md flow 4 (SC3 interlock browser sanity)"
    expected: "Deleting the project leaves the Settings Scratchpad managed root row unchanged and /global functional; the reverse clear leaves the project clone alone"
    why_human: "Visual confirmation of the machine-proven interlock through the shipped UI surfaces"

  - test: "Execute 17-UAT.md flow 5 (SC4 sentinel-leak visual sanity)"
    expected: "With a live global agent + bash tab: no foreign card on kanban columns, no new project in the sidebar, empty workspace still deletes, Activity shows no Scratchpad/Global rows; the global appears only on the bar row, /global, Settings Scratchpad and MCP session-tool labels"
    why_human: "Visual sweep across surfaces in the shipped UI; the per-surface API/DB/MCP exclusions are machine-proven (TestGlobalNoLeak/TestGlobalParity)"

  - test: "Execute 17-UAT.md flow 6 (D-12 bar-row settlement) and record the decision"
    expected: "With a task row and the global row live simultaneously, confirm 'Global · Scratchpad' is distinguishable from a task's project · task pair; record KEEP (default) or FLIP — <what confused you>; a FLIP becomes a logged follow-up, not a Phase-17 change"
    why_human: "The settlement is an explicit human decision made with the live bar in front of the user (prohibition P3 forbids implementing any flip this phase)"
---

# Phase 17: Hardening & E2E Verification Report

**Phase Goal:** The lifecycle gates proven end-to-end — reconfigure-while-live, restart-resume for both engines, the managed-root ↔ project-delete interlock, and the sentinel-leak guarantee across every enumeration surface.
**Verified:** 2026-08-28T13:55:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | SC1 reconfigure gate E2E: root change/clear while live → 409 + non-empty `reasons[{kind,target}]`; after stops → clear 200; persisted resume ids observably cleared (GCONF-04) | ✓ VERIFIED | `TestE2EGlobalReconfigureGate` PASS (this verification, 2.06s); reasons grammar asserted at `e2e_global_restart_test.go:514-526`; id-clearing proven via transcript-seeded resume-refusal 409 `'no global claude session to resume'` (`:641-642`) |
| 2 | Concurrent global-agent spawn while live → 409 `'global agent already running'` (D-34) | ✓ VERIFIED | Asserted `e2e_global_restart_test.go:590-591`; PASS |
| 3 | SC2 restart-resume, claude engine: real SIGTERM → second process on same `--db` → exactly one global entry (source global, exited, resumable, engine claude) → resume 201 with `--resume <SAME csid>` (GSESS-02 claude half) | ✓ VERIFIED | `TestE2EGlobalRestartResume` PASS (2.27s); same-csid never-forked assertion at `:780-782` |
| 4 | SC2 restart-resume, opencode engine: wrapper-agent argv proof through a REAL restart — capture persisted (resumable flip over HTTP), resume 201 with consecutive pair `-s <SAME ses_ id>` (GSESS-02 opencode half) | ✓ VERIFIED | `TestE2EGlobalOpencodeRestartResume` PASS (12.50s); live run: captured `ses_fb760fae2ffe0n9HATAaSt7yga`, resume argv `[-s ses_fb760fae2ffe0n9HATAaSt7yga]` (`e2e_opencode_resume_test.go:306`); fresh-spawn no-`-s` guard at `:222`; engine-branched append confirmed at `sessions.go:573` |
| 5 | SC2 browser half: the resumed session is the SAME conversation (agent remembers) — both engines | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Plumbing fully proven (truths 3-4); the memory judgment is 17-UAT.md flows 2-3, authored-pending — see Human Verification |
| 6 | SC2/GSESS-03 tmux survival: post-restart row orphaned+global, label byte-identical, tab alive on the socket, `reattach_tmux_name` → 201 label-preserved | ✓ VERIFIED | `TestE2EGlobalRestartResume` PASS; survivor assertions at `e2e_global_restart_test.go:744-747`+; has-session probe + reattach in the same leg |
| 7 | SC3 interlock direction 1: project delete (gated clone removal) removes ONLY the project clone; global root on disk and still configured | ✓ VERIFIED | `TestGlobalInterlockProjectDeleteKeepsGlobalRoot` PASS — same-ref `Octo/Widgets` in both namespaces, os.Stat both dirs, 204 delete |
| 8 | SC3 interlock direction 2: global clear leaves project clone + project row intact; folder/folder direction likewise | ✓ VERIFIED | `TestGlobalInterlockClearKeepsProjectClone` + `TestGlobalInterlockFolderFolderDirections` PASS |
| 9 | SC4 sentinel-leak sweep: no global entity on any enumeration surface (boards, listings, workspace guard, Activity, MCP task tools) — each surface enumerated and asserted; DB structural zero-rows proof (GINT-03, Phase-13 invariants) | ✓ VERIFIED | `TestGlobalNoLeak` 7/7 subtests PASS (unscoped list, board fetch, move/positions, project list, workspace guard 204+409, Activity raw bytes, DB COUNT); `TestGlobalParityTaskToolsLeakFree` PASS; D-55 audit table (10 rows) in 17-02-SUMMARY — guard citations spot-verified zero drift (tasks.go:194, activity.go:135, workspaces.go:215, mcp/sessions.go:516) |
| 10 | MCP honest direction (GINT-02): list_sessions surfaces the global row with Scratchpad/Global labels intact; get_session of the global id 200-shaped, never 404 | ✓ VERIFIED | `TestGlobalParitySessionToolsHonestLabels` + `TestGlobalParityGetSessionGlobalID` PASS through the real bridge struct |
| 11 | SC5 artifact: the UAT script (folder-mode root, real repo, six locked flows) exists in the 16-UAT shape with the D-60 no-worktree safety posture | ✓ VERIFIED | `17-UAT.md` read in full: frontmatter keys, 6 numbered flows, every locked string verbatim on one line, safety paragraph naming `--dangerously-skip-permissions` + no worktree net/diff tab/gated cleanup + disposable-clone suggestion, D-12 `decision:` slot, Summary counters + Gaps |
| 12 | SC5 execution: the walkthrough exercises the folder-mode root end-to-end | ⚠️ PRESENT_BEHAVIOR_UNVERIFIED | Intentionally authored-pending (17-04 output spec; orchestrator confirms it runs via /gsd-verify-work after this verification) — see Human Verification |
| 13 | D-63: custom/opencode spawn arm strips inherited KAMACU_* (incl. the HOOK_TOKEN secret) before the opencode re-injection; suite green with AND without the trio | ✓ VERIFIED | `stripKamacuEnv`/`kamacuEnvNames` at `manager.go:37/:53`, filtered `cmd.Env` at `:250`; `TestCustomEngineDoesNotGetHookEnv` + `TestOpencodeSpawnInjectsHookEnv` PASS in BOTH postures (this verification: trio-exported and `env -u` scrubbed); full suite `go test ./... -count=1 -timeout 20m` exit 0 WITH trio (all 15 packages ok, api 272s — includes post-merge commit 28a1a68); scrubbed full-suite evidence in 17-03-SUMMARY |
| 14 | D-62: `Agent.engine` union `'claude' \| 'custom' \| 'opencode'` + `isClaudeAgent = projectEngine === 'claude'`; prohibitions P1/P2/P3 honored | ✓ VERIFIED | `types.ts:66`, `TaskPage.tsx:69/:540`; `npm run build` (tsc -b && vite build) PASS; `git diff 9819b82..HEAD -- web/` touches exactly types.ts + TaskPage.tsx (BoardPage/QuotaIndicator/GlobalTaskPage untouched); `git diff 9451d51..HEAD -- web/` EMPTY (no bar-row visual change) |

**Score:** 12/14 truths verified (2 present, behavior-unverified — both owned by the intentionally pending 17-UAT.md walkthrough)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/api/e2e_global_restart_test.go` | Real-binary restart harness + SC1/SC2-claude/tmux legs | ✓ VERIFIED | 806 lines; `TestE2EGlobalReconfigureGate` + `TestE2EGlobalRestartResume` PASS; LookPath gates, KAMACU_* scrub (`:278`), `realHomeEnv` knob |
| `internal/api/e2e_opencode_resume_test.go` | Real-opencode restart-resume leg | ✓ VERIFIED | 339 lines; `TestE2EGlobalOpencodeRestartResume` PASS; reuses 17-01 harness, real-HOME posture, restore PATCH in t.Cleanup |
| `internal/api/global_noleak_test.go` | Consolidated per-surface leak family | ✓ VERIFIED | 311 lines; 7/7 subtests PASS; audit table mirrored in header |
| `internal/api/global_interlock_test.go` | Both-direction interlock + folder leg | ✓ VERIFIED | 253 lines; 3/3 PASS; four offline seams incl. SetDescriptionRunnerForTest |
| `internal/mcp/global_parity_test.go` | MCP bridge both-direction parity | ✓ VERIFIED | 197 lines; 3/3 PASS via real bridge struct |
| `internal/session/manager.go` | stripKamacuEnv production fix | ✓ VERIFIED | `:37-60`, `:250`; claude arm untouched |
| `web/src/api/types.ts` | engine union widened | ✓ VERIFIED | `:66` with provenance comment |
| `web/src/pages/TaskPage.tsx` | strict claude-only predicate | ✓ VERIFIED | `:69`, render site `:540` |
| `.planning/phases/17-hardening-e2e/17-UAT.md` | SC5 walkthrough doc | ✓ VERIFIED | 96 lines; all structural gates pass (artifact half of SC5) |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| e2e harness | real binary | `go build ./cmd/kamacu` → spawn → healthz poll → SIGTERM+Wait → re-spawn same `--db` | ✓ WIRED | Proven by 3 passing E2E legs (restarts actually happen — 16.8s family runtime) |
| serve `--claude-bin` | fake-claude recorder | FAKE_CLAUDE_ARGS_FILE env in child | ✓ WIRED | csid captured from `--session-id` pair; `--resume <csid>` re-read post-restart |
| wrapper PATCH | system OpenCode agent | PATCH `/api/agents/{id}` command → record-then-exec wrapper → restore in t.Cleanup | ✓ WIRED | Live run shows real opencode exec + argv recorded (`-s ses_fb76…`) |
| tasks/activity/workspaces guards | leak-free surfaces | `source='manual'` WHERE clauses / projects COUNT | ✓ WIRED | Guard lines spot-verified; 7/7 subtests green |
| MCP bridge | API passthrough | list_tasks→/api/tasks, list_sessions/get_session→/api/sessions | ✓ WIRED | 3/3 parity tests green through bridge struct |
| types.ts union | Go wire + 00015 seed | engine field serialization parity | ✓ WIRED | tsc build green; 00015_opencode_agent.sql present in migration log |

### Data-Flow Trace (Level 4)

Not applicable — the phase's artifacts are test harnesses and a two-line type/predicate change; no new runtime data-rendering surfaces were added. The E2E harnesses consume real process/HTTP/DB data by construction (proven by the passing legs).

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| SC1 + SC2 both engines + tmux survival | `go test ./internal/api -run 'TestE2EGlobal' -count=1 -v -timeout 20m` | 3/3 PASS (16.8s); opencode leg captured + resumed the SAME `ses_` id | ✓ PASS |
| Sentinel-leak family | `go test ./internal/api -run 'TestGlobalNoLeak' -count=1 -v` | 7/7 subtests PASS | ✓ PASS |
| Interlock both directions + folder leg | `go test ./internal/api -run 'TestGlobalInterlock' -count=1 -v` | 3/3 PASS | ✓ PASS |
| MCP bridge parity | `go test ./internal/mcp -run 'TestGlobalParity' -count=1 -v` | 3/3 PASS | ✓ PASS |
| D-63 WITH trio exported | `KAMACU_SESSION_ID=t KAMACU_HOOK_TOKEN=t KAMACU_HOOK_BASE=t go test ./internal/session -run 'TestCustomEngineDoesNotGetHookEnv\|TestOpencodeSpawnInjectsHookEnv' -count=1 -v` | 2/2 PASS | ✓ PASS |
| D-63 WITHOUT trio (scrubbed) | `env -u KAMACU_SESSION_ID -u KAMACU_HOOK_TOKEN -u KAMACU_HOOK_BASE go test ./internal/session -run '…' -count=1` | ok | ✓ PASS |
| D-62 frontend build | `cd web && npm run build` | built in 2.43s (tsc -b && vite build) | ✓ PASS |
| Full suite, under-Kamacu posture | `go test ./... -count=1 -timeout 20m` | exit 0 — all 15 packages ok (api 272s incl. the 28a1a68 marker-wait fixes; session 23s; ws 66s) | ✓ PASS |

### Probe Execution

Not applicable — no `scripts/*/tests/probe-*.sh` declared; the phase's proofs are go test families (executed above).

### Requirements Coverage

Phase 17 owns no REQ-IDs by design (REQUIREMENTS.md:105). It closes the E2E verification gates for earlier-phase requirements:

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| GCONF-04 | 17-01 | Reconfigure/clear refused 409+reasons while live; ids cleared on success | ✓ SATISFIED (E2E half) | `TestE2EGlobalReconfigureGate` PASS |
| GSESS-02 | 17-01 (claude), 17-04 (opencode) | Post-restart resume, engine-branched, keyed on persisted ids | ✓ SATISFIED (both engines, API level; memory judgment → UAT flows 2-3) | `TestE2EGlobalRestartResume` + `TestE2EGlobalOpencodeRestartResume` PASS |
| GSESS-03 | 17-01 | Global tmux tabs survive restart, reattach invisibly | ✓ SATISFIED (API level; invisible-reattach visual → UAT flow 1) | tmux-survival leg PASS |
| GINT-02 | 17-02 | MCP session tools honest labels, never 404 | ✓ SATISFIED | `TestGlobalParity*` 3/3 PASS |
| GINT-03 | 17-02 | Global sessions excluded from Activity stats/lists (and every enumeration surface) | ✓ SATISFIED | `TestGlobalNoLeak` 7/7 PASS |

Orphaned requirements: none — no plan claims REQ-IDs the roadmap doesn't assign to this phase's closure scope.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `.planning/phases/17-hardening-e2e/17-UAT.md` | result:/decision: lines | Empty result lines | ℹ️ Info | The authored-pending walkthrough contract — the deliverable IS the doc; execution is the human half (not a stub) |

Zero TBD/FIXME/XXX markers across all 13 phase-modified files. No stubs: every E2E leg drives real processes; the interlock tests assert real filesystem state; MCP tests drive the real bridge struct.

### Human Verification Required

Five items, all instrumented by `17-UAT.md` (execute via `/gsd-verify-work 17`):

### 1. SC5 folder-mode lifecycle walkthrough (flow 1)

**Test:** Configure a folder root against a real repo, `Start agent`, spawn a bash tab, navigate away/back, stop everything, clear root.
**Expected:** Live terminal, `Bash <n>` tab, invisible reattach with prior scrollback, ⋯ menu `Stop`, inline `global agent already running` on concurrent start, `No root configured yet.` after clear.
**Why human:** Real repo, auth, and visual reattach behavior in the browser.

### 2. Same-conversation resume, both engines (flows 2-3)

**Test:** Identifiable-thing → kill serve → restart on same DB → `Resume session` → ask again.
**Expected:** `Agent session ended.` + `Reset session`/`Resume session` pair → `Resuming…` → SAME conversation, agent remembers.
**Why human:** Real-binary conversation memory — the fake/wrapper argv proofs cannot see past the persisted id.

### 3. Interlock browser sanity (flow 4)

**Test:** Managed project clone + managed global root of the SAME repo; delete the project.
**Expected:** Scratchpad root row (mono owner/name + managed badge) unchanged; /global functional; reverse clear leaves the project clone alone.
**Why human:** Visual confirmation of the machine-proven interlock through the shipped UI.

### 4. Sentinel-leak visual sweep (flow 5)

**Test:** With a live global agent + bash tab, walk every enumeration surface.
**Expected:** No foreign card/sidebar project; empty workspace still deletes; Activity clean; global appears only on bar row, /global, Settings Scratchpad, MCP labels.
**Why human:** Visual sweep of the shipped UI; per-surface API/DB/MCP exclusions are machine-proven.

### 5. D-12 bar-row settlement (flow 6)

**Test:** Task row + global row live simultaneously; confirm distinguishability; record the decision.
**Expected:** `Global · Scratchpad` distinguishable from a task's project · task pair; record KEEP (default) or FLIP — reason.
**Why human:** Explicit human decision with the live bar (prohibition P3: no visual change this phase).

### Gaps Summary

**No gaps.** Every machine-checkable must-have is green: all 9 phase artifacts exist, are substantive, and are wired; all 13 automated behavioral checks pass (including a fresh full-suite run under the Kamacu env posture, which also validates post-merge commit `28a1a68` — deferred item #4's closure); all three prohibitions are honored by git-diff evidence; the D-55 audit table (10 rows) is recorded with zero guard-drift; commits `00d4258`/`5c7e3b7`/`b08432d`/`eab0486`/`b1ee7af`/`affedcf`/`de3e906`/`8ed08ad`/`1d584a8`/`4adaa24`/`9ff6570`/`28a1a68` are all present on the branch.

The two behavior-unverified truths are the intentional human halves of SC2 (conversation memory) and SC5 (walkthrough execution) — instrumented by 17-UAT.md, which the orchestrator confirmed runs via `/gsd-verify-work` after this verification. Status is therefore `human_needed`, not `gaps_found`.

Informational (not a gap, no later milestone phase): deferred-items.md item 1 — the production TMUX_TMPDIR inconsistency in `manager.go`'s tmux spawn allow-list — remains open by documented decision (17-03's contract scoped D-63 to the KAMACU_* trio), re-homed to a future quick task with a suggested one-line fix. Deferred items 2, 3 and 4 are closed (`8ed08ad`, `8ed08ad`, `28a1a68` respectively — verified present, and the full-suite run here exercises their fixes).

---

_Verified: 2026-08-28T13:55:00Z_
_Verifier: the agent (gsd-verifier)_
