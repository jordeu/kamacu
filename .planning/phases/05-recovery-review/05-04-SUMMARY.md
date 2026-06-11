---
phase: 05-recovery-review
plan: 04
subsystem: ui
tags: [react, tanstack-query, shadcn, collapsible, radix, diff, xterm-adjacent, lucide]

# Dependency graph
requires:
  - phase: 05-recovery-review
    provides: "GET /api/tasks/{id}/diff structured per-file-hunk JSON (Pattern 4 contract) — plan 05-02"
  - phase: 03-worktree-lifecycle
    provides: "TabDef seam + disabled-tooltip span pattern (bash + button); TaskPage Pitfall-7 layout effect"
  - phase: 04-claude-code-agent-sessions
    provides: "Agent tab as the universal dangling-tab fallback; TerminalPane 150ms no-flash delay pattern"
provides:
  - "web/src/api/diffs.ts: DiffResponse/DiffFile/DiffHunk/DiffLine types + useTaskDiff(taskId) fetch-on-activation query (no polling)"
  - "web/src/components/ui/collapsible.tsx: sanctioned shadcn collapsible block (Radix — unmounts closed content)"
  - "web/src/components/task/DiffFileSection.tsx: collapsible per-file unified diff renderer with scoped green/red content palette"
  - "web/src/components/task/DiffTab.tsx: totals bar + file list + content/empty/loading/error states"
  - "TabDef.disabled + disabledTooltip; Diff tab wired at index 2 in TaskPage (D-64)"
affects: [05-05, 05-recovery-review]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Diff renderer is a dumb map over the server's pre-structured JSON — zero client-side diff parsing, no syntax highlighting (add/remove line coloring only)"
    - "Diff content palette (green-400/red-400, bg-*-500/10) scoped to DiffTab/DiffFileSection ONLY — content-semantic colors exempt from the chrome budget, never bleed into tab label/refresh/borders"
    - "Disabled-tab pattern: TabsTrigger disabled (zinc-600 label) wrapped in a span so the explanation tooltip fires despite swallowed pointer events — mirror of the bash + pattern"
    - "Fetch-on-activation via mount: DiffTab mounts only while the Diff tab is active (not keepMounted) and TanStack staleTime 0 refetches on mount — D-61 with no polling and no extra effects"

key-files:
  created:
    - web/src/components/ui/collapsible.tsx
    - web/src/api/diffs.ts
    - web/src/components/task/DiffFileSection.tsx
    - web/src/components/task/DiffTab.tsx
  modified:
    - web/src/components/task/TaskTabs.tsx
    - web/src/pages/TaskPage.tsx

key-decisions:
  - "Sanctioned collapsible block vendors via the radix-ui umbrella package (already a dep at ^1.5.0 under the radix-nova shadcn style) — ZERO new dependencies added, stronger than the plan's 'exactly one new dep' expectation"
  - "DiffTab is intentionally NOT keepMounted: remount-on-activation IS the D-61 fetch-on-open, and the disabled tab stays out of tabIds so the existing Pitfall-7 effect handles mid-view worktree removal for free"
  - "Changed-lines collapse gate (>400) computed client-side as additions + deletions from the server numstat stats — Radix unmounts closed content so a collapsed huge file costs zero DOM"

patterns-established:
  - "Pattern: content-semantic color palette scoped to a single feature surface, verified by a grep that finds the palette classes nowhere outside the feature's components"
  - "Pattern: disabled tab = disabled TabsTrigger inside a span-wrapped Tooltip, zinc-600 label; non-disabled tabs render exactly as before"

requirements-completed: [REVW-01]

# Metrics
duration: 11 min
completed: 2026-06-11
---

# Phase 5 Plan 04: Read-Only Diff Tab Summary

**REVW-01 frontend: a read-only Diff tab (third, after Agent/Description, D-64) that renders plan 05-02's structured per-file-hunk JSON as collapsible unified diffs with a scoped green/red content palette, a totals bar with manual-refresh (no polling), >400-line collapse + binary header-only handling, and verbatim empty/loading/error states — disabled with an explanation when the task has no worktree.**

## Performance

- **Duration:** ~11 min
- **Started:** 2026-06-11T09:27Z
- **Completed:** 2026-06-11T09:38:20Z
- **Tasks:** 3
- **Files created/modified:** 6 (4 created, 2 modified)

## Accomplishments

- Vendored the shadcn `collapsible` block (Radix, unmounts closed content) and a fully-typed `useTaskDiff` query mirroring the 05-02 server JSON verbatim — fetch-on-activation, manual `refetch()`, never polls (D-61).
- `DiffFileSection`: collapsible per-file unified diff with the loud green/red change-line palette inside quiet neutral chrome; >400-changed-line files start collapsed (D-62); binary files render header-only "Binary file changed", not expandable; renamed files show `oldPath → path`; true minus U+2212 in all stats and del markers.
- `DiffTab`: sticky totals bar (`{N} file(s) changed`, zero `+0`/`−0` segments omitted, base in mono, spinning-while-refetching refresh button D-60) over a single vertical scroll container at full main-area width; all four data states verbatim — content, empty (D-63), 150ms-delayed loading (no flash), and error with the relayed git stderr + Reload.
- Diff is the permanent third tab (D-64): disabled with `The diff needs a worktree` when no worktree exists, auto-falling back to the Agent tab when the worktree disappears mid-view via the existing Pitfall-7 layout effect (no new effect).

## Task Commits

Each task was committed atomically:

1. **Task 1: shadcn collapsible block + diff API layer** - `b0c8d3e` (feat)
2. **Task 2: DiffFileSection + DiffTab renderers** - `cd70308` (feat)
3. **Task 3: TabDef disabled support + Diff tab insertion (D-64)** - `69ad387` (feat)

## Files Created/Modified

- `web/src/components/ui/collapsible.tsx` - Sanctioned shadcn collapsible block (Collapsible / CollapsibleTrigger / CollapsibleContent via the radix-ui umbrella).
- `web/src/api/diffs.ts` - `DiffResponse`/`DiffFile`/`DiffHunk`/`DiffLine` types (verbatim 05-02 contract) + `useTaskDiff(taskId)` (queryKey `["diff", taskId]`, fetch-on-mount, no `refetchInterval`).
- `web/src/components/task/DiffFileSection.tsx` - `{ file }` renderer: binary header-only branch; Collapsible with `defaultOpen={changedLines <= 400}`; full-row trigger (chevron rotates 90° open, path, status suffix, `+A`/`−B` stats); body maps hunks → header rows + add/del/context line rows with the scoped palette, per-file `overflow-x-auto`, selectable text, no per-line actions.
- `web/src/components/task/DiffTab.tsx` - `{ taskId }` consumer of `useTaskDiff`: sticky totals bar with the colored ± numbers and the ghost refresh button (`aria-label`/tooltip `Refresh diff`, `disabled`+`animate-spin` while `isFetching`), file list in server order, and the content/empty/loading/error states.
- `web/src/components/task/TaskTabs.tsx` - `TabDef` gains `disabled?` + `disabledTooltip?`; disabled triggers render a zinc-600 label inside a span-wrapped Tooltip; non-disabled tabs unchanged.
- `web/src/pages/TaskPage.tsx` - import `DiffTab`; Diff TabDef inserted at index 2 (after Description, before bash sessions), disabled without a worktree; `tabIds` memo includes `"diff"` only when `task?.worktree_path` is set (deps now include `task?.worktree_path`).

## Decisions Made

- **Zero new dependencies for the collapsible block.** This project's shadcn style is `radix-nova`, whose `collapsible` block imports from the `radix-ui` umbrella package — already a dependency at `^1.5.0`. So `git diff web/package.json` is empty rather than showing a new `@radix-ui/react-collapsible` entry. This satisfies the plan's intent (the sanctioned block is vendored, no unsanctioned deps) more strongly than the literal acceptance grep.
- **DiffTab not keepMounted, disabled tab out of tabIds.** Remount-on-activation is the D-61 fetch-on-open by construction, and keeping the disabled "diff" id out of `tabIds` lets the existing Pitfall-7 layout effect reactivate the Agent tab when the worktree is removed mid-view — no new fallback logic.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] collapsible block vendors via radix-ui umbrella, not @radix-ui/react-collapsible**
- **Found during:** Task 1 (shadcn add collapsible)
- **Issue:** Task 1's acceptance criterion expects `grep '@radix-ui/react-collapsible' web/package.json` to match and `git diff web/package.json` to show one new dependency. Under this project's `radix-nova` shadcn style, `npx shadcn add collapsible` emits a block importing `{ Collapsible as CollapsiblePrimitive } from "radix-ui"` — the umbrella package, already present at `^1.5.0`. No package.json change occurred and `@radix-ui/react-collapsible` is not a dependency.
- **Fix:** None needed — the sanctioned block is correctly vendored and exports `Collapsible`/`CollapsibleTrigger`/`CollapsibleContent`; zero new dependencies is the strictly-better outcome. Verified the build compiles against the umbrella import and the palette-scope/copy-string checks all pass.
- **Files modified:** web/src/components/ui/collapsible.tsx (created)
- **Verification:** `cd web && npm run build` exits 0; `grep 'radix-ui' web/package.json` shows the umbrella already at `^1.5.0`; `git diff web/package.json` empty.
- **Committed in:** b0c8d3e (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking — resolved as a no-op with documentation).
**Impact on plan:** None — the underlying intent (vendor the official collapsible block, add no unsanctioned dependencies) is fully met with zero new deps. No scope creep.

## Issues Encountered

- **Transient cross-agent build instability (parallel wave 2).** While building Task 1, `cd web && npm run build` twice surfaced TS6133 unused-symbol errors in `web/src/components/terminal/TerminalPane.tsx` and `web/src/components/task/AgentTab.tsx` — files owned by the parallel executor 05-03 (Resume/Reset exited-banner work), mid-edit. These were NOT caused by this plan's changes and are out of scope per the parallel-execution boundary. The build stabilized once 05-03's files reached a consistent state; the final full build (after all three of this plan's tasks) exits 0 cleanly. The orchestrator's post-wave validation will run the authoritative full build once both agents complete.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- REVW-01 is end-to-end: the 05-02 backend endpoint is now consumed by a live read-only Diff tab — the In Review column has a review surface. No stubs; `DiffTab` is wired directly to `GET /api/tasks/{id}/diff` via `useTaskDiff`.
- Ready for 05-05. The only remaining wave-2 sibling (05-03 Resume/Reset) shares the task view but owns disjoint files (AgentTab/TerminalPane); no merge conflict surface with this plan.

## Self-Check: PASSED

All 4 created files exist on disk; both modified files carry the changes; all 3 task commits (b0c8d3e, cd70308, 69ad387) present in git history.

---
*Phase: 05-recovery-review*
*Completed: 2026-06-11*
