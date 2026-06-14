---
phase: 11-pr-review-column
plan: 04
subsystem: ui
tags: [react, typescript, tanstack-query, localStorage, kanban, github, presentational-column]

# Dependency graph
requires:
  - phase: 11-pr-review-column (Plan 03)
    provides: "usePullRequests/useRefreshPullRequests hooks + PRCard presentational card + formatAgo (web/src/lib/time.ts) this column composes"
  - phase: 11-pr-review-column (Plan 02)
    provides: "always-200 GET /api/projects/{id}/pull-requests {state,stale,fetchedAt,prs} the hooks consume"
  - phase: 10-github-foundations
    provides: "github_integration settings KV + projects.github_repo link; useSettings()/useProjects() render-gate inputs (OFF cascade + link gate)"
  - phase: 01 (board)
    provides: "Board.tsx flex row + Column.tsx shell the Review column echoes and is appended into (outside dnd)"
provides:
  - "web/src/components/board/ReviewColumn.tsx — self-gating collapsible Review column: render gate (integration on AND project linked), collapse rail + count badge, header (collapse chevron + REVIEW label + count + refresh), inline loading/empty/degraded/stale state branches, per-project localStorage collapse persistence (default collapsed)"
  - "web/src/components/board/Board.tsx — <ReviewColumn projectId> appended after STATUSES.map inside the flex row, OUTSIDE all dnd droppables (D-13)"
  - "default-collapsed UX (supersedes D-05/D-06 'default expanded') verified live"
affects:
  - "12-open-a-review (the Review column's PRCard body becomes interactive; opening a PR creates a worktree-backed review workspace)"
  - "13-pr-worktree-cleanup (the column is the surface that self-empties as PRs merge/close)"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Self-gating presentational column: the component renders null unless useSettings().github_integration==='on' AND project.github_repo is set, so Board.tsx's props stay untouched and the OFF/unlinked board is byte-for-byte pre-v1.3 (mirrors QuotaIndicator's self-gate)"
    - "Hooks-after-gate split: the gate lives in an outer component (useSettings/useProjects only), the data hooks (usePullRequests/useRefreshPullRequests + useState) live in an inner component mounted only when the gate is open — keeps rules-of-hooks valid while a gated-off project never polls gh"
    - "Per-project localStorage UI pref (kangent:review-collapsed:{projectId}) — no SQLite, no migration, no endpoint (D-05)"
    - "Sibling-of-the-status-columns placement: appended after STATUSES.map in the same flex row but OUTSIDE DndContext's droppables — a column that is visually parallel to the kanban but never participates in drag (D-13)"
    - "Inline degrade-don't-break state machine on the column body (loading skeletons / empty / amber degraded note / stale footer), never a modal, never blocking the board (quota precedent)"

key-files:
  created:
    - web/src/components/board/ReviewColumn.tsx
  modified:
    - web/src/components/board/Board.tsx
    - web/dist/index.html
    - .planning/phases/11-pr-review-column/11-04-PLAN.md
    - .planning/phases/11-pr-review-column/11-UI-SPEC.md
    - .planning/phases/11-pr-review-column/11-CONTEXT.md

key-decisions:
  - "Default collapsed (supersedes D-05/D-06 'default expanded') per user request 2026-06-14: the board opens with the slim Review rail; the column only stays expanded once the user explicitly expands it (localStorage stores '0'; absent key reads as collapsed via getItem !== '0')"
  - "Self-gate inside ReviewColumn (RESEARCH Pattern 4 rec. b), NOT new Board props: Board passes only projectId; the column reads settings/project itself and returns null when gated — the OFF row is byte-for-byte unchanged (GHSET-02)"
  - "Outer-gate / inner-hooks split to respect rules-of-hooks while keeping the early null-return: the data hooks never run for a gated-off project"
  - "Count shown only when state==='ok' (null while loading/degraded) so the header and collapsed badge never display a fabricated 0 (D-07)"
  - "Stale-with-cache still renders the cached prs above the amber stale footer — the column never goes blank just because a refresh failed (quota precedent, PITFALL handling)"

patterns-established:
  - "Self-gating column pattern: a board column that gates itself to null on a shared query value, so the parent layout stays prop-stable and the OFF state is a true byte-for-byte revert"
  - "Default-collapsed per-project localStorage toggle keyed kangent:<feature>:{projectId}"

requirements-completed: [GHCOL-01, GHCOL-04, GHCOL-05, GHCOL-06]

# Metrics
duration: ~10 min active (602 min wall-clock incl. overnight human-verify gate + a default-collapsed design-change round)
completed: 2026-06-14
---

# Phase 11 Plan 04: PR Review Column Summary

**A self-gating, collapsible "Review" column rendered to the right of Done on a linked project's board — it polls `usePullRequests`, lists `PRCard`s in server order with inline loading/empty/degraded/stale states and a manual refresh, persists its collapse state per-project in localStorage (default collapsed), and is appended into the board flex row entirely OUTSIDE the dnd machinery so PR cards never enter the kanban.**

## Performance

- **Duration:** ~10 min active execution; 602 min wall-clock (the human-verify checkpoint spanned overnight, plus a mid-checkpoint design-change round to flip the default to collapsed)
- **Started:** 2026-06-13T18:20:52Z
- **Completed:** 2026-06-14T04:23:34Z
- **Tasks:** 3 (2 `type="auto"` + 1 `checkpoint:human-verify`, approved)
- **Files modified:** 1 created (ReviewColumn.tsx), 1 edited (Board.tsx), 1 rebuilt (web/dist/index.html), 3 docs updated (PLAN/UI-SPEC/CONTEXT)

## Accomplishments
- `ReviewColumn.tsx` (created): a self-gating collapsible column. The render gate returns `null` unless `useSettings().github_integration === 'on'` AND the project has a non-empty `github_repo`, so a gated-off or unlinked board is byte-for-byte pre-v1.3 (GHSET-02). The gate lives in an outer component (settings/project reads only); the data hooks + collapse state live in an inner component mounted only when the gate is open, so a gated-off project never polls `gh` (D-11) and rules-of-hooks stays valid.
- Expanded, it echoes `Column.tsx` exactly (`flex min-h-0 min-w-[260px] flex-1 flex-col` + the `bg-[#101013] p-3` list track, minus the droppable ring): a header with the collapse chevron, the `REVIEW` label, the count, and a right-aligned refresh button, over a card list that renders the inline state branch.
- Collapsed, it narrows to a `w-10` slim rail (clickable to expand) with an expand chevron, a vertical `REVIEW` label, and a neutral count badge.
- Inline state branches (GHCOL-05), all inside the list container, never a modal, never blocking the board: loading (1–2 `Skeleton`s), degraded (`no_gh`/`auth_required`/`error` → amber `TriangleAlert` + the canonical copy, refresh stays active), empty (`You're all caught up` / `No PRs are waiting for your review.`), list (`PRCard`s in server order, D-14, no re-sort, no local retention), and the amber stale footer (`error · {age} old`) rendered below cached prs when `stale`.
- Collapse state persisted per-project in `localStorage` (`kangent:review-collapsed:{projectId}`), now **default collapsed** (supersedes D-05/D-06 per user request) — collapsed unless the user has explicitly expanded it (stored `"0"`).
- `Board.tsx` (edited): `<ReviewColumn projectId={projectId} />` appended after `STATUSES.map` inside the existing flex row, OUTSIDE `DndContext`'s droppables — no `useDroppable`/`SortableContext`/`useSortable`. `Board`'s props are unchanged (`{tasks, projectId}`).
- Verified end-to-end by the user (all seven checks: column appears / lists / collapses-and-persists with default-collapsed / refreshes-and-self-empties / empty / degrades non-blocking / OFF cascade) on a real linked project.

## Task Commits

Each task was committed atomically:

1. **Task 1: self-gating collapsible ReviewColumn** - `1b9b542` (feat)
2. **Task 2: append ReviewColumn to Board.tsx (sibling, outside dnd)** - `d3ed2d9` (feat)
3. **Task 3: human-verify checkpoint** - no code commit (verification only); APPROVED 2026-06-14
4. **Mid-checkpoint design change: default Review column to collapsed (supersedes D-05)** - `50a564c` (feat — code + PLAN/UI-SPEC/CONTEXT doc updates)
5. **Embedded SPA rebuild** - `<see Files commit>` (build: web/dist/index.html points at the ReviewColumn-bearing asset)

**Plan metadata:** _(final docs commit — SUMMARY + STATE + ROADMAP + REQUIREMENTS)_

## Files Created/Modified
- `web/src/components/board/ReviewColumn.tsx` (created) — self-gating collapsible Review column with render gate, collapse rail + badge, header (collapse/REVIEW/count/refresh), inline state branches, per-project localStorage collapse (default collapsed).
- `web/src/components/board/Board.tsx` (modified) — appended `<ReviewColumn projectId={projectId} />` after `STATUSES.map`, outside dnd.
- `web/dist/index.html` (rebuilt) — embedded SPA placeholder now references the asset bundle containing the Review column (repo convention: `web/dist/*` gitignored except this `index.html` placeholder; hashed assets not committed).
- `.planning/phases/11-pr-review-column/11-04-PLAN.md` (modified) — default-collapsed wording + Task 3 marked complete/approved.
- `.planning/phases/11-pr-review-column/11-UI-SPEC.md` (modified) — GHCOL-06 collapse default changed to collapsed with a supersede note.
- `.planning/phases/11-pr-review-column/11-CONTEXT.md` (modified) — D-05/D-06 updated to default collapsed with a supersede note.

## Decisions Made
- **Default collapsed, superseding D-05/D-06 "default expanded"** (user request 2026-06-14): the board opens with the slim rail and the column only stays expanded once the user explicitly expands it. Implemented as `localStorage.getItem(storageKey) !== "0"` (absent => collapsed). PLAN/UI-SPEC/CONTEXT all updated with grep-verifiable assertions + supersede notes.
- **Self-gate inside the column, not new Board props** (RESEARCH Pattern 4 rec. b): keeps `Board` prop-stable and makes the OFF state a true byte-for-byte revert.
- **Outer-gate / inner-hooks split**: lets the early `return null` coexist with the data hooks without violating rules-of-hooks, and guarantees a gated-off project never polls `gh`.
- **Count only when `state==='ok'`** (null otherwise): the header/badge never shows a fabricated `0` during loading/degraded (D-07).

## Deviations from Plan

None affecting code behavior — the plan was executed as written, then amended by an explicit user design change during the checkpoint.

### Mid-checkpoint design change (user-directed, not an auto-deviation)

**1. Default Review column to collapsed (supersedes locked decision D-05/D-06)**
- **Found during:** Task 3 (human-verify checkpoint) — user feedback before approval.
- **Change:** Flipped the initial collapse default from expanded to collapsed (`getItem === "1"` → `getItem !== "0"`); updated the `// default expanded` comment to `// default collapsed`; propagated grep-verifiable "default collapsed" wording + supersede notes to 11-04-PLAN.md, 11-UI-SPEC.md (GHCOL-06), and 11-CONTEXT.md (D-05/D-06).
- **Files modified:** web/src/components/board/ReviewColumn.tsx + the three phase docs.
- **Verification:** `tsc --noEmit -p tsconfig.app.json` + `npm run build` green; user approved all seven checks (incl. the new default).
- **Committed in:** `50a564c`.

---

**Total deviations:** 0 auto-fixed; 1 user-directed design change (documented above).
**Impact on plan:** No scope creep — a single targeted UX default flip plus doc alignment. The component's structure and all other behavior are exactly as planned.

## Issues Encountered
- The Task 1 acceptance grep for "NOT a droppable" (`useDroppable|SortableContext|useSortable` must match NOTHING) initially fired on a doc comment that listed those tokens to describe what the column deliberately avoids. The component has zero dnd-kit imports/usage; reworded the comment so the literal grep reads only real markup (same prose-vs-grep artifact noted in 11-03). No code defect.
- The verification `npm run build` regenerates `web/dist/index.html` with new asset hashes. Per repo convention (`web/dist/*` gitignored except the `index.html` placeholder), the per-task verification builds were reverted to keep the placeholder stable; the embedded SPA was rebuilt and committed once at plan completion.

## User Setup Required
None - no external service configuration required. (`gh` is the host's already-authenticated soft dependency; the column degrades inline when it is missing/unauthenticated.)

## Next Phase Readiness
- Phase 11 (PR Review Column) is functionally complete: the always-200 endpoint (P02), the data hooks + PRCard (P03), and the user-facing Review column (this plan) are all wired and verified. GHCOL-01/02/03/04/05/06 all met.
- **Phase 12 (open-a-review)** can now make the `PRCard` body interactive (currently inert, D-08): clicking a card opens a worktree-backed review workspace using the rich PRSummary fields (`headRefName`/`headRefOid`/`baseRefName`/`isCrossRepository`) already fetched but unrendered here.
- No blockers. The OFF cascade + link gate are enforced both backend (P02 GATE 1/2) and frontend (this column's render gate).

---
*Phase: 11-pr-review-column*
*Completed: 2026-06-14*

## Self-Check: PASSED

- All created/modified files present (web/src/components/board/ReviewColumn.tsx, web/src/components/board/Board.tsx, 11-04-SUMMARY.md).
- All task commits present in git history (1b9b542 Task 1, d3ed2d9 Task 2, 50a564c default-collapsed design change).
- `tsc --noEmit -p tsconfig.app.json` + full `npm run build` green after every change.
- Task 3 (human-verify checkpoint) approved by the user 2026-06-14 — all seven checks pass.
