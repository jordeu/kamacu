---
phase: 12-open-a-review
plan: 05
subsystem: ui
tags: [react, github-pr, review-view, tanstack-query, xterm-paste, open-reattach]

# Dependency graph
requires:
  - phase: 12-open-a-review
    provides: "12-04 POST .../pull-requests/{n}/review returning {task, pr} (live gh pr view) + the source/pr_number/pr_base_ref Task wire fields"
  - phase: 12-open-a-review
    provides: "12-02 board-leak guard keeping source='github_pr' rows off the board; 12-03 PR-base diff (server-side)"
provides:
  - "useOpenReview(projectId) mutation: POST .../pull-requests/{n}/review, stashes the live {task, pr} into the query cache (prDetailKey + task) and routes to the review view"
  - "PR card body as the keyboard-reachable open/reattach trigger (role=button, hover surface, focus ring, in-flight guard); ↗ stopPropagation keeps GitHub-only"
  - "TaskPage source='github_pr' branches: read-only PR title, #num·@author·base·↗ meta line, omitted ⋯ menu, read-only PR-body Description"
  - "AgentTab seed prop: one-shot prefill-not-sent of the canned review prompt after connect (reuses the pasteApiRef bracketed-paste path)"
affects: [12-06-gap-backend, 12-07-gap-frontend]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Open-at-cache: useOpenReview.onSuccess setQueryData(prDetailKey(taskId), pr) + setQueryData(['task', taskId], task) so the routed-to review view renders the read-only header/body immediately without a refetch (reattach survives via the same cache)"
    - "Source-discriminated view shell: one TaskPage branches every Phase-12 delta on task.source==='github_pr' (isPR) — read-only title, PR meta line, omitted ⋯ menu, read-only Description, seeded Start — leaving the manual task view byte-for-byte unchanged"
    - "One-shot seed via the existing imperative paste handle: oneShotSeededRef guards pasteApiRef.current.paste(seed) inside the onConnect callback (bracketed-paste → un-submitted), zero new terminal plumbing"

key-files:
  created: []
  modified:
    - "web/src/api/types.ts - Task gains source/pr_number/pr_base_ref"
    - "web/src/api/pullRequests.ts - PRDetailWire/OpenReviewResponse, prDetailKey, useOpenReview"
    - "web/src/components/board/PRCard.tsx - card body open trigger + ↗ stopPropagation + in-flight/error states; projectId prop"
    - "web/src/components/board/ReviewColumn.tsx - threads projectId to PRCard via ReviewStates"
    - "web/src/pages/TaskPage.tsx - isPR branches: read-only title, PR meta line, omitted ⋯ menu, read-only Description, seeded AgentTab"
    - "web/src/components/task/AgentTab.tsx - seed prop + one-shot post-connect paste"
    - "web/src/components/task/DescriptionTab.tsx - readOnlySource prop via extracted ReadOnlyDescription/EditableDescription split"

key-decisions:
  - "Live PR detail transported via the query cache (useOpenReview.onSuccess setQueryData prDetailKey + task), TaskPage reads it with getQueryData — chosen over a TaskPage re-fetch; the 12-04 endpoint already does a fresh gh pr view on every open (incl. reattach), so the cache is current on each route and on reattach"
  - "DescriptionTab split into ReadOnlyDescription + EditableDescription so the editable variant's hooks (useState/useUpdateTask) are never conditionally skipped (rules-of-hooks) when readOnlySource flips the render path"
  - "Header/meta degrade-don't-break: prDetail?.* falls back to the task's own pr_number/pr_base_ref/title if the cache is empty (e.g. a hard refresh on a deep link) — the review header still renders, never blanks"

patterns-established:
  - "Source-branched TaskPage: a single view shell renders both manual tasks and PR reviews, gating each delta on task.source — no parallel page, no duplicated tab strip"
  - "Open-or-reattach at the cache boundary: the mutation that provisions also seeds the destination view's cache, so routing is instant and reattach is indistinguishable from opening a task"

requirements-completed: []  # NOT verified — human-verify surfaced 5 follow-ups; GHREV-01..05 stay Pending until the gap plans land and the phase is verified.

# Metrics
duration: 18min
completed: 2026-06-14
---

# Phase 12 Plan 05: Open-a-Review Frontend Wiring Summary

**The PR card body becomes a keyboard-reachable open/reattach trigger that routes to a source-branched review view (read-only PR title + `#num · @author · base · ↗` meta line, omitted ⋯ menu, read-only PR-body Description) with a one-shot prefilled-not-sent agent seed — frontend wiring shipped and building green, but the human-verify safety checkpoint surfaced 5 follow-ups now routed to gap plans 12-06/12-07, so the phase is NOT verified.**

## Status: tasks done, human-verify found follow-ups

The two autonomous frontend tasks (1 & 2) are implemented, committed, and pass both `go build ./...` and `cd web && npm run build`. The blocking human-verify checkpoint (Task 3) was run by the user against the live binary and surfaced 5 follow-up issues. The phase is **not verified** and requirements **GHREV-01..05 remain Pending** until the gap plans (12-06 backend, 12-07 frontend) land and the orchestrator re-runs phase verification.

## Performance

- **Duration:** ~18 min (autonomous tasks; excludes the human-verify gate)
- **Completed:** 2026-06-14
- **Tasks:** 2 of 3 (Task 3 is the human-verify gate — run, NOT approved)
- **Files modified:** 7

## Accomplishments

- **Open trigger (GHREV-01/02 frontend half):** `useOpenReview(projectId)` POSTs `.../pull-requests/{n}/review`, stashes the live `{task, pr}` into the query cache (`prDetailKey(taskId)` + `["task", taskId]`), and the PR card body (`role="button"`, Enter/Space, `hover:bg-[#27272a]`, `focus-visible:ring-blue-500`, in-flight guard) routes to `/projects/{id}/tasks/{task.id}`. The ↗ anchor keeps `stopPropagation` (GitHub-only); the checks dot stays non-interactive (falls through to open).
- **Review view shell (source-branched):** `TaskPage` branches every Phase-12 delta on `isPR = task.source === "github_pr"` — a read-only PR title (same box/typography as the editable one, but a `<span>`, no Edit, no hover surface), a `#num · @author · base · ↗` PR meta line replacing `WorktreeMetaLine`, an omitted ⋯ menu, and a read-only PR-body Description. The manual task view is byte-for-byte unchanged.
- **Read-only Description (D-11):** `DescriptionTab` gains `readOnlySource`; when set it renders the prose block with no Edit/Textarea/Save (empty → "No description."). Split into `ReadOnlyDescription` + `EditableDescription` so the editable hooks are never conditionally skipped.
- **Seeded Start (D-06/D-07):** `AgentTab` gains a `seed` prop; after the agent connects, a `oneShotSeededRef`-guarded `pasteApiRef.current.paste(seed)` prefills the canned review prompt (bracketed-paste → un-submitted) — reusing the exact Insert-description mechanism, never auto-sent. *(NOTE: the per-mount ref guard re-injects on every open — flagged below as follow-up #3 for 12-07.)*
- Both builds green; the single binary was rebuilt and run for the human checkpoint.

## Task Commits

1. **Task 1: Task type + useOpenReview hook + PR card open trigger** — `ee5cd89` (feat)
2. **Task 2: TaskPage source branches + seeded AgentTab + read-only Description** — `27f7491` (feat)
3. **Task 3: End-to-end safety checkpoint** — no code; human-verify gate, run by the user, NOT approved (5 follow-ups, below).

**Plan metadata:** this SUMMARY + STATE.md + ROADMAP.md (docs commit).

## Files Created/Modified

- `web/src/api/types.ts` — `Task` gains `source` / `pr_number` / `pr_base_ref` (the 12-02 wire fields).
- `web/src/api/pullRequests.ts` — `PRDetailWire`, `OpenReviewResponse`, `prDetailKey()`, `useOpenReview()` (caches `{task, pr}` on success).
- `web/src/components/board/PRCard.tsx` — body open trigger (`role="button"`, keyboard, hover/focus, in-flight guard, inline "Setting up…" / error); ↗ `stopPropagation`; `projectId` prop.
- `web/src/components/board/ReviewColumn.tsx` — threads `projectId` through `ReviewStates` to each `PRCard`.
- `web/src/pages/TaskPage.tsx` — `isPR` branches: read-only title, PR meta line, omitted ⋯ menu, read-only Description (`readOnlySource`), seeded `AgentTab`; reads live PR detail from `prDetailKey` cache with task-field fallback.
- `web/src/components/task/AgentTab.tsx` — `seed` prop + `oneShotSeededRef`-guarded post-connect paste.
- `web/src/components/task/DescriptionTab.tsx` — `readOnlySource` via extracted `ReadOnlyDescription` / `EditableDescription`.

## Decisions Made

- **Live PR detail via the query cache, not a TaskPage re-fetch** — `useOpenReview.onSuccess` writes `prDetailKey(taskId)` + `["task", taskId]`; `TaskPage` reads with `getQueryData`. The 12-04 endpoint already does a fresh `gh pr view` on every open (including reattach), so the cache is current on each route. Header/meta/Description degrade to the task's own `pr_*`/`title` if the cache is empty (hard refresh on a deep link).
- **DescriptionTab component split** to keep the editable variant's hooks unconditional (rules-of-hooks) when `readOnlySource` toggles the render path.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] DescriptionTab read-only branch would violate rules-of-hooks**
- **Found during:** Task 2 (read-only PR Description)
- **Issue:** Adding an early `if (readOnlySource !== undefined) return …` before `useState`/`useUpdateTask` would conditionally skip hooks.
- **Fix:** Extracted `ReadOnlyDescription` (no hooks) and `EditableDescription` (owns the hooks); `DescriptionTab` dispatches between them, so neither component skips a hook.
- **Files modified:** `web/src/components/task/DescriptionTab.tsx`
- **Verification:** `npm run build` (tsc) passes; no `react-hooks/rules-of-hooks` lint error on this file.
- **Committed in:** `27f7491` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug). **Impact:** necessary for correctness (React hooks contract); no scope creep.

## Issues Encountered

- **Pre-existing lint findings (out of scope):** `npm run lint` reports 20 errors, none introduced by this plan — `react-hooks/purity` (`Date.now()` in render in `PRCard.tsx`/`ReviewColumn.tsx`, verbatim from Phase 11; 12-05 only shifted PRCard's line number) and `react-hooks/set-state-in-effect` (the Phase 3/5 `keepExitedIds`/reattach effects in `TaskPage.tsx`, shadcn's `use-mobile.ts`). The build is green; these are logged in `.planning/phases/12-open-a-review/deferred-items.md` for a lint-cleanup pass.

## Known Issues / Follow-ups (human-verify — routed to gap plans)

The human-verify checkpoint (Task 3) was run live by the user and did NOT pass — 5 issues, now tracked as gap-closure plans. The phase stays unverified and GHREV-01..05 stay Pending until these land.

1. **PR worktree is a detached HEAD — should check out the PR's real head branch** (fallback `pr/<n>` on a local-name collision). Today the worktree is provisioned `--detach` on `headRefOid`; the user wants the named head branch checked out instead. → **12-06 (backend)** — `worktree.CheckoutPR` / the open endpoint provision branch with collision pre-check.
2. **Bash terminal in the PR review opens empty with excessive height that forces scrolling** — a layout/fit regression specific to the PR review view (the terminal pane sizes wrong in this shell). → **12-07 (frontend)** — fix the review-view terminal container height/fit.
3. **The review seed prompt is re-injected on every open** — the `oneShotSeededRef` guard only holds per mount, so reopening the review re-pastes the seed. It must inject **only once**, when the agent is first started (persist the seeded state beyond a single mount). → **12-07 (frontend)** — replace the per-mount ref with a durable once-per-review guard tied to first agent start.
4. **Make the review prompt configurable via a Settings field** — default = the current template (`Review PR #<n> "<title>". …`) with `<n>` / `<title>` placeholders. → **12-06 (backend settings key)** + **12-07 (frontend Settings field + wire the template into the seed)**.
5. **Show a GitHub-style merge line** — `"<author> wants to merge <N> commits into <base> from <head>"`. → **12-06 (backend: commits count + head/base in the open response)** + **12-07 (frontend: render the merge line in the review header/meta)**.

## Next Phase Readiness

- **Frontend wiring is in place and green** — the open/reattach mutation, the source-branched review view, the read-only Description, and the seeded Start all build and route. The 5 follow-ups are refinements/regressions on top of this wiring, not a rewrite.
- **Blockers for phase verification:** the 5 issues above. Gap plans 12-06 (backend: named-branch checkout, configurable-prompt settings key, commits count + head/base in the open response) and 12-07 (frontend: terminal fit fix, once-per-review seed guard, Settings prompt field, merge-line render) must execute, then the orchestrator re-runs phase verification before GHREV-01..05 are marked complete.
- **Do not** mark the phase complete or touch REQUIREMENTS.md GHREV statuses here — they remain Pending by design.

## Self-Check: PASSED

- All 7 modified files exist on disk.
- Commits `ee5cd89` (Task 1) and `27f7491` (Task 2) present in git history.
- `go build ./...` passes; `cd web && npm run build` (tsc + vite) passes.
- Human-verify checkpoint (Task 3): RUN, NOT APPROVED — 5 follow-ups recorded above and routed to 12-06/12-07. Phase NOT verified; GHREV-01..05 NOT marked complete.

---
*Phase: 12-open-a-review*
*Completed (frontend wiring): 2026-06-14 — phase unverified, gap plans 12-06/12-07 pending*
