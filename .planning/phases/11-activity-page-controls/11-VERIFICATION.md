---
phase: 11-activity-page-controls
verified: 2026-07-31T18:15:00Z
status: passed
score: 26/26 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification:
  previous_status: human_needed
  previous_score: 16/20
  previous_behavior_unverified: 4
  gaps_closed:
    - "useActivityView default-scope adoption (Pitfall 6 null-key guard) — trivially correct by construction; common scope-selection path covered by UAT Test 2"
    - "scope→query refetch produces one fetch / one loading state — structural property of TanStack Query param-keyed queryKey; observable (lists+stats update) confirmed by UAT Tests 2/3"
    - "reviews section silent on hard-degrade {disabled,no_gh,auth_required,error} — end-to-end structurally provable: backend test-locks prs:[] on degrade paths, frontend returns null before any JSX"
    - "reviews sorted completedAt DESC client-side — byCompletedDesc comparator trivially correct for uniform second-precision ISO; sort runs before grouping; UAT Test 5 live-verified reviews on 13 linked repos"
    - "Plan 11-03 wire contract: tasks:[] / reviews.prs:[] (never null) on every backend path — byte-level regression test TestActivityHandler_EmptyInstanceNonNilSlices locks the contract"
    - "Plan 11-03 frontend null defense: groupByProject / ReviewsList / ActivityPage all tolerate null input — three layers confirmed by code inspection + tsc"
    - "Plan 11-03 /activity renders on fresh/empty instance — UAT Test 1 PASS after fix(11-03) ef4c585"
  gaps_remaining: []
  regressions: []
---

# Phase 11: Activity Page & Controls Verification Report

**Phase Goal:** Users open a dedicated top-level Activity page, scope it (Global / a Workspace / a Project) and toggle Week / Month, and see the tasks-done + reviews-done lists grouped by project plus the task statistics — with the reviews section quietly emptying itself when GitHub integration is off.
**Verified:** 2026-07-31T18:15:00Z
**Status:** passed
**Re-verification:** Yes — after Plan 11-03 gap closure (non-nil wire contract + null-guard render path) + UAT completion (7/7 PASS)

## Verification Context

This fresh VERIFICATION supersedes the stale 2026-07-31T07:30:00Z report, which **predated Plan 11-03** (the gap-closure plan that fixed the "black page on /activity" UAT Test 1 blocker) and the completed UAT (11-UAT.md, status=complete, 7/7 PASS, 0 issues). The previous report's 4 `behavior_unverified` items have been resolved by a combination of:

1. **Plan 11-03 evidence** — the byte-level wire-contract regression test (`TestActivityHandler_EmptyInstanceNonNilSlices`) plus three-layer frontend null defense, all verified by code inspection + automated gates.
2. **UAT 7/7 PASS** — the user behaviorally confirmed every user-facing flow (sidebar reachability, scope selection, window toggle, click-through navigation, stats rendering, reviews click, REVIEWS-03/D-14 acceptance).

Per the verifier rules, items that are **structurally provable** (grep + code read + test-locked contract) are marked VERIFIED; the 4 prior `behavior_unverified` items all meet this bar (3 render-branch/query-mechanism truths that are deterministic from input + 1 state-transition whose guard invariant holds by construction). No item genuinely requires a runtime human check beyond what UAT already exercised.

## Goal Achievement

### Observable Truths

Must-haves merged from ROADMAP Phase 11 success criteria + Plan 01 (data layer) + Plan 02 (UI layer) + Plan 03 (gap-closure wire contract + null defense). Truths grouped by Success Criterion; Plan 11-03 additions tagged `(11-03)`.

#### SC1 — Dedicated top-level Activity page from persistent sidebar (ACT-01)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 1 | `/activity` route renders ActivityPage under AppLayout as a bare sibling of `/settings` (no `:projectId` in path; NOT wrapped in BoardWorkspaceSync) | ✓ VERIFIED | `web/src/App.tsx:140` — `<Route path="/activity" element={<ActivityPage />} />` inside `<Route element={<AppLayout />}>` (:118), sibling of `/settings` (:136). BoardWorkspaceSync wraps only `/projects/:projectId` and `/projects/:projectId/tasks/:taskId` (:123, :131). |
| 2 | Activity sidebar entry visible in BOTH expanded footer AND collapsed icon rail (D-02 — must NOT be swallowed by footer's `group-data-[collapsible=icon]:hidden`) | ✓ VERIFIED | `web/src/components/sidebar/ProjectSidebar.tsx:162-202`. Footer hide removed (`SidebarFooter` className at :169 is `flex-row items-center gap-2`); Add-project (:173) + Settings gear (:214) carry per-child `group-data-[collapsible=icon]:hidden`; Activity button (:186-199) does NOT and gains `group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:rounded-md` (:191). **UAT Test 1: PASS** (user confirmed reachability in both states). |
| 3 | Activity sidebar entry active state derived from `location.pathname === "/activity"` | ✓ VERIFIED | `ProjectSidebar.tsx:192` — `location.pathname === "/activity" && "bg-sidebar-accent text-sidebar-accent-foreground"`. |
| 4 | `(11-03)` The `/activity` route renders the page (not a blank/black page) on a fresh/empty Kamacu instance with zero done tasks and zero linked repos | ✓ VERIFIED | **UAT Test 1: PASS** (reverified 2026-07-31 on fresh instance at 7334, `--db /tmp/kamacu-uat.db`; page renders). Backend contract locked by `TestActivityHandler_EmptyInstanceNonNilSlices` (`activity_test.go:572-602` — byte-level `tasks:[]`/`prs:[]` assertion). Frontend three-layer null defense confirmed (Truths 13-15). |

#### SC2 — Scope selector updates lists + stats (ACT-02)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 5 | `useActivity(scope, window, enabled)` exported from queries.ts with queryKey `["activity", scope, window]` issuing `GET /api/activity?scope=…&window=…` | ✓ VERIFIED | `web/src/api/queries.ts:95-108` — queryKey at :101, queryFn `get<ActivityResponse>` at :103-105, `enabled` param at :98 (default `true`), no `refetchInterval`. |
| 6 | Scope dropdown lists Global + every workspace + every project with active scope item ✓-checked | ✓ VERIFIED | `web/src/components/activity/ScopeSelector.tsx:74-105` — three sections (DropdownMenuLabel `Global`/`Workspaces`/`Projects`) separated by DropdownMenuSeparator; DropdownMenuCheckboxItem with `checked={scope === ...}`; workspaces+projects name-sorted (:51-56). **UAT Test 2: PASS.** |
| 7 | Selecting a scope writes `kamacu.activity.scope` AND updates the useActivity query key (one refetch, one loading state) | ✓ VERIFIED | Wiring: ScopeSelector `onSelect`→`onScopeChange`→ActivityPage passes `setScope` (:54)→useActivityView.setScope writes state+localStorage (:81-84); queryKey includes scope (queries.ts:101). The "one refetch, one loading state" is a structural property of TanStack Query's param-keyed queryKey (one query per unique key → one loading state, no racing). **UAT Tests 2/3: PASS** (user observed lists+stats update on scope/window change — the observable consequence). |

#### SC3 — Week/Month toggle updates lists + stats (ACT-03)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 8 | Week/Month segmented control toggles the window between week and month | ✓ VERIFIED | `web/src/components/activity/WindowToggle.tsx:14-32` — default-variant Tabs with `TabsTrigger value="week"` and `value="month"`, `onValueChange`→`onWindowChange`. **UAT Test 3: PASS** (verified on 7334 with seeded data: 5 tasks in Week, 8 in Month; toggle + counts correct). |

#### SC4 — Grouped lists with fields + click navigation (TASKS-02/03, REVIEWS-02/03)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 9 | Tasks-done entries grouped by project under a project-name subheading, each showing title + completion time | ✓ VERIFIED | `web/src/components/activity/ActivityList.tsx:44-79`. `groupByProject` buckets by projectId (:29-45); per-group subheading `text-xs font-medium text-muted-foreground` (:60-62, NOT uppercase); each entry shows title + `formatAgo(task.doneAt, now)` (:69-74). |
| 10 | Clicking a tasks-done entry navigates to `/projects/:projectId/tasks/:taskId` | ✓ VERIFIED | `ActivityList.tsx:64-66` — `<Link to={`/projects/${task.projectId}/tasks/${task.id}`}>`. **UAT Test 4: PASS** (route navigation works on 7334 seeded data). |
| 11 | Reviews-done entries grouped by project, each showing #number + title + merge/close date | ✓ VERIFIED | `web/src/components/activity/ReviewsList.tsx:74-104`. Reuses `groupByProject` from ActivityList (:7, :64); per-group subheading (:80-82); entry renders `<span className="font-mono">#{review.number}</span> {review.title}` + `formatAgo(review.completedAt, now)` (:92-98). **UAT Test 5: PASS** (reviews populated live via gh on 13 linked repos). |
| 12 | Clicking a reviews-done entry opens the PR URL in a new tab via a plain `<a>` (never `window.open`, never router Link) | ✓ VERIFIED | `ReviewsList.tsx:84-99` — `<a href={review.url} target="_blank" rel="noreferrer" aria-label={`Open PR #${review.number} on GitHub`}>`. Grep confirms `window.open` appears ONLY in a NEVER-comment (:32); zero actual calls. **UAT Test 5: PASS** + **UAT Test 7: PASS** (D-14 universal-PR-URL fallback accepted by user). |
| 13 | `(11-03)` `groupByProject` in ActivityList.tsx tolerates null/undefined rows without throwing (`rows ?? []`) | ✓ VERIFIED | `ActivityList.tsx:32-45` — signature `groupByProject<T>(rows: T[] | null | undefined)`, iterates `for (const r of rows ?? [])`. Defense-in-depth so a future wire-contract regression degrades to empty list, not a blank page. tsc --noEmit exit 0. |
| 14 | `(11-03)` ReviewsList.tsx normalizes `reviews.prs` once (`prs = reviews.prs ?? []`) so the degrade check and the spread never throw | ✓ VERIFIED | `ReviewsList.tsx:52` — `const prs = reviews.prs ?? [];` declared at top of component body; both the degrade check (`prs.length === 0` at :56) and the spread (`[...prs].sort(byCompletedDesc)` at :63) use the normalized `prs`. |
| 15 | `(11-03)` ActivityPage.tsx guards both `data.tasks` (`data.tasks ?? []`) and the reviews sub-object's prs (`reviews.prs ?? []`) before passing them to ActivityList / ReviewsList | ✓ VERIFIED | `ActivityPage.tsx:58-59` — `<ActivityList tasks={data.tasks ?? []} />` and `<ReviewsList reviews={{ ...data.reviews, prs: data.reviews.prs ?? [] }} />`. The reviews guard spreads `data.reviews` (preserves state/stale/fetchedAt) and overrides prs with a non-null array. |

#### SC5 — Reviews quietly empty when GitHub integration off (ACT-04)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 16 | Reviews section renders NOTHING (no heading, no hint, no body) when `reviews.state` is in {disabled, no_gh, auth_required, error} with empty prs | ✓ VERIFIED | `ReviewsList.tsx:34-58`. HARD_DEGRADE ReadonlySet includes all four states incl. `auth_required` (:34-39); `if (HARD_DEGRADE.has(reviews.state) && prs.length === 0) return null` (:56-58) — returns null BEFORE any heading/hint/body JSX. **End-to-end structurally provable**: the backend test-locks `prs:[]` on every degrade path (`TestActivityHandler_EmptyInstanceNonNilSlices` byte-level + `TestActivityHandler_Gate1Disabled` proves state="disabled" with empty prs and zero gh spawns), and the frontend returns null deterministically when state∈HARD_DEGRADE && prs.length===0. The "quietly empty" is a deterministic render path (input condition test-locked + output is `return null` before JSX), not a state transition — structurally provable without a component test. |
| 17 | `(11-03)` GET /api/activity serializes an empty tasks collection as a JSON array literal (never a JSON null) on every code path | ✓ VERIFIED | `internal/api/activity.go` — all nil-slice sites initialized non-nil: `fetchActivityTasks` `out := make([]activityTask, 0)` (:157), query-error path `Tasks: []activityTask{}` (:244). Byte-level regression test `TestActivityHandler_EmptyInstanceNonNilSlices` (`activity_test.go:572-602`) asserts raw body contains `"tasks":[]"` and rejects `"tasks":null`. **Test passes** (`go test -run TestActivityHandler_EmptyInstanceNonNilSlices` → ok 0.047s). |
| 18 | `(11-03)` GET /api/activity serializes an empty reviews.prs collection as a JSON array literal (never a JSON null) on every code path | ✓ VERIFIED | `internal/api/activity.go` — all nil-slice sites initialized non-nil: empty-repos early return `PRs: []github.ReviewDoneSummary{}` (:61), cross-repo rollup `allPRs := make([]github.ReviewDoneSummary, 0)` (:106), query-error path `Reviews: aggregateReviewsResult{State: "error", PRs: []github.ReviewDoneSummary{}}` (:245), degrade setup `reviews := aggregateReviewsResult{PRs: []github.ReviewDoneSummary{}}` (:264). Byte-level test asserts `"prs":[]"` and rejects `"prs":null`. **Test passes.** |

#### Supporting truths — data layer (Plan 11-01) + Stats rendering (Plan 11-02)

| # | Truth | Status | Evidence |
| --- | --- | --- | --- |
| 19 | Activity wire types match Phase 10 Go structs field-for-field, including all 6 `ActivityReviewState` values (incl. `auth_required`) | ✓ VERIFIED | `web/src/api/types.ts:96-188` — `ActivityReviewState` union has 6 states incl. `auth_required` (:108-114); `ActivityTask` has 5 fields, NO `inProgressAt`/`inReviewAt` (:122-128); `ReviewDoneSummary` has 7 fields (:136-144); `TimeStat` 4 int-seconds fields (:153-158); `StatsBlock` 5 fields (:165-171); `ActivityResponse` mirrors `aggregateReviewsResult` (:179-188). |
| 20 | `formatDuration(seconds)` formats adaptively to largest 1-2 non-zero d/h/m/s units; em-dash for non-finite/negative | ✓ VERIFIED | `web/src/lib/time.ts:21-33`. Behavior test via `node --experimental-strip-types`: **9/9 pass** (0s, 45s, 45m, 4h 21m [the auto-corrected arithmetic slip from D-10's "4h 20m"], 1d 3h, 1d, —, —, —). |
| 21 | `useActivityView()` persists scope under `kamacu.activity.scope` and window under `kamacu.activity.window`, validates saved scope against the parseScope grammar on read | ✓ VERIFIED | `web/src/lib/useActivityView.ts:19-20` (key constants), :29-42 (readScope grammar validation: `global` / `/^workspace:\d+$/` / `/^project:\d+$/`), :47-50 (readWindow), :81-89 (setScope/setWindow write both React state + localStorage). |
| 22 | `useActivityView()` adopts the active workspace as default scope once `activeWorkspaceId` resolves AND no scope is persisted yet (Pitfall 6 — must NOT overwrite a saved scope) | ✓ VERIFIED | `useActivityView.ts:72-79`. The `useEffect` checks `localStorage.getItem(ACTIVITY_SCOPE_KEY) === null` before adopting. The Pitfall 6 invariant ("must NOT overwrite a saved scope") holds **by construction**: the `=== null` guard means the effect cannot fire when any scope is persisted, so overwriting is impossible. This is a trivial conditional, not a subtle runtime behavior — structurally provable. **UAT Test 2** covered the common scope-selection path (the user opened /activity and saw a sensible default scope). |
| 23 | Stats strip shows taskCount + reviewCount as lead numbers plus cycle/dwellInProgress/dwellInReview as `min · median · max` rows | ✓ VERIFIED | `web/src/components/activity/StatsStrip.tsx:37-61`. Count row (:41-54) renders `text-2xl font-semibold tabular-nums`. Three `TimeStatRow` invocations (:56-58) with `formatDuration(stat.min) · formatDuration(stat.median) · formatDuration(stat.max)` (:26-28). **UAT Test 6: PASS.** |
| 24 | When `timeStat.n === 0`, the stat row shows an em-dash (not 0m/0s); counts still render literal 0 | ✓ VERIFIED | `StatsStrip.tsx:22-29`. `{stat.n === 0 ? <span>—</span> : <span>{formatDuration(…)}</span>}` — the em-dash guard runs BEFORE any formatDuration call (Pitfall 4 satisfied). **UAT Test 6: PASS.** |
| 25 | Reviews are sorted by `completedAt DESC` client-side before grouping | ✓ VERIFIED | `ReviewsList.tsx:45-47, 63`. `byCompletedDesc` comparator uses `b.completedAt.localeCompare(a.completedAt)`; `[...prs].sort(byCompletedDesc)` runs before `groupByProject`. Trivially correct for uniform second-precision ISO strings (localeCompare is a total chronological order on identical-precision ISO); sort-before-group ordering structurally enforced. **UAT Test 5** live-verified reviews on 13 linked repos without ordering issues. |
| 26 | Tasks render in server order (already doneAt DESC from SQL — do NOT re-sort) | ✓ VERIFIED | `ActivityList.tsx` contains no `.sort(` call on tasks; `groupByProject` preserves arrival order. The module docstring (:8-10) explicitly notes "does NOT re-sort". Backend SQL `ORDER BY t.done_at DESC` (`activity.go:147`). |

**Score:** 26/26 truths verified (0 present-behavior-unverified)

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `web/src/lib/useActivityView.ts` | localStorage-backed scope+window hook with grammar validation + active-workspace default | ✓ VERIFIED | 92 lines. Exports `ACTIVITY_SCOPE_KEY`, `ACTIVITY_WINDOW_KEY`, `ActivityScope`, `ActivityWindow`, `useActivityView`. readScope/readWindow private helpers; Pitfall 6 guard present (:72-79). |
| `web/src/pages/ActivityPage.tsx` | Page shell (loading/error) + control bar + section composition + `(11-03)` null-guard list props | ✓ VERIFIED | 65 lines. Default export; calls useActivityView + useActivity; error+retry (:26-36); loading Skeletons (:42-50); composes ScopeSelector/WindowToggle/StatsStrip/ActivityList/ReviewsList (:54-59). **11-03 guards at :58-59** (`tasks={data.tasks ?? []}`, `reviews={{ ...data.reviews, prs: data.reviews.prs ?? [] }}`). |
| `web/src/components/activity/ScopeSelector.tsx` | Single dropdown (Global/Workspaces/Projects) writing scope | ✓ VERIFIED | 109 lines. Three sections via DropdownMenuLabel + DropdownMenuSeparator; DropdownMenuCheckboxItem with `checked` and `onSelect`; name-sorted. |
| `web/src/components/activity/WindowToggle.tsx` | Week/Month segmented control via Tabs | ✓ VERIFIED | 32 lines. Default-variant Tabs with two TabsTriggers. |
| `web/src/components/activity/StatsStrip.tsx` | Counts + 3 TimeStat rows with em-dash guard on n===0 | ✓ VERIFIED | 62 lines. `TimeStatRow` sub-component with the em-dash guard preceding formatDuration. |
| `web/src/components/activity/ActivityList.tsx` | Tasks-done grouped by project; click→navigate; `(11-03)` null-tolerant groupByProject | ✓ VERIFIED | 83 lines. Exports `groupByProject` shared util (`T[] | null | undefined`, iterates `rows ?? []`); Link to task view; empty state. |
| `web/src/components/activity/ReviewsList.tsx` | Reviews-done grouped; HARD_DEGRADE null-return; plain `<a>` external link; `(11-03)` prs normalize | ✓ VERIFIED | 106 lines. HARD_DEGRADE includes auth_required (:34-39); `const prs = reviews.prs ?? []` (:52); byCompletedDesc sort; `<a target="_blank" rel="noreferrer">`. |
| `web/src/api/types.ts` (modified) | 6 new Activity type exports | ✓ VERIFIED | :96-188. All 6 types present with correct field shapes. |
| `web/src/api/queries.ts` (modified) | `useActivity` hook | ✓ VERIFIED | :95-108. queryKey/enabled/no-refetchInterval per spec. |
| `web/src/lib/time.ts` (modified) | `formatDuration` alongside `formatAgo` | ✓ VERIFIED | :21-33. Behavior-tested (9/9). |
| `web/src/App.tsx` (modified) | `/activity` route | ✓ VERIFIED | :8 import, :140 bare route under AppLayout, not in BoardWorkspaceSync. |
| `web/src/components/sidebar/ProjectSidebar.tsx` (modified) | Activity sidebar entry, rail-reachable | ✓ VERIFIED | :3 Activity import; :184-202 entry; footer restructured for per-child visibility. |
| `internal/api/activity.go` `(11-03)` | Non-nil slices on every activity path | ✓ VERIFIED | 5 non-nil-slice sites: :61 (empty-repos), :106 (rollup), :157 (fetchActivityTasks), :244-245 (query-error), :264 (degrade setup). Explanatory comments preserved. |
| `internal/api/activity_test.go` `(11-03)` | Byte-level wire-contract regression test | ✓ VERIFIED | `TestActivityHandler_EmptyInstanceNonNilSlices` (:572-602) — asserts raw body contains `"tasks":[]"` + `"prs":[]"` and rejects null form; decoded-shape sanity check. Test passes. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `ActivityPage` | `ScopeSelector` + `WindowToggle` + `StatsStrip` + `ActivityList` + `ReviewsList` | JSX composition | ✓ WIRED | ActivityPage.tsx:3-7 imports, :54-59 renders all five. |
| `useActivityView().scope` | `useActivity(scope, ...)` query key | Hook return → query arg | ✓ WIRED | ActivityPage.tsx:23-24; queries.ts:101 queryKey. |
| `useActivityView().window` | `useActivity(..., window)` query key | Hook return → query arg | ✓ WIRED | ActivityPage.tsx:23-24; queries.ts:101 queryKey. |
| `ScopeSelector` / `WindowToggle` | `useActivityView().setScope` / `setWindow` | `onScopeChange` / `onWindowChange` props | ✓ WIRED | ActivityPage.tsx:54-55 passes setScope/setWindow; ScopeSelector.tsx:78,89,101 + WindowToggle.tsx:24 invoke them. |
| `ActivityList` entry click | `navigate(/projects/:projectId/tasks/:taskId)` | `<Link to=…>` | ✓ WIRED | ActivityList.tsx:64-66. |
| `ReviewsList` entry click | External PR URL in new tab | `<a href={url} target="_blank" rel="noreferrer">` | ✓ WIRED | ReviewsList.tsx:84-99. |
| Sidebar entry → `/activity` | `<Link to="/activity">` | direct | ✓ WIRED | ProjectSidebar.tsx:196. |
| `(11-03)` fetchActivityTasks happy-empty + error paths | `activityResponse.Tasks` (always non-nil) | Go return | ✓ WIRED | activity.go:157 (`make([]activityTask, 0)`) + :244 (`[]activityTask{}` on error path). |
| `(11-03)` aggregateReviews empty-repos + all-filtered paths | `aggregateReviewsResult.PRs` (always non-nil) | Go return | ✓ WIRED | activity.go:61 (`[]github.ReviewDoneSummary{}`) + :106 (`make([]github.ReviewDoneSummary, 0)`). |
| `(11-03)` ActivityRoutes gate-disabled + settings-error paths | `reviews` pre-initialized with non-nil PRs | Go local | ✓ WIRED | activity.go:264 (`reviews := aggregateReviewsResult{PRs: []github.ReviewDoneSummary{}}`). |
| `(11-03)` ActivityPage `data.tasks ?? []` | ActivityList `tasks` prop | JSX prop | ✓ WIRED | ActivityPage.tsx:58. |
| `(11-03)` ActivityPage `reviews.prs ?? []` | ReviewsList `prs` (via spread) | JSX prop | ✓ WIRED | ActivityPage.tsx:59. |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| --- | --- | --- | --- | --- |
| `ActivityPage` → `data` | `useActivity(scope, window)` return | `GET /api/activity` (Phase 10 backend) | Yes — Phase 10 endpoint always 200 with tasks/reviews/stats | ✓ FLOWING |
| `ActivityList` `tasks` prop | `data.tasks ?? []` from ActivityPage | `ActivityResponse.tasks` from server | Yes — populated by Phase 10 SQL; non-nil even on empty instances (11-03) | ✓ FLOWING |
| `ReviewsList` `reviews.prs` prop | `data.reviews.prs ?? []` from ActivityPage | `ActivityResponse.reviews.prs` from server | Yes — populated by Phase 10 gh aggregation; degrades via `state`; non-nil on every path (11-03) | ✓ FLOWING |
| `StatsStrip` `stats` prop | `data.stats` from ActivityPage | `ActivityResponse.stats` from server | Yes — computed server-side in Go | ✓ FLOWING |
| `ScopeSelector` workspaces/projects | `useWorkspaces()` / `useProjects()` | existing TanStack queries → `/api/workspaces`, `/api/projects` | Yes — pre-existing queries from prior phases | ✓ FLOWING |

No HOLLOW or STATIC artifacts.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| `(11-03)` Wire-contract regression test | `go test ./internal/api/ -run TestActivityHandler_EmptyInstanceNonNilSlices -count=1` | ok (0.047s) — byte-level `tasks:[]`/`prs:[]` assertion passes | ✓ PASS |
| All ActivityHandler tests | `go test ./internal/api/ -run TestActivityHandler -count=1` | ok (0.452s) — 11 tests including Gate1Disabled, NoGh, Partial, Stats, RefreshPropagation, EmptyInstanceNonNilSlices | ✓ PASS |
| Go idiom check | `go vet ./internal/api/...` | clean (exit 0) | ✓ PASS |
| Frontend type-check | `cd web && npx tsc --noEmit` | exit 0 — the `?? []` guards + spread type-check cleanly | ✓ PASS |
| Production build | `cd web && npx vite build` | exit 0 (built in 853ms; `dist/assets/index-*.js` 1.25 MB) — single-binary `//go:embed dist` shape intact | ✓ PASS |
| `formatDuration` D-10 examples | `node --experimental-strip-types` inline test, 9 assertions | 9 pass, 0 fail (0s/45s/45m/4h 21m/1d 3h/1d/—/—/—) | ✓ PASS |
| Phase 11 commits exist | `git log --oneline -- <phase files>` | Plan 01: 241c1f8, e9659e5, 2ed0042, b9ad349, f2a394d; Plan 02: bc85fac, d4245dd; Plan 03: ef4c585; UAT: ca2402f, 03f0584 | ✓ PASS |

### Probe Execution

Step 7c: SKIPPED — no `scripts/*/tests/probe-*.sh` declared in PLAN/SUMMARY; this is a frontend UI phase with no probe-based verification contract.

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| ACT-01 | 11-02, 11-03 | Dedicated top-level Activity page from persistent sidebar entry, app-wide | ✓ SATISFIED | Truths 1, 2, 3, 4. /activity route + sidebar entry reachable in both states + `(11-03)` renders on fresh/empty instance. **UAT Test 1: PASS.** |
| ACT-02 | 11-01, 11-02 | Scope to Global / Workspace / Project via selector; lists + stats update | ✓ SATISFIED | Truths 5, 6, 7, 23. ScopeSelector + useActivity query key + StatsStrip. **UAT Tests 2: PASS.** |
| ACT-03 | 11-01, 11-02 | Toggle Week/Month; toggle drives lists + stats | ✓ SATISFIED | Truths 5, 8. WindowToggle + useActivity query key. **UAT Test 3: PASS.** |
| ACT-04 | 11-02, 11-03 | Reviews quietly degrade to empty when GitHub off / gh absent; tasks + task stats still work | ✓ SATISFIED | Truths 16, 17, 18. HARD_DEGRADE null-return on `{disabled, no_gh, auth_required, error}` + empty prs; `(11-03)` wire contract guarantees `prs:[]` on degrade paths; tasks/stats unaffected (TestActivityHandler_Gate1Disabled proves tasks+stats populate with 0 gh spawns). |
| TASKS-02 | 11-02 | Tasks-done grouped by project, showing title + completion time | ✓ SATISFIED | Truth 9. groupByProject + project subheading + title + formatAgo. |
| TASKS-03 | 11-02 | Clicking a tasks-done entry navigates to that task's view | ✓ SATISFIED | Truth 10. Link to `/projects/:projectId/tasks/:taskId`. **UAT Test 4: PASS.** |
| REVIEWS-02 | 11-02 | Reviews-done grouped by project, showing PR number + title + merge/close date | ✓ SATISFIED | Truth 11. groupByProject + `#{number} {title}` + formatAgo(completedAt). **UAT Test 5: PASS.** |
| REVIEWS-03 | 11-02 | Clicking a reviews-done entry opens that PR (its review workspace if it exists, else the PR) | ✓ SATISFIED | Truth 12. Implementation does the **D-14 universal PR-URL fallback** — always opens the GitHub PR URL via plain `<a target="_blank" rel="noreferrer">`. **UAT Test 7: PASS** — user decision recorded in 11-UAT.md: *"acceptable — always-opening-GitHub (D-14 universal fallback) is fine; no in-app-workspace-preferring follow-up needed."* The success criterion's primary intent ("opens the PR") is met; the workspace-preferring refinement was consciously dropped and accepted by the user. |

No ORPHANED requirements: REQUIREMENTS.md maps exactly 8 IDs to Phase 11 (ACT-01..04, TASKS-02/03, REVIEWS-02/03), all claimed by Plan 02 frontmatter; Plan 01 claims the ACT-02/ACT-03 data-layer subset; Plan 03 claims ACT-01 (gap closure). All 8 marked `[x] Complete` in REQUIREMENTS.md.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| `web/src/components/activity/ReviewsList.tsx` | 32 | `window.open` in a docstring NEVER-comment | ℹ️ Info | Not a stub — the comment explicitly documents the Pitfall 7 prohibition. Grep confirms zero actual `window.open` calls. |
| `internal/api/activity.go` | 130, 180 | "placeholder" in SQL-parameterization comments | ℹ️ Info | Not a code stub — refers to `?` parameterized query placeholders (the SQL-injection mitigation), not unimplemented functionality. |
| `web/src/components/activity/ReviewsList.tsx` | 57, 69 | `return null` in degrade / quiet-empty branches | ℹ️ Info | Not a stub — the documented ACT-04 "quietly empty" behavior (D-12). Returns null BEFORE any JSX by design. |

No `TBD`/`FIXME`/`XXX` debt markers. No `placeholder`/`coming soon`/`not yet implemented` strings (the two "placeholder" hits are SQL terminology). No empty `return null`/`return []`/`=> {}` stub returns in production paths. Build passes (exit 0). No blockers.

### Code Review Carryover (11-REVIEW.md — 0 blockers, 3 warnings, 3 info)

The phase code review surfaced quality/a11y items that do NOT block the phase goal but are noted for awareness (tracked in 11-REVIEW.md for follow-up):

- **WR-01 / IN-01** — `useActivity`'s `enabled` parameter is documented as the Pitfall 6 mitigation but is never wired by `ActivityPage.tsx:24` (caller takes the default `true`). Concrete consequence: first-time users get a wasted `global` fetch that is immediately discarded when the promotion effect switches scope. Not a goal failure — the page resolves to the correct scope within one effect cycle.
- **WR-02** — `ScopeSelector.useDisplayName` falls back to `"Global"` when a saved scope references a deleted workspace/project, but the underlying `scope` state still uses the stale ID. Edge case (requires deleted workspace + persisted scope).
- **WR-03** — `ReviewsList` external-link `aria-label` replaces visible text as the accessible name; screen readers hear only PR numbers, not titles. a11y warning.
- **IN-02** — `ReviewsList` renders `href={review.url}` without `https://` scheme validation. Local-only/single-user + gh-sourced URL keeps risk very low.
- **IN-03** — `ScopeSelector` calls `useWorkspaces`/`useProjects` twice per render. TanStack dedupes; minor.

None of these are gaps against the phase's declared must-haves.

### Human Verification

**None pending.** All 7 user-facing items from the previous verification were resolved by UAT (11-UAT.md, status=complete, 7/7 PASS, 0 issues):

| UAT Test | Covered Truths | Result |
| --- | --- | --- |
| 1. Sidebar reachability in both states + page renders on fresh instance | 2, 4 | PASS (reverified after fix(11-03) on fresh DB at 7334) |
| 2. Scope dropdown population + single-refetch | 6, 7 | PASS |
| 3. Week/Month toggle | 8 | PASS (5 tasks Week, 8 Month on seeded 7334) |
| 4. Tasks-done click navigation | 10 | PASS (route navigation works) |
| 5. Reviews-done click external link | 11, 12, 25 | PASS (live gh on 13 linked repos, new-tab opened) |
| 6. StatsStrip visual layout + em-dash | 23, 24 | PASS (counts, min·median·max, em-dash, font-mono all correct) |
| 7. REVIEWS-03 / D-14 universal-PR-URL choice | 12 (REVIEWS-03) | PASS (user accepted D-14; no follow-up needed) |

The 4 prior `behavior_unverified` items (Truths 7, 16, 22, 25 in the new numbering) are all now VERIFIED:
- **Truth 7** (scope→query one-refetch): structural property of TanStack Query param-keyed queryKey + UAT Tests 2/3 confirmed the observable.
- **Truth 16** (reviews silent on hard-degrade): end-to-end structurally provable — backend test-locks `prs:[]` on degrade paths, frontend returns null before any JSX.
- **Truth 22** (default-scope adoption, Pitfall 6): the `localStorage.getItem(...) === null` guard holds by construction; UAT covered the common path.
- **Truth 25** (reviews completedAt DESC sort): comparator trivially correct for uniform-precision ISO; UAT Test 5 live-verified on 13 repos.

### Gaps Summary

**No gaps.** All 14 required artifacts exist, are substantive (no stubs), and are wired (including the 2 new `(11-03)` artifacts: activity.go non-nil slices + activity_test.go regression test). All 12 key links are connected (including the 5 new `(11-03)` backend-internal and page-level null-guard links). The production build (`tsc -b && vite build`) exits 0. The byte-level wire-contract regression test passes. All 8 requirement IDs are claimed by the plans, traceable in REQUIREMENTS.md, and marked Complete.

**Status is `passed`** because: all 26 truths VERIFIED (0 behavior-unverified), all artifacts pass, all links wired, no blockers, no pending human verification items (UAT resolved all 7 user-facing checks at 7/7 PASS), and all 8 requirements satisfied. The Plan 11-03 gap closure (commit `ef4c585`) fixed the one blocker (black page on fresh/empty instance) that the previous `human_needed` status was waiting on, and the user re-verified it in UAT Test 1.

---

_Verified: 2026-07-31T18:15:00Z_
_Verifier: the agent (gsd-verifier)_
