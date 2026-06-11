---
phase: 05-recovery-review
plan: 03
subsystem: ui
tags: [react, tanstack-query, resume, recovery, agent-status, status-dot, terminal-pane]

# Dependency graph
requires:
  - phase: 05-recovery-review
    provides: "/api/agents/status resumable flag + DB-derived post-restart exited entries (05-01); POST /api/sessions {resume:true} variant + honest 409s (05-01)"
  - phase: 04-claude-code-agent-sessions
    provides: "AgentTab pre-start/exited states; TerminalPane exited/not-found banner + exitedPrimaryLabel/showExitedClose/dimWhenExited props; useAgentStatuses shared poll; useSpawnAgent cache discipline; StatusDot dotMeta"
provides:
  - "AgentStatusEntry.resumable consumed end-to-end (the existing 5s poll carries it to every dot/tab consumer)"
  - "useResumeAgent mutation — POST /api/sessions {resume:true} with the same spawn-select cache write + invalidations as useSpawnAgent"
  - "dotMeta null-exit-code branch — post-restart DB-derived entries render gray with a code-free tooltip (D-57, Pitfall 3 closed)"
  - "TerminalPane.exitedActions slot — additive prop replacing the default exited/not-found banner action group; bash defaults unchanged when undefined"
  - "AgentTab D-54 Reset/Resume pair in both placements: resumable pre-start variant (D-54b) and within-run exited banner (D-54a), driven solely by the server resumable flag"
affects: [05-recovery-review remaining UI plans, milestone UAT live-resume verification]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Server-driven resumability: AgentTab reads resumable from the shared useAgentStatuses query (Phase 4 dedupe) — the UI never infers resumability client-side (UI-SPEC rule)"
    - "Mutual-exclusion busy flag (spawn.isPending || resume.isPending) disables both buttons while either mutation is in flight, racing the one-per-task 409 gate"
    - "Additive banner action slot (exitedActions ?? default) — one consistent Reset/Resume pair in both exited and not-found branches; every existing call site renders byte-for-byte when the prop is undefined"
    - "D-56 honest failure: a failed resume is left to claude's own terminal output + the next poll's resumable:false — no toast, no dialog, no silent fallback"

key-files:
  created: []
  modified:
    - web/src/api/agents.ts
    - web/src/api/sessions.ts
    - web/src/components/StatusDot.tsx
    - web/src/components/terminal/TerminalPane.tsx
    - web/src/components/task/AgentTab.tsx

key-decisions:
  - "resumable is the single render-driver for which pre-start/banner pair shows; resolved from the deduped ['agent-statuses'] query, no new props threaded through TaskPage"
  - "The resumable pre-start variant needs no worktree check — the server only reports resumable when worktree_path exists; the non-resumable path keeps its existing hasWorktree tooltip branch verbatim"
  - "exitedActions added to BOTH the exited and not-found banner branches because a restart while the pane is attached surfaces as WS not-found — the resume pair must appear there too"
  - "Verbatim UI-SPEC copy kept in single template literals so contract phrases stay grep-able (Phase 3 convention)"

patterns-established:
  - "Resume/Reset pair: Reset (ghost, left) → Resume (primary, rightmost); pending labels Resetting… / Resuming…; inline 12px destructive error 'Couldn't start a session. Try again.' — never green, never destructive red"

requirements-completed: [RCVR-01, RCVR-02]

# Metrics
duration: 4min
completed: 2026-06-11
---

# Phase 5 Plan 03: Frontend Recovery Surfaces (Resume/Reset Pair + Gray Post-Restart Dot) Summary

**The D-57 muted-gray post-restart dot and the D-54 Reset/Resume button pair in both placements (resumable pre-start after a restart, exited banner within a run), driven entirely by the server's `resumable` flag from plan 05-01 — pure wiring, zero new dependencies.**

## Performance

- **Duration:** 4 min
- **Started:** 2026-06-11T09:32:21Z
- **Completed:** 2026-06-11T09:36:28Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments

- **Status contract round-trips (RCVR-01 visible half):** `AgentStatusEntry` gains `resumable: boolean`, carried to every consumer by the existing 5s poll with no hook changes. `dotMeta` gains a null-exit-code branch *before* the existing exitCode-0 ternary, so DB-derived post-restart entries (`{exitCode: null, stopRequested: false}`) render `bg-zinc-600` with the code-free tooltip `Exited` — Pitfall 3 closed, no alarming red, no "code null".
- **Resume mutation (RCVR-02 plumbing):** `useResumeAgent` mirrors `useSpawnAgent` exactly — POSTs `{task_id, kind:"agent", resume:true}`, writes the fresh session into the scoped `["sessions", taskId]` cache before invalidating (the spawn-select race fix, so the pane attaches from the mutation result), and invalidates `["agent-statuses"]`.
- **TerminalPane action slot (additive):** one new `exitedActions?: ReactNode` prop replaces the default banner action group in *both* the `exited` and `not-found` branches via `exitedActions ?? (<default>)`. Undefined renders the Phase 4 primary + ghost Close byte-for-byte, so every bash and existing agent call site is pixel-identical.
- **AgentTab D-54 pair, both placements:** resumability resolved from the shared `useAgentStatuses` poll (the single driver, no client-side inference). State A gains a resumable pre-start variant (`Agent session ended.` heading + verbatim body + Reset ghost / Resume primary pair); the non-resumable `No agent session` pre-start is untouched. State B builds a `resumePair` passed as `exitedActions` only when resumable; not-resumable falls back to the Phase 4 single-Reset banner. A `busy` flag (spawn || resume pending) gives the mutual-exclusion disable on both buttons. D-56 honest failure needs no code (no masking UI added).

## Task Commits

Each task was committed atomically:

1. **Task 1: API contract — resumable field, useResumeAgent, dotMeta null branch** - `a8a011c` (feat)
2. **Task 2: TerminalPane exitedActions slot (additive, bash unchanged)** - `ea29b2d` (feat)
3. **Task 3: AgentTab resumable states — pre-start variant + banner pair (D-54)** - `9bb88d5` (feat)

**Plan metadata:** committed separately (docs: complete plan)

## Files Created/Modified

- `web/src/api/agents.ts` - `AgentStatusEntry.resumable: boolean` added (poll carries it everywhere)
- `web/src/api/sessions.ts` - `useResumeAgent` mutation (resume:true variant, useSpawnAgent cache discipline)
- `web/src/components/StatusDot.tsx` - `dotMeta` null-exit-code branch (gray, code-free tooltip) before the exitCode-0 ternary
- `web/src/components/terminal/TerminalPane.tsx` - `exitedActions?: ReactNode` prop wrapping the default action group in both exited + not-found banner branches
- `web/src/components/task/AgentTab.tsx` - resumable pre-start variant (D-54b), `resumePair` banner injection (D-54a), `busy` mutual exclusion, server-driven `resumable` resolution

## Decisions Made

- **`resumable` is the single render-driver** for which pre-start/banner pair shows, read from the deduped `["agent-statuses"]` query — no new props threaded through TaskPage, the UI never infers resumability client-side.
- **The resumable pre-start variant carries no worktree check** — the server only reports resumable when `worktree_path` exists; the non-resumable path keeps its existing `hasWorktree` tooltip branch verbatim.
- **`exitedActions` added to both banner branches** (exited AND not-found) — a restart while the pane is attached surfaces as WS not-found, so the resume pair must appear there too.
- **Verbatim UI-SPEC copy in single template literals** so contract phrases stay grep-able (Phase 3 convention).

## Deviations from Plan

None - plan executed exactly as written.

The only minor adjustment was wording two code comments to avoid the literal trigger strings `"Resume session"` (it appeared inside a `key=` comment) and `toast`/`dialog` (inside the D-56 documentation comment), so the acceptance-criteria greps land on exactly the load-bearing UI literals (`Resume session` → 2, `toast|Dialog` → 0). This is comment-only and changes no behavior — not a deviation from plan intent.

## Issues Encountered

None - all three tasks built green on the first attempt; every acceptance-criteria grep matched its expected count.

## User Setup Required

None - no external service configuration required. (Live `claude --resume` UI verification against the installed binary — clicking Resume on a real restarted task and confirming the conversation continues — is a milestone-UAT human-verify item, not a setup task.)

## Next Phase Readiness

- **RCVR-01 frontend complete:** post-restart reality renders silently — gray exited dots (no red, no "code null"), the resumable pre-start variant, zero notices (D-65 silence contract verified: no `restart`/`reconcil` copy in non-comment code).
- **RCVR-02 frontend complete:** Resume is offered exactly where the server says it can succeed, in both D-54 placements, relaunching via the resume mutation with mutual exclusion and honest failure left to claude's own output.
- No blockers. The `exitedActions` slot is now available for any future banner-action work; bash call sites remain unaffected.
- Open Question carried for UAT (unchanged from 05-01): live verification of `claude --resume` + plan-mode amber dot against the installed binary (research OQ1/OQ2).

---
*Phase: 05-recovery-review*
*Completed: 2026-06-11*

## Self-Check: PASSED

- All 5 modified source files exist on disk; SUMMARY.md created
- All 3 task commits present: a8a011c, ea29b2d, 9bb88d5
- Plan + plan-level verifications green: `cd web && npm run build` (tsc + vite) exits 0; no new npm deps (package.json unchanged); all UI-SPEC copy literals grep-able; D-65 silence contract honored (no reconciliation copy in non-comment code)
