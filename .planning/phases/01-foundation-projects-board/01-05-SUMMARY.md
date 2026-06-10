---
phase: 01-foundation-projects-board
plan: 05
subsystem: ui
tags: [react, dnd-kit, tanstack-query, kanban, shadcn]

# Dependency graph
requires:
  - phase: 01-foundation-projects-board (plan 01-02)
    provides: move endpoint with server-computed fractional positions ({status, after_id} intent API)
  - phase: 01-foundation-projects-board (plan 01-03)
    provides: typed API layer (useTasks, useCreateTask, optimistic useMoveTask), router shell, zinc theme, shadcn components
provides:
  - Interactive kanban board at /projects/:projectId with four fixed identical columns and task counts
  - dnd-kit drag-and-drop (within-column reorder + cross-column moves) persisting through useMoveTask with optimistic order
  - Click-to-open cards (5px activation distance separates click from drag)
  - Two task-creation paths — To Do quick-add row and New Task dialog — plus the plain-n shortcut
affects: [01-06 task view (cards navigate to it), 01-07 embed/build]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Board column derivation: local Record<Status, Task[]> mirror derived solely from useTasks data, frozen while a drag is active or a move mutation is pending (Pitfall 6)"
    - "Namespaced droppable ids (column:<status>) so empty columns accept drops without id collisions"
    - "handleDragOver mutates LOCAL state only; the single network call lives in handleDragEnd"

key-files:
  created:
    - web/src/components/board/Board.tsx
    - web/src/components/board/Column.tsx
    - web/src/components/board/TaskCard.tsx
    - web/src/components/board/QuickAdd.tsx
    - web/src/components/board/NewTaskDialog.tsx
  modified:
    - web/src/pages/BoardPage.tsx

key-decisions:
  - "Derivation guard extended with moveTask.isPending so the post-drop frame never snaps back to pre-optimistic query data"
  - "Dialog width set via sm:max-w-[560px] because shadcn DialogContent ships sm:max-w-sm which beats a bare max-w override at >=sm"
  - "QuickAdd rendered inside Column conditional on status === 'todo'; generic topSlot prop kept as the extension seam"

patterns-established:
  - "Board feature components consume only the 01-03 API layer (types/queries/mutations) — no direct fetch"
  - "Same-position drops skip the mutation entirely; drops outside any droppable snap back to query truth"

requirements-completed: [TASK-01, TASK-02, TASK-03]

# Metrics
duration: 8min
completed: 2026-06-10
---

# Phase 01 Plan 05: Kanban Board Summary

**dnd-kit kanban board with four fixed columns, optimistic drag persistence through the move endpoint, click-to-open cards, and dual task creation (quick-add row + 560px dialog with n shortcut)**

## Performance

- **Duration:** 8 min
- **Started:** 2026-06-10T06:57:01Z
- **Completed:** 2026-06-10T07:04:50Z
- **Tasks:** 3
- **Files modified:** 6 (5 created, 1 rewritten)

## Accomplishments

- Fully interactive board at `/projects/:projectId`: four visually identical columns (no per-column colors, D-03) with uppercase muted headers and counts, horizontal board scroll, independent column scroll
- Drag-and-drop with all three pitfall mitigations verifiably present: 5px `activationConstraint` (clicks navigate, Pitfall 4), namespaced `column:` droppable ids (empty columns accept drops, Pitfall 5), frozen derivation during active drag/pending move (Pitfall 6)
- Drops persist via `useMoveTask` — `handleDragEnd` computes `{status, afterId}` intent (afterId null at index 0, never the moving task), same-position drops skip the network entirely, error rollback comes free from the 01-03 mutation snapshot
- Both D-08 creation paths land at top of To Do via the server position rule (D-09): inline quick-add (Enter commits and stays open, Esc cancels with stopPropagation, empty blur collapses) and the New Task dialog (title + raw markdown description, submit disabled while empty/pending)
- Plain-`n` shortcut opens the dialog, guarded against inputs/textareas/contenteditable and open dialogs

## Task Commits

Each task was committed atomically:

1. **Task 1: Board, Column, TaskCard — dnd-kit structure and board visuals** - `54464aa` (feat)
2. **Task 2: Drag persistence — onDragOver/onDragEnd + optimistic move** - `e9847e8` (feat)
3. **Task 3: Task creation — quick-add row, New task dialog, "n" shortcut** - `cf9b6cc` (feat)
4. **Follow-up: derivation-source comment** - `fe9c0a8` (docs)

## Files Created/Modified

- `web/src/components/board/Board.tsx` - DndContext with sensors/closestCorners/DragOverlay, local column mirror with frozen derivation, handleDragOver (local splice) + handleDragEnd (afterId + mutate)
- `web/src/components/board/Column.tsx` - `useDroppable(column:<status>)` well + SortableContext, identical styling across statuses, accent ring on drag-over, QuickAdd in To Do
- `web/src/components/board/TaskCard.tsx` - useSortable card with line-clamp-2 title, click navigation to the task route, 40% opacity origin slot; separate TaskCardOverlay for DragOverlay lift feedback
- `web/src/components/board/QuickAdd.tsx` - collapsed `+ New task` row → inline input with Enter/Esc/blur semantics
- `web/src/components/board/NewTaskDialog.tsx` - 560px dialog with title input + markdown textarea, Create task/Cancel
- `web/src/pages/BoardPage.tsx` - header (project name + inverted New task focal button), skeleton loading, retry error state, n-shortcut listener, dialog wiring

## Decisions Made

- **isPending added to the derivation guard:** plan specified freezing derivation only during active drag; between clearing `activeTask` and the optimistic cache write landing there is a one-frame window where stale query data would snap the column order back. Guarding on `moveTask.isPending` as well closes it without any manual rollback code in Board.tsx.
- **`sm:max-w-[560px]` instead of bare `max-w-[560px]`:** shadcn's DialogContent carries `sm:max-w-sm`, which wins over a bare `max-w-*` at the sm breakpoint via tailwind-merge specificity; the responsive variant honors the UI-SPEC 560px width where it matters.
- **QuickAdd lives inside Column** (conditional on `status === "todo"`) per Task 3 acceptance criteria; the generic `topSlot` prop from Task 1 is kept as the documented extension seam.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Post-drop snap-back flicker window closed**
- **Found during:** Task 2 (drag persistence)
- **Issue:** Unfreezing derivation immediately on drop re-derives columns from pre-optimistic query data for one render before `onMutate`'s cache write lands — visible order flicker
- **Fix:** Derivation effect additionally guarded on `moveTask.isPending`; columns still derive only from query data when idle
- **Files modified:** web/src/components/board/Board.tsx
- **Verification:** build green; single derivation path preserved (no manual rollback in Board.tsx)
- **Committed in:** e9847e8 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Strengthens the plan's own Pitfall 6 mitigation; no scope creep.

## Issues Encountered

- A parallel executor (01-06) committed between my Task 3 commit and a planned amend, so the Board.tsx derivation-source comment landed as a separate `docs` commit (`fe9c0a8`) instead of being folded into `cf9b6cc`. No functional impact.
- Vite reports the main JS chunk >500 kB after minification (dependency weight: dnd-kit, react-query, radix). Informational warning only — irrelevant for a localhost single-binary tool; revisit only if 01-07 cares about embed size.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- TASK-01/02/03 complete client-side against the 01-02 API; cards navigate to `/projects/:pid/tasks/:tid`, which plan 01-06 fills in
- Full end-to-end drag/reload verification (server persistence across refresh) is exercisable once 01-07 wires the single-binary serve path; dev-mode verification works today via Vite proxy + Go server

---
*Phase: 01-foundation-projects-board*
*Completed: 2026-06-10*

## Self-Check: PASSED

- All 6 key files verified on disk
- Commits 54464aa, e9847e8, cf9b6cc, fe9c0a8 verified in git log
- `cd web && npm run build` exit 0; all plan greps pass
