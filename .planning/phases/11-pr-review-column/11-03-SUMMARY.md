---
phase: 11-pr-review-column
plan: 03
subsystem: web
tags: [react, typescript, tanstack-query, polling, presentational-card, dry-refactor]

# Dependency graph
requires:
  - phase: 11-pr-review-column (Plan 02)
    provides: "GET /api/projects/{id}/pull-requests[?refresh=1] always-200 {state,stale,fetchedAt,prs} wire contract this hook consumes"
  - phase: 07-claude-quota-indicator
    provides: "useQuota/useRefreshQuota hook pair cloned here; formatAgo helper lifted out of QuotaIndicator"
  - phase: 01 (board)
    provides: "TaskCard CardRow surface (rounded-md border bg-card px-3 py-2, line-clamp-2 flex-1 title) the PR card mirrors; StatusDot dot markup the checks dot echoes"
provides:
  - "web/src/api/pullRequests.ts — PRSummary + PullRequestsResponse types; usePullRequests(projectId) (60s visibility-paused poll); useRefreshPullRequests(projectId) (?refresh=1 write-back)"
  - "web/src/lib/time.ts — shared formatAgo (single source for quota footer + PR cards)"
  - "web/src/components/board/PRCard.tsx — presentational PR card with conditional checks dot + inert body + ↗ link"
affects:
  - "11-04 (Review column composes PRCard + the usePullRequests/useRefreshPullRequests hooks; branches on state/stale)"
  - "12-open-a-review (PRCard body becomes interactive; PRSummary head/base/fork fields feed the worktree checkout)"

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Per-project TanStack poll keyed ['pull-requests', projectId] with refetchIntervalInBackground:false (visibility-paused) — verbatim clone of the quota hook pair, re-keyed per project"
    - "?refresh=1 manual refresh writes back via qc.setQueryData (no invalidate round-trip) — useRefreshQuota precedent"
    - "Presentational card variant of TaskCard's CardRow with an INERT body — the only interactive element is a single ↗ anchor (D-08/D-09)"
    - "Conditional dot render (no dot for 'none') — no reserved gutter, no layout shift (D-04, the dotless-card rule)"
    - "DRY lift of a shared helper (formatAgo) to web/src/lib/ when a second consumer appears, rather than copying"

key-files:
  created:
    - web/src/api/pullRequests.ts
    - web/src/lib/time.ts
    - web/src/components/board/PRCard.tsx
  modified:
    - web/src/components/quota/QuotaIndicator.tsx

key-decisions:
  - "Lifted formatAgo to web/src/lib/time.ts (RESEARCH Open Q2 = LIFT, not copy) — one tier-logic shared by the quota footer and the PR card; QuotaIndicator imports it, behavior byte-identical"
  - "Checks dot uses a small local 3-way switch (checksDot) typed Exclude<checks,'none'>, NOT StatusDot's agent-status-shaped dotMeta (D-03) — keeps the PR card independent of agent semantics"
  - "Date.now() is read at render in the meta line (acceptable: the 60s poll re-renders the card; no ticking clock needed on a card per UI-SPEC)"
  - "PRSummary fetches the rich head/base/fork field set (Phase 12/13) but the card renders only the GHCOL-03 minimal set (D-01/D-02)"

requirements-completed: [GHCOL-03, GHCOL-04]

# Metrics
duration: 3min
completed: 2026-06-13
---

# Phase 11 Plan 03: PR Data Hooks + PR Card Summary

**The frontend read layer for the Review column: `usePullRequests`/`useRefreshPullRequests` (a verbatim per-project clone of the quota hook pair — 60s visibility-paused poll + `?refresh=1` write-back against the always-200 endpoint), a `formatAgo` lifted to `web/src/lib/time.ts` so the quota footer and PR cards share one implementation, and a presentational `PRCard` mirroring `TaskCard`'s `CardRow` with a conditional checks dot, an inert body, and a single ↗ link to github.com.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-06-13T18:13:58Z
- **Completed:** 2026-06-13T18:17:18Z
- **Tasks:** 3 (all `type="auto"`)
- **Files:** 3 created, 1 modified

## Accomplishments
- `usePullRequests(projectId)` polls `GET /api/projects/${projectId}/pull-requests` every 60s with `refetchIntervalInBackground:false` (GHCOL-04 visibility pause), keyed `["pull-requests", projectId]` so only the visible project's column drives a `gh` call (D-11). `useRefreshPullRequests(projectId)` hits `?refresh=1` and writes the result straight into the cache via `qc.setQueryData` — a verbatim shape clone of `useQuota`/`useRefreshQuota`.
- `PRSummary`/`PullRequestsResponse` mirror the Plan 01/02 wire contract exactly: the always-200 `{state, stale, fetchedAt, prs}` envelope plus the rich `PRSummary` (number/title/author/updatedAt/url/checks + the fetched-unrendered headRefName/headRefOid/baseRefName/isCrossRepository). Consumers branch on `state`/`stale`, never HTTP status. No date library added.
- `formatAgo` lifted to `web/src/lib/time.ts` (RESEARCH Open Q2, choice = LIFT): the exact 3-tier logic ("12s"/"7m"/"1h 5m") now lives once; `QuotaIndicator` imports it with every call site unchanged — a pure, behavior-identical refactor (zero local `formatAgo` definitions remain).
- `PRCard` is a presentational variant of `TaskCard`'s `CardRow`/`CardShell`: same `rounded-md border border-border bg-card px-3 py-2` surface and `line-clamp-2 flex-1 text-sm font-medium` title, a right-aligned cluster with the checks dot (green/red/amber, omitted entirely for `none` — no gutter, no layout shift, D-04) and the ↗ `ExternalLink` anchor (`target="_blank" rel="noreferrer"`, per-PR aria-label), plus a `#number · @author · updated X ago` meta line. The body is inert (no hover bg, no grab cursor, no body onClick, no `useSortable`) — the anchor is the only interactive element (D-08/D-09).
- Full `tsc --noEmit -p tsconfig.app.json` green after every task; the production `npm run build` (tsc -b + vite build, 2212 modules) compiles clean.

## Task Commits

Each task was committed atomically:

1. **Task 1: pullRequests.ts hooks + types** - `7a8debc` (feat)
2. **Task 2: lift formatAgo to web/src/lib/time.ts + refactor QuotaIndicator** - `4022da4` (refactor)
3. **Task 3: PRCard presentational variant** - `55239e3` (feat)

**Plan metadata:** _(final docs commit)_

## Files Created/Modified
- `web/src/api/pullRequests.ts` (created) — `PRSummary`/`PullRequestsResponse` types; `usePullRequests`/`useRefreshPullRequests` per-project hooks (60s poll, visibility-paused, `?refresh=1` write-back).
- `web/src/lib/time.ts` (created) — shared `formatAgo(iso, now)` (lifted out of QuotaIndicator).
- `web/src/components/board/PRCard.tsx` (created) — presentational PR card: conditional checks dot, inert body, single ↗ anchor, meta line via shared `formatAgo`.
- `web/src/components/quota/QuotaIndicator.tsx` (modified) — deleted the local `formatAgo`, added `import { formatAgo } from "@/lib/time"`; all call sites unchanged.

## Decisions Made
- **LIFT formatAgo, don't copy** (RESEARCH Open Q2) — one implementation in `web/src/lib/time.ts`, imported by both the quota footer and the PR card. The refactor is byte-identical in behavior (same null-guard, same 3-tier output); zero duplicate definitions.
- **Local `checksDot` switch over reusing `StatusDot.dotMeta`** (D-03) — `dotMeta` is shaped for agent status (working/waiting/idle/exited). The PR card uses a tiny 3-way switch typed `Exclude<PRSummary["checks"], "none">`, so the compiler enforces the "none renders nothing" contract and the card stays decoupled from agent semantics.
- **`Date.now()` at render for the meta age** — acceptable per UI-SPEC: the 60s poll re-renders the card, so no per-card ticking clock (like the quota popup's `useNow`) is needed.
- **Fetch rich, render minimal** (D-01/D-02) — `PRSummary` carries headRefName/headRefOid/baseRefName/isCrossRepository for Phase 12/13's worktree checkout + diff base, but the Phase 11 card renders only the GHCOL-03 set.

## Deviations from Plan

None - plan executed exactly as written. The plan's prescribed hook bodies, types, formatAgo body, and card layout were used as specified. One cosmetic adjustment with no behavioral effect: the PRCard doc comment was reworded to avoid the literal trigger tokens (`cursor-grab`) so the inert-body acceptance grep (`grep -E 'useSortable|cursor-grab|hover:bg-'`) reads only real markup, not prose — the rendered card was always inert.

## Issues Encountered
- None affecting the code. Two acceptance-criteria greps initially mis-reported during verification: (1) the inert-body grep matched the word `cursor-grab` inside a doc comment (resolved by rewording the comment — the markup was never non-inert); (2) the per-PR aria-label grep failed under single-quote shell escaping of `$` and backticks but matched cleanly with `grep -F` (the aria-label was present and correct on line 61 throughout). Neither was a code defect.

## Verification
- `npx tsc --noEmit -p tsconfig.app.json` — green (all four files typecheck).
- `npm run build` (tsc -b + vite build) — green, 2212 modules transformed.
- All acceptance criteria across the three tasks pass (60s/visibility-paused poll, per-project query key, `?refresh=1` write-back, reduced checks type, no date dep; single shared `formatAgo` with no duplicate; ↗ link with `target=_blank`/`rel=noreferrer`/per-PR aria-label, conditional dot with the three hues, CardRow-matching title, inert body).

## User Setup Required
None.

## Next Phase Readiness
- **Plan 11-04 (Review column)** can now compose `PRCard` over `usePullRequests(projectId)` / `useRefreshPullRequests(projectId)`, branching the column shell on `data.state` (`ok`/`no_gh`/`auth_required`/`error`; `disabled` is gated out by the render gate) and `data.stale` (amber stale footer), sorting `prs` by `updatedAt` desc (D-14), and rendering the collapse rail + count badge. The hooks, the card primitive, and the shared `formatAgo` are all in place; the column is the only remaining piece of the frontend.

---
*Phase: 11-pr-review-column*
*Completed: 2026-06-13*

## Self-Check: PASSED

- All created/modified files present (web/src/api/pullRequests.ts, web/src/lib/time.ts, web/src/components/board/PRCard.tsx, web/src/components/quota/QuotaIndicator.tsx, 11-03-SUMMARY.md).
- All 3 task commits present in git history (7a8debc, 4022da4, 55239e3).
- `tsc --noEmit -p tsconfig.app.json` + full `npm run build` green.
