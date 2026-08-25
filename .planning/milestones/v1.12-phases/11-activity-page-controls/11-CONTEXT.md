# Phase 11: Activity Page & Controls - Context

**Gathered:** 2026-07-31
**Status:** Ready for planning

<domain>
## Phase Boundary

The **frontend-only** Activity page for v1.12 Activity & Statistics. This phase ships the UI; every byte of data + the API already landed in Phase 10 (`GET /api/activity`). No backend work, no migration.

Delivers:
1. A **dedicated top-level Activity page** reachable from a persistent sidebar entry (like Settings), accessible app-wide.
2. A **scope selector** (Global / a Workspace / a Project) and a **Week / Month toggle** that drive both the lists and the statistics via the one combined `GET /api/activity?scope=…&window=…` query.
3. **Tasks-done list** — grouped by project, each entry showing the task title + completion time; clicking navigates to that task's view.
4. **Reviews-done list** — grouped by project, each entry showing `#N title` + merge/close date; clicking opens the PR on GitHub.
5. **Statistics** — counts (tasks done, reviews done) + min/max/median cycle (In-Progress→Done) + per-column dwell, rendered as numeric readouts (no charts).
6. **Graceful degradation** — when GitHub integration is off / `gh` absent, the reviews section renders quietly empty while tasks-done and task statistics display normally (ACT-04 / REVIEWS-04, decided at the contract level in Phase 10 D-02).

**Built on existing foundations (no new API):**
- Phase 10's combined `GET /api/activity` → `{tasks:[…], reviews:{state,stale,fetchedAt,prs:[…]}, stats:{taskCount, reviewCount, cycle, dwellInProgress, dwellInReview}}`. One endpoint → **one TanStack query, one loading state** (Phase 10 D-01).
- `reviews.state` (`ok`/`no_gh`/`disabled`/`error`/`partial`) carries degradation; the whole response is always HTTP 200 (Phase 10 D-02).
- `useProjects` / `useWorkspaces` (already on the wire with `workspace_id` / `is_default`) feed the scope selector.
- The Settings page (`SettingsPage.tsx`) is the structural template for a top-level page; the sidebar footer (next to the Settings gear) is the entry point.

**Out of this phase (per REQUIREMENTS.md Out of Scope):** custom date ranges / "All time" (ACTFUT-01), charts/graphs beyond numeric readouts (ACTFUT-02), CSV/JSON export (ACTFUT-03), per-agent/workspace breakdown (ACTFUT-04), events beyond done/merged (ACTFUT-05), time stats for reviews (STATS-04 — permanently out), multi-user/team stats.

Requirements covered: **ACT-01, ACT-02, ACT-03, ACT-04, TASKS-02, TASKS-03, REVIEWS-02, REVIEWS-03** (TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01..04 shipped complete in Phase 10).

</domain>

<decisions>
## Implementation Decisions

### Sidebar entry & collapsed-rail behavior (ACT-01)

- **D-01:** The Activity entry lives in the **sidebar footer, next to the Settings gear** — both are app-level destinations (not workspace-scoped), so they pair naturally. Today the footer holds "Add project" + the Settings gear (`ProjectSidebar.tsx` `SidebarFooter`); Activity joins them there as an icon-button `Link` to `/activity`, with its active state derived from `location.pathname === "/activity"` exactly the way the Settings gear does it.
- **D-02:** Unlike the Settings gear, **Activity renders as an icon in the collapsed icon rail** — it stays reachable with the sidebar collapsed, not hidden. The footer today self-hides wholesale via `group-data-[collapsible=icon]:hidden`; Activity must NOT ride that hide. The cleanest structure is an icon-button that is visible in BOTH states (expanded footer + collapsed rail), reusing the same `Tooltip` + active-state treatment the project rows and Settings gear use. (Implementation detail — whether that's one button rendered outside the footer's hide wrapper, or two surfaces — is the agent's call, but the *behavior* is locked: always reachable.)
- **D-03:** The icon is **lucide `Activity`** (the pulse/heartbeat line) — the natural semantic match for an activity feed. Import from `lucide-react` alongside the existing `Settings`/`Plus` imports in `ProjectSidebar.tsx`.

### Scope selector control (ACT-02, ACT-03)

- **D-04:** The scope is a **single dropdown** (one menu), shaped like the existing workspace switcher (`WorkspaceSwitcher.tsx` + `dropdown-menu.tsx`). It lists "Global", every workspace, and each project under its workspace. One control, one selection → maps 1:1 to the `scope` query param (`global` / `workspace:N` / `project:N`). Avoids a two-control (mode + target) UI. Reuse the `DropdownMenu` + selected-✓ + section/label idioms already in the switcher.
- **D-05:** **Default scope on open = the active workspace.** Activity follows the workspace context the sidebar is filtered to (`useActiveWorkspace()`). If no active workspace is resolved yet, fall back to Global. (Global is reachable explicitly from the dropdown; it's just not the default.)
- **D-06:** **Scope + window persist in `localStorage` (no URL).** Use the `kamacu.*` prefix convention (`migrateStorage.ts`); reload returns the user's last scope + window. Activity is **never deep-linkable** — consistent with how workspaces work (workspace never enters the URL either; Phase 26 D-14). The route is a bare `/activity` with no query params. (Baseline: a stale/missing saved value falls back to the default scope = active workspace, and window falls back to `week`.)
- **D-07:** The Week / Month window is a **segmented control** (Week | Month) — a binary switch reads naturally as a segmented toggle. Sits next to the scope dropdown in a page-level control bar. Maps to `window=week|month`.

### Page layout & statistics presentation (STATS-01..04)

- **D-08:** **Stats strip on top, lists below (stacked, single column).** A compact stats strip across the top of the page (counts + cycle/dwell readouts), then the tasks-done section and the reviews-done section stacked beneath it. Follows the `SettingsPage.tsx` single-column-scroll shell (`h-full overflow-y-auto p-4`, a bounded `max-w` column). Lists are NOT side-by-side.
- **D-09:** **Cycle/dwell render as compact rows: `min · median · max`.** Each time metric (Cycle, In-Progress dwell, In-Review dwell) is one line showing `min · median · max` with the count beside it ("over N tasks"), sourced from each `timeStat {n, min, max, median}` on the wire (Phase 10 D-11). Counts (`stats.taskCount`, `stats.reviewCount`) render as the lead numbers. No cards grid, no charts (Out of Scope ACTFUT-02). STATS-04 honored: reviews contribute `reviewCount` only, never a time-stat row.
- **D-10:** **Durations format adaptively to the largest 1–2 non-zero units** (d/h/m), e.g. `1d 3h`, `4h 20m`, `45m`, `30s`. Durations arrive on the wire as **seconds** (`timeStat.min/max/median` are `int64` seconds). One shared formatter (a small util) normalizes every stat readout.
- **D-11:** **When `n === 0` (no tasks done in-window), cycle/dwell show an em-dash `—`** (a muted "no data"), NOT `0m`/`0h`. Counts show `0`. Keeps the layout stable without implying a measured zero.

### Reviews section & click-through (ACT-04, REVIEWS-02, REVIEWS-03, TASKS-02, TASKS-03)

- **D-12:** **When GitHub integration is off / `gh` absent (`reviews.state === "disabled"` / `"no_gh"`), the reviews section renders QUIETLY EMPTY — no error, no hint.** The most literal reading of ACT-04's "quietly degrades to empty." Tasks-done and task statistics display normally. (For `"partial"`/`"error"` states, the successful repos' PRs render and the section otherwise stays quiet — consistent with the no-hint choice; the degrade rides in `reviews.state` + the `prs` array as designed in Phase 10 D-02/D-08.)
- **D-13:** **Both lists are grouped by project under a project-name subheading** (TASKS-02 / REVIEWS-02). The activity endpoint already annotates every entry with project provenance (`activityTask.projectName`/`projectId`; `ReviewDoneSummary.projectName`/`projectId`/`repo`), so grouping is a client-side bucketing pass over each list. Order entries within a group by recency (tasks: `doneAt` DESC — the endpoint already returns them this way; reviews: `completedAt` DESC).
- **D-14 (deliberate simplification — flag at UAT):** **Clicking a reviews-done entry opens the PR on GitHub (`ReviewDoneSummary.url`) — universally, as an external link.** REVIEWS-03 reads "opens the PR (its review workspace if it exists, else the PR)"; this decision implements the **`else the PR` fallback universally** and does NOT spend a cross-project task lookup to prefer an in-app review workspace. Rationale: resolving "review workspace exists" requires fetching `source='github_pr'` tasks across in-scope projects on click — heavyweight for an occasional view, and the in-app review workspace remains reachable from the board / review column as today. The success criterion's primary intent ("opens the PR") is satisfied; the workspace-preferring refinement is a conscious omission. (If UAT insists on the workspace preference, the resolver in D-15 is the upgrade path.)
- **D-15:** **Clicking a tasks-done entry navigates to that task's view** (`/projects/:projectId/tasks/:taskId`) — TASKS-03. `activityTask` carries both `id` and `projectId`, so this is a plain `Link`/`navigate`. No lookup needed.

### the agent's Discretion

- **Route registration** — `/activity` is a new top-level route under `<AppLayout />` in `App.tsx` (a sibling of `/settings`, NOT under `/projects/:projectId` so it needs no project in the URL). A bare `/activity`, no params (scope/window are localStorage per D-06).
- **New `ActivityPage.tsx`** under `web/src/pages/` mirroring the `SettingsPage.tsx` shell (loading skeletons, error + retry, bounded column).
- **New `useActivity(scope, window)` TanStack query** in `web/src/api/queries.ts` (or a dedicated `web/src/api/activity.ts`) with a `queryKey` of `["activity", scope, window]`; the activity wire types go in `web/src/api/types.ts`. One query, one loading state (Phase 10 D-01).
- **The scope/window UI state hook** — where the persisted scope + window live (a small `localStorage`-backed hook, like `useActiveWorkspace`) and how it's shared between the dropdown/segmented controls and the query. Follow existing patterns; `zustand` is available but the app favors raw `localStorage` hooks.
- **The collapsed-rail icon structure** (D-02) — one button rendered outside the footer's `group-data-[collapsible=icon]:hidden` wrapper vs. two surfaces; the agent picks the cleaner structure as long as Activity is reachable in both states.
- **Exact copy** (section headings, "over N tasks" phrasing, tooltip text), the `max-w` column width, and whether the control bar (scope dropdown + window segmented control) is sticky — follow existing visual language; no user preference expressed beyond the decisions above.
- **The duration formatter's exact unit labels** (`m` vs `min`, `h` vs `hr`) and rounding — pick a compact, consistent style.
- **Grouping ordering** — projects within a list ordered by name (matches the sidebar) or by most-recent-activity; agent picks the sensible default.

### Folded Todos

None — `todo.match-phase` returned no matches for Phase 11.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 11: Activity Page & Controls" — goal + the 5 success criteria (sidebar entry, scope selector, Week/Month toggle, grouped lists + click-through, reviews-empty-when-off).
- `.planning/REQUIREMENTS.md` § "v1.12 Requirements" — **ACT-01..04, TASKS-02/03, REVIEWS-02/03** (the locked Phase 11 requirements), the **Out of Scope** table (no custom date ranges, no charts, no time stats for reviews, no export, no cross-forge, no events beyond done/merged, no multi-user), and **ACTFUT-01..05** (deferred).

### Phase 10 backend — the contract this page consumes (READ FIRST)
- `.planning/phases/10-activity-data-api/10-CONTEXT.md` — the API/data decisions this page renders. Especially **D-01** (one combined endpoint → one query), **D-02** (`reviews.state` degrade, always 200), **D-11** (stats shape with `n` per stat), and the degrade-don't-break contract.
- `internal/api/activity.go` — `activityResponse { tasks, reviews, stats }` (the wire shape) + `aggregateReviewsResult { state, stale, fetchedAt, prs }`.
- `internal/api/activity_helpers.go:11-34` — `activityTask` (`id, title, doneAt, projectId, projectName`), `timeStat` (`n, min, max, median` int64 seconds), `statsBlock` (`taskCount, reviewCount, cycle, dwellInProgress, dwellInReview`).
- `internal/api/activity_helpers.go:51-80` — `parseScope` / window parsing: `scope=global|workspace:N|project:N`, `window=week|month`. The exact param strings the frontend must send.
- `internal/github/prlist.go:324-332` — `ReviewDoneSummary` (`number, title, completedAt, url, projectName, projectId, repo`) — the reviews-done entry shape (note `url` is the GitHub PR URL used by D-14).

### The page shell + sidebar entry templates (mirror these)
- `web/src/pages/SettingsPage.tsx` — **the top-level-page template**: `h-full overflow-y-auto p-4` + bounded `max-w` column, loading skeletons, error+retry (`Couldn't load … / Retry loading`). ActivityPage follows this shell.
- `web/src/components/sidebar/ProjectSidebar.tsx` `SidebarFooter` (lines ~162-192) — **where Activity mounts** (next to the Settings gear). The Settings gear's active-state idiom (`location.pathname === "/settings"`) + `Tooltip` is the template for Activity's entry. NOTE the footer's `group-data-[collapsible=icon]:hidden` — D-02 says Activity must NOT be hidden by it.
- `web/src/components/sidebar/WorkspaceSwitcher.tsx` — **the scope-dropdown template** (D-04): a `dropdown-menu` with sections, selected-✓, and the expanded-only idiom. The scope selector mirrors this shape (just a different list: Global + workspaces + projects).
- `web/src/App.tsx:114-141` — **route registration**: `<Route element={<AppLayout />}>` wraps the pages; `/settings` is a bare sibling route. `/activity` registers the same way (no `:projectId` in the path).

### Frontend data layer + state patterns
- `web/src/api/queries.ts` — `useProjects`, `useWorkspaces`, `useGithubStatus` — the query patterns + `queryKey` conventions a new `useActivity(scope, window)` follows.
- `web/src/api/types.ts` — `Project`, `Workspace`, `Task` types; the activity wire types (`activityTask`, `ReviewDoneSummary`, `statsBlock`/`timeStat`) get added here.
- `web/src/api/client.ts` — `get`/`ApiError` (the endpoint is always 200, so errors are transport-level only).
- `web/src/lib/useActiveWorkspace.ts` — **the default-scope source** (D-05) AND the `localStorage`-backed-hook template for the persisted scope + window (D-06).
- `web/src/lib/migrateStorage.ts` — the `kamacu.*` localStorage prefix convention for the new scope/window keys (D-06).
- `web/src/components/ui/dropdown-menu.tsx` — the scope dropdown primitives (submenus, radio groups, labels all already exported — Phase 26 confirmed).
- `web/src/components/ui/` — `Skeleton`, `Button`, `Tooltip`, and the segmented-control surface (a `Tabs`/toggle or a small bespoke segmented control — agent picks from the existing UI set).

### Established project patterns (constraints, not files)
- **No toast/sonner library** — all feedback (degrade states, errors) is inline / muted / dialog, never a toast (confirmed in Phase 26 specifics).
- **Degrade rides in `reviews.state`, always 200** — the page branches on `reviews.state`; the reviews section empties when degraded (D-12), the rest of the page is unaffected.
- **localStorage over URL for view state** — workspaces don't enter the URL; Activity scope/window follow suit (D-06).
- **Sidebar active state via `location.pathname`** — the Settings gear idiom; Activity reuses it.

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`SettingsPage.tsx`** — near-verbatim page shell (scroll container, bounded column, loading skeletons, error+retry) for `ActivityPage.tsx`.
- **`WorkspaceSwitcher.tsx` + `dropdown-menu.tsx`** — the scope-dropdown template (D-04): sectioned menu, selected marker, expanded-only rendering.
- **`useProjects()` + `useWorkspaces()`** — already carry `workspace_id` / `is_default`; feed the scope dropdown's workspace+project list with zero new fetches.
- **`useActiveWorkspace()`** — the default-scope source (D-05) and the `localStorage`-hook template for the persisted scope/window (D-06).
- **`ProjectSidebar.tsx` `SidebarFooter`** — the mount point + active-state idiom for the Activity entry (D-01).
- **`get` + `ApiError`** (`client.ts`) — the fetch primitive (the endpoint is always 200).
- **`Skeleton`, `Button`, `Tooltip`, `Link` (react-router)** — the UI primitives the page + entry compose from.

### Established Patterns
- **One endpoint → one TanStack query, one loading state** (Phase 10 D-01) — `useActivity(scope, window)` with `queryKey: ["activity", scope, window]`; switching scope/window just changes the key.
- **Top-level page as a bare route under `<AppLayout />`** — `/settings` today; `/activity` the same way (App.tsx:135).
- **`location.pathname` active state** for sidebar entries (Settings gear).
- **`kamacu.*` localStorage persistence** for view state (sidebar-open, active-workspace); scope/window join this (D-06).
- **Grouped-by-project rendering** — client-side bucketing over a list already annotated with project provenance (the endpoint does the annotation; the page buckets).
- **Degrade-don't-break** — branch on `reviews.state`; never let the reviews half break the tasks/stats half (Phase 10 D-02 / REVIEWS-04).
- **No toasts** — inline/muted/dialog feedback only.

### Integration Points
- New route `/activity` in `web/src/App.tsx` (bare, under `<AppLayout />`, sibling of `/settings`).
- New `ActivityPage.tsx` in `web/src/pages/`.
- New Activity entry in `ProjectSidebar.tsx` `SidebarFooter` (next to Settings gear; icon-visible in collapsed rail per D-02).
- New scope/window UI (dropdown + segmented control) + a `localStorage`-backed state hook (likely `web/src/lib/useActivityView.ts` or similar) consumed by both the controls and the query.
- New `useActivity(scope, window)` query + activity wire types in `web/src/api/`.
- Tasks-done click → `navigate(/projects/:projectId/tasks/:taskId)` (D-15); reviews-done click → external `window.open(url)` / `<a href={url}>` (D-14).

</code_context>

<specifics>
## Specific Ideas

- **This is a frontend-only phase.** Phase 10 ships the API; this phase renders it. No Go, no SQL, no migration.
- **D-14 is a deliberate simplification worth flagging at UAT.** REVIEWS-03's "review workspace if it exists, else the PR" is implemented as "always the PR URL." The success criterion's primary intent ("opens the PR") is met; the workspace-preferring refinement is consciously dropped to avoid cross-project task lookups on click. If verification rejects this, the resolver (find a `source='github_pr'` task matching `pr_number` in the entry's project) is the upgrade path — but the user explicitly chose the universal-PR-link behavior.
- **Scope follows the workspace.** Opening Activity defaults to the active workspace (D-05) — the page feels contextual to where you're working, not a detached global report. Global is one dropdown pick away.
- **Stats are numbers, not pictures.** min·median·max rows (D-09) + adaptive d/h/m (D-10) + em-dash on empty (D-11). The Out-of-Scope bar on charts (ACTFUT-02) is respected by construction.
- **The collapsed-rail reachability (D-02) is the one structural wrinkle** vs. the Settings gear — Activity must stay clickable when the sidebar is collapsed, which the footer's current wholesale `group-data-[collapsible=icon]:hidden` would defeat. The behavior is locked; the exact DOM structure is the agent's call.

</specifics>

<deferred>
## Deferred Ideas

- **Preferring the in-app review workspace over the PR URL on click** (D-14 simplification) — a future enhancement could resolve a matching `source='github_pr'` task and navigate there instead of opening GitHub. Not required by the user's choice for v1.12.
- **Custom date ranges / "All time" preset** — ACTFUT-01. Week/Month presets are enough for v1.12.
- **Throughput trend charts / bar graphs** — ACTFUT-02. v1.12 shows min/max/median as numbers only.
- **CSV/JSON export of activity data** — ACTFUT-03.
- **Per-agent or per-workspace breakdown statistics** — ACTFUT-04.
- **Activity for events beyond done/merged** (tasks moved to In Review, sessions started) — ACTFUT-05.
- **Deep-linkable scope/window via URL** — D-06 chose localStorage-only for v1.12 (consistent with workspaces). URL params are a future option if sharing/bookmarking a specific Activity view becomes valuable.
- **Time stats for reviews** — permanently out of scope (STATS-04: GitHub's merge signal has no Kamacu in-progress/in-review timestamps).

None of these were pulled into Phase 11 — discussion stayed within the Activity Page & Controls boundary.

</deferred>

---

*Phase: 11-activity-page-controls*
*Context gathered: 2026-07-31*
