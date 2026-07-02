---
phase: 23-worktree-cleanup-panel
plan: 04
subsystem: ui
tags: [react, tanstack-query, shadcn, typescript, worktree, cleanup]

# Dependency graph
requires:
  - phase: 23-worktree-cleanup-panel (plan 23-02, Wave 2)
    provides: the GET/POST /api/worktrees* backend endpoint contract these types encode
provides:
  - shadcn badge primitive (web/src/components/ui/badge.tsx) for classification + flag chips
  - useWorktreeList query hook (fetch-on-mount, no polling) + three mutation hooks (remove / clean-eligible / clear-pointer), each invalidating the ["worktrees"] key on settle
  - the full TypeScript response/request type set (WorktreeRow, ProjectGroup, WorktreeListResponse, RemoveArgs, RemoveResult, EligibleItem, CleanEligiblePreview, CleanEligibleResult, ClearPointerArgs, Classification, PRState)
affects: [Wave 3 worktree-cleanup UI — section/rows/dialogs implement against this fixed contract]

# Tech tracking
tech-stack:
  added: [shadcn badge primitive (copied-in, no npm dependency)]
  patterns:
    - "Interface-first frontend contract: types + hooks defined before the UI so Wave 3 implements against a fixed contract"
    - "Diff-tab data model reused: fetch-on-mount + manual-Refresh, no polling (D-10/D-61)"
    - "One query key, one invalidation path: every mutation invalidates ['worktrees'] onSettled (Pitfall 4)"

key-files:
  created:
    - web/src/components/ui/badge.tsx
    - web/src/api/worktreeCleanup.ts
  modified: []

key-decisions:
  - "204 remove response (client.ts → undefined) mapped inside mutationFn to { outcome: 'removed' }; a 200 { outcome: 'blocked' } is a normal outcome, not an error"
  - "useCleanEligible takes a single { dryRun } arg and branches the endpoint (?dry_run=1 preview vs applied); both paths invalidate ['worktrees'] to keep one code path"

patterns-established:
  - "Interface-first: contract (types + hooks) lands a wave before the consuming UI"
  - "No-poll admin data layer: useQuery with no refetchInterval; manual Refresh calls refetch()"

requirements-completed: [WTREE-01, WTREE-02, WTREE-03, WTREE-04]

# Metrics
duration: ~12min
completed: 2026-07-02
---

# Phase 23 Plan 04: Worktree-Cleanup Frontend Contract Summary

**shadcn `badge` primitive + the TanStack Query data layer (fetch-on-mount no-poll `useWorktreeList` + three settle-invalidating mutation hooks) and the full TypeScript type set encoding the Wave 2 worktree-cleanup endpoint contract**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-07-02
- **Completed:** 2026-07-02
- **Tasks:** 2
- **Files created:** 2

## Accomplishments
- Installed the official shadcn `badge` primitive (`Badge` + `badgeVariants` with `default`/`secondary`/`outline`/`destructive`/`ghost`/`link`) — the panel's classification chips use `secondary`/`outline` (muted metadata) and `destructive` (the `Blocked` chip), no hand-rolled `<span>`.
- Created `web/src/api/worktreeCleanup.ts`: `useWorktreeList` (GET `/api/worktrees`, queryKey `["worktrees"]`, **no polling**) plus `useRemoveWorktree`, `useCleanEligible`, `useClearPointer` — every mutation invalidates `["worktrees"]` `onSettled`.
- Encoded the complete Wave 2 backend contract in TypeScript: classification (`referenced`/`orphan`/`stale`), flags (`dirty`/`unpushed` nullable/`stash`), the `blocked` D-01 outcome, and the bulk preview/result shapes — Wave 3 implements against fixed types with no guesswork.

## Task Commits

Each task was committed atomically:

1. **Task 1: Add the shadcn badge primitive** - `e6f0527` (feat)
2. **Task 2: Add worktreeCleanup.ts hooks + types** - `90f35c9` (feat)

## Files Created/Modified
- `web/src/components/ui/badge.tsx` - shadcn `Badge` + `badgeVariants` chip primitive (official registry install).
- `web/src/api/worktreeCleanup.ts` - `useWorktreeList` + three mutation hooks and the full response/request type set for the cleanup panel.

## Decisions Made
- **204 → `{ outcome: "removed" }` mapping:** because `client.ts` resolves a 204 to `undefined`, `useRemoveWorktree`'s `mutationFn` awaits `post<RemoveResult | undefined>()` and returns `r ?? { outcome: "removed" }`. A 200 `{ outcome: "blocked", path }` is surfaced as a normal result (the D-01 permission-blocked case), never thrown as an error.
- **`useCleanEligible({ dryRun })`:** one hook, one variable arg — `dryRun` branches the endpoint (`?dry_run=1` preview returning `CleanEligiblePreview` vs the applied call returning `CleanEligibleResult`); both paths invalidate `["worktrees"]` (harmless for the dry-run, keeps one code path per the plan).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Reworded two doc comments to satisfy the literal `grep -c 'refetchInterval'` verification**
- **Found during:** Task 2 (worktreeCleanup.ts verification)
- **Issue:** The plan's `<done>`/`<verification>` require `grep -c 'refetchInterval' worktreeCleanup.ts` to return `0`. My explanatory comments said "NO refetchInterval" / "has NO refetchInterval", so the literal token appeared twice (count = 2) even though the code sets no polling interval — a false verification failure.
- **Fix:** Reworded the two comments to "sets no polling interval" / "no polling interval (D-10 no-poll)", preserving the exact meaning while removing the literal token. No code/behavior change.
- **Files modified:** web/src/api/worktreeCleanup.ts
- **Verification:** `grep -c 'refetchInterval'` → 0; `invalidateQueries` → 3; `tsc -b` + `vite build` green.
- **Committed in:** `90f35c9` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking).
**Impact on plan:** The one change is comment wording to pass the literal grep verification; zero behavior change and no scope creep.

## Issues Encountered
- **Missing `node_modules` in the fresh worktree.** The worktree checkout excludes gitignored dependencies, so `tsc`/`vite` binaries were absent and `npm run build` (the plan's `<done>` gate) could not run. Restored the pinned dependencies deterministically with `npm ci` from the committed `package-lock.json` — no new package names were introduced, so the package-install safety exclusion does not apply. `node_modules` is gitignored and not committed.
- **`npm run build` rewrites `web/dist/index.html`.** That file is a deliberately-committed `//go:embed` placeholder (`.gitignore`: `web/dist/*` ignored except `!web/dist/index.html`). After each build I reverted it with `git checkout -- web/dist/index.html` so only the two source files were committed; the `dist/` build output stays uncommitted (gitignored).

## Known Stubs
None — both files are complete: the badge is the full shadcn primitive, and every hook/type is fully specified against the Wave 2 contract. (This is an interface/contract plan; the consuming UI arrives in Wave 3 as planned — not a stub.)

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- The Wave 3 worktree-cleanup UI (Settings section, per-project groups, worktree rows, force-remove / clean-eligible / clear-pointer dialogs) can now be built against a fixed contract: `web/src/components/ui/badge.tsx` for chips and `web/src/api/worktreeCleanup.ts` for all data + mutations.
- The backend endpoints these types describe are the responsibility of Wave 2 plan 23-02; this plan only declares the client-side contract (all destructive logic + gate re-checks live server-side).

## Self-Check: PASSED

- FOUND: web/src/components/ui/badge.tsx
- FOUND: web/src/api/worktreeCleanup.ts
- FOUND commit: e6f0527 (Task 1)
- FOUND commit: 90f35c9 (Task 2)

---
*Phase: 23-worktree-cleanup-panel*
*Completed: 2026-07-02*
