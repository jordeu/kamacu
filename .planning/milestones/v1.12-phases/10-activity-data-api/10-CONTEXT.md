# Phase 10: Activity Data & API - Context

**Gathered:** 2026-07-29
**Status:** Ready for planning

<domain>
## Phase Boundary

The **backend-only data + API layer** for v1.12 Activity & Statistics. Nothing user-visible ships in this phase (the Activity page is Phase 11); this phase delivers the endpoints Phase 11 renders.

Delivers:
1. A **`GET /api/activity`** endpoint answering "which tasks were done, which reviews were completed (merged/closed), and what are the cycle/dwell stats" for any **Global / Workspace:N / Project:N** scope and a **Week (7d) / Month (30d)** window.
2. **Tasks-done list** — manual tasks moved to Done within the window+scope, each with project name, title, and `done_at`.
3. **Reviews-done list** — PRs `reviewed-by:@me` that MERGED or CLOSED within the window+scope, each with PR number, title, completedAt (merge/close date), and project provenance — backed by a **NEW** `gh` search dimension in `internal/github`.
4. **Statistics** — counts (tasks done, reviews done) + min/max/median cycle time (In Progress→Done) + per-column dwell (time in In Progress, time in In Review) for tasks done in the window.
5. **Graceful degradation** — when `gh` is unavailable / GitHub integration off / individual repo fetch fails, the reviews-done portion degrades without breaking the tasks-done list or task statistics (REVIEWS-04).

**Built on existing foundations (no migration needed):**
- Per-status timestamps (`todo_at`/`in_progress_at`/`in_review_at`/`done_at`) — migration `00006`, stamped on each move (last-entry-wins, leaving never clears).
- `internal/github` package + per-repo cache from v1.3/v1.5.
- The two-gate ladder (GHSET-02: settings toggle + project linked).

**Out of this phase (all Phase 11):** the Activity page UI, sidebar entry, scope selector, Week/Month toggle, grouping rendering, click-through navigation. Also out (per REQUIREMENTS.md Out of Scope): custom date ranges (ACTFUT-01), charts/graphs (ACTFUT-02), CSV/JSON export (ACTFUT-03), per-agent/workspace breakdown (ACTFUT-04), events beyond done/merged (ACTFUT-05), time stats for reviews (STATS-04 — GitHub merge has no Kamacu in-progress/in-review timestamps).

Requirements covered: **TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01, STATS-02, STATS-03, STATS-04**.

</domain>

<decisions>
## Implementation Decisions

### Endpoint shape (TASKS-01, REVIEWS-01, STATS-01)

- **D-01:** **One combined `GET /api/activity` endpoint.** Query params: `?scope=global|workspace:N|project:N` (single mutually-exclusive scope param; `global` default when absent) and `?window=week|month` (default `week`; 7d / 30d rolling from now). Returns `{tasks:[...], reviews:{state, stale, fetchedAt, prs:[...]}, stats:{...}}`. Phase 11 renders all three on one page → one TanStack query, one loading state. A single scope param mirrors how the frontend thinks about it (a 3-mode selector) and makes "exactly one scope active" explicit.

- **D-02:** **Reviews sub-object degrades via a `state` field; the WHOLE response is always HTTP 200.** The `reviews` object mirrors `github.Result` exactly: `{state: "ok"|"no_gh"|"disabled"|"error"|"partial", stale, fetchedAt, prs:[...]}`. Tasks + stats always populate regardless of gh state. Phase 11 branches on `reviews.state` (empty `prs` when degraded). This is the established degrade contract (GHSET-03 / GHCOL-05) applied to the activity endpoint — consistent with the review column. When the global GitHub toggle is off (GATE 1), `reviews.state="disabled"` and NO `gh` spawns; tasks + stats return normally (ACT-04 / REVIEWS-04).

### Reviews-done fetch & cache (REVIEWS-01, REVIEWS-04) — the load-bearing area

- **D-03:** **A NEW separate cache + method on `*github.Service`** (e.g. `GetMergedClosed(ctx, repo, repoDir, force)`) with its OWN per-repo cache entry + TTL. This does NOT touch the review-column cache (the existing `repoEntry.cachedAwaiting`/`cachedReviewed` 60s-TTL path driven by the 5s poll). Rationale: the review column is a hot path (5s poll, 60s TTL, 2 `--state open` searches); coupling a third merged/closed search onto it would 3x the `gh` load on that hot path for data the review column never uses. The activity page has a different access pattern (occasional views, broader scope) that justifies its own cache. The new method reuses the existing `Service` infrastructure (demand-driven, in-flight dedup, drop-cache-after-N-failures, the `no_gh`/`auth_required`/`error` classification from `listPRs`).

- **D-04:** **5min TTL** for the new merged/closed reviews cache. The activity page is an occasional "check what happened this week/month" view, not a live dashboard; merged/closed PRs don't change state. 5min cuts `gh` load 5x vs 60s while staying fresh-feeling. A `?refresh=1` (or equivalent) bypasses the TTL for manual freshness when the user wants it. (Exact TTL constant + the attempt-floor value are the agent's discretion — follow the `cacheTTL`/`attemptFloor` shape from `service.go:86-90`.)

- **D-05:** **Treat merged and closed as one "completed" state.** REVIEWS-02 shows "PR number, title, and merge/close date" without distinguishing. One `gh` search — `reviewed-by:@me --state closed` (`--state closed` includes merged in `gh`) — covers both, with a single `completedAt` date field. Simpler wire, fewer `gh` calls. (If `gh --state closed` semantics need verification, the researcher confirms the exact flag that returns both merged+closed and which JSON date field maps to `completedAt` — likely `closedAt`.)

### PR wire type for reviews-done (REVIEWS-01/02)

- **D-06:** **A dedicated `ReviewDoneSummary` type** carrying only what the activity list needs: `number, title, completedAt, url`, and **project provenance** (`projectName`, `projectId`, and/or `repo`) for grouping (REVIEWS-02: "grouped by project"). Keeps `PRSummary` (the review-column wire type) untouched and focused. The handler maps the per-repo `gh` result into this type at aggregation time, annotating the project/repo it queried (`PRSummary` has no repo/project field, so provenance is added by the aggregation loop that knows which project each repo belongs to).

### Cross-project aggregation (REVIEWS-01, REVIEWS-04)

- **D-07:** **Concurrent, bounded aggregation across in-scope repos.** Query the in-scope linked projects (pure SQL with the scope filter), then fire one `GetMergedClosed` per repo **concurrently** via a bounded `errgroup` (e.g. 5-wide). Each repo's call is cache-served most of the time (5min TTL), so concurrency rarely hits live `gh`. A failing/slow repo returns its own degraded state and does NOT block the others (per-repo context timeout/degrade within the errgroup). Fastest wall-clock for global/workspace scope.

- **D-08:** **Aggregate `reviews.state` = `ok` / `partial` / degraded.** `ok` if ALL in-scope repos succeeded; `partial` if some succeeded and some degraded (the `prs` array carries the successful ones); `no_gh`/`disabled`/`error` only if ALL repos degraded (or the global GATE 1 blocks). Phase 11 shows the list + a quiet "some repos unavailable" hint on `partial`. REVIEWS-04 satisfied — successful repos always surface; a failing repo never blocks the rest (nor the tasks list nor the stats).

### Statistics (STATS-01..04)

- **D-09:** **The rolling week/month window naturally filters pre-00006 rows** — those rows are months old (done before migration `00006` landed) and fall outside the 7/30-day window. In practice, every row in-window has both timestamps present. No special NULL-handling code path is needed beyond the window filter on `done_at`; the stats compute over whatever the window returns. (Defensive note: if a row somehow lands in-window without timestamps — e.g. a status-transition bug — it contributes to `taskCount` but is excluded from the time-stat slice; `time.Parse` failure on a NULL is the natural exclude. Not a separate code path, just don't panic on it.)

- **D-10:** **Compute cycle/dwell in Go, not SQL.** Fetch the in-window done rows with their `*_at` timestamps, parse with `time.Parse`, compute durations via `Sub`, then min/max/median over the duration slice. **Median** = sort durations ascending; middle element (odd N) or average of the two middle elements (even N). SQLite `julianday` math on ISO strings is fiddly and SQLite has no native `MEDIAN` (needs `percentile_cont` workaround); Go is cleaner and N is small (a week/month of done tasks). Durations surface as **seconds** (Phase 11 formats to human-friendly).

- **D-11:** **Stats object shape:** `stats: {taskCount, reviewCount, cycle: {n, min, max, median}, dwellInProgress: {n, min, max, median}, dwellInReview: {n, min, max, median}}`. Each time-stat carries its own `n` (the count of rows it was computed over) so Phase 11 can show "cycle time over N tasks". `reviewCount` is a plain number (**STATS-04**: reviews contribute a count, never time stats — GitHub's merge signal has no Kamacu in-progress/in-review timestamps). Durations in seconds; min/max/median are all seconds. `taskCount` is the total tasks-done count in-window (STATS-01); the `cycle.n` may be ≤ `taskCount` if any in-window row lacked timestamps (defensive, see D-09).

### Tasks-done scope (TASKS-01)

- **D-12:** **Tasks-done list = `source='manual'` only.** PR-review workspaces are also task rows (`source='github_pr'`) that can be moved to Done, but REVIEWS-01 covers PR reviews separately via `gh`; including `github_pr` rows in tasks-done would double-count review work. The tasks-done query filters `WHERE status='done' AND source='manual' AND done_at IS NOT NULL AND done_at >= <window-start>` joined to `projects` (for project name) with the scope filter (`project_id=N` / `projects.workspace_id=N` / no filter for global), ordered by `done_at DESC`. (Board-leak filter (`source='manual'`) is already the established pattern in `tasks.go` list queries.)

### the agent's Discretion

- **Exact `gh` flag/date verification for the merged/closed search** — confirm `--state closed` returns both merged+closed, and which JSON date field (`closedAt` vs `mergedAt`) maps to `completedAt`. The researcher verifies live against `gh` (this is the one genuinely open technical question — see Specifics).
- **The new method/cache-entry naming** in `internal/github` (e.g. `GetMergedClosed` + `mergedClosedEntry`, or a more generic name), the `ReviewDoneSummary` struct field names/order, and whether the new cache entry reuses the existing `repoEntry` shape (with a new field) or a separate map — follow existing patterns; the separation (D-03) is locked, the naming is not.
- **The errgroup bound value** (5-wide is a suggestion), the per-repo timeout within the aggregate, and whether `?refresh=1` propagates to every repo's `force` flag — agent's call, follow the existing `?refresh=1` precedent.
- **Exact stats struct Go types** (int64 seconds vs float64 vs `time.Duration` on the wire) — surface as seconds (integer or float), agent picks the JSON shape.
- **File organization** — new `internal/api/activity.go` (handler) + extend `internal/github` (new method/cache); new `ActivityRoutes(mux, db, ghSvc)` in `serve.go` alongside `PullRequestRoutes`. Agent confirms placement.
- **Whether the tasks-done query reuses `taskColumns`/`scanTask` or a dedicated scan** — `taskColumns` does NOT include the `*_at` columns, so either extend it (ripples to every task query) or add a dedicated `activityTaskColumns`/`scanActivityTask`. Dedicated is cleaner (the `*_at` columns aren't needed elsewhere); agent decides.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Requirements & roadmap (scope anchors)
- `.planning/ROADMAP.md` § "Phase 10: Activity Data & API" — goal + the 5 success criteria (the endpoint contract, the merged/closed reviewed-by:@me search, degrade-don't-break, counts, cycle/dwell stats).
- `.planning/REQUIREMENTS.md` § "v1.12 Requirements" — **TASKS-01, REVIEWS-01, REVIEWS-04, STATS-01, STATS-02, STATS-03, STATS-04** (locked), the **Out of Scope** table (no custom date ranges, no charts, no time stats for reviews, no export, no cross-forge, no events beyond done/merged, no multi-user), and **ACTFUT-01..05** (deferred).

### Per-status timestamps (the stats foundation — READ FIRST)
- `internal/store/migrations/00006_task_status_timestamps.sql` — the schema: `tasks.todo_at`/`in_progress_at`/`in_review_at`/`done_at` (all nullable TEXT, millisecond ISO-8601). `done_at` backfilled from `updated_at` for pre-migration Done rows; the other three stay NULL for those rows (D-09 relies on the window filtering these out).
- `internal/api/tasks.go:486-495` — the `statusAtCol` map + the `UPDATE tasks SET ... <status>_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')` move logic (last-entry-wins; leaving a status never clears its column).
- `internal/reaper/reaper.go:158-166` — confirms `done_at` is stored as millisecond ISO-8601 and parsed with the strftime format; the activity endpoint's date math follows the same parse pattern.

### The v1.3 / v1.5 GitHub surface (the code Phase 10 EXTENDS — the load-bearing area)
- `internal/github/service.go:19-25` `github.Result` — **the wire contract D-02 mirrors.** Fields: `State` (`ok`|`no_gh`|`auth_required`|`disabled`|`error`), `Stale`, `FetchedAt`, `PRs`, `Reviewed`. Always HTTP 200 on the endpoint; degradation rides in `State`.
- `internal/github/service.go:34-43` `repoEntry` + `:86-90` (`cacheTTL=60s`, `attemptFloor=10s`, `maxFailures=3`) — the per-repo cache pattern. D-03/D-04 add a NEW separate entry/method with a 5min TTL; do NOT couple onto this hot-path entry.
- `internal/github/service.go:61-146` `Service` + `Get` — the demand-driven cache + in-flight dedup + drop-cache-after-N template. The new `GetMergedClosed` (D-03) mirrors this shape with its own cache + TTL.
- `internal/github/prlist.go:194-197` `searchAwaiting` / `searchReviewed` (`user-review-requested:@me draft:false` / `reviewed-by:@me draft:false`) — the two existing `--state open` searches. D-05 adds the NEW `reviewed-by:@me --state closed` dimension.
- `internal/github/prlist.go:212-276` `listPRs` — the single I/O primitive: shells out to `gh pr list -R repo --search <search> --state <state> --json ...`, classifies outcome into `state` (`ok`/`no_gh`/`auth_required`/`error`), reduces checks, sorts by updatedAt. D-05's new search reuses this (parameterize `--state`); the researcher verifies the exact `--state closed` semantics + the date field.
- `internal/github/prlist.go:22-34` `PRSummary` — the existing per-PR wire type. D-06 does NOT extend it; a dedicated `ReviewDoneSummary` carries the activity-list shape (with provenance).
- `internal/github/prlist.go:283-304` `PRLists` + `fetchLists` — the combined runner. The new merged/closed fetch is a SEPARATE runner (D-03), NOT added to `fetchLists` (which drives the review column).

### Endpoint / handler patterns (the code Phase 10 mirrors)
- `internal/api/routes.go:18-47` `Routes` — the core route registration. Phase 10 either adds inside `Routes()` or as a new `ActivityRoutes(mux, db, ghSvc)` sibling (like `PullRequestRoutes`).
- `internal/api/pullrequests.go:30-88` `PullRequestRoutes` GET handler — **the degrade template:** GATE 1 (settings toggle off → `state:"disabled"`), GATE 2 (project unlinked → `state:"disabled"`), then `svc.Get(...)`. Always 200. The activity endpoint's global GATE 1 + per-repo degradation (D-02/D-08) follows this shape.
- `internal/api/pullrequests.go:46` signature `PullRequestRoutes(mux, db, svc, wtSvc)` — the new `ActivityRoutes(mux, db, ghSvc)` mirrors this (no `wtSvc` needed — activity is read-only, no worktree provisioning).
- `internal/api/respond.go` — `writeJSON(w, status, v)` + `writeError(w, status, msg)`.
- `internal/api/tasks.go:54-68` `taskHandlers{db,...}` + `taskColumns` + `scanTask` — the handler-struct + columns-const + scan-helper template. NOTE: `taskColumns` does NOT include the `*_at` columns (D-12 / discretion).
- `internal/api/tasks.go:194,233` — the `WHERE source = 'manual'` board-leak filter (D-12 reuses it for tasks-done).
- `cmd/kamacu/serve.go:268-269` `ghSvc := github.New(...)` + `api.PullRequestRoutes(mux, db, ghSvc, wtSvc)` — **the ONE shared `*github.Service`.** The new `ActivityRoutes` receives the SAME `ghSvc` (D-03's new method lives on it).

### Established project patterns (constraints, not files)
- **Single-writer SQLite** (`internal/store/store.go`) — `SetMaxOpenConns(1)`, parameterized `?` SQL, collect-then-update if a SELECT cursor is involved. The tasks-done + scope queries follow this.
- **Two-gate ladder** (GHSET-02) — settings toggle (GATE 1) + project linked (GATE 2) before any `gh` spawn. The activity endpoint runs GATE 1 globally (toggle off → `reviews.state="disabled"`, no spawns); GATE 2 is implicit (only linked projects are queried for the reviews loop).
- **Degrade-don't-break** — GitHub always returns 200 with `State` field; never blocks the rest of the page (GHSET-03/GHCOL-05/REVIEWS-04).
- **ISO-8601 string timestamps sort lexically == chronologically** — the window filter (`done_at >= <window-start>`) and ordering (`ORDER BY done_at DESC`) rely on this.
- **`slog` to stderr**; no stdout writes from handlers.

No external ADRs/specs — requirements are fully captured in the decisions above plus these in-repo references.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **`internal/github.Service`** (`service.go`) — the demand-driven per-repo cache. D-03 adds a new method + cache entry to it. The `Get` method (`service.go:109-146`) is the template: gate-ladder cache check → in-flight dedup → runner call → cache-store → return `Result`.
- **`internal/github.listPRs`** (`prlist.go:212`) — the single I/O primitive. D-05's new search reuses it with a different `--state` arg; the researcher verifies the exact flag + date field.
- **`internal/github.Result`** (`service.go:19`) — the wire contract D-02 mirrors (State/Stale/FetchedAt/PRs).
- **`PullRequestRoutes` GET handler** (`pullrequests.go:47-88`) — the degrade template (GATE 1 + GATE 2 + `svc.Get` + always-200).
- **`taskColumns`/`scanTask`** (`tasks.go:61-68`) — the task scan template (note: doesn't include `*_at`; D-12/discretion).
- **`writeJSON`/`writeError`** (`respond.go`) — the response helpers.
- **`settings.Get(db, settings.KeyGithubIntegration)`** — GATE 1 check (`pullrequests.go:56`).
- **`ghSvc` shared instance** (`serve.go:268`) — the new `ActivityRoutes` receives this same `*github.Service`.

### Established Patterns
- **Demand-driven per-repo cache** — no ticker goroutine; the browser poll is the only trigger. The new `GetMergedClosed` (D-03) follows this (the activity page's occasional view is the trigger).
- **In-flight dedup + drop-cache-after-N** — the `repoEntry` state machine; the new cache entry reuses it.
- **Two-gate ladder** — settings toggle + project linked before any `gh` spawn.
- **Degrade via `State` field, always 200** — never via HTTP status.
- **`source='manual'` board-leak filter** — excludes `github_pr` rows from task queries.
- **Single-writer SQLite** — parameterized `?`, collect-then-update.
- **Handler struct + `*Routes` function** — feature-scoped route registration (`PullRequestRoutes`, `WorktreeCleanupRoutes`, `UsageRoutes`).

### Integration Points
- **New `internal/api/activity.go`** — the `GET /api/activity` handler (combined response: tasks-done SQL + reviews aggregate + stats compute).
- **New method + cache entry in `internal/github`** — `GetMergedClosed` (D-03) + its per-repo cache (5min TTL, D-04) + the new `reviewed-by:@me --state closed` search (D-05).
- **New `ReviewDoneSummary` type** (D-06) — lives in `internal/github` (or `internal/api`); carries number/title/completedAt/url + project provenance.
- **New `ActivityRoutes(mux, db, ghSvc)` in `cmd/kamacu/serve.go`** — alongside `PullRequestRoutes` (`serve.go:269`), receiving the SAME `ghSvc`.
- **Tasks-done query** — `SELECT ... FROM tasks JOIN projects WHERE status='done' AND source='manual' AND done_at IS NOT NULL AND done_at >= ? AND <scope> ORDER BY done_at DESC` (D-12).
- **Stats compute** — in-Go over the tasks-done rows' `*_at` timestamps (D-10/D-11).

</code_context>

<specifics>
## Specific Ideas

- **This is a backend-only phase** — Phase 11 builds the page. Every decision here is about the API/data shape that Phase 11 consumes; no UI work.
- **The one genuinely open technical question (hand to `gsd-phase-researcher`):** the exact `gh` invocation for the merged/closed reviews search. Specifically: (a) does `gh pr list --search "reviewed-by:@me" --state closed` return BOTH merged AND closed PRs (GitHub's `--state closed` includes merged — verify live); (b) which `--json` date field maps to "completedAt" (likely `closedAt`, which is set for both merged and closed — verify `mergedAt` vs `closedAt` semantics); (c) whether the window filter on merge/close date should be done server-side via `--search "closed:>YYYY-MM-DD"` (cheaper, fewer results) or client-side after fetching (simpler, consistent with the existing fetch-everything-then-reduce pattern in `listPRs`). The researcher verifies against live `gh` (cf. how `reduceChecks` was verified against gh 2.82.0). This is the single riskiest detail — D-05 assumes `--state closed` covers both; if it doesn't, two searches (`is:merged` + `is:closed`) are needed.
- **Consistency with the review column's `Result` shape is deliberate** (D-02) — Phase 11's developer sees the same `{state, stale, fetchedAt, prs}` envelope, just under a `reviews` key in the combined response. No new mental model.
- **No double-counting** (D-12) — tasks-done is `source='manual'`; reviews-done is the `gh`-sourced list. A PR-review workspace moved to Done does NOT appear in tasks-done.
- **`ghSvc` reuse** — the new `GetMergedClosed` method lives on the existing shared `*github.Service`; the activity endpoint gets the same construction + cache benefits as the review column. No new service construction in `serve.go`.

</specifics>

<deferred>
## Deferred Ideas

- **Distinguishing merged vs closed in the reviews-done data** (D-05 treats both as "completed" with one `completedAt`). A future enhancement could add a state badge (Merged/Closed) per entry — would need `is:merged` + `is:closed` as separate searches or extra JSON fields. Not required by REVIEWS-02.
- **Custom date ranges / "All time" preset** — ACTFUT-01. Week/Month presets are enough for v1.12.
- **Throughput trend charts / bar graphs** — ACTFUT-02. v1.12 shows min/max/median as numbers only.
- **CSV/JSON export of activity data** — ACTFUT-03.
- **Per-agent or per-workspace breakdown statistics** — ACTFUT-04.
- **Activity for events beyond done/merged** (tasks moved to In Review, sessions started) — ACTFUT-05. v1.12 covers completed work only.
- **Time stats for reviews** — permanently out of scope (STATS-04: GitHub's merge signal has no Kamacu in-progress/in-review timestamps).
- **A `?refresh=1`-equivalent force flag on the activity endpoint** — D-04 mentions it; the exact param name + whether it propagates to every repo's `force` is the agent's discretion (follow the existing `?refresh=1` precedent on the review column endpoint).

None of these were pulled into Phase 10 — discussion stayed within the Activity Data & API boundary.

</deferred>

---

*Phase: 10-activity-data-api*
*Context gathered: 2026-07-29*
