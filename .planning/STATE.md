---
gsd_state_version: 1.0
milestone: v1.1
milestone_name: Settings & Polish
status: roadmap_complete
stopped_at: Roadmap created for v1.1 (Phase 6)
last_updated: "2026-06-11T11:10:07.737Z"
last_activity: 2026-06-11
progress:
  total_phases: 1
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-06-11)

**Core value:** One place to see and drive all agent work: every task gets its own isolated worktree and a persistent Claude Code session you can open, leave, and reattach to from the browser.
**Current focus:** Milestone v1.1 — Phase 6 Settings & Polish (roadmap complete, ready to plan)

## Current Position

Phase: Phase 6 — Settings & Polish (not started)
Plan: —
Status: Roadmap complete — ready for `/gsd:plan-phase 6`
Last activity: 2026-06-11 — v1.1 roadmap created (Phase 6, 12/12 requirements mapped)

Progress: [░░░░░░░░░░] 0%

## Performance Metrics

**Velocity:**

- Total plans completed: 0 (this milestone)
- Average duration: -
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| 6 | - | - | - |

**Recent Trend:**

- Last 5 plans: -
- Trend: -

*Updated after each plan completion*
| Phase 01 P01 | 4 min | 2 tasks | 7 files |
| Phase 01 P03 | 10 min | 3 tasks | 41 files |
| Phase 01 P02 | 8 min | 3 tasks | 7 files |
| Phase 01 P04 | 7 min | 2 tasks | 7 files |
| Phase 01 P05 | 8 min | 3 tasks | 6 files |
| Phase 01 P06 | 8 min | 2 tasks | 4 files |
| Phase 01 P07 | 30 min | 3 tasks | 8 files |
| Phase 02 P02 | 3 min | 2 tasks | 5 files |
| Phase 02 P01 | 12 min | 2 tasks | 5 files |
| Phase 02 P04 | 8 min | 3 tasks | 2 files |
| Phase 02 P03 | 20 min | 3 tasks | 9 files |
| Phase 02 P05 | 48 min | 3 tasks | 3 files |
| Phase 03 P02 | 11 min | 2 tasks | 6 files |
| Phase 03 P01 | 13 min | 2 tasks | 2 files |
| Phase 03 P03 | 19 min | 3 tasks | 11 files |
| Phase 03 P04 | 15 min | 3 tasks | 7 files |
| Phase 03 P05 | 6 min | 2 tasks | 3 files |
| Phase 03 P06 | 27 min | 3 tasks | 1 files |
| Phase 04-claude-code-agent-sessions P03 | 4 min | 2 tasks | 4 files |
| Phase 04 P01 | 11 min | 2 tasks | 5 files |
| Phase 04 P02 | 18 min | 3 tasks | 16 files |
| Phase 04-claude-code-agent-sessions P04 | 6 min | 3 tasks | 5 files |
| Phase 04-claude-code-agent-sessions P05 | 25 min | 3 tasks | 8 files |
| Phase 05 P02 | 7 min | 3 tasks | 7 files |
| Phase 05-recovery-review P01 | 17 min | 3 tasks | 9 files |
| Phase 05 P03 | 4 min | 3 tasks | 5 files |
| Phase 05 P04 | 11 min | 3 tasks | 6 files |
| Phase 05-recovery-review P05 | 55 min | 3 tasks | 1 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v1.1 Roadmap]: Single Phase 6 for all of v1.1 — coarse granularity + cohesive settings plumbing into existing v1.0 call sites (agent spawn, worktree create, shell spawn, branch naming) means no compelling dependency reason to split
- [v1.1 Roadmap]: AGENT-02 intentionally REVERSES D-51 — the claude extra-params field defaults to including `--dangerously-skip-permissions`, flipping v1.0's interactive-by-default posture; the user can remove the flag. Log the D-51 reversal in PROJECT.md Key Decisions at phase transition.
- [v1.1 Roadmap]: Settings are global-only (per-project overrides deferred as SET-FUT-01), applied at next spawn/creation with no restart (SET-03), stored in a new SQLite settings table (SET-02)
- [v1.1 Roadmap]: Worktree-location change affects new creations only (WT-02); existing worktrees keep stored absolute paths — never migrate live trees
- [v1.1 Roadmap]: Shell dropdown is bash-only in v1.1 but the value still comes from settings, so additional shells (SHELL-FUT-01) are future data not code
- [Roadmap]: Coarse granularity — research's 6 suggested phases compressed to 5 by merging foundation + kanban CRUD into Phase 1
- [Roadmap]: Terminal engine (Phase 2) built and proven against plain bash before Claude Code enters the picture — highest-risk subsystem de-risked first
- [Roadmap]: WS security (Origin/Host validation + per-instance token) bound to Phase 2, the phase that exposes the endpoint — not deferred hardening
- [Roadmap]: Bash tabs (TERM-04) assigned to Phase 3, where worktree cwds first exist
- [Phase 01]: Module path is 'kangent' (local-only single binary), not a github.com path
- [Phase 01]: goose dialect 'sqlite3' paired with modernc driver name 'sqlite' — intentionally different strings
- [Phase 01]: No --open/auto-browser flag in v1 server startup (default off)
- [Phase 01]: shadcn CLI 4.x: --base-color removed; init with -b radix --preset nova, zinc ramp hand-encoded in index.css per UI-SPEC
- [Phase 01]: No webfonts: removed preset-injected @fontsource-variable/geist; system sans/mono stacks only (offline-clean binary)
- [Phase 01]: TS 6 deprecates baseUrl: @ alias uses paths-only in tsconfigs
- [Phase 01]: Duplicate repo_path detected via SELECT pre-check, not constraint-error parsing (single-user scale)
- [Phase 01]: Move endpoint rejects after_id == moving task id as invalid after_id
- [Phase 01]: Sidebar collapse persisted via controlled SidebarProvider state + localStorage key kangent.sidebar (shadcn cookie write is never read back in an SPA)
- [Phase 01]: Server validation errors mirrored inline sentence-cased under the path field; 409 duplicate message gets trailing period to match UI-SPEC copy
- [Phase 01]: Deleting the currently-routed project refetches projects before navigating to / to avoid redirecting into the deleted project from stale cache
- [Phase 01]: Board column derivation guard includes moveTask.isPending (not just active drag) to prevent post-drop snap-back flicker before the optimistic cache write lands
- [Phase 01]: shadcn DialogContent width overrides need the responsive variant (sm:max-w-[560px]) since the component ships sm:max-w-sm
- [Phase 01]: Accent blue-500 applied via explicit utility classes (tab indicator, prose links) instead of editing shared index.css — parallel wave-3 plans own no shared files
- [Phase 01]: Task title edit commits through a single onBlur path (Enter/Esc funnel through blur with a cancel ref) to avoid double-mutate races
- [Phase 01]: Committed placeholder web/dist/index.html (gitignore web/dist/* with !index.html) so go build never fails on fresh clone; real build output never committed
- [Phase 01]: AppLayout wrapped once in TooltipProvider delayDuration=0 — Radix tooltips without a provider throw on first render and blank the whole app
- [Phase 01]: Collapsed-sidebar toggle gets a reserved pl-9 gutter on main instead of floating over page headers
- [Phase 02]: xterm deps pinned exactly (no caret) — addon set must move together with xterm 6
- [Phase 02]: selectionForeground left unset in zincTheme to preserve cell colors under selection
- [Phase 02]: Sessions list query uses refetchInterval 5000 to keep non-attached rows' status honest; no zustand this phase
- [Phase 02]: Stop signals every process group in the shell's session via /proc scan, not just the leader pgroup — interactive bash job control puts background jobs in their own pgroups (TERM-06 would silently break otherwise)
- [Phase 02]: Session exit notification is Done() channel + Info().ExitCode after done; WS layer sends the 'x' frame when Done fires
- [Phase 02]: Manager.Remove returns ErrNotFound (added beyond interface contract) so the REST layer maps missing to 404 vs ErrStillRunning to 409
- [Phase 02]: retry() reconnects immediately at attempt 0; the 5-step backoff applies to subsequent failures only
- [Phase 02]: Post-connect resize force-sent (bypassing change detection) so the server SIGWINCH jiggle fires on reattach with unchanged dimensions
- [Phase 02]: Focus ring scoped via has-[.xterm-helper-textarea:focus-visible] — pointer clicks may still show it (textarea focus-visible heuristic); verify live in 02-05
- [Phase 02]: WS writer treats a closed attach queue as its exit signal: session exited → 'x' frame + 1000 close; otherwise slow-consumer drop → 1013 close so the client reconnects with fresh replay
- [Phase 02]: SIGWINCH/jiggle tested behaviorally with a WINCH trap inside a foreground NON-interactive bash -c child — interactive bash defers user WINCH traps, so prompt-level traps never fire
- [Phase 02]: hostCheck hostname allowlist is port-agnostic (Vite proxy forwards Host: 127.0.0.1:5173); ensureLoopback refuses non-localhost hostnames unresolved
- [Phase 02]: Spawn-select race closed by attaching from the spawn mutation result while the sessions-list invalidation is in flight
- [Phase 02]: Integration test marker uses shell quote-splitting (mar''ker) so PTY command echo never satisfies output assertions
- [Phase 02]: Host-header 403 asserted in cmd/kangent main_test.go (hostCheck unexported); integration test covers the Origin CSWSH rejection end-to-end
- [Phase 02]: Reattach replay asserted on the FIRST WS data frame — replay must precede live output
- [Phase 03]: Separate global seq counter on Manager keeps List ordering tiebreak correct across both label families (per-task counters would collide on seq)
- [Phase 03]: Spawn cwd validation rejects existing-but-not-a-directory paths with the same error shape as nonexistent paths, before any PTY allocation
- [Phase 03]: ListByTask(0) returns only unscoped dev sessions — taskID 0 is the dev sentinel, not a wildcard
- [Phase 03]: ResolveBase verifies the current-branch ref resolves before returning it: symbolic-ref --short HEAD exits 0 on unborn HEAD, so an extra show-ref check is required for the D-25 error path
- [Phase 03]: Worktree test isolation uses a controlled temp GIT_CONFIG_GLOBAL with protocol.file.allow=always — submodule clone subprocesses read global config, not the superproject's repo-local config
- [Phase Phase 03]: provisionWorktree owns the outcome UPDATEs (success clears worktree_error) so task-create and the POST /worktree retry path share one persistence flow
- [Phase Phase 03]: GET/DELETE /worktree treat a manually deleted worktree dir as clean (dirty=0) — Remove self-heals git bookkeeping, so a missing tree never blocks cleanup
- [Phase Phase 03]: Worktree cleanup 409 bodies are exact API contract: 'sessions running' and 'worktree has uncommitted changes' — dialog-variant discriminators for 03-05
- [Phase 03]: Tab x renders only while running && !closing — closing/exited tabs drop the affordance; exited tabs close via the Phase 2 banner (chosen reading of UI-SPEC's 'x disabled')
- [Phase 03]: TaskPage is a flex h-full column (TerminalPage pattern), not viewport calc(): bash tabs fill below the strip; Description scrolls in its 860px island
- [Phase 03]: TabDef gained keepMounted — onClose presence can't discriminate mounting since closing tabs lose onClose but must keep their WS (forceMount + data-inactive:hidden)
- [Phase 03]: Verbatim UI-SPEC copy kept in single template literals in JSX so contract phrases stay grep-able and line-wrapping can never split them
- [Phase 03]: Cleanup dialog onError refetches worktree state so a 409 raced-state response re-derives the correct variant in place (Pitfall 8 server re-check)
- [Phase 03]: Done-transition cleanup offer fires from a call-site onSuccess on moveTask.mutate — shared useMoveTask hook untouched, offer predicated on the server response Task
- [Phase 03]: Integration-test git repo created on the outer t (not a subtest) so subtest TempDir cleanup never deletes .git/worktrees bookkeeping out from under later lifecycle stages
- [Phase 03]: Lifecycle test cwd proven via echo mark:$PWD expansion — echoed input only contains the literal $PWD, so PTY command echo can never satisfy the assertion
- [Phase 04]: CardRow extracted as the single inner card layout shared by CardShell, TaskCard, and TaskCardOverlay; per-card useAgentStatuses calls dedupe on the shared ["agent-statuses"] key
- [Phase 04]: shrink-0 lives on the StatusDot tooltip wrapper span (the real flex item) as well as the inner dot, so card consumers pass only the mt-[6px] optical offset
- [Phase 04]: Agent-status pump logic factored into noteAgentOutputLocked so tests drive the real code path via in-package _test helpers instead of flaky PTY byte injection
- [Phase 04]: Kangent session id generated up front in Spawn so the agent overlay hook URL embeds it before the process starts
- [Phase 04]: stopRequested set inside stopOnce.Do before SIGTERM so natural exits can never observe it true (exit 143 renders gray only when Kangent asked)
- [Phase 04]: kind=agent without task_id returns the same 409 'task has no worktree' body as a worktree-less task — one copy contract for the no-worktree gate
- [Phase 04]: Hook receiver rejects an empty configured token outright so the endpoint can never run open; token compared constant-time and checked before any session work
- [Phase 04]: Exited-session hook POSTs are ignored with 200 before body decode — late async curls are benign even when malformed
- [Phase 04]: api and ws test packages carry their own fake-claude stub writers via AgentConfig.ClaudeBin — the real claude binary is never spawned in tests
- [Phase 04]: showExitedClose gates Close on both the exited and not-found banners — a permanent Agent tab never offers Close on any banner
- [Phase 04]: TerminalPane onConnect fires in the existing conn-state effect via a ref so the D-45 optimistic clear re-fires on every reconnect, mirroring the server's per-attach clear
- [Phase 04]: Agent tab is the universal dangling-tab fallback (Pitfall-7 effect and removeTab) since it can never disappear
- [Phase 04-claude-code-agent-sessions]: Exited agent terminal renders dimmed; banner is code-free 'Agent session ended.'; primary action renamed 'Reset session' (D-41 revised after checkpoint feedback)
- [Phase 04-claude-code-agent-sessions]: Phase 5 Resume design note (RCVR-02): exited banner gains 'Resume session' primary via claude --resume + persisted claude_session_id; 'Reset session' becomes secondary
- [Phase 04-claude-code-agent-sessions]: Agent lifecycle locked in by 8-stage integration test on ClaudeBin fake-claude stub — real claude never spawned in CI
- [Phase 05]: internal/diff owns a private git runner pair: exit-0 run() for normal git, runNoIndex() honoring git diff --no-index exit-1-as-success so untracked files render as full additions without git add -N (Pitfall 1)
- [Phase 05]: Diff JSON is server-pre-structured per-file-hunks (Pattern 4); GET /api/tasks/{id}/diff re-resolves base via ResolveBase and excludes base movement via merge-base three-dot semantics (D-59)
- [Phase 05-recovery-review]: RCVR-01 reconciled by architecture: DB never records 'running', so a fresh manager + DB-derived /api/agents/status entries is the whole reconciliation — no migration, no startup mutation pass
- [Phase 05-recovery-review]: Resumability derived from transcript-glob existence (~/.claude/projects/*/<uuid>.jsonl), not a DB status column — matches claude's global lookup, self-heals D-56, claude_session_id never cleared
- [Phase 05-recovery-review]: Resume is a body-flag variant of POST /api/sessions riding the existing one-per-task 409 gate (D-67); validated AFTER the gate with a 409 'no session to resume' server-side honest-failure guard
- [Phase 05-recovery-review]: Frontend resumable is server-driven only: AgentTab reads the resumable flag from the deduped ['agent-statuses'] poll to pick which Reset/Resume pair renders — the UI never infers resumability client-side (no new props through TaskPage)
- [Phase 05-recovery-review]: dotMeta null-exit-code branch (gray, code-free 'Exited' tooltip) ordered BEFORE the exitCode-0 ternary so DB-derived post-restart entries never render red (D-57 / Pitfall 3)
- [Phase 05-recovery-review]: TerminalPane.exitedActions added to BOTH exited and not-found banner branches (a restart while attached surfaces as WS not-found); undefined default renders Phase 4 banner byte-for-byte so bash call sites are unaffected

### Pending Todos

None yet.

### Blockers/Concerns

- [Phase 6 / D-51 reversal]: AGENT-02 defaults the extra-params field to `--dangerously-skip-permissions`, reversing v1.0's interactive-by-default D-51 — intentional; log the reversal in PROJECT.md Key Decisions at transition.
- Plan-mode exit-plan approval → amber dot unconfirmed by user during 04-05 live verification — carried bug to confirm during v1.1 UAT (research OQ1)

## Session Continuity

Last session: 2026-06-11 — v1.1 roadmap created
Stopped at: Roadmap complete for Phase 6 (Settings & Polish), 12/12 requirements mapped
Resume file: .planning/ROADMAP.md
Next: `/gsd:plan-phase 6`
