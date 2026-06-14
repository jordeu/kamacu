# Deferred Items — Phase 12 (open-a-review)

Out-of-scope discoveries logged during execution. NOT fixed here (scope boundary:
only auto-fix issues directly caused by the current task's changes).

## Pre-existing lint findings (eslint `react-hooks` rules) — surfaced during 12-05

The frontend `npm run lint` reports 20 pre-existing errors, none introduced by
plan 12-05. The build (`tsc -b && vite build`) passes clean — these are
lint-only advisories from the newer `react-hooks` rule set:

- `react-hooks/purity` — `Date.now()` called during render in
  `web/src/components/board/PRCard.tsx` (the `formatAgo(pr.updatedAt, Date.now())`
  meta line, verbatim from Phase 11) and `web/src/components/board/ReviewColumn.tsx`
  (the stale footer). Pre-existing since Phase 11 (12-05 only shifted PRCard's line
  number by wrapping the body in the open-trigger div). Fix: pass a render-stable
  `now` (e.g. a single `Date.now()` hoisted to a parent, or `useState(() => Date.now())`).
- `react-hooks/set-state-in-effect` — multiple `setState`-within-`useEffect`
  patterns in `web/src/pages/TaskPage.tsx` (the `keepExitedIds`/reattach effects from
  Phase 3/5) and `web/src/hooks/use-mobile.ts` (shadcn boilerplate). Pre-existing.

These were present before 12-05 and span files/lines this plan did not author.
Deferring per the executor scope boundary — a lint-cleanup pass is the right home.
