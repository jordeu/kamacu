---
phase: 24-session-bar-board-tab-polish
plan: 04
subsystem: web
tags: [react, typescript, tanstack-query, tabs, rename, sessions, http-patch]

# Dependency graph
requires:
  - phase: 24-session-bar-board-tab-polish
    plan: 01
    provides: "PATCH /api/sessions/{id} accepting { label }, returning the updated TermSession Info; tmux-tab label persistence + Bash N re-derive on empty commit"
  - phase: 02-terminal
    provides: "TaskTabs tab-strip seam (TabDef[]) with the × affordance + button-in-button constraint"
  - phase: 01-foundation
    provides: "TaskPage title inline-edit contract (click→Input, Enter/Esc/blur, cancelRef) reused verbatim for the tab editor"
provides:
  - "useRenameSession(taskId) — TanStack mutation PATCHing /api/sessions/{id} with { label }, optimistic scoped-cache write + invalidate"
  - "TabDef.onRename — opt-in double-click inline rename affordance on a tab (bash/tmux only)"
  - "TaskTabs double-click inline editor: Enter commits, Esc cancels, blur commits; empty commit resets to Bash N (D-05); single-click still selects"
  - "TaskPage wires onRename on every running bash tab (D-03); Agent/Description/Diff keep fixed labels (D-02)"
affects: [tab-rename-ui, session-bar-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Inline click-to-edit (Enter/Esc/blur + cancelRef) lifted from TaskPage's title editor onto the tab strip, diverging only in the D-05 empty-commit-is-a-reset branch"
    - "PATCH mutation + optimistic setQueryData scoped-cache write + invalidate (copied from useSpawnSession/useStopSession)"

key-files:
  created: []
  modified:
    - web/src/api/sessions.ts
    - web/src/components/task/TaskTabs.tsx
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "Rename editor rendered only for the tab whose label is double-clicked (per-tab renamingId), inside TabsTrigger next to the × span — same button-in-button stopPropagation posture the × already respects"
  - "onRename conditioned on s.status === running (mirrors the onClose gate): an exited/closing bash tab is not renamable because a rename can't persist there"
  - "Empty/whitespace commit sends onRename(\"\") as an explicit reset; the server (Plan 01) re-derives Bash N and the returned session's label flows back through the optimistic cache write — no extra client logic"

patterns-established:
  - "TabDef opt-in affordance fields (onClose, now onRename) are set by the parent per-tab; fixed tabs simply omit them"

requirements-completed: [TABS-01, TABS-02]

# Metrics
duration: ~15min
completed: 2026-07-03
---

# Phase 24 Plan 04: Renameable Bash/Tmux Tabs (Frontend) Summary

**Double-clicking a bash/tmux tab label swaps it to an inline Input (Enter commits, Esc cancels, blur commits); committing PATCHes /api/sessions/{id} so the custom name persists (and, for tmux, survives a restart), while an empty commit resets the tab to its Bash N default — Agent/Description/Diff keep fixed labels.**

## Performance

- **Duration:** ~15 min
- **Tasks:** 3
- **Files modified:** 3

## Accomplishments

- `useRenameSession(taskId)` in `sessions.ts` — a `useMutation<TermSession, ApiError, { id; label }>` PATCHing `/api/sessions/${id}` with `{ label }`; on success it optimistically maps the returned `TermSession` into the scoped `["sessions", taskId]` cache (copying `useSpawnSession`'s write) then invalidates `["sessions"]`, so the renamed (or reset-to-`Bash N`) label renders immediately with no extra client logic. Added `patch` to the `./client` import.
- `TaskTabs.tsx` — an optional `onRename?: (label: string) => void` on `TabDef` plus a double-click inline editor: local `renamingId`/`draft` state and a `cancelRenameRef` mirroring TaskPage's `cancelTitleEditRef`. Double-clicking a label (gated on `tab.onRename`) opens an `<Input autoFocus>` in place of the label span; Enter→blur (commit), Esc→cancelRef+blur (no save), blur→commit. Per D-05 the commit path does NOT early-return on empty — a trimmed-empty commit calls `onRename("")` (reset); a non-empty value calls `onRename(trimmed)` only when it differs from the current label. The input `stopPropagation`s pointer/click (and the double-click `stopPropagation`s) so single-click still selects the tab via the Radix trigger.
- `TaskPage.tsx` — imported + instantiated `useRenameSession(taskId)` and wired `onRename: (label) => renameSession.mutate({ id: s.id, label })` onto every bash `TabDef` in `visibleSessions.map`, gated on `s.status === "running" && !closing`. Agent/Description/Diff tab defs are untouched (no `onRename`), so they keep fixed labels (D-02).

## Task Commits

1. **Task 1: Add useRenameSession mutation (D-03)** — `aa0989e` (feat)
2. **Task 2: Double-click inline rename editor in TaskTabs (D-01/D-02/D-05)** — `e9247e0` (feat)
3. **Task 3: Wire per-tab onRename in TaskPage (D-02/D-03)** — `e03efbd` (feat)

## Files Created/Modified

- `web/src/api/sessions.ts` — imported `patch`; added the `useRenameSession` mutation (PATCH + optimistic scoped-cache write + invalidate).
- `web/src/components/task/TaskTabs.tsx` — imported `Input` + `useRef`/`useState`; added `TabDef.onRename`; added `renamingId`/`draft`/`cancelRenameRef` state and a `commitRename` helper (D-05 empty-reset divergence); swapped the label span for the double-click inline `<Input>` editor.
- `web/src/pages/TaskPage.tsx` — imported + instantiated `useRenameSession`; added the running-gated `onRename` to each bash `TabDef`.

## Discretion Decisions (required by plan output spec)

- **Editor scope — the double-clicked tab only.** The editor renders for whichever tab's label was double-clicked (`renamingId === tab.id`), not just "the active tab": double-clicking selects and edits in one gesture, and a background tab can't be edited without first double-clicking it (which selects it). This is strictly more precise than "active tab only" and satisfies the plan's discretion note.
- **`onRename` gated on running sessions.** `onRename` is passed only when `s.status === "running" && !closing` — the same gate the existing `onClose` uses. An exited/muted/closing bash tab is therefore not renamable, because its session is gone and a rename can't persist meaningfully. This is the plan's recommended discretion choice, documented here.
- **Empty-commit reset relies on the server re-derive.** The client sends `onRename("")` on an empty/whitespace commit and does no local `Bash N` computation; the Plan-01 backend re-derives `Bash N` from the tmux name and returns it in the `TermSession`, which the mutation's optimistic cache write reflects. The client never needs to know the ordinal.

## Deviations from Plan

None — plan executed exactly as written.

## Provisioning Note (not a deviation)

The fresh worktree had no `web/node_modules`, so `tsc`/`eslint` were absent and the initial verification failed with "tsc: not found". Resolved by running `npm ci` from the committed `web/package-lock.json` — this installs the existing pinned dependency set (no new packages; the plan adds none), so it is dependency provisioning, not a package install subject to the slopcheck checkpoint. After provisioning, `npm run build` is green.

## Verification

- `cd web && npm run build` (tsc -b + vite build) — **exit 0, green** (all three tasks).
- Lint: the two new/edited component files that this plan owns — `sessions.ts` and `TaskTabs.tsx` — lint **clean (0 errors)**. `TaskPage.tsx` reports exactly **1** `react-hooks/set-state-in-effect` error on the pre-existing `keepExitedIds` `useEffect` (line ~131) — this is the documented pre-existing tech-debt baseline (STATE.md / PROJECT.md: "~18–20 pre-existing react-hooks eslint errors … gating build is green"), NOT introduced by this plan (my edits were an import, a hook instantiation, and an `onRename` field — none touch that effect). Zero new lint errors added.
- Whole-tree `npm run lint` reports 21 pre-existing errors across unrelated files (`use-mobile.ts`, `TaskPage.tsx`, etc.); all are the known baseline and out of scope per the executor scope boundary. Logged, not fixed.
- Manual/human-verify (deferred to the phase execute-phase checkpoint): double-click a bash tab → rename → reopen the task → label persists; for a tmux tab, restart the server → label persists (never "Bash ?"); an empty commit resets to Bash N; single-click still selects; Agent/Description/Diff show no editor.

## Embedded SPA / dist Note

`web/dist/*` is gitignored except the committed `web/dist/index.html` placeholder (root `.gitignore`: `web/dist/*` + `!web/dist/index.html`) — the real bundles are built fresh by `make build` at release time. Prior phases leave the placeholder's stale hashes committed rather than re-committing per-build output, so this plan does the same: after building for verification, the `web/dist/index.html` placeholder was restored to the base-commit state. No dist artifacts are committed by this plan.

## Threat Surface

No new security surface. The rename input crosses to the server via the Plan-01 `PATCH /api/sessions/{id}` (server-side trim + 200-rune cap + unknown-id 404 owned by Plan 01); the label renders back only as a React text node in the tab trigger (auto-escaped) and never touches the PTY. The mutation targets `s.id` from the task's own scoped `useSessions` list. No new endpoints, auth paths, or schema — nothing outside the plan's `<threat_model>`.

## Self-Check: PASSED

- Files verified present: `web/src/api/sessions.ts`, `web/src/components/task/TaskTabs.tsx`, `web/src/pages/TaskPage.tsx`, `24-04-SUMMARY.md`
- Commits verified reachable: `aa0989e`, `e9247e0`, `e03efbd`
- `npm run build` green; the two owned files lint clean; the sole `TaskPage.tsx` lint error is the pre-existing baseline

---
*Phase: 24-session-bar-board-tab-polish*
*Completed: 2026-07-03*
