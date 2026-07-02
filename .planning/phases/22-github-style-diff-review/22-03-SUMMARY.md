---
phase: 22-github-style-diff-review
plan: 03
subsystem: ui
tags: [react, tanstack-query, radix-ui, shadcn, checkbox, diff, optimistic-update]

# Dependency graph
requires:
  - phase: 22-github-style-diff-review (plan 02)
    provides: "GET /api/tasks/{id}/diff now returns per-file hash+viewed; PUT /api/tasks/{id}/diff/viewed { path, hash, viewed } → 204"
provides:
  - "DiffFile type carries hash + viewed"
  - "useToggleViewed(taskId) optimistic mutation on [\"diff\", taskId]"
  - "shadcn checkbox primitive (web/src/components/ui/checkbox.tsx)"
  - "DiffFileSection with sibling blue-500 Viewed checkbox, controlled collapse, sticky header, data-diff-path, dim-when-viewed"
  - "DiffTab keys sections on path:hash so a changed file remounts un-viewed+expanded (DIFF-04)"
affects: [22-04 (two-pane layout + file tree + scroll-spy reuses data-diff-path and the Viewed control)]

# Tech tracking
tech-stack:
  added: []  # radix-ui@1.5.0 already installed exports Checkbox — no new dependency
  patterns:
    - "Optimistic toggle mutation mirroring useMoveTask (cancel → snapshot → setQueryData → onError rollback → onSettled invalidate)"
    - "Sibling-not-child focusable control inside a collapse trigger (flex-1 trigger + sibling label, e.stopPropagation()) — TaskTabs × close precedent"
    - "Call-site Tailwind override via twMerge (data-[state=checked]:bg-blue-500 wins over primitive bg-primary)"
    - "Controlled Collapsible so manual collapse stays orthogonal to Viewed"
    - "Section identity via key=path:hash to force remount on content change"

key-files:
  created:
    - web/src/components/ui/checkbox.tsx
  modified:
    - web/src/api/diffs.ts
    - web/src/components/task/DiffFileSection.tsx
    - web/src/components/task/DiffTab.tsx

key-decisions:
  - "Hand-authored the checkbox primitive mirroring switch.tsx's unified radix-ui import (deterministic, no registry network fetch) — an explicitly acceptable path per plan notes + threat T-22-SC"
  - "Blue-500 checked fill applied at the call site (not in the primitive) via twMerge override, keeping the primitive shadcn-default"
  - "data-diff-path + scroll-mt-[41px] added to BOTH the binary and non-binary roots for Plan 04 scroll-spy forward-compat (binary needs it too)"

patterns-established:
  - "Optimistic per-file cache patch keyed by path on the [\"diff\", taskId] query"
  - "Sibling checkbox + label inside a sticky flex header row; trigger takes flex-1 min-w-0"

requirements-completed: [DIFF-02, DIFF-03, DIFF-04]

# Metrics
duration: ~20min
completed: 2026-07-02
---

# Phase 22 Plan 03: Client Viewed Control Summary

**Blue-500 "Viewed" checkbox wired to an optimistic useToggleViewed mutation, sitting as a sibling of the collapse trigger in a sticky DiffFileSection header — checking collapses+dims, unchecking re-expands, collapse stays independent, and a new content hash remounts the section un-viewed+expanded.**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-07-02T04:56Z (approx)
- **Completed:** 2026-07-02T05:16Z
- **Tasks:** 2
- **Files modified:** 3 modified + 1 created

## Accomplishments
- Extended `DiffFile` with `hash: string` and `viewed: boolean` (plan 22-02 backend contract).
- Added `useToggleViewed(taskId)` — an optimistic TanStack mutation on `["diff", taskId]` that flips `viewed` on the matching path, rolls back on error, and reconciles via `onSettled` invalidate (mirrors `useMoveTask`).
- Added the shadcn `checkbox` primitive via the unified `radix-ui` import (no new npm dependency; `radix-ui@1.5.0` already exports `Checkbox`).
- Restructured `DiffFileSection` so the header is a flex row: `CollapsibleTrigger` shrinks to `flex-1 min-w-0`; a `<label>`-wrapped Viewed checkbox is a SIBLING (never a nested `<button>`), `e.stopPropagation()` on click.
- Controlled the Collapsible (`open`/`onOpenChange`): check → `mutate(viewed:true)` + `setOpen(false)`; uncheck → `mutate(viewed:false)` + `setOpen(true)`; manual collapse stays independent of Viewed.
- Dim-when-viewed (path + ± stats → `muted-foreground`); blue-500 checked fill + blue focus ring; sticky header at `top-[41px]`; `data-diff-path` + `scroll-mt-[41px]` for Plan 04 scroll-spy.
- Binary files get the same checkbox (collapse-on-view is a harmless no-op) and dim their header when viewed.
- `DiffTab` passes `taskId` and keys each section on `` `${file.path}:${file.hash}` `` so a changed file remounts un-viewed+expanded (DIFF-04).

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend diff types, add useToggleViewed, add checkbox primitive** — `872bc5f` (feat)
2. **Task 2: Restructure DiffFileSection — sibling Viewed checkbox, controlled collapse, sticky header, dim-when-viewed** — `ade87b2` (feat)

_(STATE.md / ROADMAP.md intentionally not modified — orchestrator owns those after the wave merges.)_

## Files Created/Modified
- `web/src/api/diffs.ts` — `DiffFile` gains `hash`+`viewed`; new `useToggleViewed(taskId)` optimistic mutation against `PUT /api/tasks/{id}/diff/viewed`.
- `web/src/components/ui/checkbox.tsx` — new shadcn checkbox primitive (unified `radix-ui` `Checkbox.Root`/`Indicator`, lucide `Check`, `cn()`); default `primary` fill left intact (blue override is a call-site class).
- `web/src/components/task/DiffFileSection.tsx` — sibling Viewed checkbox, controlled collapse, sticky header, dim-when-viewed, `data-diff-path`, blue-500 override; binary branch included.
- `web/src/components/task/DiffTab.tsx` — minimal edit: pass `taskId`, key sections on `path:hash`.

## Decisions Made
- **Hand-authored checkbox primitive** instead of `npx shadcn add checkbox`: deterministic and network-free, mirrors the existing `switch.tsx` unified-import convention exactly. The plan explicitly allows either path ("no third-party registry"); threat T-22-SC is fully avoided (no registry fetch, no new dependency).
- **Blue-500 at the call site, not in the primitive**: keeps the primitive a stock shadcn checkbox; `cn()`/`twMerge` guarantees `data-[state=checked]:bg-blue-500` (+border/text) wins over the primitive's `bg-primary`.
- **`data-diff-path` + `scroll-mt-[41px]` on the binary root too**: Plan 04's file-tree scroll-spy must locate every changed file, binaries included, so both header variants expose the anchor attribute now (harmless, forward-compatible).

## Deviations from Plan
None — plan executed exactly as written. No Rule 1–4 deviations were required; both tasks matched the plan's actions and acceptance criteria.

## Issues Encountered
- **Frontend dependencies were not installed in the worktree** — `npm run build`/`lint` reported `tsc: not found` / `eslint: not found`. Resolved with `npm ci` (633 packages, 0 vulnerabilities) before verifying. This is environment setup, not a code change (no `package.json`/lockfile modification).
- **`web/dist/index.html` is a tracked `go:embed` placeholder** that `vite build` overwrites. Restored it with `git checkout -- web/dist/index.html` after each build so no build artifact leaked into the task commits.

## Verification
- `cd web && npm run build` (tsc -b + vite build): **PASS** (2221 modules, clean type-check; only the standard >500 kB chunk-size advisory).
- Lint on the touched files (`npx eslint src/api/diffs.ts src/components/ui/checkbox.tsx src/components/task/DiffFileSection.tsx src/components/task/DiffTab.tsx`): the three source files I authored/restructured are **clean**. The single reported error is pre-existing — `DiffTab.tsx:34:7` `react-hooks/set-state-in-effect` on the untouched loading-delay `useEffect` (I only edited the render loop near line 126).
- Repo-wide `npm run lint` has **20 pre-existing errors** across untouched files (`Board.tsx`, `PRCard.tsx`, `ReviewColumn.tsx`, `TerminalPane.tsx`, `useTerminalSocket.ts`, `use-mobile.ts`, `TaskPage.tsx`, `SettingsField.tsx`, `CleanupWorktreeDialog.tsx`, `RenameProjectDialog.tsx`, plus config warnings on `button.tsx`/`sidebar.tsx`/`tabs.tsx`/`StatusDot.tsx`) — all the same `set-state-in-effect` rule. These are out of scope per the executor scope boundary and were logged, not fixed.
- Acceptance greps all pass: `stopPropagation`≥1, `data-[state=checked]:bg-blue-500`≥1, `data-diff-path`≥2, no `defaultOpen`, `onOpenChange`≥1, aria `as viewed`/`as not viewed` present, `Viewed` label present, `−` (U+2212) + `tabular-nums` retained, `key=path:hash` + `taskId={taskId}` in DiffTab.
  - Note: `grep -c useToggleViewed DiffFileSection.tsx` returns 2 (import line + single call site) rather than the plan's literal `==1`; the substantive intent — the hook is invoked exactly once — is met, and the import must reference the name.

## Known Stubs
None — the checkbox is wired end-to-end to the real `useToggleViewed` mutation → the real Plan 02 `PUT .../diff/viewed` endpoint; no hardcoded/empty/placeholder data.

## Next Phase Readiness
- The Viewed control and `data-diff-path` anchors are in place; **Plan 04 (wave 4)** can now do the two-pane restructure (fixed `w-72` file tree + scroll-spy selection) reusing `data-diff-path` and this Viewed control without touching the toggle wiring.
- Binary strict-D-01 limitation carries through: a binary file marked Viewed will not auto-reset on a byte change (accepted, Plan 01 notes).
- No blockers.

---
*Phase: 22-github-style-diff-review*
*Completed: 2026-07-02*
