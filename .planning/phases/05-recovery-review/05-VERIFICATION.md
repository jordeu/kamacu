---
phase: 05-recovery-review
verified: 2026-06-11T13:00:00Z
status: passed
score: 3/3 success criteria verified (12/12 supporting truths)
---

# Phase 5: Recovery & Review Verification Report

**Phase Goal:** Work survives server restarts and finished agent work can be reviewed without leaving the app
**Verified:** 2026-06-11T13:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

The phase goal decomposes into the three ROADMAP Success Criteria (the contract). All three are
satisfied in the codebase, locked by a deterministic integration test (`TestRecoveryLifecycle`,
passes `-count=2`), the full backend suite (6 Go packages green), a clean frontend build, an
assembled single binary, and a human-approved live walkthrough against real claude v2.1.173.

### Observable Truths

| #   | Truth (Success Criterion → supporting truth)                                                                                                | Status     | Evidence |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------- |
| 1   | **SC1** Post-restart, a fresh manager over the same DB yields zero ghost sessions / bash stubs (reconciliation by architecture)              | ✓ VERIFIED | `agents.go` DB-derived pass (L123-152); `TestRecoveryLifecycle` restart stage with 4 `NewManager` refs; no migration 00004 (grep count 0) |
| 2   | **SC1** No session is ever auto-resumed into two PTYs — resume only on explicit request, behind the one-per-task 409 gate                    | ✓ VERIFIED | `sessions.go` gate L116-123 BEFORE resume validation L127-133; `grep ResumeSessionID cmd/ internal/ws/` → none; integration "one-per-task on resume" 409 stage |
| 3   | **SC1** Post-restart resumable dot renders muted gray w/ tooltip "Exited" (never red, never "code null")                                     | ✓ VERIFIED | `StatusDot.tsx` L26-32 null-exitCode branch FIRST → `bg-zinc-600` + tooltip `Exited`; 2× `bg-zinc-600` present |
| 4   | **SC1** No reconciliation UI / banners / toasts on restart (D-65 silence)                                                                    | ✓ VERIFIED | `agents.go` emits only data; no new user-facing copy in recovery files; human checklist item 1 approved (no notices) |
| 5   | **SC2** Resume spawns `claude --resume <stored uuid>` (never `--session-id`, never `--fork-session`) in the worktree                          | ✓ VERIFIED | `manager.go` L117-138 flag switch; `fork-session` only in comments; integration argv assert (`--resume` line 1, uuidA line 2) |
| 6   | **SC2** Each task's claude_session_id persists; resume reuses the same id (no mutation)                                                       | ✓ VERIFIED | `sessions.go` L143 `UPDATE tasks SET claude_session_id`; integration asserts id unchanged after resume |
| 7   | **SC2** Resume offered exactly where server says it can succeed; honest 409 "no session to resume" for NULL id or missing transcript          | ✓ VERIFIED | `sessions.go` L127-132; `resume.go` `transcriptExists` glob w/ uuid.Parse guard; integration no-session + RESUME_FAIL stages |
| 8   | **SC2** Resume/Reset pair renders in both placements (pre-start after restart, exited banner within run), server `resumable` is sole driver   | ✓ VERIFIED | `AgentTab.tsx` resumable from shared poll L43-45, pre-start variant L57+, banner pair → `exitedActions` L194; `useResumeAgent` posts `resume:true` |
| 9   | **SC3** GET /api/tasks/{id}/diff returns merge-base three-dot diff (committed+staged+unstaged) + untracked as additions                       | ✓ VERIFIED | `diff.go` `Compute` pipeline (merge-base, numstat -z, patch, ls-files + no-index); `internal/diff` tests green; integration totals.files==2 |
| 10  | **SC3** no-index exit-1 = success-with-diff; binary → binary:true/no hunks; rename -z records parse without desync                            | ✓ VERIFIED | `diff.go` `runNoIndex` L45-67 exit-1 branch; `parse.go` parseNumstatZ rename/binary handling; `TestParse` fixtures green |
| 11  | **SC3** Read-only Diff tab at index 2 (Agent, Description, Diff, Bash 1..N); disabled + tooltip "The diff needs a worktree" when no worktree   | ✓ VERIFIED | `TaskPage.tsx` diff TabDef L297-307 after description L284 before sessions L308; `disabled: !task.worktree_path`; tabIds conditional L136 |
| 12  | **SC3** DiffTab fetches on activation only (no polling); collapse >400 lines; binary header-only; empty/loading/error states verbatim         | ✓ VERIFIED | `diffs.ts` no `refetchInterval`; `DiffFileSection` `defaultOpen={changedLines<=400}`, "Binary file changed"; `DiffTab` all 4 states present |

**Score:** 3/3 ROADMAP success criteria verified (12/12 supporting truths)

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/api/resume.go` | transcriptExists glob helper | ✓ VERIFIED | uuid.Parse guard + glob; used by agents.go + sessions.go |
| `internal/session/manager.go` | SpawnOpts.ResumeSessionID + flag switch | ✓ VERIFIED | L48 field, L88 agent-only guard, L117-138 --resume/--session-id switch |
| `internal/api/agents.go` | resumable field + DB-derived post-restart entries | ✓ VERIFIED | Resumable JSON key L33; DB-derived pass L123-152 emits exitCode:null/resumable:true |
| `internal/api/sessions.go` | resume REST variant riding the gate | ✓ VERIFIED | Resume body flag, gate-before-validate ordering, fresh in-handler read |
| `internal/api/testdata/fake-claude` | argv recording + resume-fail mode | ✓ VERIFIED | executable; FAKE_CLAUDE_ARGS_FILE + FAKE_CLAUDE_RESUME_FAIL; 04-05 contract preserved |
| `internal/diff/diff.go` | Compute pipeline + no-index exit-1 runner | ✓ VERIFIED | merge-base/numstat/patch/ls-files; runNoIndex exit-1 special case |
| `internal/diff/parse.go` | numstat-z + unified-patch parsers | ✓ VERIFIED | parseNumstatZ + parsePatch into JSON shape; rename/binary/no-newline handled |
| `internal/api/diffs.go` | GET /api/tasks/{id}/diff endpoint | ✓ VERIFIED | ResolveBase → Compute; 404/409/500 relays; registered in main.go L103 |
| `cmd/kangent/main.go` | DiffRoutes registration | ✓ VERIFIED | `api.DiffRoutes(mux, db, wtSvc)` L103 |
| `web/src/components/StatusDot.tsx` | null-exitCode gray branch | ✓ VERIFIED | L26-32 first branch → bg-zinc-600 + "Exited" |
| `web/src/api/agents.ts` | resumable field on AgentStatusEntry | ✓ VERIFIED | `resumable: boolean` L11 |
| `web/src/api/sessions.ts` | useResumeAgent mutation | ✓ VERIFIED | L70 hook posting `{resume:true}` w/ cache invalidations |
| `web/src/components/terminal/TerminalPane.tsx` | exitedActions slot (additive) | ✓ VERIFIED | `exitedActions?: ReactNode` L28; 2× `exitedActions ??` (both banner branches) |
| `web/src/components/task/AgentTab.tsx` | resumable pre-start + banner pair | ✓ VERIFIED | resumable single driver; both placements; verbatim copy; no toast/Dialog |
| `web/src/components/ui/collapsible.tsx` | shadcn collapsible block | ✓ VERIFIED | exports Collapsible/Trigger/Content (Radix umbrella) |
| `web/src/api/diffs.ts` | DiffResponse types + useTaskDiff | ✓ VERIFIED | types mirror server JSON; no refetchInterval |
| `web/src/components/task/DiffTab.tsx` | totals bar + file list + states | ✓ VERIFIED | all 4 states + sticky totals + spinning refresh |
| `web/src/components/task/DiffFileSection.tsx` | collapsible per-file renderer | ✓ VERIFIED | >400 collapse gate, binary header-only, scoped green/red palette |
| `web/src/components/task/TaskTabs.tsx` | TabDef disabled support | ✓ VERIFIED | disabled/disabledTooltip + zinc-600 label + tooltip wrap |
| `web/src/pages/TaskPage.tsx` | Diff tab insertion (D-64 order) | ✓ VERIFIED | index-2 placement; conditional tabIds; Pitfall-7 fallback reused |
| `internal/api/recovery_integration_test.go` | restart + resume + diff integration test | ✓ VERIFIED | TestRecoveryLifecycle 7 stages; 4 NewManager; deterministic -count=2 |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| sessions.go create handler | SpawnOpts.ResumeSessionID | resume:true after fresh csid read | ✓ WIRED | L127-132 sets opts.ResumeSessionID = csid.String after gate |
| agents.go status handler | transcriptExists | resumable derivation + DB-derived pass | ✓ WIRED | L102 (manager pass) + L140 (DB pass) |
| diffs.go | worktree.Service.ResolveBase | base re-resolution before Compute | ✓ WIRED | L72 ResolveBase → L78 Compute(path, base) |
| diff.go | git diff --no-index /dev/null | per-untracked exit-1 addition patches | ✓ WIRED | runNoIndex L45-67 invoked per untracked file L153 |
| AgentTab.tsx | useAgentStatuses ['agent-statuses'] | resumable flag lookup (deduped poll) | ✓ WIRED | L43-45 find by taskId → resumable |
| AgentTab.tsx | TerminalPane exitedActions | banner pair injection when resumable | ✓ WIRED | L194 exitedActions={resumePair} |
| DiffTab.tsx | GET /api/tasks/{id}/diff | useTaskDiff on mount + refetch() | ✓ WIRED | diffs.ts queryFn fetches `/api/tasks/${taskId}/diff`; refetch on refresh button |
| TaskPage.tsx | TaskTabs TabDef disabled | Diff at index 2, disabled w/o worktree | ✓ WIRED | L300 disabled binding + conditional tabIds L136 |

_Note: gsd-tools `verify key-links` regex can report false failures (per phase context). All links above verified manually by reading the wired code._

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| AgentTab.tsx (resumable pair) | `resumable` | `useAgentStatuses()` 5s poll → /api/agents/status (real DB+manager derivation) | Yes (server transcript glob) | ✓ FLOWING |
| StatusDot.tsx (gray dot) | `entry.exitCode/status` | same poll; DB-derived entries carry real exitCode:null/resumable | Yes | ✓ FLOWING |
| DiffTab.tsx (file list) | `data` (DiffResponse) | useTaskDiff → /api/tasks/{id}/diff → diff.Compute over real git worktree | Yes (live git plumbing) | ✓ FLOWING |
| recovery_integration_test.go | resumable / argv / diff | real SQLite + real git fixture + fake-claude argv recording | Yes (deterministic) | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| go vet clean | `go vet ./...` | exit 0 | ✓ PASS |
| Full backend suite | `go test ./... -count=1 -p 1` | 6 pkgs ok (api 72.8s, diff, session, store, worktree, ws) | ✓ PASS |
| Restart+resume+diff lifecycle (deterministic) | `go test -run TestRecoveryLifecycle -count=2` | ok 10.7s | ✓ PASS |
| Session race-clean (mutex-adjacent spawn changes) | `go test ./internal/session -race -count=1` | ok 6.2s | ✓ PASS |
| Integration guards (no real claude) | grep recovery_integration_test.go | 4 NewManager, --resume asserted, both 409 bodies, RESUME_FAIL driven, 0 LookPath("claude") | ✓ PASS |
| Frontend build (tsc + vite) | `npm run build` | exit 0, dist emitted | ✓ PASS |
| Single binary assembles | `make build` | exit 0, bin/kangent (18MB, SPA embedded) | ✓ PASS |
| No migration 00004 (schema-free RCVR-01) | grep migrations | count 0 | ✓ PASS |
| Diff color palette scope | `grep -rln 'bg-green-500/10\|bg-red-500/10' src/ \| grep -v Diff` | empty (scoped) | ✓ PASS |

_Real claude binary deliberately NOT invoked (API quota / context). All recovery + resume paths exercised via the fake-claude stub._

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | -------------- | ----------- | ------ | -------- |
| RCVR-01 | 05-01, 05-03, 05-05 | On startup, running-but-dead sessions reconciled to exited (no ghosts) | ✓ SATISFIED | Reconciliation by architecture: no DB "running" row; DB-derived /api/agents/status entries (exitCode:null); gray dot; TestRecoveryLifecycle restart stage |
| RCVR-02 | 05-01, 05-03, 05-05 | Persist Claude session ID; offer "Resume session" via `claude --resume` in worktree after restart | ✓ SATISFIED | manager flag switch + persisted csid; useResumeAgent + Resume/Reset pair both placements; argv-asserted integration; human-approved live |
| REVW-01 | 05-02, 05-04, 05-05 | Read-only diff tab showing worktree changes vs base branch | ✓ SATISFIED | internal/diff + GET /api/tasks/{id}/diff; DiffTab/DiffFileSection; D-64 tab order; diff smoke in integration; human-verified live |

**Orphaned requirements:** none. REQUIREMENTS.md maps exactly RCVR-01, RCVR-02, REVW-01 to Phase 5 (lines 117-119); all three are claimed by phase plans and verified.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| web/src/components/task/DiffTab.tsx | 63 | `if (!data) return null` | ℹ️ Info | Legitimate TS narrowing guard reached only after error/loading states render; all 4 real states (content/empty/loading/error) implemented above it — not a stub |

No TODO/FIXME/PLACEHOLDER, no console.log-only handlers, no hardcoded-empty render data, no stub returns in any phase-5 file.

### Human Verification Required

None outstanding for sign-off. The phase's blocking human-verify checkpoint (05-05 Task 3) was
APPROVED against real claude v2.1.173: restart reconciliation, post-restart + within-run Resume,
Reset-only honesty, never-two-PTYs, the Diff tab, and the full v1 walkthrough.

**Carried open item (NOT a gap, tracked for follow-up):** plan-mode exit-plan-approval → amber dot
(research OQ1) remains empirically unconfirmed. Per phase context and 05-05-SUMMARY, this is not a
Phase 5 regression — the documented fallback is widening the Notification matcher in a follow-up.

### Gaps Summary

No gaps. All three ROADMAP success criteria are achieved in the codebase, each backed by substantive
and wired artifacts, real data flow, and a deterministic test. RCVR-01 ships intentionally schema-free
(reconciliation by architecture — the DB never records "running", so a fresh manager + DB-derived
status entries IS the reconciliation; the absence of migration 00004 is the intended design per D-66,
not a missing artifact). Build gates are green across 6 Go packages, the frontend bundle, and the
embedded single binary, and the live human walkthrough is approved. Phase goal achieved; v1 milestone
gate passed.

---

_Verified: 2026-06-11T13:00:00Z_
_Verifier: Claude (gsd-verifier)_
